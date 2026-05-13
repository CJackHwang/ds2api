'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');

async function loadUtils() {
  return import('../../webui/src/features/chatHistory/chatHistoryUtils.js');
}

test('chat history strict parser merges current input file placeholder', async () => {
  const {
    buildListModeMessages,
  } = await loadUtils();
  const t = (key) => key;
  const item = {
    messages: [{
      role: 'user',
      content: 'Continue from the latest state in the attached test-history.txt context. Treat it as the current working state and answer the latest user request directly.',
    }],
    history_text: [
      '<|begin▁of▁sentence|>',
      '<|User|>hello',
      '<|Assistant|>hi<|end▁of▁sentence|>',
    ].join(''),
  };

  const result = buildListModeMessages(item, t);
  assert.equal(result.historyMerged, true);
  assert.deepEqual(result.messages, [
    { role: 'user', content: 'hello' },
    { role: 'assistant', content: 'hi' },
  ]);
});

test('chat history strict parser inserts history after system messages', async () => {
  const {
    buildListModeMessages,
  } = await loadUtils();
  const t = (key) => key;
  const item = {
    messages: [
      { role: 'system', content: 'policy' },
      { role: 'user', content: 'latest' },
    ],
    history_text: [
      '<|begin▁of▁sentence|>',
      '<|User|>old',
      '<|Assistant|>done<|end▁of▁sentence|>',
    ].join(''),
  };

  const result = buildListModeMessages(item, t);
  assert.equal(result.historyMerged, true);
  assert.deepEqual(result.messages, [
    { role: 'system', content: 'policy' },
    { role: 'user', content: 'old' },
    { role: 'assistant', content: 'done' },
    { role: 'user', content: 'latest' },
  ]);
});

test('chat history transcript parser replaces current input file placeholder', async () => {
  const {
    buildListModeMessages,
  } = await loadUtils();
  const t = (key) => key;
  const testFilename = 'test-history.txt';
  const item = {
    messages: [{
      role: 'user',
      content: `Continue from the latest state in the attached ${testFilename} context. Treat it as the current working state and answer the latest user request directly.`,
    }],
    history_text: [
      `# ${testFilename}`,
      'Prior conversation history and tool progress.',
      '',
      '=== 1. SYSTEM ===',
      'policy',
      '',
      '=== 2. USER ===',
      'hello',
      '',
      '=== 3. ASSISTANT ===',
      'hi',
      '',
      '=== 4. TOOL ===',
      '[name=search_web tool_call_id=call_1]',
      '{"ok":true}',
      '',
      '=== 5. USER ===',
      'latest',
      '',
    ].join('\n'),
  };

  const result = buildListModeMessages(item, t);
  assert.equal(result.historyMerged, true);
  assert.deepEqual(result.messages, [
    { role: 'system', content: 'policy' },
    { role: 'user', content: 'hello' },
    { role: 'assistant', content: 'hi' },
    { role: 'tool', content: '[name=search_web tool_call_id=call_1]\n{"ok":true}' },
    { role: 'user', content: 'latest' },
  ]);
});
