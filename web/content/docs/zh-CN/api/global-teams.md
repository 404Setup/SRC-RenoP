---
title: 超级团队
order: 12
category: API 参考
description: 不可变共享前缀、T1-T4 成员权限、邀请与账户上限
---

# 超级团队

超级团队是实例级协作身份。每个团队拥有一个不可变前缀，各存储引擎可引用该前缀，而无需把团队成员复制到软件包
成员列表。团队与成员关系在内部绑定不可变账户 ID，API 响应仅显示用户名。

## 角色与所有权

角色权限逐级累积。T1 按软件包可见性提供读取权限；T2 可发布和维护版本；T3 可管理 T1/T2 成员并以团队身份创建
软件包；T4 拥有团队配置，并可授予 T3/T4。

团队必须始终保留至少一位 T4。T3 不能修改其他 T3/T4，也不能授予这两个角色。系统管理员无需加入即可管理所有
团队，但管理员添加成员时仍会检查目标账户的加入上限。管理员添加自己时不会产生无意义的消息。

## 上限

全局默认值存放在 `super_teams.create_limit` 与 `super_teams.join_limit`，默认分别为创建五个团队、加入二十个团队。
自己拥有的团队会同时计入两种用量。

当前账户通过 GET /api/super-teams/limits 读取有效上限和用量。管理员通过 GET
/api/super-teams/users/{username}/limits 与 PUT /api/super-teams/users/{username}/limits 读取或修改个人覆盖值。
`-1` 表示继承全局值，零表示禁止对应操作。全局值通过 GET /api/settings/super-teams 与 PUT
/api/settings/super-teams 配置。

## 团队生命周期

GET /api/super-teams 返回按前缀排序的分页结果。普通账户只能看到自己加入的团队，系统管理员可查看全部团队。
POST /api/super-teams 保留前缀，并将创建者设为 T4。前缀长度为 2–64，只能包含小写字母、数字、连字符或下划线，
首尾必须为字母或数字，创建后不可修改。

GET /api/super-teams/{prefix} 返回团队资料与仅含用户名的成员列表。PUT /api/super-teams/{prefix} 修改名称和描述。
DELETE /api/super-teams/{prefix} 删除团队，并在同一事务中取消待处理邀请。

## 成员流程

POST /api/super-teams/{prefix}/members 接受一至二十个用户名和一个 T1-T4 角色。普通团队管理员会创建七天有效、只能
处理一次的消息中心邀请；系统管理员则立即添加有效账户。

PUT /api/super-teams/{prefix}/members/{username} 修改角色。DELETE /api/super-teams/{prefix}/members/{username}
移除成员或退出团队。POST /api/super-teams/invitations/{id}/{decision} 接受 `accept` 或 `reject`；并发或重复响应不会
让同一邀请生效两次。

## API Token 边界

超级团队路由要求 `team:manage`，精确目标使用 `global/{prefix}`。账户上限读取要求 `account:read`，个人覆盖接口要求
`admin:users`，全局设置要求 `admin:settings`。带精确目标的 Token 无法列出全部团队，也不能创建目标外的前缀。

管理失败会返回稳定的 `X-Renop-Error-Code` 与受限的通用正文。客户端应依据 HTTP 状态和已登记错误码处理，不应直接
显示响应文本。
