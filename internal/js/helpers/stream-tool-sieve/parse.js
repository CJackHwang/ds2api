'use strict';

const {
  toStringSafe,
} = require('./state');
const {
  parseMarkupToolCalls,
  stripFencedCodeBlocks,
} = require('./parse_payload');

const TOOL_MARKUP_PREFIXES = ['<tool_call', '<function_call', '<invoke'];

function extractToolNames(tools) {
  if (!Array.isArray(tools) || tools.length === 0) {
    return [];
  }
  const out = [];
  const seen = new Set();
  for (const t of tools) {
    if (!t || typeof t !== 'object') {
      continue;
    }
    const fn = t.function && typeof t.function === 'object' ? t.function : t;
    const name = toStringSafe(fn.name);
    if (!name || seen.has(name)) {
      continue;
    }
    seen.add(name);
    out.push(name);
  }
  return out;
}

function parseToolCalls(text, toolNames) {
  return parseToolCallsDetailed(text, toolNames).calls;
}

function parseToolCallsDetailed(text, toolNames) {
  const result = emptyParseResult();
  const normalized = toStringSafe(text);
  if (!normalized) {
    return result;
  }
  result.sawToolCallSyntax = looksLikeToolCallSyntax(normalized);
  if (shouldSkipToolCallParsingForCodeFenceExample(normalized)) {
    return result;
  }

  // XML markup parsing only.
  const parsed = parseMarkupToolCalls(normalized);
  if (parsed.length === 0) {
    return result;
  }
  result.sawToolCallSyntax = true;
  const filtered = filterToolCallsDetailed(parsed, toolNames);
  result.calls = filtered.calls;
  result.rejectedToolNames = filtered.rejectedToolNames;
  result.rejectedByPolicy = filtered.rejectedToolNames.length > 0 && filtered.calls.length === 0;
  return result;
}

function parseStandaloneToolCalls(text, toolNames) {
  return parseStandaloneToolCallsDetailed(text, toolNames).calls;
}

function parseStandaloneToolCallsDetailed(text, toolNames) {
  const result = emptyParseResult();
  const trimmed = toStringSafe(text);
  if (!trimmed) {
    return result;
  }
  result.sawToolCallSyntax = looksLikeToolCallSyntax(trimmed);
  if (shouldSkipToolCallParsingForCodeFenceExample(trimmed)) {
    return result;
  }

  // XML markup parsing only.
  const parsed = parseMarkupToolCalls(trimmed);
  if (parsed.length === 0) {
    return result;
  }

  result.sawToolCallSyntax = true;
  const filtered = filterToolCallsDetailed(parsed, toolNames);
  result.calls = filtered.calls;
  result.rejectedToolNames = filtered.rejectedToolNames;
  result.rejectedByPolicy = filtered.rejectedToolNames.length > 0 && filtered.calls.length === 0;
  return result;
}

function emptyParseResult() {
  return {
    calls: [],
    sawToolCallSyntax: false,
    rejectedByPolicy: false,
    rejectedToolNames: [],
  };
}

function filterToolCallsDetailed(parsed, toolNames) {
  const calls = [];
  for (const tc of parsed) {
    if (!tc || !tc.name) {
      continue;
    }
    calls.push({
      name: tc.name,
      input: tc.input && typeof tc.input === 'object' && !Array.isArray(tc.input) ? tc.input : {},
    });
  }
  return { calls, rejectedToolNames: [] };
}

function looksLikeToolCallSyntax(text) {
  const lower = toStringSafe(text).toLowerCase();
  return TOOL_MARKUP_PREFIXES.some((prefix) => lower.includes(prefix));
}

function shouldSkipToolCallParsingForCodeFenceExample(text) {
  if (!looksLikeToolCallSyntax(text)) {
    return false;
  }
  const stripped = stripFencedCodeBlocks(text);
  return !looksLikeToolCallSyntax(stripped);
}

module.exports = {
  extractToolNames,
  parseToolCalls,
  parseToolCallsDetailed,
  parseStandaloneToolCalls,
  parseStandaloneToolCallsDetailed,
};
