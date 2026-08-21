---
title: 令牌
order: 3
category: API
---

# 用户与访问令牌

前缀：`/api/tokens`

所有接口需要 **manager/admin** 权限。普通用户通过 `/api/auth/profile/*` 修改自己的密码或上传令牌。

此处的「令牌」指账户记录：用户名、密码哈希、权限以及可选的上传令牌。数据持久化在数据库中。

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

不返回密码哈希。`tokens` 数组在存在时包含明文上传令牌。权限不足时返回 `403`。

## `GET /api/tokens/:name`

单个账户， **protobuf** `AccessTokenDto` (`application/x-protobuf`)。名称不区分大小写（以小写形式存储）。账户不存在时返回
`404`。

## `PUT /api/tokens/:name`

创建或更新。正文：`application/x-protobuf`，`CreateAccessTokenRequest`（兼容 JSON 格式输入）。

| 字段          | 含义                                                       |
|---------------|------------------------------------------------------------|
| `is_create`   | 设为 `true` 且名称已存在时返回 `409`                       |
| `secret`      | 创建时省略则自动生成 UUID 密码；更新时省略则保持原密码不变 |
| `new_name`    | 用于重命名；目标名称已存在时返回 `409`                     |
| `permissions` | 仅在提供时替换权限列表                                     |

响应：`application/x-protobuf`，`CreateAccessTokenResponse`

```protobuf
syntax = "proto3";

message CreateAccessTokenResponse {
  AccessTokenDto access_token = 1;
  string secret = 2; // 仅在本次请求生成或提供时存在
}
```

创建后请立即保存 `secret` — 明文密码之后无法再次获取。

## `DELETE /api/tokens/:name`

删除账户。成功返回 `204`。账户不存在时返回 `404`。

## 浏览器会话与 FIDO 设备（管理员）

管理员可列出并撤销任意账户的 **浏览器登录会话**与 **FIDO 安全密钥设备**。Basic/Bearer 认证不属于会话，Session 密钥永不返回。

### `GET /api/tokens/:name/sessions`

返回 `SessionList` protobuf。账户不存在时返回 `404`。

### `POST /api/tokens/:name/sessions/revoke-all`

撤销该用户全部浏览器会话。管理员操作 **自己的**账户时保留当前请求的会话。响应：`StatusOk` protobuf。

### `DELETE /api/tokens/:name/sessions/:session_id`

按 `public_id` 撤销单个会话。响应：`StatusOk` protobuf。会话 ID 不存在时为空操作（不报错）。

### `GET /api/auth/users/:username/fido`

管理员查看指定用户已绑定的 FIDO 设备列表。响应：`FidoDeviceList` protobuf。

### `DELETE /api/auth/users/:username/fido/:device_id`

管理员删除指定用户的指定 FIDO 设备。响应：`StatusOk` protobuf。

## `POST /api/tokens/:name/token`

管理员为指定用户重新签发上传令牌（替换旧值）。响应：`GenerateTokenResponse` protobuf (`token: "<uuid>"`)。

与 `/api/auth/profile/token` 功能相同，但面向其他用户。
