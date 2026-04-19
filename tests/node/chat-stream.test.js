'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');

const handler = require('../../api/chat-stream.js');
const {
  createToolSieveState,
  processToolSieveChunk,
  flushToolSieve,
} = require('../../internal/js/helpers/stream-tool-sieve.js');

const {
  parseChunkForContent,
  resolveToolcallPolicy,
  formatIncrementalToolCallDeltas,
  normalizePreparedToolNames,
  boolDefaultTrue,
  filterIncrementalToolCallDeltasByAllowed,
  buildUsage,
  estimateTokens,
  shouldSkipPath,
  isNodeStreamSupportedPath,
  extractPathname,
  trimContinuationOverlap,
} = handler.__test;

test('chat-stream exposes parser test hooks', () => {
  assert.equal(typeof parseChunkForContent, 'function');
  assert.equal(typeof resolveToolcallPolicy, 'function');
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

test('parseChunkForContent preserves space-only content tokens', () => {
  const chunk = {
    p: 'response/content',
    v: ' ',
  };
  const parsed = parseChunkForContent(chunk, false, 'text');
  assert.equal(parsed.finished, false);
  assert.deepEqual(parsed.parts, [{ text: ' ', type: 'text' }]);
});

test('parseChunkForContent strips reference markers from fragment content', () => {
  const chunk = {
    p: 'response/fragments',
    o: 'APPEND',
    v: [
      { type: 'RESPONSE', content: '广州天气 [reference:12] 多云' },
    ],
  };
  const parsed = parseChunkForContent(chunk, false, 'text');
  assert.equal(parsed.finished, false);
  assert.deepEqual(parsed.parts, [{ text: '广州天气  多云', type: 'text' }]);
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

test('node stream path guard only allows /v1/chat/completions', () => {
  assert.equal(isNodeStreamSupportedPath('/v1/chat/completions'), true);
  assert.equal(isNodeStreamSupportedPath('/v1/chat/completions?x=1'), true);
  assert.equal(isNodeStreamSupportedPath('/v1beta/models/gemini-2.5-flash:streamGenerateContent'), false);
  assert.equal(isNodeStreamSupportedPath('/anthropic/v1/messages'), false);
});

test('extractPathname strips query only', () => {
  assert.equal(extractPathname('/v1/chat/completions?stream=true'), '/v1/chat/completions');
  assert.equal(extractPathname('/v1beta/models/gemini-2.5-flash:streamGenerateContent?key=1'), '/v1beta/models/gemini-2.5-flash:streamGenerateContent');
});

test('trimContinuationOverlap preserves short normal tokens and trims long snapshots', () => {
  assert.equal(trimContinuationOverlap('我们被问到', '我们'), '我们');
  const existing = '我们被问到：这是一个很长的续答快照前缀，用来验证去重逻辑不会误伤正常 token。';
  const incoming = `${existing}继续分析`;
  assert.equal(trimContinuationOverlap(existing, incoming), '继续分析');
});
