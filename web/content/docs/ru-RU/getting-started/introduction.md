---
title: Введение
order: 1
category: Начало работы
description: RenoP как интегрированная self-hosted платформа публикации пакетов
---

# Введение в RenoP

RenoP — интегрированный self-hosted сервер публикации и распространения пакетов. Его модель ближе к частному сервису
класса Central, чем к Maven-only repository: один Go process включает UI, identity, команды, проверки, каталоги,
зеркала, storage, аудит и обновления.

## Поддерживаемые протоколы

- **Maven / Gradle**: проверенные глобальные домены, современный каталог, classic layout, Maven 2 paths, зеркала,
  Javadoc и отделённая OpenPGP-проверка.
- **Cargo**: Sparse Index, явное владение, публикация, поиск, yank/unyank, зеркала и Cargodoc.
- **npm**: явное резервирование, неизменяемые версии, scoped private packages, dist-tag, команды и зеркала.
- **Docker / OCI**: Distribution v2, резервирование образов, private teams, chunked blobs, cross-repository mounts,
  multi-architecture manifests и зеркала.
- **Files**: неструктурированное заменяемое хранилище с зеркалами, без Maven metadata и signature workflow.

## Хранилище и базы

- **Storage**: потоковый local Disk или S3-compatible backend для репозитория.
- **Database**: встроенный SQLite по умолчанию, внешние MySQL и PostgreSQL.
- **Consistency**: repository gates координируют uploads, удаления, mirror commits, GPG и смену engine/storage без
  полного
  чтения крупных объектов в память.

## Основные возможности

| Возможность                | Описание                                                                    |
|:---------------------------|:----------------------------------------------------------------------------|
| **Один сервис**            | Встроенные frontend и protocol API без отдельного runtime                   |
| **Глобальная identity**    | Публичные профили по имени и неизменяемые внутренние user ID                |
| **Точные права**           | Права репозитория, команды L0-L4, целевые и истекающие API Token            |
| **Проверенная публикация** | Домены Maven, upstream name conflicts и необязательный OpenPGP quarantine   |
| **Эксплуатация**           | Нативная служба, scheduled tasks, durable audit/messages и in-place updates |
| **Защита**                 | Bounded streaming, rate limit, bans, trusted proxy и sandboxed viewers      |

## Навигация по документации

- [Установка](./install.md) — Пакеты, платформы и source build
- [Быстрый старт](./quickstart.md) — Первый запуск, администратор и создание репозиториев
- [Архитектура](./architecture.md) — Модули, авторизация, storage и задачи
- [Конфигурация](../configuration/overview.md) — Проверяемые настройки и переменные
- [Maven и Gradle](../guides/maven-client.md) — Проверенные домены и JVM clients
- [Cargo](../guides/cargo-registry.md) — Sparse registry и lifecycle crate
- [Docker и OCI](../guides/docker-registry.md) — Резервирование, login, push и pull
- [Реестр npm](../guides/npm-registry.md) — Резервирование, настройка клиента, публикация и команды
