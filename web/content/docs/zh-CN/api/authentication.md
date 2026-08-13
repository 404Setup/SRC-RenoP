---
title: 身份认证
order: 2
category: API
---

# 身份认证与会话

前缀：`/api/auth`

账户与令牌配置可通过 `tokens.yaml`（环境变量 `RENOP_TOKENS`）提供初始数据。系统启动时会自动迁移并持久化至内嵌 SQLite 数据库（默认 `renop.db`）。权限为字符串列表。

## 权限

| 值                    | 含义                           |
|-----------------------|--------------------------------|
| `admin`/`manager`     | 管理 API（代码中视为等价）     |
| `canview:*`           | 读取全部仓库                   |
| `canview:<repo>`      | 读取单个仓库                   |
| `canupdate:*`         | 写入全部仓库                   |
| `canupdate:<repo>`    | 写入单个仓库                   |
| `allview`/`proview`   | 读取 PRIVATE（及类似受限）可见性|
| `showing`             | 列出 HIDDEN 仓库根             |

仓库可见性：

- **PUBLIC** — 匿名可读
- **HIDDEN** — 文件可读；列出根目录需要额外角色
- **PRIVATE** — 需要 `canview`/`allview`/`proview`、该仓库写权限或 manager

写入（PUT/POST/DELETE 制品）始终需要 `canupdate` 或 manager。

## 登录

### `POST /api/auth/login`

正文：`application/x-protobuf`，`LoginRequest`

| 字段     | 类型   | 约束                     |
|----------|--------|--------------------------|
| `name`   | string | 1–128 字符               |
| `secret` | string | 1–72 字节（bcrypt 上限） |

成功时返回：`SessionDetails`（protobuf）与 cookie：

- 名称：`renop_session`
- HttpOnly，SameSite=Lax
- HTTPS 时 `Secure`（含 `X-Forwarded-Proto: https` / Cloudflare visitor HTTPS）
- Max-Age ≈ 7 天

| 状态 | 原因             |
|------|------------------|
| 401  | 用户名或密码错误 |
| 403  | 账户已过期       |
| 400  | 正文无法读取     |

会话 ID 仅写入 `renop_session` cookie。登录响应中的 `session_token` 为空；浏览器依赖 cookie，脚本可将同一 ID 以 `Authorization: Session …` 发送。

## 当前用户

### `GET /api/auth/me`

返回当前会话的 `SessionDetails`（protobuf）。未认证时返回 401。

| 字段            | 含义                                                                                                                                           |
|-----------------|------------------------------------------------------------------------------------------------------------------------------------------------|
| `access_token`  | 账户摘要（name、created_at、permissions 等）                                                                                                   |
| `permissions[]` | 展开后的角色（manager 会额外得到 `access-token:manager`）                                                                                      |
| `routes[]`      | 来自 canview/canupdate 的路径权限（`route:read`/`route:write`）。manager 还会在 `*` 上得到 `route:write`，便于客户端镜像写入门禁而不必特殊处理 manager|
| `session_token` | 请求使用 `Session` 头时设置                                                                                                                    |

写入 UI（浏览器上传面板、删除按钮）与存储 PUT/POST/DELETE 需要相同的有效写权限：`admin`/`manager`、`canupdate:*` 或 `canupdate:<repo>`。

若 cookie 与当前会话不一致，会刷新 cookie。

## 登出

### `POST /api/auth/logout`

使会话失效并清除 cookie。`204 No Content`。无会话时也返回 204。

## 个人资料

以下均需已登录用户。

### `PUT /api/auth/profile/password`

正文：`application/x-protobuf`，`UpdatePasswordRequest`（兼用于 JSON 兼容输入）：

| 字段           | 类型   | 约束      |
|----------------|--------|-----------|
| `new_password` | string | 6–72 字节 |

响应：`StatusOk` protobuf（`status: success`）。长度无效时返回 400。

### `POST /api/auth/profile/token`

重新生成上传令牌（每用户一个；旧值被替换）。响应：`GenerateTokenResponse` protobuf (`token: "<uuid>"`).

Maven / curl：

```bash
curl -u admin:UPLOAD_TOKEN -T my.jar \
  http://localhost:3000/releases/com/example/demo/1.0.0/demo-1.0.0.jar
```

Basic 的 secret 可用账户密码或上传令牌，取决于账户配置。

### `GET /api/auth/profile/fido`

列出当前用户已绑定的 FIDO/WebAuthn 安全密钥设备。

响应：`application/x-protobuf`，`FidoDeviceList`

| 字段（`devices[]` 每项） | 含义                  |
|--------------------------|-----------------------|
| `id`                     | 设备唯一 ID           |
| `username`               | 账户名                |
| `name`                   | 设备自定义名称        |
| `created_at`             | 创建时间（Unix 毫秒） |

### `POST /api/auth/profile/fido/register/begin`

发起 FIDO 设备注册会话。返回 `session_id` 与 WebAuthn 注册凭证选项 `options`。

### `POST /api/auth/profile/fido/register/finish`

完成 FIDO 设备注册。传入 `session_id`、`name` 以及浏览器生成的凭证 `credential` JSON。

### `DELETE /api/auth/profile/fido/:device_id`

按 `device_id` 删除自己的一个 FIDO 安全密钥。响应：`StatusOk` protobuf。

### `POST /api/auth/fido/login/begin`

发起 FIDO 免密 / 无密码登录流程。可选传入 `username`。

### `POST /api/auth/fido/login/finish`

完成 FIDO 登录校验。认证成功后颁发浏览器 `renop_session` Cookie 并返回 `SessionDetails` protobuf。

### `GET /api/auth/profile/sessions`

列出当前用户的 **浏览器登录会话**。Basic / Bearer 认证 **不会** 创建会话，也不会出现在此列表。Session 密钥（Cookie 值）**不会** 返回。

响应：`application/x-protobuf`，`SessionList`

| 字段（`sessions[]` 每项） | 含义                                                    |
|---------------------------|---------------------------------------------------------|
| `public_id`               | 用于撤销 API 的不透明 ID（不是 Cookie 密钥）            |
| `username`                | 账户名                                                  |
| `ip`                      | 最后一次看到的客户端 IP                                 |
| `user_agent`              | 登录时的设备/User-Agent                                 |
| `created_at`              | 创建时间（Unix 毫秒）                                   |
| `last_active`             | 最后活跃（Unix 毫秒）                                   |
| `expires_at`              | 空闲过期：`last_active` + 空闲超时（通常 7 天，Unix 毫秒）|
| `current`                 | 为本次请求所用会话时为 `true`                           |

### `POST /api/auth/profile/sessions/revoke-others`

撤销当前用户除 **本请求会话** 以外的全部浏览器会话。响应：`StatusOk` protobuf（`status: success`）。

若调用方使用 Basic/Bearer（无浏览器会话），则撤销其全部浏览器会话。

### `DELETE /api/auth/profile/sessions/:session_id`

按 `public_id` 删除**自己的**一个会话。响应：`StatusOk` protobuf。缺失 ID 为 no-op。撤销当前会话会清除 Cookie。

## 管理员会话管理

管理员（`admin`/`manager`）可通过 `/api/tokens` 查看并撤销**任意**账户的浏览器会话。

### `GET /api/tokens/:name/sessions`

该用户的 `SessionList` protobuf。账户不存在时返回 `404`。非管理员时返回 `403`。

### `POST /api/tokens/:name/sessions/revoke-all`

撤销该用户全部浏览器会话。当管理员操作**自己的**账户时，会保留本请求的会话以免中途被踢出。响应：`StatusOk` protobuf。

### `DELETE /api/tokens/:name/sessions/:session_id`

按 `public_id` 撤销该用户的一个会话。响应：`StatusOk` protobuf。缺失 ID 为 no-op。

## 客户端如何携带凭证

| 场景                | 方式                               |
|---------------------|------------------------------------|
| 浏览器 UI           | Cookie（登录时设置）               |
| 调用管理 API 的脚本 | `Authorization: Session …` 或 cookie|
| Maven deploy        | Basic：`username` + 密码或上传令牌 |
| CI 私有下载         | Basic/Bearer；PUBLIC 仓库无需认证  |

`Bearer name:secret` 行为类似 Basic（密码哈希或上传令牌）。
`Bearer <upload-token>`（无用户名）通过令牌索引查找用户。
