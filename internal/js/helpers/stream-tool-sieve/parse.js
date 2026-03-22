'use strict';

const {
  toStringSafe,
} = require('./state');
const {
  buildToolCallCandidates,
  parseToolCallsPayload,
  parseMarkupToolCalls,
  parseTextKVToolCalls,
  stripFencedCodeBlocks,
} = require('./parse_payload');
const { TOOL_SEGMENT_KEYWORDS } = require('./tool-keywords');

const TOOL_NAME_LOOSE_PATTERN = /[^a-z0-9]+/g;
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

  const candidates = buildToolCallCandidates(normalized);
  let parsed = [];
  for (const c of candidates) {
    parsed = parseToolCallsPayload(c);
    if (parsed.length === 0) {
      parsed = parseMarkupToolCalls(c);
    }
    if (parsed.length === 0) {
      parsed = parseTextKVToolCalls(c);
    }
    if (parsed.length > 0) {
      result.sawToolCallSyntax = true;
      break;
    }
  }
  if (parsed.length === 0) {
    parsed = parseMarkupToolCalls(normalized);
    if (parsed.length === 0) {
      parsed = parseTextKVToolCalls(normalized);
      if (parsed.length === 0) {
        return result;
      }
    }
    result.sawToolCallSyntax = true;
  }

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
  const candidates = buildToolCallCandidates(trimmed);
  let parsed = [];
  for (const c of candidates) {
    parsed = parseToolCallsPayload(c);
    if (parsed.length === 0) {
      parsed = parseMarkupToolCalls(c);
    }
    if (parsed.length === 0) {
      parsed = parseTextKVToolCalls(c);
    }
    if (parsed.length > 0) {
      break;
    }
  }
  if (parsed.length === 0) {
    parsed = parseMarkupToolCalls(trimmed);
    if (parsed.length === 0) {
      parsed = parseTextKVToolCalls(trimmed);
      if (parsed.length === 0) {
        return result;
      }
    }
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

function resolveAllowedToolName(name, allowed, allowedCanonical) {
  const normalizedName = toStringSafe(name).trim();
  if (!normalizedName) {
    return '';
  }
  if (allowed.has(normalizedName)) {
    return normalizedName;
  }
  const lower = normalizedName.toLowerCase();
  if (allowedCanonical.has(lower)) {
    return allowedCanonical.get(lower);
  }
  const idx = lower.lastIndexOf('.');
  if (idx >= 0 && idx < lower.length - 1) {
    const tail = lower.slice(idx + 1);
    if (allowedCanonical.has(tail)) {
      return allowedCanonical.get(tail);
    }
  }
  const loose = lower.replace(TOOL_NAME_LOOSE_PATTERN, '');
  if (!loose) {
    return '';
  }
  for (const [candidateLower, canonical] of allowedCanonical.entries()) {
    if (candidateLower.replace(TOOL_NAME_LOOSE_PATTERN, '') === loose) {
      return canonical;
    }
  }
  return '';
}

function looksLikeToolCallSyntax(text) {
  const lower = toStringSafe(text).toLowerCase();
  return TOOL_SEGMENT_KEYWORDS.some((kw) => lower.includes(kw))
    || TOOL_MARKUP_PREFIXES.some((prefix) => lower.includes(prefix));
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
