---
title: API 索引
order: 1
category: API 参考
description: RenoP HTTP、REST 与 RPC API 概览
---

# RenoP HTTP API

RenoP 提供用于管理自动化、客户端集成与健康监控的完整 HTTP API。服务默认监听
`http://localhost:3000`。


[API 文档](/api) 提供系统管理、客户端集成与监控等接口说明。你也可以直接查阅或下载完整的原始 [OpenAPI 文件](/assets/openapi.yaml)。

## 路由结构

| 路由前缀                        | 用途                                         |
|:--------------------------------|:---------------------------------------------|
| `/api/*`                        | 认证、账号、设置、状态与消息等管理 API       |
| `/{repo}/*`                     | 按仓库引擎执行上传、下载与删除               |
| `/{npm-repo}/*`                 | npm packument、tarball、发布、发布标签与搜索 |
| `/index/*` 或 `/{repo}/index/*` | Cargo Sparse Index                           |
| `/v2/*`                         | Docker 与 OCI Distribution v2                |
| `/javadoc/*`                    | 沙箱化 Javadoc 在线预览                      |
| `/cargodoc/*`                   | 沙箱化 Cargodoc 在线预览                     |

## 传输格式与 Protobuf

管理 API 原生支持 JSON（`application/json`）与二进制 Protobuf（`application/x-protobuf` 或 `application/protobuf`）。通过 `Content-Type` 指定请求体编码，通过 `Accept` 协商响应格式。未显式声明或不支持的 `Accept` 默认使用 Protobuf 响应；请求未指定类型时同样采用 Protobuf 解析。完整消息定义参见 `proto/api/v1/api.proto`。

JSON 输出使用标准 snake_case 字段名，解析输入时兼容 camelCase。64 位大整数表示为十进制字符串，字节数组采用 Base64 编码。系统会严格拒绝未知或重复的 JSON 字段。常规控制请求体上限为 1 MiB（端点另有较小限制的除外）。各包管理器的原生协议、分块上传二进制流、健康检查纯文本等继续遵循各自规范。

## 认证方式

- **浏览器 Cookie**：使用 HttpOnly 的 `renop_session=<session_id>`，仅限浏览器交互，不支持通过 Header 或 URL 参数传递。
- **Bearer API Token**：在请求头中传入 `Authorization: Bearer <token>`。Token 的实际可用权限受账号自身权限约束，二者取交集。
- **包客户端 Basic Auth**：在请求头中传入 `Authorization: Basic <base64(user:password_or_token)>`。

Basic Auth 仅适用于包客户端操作，不可用于管理 API。系统不接受在 URL 查询参数或使用 `Authorization: Session` 传递凭据。

## 常用 HTTP 状态码

| 状态码                    | 含义       | 说明                           |
|:--------------------------|:-----------|:-------------------------------|
| `200 OK`                  | 成功       | 请求成功并返回响应正文         |
| `201 Created`             | 已创建     | 资源或上传任务初始化成功       |
| `204 No Content`          | 成功       | 请求成功且无响应正文           |
| `400 Bad Request`         | 请求错误   | 参数或请求正文无效             |
| `401 Unauthorized`        | 未认证     | 缺少认证信息或凭据无效         |
| `403 Forbidden`           | 无权限     | 权限不足或 IP 被临时封禁       |
| `404 Not Found`           | 未找到     | 目标资源不存在                 |
| `409 Conflict`            | 冲突       | 当前状态不允许操作或资源已存在 |
| `429 Too Many Requests`   | 请求过多   | 超出允许的请求速率             |
| `503 Service Unavailable` | 服务不可用 | 服务过载或依赖暂时不可用       |

## API 参考目录

- [认证 API](./authentication.md)
- [API Token 与用户](./tokens.md)
- [Maven API](./maven.md)
- [Cargo API](./cargo.md)
- [Docker / OCI API](./docker.md)
- [npm 存储库 API](./npm.md)
- [超级团队 API](./global-teams.md)
- [发布配额 API](./publication-quotas.md)
- [审核 API](./reviews.md)
- [消息中心 API](./messages.md)
- [存储与上传 API](./storage.md)
- [设置 API](./settings.md)
- [状态与遥测 API](./status.md)
- [GPG 加密 API](./gpg.md)
- [速率限制](./rate-limit.md)
- [更新 API](./updater.md)
