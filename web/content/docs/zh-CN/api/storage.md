---
title: 存储与分块上传 API
order: 10
category: API 接口
description: 标准 Maven HTTP 读写接口与大文件分块上传协议
---

# 存储与分块上传 API

## 1. Maven 标准文件读写

客户端可以直接通过 HTTP 标准方法操作仓库路径：`/{repo}/{path...}`

### 下载制品

- **请求**：`GET /{repo}/{path}`
- **说明**：支持 HTTP Range 断点续传，自动生成或校验 ETag 与 Last-Modified。

### 上传制品

- **请求**：`PUT /{repo}/{path}`
- **认证要求**：需拥有对应仓库的写入权限（`canupdate:{repo}`）
- **响应**：`201 Created`

### 删除制品

- **请求**：`DELETE /{repo}/{path}`
- **认证要求**：需拥有对应仓库的管理员权限（`canadmin:{repo}`）或超级管理员权限
- **响应**：`204 No Content`

---

## 2. 大文件分块上传 API

针对超大文件（如包含庞大嵌入依赖的 Fat JAR 或多模块压缩包），Web 界面与 CI 工具可使用分块上传接口。

### 初始化上传任务

- **路径**：`POST /api/upload/chunked`
- **请求体 (JSON)**：
  ```json
  {
    "repository": "releases",
    "target_path": "com/example/big-app/1.0.0/big-app-1.0.0.jar",
    "total_size": 524288000,
    "chunk_size": 10485760
  }
  ```
- **响应 (JSON)**：`{"upload_id": "up_987654321", "chunk_size": 10485760}`

### 上传分块数据

- **路径**：`PUT /api/upload/chunked/:upload_id?chunk_index=0`
- **请求体**：当前分块的二进制流

### 完成合并

- **路径**：`POST /api/upload/chunked/:upload_id/complete`
- **请求体 (JSON)**：
  ```json
  {
    "sha256": "abcdef1234567890..."
  }
  ```
- **响应**：`201 Created`
