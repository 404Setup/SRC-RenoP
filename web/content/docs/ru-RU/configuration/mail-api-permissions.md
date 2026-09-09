---
title: Разрешения почтовых API
order: 7
category: Настройка
description: Авторизация провайдеров, проверенные контракты API и требования к подключению почты
---

# Разрешения почтовых API

Здесь описаны восемь API-провайдеров из раздела [Отправка почты](mail.md), а также SMTP OAuth.
Адреса и названия разрешений сверены с указанными официальными источниками 2026-09-08.
RenoP выполняет HTTP-запросы напрямую, без SDK провайдера.

## Таблица разрешений

Предоставьте настроенному отправителю право отправки и добавьте разрешения для используемых запросов.
Принятие запроса не означает доставку. Разрешение на чтение состояния почтового ящика также может дать владельцу учётных
данных доступ к содержимому писем.

| Провайдер           | Отправка                                      | Состояние                                           | Квота / баланс                                                                  |
|---------------------|-----------------------------------------------|-----------------------------------------------------|---------------------------------------------------------------------------------|
| Cloudflare          | Email Sending: Edit с ограничением на аккаунт | Первый ответ                                        | Соответствующий запрос в RenoP не реализован                                    |
| Microsoft Graph     | `Mail.Send`                                   | `Mail.Read` для расширенных свойств                 | Соответствующий запрос не реализован                                            |
| Amazon SES v2       | `ses:SendEmail`                               | `ses:GetMessageInsights`                            | `ses:GetAccount`; без запроса баланса                                           |
| SendGrid            | `mail.send`                                   | `messages.read` и дополнение истории Email Activity | `user.credits.read`; без денежного баланса                                      |
| Gmail               | `https://www.googleapis.com/auth/gmail.send`  | `https://www.googleapis.com/auth/gmail.metadata`    | Без запроса остатка отправлений или баланса                                     |
| Alibaba Direct Mail | `dm:SingleSendMail`                           | `dm:SenderStatisticsDetailByParam`                  | `dm:DescAccountSummary`; дополнительно `bss:DescribeAcccount`                   |
| Tencent SES         | `ses:SendEmail`                               | `ses:GetSendEmailStatus`                            | Дополнительно `finance:DescribeAccountBalance`; без запроса остатка отправлений |
| Feishu / Lark       | `mail:user_mailbox.message:send`              | `mail:user_mailbox.message:readonly`                | Соответствующий запрос не реализован                                            |

Для OAuth нужна первоначальная авторизация пользователя, а для автоматического обновления — токен обновления.
RenoP обновляет сохранённые учётные данные; настройки почты не предоставляют интерактивный OAuth callback.
Используйте поток кода авторизации зарегистрированного приложения и точный URI перенаправления, затем сохраните
полученный токен конфиденциально.

## Cloudflare Email

Включите Email Sending для аккаунта, подключите домен отправителя и добавьте требуемые записи DNS.
Создайте токен **Email Sending: Edit** только для этого аккаунта, укажите его в `api_key` и задайте соответствующий
`account_id`.
Разрешения Email Routing не дают права исходящей отправки. Управление доменом и DNS отделено от разрешений для работы
отправителя.

RenoP вызывает `POST /accounts/{account_id}/email/sending/send` относительно `https://api.cloudflare.com/client/v4`.
Передаются структурированные адреса, текст и HTML; читаются `message_id`, `delivered`, `permanent_bounces`, `queued` и
`suppressed_recipients`.
Письмо в очереди провайдера остаётся в состоянии `queued_provider`; отдельные подписки Cloudflare на события не
используются.
См. [настройку и права токена](https://developers.cloudflare.com/email-service/get-started/send-emails/)
и [схему отправки](https://developers.cloudflare.com/api/resources/email_sending/methods/send/).

## Microsoft Graph

Зарегистрируйте приложение Entra в облаке почтового ящика. Личные Outlook.com-аккаунты используют делегированную
авторизацию;
организации Microsoft 365 могут использовать делегированные разрешения или разрешения приложения.
Для делегирования запросите `Mail.Send Mail.Read offline_access`, сохраните `client_id`, применимый `client_secret` и
`refresh_token`, задайте `mailbox: me`.
Tenant `common` подходит для совместимой глобальной регистрации; политика организации может требовать согласия
администратора.

Для доступа приложения нужны разрешения приложения `Mail.Send` и `Mail.Read` с согласием администратора.
Укажите действительный ID tenant, ID клиента, секрет и ID пользователя либо адрес целевого `mailbox`.
RenoP запрашивает `client_credentials` с областью `/.default` выбранного ресурса Graph.
Токены приложения не поддерживают `/me`. Ограничьте доступ к ящикам штатными средствами контроля доступа приложений
Exchange.

| Облако                                       | База API                                       | Служба выдачи токенов               |
|----------------------------------------------|------------------------------------------------|-------------------------------------|
| Глобальное / Outlook.com / Microsoft 365 GCC | `https://graph.microsoft.com/v1.0`             | `https://login.microsoftonline.com` |
| Правительство США L4 / GCC High              | `https://graph.microsoft.us/v1.0`              | `https://login.microsoftonline.us`  |
| Правительство США L5 / DoD                   | `https://dod-graph.microsoft.us/v1.0`          | `https://login.microsoftonline.us`  |
| Китай / 21Vianet                             | `https://microsoftgraph.chinacloudapi.cn/v1.0` | `https://login.chinacloudapi.cn`    |

Отправка использует `POST /me/sendMail` или `POST /users/{mailbox}/sendMail`, настроенный адрес `from` и сохранение
копии в отправленных.
Запрос состояния фильтрует отправленные по расширенному свойству отслеживания RenoP; `Mail.ReadBasic` не покрывает это
свойство.
Для делегированной отправки из другого ящика нужны `Mail.Send.Shared`, соответствующее право совместного чтения и
Exchange Send As/Send on Behalf; прямой доступ к ящику дополнительно требует Full Access.
HTTP 202 означает принятие; обнаружение сохранённого письма означает только `sent`.
См. [sendMail](https://learn.microsoft.com/en-us/graph/api/user-sendmail), [права расширенных свойств](https://learn.microsoft.com/en-us/graph/api/singlevaluelegacyextendedproperty-get?view=graph-rest-1.0),
[совместную отправку](https://learn.microsoft.com/en-us/graph/outlook-send-mail-from-other-user)
и [облачные адреса](https://learn.microsoft.com/en-us/graph/deployments).

## Amazon SES

Подтвердите отправителя в выбранном регионе и получите производственный доступ для отправки за пределы песочницы SES.
Задайте ключ IAM и секрет; временным учётным данным также нужен токен сеанса, обновляемый администратором до истечения
срока.
Учётные данные SES SMTP не заменяют ключи SES API.

Разрешите `ses:SendEmail` для допустимых ресурсов отправителя. Для `ses:GetAccount` и `ses:GetMessageInsights`
используйте ресурс `*`: ограничения на отдельную identity для этих запросов не поддерживаются.
Сведения о сообщениях требуют соответствующей функции Virtual Deliverability Manager, оплачиваемой отдельно от обычной
отправки.
RenoP подписывает запросы AWS SigV4 с сервисом `ses` и настроенным регионом.

`POST /v2/email/outbound-emails` возвращает `MessageId`; `GET /v2/email/account` предоставляет `SendingEnabled` и лимит
скользящего 24-часового периода.
`GET /v2/email/insights/{message_id}/` возвращает события получателя. Жёсткие лимиты SES действуют и при принудительной
отправке.
См. [действия IAM](https://docs.aws.amazon.com/service-authorization/latest/reference/list_sesv2.html),
[запрос аккаунта](https://docs.aws.amazon.com/ses/latest/APIReference-V2/API_GetAccount.html),
[сведения о сообщениях](https://docs.aws.amazon.com/ses/latest/APIReference-V2/API_GetMessageInsights.html)
и [региональные адреса](https://docs.aws.amazon.com/general/latest/gr/ses.html).

## Twilio SendGrid

Подтвердите отправителя или домен и создайте ограниченный API-ключ с нужными `mail.send`, `messages.read` и
`user.credits.read`.
Email Activity API требует покупки дополнительной истории; ключа или тарифа только для отправки недостаточно.
Для отправки в ЕС нужны подходящий тариф Pro или выше, субпользователь ЕС и IP ЕС; используйте его ключ в предустановке
EU.

Относительно выбранной базы `/v3` вызываются `POST /mail/send`, `GET /messages` и `GET /user/credits`.
Полученный `X-Message-Id` является префиксом ID событий отдельных получателей; дополнительно проверяется точный адрес
получателя.
Значения запроса заключаются в двойные кавычки, например `msg_id LIKE "submission-id%"`.
Credits — это количество доступных отправлений, а не денежный баланс для вывода.
См. [области API-ключа](https://www.twilio.com/docs/sendgrid/api-reference/api-key-permissions),
[доступ к активности](https://www.twilio.com/docs/sendgrid/api-reference/email-activity/filter-all-messages),
[синтаксис запросов](https://www.twilio.com/docs/sendgrid/for-developers/sending-email/getting-started-email-activity-api),
[остаток отправлений](https://www.twilio.com/docs/sendgrid/api-reference/users-api/retrieve-your-credit-balance)
и [региональную отправку](https://www.twilio.com/docs/sendgrid/api-reference/mail-send/mail-send).

## Google Gmail

Включите Gmail API в проекте Google Cloud клиента OAuth и настройте экран согласия.
Получите согласие владельца ящика на `https://www.googleapis.com/auth/gmail.send` и
`https://www.googleapis.com/auth/gmail.metadata`.
Запросите `access_type=offline`; если первый токен обновления не выдан, получите повторное согласие.
Внешнее приложение в состоянии Testing может получать токены обновления со сроком семь дней; требования публикации и
проверки ограниченных областей зависят от приложения.

Сохраните ID клиента, секрет и токен обновления. RenoP обменивает токен на `https://oauth2.googleapis.com/token`.
Отправитель должен быть авторизованным ящиком либо разрешённым псевдонимом отправки.
Почтовый коннектор не реализует JSON-ключи сервисного аккаунта или генерацию JWT для делегирования на весь домен.

`POST /users/me/messages/send` относительно `https://gmail.googleapis.com/gmail/v1` принимает base64url MIME и
возвращает ID сообщения.
RenoP проверяет метку `SENT` через `GET /users/me/messages/{message_id}` с `format=minimal`.
Это подтверждает `sent`, а не доставку. Квоты запросов API отличаются от лимитов отправки ящика.
См. [области отправки](https://developers.google.com/workspace/gmail/api/reference/rest/v1/users.messages/send),
[доступ к метаданным](https://developers.google.com/workspace/gmail/api/reference/rest/v1/users.messages/get)
и [настройку OAuth](https://developers.google.com/identity/protocols/oauth2/web-server).

## Alibaba Cloud Direct Mail

Активируйте Direct Mail, подтвердите домен и настройте отправителя в выбранном регионе.
Используйте ключ RAM с `dm:SingleSendMail`, `dm:DescAccountSummary` и `dm:SenderStatisticsDetailByParam` для ресурса
`*`.
Дополнительная сверка баланса требует **`bss:DescribeAcccount`**: три буквы `c` подряд — официальное написание
разрешения.
Имя API-действия остаётся `QueryAccountBalance`, версия `2017-12-14`; это не разрешение `dm:`.

RenoP выполняет подписанные RPC POST-запросы Direct Mail версии `2015-11-23`.
`SingleSendMail` возвращает `EnvId`, а `DescAccountSummary` — бесплатный остаток и состояние аккаунта.
Публичная статистика не содержит надёжного ID отдельного письма, поэтому после запроса результат будет `unknown`, без
сопоставления только по получателю.
Имя отправителя ограничено 15 символами. Адрес биллинга выбирается по коммерческому региону аккаунта, отдельно от
региона отправки; валюта модели должна совпадать с возвращаемой.

См. [отправку и ограничения](https://www.alibabacloud.com/help/en/direct-mail/api-dm-2015-11-23-singlesendmail),
[права на квоты](https://www.alibabacloud.com/help/en/direct-mail/api-dm-2015-11-23-descaccountsummary),
[статистику](https://www.alibabacloud.com/help/en/direct-mail/api-dm-2015-11-23-senderstatisticsdetailbyparam),
[авторизацию баланса](https://www.alibabacloud.com/help/en/user-center/developer-reference/api-bssopenapi-2017-12-14-queryaccountbalance)
и [адреса](https://www.alibabacloud.com/help/en/direct-mail/api-dm-2015-11-23-endpoint).

## Tencent Cloud SES

Подтвердите домен и адрес отправителя, предоставьте CAM-действия `ses:SendEmail` и `ses:GetSendEmailStatus`.
В официальной CAM-политике обе операции используют ресурс `*`.
Для дополнительного чтения баланса нужно `finance:DescribeAccountBalance`, хотя подписываемый API-сервис называется
`billing`.
SecretId укажите в `api_key`, SecretKey — в `api_secret`, а временный Token — в `session_token`, если он используется.

Обычным аккаунтам нужен утверждённый шаблон провайдера. Внесите его положительный ID в `tencent_template_id`.
Создайте HTML-шаблон со следующими текстовыми переменными, отправьте на проверку и после одобрения используйте его ID:

```html
<div style="padding:24px;background:#f3f5f8;font-family:Arial,sans-serif">
  <div style="max-width:600px;margin:auto;padding:24px;background:white;border:1px solid #dfe5ef;border-radius:20px">
    <p style="color:#3158c9;font-weight:bold">RenoP</p>
    <h1>{{subject}}</h1>
    <div style="white-space:pre-wrap;overflow-wrap:anywhere">{{text}}</div>
  </div>
</div>
```

RenoP передаёт тему и полный текст уведомления, экранируя HTML-символы перед подстановкой.
Переменные размещаются только в тексте, не в атрибутах или скриптах. Весь JSON `TemplateData` ограничен 800 байтами
UTF-8; длинные уведомления отклоняются до отправки без усечения.
Пример не гарантирует одобрения провайдера. В режиме шаблона оформление определяется утверждённым HTML.
Нулевой ID сохраняет `Simple` для старых аккаунтов со специальным разрешением на произвольное содержимое и не выдаёт это
разрешение обычным аккаунтам.

`SendEmail` и `GetSendEmailStatus` используют версию `2020-10-02` и подпись TC3 сервиса `ses`.
Состояние сопоставляется по ID и получателю; принятие, доставка, отклонение и временная задержка различаются.
`DescribeAccountBalance` использует версию `2018-07-09`; доступный баланс в центах переводится в миллионные доли валюты.
Китайские и международные адреса используют отдельные системы аккаунтов и валюты.
См. [SendEmail](https://intl.cloud.tencent.com/document/product/1084/39408),
[поля шаблонов и состояний](https://intl.cloud.tencent.com/document/product/1084/39418),
[действия SES CAM](https://intl.cloud.tencent.com/document/product/598/57150),
[права биллинга](https://cloud.tencent.com/document/product/555/61542)
и [API баланса](https://cloud.tencent.com/document/api/555/20253).

## Feishu и Lark Mail

Создайте и опубликуйте внутреннее приложение для организации с активным ящиком Mail.
Предоставьте `mail:user_mailbox.message:send`, `mail:user_mailbox.message:readonly` и `offline_access`.
Разрешите отправителю использовать приложение, включите обновление токена, если переключатель доступен в настройках
безопасности, опубликуйте изменения и получите согласие пользователя.
Для используемых операций отправки и проверки доставки нужен токен **пользователя**, а не tenant.

Задайте ID приложения, секрет, токен обновления и адрес ящика либо `me`.
Feishu и Lark имеют отдельные регистрации приложений, учётные данные и базы API:
`https://open.feishu.cn/open-apis` и `https://open.larksuite.com/open-apis`.
Сейчас RenoP обновляет токены через совместимый `POST /authen/v2/oauth/token` и сохраняет каждый новый токен до
отправки.

`POST /mail/v1/user_mailboxes/{mailbox}/messages/send` принимает base64url MIME с заполнением и возвращает
`data.message_id`.
`GET /mail/v1/user_mailboxes/{mailbox}/messages/{message_id}/send_status` возвращает сведения по получателям:
4 означает доставку, 3 и 6 — ошибку, а 0, 1, 2 и 5 продолжают опрос.
Состояние доставки устанавливается только по результатам доставки конкретному получателю.
См. [отправку](https://open.feishu.cn/document/server-docs/mail-v1/user_mailbox-message/send),
[состояние доставки](https://open.feishu.cn/document/uAjLw4CM/ukTMukTMukTM/reference/mail-v1/user_mailbox-message/send_status),
[доставку в Lark](https://open.larksuite.com/document/uAjLw4CM/ukTMukTMukTM/reference/mail-v1/user_mailbox-message/send_status)
и [обновление токенов](https://open.feishu.cn/document/authentication-management/access-token/refresh-user-access-token).

## SMTP OAuth

Для Gmail SMTP запросите `https://mail.google.com/`; области только для отправки через Gmail API недостаточно.
Для Outlook.com/Microsoft 365 SMTP нужны `https://outlook.office.com/SMTP.Send` и `offline_access`.
Также включите SMTP AUTH, если этого требуют политики организации или ящика Microsoft 365.
Имя пользователя SMTP — адрес ящика; задайте соответствующие данные клиента и обновления.

Автоматическое обновление SMTP использует делегированные данные, без получения токенов приложения через
`SMTP.SendAsApp`.
Можно указать токен доступа, управляемый внешней системой, но его обновление остаётся обязанностью администратора.
См. [Google SASL OAuth](https://developers.google.com/workspace/gmail/imap/xoauth2-protocol)
и [Microsoft SMTP OAuth](https://learn.microsoft.com/en-us/exchange/client-developer/legacy-protocols/how-to-authenticate-an-imap-pop-smtp-application-by-using-oauth).

## Проверка и эксплуатация

Сохраните аккаунт, отправьте тест из панели администратора, проверьте итоговое состояние задания и результат сверки.
Подтвердите получение в своём почтовом ящике. Ответ 202 при постановке в очередь означает только принятие очередью.
Без права чтения состояния принятая отправка может завершиться как `unknown`; не повторяйте её только из-за ошибки
запроса состояния.
Посмотрите код провайдера в глобальных журналах и добавьте только недостающее разрешение.

Сверка документации и локальные HTTP-тесты не доказывают пригодность конкретных ключей, подписки, домена или
регионального аккаунта.
Реальные учётные данные провайдеров при этой проверке не использовались. Отправку, квоты, баланс и доставку нужно
проверить с аккаунтами развёртывания.
Неизвестные лимиты обрабатываются по правилам [Отправки почты](mail.md), а известные сохраняются при ошибке сверки.
