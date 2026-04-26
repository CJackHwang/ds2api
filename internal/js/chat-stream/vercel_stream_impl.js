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
const { writeOpenAIError } = require('./error_shape');
const { parseChunkForContent, isCitation } = require('./sse_parse');
const { buildUsage } = require('./token_usage');
const {
  resolveToolcallPolicy,
  formatIncrementalToolCallDeltas,
  filterIncrementalToolCallDeltasByAllowed,
  boolDefaultTrue,
  resetStreamToolCallState,
} = require('./toolcall_policy');
const { createChatCompletionEmitter } = require('./stream_emitter');
const {
  asString,
  isAbortError,
  fetchStreamPrepare,
  relayPreparedFailure,
  createLeaseReleaser,
} = require('./http_internal');
const {
  trimContinuationOverlap,
} = require('./dedupe');

const DEEPSEEK_COMPLETION_URL = 'https://chat.deepseek.com/api/v0/chat/completion';
const LEAKED_TOOL_WRAPPER_FRAGMENT_PATTERN = /^\s*(?:<\/tool_calls>|<tool_calls>|tool_calls>)\s*$/i;

function sanitizeVisibleChunkText(text) {
  const value = typeof text === 'string' ? text : '';
  if (!value) {
    return '';
  }
  if (LEAKED_TOOL_WRAPPER_FRAGMENT_PATTERN.test(value)) {
    return '';
  }
  return value;
}

function toolCallKey(call) {
  if (!call || typeof call !== 'object') {
    return '';
  }
  const name = typeof call.name === 'string' ? call.name.trim() : '';
  const input = call.input && typeof call.input === 'object' && !Array.isArray(call.input) ? call.input : {};
  try {
    return `${name}::${JSON.stringify(input)}`;
  } catch (_err) {
    return `${name}::`;
  }
}

function filterNewToolCalls(calls, emittedKeys) {
  if (!Array.isArray(calls) || calls.length === 0) {
    return [];
  }
  const out = [];
  for (const call of calls) {
    const key = toolCallKey(call);
    if (!key) {
      out.push(call);
      continue;
    }
    if (emittedKeys && emittedKeys.has(key)) {
      continue;
    }
    if (emittedKeys) {
      emittedKeys.add(key);
    }
    out.push(call);
  }
  return out;
}

async function handleVercelStream(req, res, rawBody, payload) {
  const prep = await fetchStreamPrepare(req, rawBody);
  if (!prep.ok) {
    relayPreparedFailure(res, prep);
    return;
  }

  const model = asString(prep.body.model) || asString(payload.model);
  const sessionID = asString(prep.body.session_id) || `chatcmpl-${Date.now()}`;
  const leaseID = asString(prep.body.lease_id);
  const deepseekToken = asString(prep.body.deepseek_token);
  const powHeader = asString(prep.body.pow_header);
  const completionPayload = prep.body.payload && typeof prep.body.payload === 'object' ? prep.body.payload : null;
  const finalPrompt = asString(prep.body.final_prompt);
  const thinkingEnabled = toBool(prep.body.thinking_enabled);
  const searchEnabled = toBool(prep.body.search_enabled);
  const toolPolicy = resolveToolcallPolicy(prep.body, payload.tools);
  const toolNames = toolPolicy.toolNames;
  const emitEarlyToolDeltas = toolPolicy.emitEarlyToolDeltas;
  const stripReferenceMarkers = boolDefaultTrue(prep.body.compat && prep.body.compat.strip_reference_markers);

  if (!model || !leaseID || !deepseekToken || !powHeader || !completionPayload) {
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
    let completionRes;
    try {
      completionRes = await fetch(DEEPSEEK_COMPLETION_URL, {
        method: 'POST',
        headers: {
          ...BASE_HEADERS,
          authorization: `Bearer ${deepseekToken}`,
          'x-ds-pow-response': powHeader,
        },
        body: JSON.stringify(completionPayload),
        signal: upstreamController.signal,
      });
    } catch (err) {
      if (clientClosed || isAbortError(err)) {
        return;
      }
      throw err;
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
    const toolSieveEnabled = toolPolicy.toolSieveEnabled;
    const toolSieveState = createToolSieveState();
    let toolCallsEmitted = false;
    const emittedToolCallKeys = new Set();
    const streamToolCallIDs = new Map();
    const streamToolNames = new Map();
    const decoder = new TextDecoder();
    reader = completionRes.body.getReader();
    let buffered = '';
    let ended = false;
    const { sendFrame, sendDeltaFrame } = createChatCompletionEmitter({
      res,
      sessionID,
      created,
      model,
      isClosed: () => clientClosed,
    });

    const finish = async (reason) => {
      if (ended) {
        return;
      }
      ended = true;
      if (clientClosed || res.writableEnded || res.destroyed) {
        await releaseLease();
        return;
      }
      const detected = filterNewToolCalls(parseStandaloneToolCalls(outputText, toolNames), emittedToolCallKeys);
      if (detected.length > 0) {
        toolCallsEmitted = true;
        sendDeltaFrame({ tool_calls: formatOpenAIStreamToolCalls(detected, streamToolCallIDs) });
      } else if (toolSieveEnabled) {
        const tailEvents = flushToolSieve(toolSieveState, toolNames);
        for (const evt of tailEvents) {
          if (evt.type === 'tool_calls' && Array.isArray(evt.calls) && evt.calls.length > 0) {
            const filteredCalls = filterNewToolCalls(evt.calls, emittedToolCallKeys);
            if (filteredCalls.length === 0) {
              continue;
            }
            toolCallsEmitted = true;
            sendDeltaFrame({ tool_calls: formatOpenAIStreamToolCalls(filteredCalls, streamToolCallIDs) });
            resetStreamToolCallState(streamToolCallIDs, streamToolNames);
            continue;
          }
          const cleaned = sanitizeVisibleChunkText(evt.text);
          if (cleaned) {
            sendDeltaFrame({ content: cleaned });
          }
        }
      }
      if (detected.length > 0 || toolCallsEmitted) {
        reason = 'tool_calls';
      }
      sendFrame({
        id: sessionID,
        object: 'chat.completion.chunk',
        created,
        model,
        choices: [{ delta: {}, index: 0, finish_reason: reason }],
        usage: buildUsage(finalPrompt, thinkingText, outputText),
      });
      if (!res.writableEnded && !res.destroyed) {
        res.write('data: [DONE]\n\n');
      }
      await releaseLease();
      if (!res.writableEnded && !res.destroyed) {
        res.end();
      }
    };

    try {
      // eslint-disable-next-line no-constant-condition
      while (true) {
        if (clientClosed) {
          await finish('stop');
          return;
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
            await finish('stop');
            return;
          }
          let chunk;
          try {
            chunk = JSON.parse(dataStr);
          } catch (_err) {
            continue;
          }
          const parsed = parseChunkForContent(chunk, thinkingEnabled, currentType, stripReferenceMarkers);
          if (!parsed.parsed) {
            continue;
          }
          currentType = parsed.newType;
          if (parsed.errorMessage) {
            await finish('content_filter');
            return;
          }
          if (parsed.contentFilter) {
            await finish('stop');
            return;
          }
          if (parsed.finished) {
            await finish('stop');
            return;
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
                sendDeltaFrame({ reasoning_content: trimmed });
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
                sendDeltaFrame({ content: trimmed });
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
                    sendDeltaFrame({ tool_calls: formatted });
                  }
                  continue;
                }
                if (evt.type === 'tool_calls') {
                  const filteredCalls = filterNewToolCalls(evt.calls, emittedToolCallKeys);
                  if (filteredCalls.length === 0) {
                    continue;
                  }
                  toolCallsEmitted = true;
                  sendDeltaFrame({ tool_calls: formatOpenAIStreamToolCalls(filteredCalls, streamToolCallIDs) });
                  resetStreamToolCallState(streamToolCallIDs, streamToolNames);
                  continue;
                }
                const cleaned = sanitizeVisibleChunkText(evt.text);
                if (cleaned) {
                  sendDeltaFrame({ content: cleaned });
                }
              }
            }
          }
        }
      }
      await finish('stop');
    } catch (err) {
      if (clientClosed || isAbortError(err)) {
        await finish('stop');
        return;
      }
      await finish('stop');
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

module.exports = {
  handleVercelStream,
};
