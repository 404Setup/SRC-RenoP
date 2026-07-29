---
title: Ограничение частоты
order: 9
category: API
---

# Rate limits

Глобальный middleware `AnomalyMiddleware` (`middleware/anomaly.go`) выполняется до route handlers.

Контроли, по порядку:

1. Concurrent request cap (на процесс)
2. Бан по ошибкам аутентификации (по client IP)
3. Rate limit анонимных запросов (по client IP, token bucket)

При срабатывании лимита ответ может устанавливать `Connection: close`.

## Client IP

Лимиты ключуются по client IP из `utils.ExtractIP`:

- По умолчанию: peer-адрес (`c.IP()`).
- Если peer — loopback (`127.0.0.1` / `::1`) или указан в `server.trusted_proxies`, и задан `server.cdn_ip_header`,
  client IP берётся из этого заголовка. Multi-value заголовки обходятся справа налево; trusted hops пропускаются.

Неверная настройка доверия к proxy может отобразить многих клиентов на один IP или наоборот.
См. [settings.md](./settings.md).

Для Cloudflare → Caddy → RenoP задайте `cdn_ip_header` = `CF-Connecting-IP`. Loopback peers не требуют записи в
`trusted_proxies`.

## 1. Concurrent request cap

| Setting                      | Default | Effect                      |
|------------------------------|---------|-----------------------------|
| `server.max_active_requests` | `512`   | Максимум in-flight запросов |

- Учитывается каждый запрос, входящий в middleware, включая authenticated и static.
- Если текущий счётчик превысит cap, middleware возвращает **`503 Service Unavailable`**.
- Значение `0` в конфигурации нормализуется в `512` при загрузке. Unlimited mode для этого поля нет.

Concurrency limit Fiber выставляется в то же значение при старте сервера.

## 2. Auth-failure ban (по IP)

| Constant               | Value | Meaning                           |
|------------------------|-------|-----------------------------------|
| `MaxFailuresPerMinute` | `5`   | Порог числа failures до бана      |
| Failure store TTL      | 5 min | Окно жизни кэша `AnomalyFailures` |

**Считается failure:** после возврата handler, если итоговый status — **`401`** или **`403`**, счётчик failures для IP
увеличивается.

**Поведение бана:**

- Если счётчик уже **≥ 5** в начале запроса, middleware немедленно возвращает **`403 Forbidden`** и не запускает
  handler.
- Ответ бана формируется до `Next()`, поэтому счётчик снова не увеличивается.
- TTL записи — **5 минут** с последней записи; каждый зафиксированный failure обновляет запись. После expiry счётчик
  удаляется.

Бан применяется ко всем путям, кроме static frontend skip list ниже, включая уже authenticated клиентов. Повторяющиеся
401/403 от handlers банят IP независимо от credentials последующих запросов.

Примечание: имя константы содержит «per minute», но окно хранилища — **5 минут**. Счётчик только растёт до истечения
записи.

## 3. Анонимный rate limit (по IP)

Token bucket через `golang.org/x/time/rate`, один limiter на IP (`GlobalIPLimiter`).

| Constant               | Value | Meaning                                            |
|------------------------|-------|----------------------------------------------------|
| `MaxRequestsPerMinute` | `100` | Sustained rate: 100 tokens в минуту (~1 на 600 ms) |
| `MaxRequestsBurst`     | `60`  | Burst capacity (максимум tokens одновременно)      |

- **Ограничены:** запросы, не прошедшие проверку authenticated (см. ниже).
- **Освобождены:** успешно проверенные Session, Bearer или Basic, либо GET/HEAD `?token=`, разрешающийся в валидную
  session или bearer.
- **При превышении:** **`429 Too Many Requests`**.

### Аутентификация для exemption

Носители совпадают с production auth ([authentication.md](./authentication.md)):

| Carrier                                           | Verified when                                   |
|---------------------------------------------------|-------------------------------------------------|
| Cookie `renop_session` / `Authorization: Session` | Session существует и в пределах idle timeout    |
| `Authorization: Bearer <user>:<secret>`           | Username и secret соответствуют живому account  |
| `Authorization: Bearer <upload-token>`            | Token проиндексирован и не истёк                |
| `Authorization: Basic …`                          | Username и password/secret совпадают            |
| GET/HEAD `?token=`                                | Token — валидный session id или bearer как выше |

Невалидные credentials не дают exemption. Такие запросы всё равно потребляют rate-limit tokens, а 401/403 от handler
увеличивают failure counter.

Idle-expired sessions не считаются authenticated (idle timeout примерно 7 дней; см. документацию auth).

### Жизненный цикл limiter

| Behavior          | Detail                                                |
|-------------------|-------------------------------------------------------|
| Key               | Строка client IP                                      |
| Idle eviction     | Запись без использования **5 минут** удаляется        |
| Periodic cleanup  | Каждые **5 минут**                                    |
| Emergency cleanup | При более чем **10 000** IP cleanup выполняется сразу |

## Static frontend skip

Только для **GET** и **HEAD** следующие пути пропускают auth-failure ban и анонимный rate limit. Они по-прежнему
учитываются в concurrent request cap:

| Path          |
|---------------|
| `/`           |
| `/index.html` |
| `/assets/*`   |
| `/js/*`       |
| `/css/*`      |
| `/svg/*`      |

API routes, пути Maven-репозиториев и другие HTTP methods не пропускаются.

## Status codes

| Code | When                                          |
|------|-----------------------------------------------|
| 429  | Анонимный IP превысил token-bucket rate limit |
| 403  | IP забанен после слишком многих 401/403       |
| 503  | Превышен concurrent request cap процесса      |

Тела ответов обычно пустые. Заголовка `Retry-After` нет.
