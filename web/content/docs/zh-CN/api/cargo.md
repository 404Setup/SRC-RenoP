---
title: Cargo 注册源 API
order: 5
category: API 接口
description: Cargo 稀疏索引协议、Crate 发布、下载与撤回接口
---

# Cargo 注册源 API

RenoP 实现了 Cargo Registry 与 Sparse Index 规范。

## 1. 稀疏索引配置 (`config.json`)

- **路径**：`GET /{repo}/config.json` 或 `GET /{repo}/index/config.json`
- **说明**：Cargo 客户端在初次接入注册源时首先请求该文件获取 API 端点配置。

### 响应示例 (JSON)

```json
{
  "dl": "http://localhost:3000/{repo}/api/v1/crates",
  "api": "http://localhost:3000/{repo}",
  "auth-required": false
}
```

---

## 2. 稀疏索引查询

- **路径**：`GET /{repo}/index/{prefix}/{crate_name}`
- **说明**：按照 Cargo 索引分层规范返回 Crate 各版本的 JSON Lines 描述元数据。
    - 1 字符 crate：`1/{crate}`
    - 2 字符 crate：`2/{crate}`
    - 3 字符 crate：`3/{c}/{crate}`
    - 4 字符及以上：`{c1}{c2}/{c3}{c4}/{crate}`

---

## 3. 发布 Crate

- **路径**：`PUT /{repo}/api/v1/crates/new`
- **认证要求**：需携带有效 Token（`Authorization: <token>`）
- **请求体**：按照 Cargo 协议封装的 JSON 元数据长度 + JSON 元数据 + `.crate` tarball 二进制流。

---

## 4. 下载 Crate 包

- **路径**：`GET /{repo}/api/v1/crates/{crate_name}/{version}/download`
- **响应**：`.crate` 压缩包二进制流（`application/x-tar`）。

---

## 5. 撤回与恢复 Crate 版本 (Yank)

- **撤回**：`DELETE /{repo}/api/v1/crates/{crate_name}/{version}/yank`
- **恢复**：`PUT /{repo}/api/v1/crates/{crate_name}/{version}/unyank`
- **认证要求**：Crate 所有者或 Admin 权限
