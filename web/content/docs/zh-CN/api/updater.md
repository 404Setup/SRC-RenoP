---
title: 在线更新 API
order: 13
category: API 参考
description: 检查新版本、更新通道切换与应用更新接口
---

# 在线更新 API

更新写操作要求管理员会话，或带有 `admin:updates` 权限的 API Token。失败响应使用 JSON，并通过稳定的
`X-Renop-Error-Code` 响应头提供错误类型，使客户端无需显示内部文件路径或网络错误即可完成本地化。

## 读取更新状态

`GET /api/updater/status` 返回 protobuf `UpdateState`。状态可能为 `idle`、`checking`、`available`、
`downloading`、`ready_to_restart` 或 `error`。在线安装期间应轮询此接口获取进度。

## 检查更新通道

`POST /api/updater/check?channel=release|nightly` 返回 JSON `CheckResult`。可选查询参数仅覆盖本次请求所用
通道。结果包含目标文件、SHA-256、包大小、更新日志，以及当前版本到目标版本之间服务端仍保留的完整变更范围。

## 启动在线安装

`POST /api/updater/install` 在后台执行下载、完整性校验、更新包解包与架构验证。
成功返回 `{"status":"started"}`，不会自动重启进程。
安装进度与最终状态将通过状态接口查询，异常将记录为管理员通知。

## 安装离线更新包

`POST /api/updater/upload` 接受 multipart 字段 `file` 或 `package`，
内容可为 Brotli 格式（`.br`）或标准 `.zip` 安装包。
大文件可通过 `purpose=updater` 的分块上传接口传输，并通过
`POST /api/upload/chunked/{upload_id}/complete` 完成安装。

服务端在后台完成更新包解包与平台架构校验，验证通过后状态更新为 `ready_to_restart`。

## 重启

`POST /api/updater/restart` 将应用就绪的更新程序并安全重启 RenoP 服务进程。

## 稳定错误码

更新接口可在 `X-Renop-Error-Code` 中返回 `forbidden`、`insufficient_space`、`missing_file`、
`install_busy`、`invalid_package`、`incompatible_binary`、`package_too_large`、
`package_processing_failed`、`check_failed`、`notification_failed` 或 `restart_failed`。
