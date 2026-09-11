---
title: GPG 加密 API
order: 11
category: API 参考
description: OpenPGP 公钥管理与签名验证状态
---

# GPG 加密 API

## 查询账号 GPG 公钥

- **路径**：`GET /api/auth/profile/gpg`
- **认证**：必须登录。

### JSON 响应

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

## 注册 GPG 公钥

- **路径**：`POST /api/auth/profile/gpg`
- **认证**：必须登录。
- **JSON 正文**：
  ```json
  {
    "public_key_armored": "-----BEGIN PGP PUBLIC KEY BLOCK-----\n...\n-----END PGP PUBLIC KEY BLOCK-----"
  }
  ```
- **响应**：`200 OK`，并返回解析后的公钥元数据。

---

## 查询隔离中的发布

- **路径**：`GET /api/auth/profile/gpg/releases`
- **用途**：列出因等待分离签名、公钥验证或发布确认而处于隔离暂存状态的制品。
