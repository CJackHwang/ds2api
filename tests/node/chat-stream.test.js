'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { EventEmitter } = require('node:events');

const handler = require('../../api/chat-stream.js');
const { handleVercelStream } = require('../../internal/js/chat-stream/vercel_stream.js');
const {
  createToolSieveState,
  processToolSieveChunk,
  flushToolSieve,
} = require('../../internal/js/helpers/stream-tool-sieve.js');
const {
  setCorsHeaders,
} = require('../../internal/js/chat-stream/http_internal.js');

const {
  parseChunkForContent,
  resolveToolcallPolicy,
  formatIncrementalToolCallDeltas,
  normalizePreparedToolNames,
  boolDefaultTrue,
  filterIncrementalToolCallDeltasByAllowed,
  resetStreamToolCallState,
  buildUsage,
  estimateTokens,
  shouldSkipPath,
  isNodeStreamSupportedPath,
  extractPathname,
  trimContinuationOverlap,
} = handler.__test;

function createMockResponse() {
  const headers = new Map();
  return {
    setHeader(key, value) {
      headers.set(String(key).toLowerCase(), value);
    },
    getHeader(key) {
      return headers.get(String(key).toLowerCase());
    },
  };
}

class MockStreamRequest extends EventEmitter {
  constructor() {
    super();
    this.url = '/v1/chat/completions';
    this.headers = { host: 'example.test', 'content-type': 'application/json' };
  }
}

class MockStreamResponse extends EventEmitter {
  constructor() {
    super();
    this.headers = new Map();
    this.statusCode = 0;
    this.chunks = [];
    this.writableEnded = false;
    this.destroyed = false;
  }

  setHeader(key, value) {
    this.headers.set(String(key).toLowerCase(), value);
  }

  getHeader(key) {
    return this.headers.get(String(key).toLowerCase());
  }

  write(chunk) {
    this.chunks.push(Buffer.isBuffer(chunk) ? chunk.toString('utf8') : String(chunk));
    return true;
  }

  end(chunk) {
    if (chunk) {
      this.write(chunk);
    }
    this.writableEnded = true;
  }

  flushHeaders() {}

  flush() {}

  bodyText() {
    return this.chunks.join('');
  }
}

function jsonResponse(body, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json' },
  });
}

function sseResponse(lines) {
  const encoder = new TextEncoder();
  return new Response(new ReadableStream({
    start(controller) {
      for (const line of lines) {
        controller.enqueue(encoder.encode(line));
      }
      controller.close();
    },
  }), {
    status: 200,
    headers: { 'content-type': 'text/event-stream' },
  });
}

function parseSSEDataFrames(body) {
  return body
    .split('\n\n')
    .map((frame) => frame.trim())
    .filter((frame) => frame.startsWith('data:'))
    .map((frame) => frame.slice(5).trim());
}

async function runMockVercelStream(upstreamLines, prepareOverrides = {}) {
  return runMockVercelStreamSequence([upstreamLines], prepareOverrides);
}

async function runMockVercelStreamSequence(upstreamSequences, prepareOverrides = {}) {
  const originalFetch = global.fetch;
  const fetchURLs = [];
  const fetchBodies = [];
  let completionCalls = 0;
  let continueCalls = 0;
  const prepareBody = {
    session_id: 'chatcmpl-test',
    lease_id: 'lease-test',
    model: 'gpt-test',
    final_prompt: 'hello',
    thinking_enabled: false,
    search_enabled: false,
    tool_names: [],
    deepseek_token: 'deepseek-token',
    pow_header: 'pow-header',
    payload: { prompt: 'hello' },
    ...prepareOverrides,
  };
  global.fetch = async (url, init = {}) => {
    const textURL = String(url);
    fetchURLs.push(textURL);
    if (init && init.body) {
      fetchBodies.push(JSON.parse(String(init.body)));
    }
    if (textURL.includes('__stream_prepare=1')) {
      return jsonResponse(prepareBody);
    }
    if (textURL.includes('__stream_pow=1')) {
      return jsonResponse({ pow_header: 'pow-header-refreshed' });
    }
    if (textURL.includes('__stream_release=1')) {
      return jsonResponse({ success: true });
    }
    if (textURL.includes('/continue')) {
      const idx = Math.min(continueCalls + 1, upstreamSequences.length - 1);
      continueCalls += 1;
      return sseResponse(upstreamSequences[idx]);
    }
    const idx = Math.min(completionCalls, upstreamSequences.length - 1);
    completionCalls += 1;
    return sseResponse(upstreamSequences[idx]);
  };
  try {
    const req = new MockStreamRequest();
    const res = new MockStreamResponse();
    const payload = { model: 'gpt-test', stream: true };
    await handleVercelStream(req, res, Buffer.from(JSON.stringify(payload)), payload);
    return { res, frames: parseSSEDataFrames(res.bodyText()), fetchURLs, fetchBodies };
  } finally {
    global.fetch = originalFetch;
  }
}

test('chat-stream exposes parser test hooks', () => {
  assert.equal(typeof parseChunkForContent, 'function');
  assert.equal(typeof resolveToolcallPolicy, 'function');
});

test('vercel stream emits Go-parity empty-output failure on DONE', async () => {
  const { frames } = await runMockVercelStream(['data: [DONE]\n\n']);
  assert.equal(frames.length, 2);
  const failed = JSON.parse(frames[0]);
  assert.equal(failed.status_code, 503);
  assert.equal(failed.error.type, 'service_unavailable_error');
  assert.equal(failed.error.code, 'upstream_unavailable');
  assert.equal(frames[1], '[DONE]');
});

test('vercel stream retries empty output once and keeps one terminal frame', async () => {
  const { frames, fetchURLs, fetchBodies } = await runMockVercelStreamSequence([
    ['data: [DONE]\n\n'],
    ['data: {"p":"response/content","v":"visible"}\n\n', 'data: [DONE]\n\n'],
  ]);
  const parsed = frames.filter((frame) => frame !== '[DONE]').map((frame) => JSON.parse(frame));
  const completionBodies = fetchBodies.filter((body) => Object.hasOwn(body, 'prompt'));
  assert.equal(fetchURLs.filter((url) => url === 'https://chat.deepseek.com/api/v0/chat/completion').length, 2);
  assert.equal(fetchURLs.filter((url) => url.includes('__stream_pow=1')).length, 1);
  assert.equal(frames.filter((frame) => frame === '[DONE]').length, 1);
  assert.equal(parsed[0].choices[0].delta.content, 'visible');
  assert.equal(parsed[1].choices[0].finish_reason, 'stop');
  assert.equal(parsed[0].id, parsed[1].id);
  assert.match(completionBodies[1].prompt, /Previous reply had no visible output\. Please regenerate the visible final answer or tool call now\.$/);
});

test('vercel stream retries thinking-only output once', async () => {
  const { frames, fetchURLs, fetchBodies } = await runMockVercelStreamSequence([
    ['data: {"response_message_id":42,"p":"response/thinking_content","v":"plan"}\n\n', 'data: [DONE]\n\n'],
    ['data: {"p":"response/content","v":"visible"}\n\n', 'data: [DONE]\n\n'],
  ], { thinking_enabled: true });
  const parsed = frames.filter((frame) => frame !== '[DONE]').map((frame) => JSON.parse(frame));
  const completionBodies = fetchBodies.filter((body) => Object.hasOwn(body, 'prompt'));
  assert.equal(fetchURLs.filter((url) => url === 'https://chat.deepseek.com/api/v0/chat/completion').length, 2);
  assert.equal(frames.filter((frame) => frame === '[DONE]').length, 1);
  assert.equal(completionBodies[1].parent_message_id, 42);
  assert.equal(parsed[0].choices[0].delta.reasoning_content, 'plan');
  assert.equal(parsed[1].choices[0].delta.content, 'visible');
  assert.equal(parsed[2].choices[0].finish_reason, 'stop');
});

test('vercel stream coalesces many small content deltas while keeping one choice', async () => {
  const lines = Array.from({ length: 100 }, () => `data: ${JSON.stringify({ p: 'response/content', v: '字' })}\n\n`);
  lines.push('data: [DONE]\n\n');
  const { frames } = await runMockVercelStream(lines);
  const parsed = frames.filter((frame) => frame !== '[DONE]').map((frame) => JSON.parse(frame));
  const contentFrames = parsed.filter((item) => item.choices?.[0]?.delta?.content);
  const content = contentFrames.map((item) => item.choices[0].delta.content).join('');
  assert.equal(content, '字'.repeat(100));
  assert.ok(contentFrames.length < 100, `expected fewer than 100 content frames, got ${contentFrames.length}`);
  for (const item of parsed) {
    assert.equal(item.choices.length, 1);
  }
});

test('vercel stream flushes reasoning before content and before stop', async () => {
  const { frames } = await runMockVercelStream([
    `data: ${JSON.stringify({ p: 'response/fragments', o: 'APPEND', v: [
      { type: 'THINK', content: '思考' },
      { type: 'THINK', content: '过程' },
      { type: 'RESPONSE', content: '回答' },
    ] })}\n\n`,
    'data: [DONE]\n\n',
  ], { thinking_enabled: true });
  const parsed = frames.filter((frame) => frame !== '[DONE]').map((frame) => JSON.parse(frame));
  const reasoning = parsed.map((item) => item.choices?.[0]?.delta?.reasoning_content || '').join('');
  const content = parsed.map((item) => item.choices?.[0]?.delta?.content || '').join('');
  assert.equal(reasoning, '思考过程');
  assert.equal(content, '回答');
  assert.equal(parsed.at(-1).choices[0].finish_reason, 'stop');
});

test('vercel stream exhausts DeepSeek continue before synthetic retry', async () => {
  const { frames, fetchURLs, fetchBodies } = await runMockVercelStreamSequence([
    [
      'data: {"response_message_id":7,"v":{"response":{"message_id":7,"status":"WIP","auto_continue":true}}}\n\n',
      'data: [DONE]\n\n',
    ],
    ['data: {"p":"response/content","v":"continued"}\n\n', 'data: [DONE]\n\n'],
  ]);
  const parsed = frames.filter((frame) => frame !== '[DONE]').map((frame) => JSON.parse(frame));
  assert.equal(fetchURLs.filter((url) => url === 'https://chat.deepseek.com/api/v0/chat/completion').length, 1);
  assert.equal(fetchURLs.filter((url) => url === 'https://chat.deepseek.com/api/v0/chat/continue').length, 1);
  assert.equal(fetchURLs.filter((url) => url.includes('__stream_pow=1')).length, 1);
  assert.equal(parsed[0].choices[0].delta.content, 'continued');
  assert.equal(parsed[1].choices[0].finish_reason, 'stop');
  assert.equal(fetchBodies.some((body) => String(body.prompt || '').includes('Previous reply had no visible output')), false);
});

test('vercel stream continues direct quasi_status incomplete before final tool call', async () => {
  const { frames, fetchURLs } = await runMockVercelStreamSequence([
    [
      'data: {"response_message_id":7,"p":"response/content","v":"<tool_calls><invoke name=\\"write_file\\"><parameter name=\\"content\\"><![CDATA[part-one"}\n\n',
      'data: {"p":"response/quasi_status","v":"INCOMPLETE"}\n\n',
      'data: [DONE]\n\n',
    ],
    [
      'data: {"response_message_id":8,"p":"response/content","v":"-part-two]]></parameter></invoke></tool_calls>"}\n\n',
      'data: {"p":"response/status","v":"FINISHED"}\n\n',
      'data: [DONE]\n\n',
    ],
  ], { tool_names: ['write_file'] });
  const parsed = frames.filter((frame) => frame !== '[DONE]').map((frame) => JSON.parse(frame));
  const toolDelta = parsed.find((item) => item.choices?.[0]?.delta?.tool_calls);
  assert.equal(fetchURLs.filter((url) => url === 'https://chat.deepseek.com/api/v0/chat/continue').length, 1);
  assert.ok(toolDelta);
  const args = JSON.parse(toolDelta.choices[0].delta.tool_calls[0].function.arguments);
  assert.equal(args.content, 'part-one-part-two');
  assert.equal(parsed.at(-1).choices[0].finish_reason, 'tool_calls');
});



test('vercel stream usage completion_tokens does not double-count visible output', async () => {
  const sample = 'abcdefghijklmnopqrst';
  const { frames } = await runMockVercelStream([
    `data: ${JSON.stringify({ p: 'response/content', v: sample })}\n\n`,
    'data: [DONE]\n\n',
  ]);
  const parsed = frames.filter((frame) => frame !== '[DONE]').map((frame) => JSON.parse(frame));
  const terminal = parsed.find((item) => Array.isArray(item.choices) && item.choices[0] && item.choices[0].finish_reason);
  assert.ok(terminal);
  assert.equal(terminal.usage.completion_tokens, 5);
});

test('vercel stream reuses prior PoW when refresh fails', async () => {
  const originalFetch = global.fetch;
  const fetchURLs = [];
  const completionPowHeaders = [];
  let completionCalls = 0;
  global.fetch = async (url, init = {}) => {
    const textURL = String(url);
    fetchURLs.push(textURL);
    if (textURL.includes('__stream_prepare=1')) {
      return jsonResponse({
        session_id: 'chatcmpl-test',
        lease_id: 'lease-test',
        model: 'gpt-test',
        final_prompt: 'hello',
        thinking_enabled: false,
        search_enabled: false,
        tool_names: [],
        deepseek_token: 'deepseek-token',
        pow_header: 'pow-header-initial',
        payload: { prompt: 'hello' },
      });
    }
    if (textURL.includes('__stream_pow=1')) {
      return jsonResponse({}, 500);
    }
    if (textURL.includes('__stream_release=1')) {
      return jsonResponse({ success: true });
    }
    if (textURL === 'https://chat.deepseek.com/api/v0/chat/completion') {
      completionPowHeaders.push(init.headers['x-ds-pow-response']);
      completionCalls += 1;
      if (completionCalls === 1) {
        return sseResponse(['data: [DONE]\n\n']);
      }
      return sseResponse(['data: {"p":"response/content","v":"visible"}\n\n', 'data: [DONE]\n\n']);
    }
    throw new Error(`unexpected fetch url: ${textURL}`);
  };
  try {
    const req = new MockStreamRequest();
    const res = new MockStreamResponse();
    const payload = { model: 'gpt-test', stream: true };
    await handleVercelStream(req, res, Buffer.from(JSON.stringify(payload)), payload);
    const frames = parseSSEDataFrames(res.bodyText());
    const parsed = frames.filter((frame) => frame !== '[DONE]').map((frame) => JSON.parse(frame));
    assert.deepEqual(completionPowHeaders, ['pow-header-initial', 'pow-header-initial']);
    assert.equal(fetchURLs.filter((url) => url.includes('__stream_pow=1')).length, 1);
    assert.equal(parsed[0].choices[0].delta.content, 'visible');
    assert.equal(parsed[1].choices[0].finish_reason, 'stop');
  } finally {
    global.fetch = originalFetch;
  }
});

test('vercel stream emits content_filter failure when upstream filters empty output', async () => {
  const { frames } = await runMockVercelStream(['data: {"code":"content_filter"}\n\n']);
  assert.equal(frames.length, 2);
  const failed = JSON.parse(frames[0]);
  assert.equal(failed.status_code, 400);
  assert.equal(failed.error.type, 'invalid_request_error');
  assert.equal(failed.error.code, 'content_filter');
  assert.equal(frames[1], '[DONE]');
});

test('vercel stream keeps stop finish when content_filter arrives after visible text', async () => {
  const { frames } = await runMockVercelStream([
    'data: {"p":"response/content","v":"hello"}\n\n',
    'data: {"code":"content_filter"}\n\n',
  ]);
  const parsed = frames.filter((frame) => frame !== '[DONE]').map((frame) => JSON.parse(frame));
  assert.equal(parsed[0].choices[0].delta.content, 'hello');
  assert.equal(parsed[1].choices[0].finish_reason, 'stop');
  assert.equal(parsed[1].usage.completion_tokens, 1);
});

test('resolveToolcallPolicy defaults to feature-match + early emit when prepare flags missing', () => {
  const policy = resolveToolcallPolicy(
    {},
    [{ type: 'function', function: { name: 'read_file', parameters: { type: 'object' } } }],
  );
  assert.deepEqual(policy.toolNames, ['read_file']);
  assert.equal(policy.toolSieveEnabled, true);
  assert.equal(policy.emitEarlyToolDeltas, true);
});

test('resolveToolcallPolicy ignores prepare flags and keeps early emit enabled', () => {
  const policy = resolveToolcallPolicy(
    {
      tool_names: [' prepped_tool ', '', null],
      toolcall_feature_match: false,
      toolcall_early_emit_high: false,
    },
    [{ type: 'function', function: { name: 'fallback_tool', parameters: { type: 'object' } } }],
  );
  assert.deepEqual(policy.toolNames, ['prepped_tool']);
  assert.equal(policy.toolSieveEnabled, true);
  assert.equal(policy.emitEarlyToolDeltas, true);
});

test('normalizePreparedToolNames filters empty values', () => {
  assert.deepEqual(normalizePreparedToolNames([' a ', '', null, 'b']), ['a', 'b']);
});

test('boolDefaultTrue keeps false only when explicitly false', () => {
  assert.equal(boolDefaultTrue(false), false);
  assert.equal(boolDefaultTrue(true), true);
  assert.equal(boolDefaultTrue(undefined), true);
});

test('filterIncrementalToolCallDeltasByAllowed keeps unknown name and follow-up args', () => {
  const seen = new Map();
  const filtered = filterIncrementalToolCallDeltasByAllowed(
    [
      { index: 0, name: 'not_in_schema' },
      { index: 0, arguments: '{"x":1}' },
    ],
    ['read_file'],
    seen,
  );
  assert.deepEqual(filtered, [
    { index: 0, name: 'not_in_schema' },
    { index: 0, arguments: '{"x":1}' },
  ]);
  assert.equal(seen.get(0), 'not_in_schema');
});

test('filterIncrementalToolCallDeltasByAllowed keeps allowed name and args', () => {
  const seen = new Map();
  const filtered = filterIncrementalToolCallDeltasByAllowed(
    [
      { index: 0, name: 'read_file' },
      { index: 0, arguments: '{"path":"README.MD"}' },
    ],
    ['read_file'],
    seen,
  );
  assert.deepEqual(filtered, [
    { index: 0, name: 'read_file' },
    { index: 0, arguments: '{"path":"README.MD"}' },
  ]);
});

test('incremental and final tool formatting share stable id via idStore', () => {
  const idStore = new Map();
  const incremental = formatIncrementalToolCallDeltas([{ index: 0, name: 'read_file' }], idStore);
  const { formatOpenAIStreamToolCalls } = require('../../internal/js/helpers/stream-tool-sieve.js');
  const finalCalls = formatOpenAIStreamToolCalls([{ name: 'read_file', input: { path: 'README.MD' } }], idStore);
  assert.equal(incremental.length, 1);
  assert.equal(finalCalls.length, 1);
  assert.equal(incremental[0].id, finalCalls[0].id);
});

test('resetStreamToolCallState gives each completed block a fresh id', () => {
  const idStore = new Map();
  const first = formatIncrementalToolCallDeltas([{ index: 0, name: 'read_file' }], idStore);
  resetStreamToolCallState(idStore);
  const second = formatIncrementalToolCallDeltas([{ index: 0, name: 'search' }], idStore);
  assert.equal(first.length, 1);
  assert.equal(second.length, 1);
  assert.notEqual(first[0].id, second[0].id);
});

test('formatIncrementalToolCallDeltas drops empty deltas (Go parity)', () => {
  const idStore = new Map();
  const formatted = formatIncrementalToolCallDeltas([{ index: 0 }], idStore);
  assert.deepEqual(formatted, []);
});

test('parseChunkForContent keeps split response/content fragments inside response array', () => {
  const chunk = {
    p: 'response',
    v: [
      { p: 'response/content', v: '{"' },
      { p: 'response/content', v: 'tool_calls":[{"name":"read_file","input":{"path":"README.MD"}}]}' },
    ],
  };
  const parsed = parseChunkForContent(chunk, false, 'text');
  assert.equal(parsed.finished, false);
  assert.equal(parsed.newType, 'text');
  assert.equal(parsed.parts.length, 2);
  const combined = parsed.parts.map((p) => p.text).join('');
  assert.equal(combined, '{"tool_calls":[{"name":"read_file","input":{"path":"README.MD"}}]}');
});

test('parseChunkForContent + sieve passes JSON tool payload through as text (XML-only)', () => {
  const chunk = {
    p: 'response',
    v: [
      { p: 'response/content', v: '{"' },
      { p: 'response/content', v: 'tool_calls":[{"name":"read_file","input":{"path":"README.MD"}}]}' },
    ],
  };
  const parsed = parseChunkForContent(chunk, false, 'text');
  const state = createToolSieveState();
  const events = [];
  for (const part of parsed.parts) {
    events.push(...processToolSieveChunk(state, part.text, ['read_file']));
  }
  events.push(...flushToolSieve(state, ['read_file']));

  const hasToolCalls = events.some((evt) => evt.type === 'tool_calls' && evt.calls && evt.calls.length > 0);
  const leakedText = events
    .filter((evt) => evt.type === 'text' && evt.text)
    .map((evt) => evt.text)
    .join('');

  // JSON payloads are no longer intercepted — they pass through as text.
  assert.equal(hasToolCalls, false);
  assert.equal(leakedText.includes('tool_calls'), true);
});

test('parseChunkForContent consumes nested item.v array payloads', () => {
  const chunk = {
    p: 'response',
    v: [
      { p: 'response/content', v: ['A', 'B'] },
      { p: 'response/content', v: [{ content: 'C', type: 'RESPONSE' }] },
    ],
  };
  const parsed = parseChunkForContent(chunk, false, 'text');
  assert.equal(parsed.finished, false);
  assert.equal(parsed.parts.map((p) => p.text).join(''), 'ABC');
});

test('parseChunkForContent detects nested status FINISHED in array payload', () => {
  const chunk = {
    p: 'response',
    v: [{ p: 'status', v: 'FINISHED' }],
  };
  const parsed = parseChunkForContent(chunk, false, 'text');
  assert.equal(parsed.finished, true);
  assert.deepEqual(parsed.parts, []);
});

test('parseChunkForContent ignores items without v to match Go parser behavior', () => {
  const chunk = {
    p: 'response',
    v: [{ type: 'RESPONSE', content: 'no-v-content' }],
  };
  const parsed = parseChunkForContent(chunk, false, 'text');
  assert.equal(parsed.finished, false);
  assert.deepEqual(parsed.parts, []);
});

test('parseChunkForContent handles response/fragments APPEND with thinking and response transitions', () => {
  const chunk = {
    p: 'response/fragments',
    o: 'APPEND',
    v: [
      { type: 'THINK', content: '思考中' },
      { type: 'RESPONSE', content: '结论' },
    ],
  };
  const parsed = parseChunkForContent(chunk, true, 'thinking');
  assert.equal(parsed.finished, false);
  assert.equal(parsed.newType, 'text');
  assert.deepEqual(parsed.parts, [
    { text: '思考中', type: 'thinking' },
    { text: '结论', type: 'text' },
  ]);
});

test('parseChunkForContent drops thinking content when thinking is disabled', () => {
  const thinking = parseChunkForContent(
    { p: 'response/thinking_content', v: 'hidden thought' },
    false,
    'text',
  );
  assert.equal(thinking.finished, false);
  assert.equal(thinking.newType, 'thinking');
  assert.deepEqual(thinking.parts, []);

  const hiddenContinuation = parseChunkForContent(
    { v: 'still hidden' },
    false,
    thinking.newType,
  );
  assert.equal(hiddenContinuation.newType, 'thinking');
  assert.deepEqual(hiddenContinuation.parts, []);

  const answer = parseChunkForContent(
    { p: 'response/content', v: 'visible answer' },
    false,
    hiddenContinuation.newType,
  );
  assert.equal(answer.newType, 'text');
  assert.deepEqual(answer.parts, [{ text: 'visible answer', type: 'text' }]);
});

test('parseChunkForContent supports wrapped response.fragments object shape', () => {
  const chunk = {
    p: 'response',
    v: {
      response: {
        fragments: [
          { type: 'RESPONSE', content: 'A' },
          { type: 'RESPONSE', content: 'B' },
        ],
      },
    },
  };
  const parsed = parseChunkForContent(chunk, false, 'text');
  assert.equal(parsed.finished, false);
  assert.equal(parsed.parts.map((p) => p.text).join(''), 'AB');
});

test('parseChunkForContent reads object-shaped response/content payloads (Go parity)', () => {
  const parsed = parseChunkForContent({ p: 'response/content', v: { text: 'vision text' } }, false, 'text', true);
  assert.equal(parsed.parsed, true);
  assert.deepEqual(parsed.parts, [{ text: 'vision text', type: 'text' }]);
});

test('parseChunkForContent preserves space-only content tokens', () => {
  const chunk = {
    p: 'response/content',
    v: ' ',
  };
  const parsed = parseChunkForContent(chunk, false, 'text');
  assert.equal(parsed.finished, false);
  assert.deepEqual(parsed.parts, [{ text: ' ', type: 'text' }]);
});

test('parseChunkForContent strips citation and reference markers from fragment content', () => {
  const chunk = {
    p: 'response/fragments',
    o: 'APPEND',
    v: [
      { type: 'RESPONSE', content: '广州天气 [citation:1] [reference:12] 多云' },
    ],
  };
  const parsed = parseChunkForContent(chunk, false, 'text');
  assert.equal(parsed.finished, false);
  assert.deepEqual(parsed.parts, [{ text: '广州天气   多云', type: 'text' }]);
});

test('parseChunkForContent strips leaked thought control markers from content', () => {
  const chunk = {
    p: 'response/content',
    v: '<｜▁of▁thought｜>A<| of_thought |>B<| end_of_thought |>C',
  };
  const parsed = parseChunkForContent(chunk, false, 'text');
  assert.equal(parsed.finished, false);
  assert.deepEqual(parsed.parts, [{ text: 'ABC', type: 'text' }]);
});

test('parseChunkForContent detects content_filter status and ignores upstream output tokens', () => {
  const chunk = {
    p: 'response',
    v: [
      { p: 'status', v: 'CONTENT_FILTER' },
      { p: 'accumulated_token_usage', v: 77 },
    ],
  };
  const parsed = parseChunkForContent(chunk, false, 'text');
  assert.equal(parsed.parsed, true);
  assert.equal(parsed.finished, true);
  assert.equal(parsed.contentFilter, true);
  assert.equal(parsed.outputTokens, 0);
  assert.deepEqual(parsed.parts, []);
});

test('parseChunkForContent keeps error branches distinct from content_filter status', () => {
  const chunk = {
    error: { message: 'boom' },
    code: 'content_filter',
    accumulated_token_usage: 88,
  };
  const parsed = parseChunkForContent(chunk, false, 'text');
  assert.equal(parsed.parsed, true);
  assert.equal(parsed.finished, true);
  assert.equal(parsed.contentFilter, false);
  assert.equal(parsed.errorMessage.length > 0, true);
  assert.equal(parsed.outputTokens, 0);
  assert.deepEqual(parsed.parts, []);
});

test('parseChunkForContent ignores output tokens on FINISHED lines', () => {
  const parsed = parseChunkForContent(
    { p: 'response/status', v: 'FINISHED', accumulated_token_usage: 190 },
    false,
    'text',
  );
  assert.equal(parsed.parsed, true);
  assert.equal(parsed.finished, true);
  assert.equal(parsed.contentFilter, false);
  assert.equal(parsed.outputTokens, 0);
  assert.deepEqual(parsed.parts, []);
});

test('parseChunkForContent ignores output tokens from response BATCH status snapshots', () => {
  const parsed = parseChunkForContent(
    {
      p: 'response',
      o: 'BATCH',
      v: [
        { p: 'accumulated_token_usage', v: 190 },
        { p: 'quasi_status', v: 'FINISHED' },
      ],
    },
    false,
    'text',
  );
  assert.equal(parsed.parsed, true);
  assert.equal(parsed.finished, false);
  assert.equal(parsed.contentFilter, false);
  assert.equal(parsed.outputTokens, 0);
  assert.deepEqual(parsed.parts, []);
});

test('parseChunkForContent matches FINISHED case-insensitively on status paths', () => {
  const parsed = parseChunkForContent(
    { p: 'response/status', v: ' finished ', accumulated_token_usage: 190 },
    false,
    'text',
  );
  assert.equal(parsed.parsed, true);
  assert.equal(parsed.finished, true);
  assert.equal(parsed.contentFilter, false);
  assert.equal(parsed.outputTokens, 0);
  assert.deepEqual(parsed.parts, []);
});

test('parseChunkForContent filters INCOMPLETE status text without stopping stream', () => {
  const parsed = parseChunkForContent(
    { p: 'response/status', v: 'INCOMPLETE', accumulated_token_usage: 190 },
    false,
    'text',
  );
  assert.equal(parsed.parsed, true);
  assert.equal(parsed.finished, false);
  assert.equal(parsed.contentFilter, false);
  assert.equal(parsed.outputTokens, 0);
  assert.deepEqual(parsed.parts, []);
});

test('parseChunkForContent strips leaked CONTENT_FILTER suffix and preserves line breaks', () => {
  const leaked = parseChunkForContent(
    { p: 'response/content', v: '正常输出CONTENT_FILTER你好，这个问题我暂时无法回答' },
    false,
    'text',
  );
  assert.deepEqual(leaked.parts, [{ text: '正常输出', type: 'text' }]);

  const newlineTail = parseChunkForContent(
    { p: 'response/content', v: 'line1\nCONTENT_FILTERblocked' },
    false,
    'text',
  );
  assert.deepEqual(newlineTail.parts, [{ text: 'line1\n', type: 'text' }]);

  const newlineOnly = parseChunkForContent(
    { p: 'response/content', v: '\nCONTENT_FILTERblocked' },
    false,
    'text',
  );
  assert.deepEqual(newlineOnly.parts, [{ text: '\n', type: 'text' }]);
});

test('estimateTokens preserves whitespace-only strings and buildUsage accepts output token overrides', () => {
  assert.equal(estimateTokens('   '), 1);
  assert.equal(estimateTokens('\n'), 1);

  const usage = buildUsage('abcd', 'ef', 'gh', 99);
  assert.equal(usage.prompt_tokens, 1);
  assert.equal(usage.completion_tokens, 99);
  assert.equal(usage.total_tokens, 100);
  assert.equal(usage.completion_tokens_details.reasoning_tokens, 1);
});

test('shouldSkipPath skips dynamic response/fragments/*/status paths only', () => {
  assert.equal(shouldSkipPath('response/fragments/-16/status'), true);
  assert.equal(shouldSkipPath('response/fragments/8/status'), true);
  assert.equal(shouldSkipPath('response/status'), false);
});

test('node stream path guard allows OpenAI v1 and root alias chat completions paths', () => {
  assert.equal(isNodeStreamSupportedPath('/v1/chat/completions'), true);
  assert.equal(isNodeStreamSupportedPath('/v1/chat/completions?x=1'), true);
  assert.equal(isNodeStreamSupportedPath('/chat/completions'), true);
  assert.equal(isNodeStreamSupportedPath('/chat/completions?x=1'), true);
  assert.equal(isNodeStreamSupportedPath('/v1beta/models/gemini-2.5-flash:streamGenerateContent'), false);
  assert.equal(isNodeStreamSupportedPath('/anthropic/v1/messages'), false);
});

test('extractPathname strips query only', () => {
  assert.equal(extractPathname('/v1/chat/completions?stream=true'), '/v1/chat/completions');
  assert.equal(extractPathname('/v1beta/models/gemini-2.5-flash:streamGenerateContent?key=1'), '/v1beta/models/gemini-2.5-flash:streamGenerateContent');
  assert.equal(extractPathname('/chat/completions?stream=true'), '/chat/completions');
});

test('setCorsHeaders reflects requested third-party headers and blocks internal-only headers', () => {
  const res = createMockResponse();
  setCorsHeaders(res, {
    headers: {
      origin: 'app://obsidian.md',
      'access-control-request-headers': 'authorization, x-stainless-os, x-stainless-runtime, x-ds2-internal-token',
      'access-control-request-private-network': 'true',
    },
  });

  assert.equal(res.getHeader('access-control-allow-origin'), 'app://obsidian.md');
  assert.equal(res.getHeader('access-control-allow-private-network'), 'true');
  assert.equal(res.getHeader('access-control-max-age'), '600');

  const allowHeaders = String(res.getHeader('access-control-allow-headers') || '').toLowerCase();
  assert.equal(allowHeaders.includes('authorization'), true);
  assert.equal(allowHeaders.includes('x-stainless-os'), true);
  assert.equal(allowHeaders.includes('x-stainless-runtime'), true);
  assert.equal(allowHeaders.includes('x-ds2-internal-token'), false);

  const vary = String(res.getHeader('vary') || '').toLowerCase();
  assert.equal(vary.includes('origin'), true);
  assert.equal(vary.includes('access-control-request-headers'), true);
  assert.equal(vary.includes('access-control-request-private-network'), true);
});

test('trimContinuationOverlap preserves short normal tokens and trims long snapshots', () => {
  assert.equal(trimContinuationOverlap('我们被问到', '我们'), '我们');
  const existing = '我们被问到：这是一个很长的续答快照前缀，用来验证去重逻辑不会误伤正常 token。';
  const incoming = `${existing}继续分析`;
  assert.equal(trimContinuationOverlap(existing, incoming), '继续分析');
});
