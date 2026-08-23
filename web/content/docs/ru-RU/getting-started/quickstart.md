---
title: Быстрый старт
order: 3
category: Начало работы
description: Первый запуск, пароль администратора и стандартные репозитории
---

# Быстрый старт

## 1. Первый запуск

Рекомендуется задать пароль администратора перед запуском:

```bash
RENOP_DEFAULT_ADMIN_PASSWORD='your-admin-password' ./renop
```

Откройте в браузере: `http://localhost:3000`

## 2. Стандартные репозитории

| URL                               | Доступ    | Назначение                                   |
|:----------------------------------|:----------|:---------------------------------------------|
| `http://localhost:3000/releases`  | `PUBLIC`  | Релизы Maven (перезапись запрещена)          |
| `http://localhost:3000/snapshots` | `PUBLIC`  | Снапшоты Maven (перезапись разрешена)        |
| `http://localhost:3000/private`   | `PRIVATE` | Приватный репозиторий Maven (требуется вход) |

- Индекс Cargo: `http://localhost:3000/index/`
- Реестр Docker: `http://localhost:3000/v2/`

## 3. Проверка работоспособности

```bash
curl -s http://localhost:3000/api/status/health
# Ответ: "UP"
```
