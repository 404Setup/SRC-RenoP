---
title: Docker / OCI Registry
order: 3
category: ガイド
description: Image 作成と Docker、Podman、containerd、nerdctl の利用
---

# Docker / OCI Registry ガイド

format `docker` の repository を作成し、push 前に対象 image も作成します。例は repository `containers` と
image `team/service` を使い、registry name は `containers/team/service` です。

## Login と transport

```bash
docker login localhost:3000
# Username: admin
# Password: <your_password_or_API_token>
```

専用 API Token を使います。pull は `repository:read`、push は `repository:publish`、remote delete は
`repository:delete`、管理 API での予約は `package:create`、team は `team:manage` です。短期 Docker Token は
scope/target と現在の image L0-L4 の両方が許可した action だけを持ちます。

本番は HTTPS を使います。local HTTP test の場合だけ明示設定します。

```json
{
  "insecure-registries": ["localhost:3000"]
}
```

`daemon.json` 変更後は Docker daemon を再起動します。Podman/containerd にも同等の trust 設定があります。

## 作成、tag、push

`containers` を開いて `team/service` を作成し、public/private を選びます。private image は暗黙 L0 を付与
しないため、team から reader/collaborator を追加します。名前 component は小文字です。

ローカルまたは適用 upstream に同名があれば作成を拒否します。上流確認が確定しない場合も予約しません。
mirror-discovered image は pull-only です。

```bash
# Tag local image
docker tag service:latest localhost:3000/containers/team/service:1.0.0

# Push image to RenoP
docker push localhost:3000/containers/team/service:1.0.0
```

image 作成前は push grant、blob upload start、manifest publication を拒否します。管理要求失敗後も retry は
有効で、login や browser dialog を開き直す必要はありません。

## Pull と実行

```bash
# Pull image
docker pull localhost:3000/containers/team/service:1.0.0

# Run container
docker run -d -p 8080:8080 localhost:3000/containers/team/service:1.0.0
```

public image は匿名で読めます。private image は L0-L4 member または管理者が必要です。blob は image-scoped
で、別 image の digest を知っていてもアクセス権になりません。

## OCI 動作

- **Multi-architecture**: Manifest list と OCI index は amd64、arm64 などを参照できます。
- **Chunked upload**: POST/PATCH/PUT の resume と bounded temp storage。
- **Cross-repository mount**: source read と事前作成 destination write が必要です。
- **Delete**: Token capability と image/repository authorization の両方が必要です。
- **Mirror**: upstream origin を付けて stream/catalog 化し、mirror image への push は禁止します。
