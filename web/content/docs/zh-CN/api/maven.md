---
title: Maven 存储库 API
order: 4
category: API 接口
description: 已验证发布域、域团队、制品目录与 Maven 客户端访问
---

# Maven 存储库 API

RenoP Maven 存储库使用已验证的反向域名命名空间。发布者只需在账号菜单中创建并验证一次域，即可在所有有权
操作的 Maven 存储库中使用。标准 Maven 2 路径、元数据、分离签名与校验文件保持 Maven 和 Gradle 兼容。

## 域验证

通过 `POST /api/maven/domains` 创建域。RenoP 返回高强度随机验证码和固定验证目标：

- DNS 命名空间在注册根域建立 TXT；系统读取全部 TXT 值并仅接受精确匹配；
- `io.github.<account>` 使用公开 GitHub 用户的 Bio 或公开组织的 Description；
- `io.gitlab.<account>` 使用公开 GitLab 用户的 Bio 或公开群组的 Description。

通过 `POST /api/maven/domains/:domain/verify` 发起外部验证。每个域每 5 秒最多验证一次。系统管理员可使用
`/verify/force` 强制通过，此操作会写入行为日志。

验证后的域及其团队在整个 RenoP 实例中共享。切换 Maven 存储库时无需重复创建、验证或邀请成员。

## 域权限

Maven 团队归属于全局域，而非某个存储库或单个制品：

- L0：读取公开内容；
- L1：发布制品；
- L2：管理版本与描述；
- L3：邀请和管理成员；
- L4：拥有并转移域。

单次邀请请求可包含 1 至 20 个用户名。非管理员添加成员时使用消息中心邀请。所有权转移始终保留唯一 L4
所有者，所有者必须先转移所有权才能退出。

## 制品目录

`GET /api/maven/repositories/:repo/domains` 列出在指定存储库中已有制品的域。
`GET /api/maven/repositories/:repo/packages` 提供分页搜索。
`GET /api/maven/repositories/:repo/package?group=...&artifact=...` 返回制品及版本。L2 成员可通过对应 JSON
接口更新描述或删除完整版本。

旧版 Maven 存储库会在升级时建立目录索引。迁移得到的域视为已验证，但不会自动添加成员；管理员必须显式
分配权限。已配置的 Maven 镜像继续解析缺失制品。

## 布局与纯文件存储库

现代 UI 默认使用域目录。管理员可切换到经典文件树，并可随时切回。此设置只改变显示方式：任意路径仍会被
拒绝，发布仍要求已验证域和有效 Maven 路径。

独立的 `files` 格式用于非结构化内容，支持覆盖、删除、S3 与镜像，但不生成校验文件或 POM，也不执行
OpenPGP 校验。

## Maven 与 Gradle 客户端访问

读取和发布使用 `/{repo}/{maven-path}`。可使用密码，或带有 `repository:read`、`repository:publish` 的
API Token。存储库可见性控制读取，已验证域及账号当前 L0-L4 控制修改。完整契约位于
`web/assets/openapi.yaml`。
