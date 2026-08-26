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

- Clients generating repeated `401 Unauthorized` or `403 Forbidden` responses against private endpoints or login routes
  are flagged as anomalous.
- Flagged IPs receive temporary bans returning `403 Forbidden`, with ban durations scaling progressively on repeated
  offenses.

## Concurrency Limits (`max_active_requests`)

Configure `server.max_active_requests` in `config.yaml` (default: 512):

- When active concurrent in-flight requests reach this ceiling, incoming requests receive `503 Service Unavailable`.

## Trusted Proxies

When placed behind reverse proxies or CDNs, set `server.trusted_proxies` and `server.cdn_ip_header` in `config.yaml` so
rate limiting evaluates real client IPs rather than proxy IPs.
