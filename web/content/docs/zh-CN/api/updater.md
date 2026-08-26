---
title: 在线更新 API
order: 13
category: API 接口
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

`POST /api/updater/install` 在后台执行有界下载、哈希校验、Brotli/ZIP 解包与二进制平台校验。成功返回
`{"status":"started"}`，不会自动重启进程。

下载进度属于临时 UI 状态，使用 Toast 提示而不写入消息中心；检查结果与失败结果仍作为管理员通知保存。

## 安装离线更新包

`POST /api/updater/upload` 接受 multipart 字段 `file` 或 `package`，内容可为新版原始 `.br` 发布包或旧版
`.zip` 包。大文件应使用 `purpose=updater` 的分块上传接口，并通过
`POST /api/upload/chunked/{upload_id}/complete` 完成安装。

服务端全程使用有界临时存储流式处理文件，校验可执行文件平台后返回 `ready_to_restart`。失败时不会向前端
返回内部路径。

## 重启

`POST /api/updater/restart` 会应用已准备的可执行文件（如有）并重启 RenoP。连接可能在客户端收到
`{"status":"restarting"}` 前断开；官方前端会使用 Toast 提示系统即将重启。

## 稳定错误码

更新接口可在 `X-Renop-Error-Code` 中返回 `forbidden`、`insufficient_space`、`missing_file`、
`install_busy`、`invalid_package`、`incompatible_binary`、`package_too_large`、
`package_processing_failed`、`check_failed`、`notification_failed` 或 `restart_failed`。
