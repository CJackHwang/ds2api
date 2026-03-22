'use strict';

const crypto = require('crypto');

const {
  extractToolNames,
} = require('../helpers/stream-tool-sieve');

function resolveToolcallPolicy(prepBody, payloadTools) {
  const preparedToolNames = normalizePreparedToolNames(prepBody && prepBody.tool_names);
  let toolNames = preparedToolNames.length > 0 ? preparedToolNames : extractToolNames(payloadTools);
  if (toolNames.length === 0 && Array.isArray(payloadTools) && payloadTools.length > 0) {
    toolNames = ['__any_tool__'];
  }
  const featureMatchEnabled = boolDefaultTrue(prepBody && prepBody.toolcall_feature_match);
  const emitEarlyToolDeltas = featureMatchEnabled && boolDefaultTrue(prepBody && prepBody.toolcall_early_emit_high);
  return {
    toolNames,
    toolSieveEnabled: toolNames.length > 0,
    emitEarlyToolDeltas,
  };
}

function normalizePreparedToolNames(v) {
  if (!Array.isArray(v) || v.length === 0) {
    return [];
  }
  const out = [];
  for (const item of v) {
    const name = asString(item);
    if (!name) {
      continue;
    }
    out.push(name);
  }
  return out;
}

function boolDefaultTrue(v) {
  return v !== false;
}

function formatIncrementalToolCallDeltas(deltas, idStore) {
  if (!Array.isArray(deltas) || deltas.length === 0) {
    return [];
  }
  const out = [];
  for (const d of deltas) {
    if (!d || typeof d !== 'object') {
      continue;
    }
    const index = Number.isInteger(d.index) ? d.index : 0;
    const id = ensureStreamToolCallID(idStore, index);
    const item = {
      index,
      id,
      type: 'function',
    };
    const fn = {};
    if (asString(d.name)) {
      fn.name = asString(d.name);
    }
    if (typeof d.arguments === 'string' && d.arguments !== '') {
      fn.arguments = d.arguments;
    }
    if (Object.keys(fn).length === 0) {
      continue;
    }
    if (Object.keys(fn).length > 0) {
      item.function = fn;
    }
    out.push(item);
  }
  return out;
}

function filterIncrementalToolCallDeltasByAllowed(deltas, allowedNames, seenNames) {
  if (!Array.isArray(deltas) || deltas.length === 0) {
    return [];
  }
  const seen = seenNames instanceof Map ? seenNames : new Map();
  const out = [];
  for (const d of deltas) {
    if (!d || typeof d !== 'object') {
      continue;
    }
    const index = Number.isInteger(d.index) ? d.index : 0;
    const name = asString(d.name);
    if (name) {
      seen.set(index, name);
      out.push(d);
      continue;
    }
    const existing = asString(seen.get(index));
    if (!existing) {
      continue;
    }
    out.push(d);
  }
  return out;
}

function ensureStreamToolCallID(idStore, index) {
  const key = Number.isInteger(index) ? index : 0;
  const existing = idStore.get(key);
  if (existing) {
    return existing;
  }
  const next = `call_${newCallID()}`;
  idStore.set(key, next);
  return next;
}

function newCallID() {
  if (typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID().replace(/-/g, '');
  }
  return `${Date.now()}${Math.floor(Math.random() * 1e9)}`;
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
  resolveToolcallPolicy,
  normalizePreparedToolNames,
  boolDefaultTrue,
  formatIncrementalToolCallDeltas,
  filterIncrementalToolCallDeltasByAllowed,
};
