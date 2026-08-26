---
title: In-Place Updater API
order: 13
category: API Reference
description: Checking updates, channel switching, and applying updates
---

# In-Place Updater API

Updater mutations require a manager session or an API token with `admin:updates`. Failures return JSON and the stable
`X-Renop-Error-Code` header so clients can localize messages without displaying internal filesystem or network errors.

## Read updater state

`GET /api/updater/status` returns protobuf `UpdateState`. The status is one of `idle`, `checking`, `available`,
`downloading`, `ready_to_restart`, or `error`. While an online installation is running, poll this endpoint for progress.

## Check the configured channel

`POST /api/updater/check?channel=release|nightly` returns a JSON `CheckResult`. The optional query overrides the current
channel for this request. The result includes the selected target, SHA-256 digest, package size, release notes, and the
full retained change range between the running and target versions.

## Start an online installation

`POST /api/updater/install` starts bounded background download, digest verification, Brotli/ZIP extraction, and binary
validation. A successful response is `{"status":"started"}`. It does not restart the process automatically.

Download progress is transient UI state and is shown as a toast, not stored in the message center. Durable check and
failure results remain administrator notifications.

## Install an offline package

`POST /api/updater/upload` accepts multipart field `file` or `package` containing a raw `.br` release package or legacy
`.zip` package. Large packages should use the chunked upload API with `purpose=updater`; complete the upload through
`POST /api/upload/chunked/{upload_id}/complete`.

The server streams the package through bounded temporary storage, validates the executable platform, and returns
`ready_to_restart`. Failed packages are classified without returning internal paths.

## Restart

`POST /api/updater/restart` applies the prepared executable, when present, and restarts RenoP. The connection may close
before the JSON `{"status":"restarting"}` response reaches the client. The official frontend shows the imminent restart
as a toast.

## Stable error codes

Updater failures may return `forbidden`, `insufficient_space`, `missing_file`, `install_busy`, `invalid_package`,
`incompatible_binary`, `package_too_large`, `package_processing_failed`, `check_failed`, `notification_failed`, or
`restart_failed` in `X-Renop-Error-Code`.
