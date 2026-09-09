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

近期的 GitLab.com
绑定也可自动验证新命名空间：账号必须拥有个人命名空间或顶层群组，且证明在最近一小时内取得。公开群组成员资格、子群组所有权和自托管实例均不足以证明。详见[第三方登录](../security/oauth-login.md)。

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

省略 `version` 或使用空字符串表示锁定整个包。DELETE 接受 `{"version":"1.2.3"}`，仅移除对应的手动锁定，系统锁定独立保留。API
Token 不能管理锁定。界面会本地化显示公开原因：`hold`、`prohibited`、`expired`、`trojan`、`abuse`、`dmca`、`reup`、`squatting`、
`quality`。

两种模式都会冻结写入。`write` 保留已存储文件的下载；`read` 还会将元数据限制为管理员、所属仓库版主、发布域所有者与协作者（包括
L0），以及包或发布域所绑定超级团队的成员可见。任何人都不能下载文件内容、POM、签名或校验文件。有权限的人员仍可查看目录详情和解析后的
`maven-metadata.xml`；其他人不能通过目录、搜索、文件树、版本 API 或统计发现受限的包或版本。共享 XML 元数据会移除隐藏版本并调整
latest/release，存储中的原文件不变。

锁定覆盖任意附属文件及带时间戳的 SNAPSHOT 文件。冻结文件不会因缓存到期而删除，也不会从镜像重新填充。版本被锁定时，共享元数据不能被覆盖；整个包的永久弃用及仓库重配置、删除也会被阻止。详情返回公开的
`locks`、`moderator`、`member`、`version_locked` 状态。受限写入返回 `423` 和 `X-Renop-Error-Code: resource_locked`；受限读取返回
`404`。

版本删除会在修改存储前校验每个坐标段，拒绝路径分隔符和点目录别名。

全局团队锁定由其绑定的包和发布域继承，发布域响应包含公开的 `locks`
。发布域锁定会冻结验证、成员变更、所有权转移、关闭和新发布，覆盖未入索引的文件与元数据。读取锁定保留已有协作者和有权版主的元数据访问，禁止所有文件内容读取。独立验证的子发布域保持自身权限。团队成员关系与发布域权限会保留，并在全部适用限制解除后恢复。

系统管理员和全局版主可使用有效的浏览器会话，管理发布域在所有仓库中的锁定：

- `PUT /api/maven/domains/{domain}/locks`
- `DELETE /api/maven/domains/{domain}/locks`

```json
{"mode":"write","reason":"hold"}
```

PUT 使用下方请求体；DELETE 使用 `{}`，成功均返回 `204`。API 令牌和仅有仓库权限的版主不能管理全局发布域。解除人工域锁后，系统锁和继承的团队锁仍然生效。发布域详情通过
`moderator` 表示当前请求的管理权限；公开页、账号页和仓库内的发布域页面共用同一控件。仓库更新、迁移和删除也会检查仅存在于磁盘或文件索引中的锁定命名空间。

## 发布域状态与赎回

RenoP 使用 [IANA 引导数据](https://data.iana.org/rdap/dns.json)通过 RDAP 检查注册域。
每分钟最多检查 16 个到期任务；正常结果间隔六小时检查，查询不可用时在 15 分钟后重试。

| 注册状态                                                                              | 公开锁定原因 |
|---------------------------------------------------------------------------------------|--------------|
| `serverHold`、`clientHold`                                                            | `hold`       |
| `pendingDelete`、`restorable`、`redemptionPeriod`、`pendingRestore`，或已超过到期时间 | `expired`    |
| `clientRenewProhibited`、`serverRenewProhibited`、`renew prohibited`                  | `prohibited` |

同时识别 RDAP 中以空格分隔的等价状态。查询故障不会解除限制，也不会延长已知到期时间。
注册局查询不可用时无法完成新的状态检查；所有权验证及管理员批准认领均要求状态正常。

对于 `io.github.<account>` 和 `io.gitlab.<account>`，RenoP 检查对应公开账号，不查询注册域到期时间。
验证时保存不可变数字 ID 和个人或组织／群组类型。账号不存在时冻结发布；
即使名称相同，ID 或类型变化也按争议处理。未保存 ID 的旧版已验证账号命名空间需要重新提供证明才能恢复权限。

状态锁冻结发布域及其内容，保留下载和成员记录。首次锁定会更换验证码。
外部账号或域名恢复后，当前 L4 所有者须发布新的证明并点击 **申请赎回**，
使用有效的 `renop_session` Cookie 调用 `POST /api/maven/domains/{domain}/redeem`。
每个域每五秒最多尝试一次。后台检查恢复正常不会自动恢复控制权。
赎回只解除状态锁，独立的人工锁、系统锁和团队锁仍然生效。
发布域响应中的 `health` 包含状态、检查与到期时间、提供商身份、锁定时间和释放时间。

## 释放后的发布域旧包

安全锁默认保留名称两个日历年。[发布域设置](settings.md)支持配置 1–100 个月或年，
修改仅影响之后的新锁定。主动关闭仍采用独立的 31 天保留期。
释放后，新认领者须证明所有权并获得管理员批准；批准时会重新检查外部状态。

历史包（包括原先未登记到目录的索引文件）在重新认领后仍可下载，但禁止写入。
发布域认领获批后，可以发布新的包名。当前发布域所有者可在旧包页面申请恢复发布权限，
对应 `POST /api/tickets/maven-restorations`，由仓库版主或系统管理员批准。
批准时重新核对当前认领和所有权，仅恢复该包，并将其绑定到发布域当前所属团队。
镜像包和永久弃用的包不能恢复。参见[审核 API](reviews.md)。

认领准备会流式读取磁盘或 S3 元数据，时限为一分钟。存储错误或超时不会改变当前认领状态。
待审核的发布继续保留原审核流程，不会被导入公共目录。
