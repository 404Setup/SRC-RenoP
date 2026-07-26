---
title: Rate Limit
order: 9
category: API
---

# Rate limiting and anomaly protection

Global middleware (`AnomalyMiddleware`) runs on every request before route handlers. Implementation:
`middleware/anomaly.go`.

Three independent controls apply in order:

1. **Concurrent request cap** — process-wide
2. **Auth-failure ban** — per client IP
3. **Anonymous request rate limit** — per client IP (token bucket)

When a limit triggers, the response may set `Connection: close`.

## Client IP

Limits key off the client IP from `utils.ExtractIP`:

- Default: peer address (`c.IP()`).
- If the peer is loopback (`127.0.0.1` / `::1`, typical for local Caddy/nginx) or listed in
  `server.trusted_proxies`, **and** `server.cdn_ip_header` is set, the client IP is taken from that header (multi-value
  headers are walked right-to-left, skipping trusted hops).

Misconfigured proxy trust can map many users to one IP (or the reverse). See server config
in [settings.md](./settings.md).

For Cloudflare → Caddy → RenoP, set `cdn_ip_header` to `CF-Connecting-IP`; loopback peers do not need an entry in
`trusted_proxies`.

## 1. Concurrent request cap

| Setting                      | Default | Effect                                    |
|------------------------------|---------|-------------------------------------------|
| `server.max_active_requests` | `2000`  | Max in-flight requests across the process |

- Counted for **all** requests that enter the middleware (including authenticated and static).
- When the live count would exceed the cap → **`503 Service Unavailable`**.
- `0` in config is normalized to the default (`2000`) at load time; there is no “unlimited” mode via this field.

Fiber’s own concurrency limit is aligned with this value at server start.

## 2. Auth-failure ban (per IP)

| Constant               | Value | Meaning                                |
|------------------------|-------|----------------------------------------|
| `MaxFailuresPerMinute` | `5`   | Failure count threshold before ban     |
| Failure store TTL      | 5 min | `AnomalyFailures` BigCache life window |

**What counts as a failure**

After the handler runs, if the final status is **`401`** or **`403`**, the per-IP failure counter is incremented (8-byte
little-endian counter in cache).

**Ban behavior**

- If the counter is already **≥ 5** at the start of a request → **`403 Forbidden`** immediately (handler not run).
- The ban response itself is returned *before* `Next()`, so it does **not** increment the counter again.
- Cache entry TTL is **5 minutes** from the last write (each recorded failure refreshes the entry). After expiry the
  counter is gone and the IP can try again.

**Scope**

Applies to **all** paths that pass the static-frontend skip (see below), including authenticated clients. A client that
keeps getting 401/403 from handlers will hit this ban regardless of credentials on later requests.

Note: the constant name says “per minute”, but the store window is **5 minutes** and the counter only increases until
the entry expires.

## 3. Anonymous request rate limit (per IP)

Token bucket via `golang.org/x/time/rate`, one limiter per IP (`GlobalIPLimiter`).

| Constant               | Value | Meaning                                               |
|------------------------|-------|-------------------------------------------------------|
| `MaxRequestsPerMinute` | `100` | Sustained rate: 100 tokens / minute (~1 every 600 ms) |
| `MaxRequestsBurst`     | `60`  | Burst capacity (max tokens held at once)              |

- **Who is limited:** requests that are **not** verified as authenticated (see below).
- **Who is exempt:** successfully verified Session / Bearer / Basic (or GET/HEAD `?token=` that resolves to a valid
  session or bearer).
- **On exceed:** **`429 Too Many Requests`**.

### What counts as “verified authenticated”

Same idea as production auth carriers (see [authentication.md](./authentication.md)):

| Carrier                                           | Verified when                                  |
|---------------------------------------------------|------------------------------------------------|
| Cookie `renop_session` / `Authorization: Session` | Session exists and is within idle timeout      |
| `Authorization: Bearer <user>:<secret>`           | Username + secret match a live account         |
| `Authorization: Bearer <upload-token>`            | Token is indexed and not expired               |
| `Authorization: Basic …`                          | Username + password/secret match               |
| GET/HEAD `?token=`                                | Token is a valid session id or bearer as above |

Present but **invalid** credentials do **not** exempt the request; they still consume rate-limit tokens and may also
raise the failure counter if the handler returns 401/403.

Idle sessions are not treated as authenticated (idle timeout ≈ 7 days; see authentication docs).

### Limiter lifecycle

| Behavior          | Detail                                                     |
|-------------------|------------------------------------------------------------|
| Key               | Client IP string                                           |
| Idle eviction     | Entry unused for **5 minutes** is removed                  |
| Periodic cleanup  | Every **5 minutes**                                        |
| Emergency cleanup | When stored IPs exceed **10,000**, a cleanup runs promptly |

## Static frontend skip

For **GET** and **HEAD** only, these paths skip the auth-failure ban and the anonymous rate limit (they still count
toward the concurrent cap):

| Path pattern  |
|---------------|
| `/`           |
| `/index.html` |
| `/assets/*`   |
| `/js/*`       |
| `/css/*`      |
| `/svg/*`      |

API routes, Maven repository paths, and other methods are **not** skipped.

## Status codes (this middleware)

| Code | When                                              |
|------|---------------------------------------------------|
| 429  | Anonymous IP exceeded the token-bucket rate limit |
| 403  | IP banned after too many 401/403 responses        |
| 503  | Process concurrent request cap exceeded           |

Bodies are typically empty; trust the status code. There is no `Retry-After` header today.

## Client guidance

1. Prefer authenticated access (session cookie, Basic, or Bearer) for high-volume or scripted traffic so the per-IP
   anonymous bucket does not apply.
2. Avoid tight login / probe loops: **5** handler-level **401/403** results from one IP can ban that IP for up to **~5
   minutes**.
3. Behind a reverse proxy or CDN, set `trusted_proxies` and `cdn_ip_header` correctly so rate limits apply per real
   client, not per proxy.
4. On **429**, back off (e.g. exponential) before retrying unauthenticated calls.
5. On **503** from this layer, the instance is saturated; retry later or scale / raise `max_active_requests` if
   appropriate.
