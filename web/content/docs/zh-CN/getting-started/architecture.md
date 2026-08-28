---
title: 系统架构
order: 4
category: 快速开始
description: 模块化服务、授权、流式存储与异步任务
---

# 系统架构

RenoP 是单个 Go 进程，在传输、包协议、授权、持久化与后台维护之间保持明确边界。内嵌前端调用与外部客户端
相同的有界 API。

## 模块边界

```text
Browser and package clients
        |
HTTP routing, rate limits, authentication, API-token policy
        |
Maven | npm | Cargo | Docker | Files | Management services
        |
Repository gate and publication workflows
        |
Disk or S3 storage          SQL database
        |                       |
File index and mirrors      Identity, teams, audit, messages
```

- `internal/api` 与中间件负责通用 HTTP 契约、搜索、异常检测和凭据边界；
- 各引擎服务负责 Maven 域/目录、npm packument、Cargo Sparse Index、Docker Distribution v2 与文档预览；
- 数据库层为 SQLite、MySQL 与 PostgreSQL 提供方言感知事务；
- Disk/S3 流式处理大型正文，文件索引提供有界元数据遍历。

## 请求与任务流水线

### 流式处理与一致性

上传和下载在客户端与 Disk/S3 之间流式传输。哈希、Brotli/ZIP 解包、镜像缓存与 GPG 发布使用有界 Reader 和
临时文件。分片存储库门控防止存储/引擎变更与上传、删除、镜像提交或最终发布产生竞争。

### 认证与授权

浏览器会话只使用 Cookie；Basic 仅用于标准包协议。Bearer API Token 的权限与精确目标限制会在每次请求时和
账号当前存储库权限、L0-L4 包/域成员关系取交集。不可变用户 ID 在用户名变更后继续保持所有权。

### 异步任务

进程级不可重入调度器合并状态快照、清理、索引持久化、下载计数刷新与更新检查。要求顺序的审计、GPG、Token
修改与文件监控仍使用专用串行 Worker。持久工作流结果进入消息中心，临时进度使用 UI 状态或 Toast。
