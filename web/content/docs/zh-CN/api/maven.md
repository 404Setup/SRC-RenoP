---
title: Maven 元数据 API
order: 4
category: API 接口
description: Maven 制品检索、元数据查询、版本发现与徽章生成接口
---

# Maven 元数据 API

## 1. 搜索制品

- **路径**：`GET /api/search`
- **查询参数**：
    - `q`：检索关键字（匹配 groupId、artifactId 或版本号）
    - `repo`：指定仓库（可选，默认全部可读仓库）
    - `limit`：返回数量（默认 20，上限 100）

### 响应 (JSON)

```json
{
  "results": [
    {
      "repository": "releases",
      "group_id": "com.example",
      "artifact_id": "my-library",
      "latest_version": "1.2.0",
      "versions": ["1.0.0", "1.1.0", "1.2.0"],
      "last_updated": 1740000000
    }
  ]
}
```

---

## 2. 获取制品详细信息

- **路径**：`GET /api/maven/details/:repo/:group/:artifact`
- **说明**：查询指定制品的版本列表、依赖树与打包类型。

---

## 3. 生成版本状态 SVG 徽章 (Badge)

- **路径**：`GET /api/maven/badge/:repo/:group/:artifact/version.svg`
- **响应**：返回标准 SVG 矢量图像（适用于嵌入 README 中展示最新版本号）。
- **示例**：
  ```markdown
  ![Version](http://localhost:3000/api/maven/badge/releases/com.example/my-library/version.svg)
  ```

---

## 4. 自动生成 POM 片段

- **路径**：`GET /api/maven/pom-snippet/:repo/:group/:artifact/:version`
- **响应 (JSON)**：包含 Maven XML、Gradle Kotlin DSL 与 Groovy DSL 的依赖声明字符串。
