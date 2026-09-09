---
title: 工单 API
order: 13
category: API 参考
description: 工单将反馈、建议、举报、所有权转让与发布审批合并为持久化流程。工单中心取代原审核中心，保留已有任务 ID 与待发布内容。
---

# 工单 API

工单将反馈、建议、举报、所有权转让与发布审批合并为持久化流程。工单中心取代原审核中心，保留已有任务 ID 与待发布内容。

## 适用范围与凭据

所有接口都要求有效的浏览器 `renop_session` Cookie。Basic 凭据、Bearer API Token 和不带该 Cookie 的会话令牌均被拒绝。入口为 `/account/tickets`，旧 `/account/reviews` 链接会跳转至此。

仓库版主可查看其范围内所有状态的工单，包括团队审批阶段；系统管理员可查看所有范围。T3/T4 团队成员只处理分配给本团队的转让和创建流程，团队审批仍先于仓库审批。申请记录关联不可变账号身份。

有权查看工单的处理人员可以互相看到身份。申请人不会收到 `assignee`、`escalated_by` 或 `decided_by`。被举报账号即使是管理员，也不能读取或处理针对自己的举报；被举报对象不会获得举报者身份或工单访问权限。

待处理提醒跟随当前审批阶段。最终结果通过消息，以及启用时的邮件，按申请人账号语言通知申请人。只有举报以 `upheld` 完成时才另行通知被举报对象；通知只含资源与结果，不含工单 ID、举报者、处理人员或私密正文。驳回与撤回不通知被举报对象，原有邮件场景标识保持兼容。

举报审计事件不记录账号、处理人、会话与 IP 身份；处理人员的身份归属保留在有权访问的工单内。

## 提交反馈、建议或举报

POST /api/tickets 接受 `kind`（`feedback`、`suggestion` 或 `report`）、`title`（1–160 字符）、`body`（1–8000 字符）及可选的 `repository`。JSON 正文上限 48 KiB。每个账号最多有 16 个待处理支持工单，每 24 小时最多新建 24 个；全部流程共享 4096 个待处理任务上限。

举报还需 `target`：`format`、`repository`、`name` 及可选的 `version`。类型支持 `user`、`superteam`、`maven-domain`、`maven`、`cargo`、`npm`、`docker`。全局资源不填仓库。包举报必须匹配仓库格式并满足当前读取权限，可举报其他用户、可见的包或版本、发布域及团队；不能举报自己的资源、隐藏资源，也不能重复提交相同的待处理举报。

创建返回 `201`、工单及 `Location`，举报对象的所有者按不可变账号 ID 记录。记录 `upheld` 不会自动封禁账号或锁定包；记录处罚成立前，应先使用现有管理功能执行相应处理。

```json
{"kind":"report","title":"Package report","body":"Please investigate this version.","target":{"format":"npm","repository":"npm","name":"@platform/tool","version":"1.0.0"}}
```

## 接管、升级与处理

POST /api/tickets/{id}/action 接受 `action`：`claim`、`release`、`escalate`、`process`、`complete` 或 `close`。处理和审批前必须接管工单，原子接管后其他人员仍可查看但不能处理。系统管理员可以用 `force: true` 接管版主占用的工单，但不能抢占尚未释放工单的另一位系统管理员。

升级会释放工单并限定下一位接管人为系统管理员；管理员也可升级给另一位管理员。每个工单最多升级三次，第三次升级后的接管人必须完成处理，不能再释放、升级或被强制接管。团队批准后会释放接管状态，等待仓库阶段的处理人接管。

支持工单的 `process` 记录已处理状态但不发送最终通知，`complete` 完成工单，`close` 关闭工单。这些操作都需要 1–4096 字符的 `response`。反馈和建议的 `outcome` 为 `resolved`，举报为 `upheld` 或 `dismissed`，关闭记录为 `closed`。操作 JSON 上限 24 KiB。发布与转让需接管后使用下方决策接口。

GET /api/tickets/{id} 返回有权查看的详情，以及根据当前权限和接管状态计算的 `actions` 数组，界面据此显示操作。接管冲突返回 `409`，错误码为 `ticket_claim_required`、`ticket_occupied` 或 `ticket_escalation_limit`。

## 转让规则

申请人必须具有项目或发布域的有效 L4 所有权，或当前拥有仓库/系统管理权限。转让到超级团队时，申请人
还必须是目标团队成员。审核由该团队 T3/T4 管理员或系统管理员完成；申请人自身具备审核权限时也可以处理。

转让只修改所有权绑定，不会复制或删除包级成员。系统不接受两个团队之间的直接转让：应先将符合条件
的项目转回个人所有，再提交新的转入申请。

带命名空间的 Docker 镜像和带作用域的 npm 软件包使用了团队的不可变前缀，因此不能转回个人所有。
来自镜像源的资源也不能转让。

## 发布规则

Maven 仓库可以关闭审核、仅审核新建制品的第一个版本，或审核每个版本。启用审核后会关闭重新部署。
本地文件会先写入存储，但在仓库版主或系统管理员作出决定前不会进入公开索引。镜像下载不进入审核。

要求分离式 GPG 签名时，系统会先完成签名验证，再创建发布审核。同一版本的校验和、签名和 Maven 元数据
会合并到同一任务。最后一个文件到达后的五秒静默期内不能处理任务，以免客户端仍在上传。版本批准后会
被封存，不能继续添加文件。

npm 的 T2 成员始终先等待团队 T3/T4 批准，且不占用名称。若仓库启用了创建审核，同一任务会继续转交版主；
否则团队批准会原子创建软件包。最终批准会重新检查仓库权限与当前团队成员资格，再向申请人授予 L4。
`new_packages` 模式下，后续版本正常发布；`every_version` 模式还会隐藏
每个 tarball，并在批准前保留有界的 manifest 与 dist-tag 载荷。镜像内容不创建审核任务。

Cargo 发布会先保存并隐藏 crate 归档，不修改 sparse index 与公共目录。批准时先向两处元数据写入不可变版本，
再开放归档；拒绝时删除隐藏归档。`new_packages` 策略下，首个可见版本获批前，该 crate 仍视为新包。镜像获取的
crate 不进入审核流程。

Docker 的 T2 创建申请与 npm 一样，依次经过团队阶段和可选的仓库阶段。最终批准会重新检查本地与上游名称冲突、
仓库权限和当前团队成员资格，再预占镜像。`new_packages` 模式下，后续 Manifest 正常发布；`every_version` 模式会将每个
Manifest 的
原始字节保存为有界虚拟文件，批准前不写入引用和标签。审核相同 Digest 的新标签不会隐藏已有标签，Manifest、
Blob 关联、标签和审核结果在同一事务中写入。镜像源导入不进入审核流程。

## 查询任务

GET /api/tickets 返回有界分页。`view` 为 `reviewer` 或 `requested`；`status` 为 `unprocessed`（默认）、`in_progress`、`processed`、`closed`、`completed` 或 `all`。`limit` 为 1–100，`offset` 不得为负。逗号分隔的 `types` 支持原流程资源类型以及 `support`、`user`、`superteam`、`maven-domain`、`maven`、`cargo`、`npm`、`docker`。

`ticket_status` 表示统一工单状态，原 `status` 保留流程结果：`pending`、`approved`、`rejected` 或 `cancelled`。历史任务首次接管时写入工单状态。

响应包含 `tasks`、`total`、`limit`、`offset` 和最终使用的 `view`。每个任务保留来源、目标与当前审核团队前缀、
申请人显示名称、时间、当前状态及已完成的决策信息。非空 `review_team_prefix` 表示任务由该团队 T3/T4 处理；
T2 创建申请获得团队批准后会清除此字段，并以 `pending` 状态继续等待仓库审核。
发布任务还包含 `resource_version`、`file_count`、`total_size` 和最后文件时间。
npm/Docker 显式创建任务使用保留的 `resource_version` 值 `@create`，并通过同一文件 API 提供有界 JSON 申请。

## 提交转让

POST /api/tickets/super-team-transfers 接受 `resource_type`、`repository`、`resource_key` 和
`target_team_prefix`。Maven 发布域不传 `repository`；Maven 制品的资源键采用 `groupId:artifactId`。
目标团队为空表示申请转回个人所有。

同一资源同时只能有一项所有权转让处于待审核状态，与申请转入的目标无关。创建成功返回 `201 Created`、任务
正文及其 API 位置。

## 审核文件

GET /api/tickets/{id}/files 最多返回 256 个仓库相对路径，并包含稳定文件标识、大小、上传时间和关键文件
标记。GET /api/tickets/{id}/files/{file_id} 流式返回一个被隐藏的文件。只有申请人、当前审核团队 T3/T4、对应仓库版主或系统管理员使用浏览器会话时可以访问。

网页审核中心最多使用四个自适应下载任务，每次失败会重试两次。全部成功后，浏览器会按照标准仓库路径
生成 ZIP。仍有文件失败时，系统会分别打开关键文件，不会提供内容不完整的压缩包。

## 处理或取消

当前处理人必须已接管此阶段的工单。POST /api/tickets/{id}/decision 接受 `approved` 或 `rejected`。批准 T2 创建申请时，若仍需仓库审核，会返回同一
`pending` 任务并清空 `review_team_prefix`；否则直接完成创建。拒绝转让时必须提供不超过 512 个字符的理由。
拒绝发布时必须提供 `reason_code`，可选值为 `invalid_metadata`、`quality`、`policy_violation`、`copyright`、
`malware` 或 `custom`；自定义理由最多 505 个字符。批准时会先写入对应引擎的版本元数据再开放文件，拒绝时会删除隐藏文件。

DELETE /api/tickets/{id} 允许申请人撤回待处理的支持、所有权转让或 Maven 恢复申请，关闭工单并阻止后续决策。发布任务不能通过此接口取消，并发最终决策不能重复修改资源。

## 错误处理

失败响应通过 `X-Renop-Error-Code` 提供稳定错误码。`400` 表示筛选、资源标识或决策无效；`403` 表示
缺少所有权、目标团队成员资格或审核权限；`404` 表示任务或文件不存在；`409` 表示已有相同申请、任务已经
完成、所有权已变化、转让受到限制，或发布仍在接收文件。

客户端应翻译已注册的错误码，不应直接显示响应正文。

## 恢复重新认领的 Maven 包

`POST /api/tickets/maven-restorations` 要求当前发布域 L4 所有者的有效 `renop_session` Cookie：

```json
{"resource_type":"maven_artifact","repository":"releases","resource_key":"com.example:demo"}
```

成功返回 HTTP `201` 和状态为 `pending` 的 `maven_restore` 任务；已有相同待处理申请时返回 `409`。
对应仓库版主和系统管理员可见此任务，使用已有的决策和取消接口。
批准会在事务中核对认领状态、有效所有权和独立锁定，再恢复发布权限及当前发布域团队绑定。
认领状态变化会取消过期申请；拒绝或取消申请会保留包的限制和下载。
每个账号最多有 64 个待处理申请，全局最多 4096 个。此类任务不提供审核文件包下载。
