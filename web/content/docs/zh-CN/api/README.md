---
title: API 索引
order: 1
category: API 参考
description: RenoP HTTP、REST 与 RPC API 概览
---

# RenoP HTTP API

RenoP 提供用于管理自动化、客户端集成与健康监控的完整 HTTP API。服务默认监听
`http://localhost:3000`。


独立的 [API 页面](/api) 可切换 RenoPAPI Markdown 与 OpenAPI 渲染。`/api/authentication` 等深层链接打开文章，`?view=openapi` 选择接口规范。本地托管的 OpenAPI 渲染器按需加载，不执行 API 请求。仍可下载原始 [OpenAPI 文件](/assets/openapi.yaml)。常规文档不再包含 API 分类；旧 `/docs/api/...` 书签会转到 `/api/...`。

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

基于消息模式的管理 API 支持 JSON（`application/json`）和二进制 protobuf（`application/x-protobuf` 或
`application/protobuf`）。`Content-Type` 决定请求解码方式，`Accept` 决定响应格式。未指定或不支持的 `Accept`
会保留旧客户端使用的 protobuf 响应；未指定请求类型时也沿用 protobuf 解码。消息定义见 `proto/api/v1/api.proto`。

JSON 输出使用原始 snake_case 字段名，输入也接受 protobuf 的 camelCase 名称。64 位整数使用十进制字符串，
字节字段使用 Base64 字符串。未知或重复的 JSON 字段会被拒绝。控制请求仍限制为 1 MiB，并保留端点原有的
更小上限。原生仓库协议、上传二进制分块、健康检查纯文本与各端点的错误格式维持原状。

## 认证方式

- **浏览器 Cookie**：`renop_session=<session_id>`。HttpOnly 会话密钥不接受通过请求头或 URL 传递。
- **Bearer API Token**：`Authorization: Bearer <token>`。Token 能力始终与账号当前权限取交集。
- **包协议 Basic Auth**：`Authorization: Basic <base64(user:password_or_token)>`。

Basic Auth 不可调用管理 API。URL 查询参数凭据与 `Authorization: Session` 均会被拒绝。

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
