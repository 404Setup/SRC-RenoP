---
title: Глобальные команды
order: 12
category: Справочник API
description: Неизменяемые общие префиксы, роли T1-T4, приглашения и лимиты аккаунта
---

# Глобальные команды

Глобальная команда — общая для instance identity совместной публикации. Её неизменяемый префикс используется разными
package engines без копирования участников в команды каждого пакета. Внутри хранятся immutable account IDs, а API
показывает только usernames.

## Роли и владение

T1 читает согласно видимости пакета. T2 публикует и сопровождает версии. T3 управляет участниками T1/T2 и создаёт
пакеты команды. T4 владеет настройками и выдаёт T3/T4.

Должен оставаться хотя бы один T4. T3 не может менять или выдавать T3/T4. System administrator управляет любой командой
без вступления, но при добавлении проверяется лимит целевого аккаунта. Добавление самого администратора не создаёт
лишнее сообщение.

## Лимиты

Значения `super_teams.create_limit` и `super_teams.join_limit` по умолчанию равны пяти и двадцати. Собственная команда
учитывается в обоих счётчиках.

GET /api/super-teams/limits возвращает эффективные лимиты. Manager использует GET
/api/super-teams/users/{username}/limits и PUT /api/super-teams/users/{username}/limits для индивидуальных значений.
`-1` означает наследование, ноль запрещает операцию. GET /api/settings/super-teams и PUT /api/settings/super-teams
изменяют глобальные значения.

## Жизненный цикл

GET /api/super-teams возвращает страницу по префиксу. Обычный аккаунт видит свои команды, administrator — все. POST
/api/super-teams резервирует префикс и назначает caller роль T4. Префикс имеет длину 2–64, состоит из строчных букв,
цифр, дефисов и подчёркиваний, начинается и заканчивается буквой или цифрой и не меняется.

GET /api/super-teams/{prefix} возвращает metadata и usernames участников. PUT /api/super-teams/{prefix} меняет имя и
описание. DELETE /api/super-teams/{prefix} удаляет команду и атомарно отменяет pending invitations.

## Работа с участниками

POST /api/super-teams/{prefix}/members принимает от одного до двадцати usernames и роль T1-T4. Обычный manager создаёт
одноразовое приглашение message center на семь дней; system administrator добавляет сразу.

PUT /api/super-teams/{prefix}/members/{username} меняет роль. DELETE /api/super-teams/{prefix}/members/{username}
удаляет участника или выполняет выход. POST /api/super-teams/invitations/{id}/{decision} принимает `accept` или `reject`
и не допускает двойного применения при повторном или конкурентном ответе.

## Границы API Token

Маршруты требуют `team:manage`, exact target имеет вид `global/{prefix}`. Чтение лимита требует `account:read`,
индивидуальная настройка — `admin:users`, глобальная — `admin:settings`. Targeted Token не может перечислить все команды
или создать другой префикс.

Ошибки возвращают стабильный `X-Renop-Error-Code` и bounded generic body. Client должен использовать HTTP status и
зарегистрированный code, а не показывать raw response text.
