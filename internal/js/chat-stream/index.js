'use strict';

const {
  writeOpenAIError,
} = require('./error_shape');
const {
  parseChunkForContent,
  extractContentRecursive,
  filterLeakedContentFilterParts,
  hasContentFilterStatus,
  extractAccumulatedTokenUsage,
  shouldSkipPath,
  stripReferenceMarkers,
} = require('./sse_parse');
const {
  resolveToolcallPolicy,
  formatIncrementalToolCallDeltas,
  normalizePreparedToolNames,
  boolDefaultTrue,
  filterIncrementalToolCallDeltasByAllowed,
  resetStreamToolCallState,
} = require('./toolcall_policy');
const {
  estimateTokens,
  buildUsage,
} = require('./token_usage');
const {
  setCorsHeaders,
  readRawBody,
  asString,
} = require('./http_internal');
const {
  proxyToGo,
} = require('./proxy_go');
const {
  handleVercelStream,
} = require('./vercel_stream');
const {
  trimContinuationOverlap,
} = require('./dedupe');

async function handler(req, res) {
  setCorsHeaders(res, req);
  if (req.method === 'OPTIONS') {
    res.statusCode = 204;
    res.end();
    return;
  }
  if (req.method !== 'POST') {
    writeOpenAIError(res, 405, 'method not allowed');
    return;
  }

  const rawBody = await readRawBody(req);

  // Hard guard: only use Node data path for streaming on Vercel runtime.
  // Any non-Vercel runtime always falls back to Go for full behavior parity.
  if (!isVercelRuntime()) {
    await proxyToGo(req, res, rawBody);
    return;
  }

  let payload;
  try {
    payload = JSON.parse(rawBody.toString('utf8') || '{}');
  } catch (_err) {
    writeOpenAIError(res, 400, 'invalid json');
    return;
  }

  // Keep all non-stream behavior and non-OpenAI-chat paths on Go side to avoid
  // protocol-shape regressions (e.g. Gemini/Claude clients expecting their own formats).
  if (!toBool(payload.stream) || !isNodeStreamSupportedPath(req.url || '')) {
    await proxyToGo(req, res, rawBody);
    return;
  }

  await handleVercelStream(req, res, rawBody, payload);
}

function toBool(v) {
  return v === true;
}

function isVercelRuntime() {
  return asString(process.env.VERCEL) !== '' || asString(process.env.NOW_REGION) !== '';
}

function isNodeStreamSupportedPath(rawURL) {
  const path = extractPathname(rawURL);
  return path === '/v1/chat/completions' || path === '/chat/completions';
}

function extractPathname(rawURL) {
  const text = asString(rawURL);
  if (!text) {
    return '';
  }
  const q = text.indexOf('?');
  if (q >= 0) {
    return text.slice(0, q);
  }
  return text;
}

module.exports = handler;

module.exports.__test = {
  parseChunkForContent,
  extractContentRecursive,
  shouldSkipPath,
  stripReferenceMarkers,
  asString,
  resolveToolcallPolicy,
  formatIncrementalToolCallDeltas,
  normalizePreparedToolNames,
  boolDefaultTrue,
  filterIncrementalToolCallDeltasByAllowed,
  resetStreamToolCallState,
  estimateTokens,
  buildUsage,
  filterLeakedContentFilterParts,
  hasContentFilterStatus,
  extractAccumulatedTokenUsage,
  isNodeStreamSupportedPath,
  extractPathname,
  trimContinuationOverlap,
};
