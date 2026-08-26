---
title: API Token 与用户
order: 3
category: API 接口
description: 细粒度 API Token 生命周期、认证边界与管理员用户接口
---

# API Token 与用户

API Token 是归属于单个账号的长期机器凭据。RenoP 只存储 256 位随机密钥的 SHA-256 查询摘要。明文仅在
创建成功时返回一次，之后不可恢复。

每次请求必须同时通过两项独立检查：

- Token 包含接口要求的能力；
- 所属账号当前仍有权对目标资源执行该操作。

因此，账号角色、存储库权限或团队成员关系变更无需重新创建 Token 即可生效。

## 管理自己的 API Token

Token 管理接口只接受 HttpOnly `renop_session` 浏览器 Cookie。API Token、密码、
`Authorization: Session` 与 URL 查询参数均不可管理凭据密钥。

### 查询可分配权限

`GET /api/auth/profile/api-tokens/scopes`

响应按照账号当前权限过滤，普通账号不会获得管理员权限项。

```json
{
  "scopes": ["repository:read", "repository:publish", "package:metadata"],
  "target_kinds": {
    "repository:read": "repository",
    "repository:publish": "repository",
    "package:metadata": "package"
  },
  "target_limit": 128
}
```

### 创建 Token

`POST /api/auth/profile/api-tokens`

```json
{
  "name": "CI publishing",
  "scopes": ["repository:read", "repository:publish"],
  "targets": {
    "repository:publish": ["releases"]
  },
  "expires_at": 1798761600000
}
```

`expires_at` 为可选 Unix 毫秒时间，范围为创建后 5 分钟至 5 年；省略或传入 null 表示 Token 本身不过期。
每个账号最多拥有 50 个 API Token。

`targets` 可分别限制每项权限。未出现在 `targets` 中的权限可用于账号当前有权访问的全部目标。存储库目标
直接使用精确名称；包目标使用 `repository/package`，Maven 可使用
`maven-releases/com.example/library`；团队目标使用 `package/repository/package` 或 `domain/example.com`；
域目标使用规范化域名。单个 Token 最多包含 128 个目标。

目标限制始终与存储库权限及团队当前 L0-L4 权限取交集，不会产生权限提升。

创建成功返回 `201 Created` 与 `Cache-Control: no-store`：

```json
{
  "token": {
    "id": "07cdcf2e-0828-4a29-9817-cf771cc9fb0a",
    "name": "CI publishing",
    "scopes": ["repository:publish", "repository:read"],
    "targets": {"repository:publish": ["releases"]},
    "created_at": 1787731200000,
    "expires_at": 1798761600000
  },
  "secret": "rnp_pat_EXAMPLE_REDACTED_COPY_THE_REAL_VALUE_ONCE"
}
```

### 查询 Token 元数据

`GET /api/auth/profile/api-tokens` 只返回非敏感元数据与账号上限，不包含 Token 明文。

### 撤销 Token

`DELETE /api/auth/profile/api-tokens/{token_id}` 返回 `204 No Content`，并立即清除认证缓存。

## 权限参考

| Scope | 能力 |
|:------|:-----|
| `repository:read` | 读取存储库目录、元数据、文件、镜像与版本 |
| `repository:publish` | 通过 Maven、Cargo、Docker、files 或分块上传发布 |
| `repository:delete` | 删除文件、版本、标签或镜像 |
| `package:create` | 通过存储库授权后创建 Cargo 包或 Docker 镜像 |
| `package:metadata` | 更新包描述及其他元数据 |
| `package:lifecycle` | 归档、恢复、yank 或 unyank 包与版本 |
| `team:manage` | 查看和管理 Cargo、Docker 与 Maven 域团队及邀请 |
| `domain:read` | 读取 Maven 域私有配置 |
| `domain:create` | 创建 Maven 域 |
| `domain:verify` | 验证或强制验证 Maven 域 |
| `domain:delete` | 删除 Maven 域 |
| `messages:read` | 读取、标记和删除账号消息 |
| `account:read` | 读取账号私有数据与个人行为日志 |
| `account:write` | 通过 API 更新公开个人资料 |
| `statistics:read` | 查询账号有权查看的下载统计 |
| `admin:users` | 管理用户账号及登录设备 |
| `admin:repositories` | 管理存储库与重建索引 |
| `admin:settings` | 管理系统设置与诊断 |
| `admin:audit` | 读取或清理管理员行为与状态数据 |
| `admin:notifications` | 编写管理员通知 |
| `admin:updates` | 检查、上传、安装更新及重启 |
| `admin:statistics` | 查询系统级统计 |

只有管理员可以创建 `admin:*` 权限；所属账号失去管理员角色后，权限会立即失效。已有 Token 的
`package:manage` 与 `domain:manage` 保持兼容，但新 Token 不再允许选择。

## 使用 Token

调用有权访问的管理 API 时使用 Bearer：

```http
Authorization: Bearer rnp_pat_REDACTED
```

标准包客户端可将同一个 Token 作为所属用户名的 Basic 密码。Basic 仅限包协议。Cargo 将 Token 作为完整
`Authorization` 值发送；Docker 通过 `/v2/token` 换取短期 Token，其中只包含 API Token 权限与镜像权限
共同允许的动作。

## 兼容接口

管理员用户增删改查仍位于 `/api/tokens`，但管理员不能代替其他用户创建凭据。旧接口
`POST /api/auth/profile/token` 仍可为当前账号创建一个不设有效期的额外发布 Token；新集成应使用细粒度
个人资料接口。
