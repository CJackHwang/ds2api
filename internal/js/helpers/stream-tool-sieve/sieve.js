'use strict';
const {
  resetIncrementalToolState,
  noteText,
  insideCodeFenceWithState,
} = require('./state');
const { trimWrappingJSONFence } = require('./jsonscan');
const {
  XML_TOOL_SEGMENT_TAGS,
} = require('./tool-keywords');
const {
  consumeXMLToolCapture: consumeXMLToolCaptureImpl,
  hasOpenXMLToolTag,
  findPartialXMLToolTagStart,
} = require('./sieve-xml');
function processToolSieveChunk(state, chunk, toolNames) {
  if (!state) {
    return [];
  }
  if (chunk) {
    state.pending += chunk;
  }
  const events = [];
  while (true) {
    if (Array.isArray(state.pendingToolCalls) && state.pendingToolCalls.length > 0) {
      events.push({ type: 'tool_calls', calls: state.pendingToolCalls });
      state.pendingToolRaw = '';
      state.pendingToolCalls = [];
      continue;
    }
    if (state.capturing) {
      if (state.pending) {
        state.capture += state.pending;
        state.pending = '';
      }
      const consumed = consumeToolCapture(state, toolNames);
      if (!consumed.ready) {
        break;
      }
      const captured = state.capture;
      state.capture = '';
      state.capturing = false;
      resetIncrementalToolState(state);

      if (Array.isArray(consumed.calls) && consumed.calls.length > 0) {
        state.pendingToolRaw = captured;
        state.pendingToolCalls = consumed.calls;
        if (consumed.suffix) {
          state.pending = consumed.suffix + state.pending;
        }
        continue;
      }
      if (consumed.prefix) {
        noteText(state, consumed.prefix);
        events.push({ type: 'text', text: consumed.prefix });
      }
      if (consumed.suffix) {
        state.pending += consumed.suffix;
      }
      continue;
    }
    const pending = state.pending || '';
    if (!pending) {
      break;
    }
    const start = findToolSegmentStart(state, pending);
    if (start >= 0) {
      const prefix = pending.slice(0, start);
      if (prefix) {
        noteText(state, prefix);
        events.push({ type: 'text', text: prefix });
      }
      state.pending = '';
      state.capture += pending.slice(start);
      state.capturing = true;
      resetIncrementalToolState(state);
      continue;
    }
    const [safe, hold] = splitSafeContentForToolDetection(state, pending);
    if (!safe) {
      break;
    }
    state.pending = hold;
    noteText(state, safe);
    events.push({ type: 'text', text: safe });
  }
  return events;
}

function flushToolSieve(state, toolNames) {
  if (!state) {
    return [];
  }
  const events = processToolSieveChunk(state, '', toolNames);
  if (Array.isArray(state.pendingToolCalls) && state.pendingToolCalls.length > 0) {
    events.push({ type: 'tool_calls', calls: state.pendingToolCalls });
    state.pendingToolRaw = '';
    state.pendingToolCalls = [];
  }
  if (state.capturing) {
    const consumed = consumeToolCapture(state, toolNames);
    if (consumed.ready) {
      if (consumed.prefix) {
        noteText(state, consumed.prefix);
        events.push({ type: 'text', text: consumed.prefix });
      }
      if (Array.isArray(consumed.calls) && consumed.calls.length > 0) {
        events.push({ type: 'tool_calls', calls: consumed.calls });
      }
      if (consumed.suffix) {
        noteText(state, consumed.suffix);
        events.push({ type: 'text', text: consumed.suffix });
      }
    } else if (state.capture) {
      const content = state.capture;
      noteText(state, content);
      events.push({ type: 'text', text: content });
    }
    state.capture = '';
    state.capturing = false;
    resetIncrementalToolState(state);
  }
  if (state.pending) {
    noteText(state, state.pending);
    events.push({ type: 'text', text: state.pending });
    state.pending = '';
  }
  return events;
}

function splitSafeContentForToolDetection(state, s) {
  const text = s || '';
  if (!text) {
    return ['', ''];
  }
  // Only hold back partial XML tool tags.
  const xmlIdx = findPartialXMLToolTagStart(text);
  if (xmlIdx >= 0) {
    if (insideCodeFenceWithState(state, text.slice(0, xmlIdx))) {
      return [text, ''];
    }
    if (xmlIdx > 0) {
      return [text.slice(0, xmlIdx), text.slice(xmlIdx)];
    }
    return ['', text];
  }
  return [text, ''];
}

function findToolSegmentStart(state, s) {
  if (!s) {
    return -1;
  }
  const lower = s.toLowerCase();
  let offset = 0;
  while (true) {
    // Only check XML tool tags.
    let bestIdx = -1;
    let matchedTag = '';
    for (const tag of XML_TOOL_SEGMENT_TAGS) {
      const idx = lower.indexOf(tag, offset);
      if (idx >= 0 && (bestIdx < 0 || idx < bestIdx)) {
        bestIdx = idx;
        matchedTag = tag;
      }
    }
    if (bestIdx < 0) {
      return -1;
    }
    if (!insideCodeFenceWithState(state, s.slice(0, bestIdx))) {
      return bestIdx;
    }
    offset = bestIdx + matchedTag.length;
  }
}

function consumeToolCapture(state, toolNames) {
  const captured = state.capture || '';
  if (!captured) {
    return { ready: false, prefix: '', calls: [], suffix: '' };
  }

  // XML-only tool call extraction.
  const xmlResult = consumeXMLToolCaptureImpl(captured, toolNames, trimWrappingJSONFence);
  if (xmlResult.ready) {
    return xmlResult;
  }
  // If XML tags are present but block is incomplete, keep buffering.
  if (hasOpenXMLToolTag(captured)) {
    return { ready: false, prefix: '', calls: [], suffix: '' };
  }

  // No XML tool tags detected — release captured content as text.
  return {
    ready: true,
    prefix: captured,
    calls: [],
    suffix: '',
  };
}

module.exports = {
  processToolSieveChunk,
  flushToolSieve,
};
