---
title: 访问令牌与 GPG 签名
order: 2
category: 安全与权限
description: 个人访问令牌 (PAT)、上传令牌与 OpenPGP 签名验证
---

# 访问令牌与 GPG 签名

为了支持 CI/CD 流水线自动化与制品完整性校验，RenoP 提供了独立的访问令牌机制与 GPG 签名管理。

## 1. 访问令牌类型

在 Web 管理控制台的「令牌管理」页面中，可以创建两种类型的令牌：

### 个人访问令牌 (PAT)

- 与特定用户绑定，继承该用户的权限范围。
- 适合开发者本地构建工具（如 Maven `settings.xml`、Cargo `credentials.toml` 或 Docker CLI）使用，避免直接暴露登录主密码。
- 支持设置过期时间，可随时在控制台吊销。

### 上传令牌 (Upload Token)

- 专门用于自动化构建工具与 CI/CD 流水线（如 GitHub Actions、GitLab CI、Jenkins）。
- 可精确限制仅允许向指定的单个或多个仓库推送制品，禁止执行读取私有仓库或调用管理 API 等其他操作。

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
