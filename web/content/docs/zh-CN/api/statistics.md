---
title: 下载统计 API
order: 14
category: API 接口
description: 有界下载计数、分层查询、存储库控制及 API Token 要求
---

# 下载统计 API

RenoP 对成功的软件包下载进行聚合，不为每次请求单独保存数据库记录。计数包含下载次数、逻辑字节数与最后
更新时间。用户归属绑定账号的不可变 ID，因此修改用户名不会拆分历史数据。

Maven、Cargo 与 Docker 存储库默认启用统计；非结构化 `files` 引擎需要手动启用。校验和、分离签名、Maven
元数据及 Javadoc 伴随请求不会计入。`HEAD`、`304`、失败请求及非起始分段请求不会计入。Docker 在返回
Manifest 时记录一次拉取，不按每个 Blob 重复计数。

## 账号查询

`GET /api/statistics` 返回 API Token 所属账号的统计。`GET /api/statistics/users/:username` 使用相同的账号
边界；查询其他账号必须使用系统管理员 Token。

两个接口都只接受带 `statistics:read` 的 Bearer API Token。浏览器 Session Cookie 与 Basic 凭据会被拒绝。
查询前会先写入内存中的待处理计数，因此成功响应包含当前服务进程已经接受的下载。

## 系统查询

`GET /api/statistics/system` 要求系统管理员账号及 `admin:statistics` 权限。支持按 `user`、`repository`、
`namespace`、`package` 或 `version` 分组；账号接口支持除 `user` 外的全部分组。

可选精确筛选项为 `username`（仅 system）、`repository`、`format`、`namespace`、`package` 与 `version`。
分页使用 1 至 100 的 `limit` 和从零开始、最大为 1,000,000 的 `offset`。每页还返回完整筛选结果的
`count`、`bytes`，以及分组记录总数。

## 存储库控制

管理员通过 `GET /api/settings/repositories/download-statistics` 查询有效开关，通过
`PUT /api/settings/repositories/:name/download-statistics` 修改单个存储库。JSON 请求体为
`{"enabled": true}` 或 `{"enabled": false}`。

`DELETE /api/settings/repositories/:name/download-statistics` 会永久清除已保存和待写入计数。对于 Docker，
还会重置镜像页面显示的兼容拉取计数。删除存储库时会自动清除其统计数据。

完整响应结构与参数限制见 `web/assets/openapi.yaml`。
