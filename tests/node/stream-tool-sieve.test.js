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

test('parseToolCalls keeps non-object argument strings as _raw (Go parity)', () => {
  const payload = JSON.stringify({
    tool_calls: [
      { name: 'read_file', input: '123' },
      { name: 'list_dir', input: '[1,2,3]' },
    ],
  });
  const calls = parseToolCalls(payload, ['read_file', 'list_dir']);
  assert.deepEqual(calls, [
    { name: 'read_file', input: { _raw: '123' } },
    { name: 'list_dir', input: { _raw: '[1,2,3]' } },
  ]);
});

test('parseToolCalls keeps unknown schema names when toolNames is provided', () => {
  const payload = JSON.stringify({
    tool_calls: [{ name: 'not_in_schema', input: { q: 'go' } }],
  });
  const calls = parseToolCalls(payload, ['search']);
  assert.equal(calls.length, 1);
  assert.equal(calls[0].name, 'not_in_schema');
});

test('parseToolCalls keeps original tool name casing', () => {
  const payload = JSON.stringify({
    tool_calls: [{ name: 'Read_File', input: { path: 'README.MD' } }],
  });
  const calls = parseToolCalls(payload, ['read_file']);
  assert.deepEqual(calls, [{ name: 'Read_File', input: { path: 'README.MD' } }]);
});

test('parseToolCalls accepts all names when toolNames is empty', () => {
  const payload = JSON.stringify({
    tool_calls: [{ name: 'not_in_schema', input: { q: 'go' } }],
  });
  const calls = parseToolCalls(payload, []);
  assert.equal(calls.length, 1);

  const detailed = parseToolCallsDetailed(payload, []);
  assert.equal(detailed.sawToolCallSyntax, true);
  assert.equal(detailed.rejectedByPolicy, false);
  assert.deepEqual(detailed.rejectedToolNames, []);
});

test('parseToolCalls ignores tool_call payloads that exist only inside fenced code blocks', () => {
  const text = [
    'I will call a tool now.',
    '```json',
    '{"tool_calls":[{"function":{"name":"read_file","arguments":"{\\"path\\":\\"README.md\\"}"}}]}',
    '```',
  ].join('\n');
  const calls = parseToolCalls(text, ['read_file']);
  assert.equal(calls.length, 0);
});

test('parseToolCalls parses text-kv fallback payload', () => {
  const text = [
    '[TOOL_CALL_HISTORY]',
    'function.name: execute_command',
    'function.arguments: {"command":"cd scripts && python check_syntax.py example.py","cwd":null,"timeout":30}',
    '[/TOOL_CALL_HISTORY]',
    'Some other text thinking...',
  ].join('\n');
  const calls = parseToolCalls(text, ['execute_command']);
  assert.equal(calls.length, 1);
  assert.equal(calls[0].name, 'execute_command');
  assert.equal(calls[0].input.command, 'cd scripts && python check_syntax.py example.py');
});

test('parseToolCalls parses multiple text-kv fallback payloads', () => {
  const text = [
    'function.name: read_file',
    'function.arguments: {"path":"abc.txt"}',
    '',
    'function.name: bash',
    'function.arguments: {"command":"ls"}',
  ].join('\n');
  const calls = parseToolCalls(text, ['read_file', 'bash']);
  assert.equal(calls.length, 2);
  assert.equal(calls[0].name, 'read_file');
  assert.equal(calls[1].name, 'bash');
});

test('parseStandaloneToolCalls parses mixed prose payload', () => {
  const mixed = '这里是示例：{"tool_calls":[{"name":"read_file","input":{"path":"README.MD"}}]}，请勿执行。';
  const standalone = '{"tool_calls":[{"name":"read_file","input":{"path":"README.MD"}}]}';
  const mixedCalls = parseStandaloneToolCalls(mixed, ['read_file']);
  const standaloneCalls = parseStandaloneToolCalls(standalone, ['read_file']);
  assert.equal(mixedCalls.length, 1);
  assert.equal(standaloneCalls.length, 1);
});

test('parseStandaloneToolCalls ignores fenced code block tool_call payload', () => {
  const fenced = ['```json', '{"tool_calls":[{"name":"read_file","input":{"path":"README.MD"}}]}', '```'].join('\n');
  const calls = parseStandaloneToolCalls(fenced, ['read_file']);
  assert.equal(calls.length, 0);
});

test('parseStandaloneToolCalls ignores chat transcript message envelope with tool_calls', () => {
  const transcript = JSON.stringify([
    { role: 'user', content: '请展示完整会话' },
    {
      role: 'assistant',
      content: null,
      tool_calls: [{ function: { name: 'read_file', arguments: '{"path":"README.MD"}' } }],
    },
  ]);
  const calls = parseStandaloneToolCalls(transcript, ['read_file']);
  assert.equal(calls.length, 0);
});


test('sieve emits tool_calls in the same chunk processing tick once payload is complete', () => {
  const state = createToolSieveState();
  const first = processToolSieveChunk(state, '{"', ['read_file']);
  const second = processToolSieveChunk(
    state,
    'tool_calls":[{"name":"read_file","input":{"path":"README.MD"}}]}',
    ['read_file'],
  );
  const firstCalls = first.filter((evt) => evt.type === 'tool_calls').flatMap((evt) => evt.calls || []);
  const secondCalls = second.filter((evt) => evt.type === 'tool_calls').flatMap((evt) => evt.calls || []);
  assert.equal(firstCalls.length, 0);
  assert.equal(secondCalls.length, 1);
  assert.equal(secondCalls[0].name, 'read_file');
});

test('sieve emits tool_calls when late key convergence forms a complete payload', () => {
  const events = runSieve(
    [
      '{"',
      'tool_calls":[{"name":"read_file","input":{"path":"README.MD"}}]}',
      '后置正文C。',
    ],
    ['read_file'],
  );
  const leakedText = collectText(events);
  const finalCalls = events.filter((evt) => evt.type === 'tool_calls').flatMap((evt) => evt.calls || []);
  assert.equal(finalCalls.length, 1);
  assert.equal(finalCalls[0].name, 'read_file');
  assert.equal(leakedText.includes('后置正文C。'), true);
  assert.equal(leakedText.toLowerCase().includes('tool_calls'), false);
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

test('sieve flushes incomplete captured tool json as text on stream finalize', () => {
  const events = runSieve(
    ['前置正文F。', '{"tool_calls":[{"name":"read_file"'],
    ['read_file'],
  );
  const leakedText = collectText(events);
  assert.equal(leakedText.includes('前置正文F。'), true);
  assert.equal(leakedText.toLowerCase().includes('tool_calls'), true);
  assert.equal(leakedText.includes('{'), true);
});

test('sieve still intercepts large tool json payloads over previous capture limit', () => {
  const large = 'a'.repeat(9000);
  const payload = `{"tool_calls":[{"name":"read_file","input":{"path":"${large}"}}]}`;
  const events = runSieve(
    [payload.slice(0, 3000), payload.slice(3000, 7000), payload.slice(7000)],
    ['read_file'],
  );
  const leakedText = collectText(events);
  const hasToolCall = events.some((evt) => evt.type === 'tool_calls' && evt.calls?.length > 0);
  const hasToolDelta = events.some((evt) => evt.type === 'tool_call_deltas' && evt.deltas?.length > 0);
  assert.equal(hasToolCall || hasToolDelta, true);
  assert.equal(leakedText.toLowerCase().includes('tool_calls'), false);
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

test('sieve swallows leaked TOOL_CALL_HISTORY marker blocks', () => {
  const events = runSieve(
    [
      '前置文本。',
      '[TOOL_CALL_HISTORY]\nstatus: already_called\nfunction.name: exec\nfunction.arguments: {}\n[/TOOL_CALL_HISTORY]',
      '后置文本。',
    ],
    ['exec'],
  );
  const leakedText = collectText(events);
  const hasToolCall = events.some((evt) => evt.type === 'tool_calls');
  assert.equal(hasToolCall, false);
  assert.equal(leakedText.includes('前置文本。'), true);
  assert.equal(leakedText.includes('后置文本。'), true);
  assert.equal(leakedText.includes('[TOOL_CALL_HISTORY]'), false);
});

test('sieve swallows leaked TOOL_RESULT_HISTORY marker blocks', () => {
  const events = runSieve(
    [
      '前置文本。',
      '[TOOL_RESULT_HISTORY]\nstatus: already_called\nfunction.name: exec\nfunction.arguments: {}\n[/TOOL_RESULT_HISTORY]',
      '后置文本。',
    ],
    ['exec'],
  );
  const leakedText = collectText(events);
  const hasToolCall = events.some((evt) => evt.type === 'tool_calls');
  assert.equal(hasToolCall, false);
  assert.equal(leakedText.includes('前置文本。'), true);
  assert.equal(leakedText.includes('后置文本。'), true);
  assert.equal(leakedText.includes('[TOOL_RESULT_HISTORY]'), false);
});

test('sieve preserves text spacing when TOOL_RESULT_HISTORY spans chunks', () => {
  const events = runSieve(
    [
      'Hello ',
      '[TOOL_RESULT_HISTORY]\nstatus: already_called\n',
      'function.name: exec\nfunction.arguments: {}\n[/TOOL_RESULT_HISTORY]',
      'world',
    ],
    ['exec'],
  );
  const leakedText = collectText(events);
  const hasToolCall = events.some((evt) => evt.type === 'tool_calls' && evt.calls?.length > 0);
  assert.equal(hasToolCall, false);
  assert.equal(leakedText, 'Hello world');
});

test('sieve emits unknown tool payload (no args) as executable tool call', () => {
  const events = runSieve(
    ['{"tool_calls":[{"name":"not_in_schema"}]}', '后置正文G。'],
    ['read_file'],
  );
  const leakedText = collectText(events);
  const hasToolCall = events.some((evt) => evt.type === 'tool_calls' && Array.isArray(evt.calls) && evt.calls.length > 0);
  const hasToolDelta = events.some((evt) => evt.type === 'tool_call_deltas' && Array.isArray(evt.deltas) && evt.deltas.length > 0);
  assert.equal(hasToolCall || hasToolDelta, true);
  assert.equal(leakedText.toLowerCase().includes('tool_calls'), false);
  assert.equal(leakedText.includes('后置正文G。'), true);
});

test('sieve emits final tool_calls for split arguments payload without incremental deltas', () => {
  const state = createToolSieveState();
  const first = processToolSieveChunk(
    state,
    '{"tool_calls":[{"name":"read_file","input":{"path":"READ',
    ['read_file'],
  );
  const second = processToolSieveChunk(
    state,
    'ME.MD","mode":"head"}}]}',
    ['read_file'],
  );
  const tail = flushToolSieve(state, ['read_file']);
  const events = [...first, ...second, ...tail];
  const deltaEvents = events.filter((evt) => evt.type === 'tool_call_deltas');
  assert.equal(deltaEvents.length, 0);
  const finalCalls = events.filter((evt) => evt.type === 'tool_calls').flatMap((evt) => evt.calls || []);
  assert.equal(finalCalls.length, 1);
  assert.equal(finalCalls[0].name, 'read_file');
  assert.deepEqual(finalCalls[0].input, { path: 'README.MD', mode: 'head' });
});

test('sieve still emits tool_calls when leading prose exists before tool json', () => {
  const events = runSieve(
    ['我将调用工具。', '{"tool_calls":[{"name":"read_file","input":{"path":"README.MD"}}]}'],
    ['read_file'],
  );
  const hasTool = events.some((evt) => (evt.type === 'tool_calls' && evt.calls?.length > 0) || (evt.type === 'tool_call_deltas' && evt.deltas?.length > 0));
  const leakedText = collectText(events);
  assert.equal(hasTool, true);
  assert.equal(leakedText.includes('我将调用工具。'), true);
  assert.equal(leakedText.toLowerCase().includes('tool_calls'), false);
});

test('sieve emits tool_calls and keeps trailing prose when payload and prose share a chunk', () => {
  const events = runSieve(
    ['{"tool_calls":[{"name":"read_file","input":{"path":"README.MD"}}]}然后继续解释。'],
    ['read_file'],
  );
  const hasTool = events.some((evt) => (evt.type === 'tool_calls' && evt.calls?.length > 0) || (evt.type === 'tool_call_deltas' && evt.deltas?.length > 0));
  const leakedText = collectText(events);
  assert.equal(hasTool, true);
  assert.equal(leakedText.includes('然后继续解释。'), true);
  assert.equal(leakedText.toLowerCase().includes('tool_calls'), false);
});

test('sieve preserves closed fence before standalone tool payload', () => {
  const events = runSieve(
    ['先给一个代码示例：\n```text\nhello\n```\n{"tool_calls":[{"name":"read_file","input":{"path":"README.MD"}}]}'],
    ['read_file'],
  );
  const hasTool = events.some((evt) => evt.type === 'tool_calls' && evt.calls?.length > 0);
  const leakedText = collectText(events);
  assert.equal(hasTool, true);
  assert.equal(leakedText.includes('```'), true);
  assert.equal(leakedText.toLowerCase().includes('tool_calls'), false);
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

test('sieve ignores tool-like payload inside nested fences and resumes detection after close', () => {
  const events = runSieve(
    [
      '外层示例开始\n````markdown\n',
      '```json\n{"tool_calls":[{"name":"read_file","input":{"path":"README.MD"}}]}\n```\n',
      '````\n',
      '{"tool_calls":[{"name":"read_file","input":{"path":"README2.MD"}}]}',
    ],
    ['read_file'],
  );
  const calls = events.filter((evt) => evt.type === 'tool_calls').flatMap((evt) => evt.calls || []);
  const leakedText = collectText(events);
  assert.equal(calls.length, 1);
  assert.equal(calls[0].input.path, 'README2.MD');
  assert.equal(leakedText.includes('README.MD'), true);
  assert.equal(leakedText.includes('README2.MD'), false);
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
