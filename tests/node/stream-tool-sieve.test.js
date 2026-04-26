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
  const payload = '<tool_calls><invoke name="read_file"><parameter name="path">README.MD</parameter></invoke></tool_calls>';
  const calls = parseToolCalls(payload, ['read_file']);
  assert.equal(calls.length, 1);
  assert.equal(calls[0].name, 'read_file');
  assert.deepEqual(calls[0].input, { path: 'README.MD' });
});

test('parseToolCalls parses DSML markup tool call', () => {
  const payload = '<|DSML|tool_calls><|DSML|invoke name="read_file"><|DSML|parameter name="path" string="true">README.MD</|DSML|parameter></|DSML|invoke></|DSML|tool_calls>';
  const calls = parseToolCalls(payload, ['read_file']);
  assert.equal(calls.length, 1);
  assert.equal(calls[0].name, 'read_file');
  assert.deepEqual(calls[0].input, { path: 'README.MD' });
});

test('parseToolCalls parses loose DSML markup tool call', () => {
  const payload = '<DSML|tool_calls><DSML|invoke name="read_file"><DSML|parameter name="path" string="true">README.MD</DSML|parameter></DSML|invoke></DSML|tool_calls>';
  const calls = parseToolCalls(payload, ['read_file']);
  assert.equal(calls.length, 1);
  assert.equal(calls[0].name, 'read_file');
  assert.deepEqual(calls[0].input, { path: 'README.MD' });
});

test('parseToolCalls parses bare DSML markup tool call', () => {
  const payload = '<DSMLtool_calls><DSMLinvoke name="Read"><DSMLparameter name="file_path" string="true">E:/KSWJJ/blog/backend/storage/articles.go</DSMLparameter><DSMLparameter name="limit" string="false">30</DSMLparameter><DSMLparameter name="offset" string="false">295</DSMLparameter></DSMLinvoke></DSMLtool_calls>';
  const calls = parseToolCalls(payload, ['Read']);
  assert.equal(calls.length, 1);
  assert.equal(calls[0].name, 'Read');
  assert.deepEqual(calls[0].input, { file_path: 'E:/KSWJJ/blog/backend/storage/articles.go', limit: 30, offset: 295 });
});

test('parseToolCalls parses DSML typed parameters', () => {
  const payload = '<|DSML|tool_calls><|DSML|invoke name="search"><|DSML|parameter name="top_k" string="false">3</|DSML|parameter><|DSML|parameter name="exact" string="false">true</|DSML|parameter><|DSML|parameter name="filters" string="false">{"lang":"go"}</|DSML|parameter></|DSML|invoke></|DSML|tool_calls>';
  const calls = parseToolCalls(payload, ['search']);
  assert.equal(calls.length, 1);
  assert.equal(calls[0].input.top_k, 3);
  assert.equal(calls[0].input.exact, true);
  assert.deepEqual(calls[0].input.filters, { lang: 'go' });
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
    '<tool_calls><invoke name="read_file"><parameter name="path">README.md</parameter></invoke></tool_calls>',
    '```',
  ].join('\n');
  const calls = parseToolCalls(text, ['read_file']);
  assert.equal(calls.length, 0);
});

test('parseToolCalls keeps unknown schema names when toolNames is provided', () => {
  const payload = '<tool_calls><invoke name="not_in_schema"><parameter name="q">go</parameter></invoke></tool_calls>';
  const calls = parseToolCalls(payload, ['search']);
  assert.equal(calls.length, 1);
  assert.equal(calls[0].name, 'not_in_schema');
});

test('sieve emits tool_calls for XML tool call payload', () => {
  const events = runSieve(
    ['<tool_calls><invoke name="read_file"><parameter name="path">README.MD</parameter></invoke></tool_calls>'],
    ['read_file'],
  );
  const finalCalls = events.filter((evt) => evt.type === 'tool_calls').flatMap((evt) => evt.calls || []);
  assert.equal(finalCalls.length, 1);
  assert.equal(finalCalls[0].name, 'read_file');
});

test('sieve emits tool_calls when XML tag spans multiple chunks', () => {
  const events = runSieve(
    [
      '<tool_calls><invoke name="read_file">',
      '<parameter name="path">README.MD</parameter></invoke></tool_calls>',
    ],
    ['read_file'],
  );
  const finalCalls = events.filter((evt) => evt.type === 'tool_calls').flatMap((evt) => evt.calls || []);
  assert.equal(finalCalls.length, 1);
  assert.equal(finalCalls[0].name, 'read_file');
});

test('sieve emits tool_calls for DSML tool call payload', () => {
  const events = runSieve(
    ['<|DSML|tool_calls><|DSML|invoke name="read_file"><|DSML|parameter name="path" string="true">README.MD</|DSML|parameter></|DSML|invoke></|DSML|tool_calls>'],
    ['read_file'],
  );
  const finalCalls = events.filter((evt) => evt.type === 'tool_calls').flatMap((evt) => evt.calls || []);
  assert.equal(finalCalls.length, 1);
  assert.equal(finalCalls[0].name, 'read_file');
});

test('sieve emits tool_calls for loose DSML tool call payload', () => {
  const events = runSieve(
    ['<DSML|tool_calls><DSML|invoke name="read_file"><DSML|parameter name="path" string="true">README.MD</DSML|parameter></DSML|invoke></DSML|tool_calls>'],
    ['read_file'],
  );
  const finalCalls = events.filter((evt) => evt.type === 'tool_calls').flatMap((evt) => evt.calls || []);
  assert.equal(finalCalls.length, 1);
  assert.equal(finalCalls[0].name, 'read_file');
});

test('sieve keeps long XML tool calls buffered until the closing tag arrives', () => {
  const longContent = 'x'.repeat(4096);
  const splitAt = longContent.length / 2;
  const events = runSieve(
    [
      '<tool_calls>\n  <invoke name="write_to_file">\n    <parameter name="content"><![CDATA[',
      longContent.slice(0, splitAt),
      longContent.slice(splitAt),
      ']]></parameter>\n  </invoke>\n</tool_calls>',
    ],
    ['write_to_file'],
  );
  const leakedText = collectText(events);
  const finalCalls = events.filter((evt) => evt.type === 'tool_calls').flatMap((evt) => evt.calls || []);
  assert.equal(leakedText, '');
  assert.equal(finalCalls.length, 1);
  assert.equal(finalCalls[0].name, 'write_to_file');
  assert.equal(finalCalls[0].input.content, longContent);
});

test('sieve keeps CDATA tool examples buffered until the outer closing tag arrives', () => {
  const content = [
    '# DS2API 4.0 更新内容',
    '',
    'x'.repeat(4096),
    '```xml',
    '<tool_calls>',
    '  <invoke name="demo">',
    '    <parameter name="value">x</parameter>',
    '  </invoke>',
    '</tool_calls>',
    '```',
    'tail',
  ].join('\n');
  const innerClose = content.indexOf('</tool_calls>') + '</tool_calls>'.length;
  const state = createToolSieveState();
  const chunks = [
    '<tool_calls>\n  <invoke name="Write">\n    <parameter name="content"><![CDATA[',
    content.slice(0, innerClose),
    content.slice(innerClose),
    ']]></parameter>\n    <parameter name="file_path">DS2API-4.0-Release-Notes.md</parameter>\n  </invoke>\n</tool_calls>',
  ];
  const events = [];
  chunks.forEach((chunk, idx) => {
    const next = processToolSieveChunk(state, chunk, ['Write']);
    if (idx <= 1) {
      assert.deepEqual(next, []);
    }
    events.push(...next);
  });
  events.push(...flushToolSieve(state, ['Write']));

  const leakedText = collectText(events);
  const finalCalls = events.filter((evt) => evt.type === 'tool_calls').flatMap((evt) => evt.calls || []);
  assert.equal(leakedText, '');
  assert.equal(finalCalls.length, 1);
  assert.equal(finalCalls[0].name, 'Write');
  assert.equal(finalCalls[0].input.content, content);
});

test('parseToolCalls keeps XML-looking CDATA content intact', () => {
  const content = [
    '# Release notes',
    '```xml',
    '<tool_calls><invoke name="demo"><parameter name="value">x</parameter></invoke></tool_calls>',
    '```',
  ].join('\n');
  const payload = `<tool_calls><invoke name="Write"><parameter name="content"><![CDATA[${content}]]></parameter><parameter name="file_path">DS2API-4.0-Release-Notes.md</parameter></invoke></tool_calls>`;
  const calls = parseToolCalls(payload, ['Write']);
  assert.equal(calls.length, 1);
  assert.equal(calls[0].input.content, content);
  assert.equal(calls[0].input.file_path, 'DS2API-4.0-Release-Notes.md');
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
  const chunk = '<tool_calls><invoke name="read_file"><param>{"path":"README.MD"}</param></invoke></tool_calls>';
  const events = runSieve([chunk], ['read_file']);
  const leakedText = collectText(events);
  const hasToolCalls = events.some((evt) => evt.type === 'tool_calls' && evt.calls?.length > 0);
  assert.equal(hasToolCalls, false);
  assert.equal(leakedText, chunk);
});

test('sieve keeps bare tool_call XML as plain text without wrapper', () => {
  const chunk = '<invoke name="read_file"><parameter name="path">README.MD</parameter></invoke>';
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
      '  <invoke name="read_file">\n',
    ],
    ['read_file'],
  );
  const leakedText = collectText(events);
  const expected = ['前置正文G。', '<tool_calls>\n', '  <invoke name="read_file">\n'].join('');
  const hasToolCalls = events.some((evt) => evt.type === 'tool_calls' && evt.calls?.length > 0);
  assert.equal(hasToolCalls, false);
  assert.equal(leakedText, expected);
});

test('sieve captures XML wrapper tags with attributes without leaking wrapper text', () => {
  const events = runSieve(
    [
      '前置正文H。',
      '<tool_calls id="x"><invoke name="read_file"><parameter name="path">README.MD</parameter></invoke></tool_calls>',
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
  const payload = '<tool_calls><invoke name="read_file"><parameter name="path">README.md</function></invoke></tool_calls>';
  const calls = parseToolCalls(payload, ['read_file']);
  assert.equal(calls.length, 0);
});
