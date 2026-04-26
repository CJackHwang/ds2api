'use strict';

const TOOL_CALL_MARKUP_BLOCK_PATTERN = /<(?:[a-z0-9_:-]+:)?(tool_call|function_call|invoke)\b([^>]*)>([\s\S]*?)<\/(?:[a-z0-9_:-]+:)?\1>/gi;
const TOOL_CALL_MARKUP_SELFCLOSE_PATTERN = /<(?:[a-z0-9_:-]+:)?invoke\b([^>]*)\/>/gi;
const TOOL_CALL_OPEN_TAG_PATTERN = /<tool_call\b[^>]*>/gi;
const TOOL_CALL_SINGLE_BLOCK_PATTERN = /^<(?:[a-z0-9_:-]+:)?tool_call\b([^>]*)>([\s\S]*?)<\/(?:[a-z0-9_:-]+:)?tool_call>$/i;
const MALFORMED_TOOL_NAME_TAG_PATTERN = /<(tool_name|function_name)\s*=\s*["']([^<"']+)<\/\1>/gi;
const TOOL_CALL_MARKUP_KV_PATTERN = /<(?:[a-z0-9_:-]+:)?([a-z0-9_.-]+)\b([^>]*)>([\s\S]*?)<\/(?:[a-z0-9_:-]+:)?([a-z0-9_.-]+)>/gi;
const TOOL_CALL_MARKUP_ATTR_PATTERN = /(name|function|tool)\s*=\s*["']([^"']+)["']/i;
const DIRECT_NAMED_MARKUP_PARAM_PATTERN = /<(?:[a-z0-9_:-]+:)?(?:parameter|argument)\b[^>]*\bname\s*=\s*["']([^"']+)["'][^>]*>\s*([\s\S]*?)(?:<\/(?:[a-z0-9_:-]+:)?(?:parameter|argument)>|(?=<(?:[a-z0-9_:-]+:)?(?:parameter|argument)\b)|(?=<(?:[a-z0-9_:-]+:)?tool_call\b)|(?=<\/(?:[a-z0-9_:-]+:)?tool_call>)|(?=<\/(?:[a-z0-9_:-]+:)?tool_calls>)|$)/gi;
const TOOL_CALL_MARKUP_NAME_ATTR_PATTERNS = [
  /<(?:[a-z0-9_:-]+:)?tool_name\b[^>]*\bname\s*=\s*["']([^"']+)["'][^>]*>/i,
  /<(?:[a-z0-9_:-]+:)?function_name\b[^>]*\bname\s*=\s*["']([^"']+)["'][^>]*>/i,
];
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
const CDATA_PATTERN = /^<!\[CDATA\[([\s\S]*?)]]>$/i;
const HTML_ENTITIES_PATTERN = /&[a-z0-9#]+;/gi;

const {
  toStringSafe,
} = require('./state');

function stripFencedCodeBlocks(text) {
  const t = typeof text === 'string' ? text : '';
  if (!t) {
    return '';
  }
  return t.replace(/```[\s\S]*?```/g, ' ');
}

function repairMalformedToolNameTags(text) {
  return toStringSafe(text).replace(MALFORMED_TOOL_NAME_TAG_PATTERN, '<$1>$2</$1>');
}

function parseMarkupToolCalls(text) {
  const raw = repairMalformedToolNameTags(toStringSafe(text)).trim();
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
  const repaired = parseRepairedToolCallBlocks(raw);
  if (repaired.length > out.length) {
    return repaired;
  }
  return out;
}

function parseRepairedToolCallBlocks(text) {
  const raw = toStringSafe(text).trim();
  if (!raw) {
    return [];
  }
  const lower = raw.toLowerCase();
  const starts = [...raw.matchAll(TOOL_CALL_OPEN_TAG_PATTERN)].map((m) => m.index).filter((v) => Number.isInteger(v));
  if (starts.length === 0) {
    return [];
  }
  const out = [];
  for (let i = 0; i < starts.length; i += 1) {
    const start = starts[i];
    const nextStart = i + 1 < starts.length ? starts[i + 1] : raw.length;
    const explicitRel = lower.indexOf('</tool_call>', start);
    const explicitEnd = explicitRel >= 0 ? explicitRel + '</tool_call>'.length : -1;
    const wrapperRel = lower.indexOf('</tool_calls>', start);
    const wrapperPos = wrapperRel >= 0 ? wrapperRel : -1;

    let end = raw.length;
    let implicitClose = true;
    if (explicitEnd >= 0 && explicitEnd <= nextStart && (wrapperPos < 0 || explicitEnd <= wrapperPos)) {
      end = explicitEnd;
      implicitClose = false;
    } else if (wrapperPos >= 0 && wrapperPos < nextStart) {
      end = wrapperPos;
    } else if (nextStart < raw.length) {
      end = nextStart;
    } else if (explicitEnd >= 0) {
      end = explicitEnd;
      implicitClose = false;
    } else if (wrapperPos >= 0) {
      end = wrapperPos;
    }

    let segment = raw.slice(start, end).trim();
    if (!segment) {
      continue;
    }
    if (implicitClose && !segment.toLowerCase().includes('</tool_call>')) {
      segment += '</tool_call>';
    }
    const match = segment.match(TOOL_CALL_SINGLE_BLOCK_PATTERN);
    if (!match) {
      continue;
    }
    const parsed = parseMarkupSingleToolCall(toStringSafe(match[1]).trim(), toStringSafe(match[2]).trim());
    if (parsed) {
      out.push(parsed);
    }
  }
  return out;
}

function parseMarkupSingleToolCall(attrs, inner) {
  // Try inline JSON parse for the inner content.
  if (inner) {
    try {
      const decoded = JSON.parse(inner);
      if (decoded && typeof decoded === 'object' && !Array.isArray(decoded) && decoded.name) {
        return {
          name: toStringSafe(decoded.name),
          input: decoded.input && typeof decoded.input === 'object' && !Array.isArray(decoded.input) ? decoded.input : {},
        };
      }
    } catch (_err) {
      // Not JSON, continue with markup parsing.
    }
  }
  let name = '';
  const attrMatch = attrs.match(TOOL_CALL_MARKUP_ATTR_PATTERN);
  if (attrMatch && attrMatch[2]) {
    name = toStringSafe(attrMatch[2]).trim();
  }
  if (!name) {
    name = extractMarkupToolName(inner);
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
  input = mergeDirectNamedMarkupParams(input, inner);
  return { name, input };
}

function parseMarkupInput(raw) {
  const s = toStringSafe(raw).trim();
  if (!s) {
    return {};
  }
  // Prioritize XML-style KV tags (e.g., <arg>val</arg>)
  const kv = parseMarkupKVObject(s);
  if (Object.keys(kv).length > 0) {
    return kv;
  }

  // Fallback to JSON parsing
  const parsed = parseToolCallInput(s);
  if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
    if (Object.keys(parsed).length > 0) {
      return parsed;
    }
  }

  return { _raw: extractRawTagValue(s) };
}

function parseMarkupKVObject(text) {
  const raw = toStringSafe(text).trim();
  if (!raw) {
    return {};
  }
  const out = {};
  for (const m of raw.matchAll(TOOL_CALL_MARKUP_KV_PATTERN)) {
    let key = toStringSafe(m[1]).trim();
    const attrs = toStringSafe(m[2]).trim();
    const endKey = toStringSafe(m[4]).trim();
    if (!key || key.toLowerCase() !== endKey.toLowerCase()) {
      continue;
    }
    if ((key.toLowerCase() === 'parameter' || key.toLowerCase() === 'argument') && attrs) {
      const attrMatch = attrs.match(TOOL_CALL_MARKUP_ATTR_PATTERN);
      if (attrMatch && attrMatch[1] && attrMatch[1].toLowerCase() === 'name' && attrMatch[2]) {
        key = toStringSafe(attrMatch[2]).trim();
      }
    }
    if (!key) {
      continue;
    }
    const value = parseMarkupValue(m[3]);
    if (value === undefined || value === null) {
      continue;
    }
    appendMarkupValue(out, key, value);
  }
  return out;
}

function extractMarkupToolName(inner) {
  const direct = extractRawTagValue(findMarkupTagValue(inner, TOOL_CALL_MARKUP_NAME_PATTERNS));
  if (direct) {
    return direct;
  }
  const raw = toStringSafe(inner);
  for (const pattern of TOOL_CALL_MARKUP_NAME_ATTR_PATTERNS) {
    const match = raw.match(pattern);
    if (match && match[1]) {
      return toStringSafe(match[1]).trim();
    }
  }
  return '';
}

function parseMarkupValue(raw) {
  const s = toStringSafe(extractRawTagValue(raw)).trim();
  if (!s) {
    return '';
  }

  if (s.includes('<') && s.includes('>')) {
    const nested = parseMarkupInput(s);
    if (nested && typeof nested === 'object' && !Array.isArray(nested)) {
      if (isOnlyRawValue(nested)) {
        return toStringSafe(nested._raw);
      }
      return nested;
    }
  }

  try {
    return JSON.parse(s);
  } catch (_err) {
    return s;
  }
}

function extractRawTagValue(inner) {
  const s = toStringSafe(inner).trim();
  if (!s) {
    return '';
  }

  // 1. Check for CDATA
  const cdataMatch = s.match(CDATA_PATTERN);
  if (cdataMatch && cdataMatch[1] !== undefined) {
    return cdataMatch[1];
  }

  // 2. Fallback to unescaping standard HTML entities
  // Note: we avoid broad tag stripping here to preserve user content (like < symbols in code)
  return unescapeHtml(inner);
}

function unescapeHtml(safe) {
  if (!safe) return '';
  return safe.replace(/&amp;/g, '&')
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&quot;/g, '"')
    .replace(/&#039;/g, "'")
    .replace(/&#x27;/g, "'");
}

function stripTagText(text) {
  return toStringSafe(text).replace(/<[^>]+>/g, ' ').trim();
}

function findMarkupTagValue(text, patterns) {
  const source = toStringSafe(text);
  for (const p of patterns) {
    const m = source.match(p);
    if (m && m[1] !== undefined) {
      return toStringSafe(m[1]);
    }
  }
  return '';
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

function appendMarkupValue(out, key, value) {
  if (Object.prototype.hasOwnProperty.call(out, key)) {
    const current = out[key];
    if (Array.isArray(current)) {
      current.push(value);
      return;
    }
    out[key] = [current, value];
    return;
  }
  out[key] = value;
}

function clonePlainObject(obj) {
  if (!obj || typeof obj !== 'object' || Array.isArray(obj)) {
    return {};
  }
  return { ...obj };
}

function mergeDirectNamedMarkupParams(base, inner) {
  const out = clonePlainObject(base);
  let found = false;
  for (const match of toStringSafe(inner).matchAll(DIRECT_NAMED_MARKUP_PARAM_PATTERN)) {
    const key = toStringSafe(match[1]).trim();
    if (!key) {
      continue;
    }
    const value = extractRawTagValue(match[2]);
    if (!value) {
      continue;
    }
    const parsed = parseToolCallInput(value);
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed) && Object.keys(parsed).length > 0 && !isOnlyRawValue(parsed)) {
      if (!Object.prototype.hasOwnProperty.call(out, key) || JSON.stringify(out[key]) !== JSON.stringify(parsed)) {
        appendMarkupValue(out, key, parsed);
      }
    } else if (!Object.prototype.hasOwnProperty.call(out, key) || JSON.stringify(out[key]) !== JSON.stringify(value)) {
      appendMarkupValue(out, key, value);
    }
    found = true;
  }
  if (found && Object.prototype.hasOwnProperty.call(out, '_raw') && Object.keys(out).length > 1) {
    delete out._raw;
  }
  return out;
}

function isOnlyRawValue(obj) {
  if (!obj || typeof obj !== 'object' || Array.isArray(obj)) {
    return false;
  }
  const keys = Object.keys(obj);
  return keys.length === 1 && keys[0] === '_raw';
}

module.exports = {
  stripFencedCodeBlocks,
  parseMarkupToolCalls,
};
