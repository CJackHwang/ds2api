'use strict';

// Implementation moved here to keep the line-gate wrapper tiny.

const {
  createToolSieveState,
  processToolSieveChunk,
  flushToolSieve,
  parseStandaloneToolCalls,
  formatOpenAIStreamToolCalls,
} = require('../helpers/stream-tool-sieve');
const { BASE_HEADERS } = require('../shared/deepseek-constants');
const { writeOpenAIError, openAIErrorType } = require('./error_shape');
const { parseChunkForContent, isCitation } = require('./sse_parse');
const { buildUsage } = require('./token_usage');
const {
  resolveToolcallPolicy,
  formatIncrementalToolCallDeltas,
  filterIncrementalToolCallDeltasByAllowed,
  resetStreamToolCallState,
} = require('./toolcall_policy');
const { createChatCompletionEmitter, createDeltaCoalescer } = require('./stream_emitter');
const {
  asString,
  isAbortError,
  fetchStreamPrepare,
  fetchStreamPow,
  fetchStreamSwitch,
  relayPreparedFailure,
  createLeaseReleaser,
} = require('./http_internal');
const {
  trimContinuationOverlap,
} = require('./dedupe');

const DEEPSEEK_COMPLETION_URL = 'https://chat.deepseek.com/api/v0/chat/completion';
const DEEPSEEK_CONTINUE_URL = 'https://chat.deepseek.com/api/v0/chat/continue';
const EMPTY_OUTPUT_RETRY_SUFFIX = 'Previous reply had no visible output. Please regenerate the visible final answer or tool call now.';
const EMPTY_OUTPUT_RETRY_MAX_ATTEMPTS = 1;
const AUTO_CONTINUE_MAX_ROUNDS = 8;

async function handleVercelStream(req, res, rawBody, payload) {
  const prep = await fetchStreamPrepare(req, rawBody);
  if (!prep.ok) {
    relayPreparedFailure(res, prep);
    return;
  }

  const model = asString(prep.body.model) || asString(payload.model);
  const responseID = asString(prep.body.session_id) || `chatcmpl-${Date.now()}`;
  const leaseID = asString(prep.body.lease_id);
  let deepseekToken = asString(prep.body.deepseek_token);
  const initialPowHeader = asString(prep.body.pow_header);
  let completionPayload = prep.body.payload && typeof prep.body.payload === 'object' ? prep.body.payload : null;
  const finalPrompt = asString(prep.body.final_prompt);
  const thinkingEnabled = toBool(prep.body.thinking_enabled);
  const searchEnabled = toBool(prep.body.search_enabled);
  const toolPolicy = resolveToolcallPolicy(prep.body, payload.tools);
  const toolNames = toolPolicy.toolNames;
  const emitEarlyToolDeltas = toolPolicy.emitEarlyToolDeltas;
  const stripReferenceMarkers = true;

  if (!model || !leaseID || !deepseekToken || !initialPowHeader || !completionPayload) {
    writeOpenAIError(res, 500, 'invalid vercel prepare response');
    return;
  }

  const releaseLease = createLeaseReleaser(req, leaseID);
  const upstreamController = new AbortController();
  let clientClosed = false;
  let reader = null;
  const markClientClosed = () => {
    if (clientClosed) {
      return;
    }
    clientClosed = true;
    upstreamController.abort();
    if (reader && typeof reader.cancel === 'function') {
      Promise.resolve(reader.cancel()).catch(() => {});
    }
  };
  const onReqAborted = () => markClientClosed();
  const onResClose = () => {
    if (!res.writableEnded) {
      markClientClosed();
    }
  };
  req.on('aborted', onReqAborted);
  res.on('close', onResClose);

  try {
    let currentPowHeader = initialPowHeader;
    const refreshPowHeader = async (roundType) => {
      try {
        const pow = await fetchStreamPow(req, leaseID);
        const nextPowHeader = asString(pow.body && pow.body.pow_header);
        if (pow.ok && nextPowHeader) {
          currentPowHeader = nextPowHeader;
          return currentPowHeader;
        }
        console.warn('[vercel_stream_pow] refresh failed, reusing previous PoW', {
          round_type: roundType,
          status: pow.status || 0,
        });
      } catch (err) {
        if (clientClosed || isAbortError(err)) {
          return '';
        }
        console.warn('[vercel_stream_pow] refresh failed, reusing previous PoW', {
          round_type: roundType,
          error: err,
        });
      }
      return currentPowHeader;
    };

    const fetchDeepSeekStream = async (url, bodyPayload, powHeader) => {
      try {
        return await fetch(url, {
          method: 'POST',
          headers: {
            ...BASE_HEADERS,
            authorization: `Bearer ${deepseekToken}`,
            'x-ds-pow-response': powHeader,
          },
          body: JSON.stringify(bodyPayload),
          signal: upstreamController.signal,
        });
      } catch (err) {
        if (clientClosed || isAbortError(err)) {
          return null;
        }
        throw err;
      }
    };
    const fetchCompletion = (bodyPayload) => fetchDeepSeekStream(DEEPSEEK_COMPLETION_URL, bodyPayload, currentPowHeader);
    let activeDeepSeekSessionID = responseID;
    const fetchContinue = async (messageID) => {
      const powHeader = await refreshPowHeader('continue');
      if (!powHeader) {
        return null;
      }
      return fetchDeepSeekStream(DEEPSEEK_CONTINUE_URL, {
        chat_session_id: activeDeepSeekSessionID,
        message_id: messageID,
        fallback_to_resume: true,
      }, powHeader);
    };

    let completionRes = await fetchCompletion(completionPayload);
    if (completionRes === null) {
      return;
    }
    if (clientClosed) {
      return;
    }

    if (!completionRes.ok || !completionRes.body) {
      const detail = completionRes.body ? await completionRes.text() : '';
      const status = completionRes.ok ? 500 : completionRes.status || 500;
      writeOpenAIError(res, status, detail);
      return;
    }

    res.statusCode = 200;
    res.setHeader('Content-Type', 'text/event-stream');
    res.setHeader('Cache-Control', 'no-cache, no-transform');
    res.setHeader('Connection', 'keep-alive');
    res.setHeader('X-Accel-Buffering', 'no');
    if (typeof res.flushHeaders === 'function') {
      res.flushHeaders();
    }

    const created = Math.floor(Date.now() / 1000);
    let currentType = thinkingEnabled ? 'thinking' : 'text';
    let thinkingText = '';
    let outputText = '';
    let usagePrompt = finalPrompt;
    const toolSieveEnabled = toolPolicy.toolSieveEnabled;
    const toolSieveState = createToolSieveState();
    let toolCallsEmitted = false;
    let toolCallsDoneEmitted = false;
    const streamToolCallIDs = new Map();
    const streamToolNames = new Map();
    const decoder = new TextDecoder();
    let buffered = '';
    let ended = false;
    const { sendFrame, sendDeltaFrame } = createChatCompletionEmitter({
      res,
      sessionID: responseID,
      created,
      model,
      isClosed: () => clientClosed,
    });
    const deltaCoalescer = createDeltaCoalescer({ sendDeltaFrame });

    const finish = async (reason, options = {}) => {
      if (ended) {
        return true;
      }
      if (clientClosed || res.writableEnded || res.destroyed) {
        ended = true;
        await releaseLease();
        return true;
      }
      deltaCoalescer.flush();
      const detected = parseStandaloneToolCalls(outputText, toolNames);
      if (detected.length > 0 && !toolCallsDoneEmitted) {
        toolCallsEmitted = true;
        toolCallsDoneEmitted = true;
        sendDeltaFrame({ tool_calls: formatOpenAIStreamToolCalls(detected, streamToolCallIDs, payload.tools) });
      } else if (toolSieveEnabled) {
        const tailEvents = flushToolSieve(toolSieveState, toolNames);
        for (const evt of tailEvents) {
          if (evt.type === 'tool_calls' && Array.isArray(evt.calls) && evt.calls.length > 0) {
            deltaCoalescer.flush();
            toolCallsEmitted = true;
            toolCallsDoneEmitted = true;
            sendDeltaFrame({ tool_calls: formatOpenAIStreamToolCalls(evt.calls, streamToolCallIDs, payload.tools) });
            resetStreamToolCallState(streamToolCallIDs, streamToolNames);
            continue;
          }
          if (evt.text) {
            deltaCoalescer.append('content', evt.text);
          }
        }
        deltaCoalescer.flush();
      }
      if (detected.length > 0 || toolCallsEmitted) {
        reason = 'tool_calls';
      }
      if (detected.length === 0 && !toolCallsEmitted && outputText.trim() === '') {
        if (options.deferEmpty && reason !== 'content_filter') {
          return false;
        }
        ended = true;
        const detail = upstreamEmptyOutputDetail(reason === 'content_filter', outputText, thinkingText);
        sendFailedChunk(res, detail.status, detail.message, detail.code);
        await releaseLease();
        if (!res.writableEnded && !res.destroyed) {
          res.end();
        }
        return true;
      }
      ended = true;
      sendFrame({
        id: responseID,
        object: 'chat.completion.chunk',
        created,
        model,
        choices: [{ delta: {}, index: 0, finish_reason: reason }],
        usage: buildUsage(usagePrompt, thinkingText, outputText),
      });
      if (!res.writableEnded && !res.destroyed) {
        res.write('data: [DONE]\n\n');
      }
      await releaseLease();
      if (!res.writableEnded && !res.destroyed) {
        res.end();
      }
      return true;
    };

    const processStream = async (initialResponse, allowDeferEmpty) => {
      let currentResponse = initialResponse;
      let continueState = createContinueState(activeDeepSeekSessionID);
      let continueRounds = 0;
      // eslint-disable-next-line no-constant-condition
      while (true) {
        reader = currentResponse.body.getReader();
        buffered = '';
        let streamEnded = false;
        try {
          // eslint-disable-next-line no-constant-condition
          while (true) {
            if (clientClosed) {
              await finish('stop');
              return { terminal: true, retryable: false };
            }
            const { value, done } = await reader.read();
            if (done) {
              break;
            }
            buffered += decoder.decode(value, { stream: true });
            const lines = buffered.split('\n');
            buffered = lines.pop() || '';

            for (const rawLine of lines) {
              const line = rawLine.trim();
              if (!line.startsWith('data:')) {
                continue;
              }
              const dataStr = line.slice(5).trim();
              if (!dataStr) {
                continue;
              }
              if (dataStr === '[DONE]') {
                streamEnded = true;
                break;
              }
              let chunk;
              try {
                chunk = JSON.parse(dataStr);
              } catch (_err) {
                continue;
              }
              observeContinueState(continueState, chunk);
              const parsed = parseChunkForContent(chunk, thinkingEnabled, currentType, stripReferenceMarkers);
              if (!parsed.parsed) {
                continue;
              }
              currentType = parsed.newType;
              if (parsed.errorMessage) {
                return { terminal: await finish('content_filter'), retryable: false };
              }
              if (parsed.contentFilter) {
                return { terminal: await finish(outputText.trim() === '' ? 'content_filter' : 'stop'), retryable: false };
              }
              if (parsed.finished) {
                streamEnded = true;
                break;
              }

              for (const p of parsed.parts) {
                if (!p.text) {
                  continue;
                }
                if (p.type === 'thinking') {
                  if (thinkingEnabled) {
                    const trimmed = trimContinuationOverlap(thinkingText, p.text);
                    if (!trimmed) {
                      continue;
                    }
                    thinkingText += trimmed;
                    deltaCoalescer.append('reasoning_content', trimmed);
                  }
                } else {
                  const trimmed = trimContinuationOverlap(outputText, p.text);
                  if (!trimmed) {
                    continue;
                  }
                  if (searchEnabled && isCitation(trimmed)) {
                    continue;
                  }
                  outputText += trimmed;
                  if (!toolSieveEnabled) {
                    deltaCoalescer.append('content', trimmed);
                    continue;
                  }
                  const events = processToolSieveChunk(toolSieveState, trimmed, toolNames);
                  for (const evt of events) {
                    if (evt.type === 'tool_call_deltas') {
                      if (!emitEarlyToolDeltas) {
                        continue;
                      }
                      const filtered = filterIncrementalToolCallDeltasByAllowed(evt.deltas, toolNames, streamToolNames);
                      const formatted = formatIncrementalToolCallDeltas(filtered, streamToolCallIDs);
                      if (formatted.length > 0) {
                        toolCallsEmitted = true;
                        deltaCoalescer.flush();
                        sendDeltaFrame({ tool_calls: formatted });
                      }
                      continue;
                    }
                    if (evt.type === 'tool_calls') {
                      toolCallsEmitted = true;
                      toolCallsDoneEmitted = true;
                      deltaCoalescer.flush();
                      sendDeltaFrame({ tool_calls: formatOpenAIStreamToolCalls(evt.calls, streamToolCallIDs, payload.tools) });
                      resetStreamToolCallState(streamToolCallIDs, streamToolNames);
                      continue;
                    }
                    if (evt.text) {
                      deltaCoalescer.append('content', evt.text);
                    }
                  }
                }
              }
              if (streamEnded) {
                break;
              }
            }
            if (streamEnded) {
              break;
            }
          }
        } catch (err) {
          if (clientClosed || isAbortError(err)) {
            await finish('stop');
            return { terminal: true, retryable: false };
          }
          await finish('stop');
          return { terminal: true, retryable: false };
        }

        if (shouldAutoContinue(continueState) && continueRounds < AUTO_CONTINUE_MAX_ROUNDS) {
          continueRounds += 1;
          const nextRes = await fetchContinue(continueState.responseMessageID);
          if (nextRes === null) {
            return { terminal: true, retryable: false };
          }
          if (!nextRes.ok || !nextRes.body) {
            return { terminal: await finish('stop'), retryable: false };
          }
          continueState = prepareContinueStateForNextRound(continueState);
          currentResponse = nextRes;
          continue;
        }
        break;
      }

      const terminal = await finish('stop', { deferEmpty: allowDeferEmpty });
      return { terminal, retryable: !terminal && allowDeferEmpty, responseMessageID: continueState.responseMessageID };
    };

    let retryAttempts = 0;
    let accountSwitchAttempted = false;
    // eslint-disable-next-line no-constant-condition
    while (true) {
      const allowDeferEmpty = retryAttempts < EMPTY_OUTPUT_RETRY_MAX_ATTEMPTS || !accountSwitchAttempted;
      const processed = await processStream(completionRes, allowDeferEmpty);
      if (processed.terminal) {
        return;
      }
      if (!processed.retryable) {
        await finish('stop');
        return;
      }
      if (retryAttempts >= EMPTY_OUTPUT_RETRY_MAX_ATTEMPTS) {
        if (!accountSwitchAttempted) {
          accountSwitchAttempted = true;
          const switched = await fetchStreamSwitch(req, leaseID);
          if (switched.ok && switched.body && switched.body.payload && typeof switched.body.payload === 'object') {
            completionPayload = switched.body.payload;
            deepseekToken = asString(switched.body.deepseek_token) || deepseekToken;
            currentPowHeader = asString(switched.body.pow_header) || currentPowHeader;
            activeDeepSeekSessionID = asString(switched.body.session_id) || activeDeepSeekSessionID;
            usagePrompt = finalPrompt;
            completionRes = await fetchCompletion(completionPayload);
            if (completionRes === null) {
              return;
            }
            if (!completionRes.ok || !completionRes.body) {
              await finish('stop');
              return;
            }
            continue;
          }
        }
        await finish('stop');
        return;
      }
      retryAttempts += 1;
      console.info('[openai_empty_retry] attempting synthetic retry', {
        surface: 'chat.completions',
        stream: true,
        retry_attempt: retryAttempts,
        parent_message_id: processed.responseMessageID || 0,
      });
      usagePrompt = usagePromptWithEmptyOutputRetry(finalPrompt, retryAttempts);
      const retryPowHeader = await refreshPowHeader('retry');
      if (!retryPowHeader) {
        return;
      }
      completionRes = await fetchDeepSeekStream(
        DEEPSEEK_COMPLETION_URL,
        clonePayloadForEmptyOutputRetry(completionPayload, processed.responseMessageID),
        retryPowHeader,
      );
      if (completionRes === null) {
        return;
      }
      if (!completionRes.ok || !completionRes.body) {
        await finish('stop');
        return;
      }
    }
  } finally {
    req.removeListener('aborted', onReqAborted);
    res.removeListener('close', onResClose);
    await releaseLease();
  }
}

function toBool(v) {
  return v === true;
}

function clonePayloadForEmptyOutputRetry(payload, parentMessageID) {
  const clone = {
    ...(payload || {}),
    prompt: appendEmptyOutputRetrySuffix(asString(payload && payload.prompt)),
  };
  if (parentMessageID && parentMessageID > 0) {
    clone.parent_message_id = parentMessageID;
  }
  return clone;
}

function appendEmptyOutputRetrySuffix(prompt) {
  const base = asString(prompt).trimEnd();
  if (!base) {
    return EMPTY_OUTPUT_RETRY_SUFFIX;
  }
  return `${base}\n\n${EMPTY_OUTPUT_RETRY_SUFFIX}`;
}

function usagePromptWithEmptyOutputRetry(originalPrompt, attempts) {
  if (!attempts || attempts <= 0) {
    return originalPrompt;
  }
  const parts = [originalPrompt];
  let next = originalPrompt;
  for (let i = 0; i < attempts; i += 1) {
    next = appendEmptyOutputRetrySuffix(next);
    parts.push(next);
  }
  return parts.join('\n');
}

function createContinueState(sessionID) {
  return {
    sessionID: asString(sessionID),
    responseMessageID: 0,
    lastStatus: '',
    finished: false,
  };
}

function prepareContinueStateForNextRound(state) {
  return {
    ...state,
    lastStatus: '',
    finished: false,
  };
}

function observeContinueState(state, chunk) {
  if (!state || !chunk || typeof chunk !== 'object') {
    return;
  }
  const topID = numberValue(chunk.response_message_id);
  if (topID > 0) {
    state.responseMessageID = topID;
  }
  observeContinueDirectPatch(state, chunk.p, chunk.v);
  if (chunk.p === 'response') {
    observeContinueBatchPatches(state, 'response', chunk.v);
  } else {
    observeContinueBatchPatches(state, '', chunk.v);
  }
  const response = chunk.v && typeof chunk.v === 'object' ? chunk.v.response : null;
  observeContinueResponseObject(state, response);
  const messageResponse = chunk.message && typeof chunk.message === 'object' && chunk.message.response;
  observeContinueResponseObject(state, messageResponse);
}

function observeContinueDirectPatch(state, path, value) {
  if (!state) {
    return;
  }
  switch (asString(path).trim().replace(/^\/+|\/+$/g, '')) {
    case 'response/status':
    case 'status':
    case 'response/quasi_status':
    case 'quasi_status':
      setContinueStatus(state, asString(value));
      break;
    case 'response/auto_continue':
    case 'auto_continue':
      if (value === true) {
        state.lastStatus = 'AUTO_CONTINUE';
      }
      break;
    default:
      break;
  }
}

function observeContinueResponseObject(state, response) {
  if (!state || !response || typeof response !== 'object') {
    return;
  }
  const id = numberValue(response.message_id);
  if (id > 0) {
    state.responseMessageID = id;
  }
  setContinueStatus(state, asString(response.status));
  if (response.auto_continue === true) {
    state.lastStatus = 'AUTO_CONTINUE';
  }
}

function observeContinueBatchPatches(state, parentPath, raw) {
  if (!state || !Array.isArray(raw)) {
    return;
  }
  for (const patch of raw) {
    if (!patch || typeof patch !== 'object') {
      continue;
    }
    const path = asString(patch.p).trim();
    if (!path) {
      continue;
    }
    let fullPath = path;
    const parent = asString(parentPath).trim().replace(/^\/+|\/+$/g, '');
    if (parent && !path.includes('/')) {
      fullPath = `${parent}/${path}`;
    }
    switch (fullPath.replace(/^\/+|\/+$/g, '')) {
      case 'response/status':
      case 'status':
      case 'response/quasi_status':
      case 'quasi_status':
        setContinueStatus(state, asString(patch.v));
        break;
      case 'response/auto_continue':
      case 'auto_continue':
        if (patch.v === true) {
          state.lastStatus = 'AUTO_CONTINUE';
        }
        break;
      default:
        break;
    }
  }
}

function setContinueStatus(state, status) {
  const normalized = asString(status).trim();
  if (!normalized) {
    return;
  }
  state.lastStatus = normalized;
  if (['FINISHED', 'CONTENT_FILTER'].includes(normalized.toUpperCase())) {
    state.finished = true;
  }
}

function shouldAutoContinue(state) {
  if (!state || state.finished || !state.sessionID || state.responseMessageID <= 0) {
    return false;
  }
  return ['INCOMPLETE', 'AUTO_CONTINUE'].includes(asString(state.lastStatus).trim().toUpperCase());
}

function numberValue(v) {
  if (typeof v === 'number' && Number.isFinite(v)) {
    return Math.trunc(v);
  }
  const parsed = Number.parseInt(asString(v), 10);
  return Number.isFinite(parsed) ? parsed : 0;
}

function upstreamEmptyOutputDetail(contentFilter, _text, thinking) {
  if (contentFilter) {
    return {
      status: 400,
      message: 'Upstream content filtered the response and returned no output.',
      code: 'content_filter',
    };
  }
  if (thinking !== '') {
    return {
      status: 429,
      message: 'Upstream account hit a rate limit and returned reasoning without visible output.',
      code: 'upstream_empty_output',
    };
  }
  return {
    status: 503,
    message: 'Upstream service is unavailable and returned no output.',
    code: 'upstream_unavailable',
  };
}

function sendFailedChunk(res, status, message, code) {
  res.write(`data: ${JSON.stringify({
    status_code: status,
    error: {
      message,
      type: openAIErrorType(status),
      code,
      param: null,
    },
  })}\n\n`);
  if (!res.writableEnded && !res.destroyed) {
    res.write('data: [DONE]\n\n');
  }
  if (typeof res.flush === 'function') {
    res.flush();
  }
}

module.exports = {
  handleVercelStream,
};
