---
title: API 索引
order: 1
category: API
---

# RenoP HTTP API

默认监听地址：`0.0.0.0:3000`。

| 路径        | 用途                               |
|-------------|-------------------------------------|
| `/api/*`    | 管理 API（登录、设置、状态等）      |
| `/{repo}/…` | Maven 仓库布局（下载/上传/删除）    |

错误响应正文多为纯文本（`Unauthorized`、`Forbidden`、`Not found`）。以状态码为准。

## 索引

| 文件                                     | 内容                                    |
|------------------------------------------|-----------------------------------------|
| [authentication.md](./authentication.md) | 登录、会话、权限                        |
| [tokens.md](./tokens.md)                 | 账户管理（manager）                     |
| [maven.md](./maven.md)                   | 浏览、版本、徽章、生成 POM              |
| [status.md](./status.md)                 | 健康检查与运行时状态                    |
| [settings.md](./settings.md)             | 配置域、仓库、索引重建                  |
| [updater.md](./updater.md)               | 在线/离线更新                           |
| [storage.md](./storage.md)               | 仓库路径上的 GET/PUT/DELETE、可选分块上传|
| [rate-limit.md](./rate-limit.md)         | IP 限流、认证失败封禁、并发请求上限     |

机器可读规范：[openapi.yaml](/assets/openapi.yaml)。
Proto 定义：`proto/api/v1/api.proto`（生成的 Go 代码在 `pb/` 下）。

## JSON 与 Protobuf

多数接口仍使用 JSON。下列接口使用 `application/x-protobuf`：

| 接口                                         | 方向               |
|----------------------------------------------|--------------------|
| `POST /api/auth/login`                       | request + response |
| `GET /api/auth/me`                           | response           |
| `GET /api/tokens`                            | response           |
| `GET /api/status/instance`                   | response           |
| `GET /api/status/snapshots`                  | response           |
| `GET /api/updater/status`                    | response           |
| `POST /api/settings/index/rebuild`           | request            |
| `GET /api/settings/domains`                  | response           |
| `GET /api/settings/domain/:name`             | response           |
| `PUT /api/settings/domain/:name`             | request            |
| `GET /api/settings/maven/repositories`       | response           |
| `PUT /api/settings/maven/repositories/:name` | request            |
| `GET /api/maven/details…`                    | response           |
| `GET /api/maven/repo-details/:repo`          | response           |
| `POST /api/upload/chunked/`                  | request + response |
| `POST /api/upload/chunked/:id/complete`      | response           |

字段名与 proto 一致（snake_case）。可用 `protoc` 生成客户端，或参考前端的 `protobufjs` 编解码。

```bash
curl -s http://localhost:3000/api/status/health
# "UP"
```

```bash
# 登录后 cookie 名为 renop_session
curl -s -b 'renop_session=<session-id>' \
  -H 'Accept: application/x-protobuf' \
  http://localhost:3000/api/auth/me \
  -o me.bin
```

## 身份认证

支持的传递方式：

1. Cookie：`renop_session=<id>`
2. `Authorization: Session <id>`
3. `Authorization: Basic base64(user:password_or_upload_token)`
4. `Authorization: Bearer <user>:<secret>` 或 `Bearer <upload-token>`
5. 仅 GET/HEAD：`?token=<session-or-bearer>`

会话在约 **7 天** 空闲后过期，有活动时会续期。

| 角色         | 能力                                      |
|--------------|-------------------------------------------|
| 匿名         | 读取 PUBLIC 仓库；管理 API 大多返回 401/403|
| 普通用户     | 通过 `canview:`/`canupdate:` 访问仓库     |
| manager/admin| 用户、设置、更新器等管理 API              |

详情见 [authentication.md](./authentication.md)。

## 状态码

| 码  | 含义                                        |
|-----|---------------------------------------------|
| 200 | 成功（正文可能为空或纯文本）                |
| 201 | 上传已创建                                  |
| 204 | 成功，无正文                                |
| 400 | 参数/正文无效                               |
| 401 | 未认证或凭证无效                            |
| 403 | 无权限、已过期，或多次 401/403 后 IP 被封禁 |
| 404 | 不存在，私有读取也可能返回 404 而非 403     |
| 409 | 冲突（名称占用、更新已在进行）              |
| 429 | 匿名 IP 超过请求速率限制                    |
| 503 | 过载（例如并发请求上限）                    |
| 507 | 磁盘空间不足                                |

限流与异常规则见 [rate-limit.md](./rate-limit.md)。

实例版本：`GET /api/status/instance` 上的 `version`。没有单独的 API 版本字段。
