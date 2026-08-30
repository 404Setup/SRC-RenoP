---
title: Исходящий прокси
order: 4
category: Конфигурация
description: Именованные HTTP, HTTPS и SOCKS5 прокси и маршрутизация зеркал
---

# Настройка исходящего прокси

Настройте прокси, если RenoP должен обращаться к Maven Central, crates.io, Docker, GitHub, GitLab или серверам GPG через
контролируемый канал. Процесс использует общие ограниченные HTTP transports для каждой политики маршрутизации.

## Схема настройки

```yaml
proxy:
  selected: "corp_http"
  proxies:
    - name: "corp_http"
      url: "http://10.0.0.1:8080"
      username: "proxy-user"
      password: "proxy-password"
    - name: "socks_proxy"
      url: "socks5://10.0.0.2:1080"
      username: ""
      password: ""
```

`selected` — глобальное значение; пустая строка означает прямое соединение. Допускается до 16 прокси с уникальными
именами. URL использует `http`, `https` или `socks5`, содержит подходящие host и port, но не credentials, path, query
или
fragment. Секреты задавайте только через `username` и `password`.

## Правила маршрутизации

| Селектор   | Результат                           |
|:-----------|:------------------------------------|
| `""`       | Наследовать `proxy.selected`        |
| `direct`   | Обойти все прокси                   |
| Имя прокси | Использовать указанную конфигурацию |

Изменение выбора или credentials сбрасывает связанные общие клиенты. Неизвестное имя отклоняется, а не заменяется
молчаливым прямым соединением.

## Выбор для зеркала

Каждое зеркало может переопределить маршрут полем `proxy`:

```yaml
repositories:
  releases:
    name: releases
    format: maven
    mirrors:
      - name: "maven-central"
        url: "https://repo1.maven.org/maven2"
        proxy: "corp_http"
      - name: "internal"
        url: "https://mirror.internal/maven"
        proxy: "direct"
      - name: "default-route"
        url: "https://plugins.gradle.org/m2"
        proxy: ""
```

Используйте `direct` для внутренних сервисов, которые не должны идти через глобальный прокси. Не помещайте секреты в
URL зеркал или журналы.
