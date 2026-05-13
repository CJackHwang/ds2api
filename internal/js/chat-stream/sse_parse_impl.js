'use strict';

// Implementation moved here to keep the line-gate wrapper tiny.

const {
  SKIP_PATTERNS,
  SKIP_EXACT_PATHS,
} = require('../shared/deepseek-constants');

const LEAKED_BOS_MARKER_PATTERN = /<[\|\uFF5C]\s*begin[_▁]of[_▁]sentence\s*[\|\uFF5C]>/gi;
const LEAKED_THOUGHT_MARKER_PATTERN = /<[\|\uFF5C]\s*(?:begin[_▁])?[_▁]*of[_▁]thought\s*[\|\uFF5C]>/gi;
const LEAKED_META_MARKER_PATTERN = /<[\|\uFF5C]\s*(?:assistant|tool|end[_▁]of[_▁]sentence|end[_▁]of[_▁]thinking|end[_▁]of[_▁]thought|end[_▁]of[_▁]toolresults|end[_▁]of[_▁]instructions)\s*[\|\uFF5C]>/gi;



function stripThinkTags(text) {
  if (typeof text !== 'string' || !text) {
    return text;
  }
  return text.replace(/<\/?\s*think\s*>/gi, '');
}

function splitThinkingParts(parts) {
  const out = [];
  let thinkingDone = false;
  for (const p of parts) {
    if (!p) continue;
    if (thinkingDone && p.type === 'thinking') {
      const cleaned = stripThinkTags(p.text);
      if (cleaned) {
        out.push({ text: cleaned, type: 'text' });
      }
      continue;
    }
    if (p.type !== 'thinking') {
      const cleaned = stripThinkTags(p.text);
      if (cleaned) {
        out.push({ text: cleaned, type: p.type });
      }
      continue;
    }
    const match = /<\/\s*think\s*>/i.exec(p.text);
    if (!match) {
      out.push(p);
      continue;
    }
    thinkingDone = true;
    const before = p.text.substring(0, match.index);
    let after = p.text.substring(match.index + match[0].length);
    if (before) {
      out.push({ text: before, type: 'thinking' });
    }
    after = stripThinkTags(after);
    if (after) {
      out.push({ text: after, type: 'text' });
    }
  }
  return { parts: out, transitioned: thinkingDone };
}

function dropThinkingParts(parts) {
  if (!Array.isArray(parts) || parts.length === 0) {
    return parts;
  }
  return parts.filter((p) => p && p.type !== 'thinking');
}

function finalizeThinkingParts(parts, thinkingEnabled, newType) {
  const splitResult = splitThinkingParts(parts);
  let finalType = newType;
  let finalParts = splitResult.parts;
  if (splitResult.transitioned) {
    finalType = 'text';
  }
  if (!thinkingEnabled) {
    finalParts = dropThinkingParts(finalParts);
  }
  return { parts: finalParts, newType: finalType };
}

function parseChunkForContent(chunk, thinkingEnabled, currentType, stripReferenceMarkers = true) {
  if (!chunk || typeof chunk !== 'object') {
    return {
      parsed: false,
      parts: [],
      finished: false,
      contentFilter: false,
      errorMessage: '',
      outputTokens: 0,
      newType: currentType,
    };
  }

  const usage = extractAccumulatedTokenUsage(chunk);
  const promptTokens = usage.prompt;
  const outputTokens = usage.output;

  if (Object.prototype.hasOwnProperty.call(chunk, 'error')) {
    return {
      parsed: true,
      parts: [],
      finished: true,
      contentFilter: false,
      errorMessage: formatErrorMessage(chunk.error),
      promptTokens,
      outputTokens,
      newType: currentType,
    };
  }

  const pathValue = asString(chunk.p);

  if (hasContentFilterStatus(chunk)) {
    return {
      parsed: true,
      parts: [],
      finished: true,
      contentFilter: true,
      errorMessage: '',
      promptTokens,
      outputTokens,
      newType: currentType,
    };
  }

  if (shouldSkipPath(pathValue)) {
    return {
      parsed: true,
      parts: [],
      finished: false,
      contentFilter: false,
      errorMessage: '',
      promptTokens,
      outputTokens,
      newType: currentType,
    };
  }
  if (isStatusPath(pathValue)) {
    if (isFinishedStatus(chunk.v)) {
      return {
        parsed: true,
        parts: [],
        finished: true,
        contentFilter: false,
        errorMessage: '',
        promptTokens,
        outputTokens,
        newType: currentType,
      };
    }
    return {
      parsed: true,
      parts: [],
      finished: false,
      contentFilter: false,
      errorMessage: '',
      promptTokens,
      outputTokens,
      newType: currentType,
    };
  }

  if (!Object.prototype.hasOwnProperty.call(chunk, 'v')) {
    return {
      parsed: true,
      parts: [],
      finished: false,
      contentFilter: false,
      errorMessage: '',
      promptTokens,
      outputTokens,
      newType: currentType,
    };
  }

  let newType = currentType;
  const parts = [];

  if (pathValue === 'response/fragments' && asString(chunk.o).toUpperCase() === 'APPEND' && Array.isArray(chunk.v)) {
    for (const frag of chunk.v) {
      if (!frag || typeof frag !== 'object') {
        continue;
      }
      const fragType = asString(frag.type).toUpperCase();
      const content = asContentString(frag.content, stripReferenceMarkers);
      if (!content) {
        continue;
      }
      if (fragType === 'THINK' || fragType === 'THINKING') {
        newType = 'thinking';
        parts.push({ text: content, type: 'thinking' });
      } else if (fragType === 'RESPONSE') {
        newType = 'text';
        parts.push({ text: content, type: 'text' });
      } else {
        parts.push({ text: content, type: 'text' });
      }
    }
  }

  if (pathValue === 'response' && Array.isArray(chunk.v)) {
    for (const item of chunk.v) {
      if (!item || typeof item !== 'object') {
        continue;
      }
      if (item.p === 'fragments' && item.o === 'APPEND' && Array.isArray(item.v)) {
        for (const frag of item.v) {
          const fragType = asString(frag && frag.type).toUpperCase();
          if (fragType === 'THINK' || fragType === 'THINKING') {
            newType = 'thinking';
          } else if (fragType === 'RESPONSE') {
            newType = 'text';
          }
        }
      }
    }
  }

  if (pathValue === 'response/content') {
    newType = 'text';
  } else if (pathValue === 'response/thinking_content' && (!thinkingEnabled || newType !== 'text')) {
    newType = 'thinking';
  }

  let partType = 'text';
  if (pathValue === 'response/thinking_content') {
    if (!thinkingEnabled) {
      partType = 'thinking';
    } else if (newType === 'text') {
      partType = 'text';
    } else {
      partType = 'thinking';
    }
  } else if (pathValue === 'response/content') {
    partType = 'text';
  } else if (pathValue.includes('response/fragments') && pathValue.includes('/content')) {
    partType = newType;
  } else if (!pathValue) {
    partType = newType || 'text';
  }

  const val = chunk.v;
  if (typeof val === 'string') {
    if (isFinishedStatus(val) && (!pathValue || pathValue === 'status')) {
      return {
        parsed: true,
        parts: [],
        finished: true,
        contentFilter: false,
        errorMessage: '',
        promptTokens,
        outputTokens,
        newType,
      };
    }
    if (isStatusPath(pathValue)) {
      return {
        parsed: true,
        parts: [],
        finished: false,
        contentFilter: false,
        errorMessage: '',
        promptTokens,
        outputTokens,
        newType,
      };
    }
    const content = asContentString(val, stripReferenceMarkers);
    if (content) {
      parts.push({ text: content, type: partType });
    }
    
    let resolvedParts = filterLeakedContentFilterParts(parts);
    const finalized = finalizeThinkingParts(resolvedParts, thinkingEnabled, newType);
    
    return {
      parsed: true,
      parts: finalized.parts,
      finished: false,
      contentFilter: false,
      errorMessage: '',
      promptTokens,
      outputTokens,
      newType: finalized.newType,
    };
  }

  if (Array.isArray(val)) {
    const extracted = extractContentRecursive(val, partType, stripReferenceMarkers);
    if (extracted.finished) {
      return {
        parsed: true,
        parts: [],
        finished: true,
        contentFilter: false,
        errorMessage: '',
        promptTokens,
        outputTokens,
        newType,
      };
    }
    parts.push(...extracted.parts);
    
    let resolvedParts = filterLeakedContentFilterParts(parts);
    const finalized = finalizeThinkingParts(resolvedParts, thinkingEnabled, newType);
    
    return {
      parsed: true,
      parts: finalized.parts,
      finished: false,
      contentFilter: false,
      errorMessage: '',
      promptTokens,
      outputTokens,
      newType: finalized.newType,
    };
  }

  if (val && typeof val === 'object') {
    const directContent = asContentString(val, stripReferenceMarkers);
    if (directContent) {
      parts.push({ text: directContent, type: partType });
    }
    const resp = val.response && typeof val.response === 'object' ? val.response : val;
    if (Array.isArray(resp.fragments)) {
      for (const frag of resp.fragments) {
        if (!frag || typeof frag !== 'object') {
          continue;
        }
        const content = asContentString(frag.content, stripReferenceMarkers);
        if (!content) {
          continue;
        }
        const t = asString(frag.type).toUpperCase();
        if (t === 'THINK' || t === 'THINKING') {
          newType = 'thinking';
          parts.push({ text: content, type: 'thinking' });
        } else if (t === 'RESPONSE') {
          newType = 'text';
          parts.push({ text: content, type: 'text' });
        } else {
          parts.push({ text: content, type: partType });
        }
      }
    }
  }
  
  let resolvedParts = filterLeakedContentFilterParts(parts);
  const finalized = finalizeThinkingParts(resolvedParts, thinkingEnabled, newType);

  return {
    parsed: true,
    parts: finalized.parts,
    finished: false,
    contentFilter: false,
    errorMessage: '',
    promptTokens,
    outputTokens,
    newType: finalized.newType,
  };
}

function extractContentRecursive(items, defaultType, stripReferenceMarkers = true) {
  const parts = [];
  for (const it of items) {
    if (!it || typeof it !== 'object') {
      continue;
    }
    if (!Object.prototype.hasOwnProperty.call(it, 'v')) {
      continue;
    }
    const itemPath = asString(it.p);
    const itemV = it.v;
    if (isStatusPath(itemPath)) {
      if (isFinishedStatus(itemV)) {
        return { parts: [], finished: true };
      }
      continue;
    }
    if (shouldSkipPath(itemPath)) {
      continue;
    }
    const content = asContentString(it.content, stripReferenceMarkers);
    if (content) {
      const typeName = asString(it.type).toUpperCase();
      if (typeName === 'THINK' || typeName === 'THINKING') {
        parts.push({ text: content, type: 'thinking' });
      } else if (typeName === 'RESPONSE') {
        parts.push({ text: content, type: 'text' });
      } else {
        parts.push({ text: content, type: defaultType });
      }
      continue;
    }

    let partType = defaultType;
    if (itemPath.includes('thinking')) {
      partType = 'thinking';
    } else if (itemPath.includes('content') || itemPath === 'response' || itemPath === 'fragments') {
      partType = 'text';
    }

    if (typeof itemV === 'string') {
      if (isStatusPath(itemPath)) {
        continue;
      }
      if (itemV && itemV !== 'FINISHED') {
        const content = asContentString(itemV, stripReferenceMarkers);
        if (content) {
          parts.push({ text: content, type: partType });
        }
      }
      continue;
    }

    if (!Array.isArray(itemV)) {
      continue;
    }
    for (const inner of itemV) {
      if (typeof inner === 'string') {
        if (inner) {
          const content = asContentString(inner, stripReferenceMarkers);
          if (content) {
            parts.push({ text: content, type: partType });
          }
        }
        continue;
      }
      if (!inner || typeof inner !== 'object') {
        continue;
      }
      const ct = asContentString(inner.content, stripReferenceMarkers);
      if (!ct) {
        continue;
      }
      const typeName = asString(inner.type).toUpperCase();
      if (typeName === 'THINK' || typeName === 'THINKING') {
        parts.push({ text: ct, type: 'thinking' });
      } else if (typeName === 'RESPONSE') {
        parts.push({ text: ct, type: 'text' });
      } else {
        parts.push({ text: ct, type: partType });
      }
    }
  }
  return { parts, finished: false };
}

function isStatusPath(pathValue) {
  return pathValue === 'response/status' || pathValue === 'status';
}

function isFinishedStatus(value) {
  return asString(value).toUpperCase() === 'FINISHED';
}

function filterLeakedContentFilterParts(parts) {
  if (!Array.isArray(parts) || parts.length === 0) {
    return parts;
  }
  const out = [];
  for (const p of parts) {
    if (!p || typeof p !== 'object') {
      continue;
    }
    const { text, stripped } = stripLeakedContentFilterSuffix(p.text);
    if (stripped && shouldDropCleanedLeakedChunk(text)) {
      continue;
    }
    if (stripped) {
      out.push({ ...p, text });
      continue;
    }
    out.push(p);
  }
  return out;
}

function stripLeakedContentFilterSuffix(text) {
  if (typeof text !== 'string' || text === '') {
    return { text, stripped: false };
  }
  const upperText = text.toUpperCase();
  const idx = upperText.indexOf('CONTENT_FILTER');
  if (idx < 0) {
    return { text, stripped: false };
  }
  return {
    text: text.slice(0, idx).replace(/[ \t\r]+$/g, ''),
    stripped: true,
  };
}

function shouldDropCleanedLeakedChunk(cleaned) {
  if (cleaned === '') {
    return true;
  }
  if (typeof cleaned === 'string' && cleaned.includes('\n')) {
    return false;
  }
  return asString(cleaned).trim() === '';
}

function hasContentFilterStatus(chunk) {
  if (!chunk || typeof chunk !== 'object') {
    return false;
  }
  const code = asString(chunk.code);
  if (code && code.toLowerCase() === 'content_filter') {
    return true;
  }
  return hasContentFilterStatusValue(chunk);
}

function hasContentFilterStatusValue(v) {
  if (Array.isArray(v)) {
    for (const item of v) {
      if (hasContentFilterStatusValue(item)) {
        return true;
      }
    }
    return false;
  }
  if (!v || typeof v !== 'object') {
    return false;
  }
  const pathValue = asString(v.p);
  if (pathValue && pathValue.toLowerCase().includes('status')) {
    if (asString(v.v).toLowerCase() === 'content_filter') {
      return true;
    }
  }
  if (asString(v.code).toLowerCase() === 'content_filter') {
    return true;
  }
  for (const value of Object.values(v)) {
    if (hasContentFilterStatusValue(value)) {
      return true;
    }
  }
  return false;
}

function extractAccumulatedTokenUsage(chunk) {
  // 临时策略：忽略上游 usage 字段（accumulated_token_usage / token_usage），
  // 统一使用内部估算计数，避免上下文累计口径误差。
  void chunk;
  return { prompt: 0, output: 0 };
}

function formatErrorMessage(v) {
  if (typeof v === 'string') {
    return v;
  }
  if (v == null) {
    return String(v);
  }
  try {
    return JSON.stringify(v);
  } catch (_err) {
    return String(v);
  }
}

function shouldSkipPath(pathValue) {
  if (isFragmentStatusPath(pathValue)) {
    return true;
  }
  if (SKIP_EXACT_PATHS.has(pathValue)) {
    return true;
  }
  for (const p of SKIP_PATTERNS) {
    if (pathValue.includes(p)) {
      return true;
    }
  }
  return false;
}

function isFragmentStatusPath(pathValue) {
  if (!pathValue || pathValue === 'response/status') {
    return false;
  }
  return /^response\/fragments\/-?\d+\/status$/i.test(pathValue);
}

function isCitation(text) {
  return asString(text).trim().startsWith('[citation:');
}

function asContentString(v, stripReferenceMarkers = true) {
  if (typeof v === 'string') {
    return stripReferenceMarkers ? stripReferenceMarkersText(v) : v;
  }
  if (Array.isArray(v)) {
    let out = '';
    for (const item of v) {
      out += asContentString(item, stripReferenceMarkers);
    }
    return out;
  }
  if (v && typeof v === 'object') {
    if (Object.prototype.hasOwnProperty.call(v, 'content')) {
      return asContentString(v.content, stripReferenceMarkers);
    }
    if (Object.prototype.hasOwnProperty.call(v, 'v')) {
      return asContentString(v.v, stripReferenceMarkers);
    }
    if (Object.prototype.hasOwnProperty.call(v, 'text')) {
      return asContentString(v.text, stripReferenceMarkers);
    }
    if (Object.prototype.hasOwnProperty.call(v, 'value')) {
      return asContentString(v.value, stripReferenceMarkers);
    }
    return '';
  }
  if (v == null) {
    return '';
  }
  const text = String(v);
  return stripReferenceMarkers ? stripReferenceMarkersText(text) : text;
}

function stripReferenceMarkersText(text) {
  if (!text) {
    return text;
  }
  return text
    .replace(/\[(?:citation|reference):\s*\d+\]/gi, '')
    .replace(LEAKED_BOS_MARKER_PATTERN, '')
    .replace(LEAKED_THOUGHT_MARKER_PATTERN, '')
    .replace(LEAKED_META_MARKER_PATTERN, '');
}

function asString(v) {
  if (typeof v === 'string') {
    return v.trim();
  }
  if (Array.isArray(v)) {
    return asString(v[0]);
  }
  if (v == null) {
    return '';
  }
  return String(v).trim();
}

module.exports = {
  parseChunkForContent,
  extractContentRecursive,
  filterLeakedContentFilterParts,
  hasContentFilterStatus,
  extractAccumulatedTokenUsage,
  shouldSkipPath,
  isFragmentStatusPath,
  isCitation,
  stripReferenceMarkers: stripReferenceMarkersText,
  stripThinkTags,
};
