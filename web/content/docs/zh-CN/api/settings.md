---
title: 系统设置 API
order: 8
category: API 接口
description: 读取与修改服务端配置、仓库设置与重建索引接口
---

# 系统设置 API

## 1. 获取全局配置

- **路径**：`GET /api/settings/config`
- **认证要求**：Manager 或 Admin 权限

---

## 2. 修改全局配置

- **路径**：`PUT /api/settings/config`
- **认证要求**：Admin 权限
- **说明**：支持动态更新域名、前端定制、出站代理等配置项。监听端口与 TLS 变更需重启后生效。

---

## 3. 仓库配置管理

### 获取所有仓库配置

- **路径**：`GET /api/settings/maven/repositories`
- **认证要求**：Manager 或 Admin 权限

### 修改指定仓库配置

- **路径**：`PUT /api/settings/maven/repositories/:name`
- **认证要求**：Manager 或 Admin 权限

---

## 4. 重建制品索引

- **路径**：`POST /api/settings/index/rebuild`
- **认证要求**：Admin 权限
- **说明**：异步重新扫描存储目录并重建 `index.json` 搜索缓存。
- **响应**：`202 Accepted`，`{"message": "Index rebuild triggered"}`
