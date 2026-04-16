'use strict';

const TOOL_CALL_PATTERN = /\{\s*["']tool_calls["']\s*:\s*\[(.*?)\]\s*\}/s;
const TOOL_CALL_MARKUP_BLOCK_PATTERN = /<(?:[a-z0-9_:-]+:)?(tool_call|function_call|invoke)\b([^>]*)>([\s\S]*?)<\/(?:[a-z0-9_:-]+:)?\1>/gi;
const TOOL_CALL_MARKUP_SELFCLOSE_PATTERN = /<(?:[a-z0-9_:-]+:)?invoke\b([^>]*)\/>/gi;
const TOOL_CALL_MARKUP_KV_PATTERN = /<(?:[a-z0-9_:-]+:)?([a-z0-9_.-]+)\b[^>]*>([\s\S]*?)<\/(?:[a-z0-9_:-]+:)?\1>/gi;
const TOOL_CALL_MARKUP_ATTR_PATTERN = /(name|function|tool)\s*=\s*"([^"]+)"/i;
const TOOL_CALL_MARKUP_NAME_PATTERNS = [
  /<(?:[a-z0-9_:-]+:)?tool_name\b[^>]*>([\s\S]*?)<\/(?:[a-z0-9_:-]+:)?tool_name>/i,
  /<(?:[a-z0-9_:-]+:)?function_name\b[^>]*>([\s\S]*?)<\/(?:[a-z0-9_:-]+:)?function_name>/i,
  /<(?:[a-z0-9_:-]+:)?name\b[^>]*>([\s\S]*?)<\/(?:[a-z0-9_:-]+:)?name>/i,
  /<(?:[a-z0-9_:-]+:)?function\b[^>]*>([\s\S]*?)<\/(?:[a-z0-9_:-]+:)?function>/i,
];
const TOOL_CALL_MARKUP_ARGS_PATTERNS = [
  /<(?:[a-z0-9_:-]+:)?input\b[^>]*>([\s\S]*?)<\/(?:[a-z0-9_:-]+:)?input>/i,
  /<(?:[a-z0-9_:-]+:)?arguments\b[^>]*>([\s\S]*?)<\/(?:[a-z0-9_:-]+:)?arguments>/i,
  /<(?:[a-z0-9_:-]+:)?argument\b[^>]*>([\s\S]*?)<\/(?:[a-z0-9_:-]+:)?argument>/i,
  /<(?:[a-z0-9_:-]+:)?parameters\b[^>]*>([\s\S]*?)<\/(?:[a-z0-9_:-]+:)?parameters>/i,
  /<(?:[a-z0-9_:-]+:)?parameter\b[^>]*>([\s\S]*?)<\/(?:[a-z0-9_:-]+:)?parameter>/i,
  /<(?:[a-z0-9_:-]+:)?args\b[^>]*>([\s\S]*?)<\/(?:[a-z0-9_:-]+:)?args>/i,
  /<(?:[a-z0-9_:-]+:)?params\b[^>]*>([\s\S]*?)<\/(?:[a-z0-9_:-]+:)?params>/i,
];
const NAMED_PARAMETER_PATTERN = /<(?:[a-z0-9_:-]+:)?parameter\b[^>]*\bname="([^"]+)"[^>]*>([\s\S]*?)<\/(?:[a-z0-9_:-]+:)?parameter>/gi;
const TEXT_KV_NAME_PATTERN = /function\.name:\s*([a-zA-Z0-9_.-]+)/gi;

const {
  toStringSafe,
} = require('./state');
const {
  extractJSONObjectFrom,
} = require('./jsonscan');

function stripFencedCodeBlocks(text) {
  const t = typeof text === 'string' ? text : '';
  if (!t) {
    return '';
  }
  return t.replace(/```[\s\S]*?```/g, ' ');
}

function buildToolCallCandidates(text) {
  const trimmed = toStringSafe(text);
  const candidates = [trimmed];

  const fenced = trimmed.match(/```(?:json)?\s*([\s\S]*?)\s*```/gi) || [];
  for (const block of fenced) {
    const m = block.match(/```(?:json)?\s*([\s\S]*?)\s*```/i);
    if (m && m[1]) {
      candidates.push(toStringSafe(m[1]));
    }
  }

  for (const candidate of extractToolCallObjects(trimmed)) {
    candidates.push(toStringSafe(candidate));
  }

  const first = trimmed.indexOf('{');
  const last = trimmed.lastIndexOf('}');
  if (first >= 0 && last > first) {
    candidates.push(toStringSafe(trimmed.slice(first, last + 1)));
  }
  const firstArr = trimmed.indexOf('[');
  const lastArr = trimmed.lastIndexOf(']');
  if (firstArr >= 0 && lastArr > firstArr) {
    candidates.push(toStringSafe(trimmed.slice(firstArr, lastArr + 1)));
  }

  const m = trimmed.match(TOOL_CALL_PATTERN);
  if (m && m[1]) {
    candidates.push(`{"tool_calls":[${m[1]}]}`);
  }

  return [...new Set(candidates.filter(Boolean))];
}

function extractToolCallObjects(text) {
  const raw = toStringSafe(text);
  if (!raw) {
    return [];
  }
  const lower = raw.toLowerCase();
  const out = [];
  let offset = 0;

  // eslint-disable-next-line no-constant-condition
  while (true) {
    const idxToolCalls = lower.indexOf('tool_calls', offset);
    const idxFunction = lower.indexOf('"function"', offset);
    const idxFunctionCall = lower.indexOf('functioncall', offset);
    const idxToolUse = lower.indexOf('"tool_use"', offset);
    let idx = -1;
    let matched = '';
    if (idxToolCalls >= 0 && (idxFunction < 0 || idxToolCalls <= idxFunction)) {
      idx = idxToolCalls;
      matched = 'tool_calls';
    } else if (idxFunction >= 0) {
      idx = idxFunction;
      matched = '"function"';
    }
    if (idxFunctionCall >= 0 && (idx < 0 || idxFunctionCall < idx)) {
      idx = idxFunctionCall;
      matched = 'functioncall';
    }
    if (idxToolUse >= 0 && (idx < 0 || idxToolUse < idx)) {
      idx = idxToolUse;
      matched = '"tool_use"';
    }
    if (idx < 0) {
      break;
    }
    let start = raw.slice(0, idx).lastIndexOf('{');
    while (start >= 0) {
      const obj = extractJSONObjectFrom(raw, start);
      if (obj.ok) {
        out.push(raw.slice(start, obj.end).trim());
        // Ensure forward progress even when the matched keyword is outside
        // the extracted JSON object (e.g. closing XML wrapper tags containing
        // "tool_calls" after an earlier JSON arguments object).
        offset = Math.max(obj.end, idx + matched.length);
        idx = -1;
        break;
      }
      start = raw.slice(0, start).lastIndexOf('{');
    }
    if (idx >= 0) {
      offset = idx + matched.length;
    }
  }

  return out;
}

function parseToolCallsPayload(payload) {
  let decoded;
  try {
    decoded = JSON.parse(payload);
  } catch (_err) {
    return [];
  }

  if (Array.isArray(decoded)) {
    return parseToolCallList(decoded);
  }
  if (!decoded || typeof decoded !== 'object') {
    return [];
  }
  if (decoded.tool_calls) {
    if (isLikelyChatMessageEnvelope(decoded)) {
      return [];
    }
    return parseToolCallList(decoded.tool_calls);
  }

  const one = parseToolCallItem(decoded);
  return one ? [one] : [];
}

function isLikelyChatMessageEnvelope(value) {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return false;
  }
  if (!Object.prototype.hasOwnProperty.call(value, 'tool_calls')) {
    return false;
  }
  const role = toStringSafe(value.role).trim().toLowerCase();
  if (role === 'assistant' || role === 'tool' || role === 'user' || role === 'system') {
    return true;
  }
  return Object.prototype.hasOwnProperty.call(value, 'tool_call_id')
    || Object.prototype.hasOwnProperty.call(value, 'content');
}

function parseMarkupToolCalls(text) {
  const raw = toStringSafe(text).trim();
  if (!raw) {
    return [];
  }
  const out = [];
  for (const m of raw.matchAll(TOOL_CALL_MARKUP_BLOCK_PATTERN)) {
    const parsed = parseMarkupSingleToolCall(toStringSafe(m[2]).trim(), toStringSafe(m[3]).trim());
    if (parsed) {
      out.push(parsed);
    }
  }
  for (const m of raw.matchAll(TOOL_CALL_MARKUP_SELFCLOSE_PATTERN)) {
    const parsed = parseMarkupSingleToolCall(toStringSafe(m[1]).trim(), '');
    if (parsed) {
      out.push(parsed);
    }
  }
  return out;
}

function parseTextKVToolCalls(text) {
  const raw = toStringSafe(text);
  if (!raw) {
    return [];
  }
  const out = [];
  const matches = [...raw.matchAll(TEXT_KV_NAME_PATTERN)];
  if (matches.length === 0) {
    return out;
  }
  for (let i = 0; i < matches.length; i += 1) {
    const match = matches[i];
    const name = toStringSafe(match[1]).trim();
    if (!name) {
      continue;
    }
    const nameEnd = match.index + toStringSafe(match[0]).length;
    const searchEnd = i + 1 < matches.length ? matches[i + 1].index : raw.length;
    const searchArea = raw.slice(nameEnd, searchEnd);
    const argIdx = searchArea.indexOf('function.arguments:');
    if (argIdx < 0) {
      continue;
    }
    const argStart = nameEnd + argIdx + 'function.arguments:'.length;
    const bracePos = raw.slice(argStart, searchEnd).indexOf('{');
    if (bracePos < 0) {
      continue;
    }
    const objStart = argStart + bracePos;
    const obj = extractJSONObjectFrom(raw, objStart);
    if (!obj.ok) {
      continue;
    }
    out.push({
      name,
      input: parseToolCallInput(raw.slice(objStart, obj.end)),
    });
  }
  return out;
}

function parseMarkupSingleToolCall(attrs, inner) {
  const embedded = parseToolCallsPayload(inner);
  if (embedded.length > 0) {
    return embedded[0];
  }
  let name = '';
  const attrMatch = attrs.match(TOOL_CALL_MARKUP_ATTR_PATTERN);
  if (attrMatch && attrMatch[2]) {
    name = toStringSafe(attrMatch[2]).trim();
  }
  if (!name) {
    name = stripTagText(findMarkupTagValue(inner, TOOL_CALL_MARKUP_NAME_PATTERNS));
  }
  if (!name) {
    return null;
  }

  let input = {};
  const argsRaw = findMarkupTagValue(inner, TOOL_CALL_MARKUP_ARGS_PATTERNS);
  if (argsRaw) {
    input = parseMarkupInput(argsRaw);
  } else {
    const kv = parseMarkupKVObject(inner);
    if (Object.keys(kv).length > 0) {
      input = kv;
    }
  }
  return { name, input };
}

function parseMarkupInput(raw) {
  const s = toStringSafe(raw).trim();
  if (!s) {
    return {};
  }
  const named = parseNamedParameterElements(s);
  if (Object.keys(named).length > 0) {
    return named;
  }
  const parsed = parseToolCallInput(s);
  if (parsed && typeof parsed === 'object' && !Array.isArray(parsed) && Object.keys(parsed).length > 0) {
    return parsed;
  }
  const kv = parseMarkupKVObject(s);
  if (Object.keys(kv).length > 0) {
    return kv;
  }
  return { _raw: stripTagText(s) };
}

function parseNamedParameterElements(text) {
  const raw = toStringSafe(text);
  if (!raw) {
    return {};
  }
  const out = {};
  for (const m of raw.matchAll(NAMED_PARAMETER_PATTERN)) {
    const key = toStringSafe(m[1]).trim();
    if (!key) {
      continue;
    }
    let value = toStringSafe(m[2]);
    value = unwrapCDATA(value);
    value = normalizeParamValue(value);
    out[key] = value;
  }
  return out;
}

function unwrapCDATA(value) {
  const t = toStringSafe(value).trim();
  if (t.startsWith('<![CDATA[') && t.endsWith(']]>')) {
    return t.slice('<![CDATA['.length, t.length - ']]>'.length);
  }
  return value;
}

function normalizeParamValue(value) {
  const s = toStringSafe(value);
  if (s.includes('\n')) {
    return s.replace(/^\n+/, '').replace(/\n+$/, '');
  }
  return s.trim();
}

function parseMarkupKVObject(text) {
  const raw = toStringSafe(text).trim();
  if (!raw) {
    return {};
  }
  const out = {};
  for (const m of raw.matchAll(TOOL_CALL_MARKUP_KV_PATTERN)) {
    const key = toStringSafe(m[1]).trim();
    if (!key) {
      continue;
    }
    const valueRaw = stripTagText(m[2]);
    if (!valueRaw) {
      continue;
    }
    try {
      out[key] = JSON.parse(valueRaw);
    } catch (_err) {
      out[key] = valueRaw;
    }
  }
  return out;
}

function stripTagText(text) {
  return toStringSafe(text).replace(/<[^>]+>/g, ' ').trim();
}

function findMarkupTagValue(text, patterns) {
  const source = toStringSafe(text);
  for (const p of patterns) {
    const m = source.match(p);
    if (m && m[1]) {
      return toStringSafe(m[1]);
    }
  }
  return '';
}

function parseToolCallList(v) {
  if (!Array.isArray(v)) {
    return [];
  }
  const out = [];
  for (const item of v) {
    if (!item || typeof item !== 'object') {
      continue;
    }
    const one = parseToolCallItem(item);
    if (one) {
      out.push(one);
    }
  }
  return out;
}

function parseToolCallItem(m) {
  let name = toStringSafe(m.name);
  let inputRaw = m.input;
  let hasInput = Object.prototype.hasOwnProperty.call(m, 'input');
  const fnCall = m.functionCall && typeof m.functionCall === 'object' ? m.functionCall : null;
  if (fnCall) {
    if (!name) {
      name = toStringSafe(fnCall.name);
    }
    if (!hasInput && Object.prototype.hasOwnProperty.call(fnCall, 'args')) {
      inputRaw = fnCall.args;
      hasInput = true;
    }
    if (!hasInput && Object.prototype.hasOwnProperty.call(fnCall, 'arguments')) {
      inputRaw = fnCall.arguments;
      hasInput = true;
    }
  }
  const fn = m.function && typeof m.function === 'object' ? m.function : null;

  if (fn) {
    if (!name) {
      name = toStringSafe(fn.name);
    }
    if (!hasInput && Object.prototype.hasOwnProperty.call(fn, 'arguments')) {
      inputRaw = fn.arguments;
      hasInput = true;
    }
  }

  if (!hasInput) {
    for (const k of ['arguments', 'args', 'parameters', 'params']) {
      if (Object.prototype.hasOwnProperty.call(m, k)) {
        inputRaw = m[k];
        hasInput = true;
        break;
      }
    }
  }

  if (!name) {
    return null;
  }

  return {
    name,
    input: parseToolCallInput(inputRaw),
  };
}

function parseToolCallInput(v) {
  if (v == null) {
    return {};
  }
  if (typeof v === 'string') {
    const raw = toStringSafe(v);
    if (!raw) {
      return {};
    }
    try {
      const parsed = JSON.parse(raw);
      if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
        return parsed;
      }
      return { _raw: raw };
    } catch (_err) {
      return { _raw: raw };
    }
  }
  if (typeof v === 'object' && !Array.isArray(v)) {
    return v;
  }
  try {
    const parsed = JSON.parse(JSON.stringify(v));
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      return parsed;
    }
  } catch (_err) {
    return {};
  }
  return {};
}

module.exports = {
  stripFencedCodeBlocks,
  buildToolCallCandidates,
  parseToolCallsPayload,
  parseMarkupToolCalls,
  parseTextKVToolCalls,
};
