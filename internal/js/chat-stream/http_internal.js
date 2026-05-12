'use strict';

const {
  writeOpenAIError,
} = require('./error_shape');
const {
  setCorsHeaders,
} = require('./cors');

function header(req, key) {
  if (!req || !req.headers) {
    return '';
  }
  return asString(req.headers[key.toLowerCase()]);
}

async function readRawBody(req) {
  if (Buffer.isBuffer(req.body)) {
    return req.body;
  }
  if (typeof req.body === 'string') {
    return Buffer.from(req.body);
  }
  if (req.body && typeof req.body === 'object') {
    return Buffer.from(JSON.stringify(req.body));
  }
  const chunks = [];
  for await (const chunk of req) {
    chunks.push(Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk));
  }
  return Buffer.concat(chunks);
}

async function fetchStreamPrepare(req, rawBody) {
  const url = buildInternalGoURL(req);
  url.searchParams.set('__stream_prepare', '1');

  const upstream = await fetch(url.toString(), {
    method: 'POST',
    headers: buildInternalGoHeaders(req, { withInternalToken: true, withContentType: true }),
    body: rawBody,
  });

  const text = await upstream.text();
  let body = {};
  try {
    body = JSON.parse(text || '{}');
  } catch (_err) {
    body = {};
  }

  return {
    ok: upstream.ok,
    status: upstream.status,
    contentType: upstream.headers.get('content-type') || 'application/json',
    text,
    body,
  };
}

async function fetchStreamPow(req, leaseID) {
  const url = buildInternalGoURL(req);
  url.searchParams.set('__stream_pow', '1');

  const upstream = await fetch(url.toString(), {
    method: 'POST',
    headers: buildInternalGoHeaders(req, { withInternalToken: true, withContentType: true }),
    body: Buffer.from(JSON.stringify({ lease_id: leaseID })),
  });

  const text = await upstream.text();
  let body = {};
  try {
    body = JSON.parse(text || '{}');
  } catch (_err) {
    body = {};
  }

  return {
    ok: upstream.ok,
    status: upstream.status,
    contentType: upstream.headers.get('content-type') || 'application/json',
    text,
    body,
  };
}

async function fetchStreamSwitch(req, leaseID) {
  const url = buildInternalGoURL(req);
  url.searchParams.set('__stream_switch', '1');

  const upstream = await fetch(url.toString(), {
    method: 'POST',
    headers: buildInternalGoHeaders(req, { withInternalToken: true, withContentType: true }),
    body: Buffer.from(JSON.stringify({ lease_id: leaseID })),
  });

  const text = await upstream.text();
  let body = {};
  try {
    body = JSON.parse(text || '{}');
  } catch (_err) {
    body = {};
  }

  return {
    ok: upstream.ok,
    status: upstream.status,
    contentType: upstream.headers.get('content-type') || 'application/json',
    text,
    body,
  };
}

function relayPreparedFailure(res, prep) {
  if (prep.status === 401 && looksLikeVercelAuthPage(prep.text)) {
    writeOpenAIError(
      res,
      401,
      'Vercel Deployment Protection blocked internal prepare request. Disable protection for this deployment or set VERCEL_AUTOMATION_BYPASS_SECRET.',
    );
    return;
  }
  res.statusCode = prep.status || 500;
  res.setHeader('Content-Type', prep.contentType || 'application/json');
  if (prep.text) {
    res.end(prep.text);
    return;
  }
  writeOpenAIError(res, prep.status || 500, 'vercel prepare failed');
}

async function safeReadText(resp) {
  if (!resp) {
    return '';
  }
  try {
    const text = await resp.text();
    return text.trim();
  } catch (_err) {
    return '';
  }
}

function internalSecret() {
  return asString(process.env.DS2API_VERCEL_INTERNAL_SECRET) || asString(process.env.DS2API_ADMIN_KEY) || 'admin';
}

function buildInternalGoURL(req) {
  const proto = asString(header(req, 'x-forwarded-proto')) || 'https';
  const host = asString(header(req, 'host'));
  const url = new URL(`${proto}://${host}${req.url || '/v1/chat/completions'}`);
  url.searchParams.set('__go', '1');
  const protectionBypass = resolveProtectionBypass(req);
  if (protectionBypass) {
    url.searchParams.set('x-vercel-protection-bypass', protectionBypass);
  }
  return url;
}

function buildInternalGoHeaders(req, opts = {}) {
  const headers = {
    authorization: asString(header(req, 'authorization')),
    'x-api-key': asString(header(req, 'x-api-key')),
    'x-ds2-target-account': asString(header(req, 'x-ds2-target-account')),
    'x-vercel-protection-bypass': resolveProtectionBypass(req),
  };
  if (opts.withInternalToken) {
    headers['x-ds2-internal-token'] = internalSecret();
  }
  if (opts.withContentType) {
    headers['content-type'] = asString(header(req, 'content-type')) || 'application/json';
  }
  return headers;
}

function createLeaseReleaser(req, leaseID) {
  let released = false;
  return async () => {
    if (released || !leaseID) {
      return;
    }
    released = true;
    try {
      await releaseStreamLease(req, leaseID);
    } catch (_err) {
      // Ignore release errors. Lease TTL cleanup on Go side still prevents permanent leaks.
    }
  };
}

async function releaseStreamLease(req, leaseID) {
  const url = buildInternalGoURL(req);
  url.searchParams.set('__stream_release', '1');
  const body = Buffer.from(JSON.stringify({ lease_id: leaseID }));

  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), 1500);
  try {
    await fetch(url.toString(), {
      method: 'POST',
      headers: buildInternalGoHeaders(req, { withInternalToken: true, withContentType: true }),
      body,
      signal: controller.signal,
    });
  } finally {
    clearTimeout(timeout);
  }
}

function resolveProtectionBypass(req) {
  const fromHeader = asString(header(req, 'x-vercel-protection-bypass'));
  if (fromHeader) {
    return fromHeader;
  }
  return asString(process.env.VERCEL_AUTOMATION_BYPASS_SECRET) || asString(process.env.DS2API_VERCEL_PROTECTION_BYPASS);
}

function looksLikeVercelAuthPage(text) {
  const body = asString(text).toLowerCase();
  if (!body) {
    return false;
  }
  return body.includes('authentication required') && body.includes('vercel');
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

function isAbortError(err) {
  if (!err || typeof err !== 'object') {
    return false;
  }
  return err.name === 'AbortError' || err.code === 'ABORT_ERR';
}

module.exports = {
  setCorsHeaders,
  header,
  readRawBody,
  fetchStreamPrepare,
  fetchStreamPow,
  fetchStreamSwitch,
  relayPreparedFailure,
  safeReadText,
  buildInternalGoURL,
  buildInternalGoHeaders,
  createLeaseReleaser,
  releaseStreamLease,
  resolveProtectionBypass,
  looksLikeVercelAuthPage,
  asString,
  isAbortError,
};
