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

## 浏览器设置分页

设置界面为服务器公布的 15 类配置分别提供独立页面。桌面端在表单旁显示分类导航，小屏幕使用分类选择器。
上一页和下一页按相同顺序切换。打开一个页面只读取该类配置，不会同时拉取全部服务配置。

切换分类或语言时，每页都会保留未保存的草稿。保存只更新当前页，提交期间暂时禁止编辑和切页。
保存失败会保留草稿；放弃更改需确认后重新读取当前页。分类导航会标记未保存的更改。
刷新或关闭浏览器时会提示未保存内容，但草稿仅保存在内存中，退出账号或切换账号时会清除。
已有的只写凭据保持隐藏，成功保存后会从草稿中清除本次输入的密钥。

GPG 仍属于服务配置。全局团队限制、发布配额、注册、缓存、邮件、OAuth 提供商和发布域安全拥有独立页面，
继续使用原有 JSON API。表单标签和提示与控件关联，切页时会将键盘焦点移到新页面标题。

当前分类、未保存标记与次要文字在浅色和深色模式下都使用统一主题颜色。

## 读取与更新设置域

- **读取**：`GET /api/settings/domain/:name`
- **更新**：`PUT /api/settings/domain/:name`
- **行为**：请求与响应结构取决于 `:name`。未知字段和无效值会被拒绝。主机、端口、TLS、数据库及部分运行时
  参数变更可能要求重启服务。

**第三方登录**：`GET /api/settings/oauth-providers` 返回内置 GitHub 条目、脱敏客户端和预设。`PUT /api/settings/oauth-providers` 必须包含 `providers` 数组，并原子保存两组配置。支持 GitHub 加最多 32 个其他客户端，请求上限为 128 KiB。为兼容旧客户端，省略 GitHub 会保留其配置；应关闭该条目并使用 `clear_client_secret` 清除密钥。空数组仅移除其他客户端。兼容端点 `GET /api/settings/github-oauth` 和 `PUT /api/settings/github-oauth` 继续管理同一份 GitHub 配置。

[OAuth](../security/oauth-login.md)

## 存储库设置

优先使用 `/api/settings/repositories`。带 Maven 前缀的旧接口继续用于兼容。

仓库变更先提交数据库，再替换运行中的配置。删除最后一个仓库后，重启仍保留空集合；旧 YAML 仅作为首次迁移的来源。

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

[法律文档与 Cookie 偏好](../configuration/legal.md)

设置分页、服务提供方编辑器和关联字段使用可取消的过渡动画，并尊重减少动态效果偏好。未配置提供方或邮箱时采用统一提示样式。关闭的下拉控件不会保留选项菜单或文档监听器，打开时才按需创建。

内置资源从可执行文件流式发送。RenoP 缓存资源类型、长度和 ETag，不再在 Go 堆中额外保留每个脚本包与压缩版本的完整副本，仍支持预压缩协商和条件请求。

[安全验证](../security/captcha.md)
