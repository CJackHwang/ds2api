# DeepSeek Web Client Request Fingerprint

> Captured from official DeepSeek web client (chat.deepseek.com) on 2026-05-13.

## 1. Headers (all endpoints)

All three endpoints share identical headers. Only `referer` varies per session.

### Static Headers (same across all requests)

```
:authority: chat.deepseek.com
:scheme: https
accept: */*
accept-encoding: gzip, deflate, br, zstd
accept-language: zh-CN,zh;q=0.9
cache-control: no-cache
content-type: application/json
origin: https://chat.deepseek.com
pragma: no-cache
priority: u=1, i
sec-ch-ua: "Google Chrome";v="135", "Not-A.Brand";v="8", "Chromium";v="135"
sec-ch-ua-mobile: ?0
sec-ch-ua-platform: "Windows"
sec-fetch-dest: empty
sec-fetch-mode: cors
sec-fetch-site: same-origin
user-agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/135.0.0.0 Safari/537.36
x-app-version: 2.0.0
x-client-locale: zh_CN
x-client-platform: web
x-client-timezone-offset: 28800
x-client-version: 2.0.0
```

### Dynamic Headers (per-request)

```
authorization: Bearer <token>
x-ds-pow-response: <base64-encoded PoW>  (completion only)
```

### Per-Session Headers

```
referer: https://chat.deepseek.com/a/chat/s/<chat_session_id>
```

### Optional Headers (not always present)

```
x-hif-dliq: <static fingerprint value>  (only present in some sessions)
x-hif-leim: <static fingerprint value>  (only present in some sessions)
```

These were present in one logged-in session but absent in a new incognito session. Not required for requests to succeed.

---

## 2. Cookies

All cookies are sent on every request. Values differ per browser instance.

| Cookie | Domain | Lifetime | Source |
|--------|--------|----------|--------|
| `smidV2` | chat.deepseek.com | ~1 year | JavaScript-generated |
| `ds_cookie_preference` | .deepseek.com | ~1 year | User preference |
| `aws-waf-token` | .chat.deepseek.com | ~4 days | AWS WAF JS |
| `ds_session_id` | chat.deepseek.com | Session | Server-set |
| `.thumbcache_<hash>` | chat.deepseek.com | ~1 year | localStorage |

Note: In incognito window, cookies are simpler (fewer values). The minimum required cookies appear to be `ds_session_id` and `aws-waf-token`.

---

## 3. TLS Fingerprint

- **Current DS2API**: `HelloAndroid_11_OkHttp` (Android app)
- **Official Web Client**: Chrome 135 on Windows
- **HTTP Version**: HTTP/1.1 (ForceAttemptHTTP2: false)

---

## 4. Endpoint Details

### 4.1 POST /api/v0/chat_session/create

**Request Body:**
```json
{}
```
(Empty object)

**Request Headers:** Standard headers (no `x-ds-pow-response`)

**Response:**
```json
{
  "code": 0,
  "msg": "",
  "data": {
    "biz_code": 0,
    "biz_msg": "",
    "biz_data": {
      "chat_session": {
        "id": "463764d5-...",
        "seq_id": 200820062,
        "agent": "chat",
        "model_type": "default",
        "title": null,
        "title_type": "WIP",
        "version": 0,
        "current_message_id": null,
        "pinned": false,
        "inserted_at": 1778656862.252,
        "updated_at": 1778656862.252
      },
      "ttl_seconds": 259200
    }
  }
}
```

### 4.2 POST /api/v0/chat/create_pow_challenge

**Request Body:**
```json
{
  "target_path": "/api/v0/chat/completion"
}
```

**Request Headers:** Standard headers (no `x-ds-pow-response`)

**Response:**
```json
{
  "code": 0,
  "msg": "",
  "data": {
    "biz_code": 0,
    "biz_msg": "",
    "biz_data": {
      "challenge": {
        "algorithm": "DeepSeekHashV1",
        "challenge": "3e96dda5b3c83f14c387288108b21262b...",
        "salt": "0a8b3a624b1217b2...",
        "signature": "82191fd611b9d0dc861d262983f7cdf5...",
        "difficulty": 144000,
        "expire_at": 1778657378900,
        "expire_after": 300000,
        "target_path": "/api/v0/chat/completion"
      }
    }
  }
}
```

Note: `difficulty` field is new (not in current DS2API code which only reads challenge/salt/signature).

### 4.3 POST /api/v0/chat/completion

**Request Body (first message):**
```json
{
  "chat_session_id": "d6047d91-...",
  "parent_message_id": null,
  "model_type": "expert",
  "prompt": "你好",
  "ref_file_ids": [],
  "thinking_enabled": false,
  "search_enabled": true,
  "preempt": false
}
```

**Request Body (continuation message):**
```json
{
  "chat_session_id": "d6047d91-...",
  "parent_message_id": 2,
  "model_type": null,
  "prompt": "你是谁？",
  "ref_file_ids": [],
  "thinking_enabled": false,
  "search_enabled": true,
  "preempt": false
}
```

**Request Headers:** Standard headers + `x-ds-pow-response`

**Response (SSE stream):**
```
event: ready
data: {"request_message_id":3,"response_message_id":4,"model_type":"expert"}

event: update_session
data: {"updated_at":1778656974.416658}

data: {"v":{"response":{"message_id":4,"parent_id":3,...}}}

data: {"p":"response/fragments/-1/content","o":"APPEND","v":"Deep"}

data: {"v":"Seek"}

data: {"v":"，"}

...

data: {"p":"response","o":"BATCH","v":[{"p":"accumulated_token_usage","v":258},{"p":"quasi_status","v":"FINISHED"}]}

data: {"p":"response/status","o":"SET","v":"FINISHED"}

event: update_session
data: {"updated_at":1778656980.5914998}

event: close
data: {"click_behavior":"none","auto_resume":false}
```

---

## 5. Key Differences from Current DS2API

| Field | Current DS2API | Official Web Client |
|-------|---------------|-------------------|
| Platform | `android` | `web` |
| User-Agent | `DeepSeek/2.0.x Android/xx` | Chrome 135 |
| TLS fingerprint | `HelloAndroid_11_OkHttp` | Chrome |
| `x-client-version` | `2.0.{1-9}` (randomized) | `2.0.0` |
| `x-app-version` | missing | `2.0.0` |
| `x-client-timezone-offset` | missing | `28800` |
| `origin` | missing | `https://chat.deepseek.com` |
| `referer` | missing | dynamic per session |
| `sec-ch-ua` | missing | Chrome 135 |
| `sec-fetch-*` | missing | `empty/cors/same-origin` |
| `cookie` | missing | managed by browser |
| Request body `preempt` | missing | `false` |
| PoW difficulty | not parsed | `difficulty` field |

---

## 6. Anti-Detection Summary

### Current DS2API Anti-Detection Measures
1. ✅ TLS fingerprint via uTLS (but wrong platform: Android instead of Chrome)
2. ✅ Version number randomization (but not needed for web client)
3. ✅ Random delay before requests (50-200ms)
4. ✅ HTTP/1.1 forced
5. ✅ PoW challenge solving
6. ❌ Missing web platform headers
7. ❌ Missing cookies
8. ❌ Wrong TLS fingerprint (Android vs Chrome)

### Recommended Changes
1. Switch TLS fingerprint from Android to Chrome
2. Add CookieJar for automatic cookie management
3. Add all missing web platform headers
4. Add `preempt: false` to completion payload
5. Make cookies configurable in config.json (for JavaScript-generated ones)
