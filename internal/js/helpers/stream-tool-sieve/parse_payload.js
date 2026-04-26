'use strict';

const TOOL_CALL_MARKUP_KV_PATTERN = /<(?:[a-z0-9_:-]+:)?([a-z0-9_.-]+)\b[^>]*>([\s\S]*?)<\/(?:[a-z0-9_:-]+:)?\1>/gi;
const CDATA_PATTERN = /^<!\[CDATA\[([\s\S]*?)]]>$/i;
const XML_ATTR_PATTERN = /\b([a-z0-9_:-]+)\s*=\s*("([^"]*)"|'([^']*)')/gi;

const {
  toStringSafe,
} = require('./state');

const DSML_TOOL_TAG_PATTERN = /<\s*\/?\s*(?:\|?\s*dsml\s*\|)\s*(tool_calls|invoke|parameter)\b/i;
const DSML_TOOL_TAG_NORMALIZE_PATTERN = /<\s*(\/?)\s*(?:\|?\s*dsml\s*\|)\s*(tool_calls|invoke|parameter)\b/gi;

function stripFencedCodeBlocks(text) {
  const t = typeof text === 'string' ? text : '';
  if (!t) {
    return '';
  }
  return t.replace(/```[\s\S]*?```/g, ' ');
}

function parseMarkupToolCalls(text) {
  const raw = normalizeDSMLToolCallSyntax(toStringSafe(text)).trim();
  if (!raw) {
    return [];
  }
  const out = [];
  for (const wrapper of findXmlElementBlocks(raw, 'tool_calls')) {
    const body = toStringSafe(wrapper.body);
    for (const block of findXmlElementBlocks(body, 'invoke')) {
      const parsed = parseMarkupSingleToolCall(block);
      if (parsed) {
        out.push(parsed);
      }
    }
  }
  return out;
}

function normalizeDSMLToolCallSyntax(text) {
  const raw = toStringSafe(text);
  if (!raw) {
    return '';
  }
  if (!DSML_TOOL_TAG_PATTERN.test(raw)) {
    return raw;
  }
  DSML_TOOL_TAG_PATTERN.lastIndex = 0;
  return raw.replace(DSML_TOOL_TAG_NORMALIZE_PATTERN, '<$1$2');
}

function parseMarkupSingleToolCall(block) {
  const attrs = parseTagAttributes(block.attrs);
  const name = toStringSafe(attrs.name).trim();
  if (!name) {
    return null;
  }
  const inner = toStringSafe(block.body).trim();

  if (inner) {
    try {
      const decoded = JSON.parse(inner);
      if (decoded && typeof decoded === 'object' && !Array.isArray(decoded)) {
        return {
          name,
          input: decoded.input && typeof decoded.input === 'object' && !Array.isArray(decoded.input)
            ? decoded.input
            : decoded.parameters && typeof decoded.parameters === 'object' && !Array.isArray(decoded.parameters)
              ? decoded.parameters
              : {},
        };
      }
    } catch (_err) {
      // Not JSON, continue with markup parsing.
    }
  }
  const input = {};
  for (const match of findXmlElementBlocks(inner, 'parameter')) {
    const parameterAttrs = parseTagAttributes(match.attrs);
    const paramName = toStringSafe(parameterAttrs.name).trim();
    if (!paramName) {
      continue;
    }
    appendMarkupValue(input, paramName, parseMarkupValueWithStringHint(match.body, parameterAttrs.string));
  }
  if (Object.keys(input).length === 0 && inner.trim() !== '') {
    return null;
  }
  return { name, input };
}

function findXmlElementBlocks(text, tag) {
  const source = toStringSafe(text);
  const name = toStringSafe(tag).toLowerCase();
  if (!source || !name) {
    return [];
  }
  const out = [];
  let pos = 0;
  while (pos < source.length) {
    const start = findXmlStartTagOutsideCDATA(source, name, pos);
    if (!start) {
      break;
    }
    const end = findMatchingXmlEndTagOutsideCDATA(source, name, start.bodyStart);
    if (!end) {
      break;
    }
    out.push({
      attrs: start.attrs,
      body: source.slice(start.bodyStart, end.closeStart),
      start: start.start,
      end: end.closeEnd,
    });
    pos = end.closeEnd;
  }
  return out;
}

function findXmlStartTagOutsideCDATA(text, tag, from) {
  const lower = text.toLowerCase();
  const target = `<${tag}`;
  for (let i = Math.max(0, from || 0); i < text.length;) {
    const skipped = skipXmlIgnoredSection(lower, i);
    if (skipped.blocked) {
      return null;
    }
    if (skipped.advanced) {
      i = skipped.next;
      continue;
    }
    if (lower.startsWith(target, i) && hasXmlTagBoundary(text, i + target.length)) {
      const tagEnd = findXmlTagEnd(text, i + target.length);
      if (tagEnd < 0) {
        return null;
      }
      return {
        start: i,
        bodyStart: tagEnd + 1,
        attrs: text.slice(i + target.length, tagEnd),
      };
    }
    i += 1;
  }
  return null;
}

function findMatchingXmlEndTagOutsideCDATA(text, tag, from) {
  const lower = text.toLowerCase();
  const openTarget = `<${tag}`;
  const closeTarget = `</${tag}`;
  let depth = 1;
  for (let i = Math.max(0, from || 0); i < text.length;) {
    const skipped = skipXmlIgnoredSection(lower, i);
    if (skipped.blocked) {
      return null;
    }
    if (skipped.advanced) {
      i = skipped.next;
      continue;
    }
    if (lower.startsWith(closeTarget, i) && hasXmlTagBoundary(text, i + closeTarget.length)) {
      const tagEnd = findXmlTagEnd(text, i + closeTarget.length);
      if (tagEnd < 0) {
        return null;
      }
      depth -= 1;
      if (depth === 0) {
        return { closeStart: i, closeEnd: tagEnd + 1 };
      }
      i = tagEnd + 1;
      continue;
    }
    if (lower.startsWith(openTarget, i) && hasXmlTagBoundary(text, i + openTarget.length)) {
      const tagEnd = findXmlTagEnd(text, i + openTarget.length);
      if (tagEnd < 0) {
        return null;
      }
      if (!isSelfClosingXmlTag(text.slice(i, tagEnd))) {
        depth += 1;
      }
      i = tagEnd + 1;
      continue;
    }
    i += 1;
  }
  return null;
}

function skipXmlIgnoredSection(lower, i) {
  if (lower.startsWith('<![cdata[', i)) {
    const end = lower.indexOf(']]>', i + '<![cdata['.length);
    if (end < 0) {
      return { advanced: false, blocked: true, next: i };
    }
    return { advanced: true, blocked: false, next: end + ']]>'.length };
  }
  if (lower.startsWith('<!--', i)) {
    const end = lower.indexOf('-->', i + '<!--'.length);
    if (end < 0) {
      return { advanced: false, blocked: true, next: i };
    }
    return { advanced: true, blocked: false, next: end + '-->'.length };
  }
  return { advanced: false, blocked: false, next: i };
}

function findXmlTagEnd(text, from) {
  let quote = '';
  for (let i = Math.max(0, from || 0); i < text.length; i += 1) {
    const ch = text[i];
    if (quote) {
      if (ch === quote) {
        quote = '';
      }
      continue;
    }
    if (ch === '"' || ch === "'") {
      quote = ch;
      continue;
    }
    if (ch === '>') {
      return i;
    }
  }
  return -1;
}

function hasXmlTagBoundary(text, idx) {
  if (idx >= text.length) {
    return true;
  }
  return [' ', '\t', '\n', '\r', '>', '/'].includes(text[idx]);
}

function isSelfClosingXmlTag(startTag) {
  return toStringSafe(startTag).trim().endsWith('/');
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
    const key = toStringSafe(m[1]).trim();
    if (!key) {
      continue;
    }
    const value = parseMarkupValue(m[2]);
    if (value === undefined || value === null) {
      continue;
    }
    appendMarkupValue(out, key, value);
  }
  return out;
}

function parseMarkupValue(raw) {
  const cdata = extractStandaloneCDATA(raw);
  if (cdata.ok) {
    return cdata.value;
  }
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

  if (s.startsWith('{') || s.startsWith('[')) {
    try {
      return JSON.parse(s);
    } catch (_err) {
      return s;
    }
  }
  return s;
}

function parseMarkupValueWithStringHint(raw, stringAttr) {
  const hint = toStringSafe(stringAttr).trim().toLowerCase();
  if (hint === 'true') {
    return unescapeHtml(extractRawTagValue(raw));
  }
  if (hint === 'false') {
    const value = toStringSafe(unescapeHtml(extractRawTagValue(raw))).trim();
    if (!value) {
      return '';
    }
    try {
      return JSON.parse(value);
    } catch (_err) {
      return parseMarkupValue(raw);
    }
  }
  return parseMarkupValue(raw);
}

function extractRawTagValue(inner) {
  const s = toStringSafe(inner).trim();
  if (!s) {
    return '';
  }

  // 1. Check for CDATA
  const cdata = extractStandaloneCDATA(s);
  if (cdata.ok) {
    return cdata.value;
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

function extractStandaloneCDATA(inner) {
  const s = toStringSafe(inner).trim();
  const cdataMatch = s.match(CDATA_PATTERN);
  if (cdataMatch && cdataMatch[1] !== undefined) {
    return { ok: true, value: cdataMatch[1] };
  }
  return { ok: false, value: '' };
}

function parseTagAttributes(raw) {
  const source = toStringSafe(raw);
  const out = {};
  if (!source) {
    return out;
  }
  for (const match of source.matchAll(XML_ATTR_PATTERN)) {
    const key = toStringSafe(match[1]).trim().toLowerCase();
    if (!key) {
      continue;
    }
    out[key] = match[3] || match[4] || '';
  }
  return out;
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
  normalizeDSMLToolCallSyntax,
};
