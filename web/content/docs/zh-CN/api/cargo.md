---
title: Cargo 存储库 API
order: 5
category: API 接口
description: Cargo Sparse Index、crate 发布、下载与 yank
---

# Cargo 存储库 API

RenoP 实现 Cargo Registry 与 Sparse Index 规范。

## Sparse Index 配置 (`config.json`)

- **路径**：`GET /{repo}/config.json` 或 `GET /{repo}/index/config.json`
- **用途**：Cargo 首次连接时读取此文件，以发现存储库接口。

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

---

## 下载 crate

- **路径**：`GET /{repo}/api/v1/crates/{crate_name}/{version}/download`
- **响应**：`application/x-tar` 类型的 `.crate` 归档。

---

## Yank 与 unyank

- **Yank**：`DELETE /{repo}/api/v1/crates/{crate_name}/{version}/yank`
- **Unyank**：`PUT /{repo}/api/v1/crates/{crate_name}/{version}/unyank`
- **认证**：crate 所有者或管理员。
