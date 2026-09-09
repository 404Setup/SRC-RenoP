---
title: Maven 仓库 API
order: 4
category: API 参考
description: 已验证发布域、域团队、制品目录与 Maven 客户端访问
---

# Maven 仓库 API

RenoP Maven 仓库使用已验证的反向域名命名空间。发布者只需在账号菜单中创建并验证一次域，即可在所有有权
操作的 Maven 仓库中使用。标准 Maven 2 路径、元数据、分离签名与校验文件保持 Maven 和 Gradle 兼容。

## 域验证

通过 `POST /api/maven/domains` 创建域。RenoP 返回高强度随机验证码和固定验证目标：

- DNS 命名空间在注册根域建立 TXT；系统读取全部 TXT 值并仅接受精确匹配；
- `io.github.<account>` 使用公开 GitHub 用户的 Bio 或公开组织的 Description；
- `io.gitlab.<account>` 使用公开 GitLab 用户的 Bio 或公开群组的 Description。

通过 `POST /api/maven/domains/:domain/verify` 发起外部验证。每个域每 5 秒最多验证一次。系统管理员可使用
`/verify/force` 强制通过，此操作会写入行为日志。

验证后的域及其团队在整个 RenoP 实例中共享。切换 Maven 仓库时无需重复创建、验证或邀请成员。

近期的 GitLab.com 绑定也可自动验证新命名空间：账号必须拥有个人命名空间或顶层群组，且证明在最近一小时内取得。公开群组成员资格、子群组所有权和自托管实例均不足以证明。详见[第三方登录](../security/oauth-login.md)。

## 域权限

Maven 团队归属于全局域，而非某个仓库或单个制品：

- L0：读取公开内容；
- L1：发布制品；
- L2：管理版本与描述；
- L3：邀请和管理成员；
- L4：拥有并转移域。

单次邀请请求可包含 1 至 20 个用户名。非管理员添加成员时使用消息中心邀请。所有权转移始终保留唯一 L4
所有者，所有者必须先转移所有权才能退出。

## 制品目录

`GET /api/maven/repositories/:repo/domains` 列出在指定仓库中已有制品的域。
`GET /api/maven/repositories/:repo/packages` 提供分页搜索。
`GET /api/maven/repositories/:repo/package?group=...&artifact=...` 返回制品及版本。L2 成员可通过对应 JSON
接口更新描述或删除完整版本。

详情响应会汇总已索引主要文件、大小、修改时间、可用校验和与分离签名覆盖情况；每个版本最多返回 64 个
主要文件。若最新已索引 POM 不超过 2 MiB，RenoP 会以流式方式解析，并返回项目、组织、许可证、开发者、
源代码管理、问题跟踪、父项目及直接依赖信息。直接依赖最多返回 128 项；校验和与签名伴随文件不计入主要文件。

域内 L2-L4 成员或管理员可通过制品更新接口维护独立的包级 Markdown README。README 上限为 512 KiB，
只在详情接口中返回，并通过共用的元素与 URL 白名单渲染。POM 或目录中的短描述仍是独立字段。

旧版 Maven 仓库会在升级时建立目录索引。迁移得到的域视为已验证，但不会自动添加成员；管理员必须显式
分配权限。已配置的 Maven 镜像继续解析缺失制品。

## 布局与纯文件仓库

现代 UI 默认使用域目录。管理员可切换到经典文件树，并可随时切回。此设置只改变显示方式：任意路径仍会被
拒绝，发布仍要求已验证域和有效 Maven 路径。

独立的 `files` 格式用于非结构化内容，支持覆盖、删除、S3 与镜像，但不生成校验文件或 POM，也不执行
OpenPGP 校验。

## Maven 与 Gradle 客户端访问

读取和发布使用 `/{repo}/{maven-path}`。可使用密码，或带有 `repository:read`、`repository:publish` 的
API Token。仓库可见性控制读取，已验证域及账号当前 L0-L4 控制修改。完整契约位于
`web/assets/openapi.yaml`。

## 资源锁定

仓库管理员和所属仓库的版主通过有效的浏览器会话 Cookie 管理包或版本锁定：

- `PUT /api/maven/repositories/{repo_name}/package/locks?group={group}&artifact={artifact}`
- `DELETE /api/maven/repositories/{repo_name}/package/locks?group={group}&artifact={artifact}`

```json
{"version":"1.2.3","mode":"read","reason":"trojan"}
```

省略 `version` 或使用空字符串表示锁定整个包。DELETE 接受 `{"version":"1.2.3"}`，仅移除对应的手动锁定，系统锁定独立保留。API Token 不能管理锁定。界面会本地化显示公开原因：`hold`、`prohibited`、`expired`、`trojan`、`abuse`、`dmca`、`reup`、`squatting`、`quality`。

两种模式都会冻结写入。`write` 保留已存储文件的下载；`read` 还会将元数据限制为管理员、所属仓库版主、发布域所有者与协作者（包括 L0），以及包或发布域所绑定超级团队的成员可见。任何人都不能下载文件内容、POM、签名或校验文件。有权限的人员仍可查看目录详情和解析后的 `maven-metadata.xml`；其他人不能通过目录、搜索、文件树、版本 API 或统计发现受限的包或版本。共享 XML 元数据会移除隐藏版本并调整 latest/release，存储中的原文件不变。

锁定覆盖任意附属文件及带时间戳的 SNAPSHOT 文件。冻结文件不会因缓存到期而删除，也不会从镜像重新填充。版本被锁定时，共享元数据不能被覆盖；整个包的永久弃用及仓库重配置、删除也会被阻止。详情返回公开的 `locks`、`moderator`、`member`、`version_locked` 状态。受限写入返回 `423` 和 `X-Renop-Error-Code: resource_locked`；受限读取返回 `404`。

版本删除会在修改存储前校验每个坐标段，拒绝路径分隔符和点目录别名。

全局团队锁定由其绑定的包和发布域继承，发布域响应包含公开的 `locks`。发布域锁定会冻结验证、成员变更、所有权转移、关闭和新发布，覆盖未入索引的文件与元数据。读取锁定保留已有协作者和有权版主的元数据访问，禁止所有文件内容读取。独立验证的子发布域保持自身权限。团队成员关系与发布域权限会保留，并在全部适用限制解除后恢复。
