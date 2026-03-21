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

test('resolveToolcallPolicy enables sieve even without declared tools to avoid raw tool json leaks', () => {
  const policy = resolveToolcallPolicy({}, undefined);
  assert.deepEqual(policy.toolNames, []);
  assert.equal(policy.toolSieveEnabled, true);
  assert.equal(policy.emitEarlyToolDeltas, true);
});

test('resolveToolcallPolicy respects prepare flags and prepared tool names', () => {
  const policy = resolveToolcallPolicy(
    {
      tool_names: [' prepped_tool ', '', null],
      toolcall_feature_match: false,
      toolcall_early_emit_high: false,
    },
    [{ type: 'function', function: { name: 'fallback_tool', parameters: { type: 'object' } } }],
  );
  assert.deepEqual(policy.toolNames, ['prepped_tool']);
  assert.equal(policy.toolSieveEnabled, false);
  assert.equal(policy.emitEarlyToolDeltas, false);
});

test('normalizePreparedToolNames filters empty values', () => {
  assert.deepEqual(normalizePreparedToolNames([' a ', '', null, 'b']), ['a', 'b']);
});

test('boolDefaultTrue keeps false only when explicitly false', () => {
  assert.equal(boolDefaultTrue(false), false);
  assert.equal(boolDefaultTrue(true), true);
  assert.equal(boolDefaultTrue(undefined), true);
});

test('filterIncrementalToolCallDeltasByAllowed blocks unknown name and follow-up args', () => {
  const seen = new Map();
  const filtered = filterIncrementalToolCallDeltasByAllowed(
    [
      { index: 0, name: 'not_in_schema' },
      { index: 0, arguments: '{"x":1}' },
    ],
    ['read_file'],
    seen,
  );
  assert.deepEqual(filtered, []);
  assert.equal(seen.get(0), '__blocked__');
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

test('parseChunkForContent + sieve does not leak suspicious prefix in split tool json case', () => {
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
  const hasToolDeltas = events.some((evt) => evt.type === 'tool_call_deltas' && evt.deltas && evt.deltas.length > 0);
  const leakedText = events
    .filter((evt) => evt.type === 'text' && evt.text)
    .map((evt) => evt.text)
    .join('');

  assert.equal(hasToolCalls || hasToolDeltas, true);
  assert.equal(leakedText.includes('{'), false);
  assert.equal(leakedText.toLowerCase().includes('tool_calls'), false);
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
