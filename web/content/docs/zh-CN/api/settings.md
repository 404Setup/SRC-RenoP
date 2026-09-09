---
title: 设置 API
order: 8
category: API 参考
description: 按域管理服务设置、存储库与索引重建
---

# 设置 API

设置接口要求管理员账号，或根据操作提供带有 `admin:settings`、`admin:repositories` 的 API Token。
`proto/api/v1/api.proto` 中定义的响应使用 protobuf。

## 查询设置域

- **路径**：`GET /api/settings/domains`
- **响应**：服务端当前支持的稳定域名，包括 `server`、`proxy`、`storage`、`updater` 与 `index`。

## 读取与更新设置域

- **读取**：`GET /api/settings/domain/:name`
- **更新**：`PUT /api/settings/domain/:name`
- **行为**：请求与响应结构取决于 `:name`。未知字段和无效值会被拒绝。主机、端口、TLS、数据库及部分运行时
  参数变更可能要求重启服务。
- **GitHub OAuth**：`GET /api/settings/github-oauth` 返回脱敏状态；`PUT /api/settings/github-oauth` 更新
  Client ID 与只写 Secret。

**其他 OAuth 服务**：`GET /api/settings/oauth-providers` 返回隐藏密钥的客户端配置和预设；`PUT /api/settings/oauth-providers` 替换列表。必须提供 `providers` 数组，显式空数组会移除全部已配置客户端。最多支持 32 个客户端，请求体上限为 128 KiB。凭据、预设和账号绑定详见[第三方登录](../security/oauth-login.md)。

## 存储库设置

优先使用 `/api/settings/repositories`。带 Maven 前缀的旧接口继续用于兼容。

### 查询存储库

- **路径**：`GET /api/settings/repositories`
- **兼容别名**：`GET /api/settings/maven/repositories`

### 创建、更新、删除与迁移

- **创建或更新**：`PUT /api/settings/repositories/:name`
- **删除**：`DELETE /api/settings/repositories/:name`
- **Maven/files 迁移**：`POST /api/settings/repositories/:name/migrate/:target`，`:target` 为 `maven` 或
  `files`。存储对象保持原位，切回 Maven 时重建目录。

## 重建搜索索引

- **路径**：`POST /api/settings/index/rebuild`
- **行为**：提交可合并的后台重建任务，不会并发启动重复任务。

## 发布域保留周期

`GET /api/settings/maven-domains` 和 `PUT /api/settings/maven-domains` 使用 JSON。
设置发现列表包含 `maven_domains`，默认值为：

```json
{"release_value":2,"release_unit":"year"}
```

`release_value` 是 1–100 的整数；`release_unit` 支持 `month` 或 `year`，按 UTC 日历计算。
配置文件在 `maven_domains` 下保存相同字段。保存后仅影响新建安全锁，
不会改变已有释放日期，也不会改变主动关闭的独立 31 天保留期。
参见 [Maven 发布域状态](maven.md)。
