---
title: GPG 签名
order: 5
category: API
description: 注册签名密钥并验证 Maven 制品签名
---

# GPG 签名

RenoP 支持验证 Maven 制品的 OpenPGP 分离签名。GPG 策略适用于 `.jar`、`.pom` 和 `.module` 文件。只有使用
上传账户已注册的密钥验证成功后，签名记录才会保存。

## 配置

在 `config.yaml` 的 `server.gpg.key_servers` 中配置 1–8 个 HTTPS 密钥服务器。也可以通过设置 API 的
`server.gpg` 字段进行配置。用户注册密钥时，RenoP 使用这些服务器解析密钥 ID
或指纹。详见[配置概览](../configuration/overview.md)
和[设置 API](./settings.md)。

仓库设置 `require_gpg_signature: true` 后，以上三类受保护制品必须提供签名。校验和文件及 Maven 元数据伴随文件会随同一次发布处理。
详见[仓库与镜像](../configuration/repositories.md)。

## 注册密钥

通过认证的用户最多可以在个人资料中注册 10 个公钥：

| 方法     | 接口                                 | 返回值                    |
|----------|--------------------------------------|---------------------------|
| `GET`    | `/api/auth/profile/gpg`              | `GpgKeyList`              |
| `POST`   | `/api/auth/profile/gpg`              | `GpgKeyDto`               |
| `DELETE` | `/api/auth/profile/gpg/:fingerprint` | 空的 `204` 响应           |
| `GET`    | `/api/auth/profile/gpg/releases`     | `GpgReleaseList` 发布记录 |

`POST` 请求正文为 `GpgKeyReferenceRequest`（`application/x-protobuf`）：

```protobuf
syntax = "proto3";

message GpgKeyReferenceRequest {
  string key_id = 1;
}
```

短密钥 ID 存在歧义时必须使用完整指纹。服务端会将解析后的公钥保存到数据库，不接受私钥材料。上述接口均要求账户认证；用户只能查看自己的发布记录。

## 上传带签名的制品

制品及其分离签名必须使用相同的 Maven 路径。签名文件名必须是在制品文件名后追加小写 `.asc`，例如
`demo-1.0.0.jar.asc`。

单次上传制品时，如果同时上传对应签名，请设置 `X-RenoP-GPG-Signature-Expected: true`：

```bash
curl -u alice:TOKEN \
  -H 'X-RenoP-GPG-Signature-Expected: true' \
  -T demo-1.0.0.jar \
  'https://repo.example/releases/com/example/demo/1.0.0/demo-1.0.0.jar'

curl -u alice:TOKEN \
  -T demo-1.0.0.jar.asc \
  'https://repo.example/releases/com/example/demo/1.0.0/demo-1.0.0.jar.asc'
```

使用分块上传时，在 `ChunkedUploadInitRequest` 中设置 `gpg_signature_expected: true`，不再使用上述请求头。浏览器上传表单检测到匹配的
`.asc` 文件后会自动设置该字段。

分离签名必须是 ASCII Armor 格式的 OpenPGP 签名，大小不超过 1 MiB；签名密钥必须是上传者已注册的密钥。仓库要求签名，或上传显式声明需要签名时，制品会在
GPG 隔离区中保留，直到制品与签名验证完成。缺少匹配文件的发布任务约 15 分钟后过期，并记录为失败。

签名不是必需项且上传未设置期望标志时，制品会作为未签名文件直接发布。之后上传 `.asc` 仍可创建已验证的签名记录。替换制品会使旧签名记录失效，除非新发布已验证成功。

## 查询验证结果

### `GET /api/maven/signatures/:repo_name/*`

对已验证的 `.jar`、`.pom` 或 `.module` 制品返回 `GpgSignatureDetails`（`application/x-protobuf`
）。请求需要仓库读取权限。记录不存在、扩展名不受支持或制品不可访问时返回 `404`。

主要字段：

| 字段                                      | 含义                      |
|-------------------------------------------|---------------------------|
| `repository` / `artifact_path`            | 仓库名称与 Maven 相对路径 |
| `fingerprint` / `key_id`                  | 签名公钥标识              |
| `primary_identity`                        | 解析后的公钥主身份        |
| `uploader`                                | 提交该发布任务的账户      |
| `signature_created_at` / `verified_at`    | Unix 毫秒时间戳           |
| `hash_algorithm` / `public_key_algorithm` | 验证时记录的算法          |

只有存在已验证签名记录时，`FileDetails.signed` 才为 `true`。浏览器会为已签名制品显示锁形操作按钮，点击后请求上述接口。支持的文本文件仍可预览，其中包括
`.pom` 和 `.module`。

发布失败及失败原因可通过 `GET /api/auth/profile/gpg/releases` 查询。状态包括 `queued`、`validating`、`success` 和 `failed`。
