---
title: Rate Limit
order: 9
category: API
---

# Rate limits

Global middleware `AnomalyMiddleware` (`middleware/anomaly.go`) runs before route handlers.

Controls, in order:

1. Concurrent request cap (process-wide)
2. Auth-failure ban (per client IP)
3. Anonymous request rate limit (per client IP, token bucket)

When a limit is applied, the response may set `Connection: close`.

## Client IP

Limits are keyed by the client IP from `utils.ExtractIP`:

- Default: peer address (`c.IP()`).
- If the peer is loopback (`127.0.0.1` / `::1`) or listed in `server.trusted_proxies`, and `server.cdn_ip_header` is
  set, the client IP is taken from that header. Multi-value headers are scanned from right to left; trusted hops are
  skipped.

Incorrect proxy trust configuration can map many clients to one IP, or the reverse. See [settings.md](./settings.md).

For Cloudflare → Caddy → RenoP, set `cdn_ip_header` to `CF-Connecting-IP`. Loopback peers do not require an entry in
`trusted_proxies`.

## 1. Concurrent request cap

| Setting                      | Default | Effect                     |
|------------------------------|---------|----------------------------|
| `server.max_active_requests` | `2000`  | Maximum in-flight requests |

- Every request that enters the middleware is counted, including authenticated and static requests.
- If the live count would exceed the cap, the middleware returns **`503 Service Unavailable`**.
- A configured value of `0` is normalized to `2000` at load time. This field has no unlimited mode.

Fiber’s own concurrency limit is set to the same value when the server starts.

## 2. Auth-failure ban (per IP)

| Constant               | Value | Meaning                                     |
|------------------------|-------|---------------------------------------------|
| `MaxFailuresPerMinute` | `5`   | Failure count threshold before ban          |
| Failure store TTL      | 5 min | Lifetime window for `AnomalyFailures` cache |

**Counted as a failure:** after the handler returns, if the final status is **`401`** or **`403`**, the per-IP failure
counter is incremented (8-byte little-endian value in cache).

**Ban behavior:**

- If the counter is already **≥ 5** at the start of a request, the middleware returns **`403 Forbidden`** immediately
  and does not run the handler.
- The ban response is produced before `Next()`, so it does not increment the counter again.
- Cache entry TTL is **5 minutes** from the last write; each recorded failure refreshes the entry. After expiry the
  counter is removed.

The ban applies to all paths except the static frontend skip list below, including already authenticated clients.
Repeated 401/403 responses from handlers will ban the IP regardless of credentials on later requests.

Note: the constant name refers to “per minute”, but the store window is **5 minutes**. The counter only increases until
the entry expires.

## 3. Anonymous request rate limit (per IP)

Token bucket via `golang.org/x/time/rate`, one limiter per IP (`GlobalIPLimiter`).

| Constant               | Value | Meaning                                               |
|------------------------|-------|-------------------------------------------------------|
| `MaxRequestsPerMinute` | `100` | Sustained rate: 100 tokens per minute (~1 per 600 ms) |
| `MaxRequestsBurst`     | `60`  | Burst capacity (maximum tokens held at once)          |

- **Limited:** requests that are not verified as authenticated (see below).
- **Exempt:** successfully verified Session, Bearer, or Basic credentials, or GET/HEAD `?token=` that resolves to a
  valid session or bearer.
- **On exceed:** **`429 Too Many Requests`**.

### Authentication required for exemption

Carriers match production auth ([authentication.md](./authentication.md)):

| Carrier                                           | Verified when                                  |
|---------------------------------------------------|------------------------------------------------|
| Cookie `renop_session` / `Authorization: Session` | Session exists and is within idle timeout      |
| `Authorization: Bearer <user>:<secret>`           | Username and secret match a live account       |
| `Authorization: Bearer <upload-token>`            | Token is indexed and not expired               |
| `Authorization: Basic …`                          | Username and password/secret match             |
| GET/HEAD `?token=`                                | Token is a valid session id or bearer as above |

Invalid credentials do not grant exemption. Such requests still consume rate-limit tokens, and a handler 401/403 still
increments the failure counter.

Idle-expired sessions are not treated as authenticated (idle timeout approximately 7 days; see authentication
documentation).

### Limiter lifecycle

| Behavior          | Detail                                                   |
|-------------------|----------------------------------------------------------|
| Key               | Client IP string                                         |
| Idle eviction     | Entry unused for **5 minutes** is removed                |
| Periodic cleanup  | Every **5 minutes**                                      |
| Emergency cleanup | When stored IPs exceed **10,000**, cleanup runs promptly |

## Static frontend skip

For **GET** and **HEAD** only, the following paths skip the auth-failure ban and the anonymous rate limit. They still
count toward the concurrent request cap:

| Path          |
|---------------|
| `/`           |
| `/index.html` |
| `/assets/*`   |
| `/js/*`       |
| `/css/*`      |
| `/svg/*`      |

API routes, Maven repository paths, and other HTTP methods are not skipped.

## Status codes

| Code | When                                              |
|------|---------------------------------------------------|
| 429  | Anonymous IP exceeded the token-bucket rate limit |
| 403  | IP banned after too many 401/403 responses        |
| 503  | Process concurrent request cap exceeded           |

Response bodies are typically empty. There is no `Retry-After` header.
