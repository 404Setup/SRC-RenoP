---
title: 全局日志与行为日志
order: 13
category: API 参考
description: 查看系统诊断并筛选账号操作记录
---

# 全局日志与行为日志

## 访问

管理员可从仪表盘打开**全局日志**。全局接口要求当前账号具有系统管理员权限；API Token 还需要 `admin:audit`。仓库版主不具有此权限。个人行为日志仅包含当前账号，管理员可查看其他账号的行为日志。

触发者是发起操作的账号，操作者是实际执行操作的账号或系统任务。现有直接操作及历史记录使用操作者作为触发者。个人视图对两种身份采用相同的隐藏和筛选规则，触发来源仍使用 `trigger`。账号改名及注销清理同时处理这两种身份。

```http
GET /api/auth/logs?kind=system&severity=error&trigger=http&page=1&page_size=20
GET /api/auth/profile/audit-logs?action=LOGIN&from=1788739200000&until=1788825599999
GET /api/auth/users/alice/audit-logs?operator=admin&trigger=web
```

## 筛选

所有条件使用 AND 组合；文本精确匹配，留空表示不筛选。时间控件使用浏览器本地时间并发送 Unix 毫秒值。无效条件返回 `400` 和 `LOG_FILTER_INVALID`。操作类型和触发来源最长 64 字节，账号和操作者最长 255 字节。

| 参数 | 取值 |
| --- | --- |
| `kind` | `audit`, `system` |
| `action` | 精确匹配操作 ID，例如 `LOGIN`, `SYSTEM_LOG`, `SYSTEM_HTTP_ERROR` |
| `operator` | 精确匹配用户名；个人视图还可使用 `@administrator` |
| `initiator` | 精确匹配用户名；个人视图还可使用 `@administrator` |
| `username` | 受影响的账号，仅全局视图 |
| `trigger` | 精确匹配来源 `web`, `api`, `http`, `system`, `unknown` |
| `severity` | `info`, `warning`, `error` |
| `from`, `until` | 包含边界的 Unix 毫秒值，可省略任意一侧 |
| `page`, `page_size` | 默认 `1`、`20`；每页 `1`–`200`；偏移量最多 `1000000` |

## 可见性

响应继续使用 `AuditLogList` protobuf，并新增 `kind`、`trigger`、`initiator`、`severity` 字段。个人和指定账号接口固定返回 `audit` 类型。个人视图将其他操作者统一显示为管理员，`operator=@administrator` 可选择这一组；直接猜测其他操作者用户名一律返回 `400`，不存在的用户名也相同，避免泄露隐藏的管理员身份。`username` 条件无法扩大账号接口的范围。

## 系统诊断

后台服务启动后开始收集进程日志；HTTP 服务错误会被记录，诊断内容不会返回给公共调用方。入库前会隐藏常见的密码、认证头、令牌和 URL 用户信息，每条记录最多 4096 字节。现有日志根据认证方法推导触发来源，迁移的历史记录使用 `unknown`。无结构进程日志按关键词推测级别，结构化 HTTP 错误使用 `error`。

## 保留与写入

单个消费者串行写入行为和系统记录，正常关闭时恢复进程日志输出并排空队列。队列上限为 500 条；满时行为日志继续使用现有同步写入回退，进程日志保留在原输出中、增加失败计数，并在恢复容量后记录队列溢出。写入失败仅输出到原进程日志，避免递归。行为日志沿用 `audit_log` 的期限与数量上限；系统日志独立计算，最多保留 30 天、10000 条，若配置更低则采用较低值。每十分清理一次。账号注销保留期与永久清除规则也适用于关联系统记录。不会导入历史进程日志。
