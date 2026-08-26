---
title: API Token 与 GPG 签名
order: 2
category: 安全与权限
description: 细粒度 API Token 与 OpenPGP 签名验证
---

# API Token 与 GPG 签名

为了支持 CI/CD 自动化与制品完整性校验，RenoP 提供细粒度 API Token 与 GPG 签名管理。

## 1. 细粒度 API Token

用户可在个人资料中创建、查看和撤销 API Token。每个 Token 具有私有名称、一个或多个能力范围以及可选
有效期。256 位随机密钥只显示一次，数据库仅保存其 SHA-256 查询摘要。

请为每台工作站或自动化流水线选择满足用途的最小范围和最短有效期。请求只有在 Token 含有所需范围，且
所属账号当前仍拥有对应的系统、仓库、发布域或包团队权限时才会通过。管理员范围不会在账号失去管理员
身份后继续授权，撤销操作也会立即清除认证缓存。

浏览器会话密钥仅可通过 HttpOnly `renop_session` Cookie 使用；Basic 凭据仅可用于标准包协议。API 自动化
应使用 `Authorization: Bearer <token>`。RenoP 不接受 URL 查询参数中的凭据或
`Authorization: Session`。旧版明文上传 Token 会自动迁移为仅含 `repository:read` 与
`repository:publish` 的哈希凭据。

---

## 2. GPG 分离签名与校验机制

在 Maven 制品分发中，为了防止制品在传输或镜像过程中被恶意篡改，通常会为 `.jar` 和 `.pom` 文件生成独立的 `.asc` 签名。

### 开启强制 GPG 签名校验

在 `repositories.yaml` 中将仓库的 `require_gpg_signature` 设为 `true`：

```yaml
repositories:
  releases:
    name: releases
    require_gpg_signature: true
```

### 签名验证工作流程

1. 开发者或 CI 工具上传制品包（如 `mylib-1.0.0.jar`）。
2. 在尚未收到对应的 `.asc` 签名文件前，RenoP 将该制品放入隔离队列（磁盘上的 `.renop.tmp.gpg` 目录），对外隐藏不可见。
3. 随后上传对应的签名文件 `mylib-1.0.0.jar.asc`。
4. RenoP 自动从预先配置的 OpenPGP 密钥服务器（如 `keys.openpgp.org`）或用户已登记的公钥中拉取签名公钥，校验签名有效性。
5. 验签成功后，制品与签名文件自动从隔离队列转移至正式仓库目录中供外界下载；验签失败则拒绝发布。

### 登记个人 GPG 公钥

用户可以在 Web 控制台的「个人资料」->「GPG 密钥」页面中粘贴自己的 OpenPGP 公钥文本，RenoP 将优先使用该公钥进行验签。
