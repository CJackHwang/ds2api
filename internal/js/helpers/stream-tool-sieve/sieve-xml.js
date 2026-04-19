'use strict';
const { parseToolCalls } = require('./parse');

// Tag pairs ordered longest-first: wrapper tags checked before inner tags.
const XML_TOOL_TAG_PAIRS = [
  { open: '<tool_calls', close: '</tool_calls>' },
  { open: '<tool_call', close: '</tool_call>' },
  { open: '<function_calls', close: '</function_calls>' },
  { open: '<function_call', close: '</function_call>' },
  { open: '<invoke', close: '</invoke>' },
  { open: '<tool_use', close: '</tool_use>' },
];

const XML_TOOL_OPENING_TAGS = XML_TOOL_TAG_PAIRS.map(p => p.open);

function consumeXMLToolCapture(captured, toolNames, trimWrappingJSONFence) {
  const lower = captured.toLowerCase();
  // Find the FIRST matching open/close pair, preferring wrapper tags.
  for (const pair of XML_TOOL_TAG_PAIRS) {
    const openIdx = lower.indexOf(pair.open);
    if (openIdx < 0) {
      continue;
    }
    // Find the LAST occurrence of the specific closing tag.
    const closeIdx = lower.lastIndexOf(pair.close);
    if (closeIdx < openIdx) {
      // Opening tag present but specific closing tag hasn't arrived.
      // Return not-ready — do NOT fall through to inner pairs.
      return { ready: false, prefix: '', calls: [], suffix: '' };
    }
    const closeEnd = closeIdx + pair.close.length;
    const xmlBlock = captured.slice(openIdx, closeEnd);
    let prefixPart = captured.slice(0, openIdx);
    let suffixPart = captured.slice(closeEnd);
    const parsed = parseToolCalls(xmlBlock, toolNames);
    if (Array.isArray(parsed) && parsed.length > 0) {
      const trimmedFence = trimWrappingJSONFence(prefixPart, suffixPart);
      return {
        ready: true,
        prefix: trimmedFence.prefix,
        calls: parsed,
        suffix: trimmedFence.suffix,
      };
    }
    // If this block failed to become a tool call, pass it through as text.
    return { ready: true, prefix: prefixPart + xmlBlock, calls: [], suffix: suffixPart };
  }
  return { ready: false, prefix: '', calls: [], suffix: '' };
}

function hasOpenXMLToolTag(captured) {
  const lower = captured.toLowerCase();
  for (const pair of XML_TOOL_TAG_PAIRS) {
    if (lower.includes(pair.open)) {
      if (!lower.includes(pair.close)) {
        return true;
      }
    }
  }
  return false;
}

function findPartialXMLToolTagStart(s) {
  const lastLT = s.lastIndexOf('<');
  if (lastLT < 0) {
    return -1;
  }
  const tail = s.slice(lastLT);
  if (tail.includes('>')) {
    return -1;
  }
  const lowerTail = tail.toLowerCase();
  for (const tag of XML_TOOL_OPENING_TAGS) {
    const tagWithLT = tag.startsWith('<') ? tag : '<' + tag;
    if (tagWithLT.startsWith(lowerTail)) {
      return lastLT;
    }
  }
  return -1;
}

module.exports = {
  consumeXMLToolCapture,
  hasOpenXMLToolTag,
  findPartialXMLToolTagStart,
};
