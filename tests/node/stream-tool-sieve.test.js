'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');

const {
  extractToolNames,
  createToolSieveState,
  processToolSieveChunk,
  flushToolSieve,
  parseToolCalls,
  parseToolCallsDetailed,
  parseStandaloneToolCalls,
  formatOpenAIStreamToolCalls,
} = require('../../internal/js/helpers/stream-tool-sieve.js');

function runSieve(chunks, toolNames) {
  const state = createToolSieveState();
  const events = [];
  for (const chunk of chunks) {
    events.push(...processToolSieveChunk(state, chunk, toolNames));
  }
  events.push(...flushToolSieve(state, toolNames));
  return events;
}

function collectText(events) {
  return events
    .filter((evt) => evt.type === 'text' && evt.text)
    .map((evt) => evt.text)
    .join('');
}

test('extractToolNames keeps only declared tool names (Go parity)', () => {
  const names = extractToolNames([
    { function: { description: 'no name tool' } },
    { function: { name: ' read_file ' } },
    { function: { name: 'read_file' } },
    {},
  ]);
  assert.deepEqual(names, ['read_file']);
});

test('parseToolCalls parses XML markup tool call', () => {
  const payload = '<tool_call><tool_name>read_file</tool_name><parameters>{"path":"README.MD"}</parameters></tool_call>';
  const calls = parseToolCalls(payload, ['read_file']);
  assert.equal(calls.length, 1);
  assert.equal(calls[0].name, 'read_file');
  assert.deepEqual(calls[0].input, { path: 'README.MD' });
});

test('parseToolCalls ignores JSON tool_calls payload (XML-only)', () => {
  const payload = JSON.stringify({
    tool_calls: [{ name: 'read_file', input: { path: 'README.MD' } }],
  });
  const calls = parseToolCalls(payload, ['read_file']);
  assert.equal(calls.length, 0);
});

test('parseToolCalls ignores tool_call payloads that exist only inside fenced code blocks', () => {
  const text = [
    'I will call a tool now.',
    '```xml',
    '<tool_call><tool_name>read_file</tool_name><parameters>{"path":"README.md"}</parameters></tool_call>',
    '```',
  ].join('\n');
  const calls = parseToolCalls(text, ['read_file']);
  assert.equal(calls.length, 0);
});

test('parseToolCalls keeps unknown schema names when toolNames is provided', () => {
  const payload = '<tool_call><tool_name>not_in_schema</tool_name><parameters>{"q":"go"}</parameters></tool_call>';
  const calls = parseToolCalls(payload, ['search']);
  assert.equal(calls.length, 1);
  assert.equal(calls[0].name, 'not_in_schema');
});

test('sieve emits tool_calls for XML tool call payload', () => {
  const events = runSieve(
    ['<tool_call><tool_name>read_file</tool_name><parameters>{"path":"README.MD"}</parameters></tool_call>'],
    ['read_file'],
  );
  const finalCalls = events.filter((evt) => evt.type === 'tool_calls').flatMap((evt) => evt.calls || []);
  assert.equal(finalCalls.length, 1);
  assert.equal(finalCalls[0].name, 'read_file');
});

test('sieve emits tool_calls when XML tag spans multiple chunks', () => {
  const events = runSieve(
    [
      '<tool_call><tool_name>read_file</tool_name>',
      '<parameters>{"path":"README.MD"}</parameters></tool_call>',
    ],
    ['read_file'],
  );
  const finalCalls = events.filter((evt) => evt.type === 'tool_calls').flatMap((evt) => evt.calls || []);
  assert.equal(finalCalls.length, 1);
  assert.equal(finalCalls[0].name, 'read_file');
});

test('sieve passes JSON tool_calls payload through as text (XML-only)', () => {
  const events = runSieve(
    ['{"tool_calls":[{"name":"read_file","input":{"path":"README.MD"}}]}'],
    ['read_file'],
  );
  const leakedText = collectText(events);
  const hasToolCall = events.some((evt) => evt.type === 'tool_calls' && evt.calls?.length > 0);
  assert.equal(hasToolCall, false);
  assert.equal(leakedText.includes('tool_calls'), true);
});

test('sieve keeps embedded invalid tool-like json as normal text to avoid stream stalls', () => {
  const events = runSieve(
    [
      '前置正文D。',
      "{'tool_calls':[{'name':'read_file','input':{'path':'README.MD'}}]}",
      '后置正文E。',
    ],
    ['read_file'],
  );
  const leakedText = collectText(events);
  const hasToolCall = events.some((evt) => evt.type === 'tool_calls');
  assert.equal(hasToolCall, false);
  assert.equal(leakedText.includes('前置正文D。'), true);
  assert.equal(leakedText.includes('后置正文E。'), true);
  assert.equal(leakedText.toLowerCase().includes('tool_calls'), true);
});

test('sieve passes malformed executable-looking XML through as text', () => {
  const chunk = '<tool_call><parameters>{"path":"README.MD"}</parameters></tool_call>';
  const events = runSieve([chunk], ['read_file']);
  const leakedText = collectText(events);
  const hasToolCalls = events.some((evt) => evt.type === 'tool_calls' && evt.calls?.length > 0);
  assert.equal(hasToolCalls, false);
  assert.equal(leakedText, chunk);
});

test('sieve flushes incomplete captured XML tool blocks by falling back to raw text', () => {
  const events = runSieve(
    [
      '前置正文G。',
      '<tool_calls>\n',
      '  <tool_call>\n',
      '    <tool_name>read_file</tool_name>\n',
    ],
    ['read_file'],
  );
  const leakedText = collectText(events);
  const expected = ['前置正文G。', '<tool_calls>\n', '  <tool_call>\n', '    <tool_name>read_file</tool_name>\n'].join('');
  const hasToolCalls = events.some((evt) => evt.type === 'tool_calls' && evt.calls?.length > 0);
  assert.equal(hasToolCalls, false);
  assert.equal(leakedText, expected);
});

test('sieve captures XML wrapper tags with attributes without leaking wrapper text', () => {
  const events = runSieve(
    [
      '前置正文H。',
      '<tool_calls id="x"><tool_call><tool_name>read_file</tool_name><parameters>{"path":"README.MD"}</parameters></tool_call></tool_calls>',
      '后置正文I。',
    ],
    ['read_file'],
  );
  const leakedText = collectText(events);
  const hasToolCall = events.some((evt) => evt.type === 'tool_calls' && evt.calls?.length > 0);
  assert.equal(hasToolCall, true);
  assert.equal(leakedText.includes('前置正文H。'), true);
  assert.equal(leakedText.includes('后置正文I。'), true);
  assert.equal(leakedText.includes('<tool_calls id=\"x\">'), false);
  assert.equal(leakedText.includes('</tool_calls>'), false);
});

test('sieve keeps plain text intact in tool mode when no tool call appears', () => {
  const events = runSieve(
    ['你好，', '这是普通文本回复。', '请继续。'],
    ['read_file'],
  );
  const leakedText = collectText(events);
  const hasToolCall = events.some((evt) => evt.type === 'tool_calls');
  assert.equal(hasToolCall, false);
  assert.equal(leakedText, '你好，这是普通文本回复。请继续。');
});

test('sieve keeps plain "tool_calls" prose as text when no valid payload follows', () => {
  const events = runSieve(
    ['前置。', '这里提到 tool_calls 只是解释，不是调用。', '后置。'],
    ['read_file'],
  );
  const leakedText = collectText(events);
  const hasToolCall = events.some((evt) => evt.type === 'tool_calls' && evt.calls?.length > 0);
  assert.equal(hasToolCall, false);
  assert.equal(leakedText.includes('tool_calls'), true);
  assert.equal(leakedText, '前置。这里提到 tool_calls 只是解释，不是调用。后置。');
});

test('sieve keeps numbered planning prose when no tool payload follows', () => {
  const events = runSieve(
    ['好的，我会依次测试每个工具。\n\n1. 获取当前时间'],
    ['get_current_time'],
  );
  const leakedText = collectText(events);
  const hasToolCall = events.some((evt) => evt.type === 'tool_calls' && evt.calls?.length > 0);
  assert.equal(hasToolCall, false);
  assert.equal(leakedText, '好的，我会依次测试每个工具。\n\n1. 获取当前时间');
});

test('sieve does not trigger tool calls for long fenced examples beyond legacy tail window', () => {
  const longPadding = 'x'.repeat(700);
  const events = runSieve(
    [
      `前置说明\n\`\`\`json\n${longPadding}\n`,
      '{"tool_calls":[{"name":"read_file","input":{"path":"README.MD"}}]}\n',
      '```',
      '\n后置说明',
    ],
    ['read_file'],
  );
  const hasTool = events.some((evt) => evt.type === 'tool_calls' && evt.calls?.length > 0);
  const leakedText = collectText(events);
  assert.equal(hasTool, false);
  assert.equal(leakedText.includes('后置说明'), true);
  assert.equal(leakedText.toLowerCase().includes('tool_calls'), true);
});

test('sieve keeps fence state when triple-backticks are split across chunks', () => {
  const events = runSieve(
    [
      '示例开始\n``',
      '`json\n{"tool_calls":[{"name":"read_file","input":{"path":"README.MD"}}]}\n',
      '```',
      '\n示例结束',
    ],
    ['read_file'],
  );
  const hasTool = events.some((evt) => evt.type === 'tool_calls' && evt.calls?.length > 0);
  const leakedText = collectText(events);
  assert.equal(hasTool, false);
  assert.equal(leakedText.includes('示例结束'), true);
  assert.equal(leakedText.toLowerCase().includes('tool_calls'), true);
});

test('formatOpenAIStreamToolCalls reuses ids with the same idStore', () => {
  const idStore = new Map();
  const calls = [{ name: 'read_file', input: { path: 'README.MD' } }];
  const first = formatOpenAIStreamToolCalls(calls, idStore);
  const second = formatOpenAIStreamToolCalls(calls, idStore);
  assert.equal(first.length, 1);
  assert.equal(second.length, 1);
  assert.equal(first[0].id, second[0].id);
});

test('parseToolCalls rejects mismatched markup tags', () => {
  const payload = '<tool_call><name>read_file</function><arguments>{"path":"README.md"}</arguments></tool_call>';
  const calls = parseToolCalls(payload, ['read_file']);
  assert.equal(calls.length, 0);
});
