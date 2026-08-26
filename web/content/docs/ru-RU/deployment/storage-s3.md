---
title: Архитектура хранилища
order: 3
category: Развёртывание
description: Локальные файлы и S3-совместимые object backends для каждого репозитория
---

# Архитектура хранилища

RenoP поддерживает локальный диск и S3-совместимые object services. Каждый репозиторий выбирает backend, а repository
gate сериализует изменения с активными операциями.

## Локальная файловая система

Корень задаётся `storage_path` в `config.yaml`, по умолчанию `storage`.

### Организация

- **Maven/files**: `{storage_path}/{repo}/{path}`
- **Cargo**: index и archives изолированы в каталоге репозитория
- **Docker**: blobs, manifests и references изолированы и проверяются по image

Физические имена — внутренняя реализация. Используйте protocol API и не изменяйте каталог напрямую.

### Надёжность записи

- Upload использует ограниченные временные файлы и до commit проверяет size, hash и policy.
- Финальная публикация атомарна, если это поддерживает файловая система.
- Mirror commits, удаления, миграции и GPG publications синхронизируются со сменой backend.

---

## S3-совместимое object storage

S3 подходит для managed object storage. Multi-node также требует внешнюю базу и coordination в рамках гарантий RenoP;
сам S3 не превращает один процесс в cluster.

### Поставщики

- **AWS S3**
- **MinIO**
- **Cloudflare R2**
- любой сервис с необходимым S3 API

### Пример (`repositories.yaml`)

```yaml
repositories:
  releases:
    name: releases
    s3:
      enabled: true
      endpoint: "https://minio.internal:9000"
      bucket: "renop-storage"
      key_prefix: "releases/"
      region: "us-east-1"
      access_key_id: "ACCESS_KEY"
      secret_access_key: "SECRET_KEY"
      force_path_style: true
      redirect_downloads: false
```

Bucket создаётся заранее, а credentials должны разрешать read/write/list/delete под `key_prefix`. Используйте TLS и
secret manager, не добавляйте ключи в Git.

### Режимы скачивания

- **Proxy streaming (`redirect_downloads: false`)**: RenoP авторизует и передаёт данные из S3 клиенту. Bucket остаётся
   закрытым, S3 URL не раскрывается.
- **Direct redirect (`redirect_downloads: true`)**: после авторизации RenoP отвечает `302 Found` на краткосрочный
   presigned URL, уменьшая свою нагрузку на сеть.
