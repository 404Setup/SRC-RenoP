---
title: Docker и OCI Registry
order: 3
category: Руководства
description: Создание образов и работа Docker, Podman, containerd или nerdctl с RenoP
---

# Руководство Docker и OCI Registry

Создайте repository формата `docker`, затем до push создайте каждый image. Примеры используют repository `containers`
и image `team/service`, полное registry name — `containers/team/service`.

## Login и transport

```bash
docker login localhost:3000
# Username: admin
# Password: <your_password_or_API_token>
```

Используйте отдельный API Token: `repository:read` для pull, `repository:publish` для push, `repository:delete` для
удаления, `package:create` для резервирования через API и `team:manage` для участников. Краткосрочный Docker Token
получает только actions, разрешённые scopes/targets и текущей L0-L4 policy image.

В production используйте HTTPS. Только для локального HTTP теста:

```json
{
  "insecure-registries": ["localhost:3000"]
}
```

После `daemon.json` перезапустите Docker daemon. Podman/containerd имеют аналогичные trust settings.

## Создание, tag и push

Откройте `containers`, создайте `team/service` и выберите public/private. Private image не выдаёт неявный L0; добавьте
readers или collaborators в team. Компоненты имени должны быть в нижнем регистре.

Имя, занятое локально или на подходящем upstream, отклоняется. Неопределённая проверка также не резервирует имя.
Mirror-discovered images остаются pull-only.

```bash
# Tag local image
docker tag service:latest localhost:3000/containers/team/service:1.0.0

# Push image to RenoP
docker push localhost:3000/containers/team/service:1.0.0
```

До создания image RenoP не выдаёт push, не начинает blob upload и не принимает manifest. После ошибки retry остаётся
валидным, повторно открывать login или browser dialog не нужно.

При любой политике проверки создание image возвращает `202 Accepted` и не резервирует имя до одобрения. В режиме
`new_packages` следующие push выполняются обычно. В режиме `every_version` каждая отправка manifest также возвращает
идентификатор проверки и до одобрения отсутствует в pull, tag-list и каталоге. При одобрении повторно проверяются
издатель и связанные blobs, после чего tag публикуется атомарно. Отклонение удаляет только виртуальный manifest, не
затрагивая общие blobs и существующие tags. Импорт из зеркала проверку не проходит.

## Pull и запуск

```bash
# Pull image
docker pull localhost:3000/containers/team/service:1.0.0

# Run container
docker run -d -p 8080:8080 localhost:3000/containers/team/service:1.0.0
```

Public image читается анонимно. Private image требует L0-L4 участника или администратора. Blob остаётся image-scoped:
знание digest другого image не даёт доступа.

## Поведение OCI

- **Multi-architecture**: Manifest lists и OCI indexes для amd64, arm64 и других платформ.
- **Chunked uploads**: Возобновляемые POST/PATCH/PUT с bounded temp storage.
- **Cross-repository mounts**: Нужны source read и write в предварительно созданную destination.
- **Deletion**: Требуются и Token capability, и image/repository authorization.
- **Mirrors**: Upstream response stream/catalog с origin metadata; push в mirror image запрещён.
