---
title: GPG 密钥与签名 API
order: 11
category: API 接口
description: 个人 GPG 公钥管理与签名校验接口
---

# GPG 密钥与签名 API

## 1. 查询当前用户的 GPG 公钥

- **路径**：`GET /api/auth/profile/gpg`
- **认证要求**：需已登录

### 响应 (JSON)

```json
{
  "keys": [
    {
      "key_id": "9B27346A83C1D0EE",
      "fingerprint": "A518767AE71A1C38BCE3178C9B27346A83C1D0EE",
      "user_id": "Developer <dev@example.com>",
      "created_at": 1740000000
    }
  ]
}
```

---

## 2. 导入/登记 GPG 公钥

- **路径**：`POST /api/auth/profile/gpg`
- **认证要求**：需已登录
- **请求体 (JSON)**：
  ```json
  {
    "public_key_armored": "-----BEGIN PGP PUBLIC KEY BLOCK-----\n...\n-----END PGP PUBLIC KEY BLOCK-----"
  }
  ```
- **响应**：`200 OK`，返回已解析的密钥元数据。

---

## 3. 查询隔离区暂存的待验签发布包

- **路径**：`GET /api/auth/profile/gpg/releases`
- **说明**：列出当前用户上传但因缺少对应 `.asc` 签名文件暂留在隔离区中的待发布制品。
