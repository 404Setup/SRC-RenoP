---
title: 发布配额
order: 18
category: API 接口
description: 用户与超级团队的周期发布限制
---

# 发布配额

发布配额限制本地上传的文件数量、总大小和完整项目发布次数。新安装默认每月允许 600 个文件、40 MiB 与
20 次发布。系统管理员可以修改全局默认值，也可以为单个用户或超级团队配置独立值。

## 策略

`period` 可设为 `day`、`week` 或 `month`，周期边界统一使用 UTC。只有对象独立配置允许将上限设为零，
此时对应操作会被禁止。仅管理员可配置的 `unlimited` 会停止该对象的配额消耗。提交空对象可恢复全部
全局默认值。

## 所有权

个人项目消耗发布用户的配额。绑定到超级团队的软件包或 Maven 发布域只消耗对应团队的配额。所有权转移
只影响后续发布，不迁移历史用量。来自上游镜像的下载与目录更新不会消耗发布配额。

## 用量统计

Cargo 与 npm 每个成功版本统计一个存储文件和一次完整发布。Docker 在提交 manifest 时统计 manifest、
配置与层描述符，并计为一次发布。Maven 对每个客户端 PUT 统计一个文件，在 POM 被接受时统计一次项目
发布。纯文件引擎的每个 PUT 统计一个文件和一次发布。服务端生成的索引与校验文件不会单独增加用量。

并发上传会先建立可过期的持久化预留。协议验证成功后预留转为已用额度；失败或遗留预留会被释放或由
定时清理任务移除。状态响应同时包含已用量与有效预留，避免并行请求共同突破上限。

## 接口

```http
GET /api/publication-quota
GET /api/publication-quota/users/{username}
PUT /api/publication-quota/users/{username}
GET /api/publication-quota/super-teams/{prefix}
PUT /api/publication-quota/super-teams/{prefix}
GET /api/settings/publication-quota
PUT /api/settings/publication-quota
```

当前用户可以读取自己的状态，超级团队成员可以读取团队状态。只有系统管理员可以读取其他用户、调整
独立配置、启用 `unlimited` 或修改全局默认值。

## 强制执行

配额耗尽时返回 `429 Too Many Requests`。`X-Renop-Error-Code` 使用
`publication_file_quota`、`publication_byte_quota` 或 `publication_count_quota` 标识具体限制。配额检查在
认证、存储库权限、软件包预创建、命名空间绑定与 Maven 域验证之后执行，不会赋予用户原本没有的权限。
