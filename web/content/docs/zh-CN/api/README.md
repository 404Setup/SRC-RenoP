---
title: API 索引
order: 1
category: API 接口
description: RenoP HTTP、REST 与 RPC API 概览
---

# RenoP HTTP API

RenoP 提供用于管理自动化、客户端集成与健康监控的完整 HTTP API。服务默认监听
`http://localhost:3000`。

## 路由结构

| 路由前缀                        | 用途                                                     |
|:--------------------------------|:---------------------------------------------------------|
| `/api/*`                        | 认证、账号、设置、状态与消息等管理 API                   |
| `/{repo}/*`                     | 按仓库引擎执行上传、下载与删除                           |
| `/{npm-repo}/*`                 | npm packument、tarball、发布、发布标签与搜索             |
| `/index/*` 或 `/{repo}/index/*` | Cargo Sparse Index                                       |
| `/v2/*`                         | Docker 与 OCI Distribution v2                            |
| `/javadoc/*`                    | 沙箱化 Javadoc 在线预览                                  |
| `/cargodoc/*`                   | 沙箱化 Cargodoc 在线预览                                 |

## 传输格式与 Protobuf

多数管理 API 使用 JSON。高吞吐接口同时支持 `application/x-protobuf` 格式的 Google Protocol Buffers。

根据接口要求设置 `Accept: application/x-protobuf` 或 `Content-Type: application/x-protobuf`。协议定义位于
`proto/api/v1/api.proto`。

## 认证方式

- **浏览器 Cookie**：`renop_session=<session_id>`。HttpOnly 会话密钥不接受通过请求头或 URL 传递。
- **Bearer API Token**：`Authorization: Bearer <token>`。Token 能力始终与账号当前权限取交集。
- **包协议 Basic Auth**：`Authorization: Basic <base64(user:password_or_token)>`。

Basic Auth 不可调用管理 API。URL 查询参数凭据与 `Authorization: Session` 均会被拒绝。

## 常用 HTTP 状态码

| 状态码                    | 含义       | 说明                                         |
|:--------------------------|:-----------|:---------------------------------------------|
| `200 OK`                  | 成功       | 请求成功并返回响应正文                       |
| `201 Created`             | 已创建     | 资源或上传任务初始化成功                     |
| `204 No Content`          | 成功       | 请求成功且无响应正文                         |
| `400 Bad Request`         | 请求错误   | 参数或请求正文无效                           |
| `401 Unauthorized`        | 未认证     | 缺少认证信息或凭据无效                       |
| `403 Forbidden`           | 无权限     | 权限不足或 IP 被临时封禁                     |
| `404 Not Found`           | 未找到     | 目标资源不存在                               |
| `409 Conflict`            | 冲突       | 当前状态不允许操作或资源已存在               |
| `429 Too Many Requests`   | 请求过多   | 超出允许的请求速率                           |
| `503 Service Unavailable` | 服务不可用 | 服务过载或依赖暂时不可用                     |

## API 参考目录

- [认证 API](./authentication.md)
- [API Token 与用户](./tokens.md)
- [Maven API](./maven.md)
- [Cargo API](./cargo.md)
- [Docker / OCI API](./docker.md)
- [npm 存储库 API](./npm.md)
- [消息中心 API](./messages.md)
- [存储与上传 API](./storage.md)
- [设置 API](./settings.md)
- [状态与遥测 API](./status.md)
- [GPG 加密 API](./gpg.md)
- [速率限制](./rate-limit.md)
- [更新 API](./updater.md)
