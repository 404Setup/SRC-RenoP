---
title: npm 存储库 API
order: 7
category: API 参考
description: npm 软件包元数据、发布、tarball、发布标签、团队与管理接口
---

# npm 存储库 API

每个格式为 `npm` 的存储库都在 `/{repo}/` 下提供兼容 npm 的 JSON 存储库。在首次发布前，必须通过管理 API
或 Web 界面预留软件包名称。

## 存储库发现与身份

- **可用性**：`GET /{repo}/-/ping`
- **当前账号**：`GET /{repo}/-/whoami`
- **搜索**：`GET /{repo}/-/v1/search?text={query}&size={limit}&from={offset}`

协议错误使用包含稳定 `error` 与 `reason` 字段的 JSON：

```json
{
  "error": "not_found",
  "reason": "npm package was not found"
}
```

## 软件包元数据与 tarball

- **完整或精简 packument**：`GET /{repo}/{package}`
- **tarball**：`GET /{repo}/{package}/-/{name}-{version}.tgz`
- **发布或编辑元数据**：`PUT /{repo}/{package}`

作用域软件包名称可编码为单个路径参数，例如 `%40example%2Flibrary`。packument 响应支持 ETag 与 Last-Modified
条件请求。请求 `application/vnd.npm.install-v1+json` 的客户端会收到大小受限的精简元数据。私有响应禁止共享缓存。

一次发布文档只能包含一个语义化版本和一个 base64 tarball 附件。JSON 正文上限为 96 MiB，压缩 tarball 为
64 MiB，解压内容为 512 MiB，文件条目为 100,000，`package.json` 为 2 MiB。每个软件包最多保留 5,000 条版本
记录和合计 4 MiB 的版本元数据。服务器会将解码后的归档数据流式写入暂存区，不会发布只完成部分验证的 tarball。

## 发布标签与生命周期

- **列出标签**：`GET /{repo}/-/package/{package}/dist-tags`
- **设置标签**：`PUT /{repo}/-/package/{package}/dist-tags/{tag}`
- **删除标签**：`DELETE /{repo}/-/package/{package}/dist-tags/{tag}`
- **按修订号更新元数据或取消发布**：`PUT /{repo}/{package}/-rev/{revision}`
- **按修订号删除软件包**：`DELETE /{repo}/{package}/-rev/{revision}`

版本不可变。取消发布与删除会建立墓碑，因此已发布的语义化版本不可复用。修订号冲突返回 `409 Conflict`，
客户端需要重新获取当前 packument。

## 浏览器管理 API

同源管理接口使用 JSON，失败时通过稳定的 `X-Renop-Error-Code` 响应头报告错误类型。

- `GET /api/npm/repositories/{repo}/packages`
- `POST /api/npm/repositories/{repo}/packages`
- `PUT /api/npm/repositories/{repo}/packages?package={package}`
- `DELETE /api/npm/repositories/{repo}/packages?package={package}`
- `DELETE /api/npm/repositories/{repo}/versions?package={package}&version={version}`
- `GET /api/npm/repositories/{repo}/owners?package={package}`
- `POST /api/npm/repositories/{repo}/owners?package={package}`
- `PUT /api/npm/repositories/{repo}/owners/{user}?package={package}`
- `DELETE /api/npm/repositories/{repo}/owners/{user}?package={package}`
- `GET /api/npm/repositories/{repo}/users/search?package={package}&q={query}`
- `POST /api/npm/repositories/{repo}/invitations/{id}/{accept|reject}`

目录响应通过 `limit` 与 `offset` 分页；`limit` 范围为 1 至 100。调用者没有软件包成员关系或管理员权限时，
私有软件包不会出现。团队详情只返回给 L3/L4 成员和管理员。

软件包详情响应会从选定的已发布版本中返回大小受限的 README、作者、贡献者、维护者、许可证、运行环境、
关键词与项目链接元数据。浏览器通过元素与 URL 白名单渲染 README Markdown，不会激活软件包提供的 HTML
或不安全链接。

## 认证与授权

npm 客户端可使用账号密码或 API Token 进行 Basic 认证，也可将 API Token 用作 `_authToken`。Bearer API Token
权限会与账号当前权限和可选的精确存储库、软件包或团队目标取交集。发布仍要求软件包已存在且用户至少为 L1；
元数据与取消发布需要 L2，团队变更需要 L3，所有权与软件包删除需要 L4。
