'use strict';

const DEFAULT_CORS_ALLOW_HEADERS = [
  'Content-Type',
  'Authorization',
  'X-API-Key',
  'X-Ds2-Target-Account',
  'X-Ds2-Source',
  'X-Vercel-Protection-Bypass',
  'X-Goog-Api-Key',
  'Anthropic-Version',
  'Anthropic-Beta',
];

const BLOCKED_CORS_REQUEST_HEADERS = new Set([
  'x-ds2-internal-token',
]);

function setCorsHeaders(res, req) {
  const origin = asString(readHeader(req, 'origin'));
  res.setHeader('Access-Control-Allow-Origin', origin || '*');
  if (origin) {
    addVaryHeader(res, 'Origin');
  }
  res.setHeader('Access-Control-Allow-Methods', 'GET, POST, OPTIONS, PUT, DELETE');
  res.setHeader('Access-Control-Max-Age', '600');
  res.setHeader(
    'Access-Control-Allow-Headers',
    buildCORSAllowHeaders(req),
  );
  addVaryHeader(res, 'Access-Control-Request-Headers');
  if (asString(readHeader(req, 'access-control-request-private-network')).toLowerCase() === 'true') {
    res.setHeader('Access-Control-Allow-Private-Network', 'true');
    addVaryHeader(res, 'Access-Control-Request-Private-Network');
  }
}

function buildCORSAllowHeaders(req) {
  const seen = new Set();
  const headers = [];
  for (const name of DEFAULT_CORS_ALLOW_HEADERS) {
    appendCORSHeaderName(headers, seen, name);
  }
  for (const name of splitCORSRequestHeaders(readHeader(req, 'access-control-request-headers'))) {
    appendCORSHeaderName(headers, seen, name);
  }
  return headers.join(', ');
}

function splitCORSRequestHeaders(raw) {
  const text = asString(raw);
  if (!text) {
    return [];
  }
  return text
    .split(',')
    .map((part) => asString(part))
    .filter((name) => isValidCORSHeaderToken(name))
    .filter((name) => !BLOCKED_CORS_REQUEST_HEADERS.has(name.toLowerCase()));
}

function appendCORSHeaderName(headers, seen, name) {
  const text = asString(name);
  if (!isValidCORSHeaderToken(text)) {
    return;
  }
  const lower = text.toLowerCase();
  if (BLOCKED_CORS_REQUEST_HEADERS.has(lower) || seen.has(lower)) {
    return;
  }
  seen.add(lower);
  headers.push(text);
}

function isValidCORSHeaderToken(name) {
  return /^[A-Za-z0-9!#$%&'*+.^_`|~-]+$/.test(asString(name));
}

function addVaryHeader(res, token) {
  const text = asString(token);
  if (!text || typeof res.setHeader !== 'function') {
    return;
  }
  const current = typeof res.getHeader === 'function' ? res.getHeader('Vary') : '';
  const seen = new Set();
  const merged = [];
  const addToken = (value) => {
    const trimmed = asString(value);
    if (!trimmed) {
      return;
    }
    const lower = trimmed.toLowerCase();
    if (seen.has(lower)) {
      return;
    }
    seen.add(lower);
    merged.push(trimmed);
  };
  if (Array.isArray(current)) {
    for (const value of current) {
      for (const part of String(value).split(',')) {
        addToken(part);
      }
    }
  } else {
    for (const part of String(current || '').split(',')) {
      addToken(part);
    }
  }
  addToken(text);
  res.setHeader('Vary', merged.join(', '));
}

function readHeader(req, key) {
  if (!req || !req.headers) {
    return '';
  }
  return req.headers[String(key).toLowerCase()];
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
  setCorsHeaders,
};
