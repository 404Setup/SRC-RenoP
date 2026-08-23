---
title: Maven Metadata API
order: 4
category: API Reference
description: Artifact search, metadata details, version discovery, and SVG badge generation
---

# Maven Metadata API

## 1. Search Artifacts

- **Path**: `GET /api/search`
- **Query Parameters**:
    - `q`: Search keyword (matches groupId, artifactId, or version)
    - `repo`: Repository filter (optional)
    - `limit`: Result count (default: 20, max: 100)

### Response (JSON)

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

## 2. Artifact Details

- **Path**: `GET /api/maven/details/:repo/:group/:artifact`
- **Description**: Returns version history, packaging formats, and dependencies for a coordinate.

---

## 3. Version Status SVG Badge

- **Path**: `GET /api/maven/badge/:repo/:group/:artifact/version.svg`
- **Response**: SVG vector image displaying the latest version.
- **Example**:
  ```markdown
  ![Version](http://localhost:3000/api/maven/badge/releases/com.example/my-library/version.svg)
  ```

---

## 4. Generate POM Snippet

- **Path**: `GET /api/maven/pom-snippet/:repo/:group/:artifact/:version`
- **Response (JSON)**: Contains Maven XML, Gradle Kotlin DSL, and Groovy DSL dependency declaration strings.
