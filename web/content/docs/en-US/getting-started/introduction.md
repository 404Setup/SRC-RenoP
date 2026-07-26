---
title: Introduction
order: 1
category: Getting started
description: What RenoP is and who it is for
---

# Introduction

RenoP is a lightweight, rapidly deployable **self-hosted Maven server** for individuals and teams.

It focuses on:

- Fast setup with sensible defaults
- Release, snapshot, and private repositories
- Maven mirror proxying with local caching
- A small web UI for browsing, uploads, users, tokens, and health

If you intend to use it for **public hosting**, RenoP currently does not target that scenario as a primary goal.

## Design goals

| Goal         | Meaning                                              |
|--------------|------------------------------------------------------|
| Simple ops   | One binary, config files in the working directory    |
| Maven-native | Standard repository layouts and client compatibility |
| Transparent  | No ads, no product telemetry, free community edition |

## Feature highlights

- **Release / snapshot / private** repositories with Maven layouts
- **Upstream mirrors** with local cache and negative cache
- **Web UI** for browsing, upload, users, tokens, and health
- **Local disk or S3-compatible** object storage
- **Auth**: sessions, Basic, Bearer / upload tokens, repository permissions
- **Extras**: checksums, Javadoc browsing, online updater, chunked upload API

## Next steps

1. [Install](./install.md) a release or preview build
2. Follow the [Quick start](./quickstart.md)
3. Connect a [Maven client](./maven-client.md)
4. Review [Configuration](../configuration/overview.md) when you need more control
