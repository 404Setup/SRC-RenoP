---
title: Rate Limiting & Defense
order: 12
category: API Reference
description: Rate limiting algorithms, anomaly detection, and IP defense policies
---

# Rate Limiting & Defense

RenoP implements multi-tier rate limiting and anomaly mitigation to protect services against brute force,
denial-of-service, and scraping.

## Anonymous Rate Limiting

For unauthenticated client IPs, requests are governed by a sliding-window token bucket algorithm:

- **Public Artifact Downloads**: Generous rate limits.
- **Search & Metadata**: Stricter limits; exceeding thresholds yields `429 Too Many Requests`.

## Repeated Authentication Failures & IP Bans

- Failed credentials or login attempts returning `401 Unauthorized` or `403 Forbidden` are counted per IP.
- After 10 failures, subsequent requests return `403 Forbidden`. The counter expires five minutes after the last
  counted failure; rejected requests do not extend this interval.
- Anonymous permission challenges and permission denials for valid sessions do not count. Frontend pages and static
  assets remain accessible.

## Concurrency Limits (`max_active_requests`)

Configure `server.max_active_requests` in `config.yaml` (default: 512):

- When active concurrent in-flight requests reach this ceiling, incoming requests receive `503 Service Unavailable`.

## Trusted Proxies

When placed behind reverse proxies or CDNs, set `server.trusted_proxies` and `server.cdn_ip_header` in `config.yaml` so
rate limiting evaluates real client IPs rather than proxy IPs.
