---
title: Maven
order: 4
category: API
---

# Maven 浏览与辅助接口

前缀：`/api/maven`（徽章在 `/api/badge` 下）

这些接口读取索引与元数据。实际制品文件位于 `/{repo}/group/artifact/…`，详见 [storage.md](./storage.md)。

路径参数使用 Maven 布局，例如：

```text
com/example/demo
com/example/demo/1.0.0
```

读权限不足时通常返回 `404`。

## 目录与文件详情（Protobuf）

### `GET /api/maven/details`

当前用户可见的仓库，包装为虚拟根。

响应：`FileDetails`（`application/x-protobuf`）

```text
type = DIRECTORY
name = "repositories"
files[] = { type: DIRECTORY, name: "<repo>" }
```

### `GET /api/maven/details/:repo_name`

仓库根（含子项）。

### `GET /api/maven/details/:repo_name/*`

路径详情。目录包含 `files` 列表；文件包含 `content_length` 与 `last_modified_time`（RFC3339Nano 格式）。

`type` 为 `FILE` 或 `DIRECTORY`。

### `GET /api/maven/repo-details/:repo_name`

统计与镜像摘要。响应：`RepoDetailsResponse`。

| 字段                                                | 含义                                                                 |
|-----------------------------------------------------|----------------------------------------------------------------------|
| `name` / `visibility`                               | 仓库名称、可见性                                                     |
| `total_size` / `artifact_size` / `metadata_size`    | 总大小、制品大小、元数据大小（字节）                                 |
| `total_files` / `artifact_count` / `metadata_count` | 文件总数、制品数量、元数据数量（校验和与 maven-metadata 计为元数据） |
| `mirrors[]`                                         | 镜像配置：name、url、persist、cache_ttl、negative_cache 等           |

无读权限 → `403`（与 details 常用 `404` 不同）。

## 版本查询（Protobuf）

路径应指向含 `maven-metadata.xml` 的坐标目录（groupId/artifactId）。

### `GET /api/maven/versions/:repo_name/*`

| 查询     | 默认   | 含义         |
|----------|--------|--------------|
| `filter` | —      | 版本子串过滤 |
| `sorted` | `true` | 排序结果     |

响应：`application/x-protobuf`，`VersionsResponse`

```protobuf
message VersionsResponse {
  bool is_snapshot = 1;
  repeated string versions = 2;
}
```

### `GET /api/maven/latest/version/:repo_name/*`

相同查询参数；加 `type=raw` 返回纯版本字符串（`text/plain`）。

默认响应：`application/x-protobuf`，`LatestVersionResponse`

```protobuf
message LatestVersionResponse {
  bool is_snapshot = 1;
  string version = 2;
}
```

### `GET /api/maven/latest/details/:repo_name/*`

最新匹配制品的 `FileDetails`（ `application/x-protobuf` ）。

| 查询         | 默认  | 含义       |
|--------------|-------|------------|
| `extension`  | `jar` | 扩展名     |
| `classifier` | —     | classifier |
| `filter`     | —     | 版本过滤   |

### `GET /api/maven/latest/file/:repo_name/*`

解析最新版本后通过存储层获取（重定向或返回正文，类似直接访问制品 URL）。

## 徽章

### `GET /api/badge/latest/:repo_name/*`

含最新版本的 SVG 徽章。`Content-Type: image/svg+xml`。

| 查询     | 含义                          |
|----------|-------------------------------|
| `name`   | 左侧标签（默认：仓库名）      |
| `color`  | 右侧颜色（字母数字或 `#hex`） |
| `prefix` | 版本前缀文本                  |
| `filter` | 版本过滤                      |

```markdown
![latest](https://your-host/api/badge/latest/releases/com/example/demo)
```

## 生成 POM

### `POST /api/maven/generate/pom/:repo_name/*`

需要对仓库的写权限。正文：`application/x-protobuf`，`PomDetails`（兼容 JSON 格式输入）。

```protobuf
message PomDetails {
  string group_id = 1;
  string artifact_id = 2;
  string version = 3;
}
```

路径可以 `.pom` 结尾，或为坐标目录（此时文件名为 `artifact_id-version.pom`）。

磁盘空间不足时返回 `507`。成功时写入 POM 文件并更新索引。

## 隐私政策

### `GET|HEAD /api/privacy-policy`

若工作目录存在 `privacy-policy.txt` 文件，以 `text/plain` 格式返回其内容；否则返回 `404`。此接口与 Maven 无关，挂载在同一 API 组上。
