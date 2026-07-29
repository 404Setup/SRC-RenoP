---
title: Introduction
order: 1
category: Getting started
description: What RenoP is
---

# Introduction

RenoP is a self-hosted Maven server.

- Release, snapshot, and private repositories
- Upstream mirror proxy with local caching
- Web UI for browse, upload, users, tokens, and health

Public multi-tenant hosting is out of scope for now.

## Goals

| Goal         | Meaning                                           |
|--------------|---------------------------------------------------|
| Simple ops   | One binary; config lives in the working directory |
| Maven layout | Standard repository paths; normal clients work    |
| No junk      | No ads, no product telemetry, free                |

## Features

- **Release / snapshot / private** repositories (Maven layout)
- **Upstream mirrors** with local cache and negative cache
- **Web UI**: browse, upload, users, tokens, health
- **Local disk or S3-compatible** storage
- **Auth**: sessions, Basic, Bearer / upload tokens, repo permissions
- **Also**: checksums, Javadoc browsing, online updater, chunked upload API

## Next steps

1. [Install](./install.md)
2. [Quick start](./quickstart.md)
3. [Maven client](./maven-client.md)
4. [Configuration](../configuration/overview.md)
