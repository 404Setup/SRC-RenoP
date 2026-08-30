---
title: npm 存储库
order: 4
category: 使用指南
description: 预留软件包，并通过 npm、pnpm、Yarn 或 Bun 使用 RenoP
---

# npm 存储库指南

先创建格式为 `npm` 的存储库，再从存储库页面预留每个软件包，然后才可发布。RenoP 不允许客户端在发布时
隐式创建软件包名称。以下示例使用 `javascript` 存储库和 `@example/library` 软件包。

## 配置客户端

创建具有存储库读取与发布权限的可过期 API Token。只有自动化流程确实需要时，才授予软件包生命周期或团队
管理权限。若此存储库用于全部软件包，可在项目或用户级 `.npmrc` 中写入：

```ini
registry=https://packages.example.com/javascript/
//packages.example.com/javascript/:_authToken=${RENOP_NPM_TOKEN}
always-auth=true
```

若只需将一个作用域交给 RenoP，请保留默认存储库并单独配置该作用域：

```ini
@example:registry=https://packages.example.com/javascript/
//packages.example.com/javascript/:_authToken=${RENOP_NPM_TOKEN}
always-auth=true
```

除受信任的本地开发网络外，应始终使用 HTTPS。自动化流程应使用 API Token；账号密码仅用于标准软件包协议认证。

## 准备并发布软件包

预留名称必须与 `package.json` 中的 `name` 完全一致。版本必须符合语义化版本规范，成功发布后不可修改。

```json
{
  "name": "@example/library",
  "version": "1.0.0",
  "description": "Example library",
  "publishConfig": {
    "registry": "https://packages.example.com/javascript/"
  }
}
```

可通过任意兼容客户端发布和安装：

```bash
npm publish
npm install @example/library
pnpm add @example/library
yarn add @example/library
bun add @example/library
```

RenoP 会验证大小受限的 gzip tarball，确认 `package/package.json` 与请求一致，计算 npm 兼容的 SHA-1 与 SHA-512
完整性值，并且只在全部验证通过后提交归档。

启用任一发布审核策略后，创建软件包会返回 `202 Accepted`，且批准前不会占用名称。`new_packages` 模式下，
后续 `npm publish` 正常执行；`every_version` 模式还会将每次发布送审，批准前不会出现在 packument 与 tarball
路由中。存储库版主或系统管理员会一并审核不可变 manifest、dist-tag 与 tarball。

## 可见性与软件包团队

公开软件包遵循存储库可见性。私有软件包必须使用作用域，并要求明确的软件包成员关系或管理员权限。L0 可读取，
L1 可发布版本，L2 可管理版本与元数据，L3 可管理软件包团队，L4 拥有软件包。移除或降级成员时，不得使软件包
失去最后一位 L4 所有者。

可在软件包页面邀请已有 RenoP 账号。邀请是通知中心中的持久操作。镜像软件包不设本地团队，前端会标明其上游
来源，并且始终只读。

## 发布标签、弃用与取消发布

使用标准 npm 命令管理发布标签和弃用元数据：

```bash
npm dist-tag add @example/library@1.0.0 stable
npm deprecate @example/library@1.0.0 "Use version 2"
npm unpublish @example/library@1.0.0
```

取消发布会为版本建立墓碑并删除 tarball，但版本号不可复用。删除软件包会为全部版本建立墓碑，并继续保留软件包名称。

## 上游镜像

npm 存储库可按顺序代理上游存储库。可用精确软件包名称和 `@scope/*` 规则约束镜像范围。RenoP 会限制 packument
大小、合并并发刷新、将上游 tarball 地址替换为本地地址，并从本地目录移除上游已删除的版本。通过镜像发现的软件包
不能接受本地推送。
