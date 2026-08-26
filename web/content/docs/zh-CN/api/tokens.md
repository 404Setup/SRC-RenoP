---
title: API Token 与用户
order: 3
category: API 接口
description: 细粒度 API Token 生命周期、认证边界与管理员用户接口
---

# API Token 与用户

API Token 是归属于单个账号的持久机器凭据。每个密钥包含 256 位随机熵，RenoP 仅保存其 SHA-256 查询摘要。
明文只会在创建成功时返回一次，之后无法恢复。

使用 Token 执行请求时必须同时满足两个条件：Token 含有接口要求的能力范围，并且其所属账号当前仍有权操作
目标资源。因此，修改账号角色、仓库权限或包团队等级后，无需重新创建 Token 即可立即收紧权限。

## 管理自己的 API Token

Token 管理接口只接受 HttpOnly `renop_session` 浏览器 Cookie。API Token、密码、`Authorization: Session`
以及 URL 查询参数均不能用于管理 Token 密钥。

### 查询当前可分配的权限范围

`GET /api/auth/profile/api-tokens/scopes`

返回结果会按照当前账号权限过滤，普通账号不会获得任何 `admin:*` 范围。

响应同时返回各权限支持的目标类型以及单个 Token 最多 128 个目标的限制。

### 创建 Token

`POST /api/auth/profile/api-tokens`

```json
{
  "name": "CI 发布",
  "scopes": ["repository:read", "repository:publish"],
  "targets": {
    "repository:publish": ["releases"]
  },
  "expires_at": 1798761600000
}
```

`expires_at` 是可选的 Unix 毫秒时间戳，必须处于创建后 5 分钟至 5 年之间；省略或传入 `null` 表示不设置
凭据级有效期。每个账号最多可拥有 50 个 API Token。

`targets` 为可选配置，可分别限定每项权限的精确目标。没有出现在 `targets` 中的权限可以访问所属账号当前
有权操作的全部目标。存储库目标直接使用存储库名称；包目标使用 `存储库/包`，Maven 包可使用类似
`maven-releases/com.example/library` 的“存储库/Group ID/Artifact ID”；团队目标使用
`package/存储库/包` 或 `domain/example.com`；域目标使用规范化域名。一个 Token 最多可配置 128 个目标。
目标限制始终与账号的存储库权限及包/域团队当前 L0-L4 权限取交集，不会产生权限提升。

成功时返回 `201 Created` 和 `Cache-Control: no-store`。响应中的 `secret` 是唯一一次可见的明文；列表接口
只返回 `id`、名称、权限范围、创建时间和过期时间。

### 查询 Token 元数据

`GET /api/auth/profile/api-tokens`

### 撤销 Token

`DELETE /api/auth/profile/api-tokens/{token_id}`

成功撤销返回 `204 No Content`，并立即清除缓存的认证结果。

## 权限范围

| 范围 | 能力 |
|:-----|:-----|
| `repository:read` | 读取仓库目录、元数据、文件、镜像和版本 |
| `repository:publish` | 通过 Maven、Cargo、Docker、纯文件或分块上传协议发布 |
| `repository:delete` | 删除仓库文件、包版本、标签或镜像 |
| `package:create` | 在仓库授权通过后创建新的 Cargo 包或 Docker 镜像 |
| `package:metadata` | 更新包描述及其他包元数据 |
| `package:lifecycle` | 归档、恢复、撤回或重新开放包与版本 |
| `team:manage` | 查看并管理 Cargo、Docker 及 Maven 发布域团队与邀请 |
| `domain:read` | 读取私有 Maven 发布域配置 |
| `domain:create` | 创建 Maven 发布域 |
| `domain:verify` | 请求或强制执行 Maven 发布域所有权验证 |
| `domain:delete` | 删除 Maven 发布域 |
| `messages:read` | 读取、标记和删除账号通知 |
| `account:read` | 读取私有账号数据及个人行为日志 |
| `account:write` | 通过 API 更新账号公开资料 |
| `statistics:read` | 查询账号可见的下载统计 |
| `admin:users` | 管理用户账号及其登录设备 |
| `admin:repositories` | 管理仓库及重建仓库索引 |
| `admin:settings` | 管理系统设置与诊断功能 |
| `admin:audit` | 读取或清理管理员可见的行为与状态数据 |
| `admin:notifications` | 发送管理员通知 |
| `admin:updates` | 检查、上传、安装系统更新及重启服务 |
| `admin:statistics` | 查询系统级下载统计 |

只有系统管理员可以创建 `admin:*` Token；所属账号失去管理员身份后，这些范围也会立即停止授权。已有
`package:manage` 与 `domain:manage` Token 保持兼容，但新 Token 不再允许选择这两个宽泛范围。

## 使用 Token

API 自动化应使用 Bearer 认证：

```http
Authorization: Bearer rnp_pat_REDACTED
```

标准包客户端可以把同一个 Token 作为所属用户名的 Basic 密码。Basic 认证仅限包协议，不能调用管理 API。
Cargo 会把 Token 作为不带 Bearer 前缀的完整 `Authorization` 值发送，RenoP 仍会执行相同的范围检查。
Docker 先在 `/v2/token` 交换短期 Registry Token，其 pull、push 和 delete 动作会同时受 API Token 范围与
镜像团队权限限制。

管理员用户增删改查仍位于 `/api/tokens`，但管理员不能代替其他用户创建凭据。兼容接口
`POST /api/auth/profile/token` 仍可为当前登录账号额外创建一个不设有效期的发布 Token；新集成应使用细粒度接口。
