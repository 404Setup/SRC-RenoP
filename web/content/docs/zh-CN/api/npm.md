---
title: npm 仓库 API
order: 7
category: API 参考
description: npm 软件包元数据、发布、tarball、发布标签、团队与管理接口
---

# npm 仓库 API

每个格式为 `npm` 的仓库都在 `/{repo}/` 下提供兼容 npm 的 JSON 仓库。在首次发布前，必须通过管理 API
或 Web 界面预留软件包名称。

## 仓库发现与身份

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

每次发布仅支持包含一个语义化版本及一个 Base64 编码的 tarball 附件。
压缩 tarball 限制为 64 MiB，解压限制为 512 MiB，`package.json` 上限为 2 MiB。
服务端执行严格的归档完整性校验，校验通过后制品方可正式入库发布。

## 发布标签与生命周期

- **列出标签**：`GET /{repo}/-/package/{package}/dist-tags`
- **设置标签**：`PUT /{repo}/-/package/{package}/dist-tags/{tag}`
- **删除标签**：`DELETE /{repo}/-/package/{package}/dist-tags/{tag}`
- **按修订号更新元数据或取消发布**：`PUT /{repo}/{package}/-rev/{revision}`
- **按修订号删除软件包**：`DELETE /{repo}/{package}/-rev/{revision}`

软件包版本具备不可变性。取消发布或删除后将永久废弃该版本号，不可重新上传同名版本。
修订号冲突时返回 `409 Conflict`，客户端需重新获取最新元数据后重试。

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
权限会与账号当前权限和可选的精确仓库、软件包或团队目标取交集。发布仍要求软件包已存在且用户至少为 L1；
元数据与取消发布需要 L2，团队变更需要 L3，所有权与软件包删除需要 L4。

## 资源锁定

管理员和此仓库的版主使用有效的浏览器会话 Cookie 管理锁定：

- `PUT /api/npm/repositories/{repo}/locks?package={package}`
- `DELETE /api/npm/repositories/{repo}/locks?package={package}`

```json
{"version":"1.2.3","mode":"read","reason":"trojan"}
```

省略 `version` 或使用 `""` 可锁定整个包。模式为 `write` 和 `read`。本地化公开显示的原因包括
`hold`、`prohibited`、`expired`、`trojan`、`abuse`、`dmca`、`reup`、`squatting` 和 `quality`。
DELETE 接受 `{"version":"1.2.3"}`，仅移除指定的人工锁定，系统锁定仍独立生效。
API 令牌不能管理锁定。包和版本的详细信息会公开显示 `locks` 记录。

两种模式均冻结修改。禁止读取还会将元数据的可见范围限制为管理员、仓库版主、所有者和协作者，
包括 L0 成员与绑定的全局团队成员；所有人均无法下载文件。
其他访问者无法在完整或精简 packument、搜索、用户资料和团队资源中看到受限版本及其标签。
经过过滤的元数据会禁用缓存，也不会返回过期的 `304`。

版本锁定还会冻结关联的 dist-tag 修改，并阻止整个包的归档、永久弃用和删除。
完整 packument 可以重复包含未修改的锁定元数据。使用不会移动锁定版本标签的新标签时，仍可发布其他版本。
任何锁定都会停止上游 packument 的整体替换；受影响的已缓存 tarball 不会过期或重新获取。
存在锁定时，仓库不可重新配置或删除。

被拒绝的修改返回 `423` 及 `X-Renop-Error-Code: resource_locked`；被拒绝的读取返回 `404`。
