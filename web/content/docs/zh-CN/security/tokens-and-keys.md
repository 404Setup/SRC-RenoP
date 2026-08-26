---
title: Token 与 GPG 签名
order: 2
category: 安全
description: 细粒度机器凭据、恢复材料与 OpenPGP 发布校验
---

# Token 与 GPG 签名

RenoP 将浏览器会话、API Token、密码认证、恢复材料与制品签名密钥分离，并分别使用不同的存储、传输与撤销规则。

## API Token 与恢复材料

API Token 使用 256 位随机数与 `rnp_pat_` 前缀。密钥只显示一次，仅存储 SHA-256 查询摘要。每个 Token 具有
私有名称、一个或多个权限、可选精确存储库/包/团队/域目标及可选有效期。账号最多拥有 50 个 Token，每个
Token 最多包含 128 个目标。

应使用最小权限与尽可能短的有效期。Token 策略与所属账号当前系统、存储库、域或包团队权限必须同时允许操作。
撤销会立即清理认证缓存。旧版明文上传 Token 会迁移为哈希兼容凭据。

浏览器会话只使用 Cookie，Basic 只用于包协议，API 自动化使用 `Authorization: Bearer <token>`。查询参数中的
凭据会被忽略或拒绝。

恢复代码独立于 API Token。一次生成 12 串一次性高强度代码，只存储 Argon2id verifier。4 串不同且未使用的
代码会原子重设密码、消耗代码、撤销会话并重新启用密码登录。应离线保存，并在使用或疑似泄露后重新生成。

---

## OpenPGP 分离签名校验

Maven 存储库可要求制品公开前具有有效 `.asc` 分离签名。用户在账号中注册公钥，私钥不会进入 RenoP。

### 启用校验

```yaml
repositories:
  releases:
    name: releases
    format: maven
    require_gpg_signature: true
```

### 发布流程

1. RenoP 将制品流式写入 `.renop.tmp.gpg` 并创建有界待处理发布；
2. 匹配 `.asc` 可在截止时间内先于或晚于制品上传；
3. RenoP 解析无歧义的已注册指纹，并在存储库门控内重新检查签名、上传者与存储库/域策略；
4. 有效制品/签名对原子提交，并保存验证元数据供 UI 使用；
5. 无效、缺失、过期、已删除或未授权发布使用稳定原因失败。

密钥服务器 URL 必须使用 HTTPS，并配置在 `server.gpg.key_servers`。出站请求遵循代理策略，使用有界客户端，
且绝不会上传私钥。
