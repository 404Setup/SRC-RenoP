---
title: Cargo 仓库 API
order: 5
category: API 参考
description: Cargo Sparse Index、crate 发布、下载与 yank
---

# Cargo 仓库 API

RenoP 实现 Cargo Registry 与 Sparse Index 规范。

## Sparse Index 配置 (`config.json`)

- **路径**：`GET /{repo}/config.json` 或 `GET /{repo}/index/config.json`
- **用途**：Cargo 首次连接时读取此文件，以发现仓库接口。

### JSON 响应

```json
{
  "dl": "http://localhost:3000/{repo}/api/v1/crates",
  "api": "http://localhost:3000/{repo}",
  "auth-required": false
}
```

---

## Sparse Index 元数据

- **路径**：`GET /{repo}/index/{prefix}/{crate_name}`
- **用途**：按照 Cargo 标准 crate 名称分片规则返回逐行 JSON 元数据。

---

## 发布 crate

- **路径**：`PUT /{repo}/api/v1/crates/new`
- **认证**：需要在 `Authorization: <token>` 中提供 Token。
- **正文**：4 字节 JSON 长度、JSON 元数据以及 `.crate` 二进制归档。
- **名称冲突**：首次发布时，若规范化名称已存在于本地或适用镜像，返回 `409 Conflict`；无法确定上游结果时
  返回 `503 Service Unavailable`。

发布时，RenoP 会解析 `Cargo.toml` 中的 `package.readme` 声明，
从归档中提取对应文件供软件包详情页展示说明文档（上限 512 KiB）。
目录浏览与检索结果不加载 README 正文以提升列表查询性能。

---

## 下载 crate

- **路径**：`GET /{repo}/api/v1/crates/{crate_name}/{version}/download`
- **响应**：`application/x-tar` 类型的 `.crate` 归档。

---

## Yank 与 unyank

- **Yank**：`DELETE /{repo}/api/v1/crates/{crate_name}/{version}/yank`
- **Unyank**：`PUT /{repo}/api/v1/crates/{crate_name}/{version}/unyank`
- **认证**：crate 所有者或管理员。

## 资源锁定

管理员和此仓库的版主使用有效的浏览器会话 Cookie，调用 `PUT /{repo}/api/v1/crates/{crate_name}/locks`。
API 令牌和包客户端凭据不能管理锁定。

```json
{"version":"1.2.3","mode":"read","reason":"trojan"}
```

省略 `version` 或使用 `""` 可锁定整个包。模式为 `write` 和 `read`。支持本地化公开显示的原因有：
`hold`、`prohibited`、`expired`、`trojan`、`abuse`、`dmca`、`reup`、`squatting` 和 `quality`。
修改成功返回 `{"ok":true}`。移除指定的人工锁定时，调用
`DELETE /{repo}/api/v1/crates/{crate_name}/locks`，请求体为 `{"version":"1.2.3"}`。

禁止写入会冻结修改和上游刷新，已存储的文件仍可下载。禁止读取还会阻止所有文件下载和文档预览，包括工作人员。
元数据仅对管理员、仓库版主、所有者和协作者可见，包括 L0 成员及绑定的全局团队成员。其他访问者无法在元数据、
稀疏索引、搜索、用户资料或团队资源列表中看到被锁定的包或版本。

包和版本元数据中的公开 `locks` 记录包含 `mode`、`reason`、`source` 和 `locked_at`。
系统锁定与人工锁定相互独立，移除人工锁定不会解除系统限制。修改请求返回 `423` 及
`X-Renop-Error-Code: resource_locked`，被拒绝的读取返回 `404`。版本锁定会阻止整个包的归档、
永久弃用和删除，但仍可发布其他版本。存在资源锁定时，仓库不可重新配置或删除。
