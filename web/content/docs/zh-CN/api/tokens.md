---
title: 令牌
order: 3
category: API
---

# 用户与访问令牌

前缀：`/api/tokens`

所有接口需要 **manager / admin**。普通用户通过
`/api/auth/profile/*` 修改自己的密码或上传令牌。

此处的「令牌」指账户记录：用户名、密码哈希、权限、可选上传令牌。持久化在
`tokens.yaml`。

## `GET /api/tokens`

列出全部账户。响应：`application/x-protobuf`，`AccessTokenList`。

形状（JSON 示意）：

```json
{
  "tokens": [
    {
      "identifier": {"type": "PERSISTENT", "value": 1},
      "name": "admin",
      "created_at": "2026-01-01T00:00:00Z",
      "description": "…",
      "expires_at": null,
      "tokens": ["<upload-token-if-any>"],
      "permissions": ["manager", "canview:*", "canupdate:*"]
    }
  ]
}
```

从不返回密码哈希。`tokens` 数组在存在时包含明文上传令牌。禁止 → 403。

## `GET /api/tokens/:name`

单个账户， **protobuf** `AccessTokenDto` (`application/x-protobuf`)。名称不区分大小写（小写存储）。不存在 → 404。

## `PUT /api/tokens/:name`

创建或更新。正文：`application/x-protobuf`，`CreateAccessTokenRequest`（兼用于 JSON 兼容输入）。

| 字段          | 含义                                             |
|---------------|--------------------------------------------------|
| `is_create`   | `true` 且名称已存在 → 409                        |
| `secret`      | 创建时省略则生成 UUID 密码；更新时省略则不改密码 |
| `new_name`    | 重命名；目标冲突 → 409                           |
| `permissions` | 仅在提供时替换权限列表                           |

响应：`application/x-protobuf`，`CreateAccessTokenResponse`

```protobuf
message CreateAccessTokenResponse {
  AccessTokenDto access_token = 1;
  string secret = 2; // 仅在本次请求生成或提供时存在
}
```

创建后请立即保存 `secret` — 明文密码之后无法恢复。

## `DELETE /api/tokens/:name`

删除账户。`204`。不存在 → 404。

## 浏览器会话与 FIDO 设备（管理员）

管理员可列出并撤销任意账户的 **浏览器登录会话** 与 **FIDO 安全密钥设备**。Basic/Bearer 不是会话。Session 密钥永不返回。

### `GET /api/tokens/:name/sessions`

`SessionList` protobuf。账户不存在 → `404`。

### `POST /api/tokens/:name/sessions/revoke-all`

撤销该用户全部浏览器会话。管理员操作 **自己的** 账户时保留本请求会话。响应：`StatusOk` protobuf。

### `DELETE /api/tokens/:name/sessions/:session_id`

按 `public_id` 撤销一个会话。响应：`StatusOk` protobuf。缺失 id 为 no-op。

### `GET /api/auth/users/:username/fido`

管理员查看指定用户已绑定的 FIDO 设备列表。响应：`FidoDeviceList` protobuf。

### `DELETE /api/auth/users/:username/fido/:device_id`

管理员删除指定用户的指定 FIDO 设备。响应：`StatusOk` protobuf。

## `POST /api/tokens/:name/token`

管理员为用户重新签发上传令牌（替换旧值）。响应：`GenerateTokenResponse` protobuf (`token: "<uuid>"`).

与 `/api/auth/profile/token` 思路相同，但面向其他用户。
