---
title: API 索引
order: 1
category: API 接口
description: RenoP HTTP RESTful 与 RPC API 规范与端点索引
---

# RenoP API 概览

RenoP 提供了完整的 HTTP API，用于自动化管理、客户端接入与系统监控。服务默认监听在 `http://localhost:3000`。

## API 路由前缀划分

| 路由前缀                        | 用途说明                                             |
|:--------------------------------|:-----------------------------------------------------|
| `/api/*`                        | 管理 API（认证、用户与令牌、设置、监控、消息中心等） |
| `/{repo}/*`                     | Maven 仓库标准路径（制品拉取、上传与删除）           |
| `/index/*` 或 `/{repo}/index/*` | Cargo 稀疏索引协议端点                               |
| `/v2/*`                         | Docker 与 OCI Registry v2 规范端点                   |
| `/javadoc/*`                    | Javadoc 在线 HTML 预览                               |
| `/cargodoc/*`                   | Cargodoc 在线 HTML 预览                              |

## 数据格式与 Protobuf 支持

大部分管理接口支持标准 JSON 格式。对于高频数据交换接口，RenoP 同时支持 Google Protocol Buffers (`application/x-protobuf`)
以降低传输开销。

客户端可以通过在请求头中指定 `Accept: application/x-protobuf` 或 `Content-Type: application/x-protobuf` 来使用二进制协议。完整的
Proto 协议定义文件位于仓库中的 `proto/api/v1/api.proto`。

## 认证方式

请求需认证的 API 时，支持以下方式：

1. **浏览器 Cookie**：HttpOnly `renop_session=<session_id>`；会话密钥不能通过请求头或 URL 使用。
2. **Bearer API Token**：`Authorization: Bearer <token>`；接口范围与账号实时权限取交集。
3. **包协议 Basic Auth**：`Authorization: Basic <base64(user:password_or_token)>`。

Basic 凭据不能调用管理 API。查询参数凭据和 `Authorization: Session` 均会被拒绝。

## 常用状态码说明

| 状态码                    | 含义       | 说明                                                         |
|:--------------------------|:-----------|:-------------------------------------------------------------|
| `200 OK`                  | 成功       | 请求处理成功并返回数据                                       |
| `201 Created`             | 创建成功   | 资源或上传任务创建成功                                       |
| `204 No Content`          | 成功无正文 | 操作成功（如删除或标记状态），无额外响应内容                 |
| `400 Bad Request`         | 参数错误   | 请求体格式错误或必填字段缺失                                 |
| `401 Unauthorized`        | 未认证     | 未提供有效认证凭证                                           |
| `403 Forbidden`           | 无权限     | 权限不足，或因连续认证失败触发 IP 临时封禁                   |
| `404 Not Found`           | 资源不存在 | 请求的制品或接口不存在（访问无权限的私有仓库也可能返回 404） |
| `409 Conflict`            | 冲突       | 资源已存在或不可覆盖部署                                     |
| `429 Too Many Requests`   | 触发限流   | 请求频率超过系统设定阈值                                     |
| `503 Service Unavailable` | 服务过载   | 当前活跃请求数达到上限                                       |

## API 详细文档索引

- [认证与会话 API](./authentication.md)
- [用户与令牌 API](./tokens.md)
- [Maven 制品与元数据 API](./maven.md)
- [Cargo 注册源 API](./cargo.md)
- [Docker / OCI 镜像库 API](./docker.md)
- [消息中心 API](./messages.md)
- [存储与分块上传 API](./storage.md)
- [系统设置 API](./settings.md)
- [健康与状态监控 API](./status.md)
- [GPG 密钥与签名 API](./gpg.md)
- [限流与安全防御说明](./rate-limit.md)
- [在线更新 API](./updater.md)
