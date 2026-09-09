/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package mail

import (
	"bytes"
	"errors"
	"html/template"
	"net/url"
	"slices"
	"strings"

	"renop/internal/locale"
)

// Scenes are the stable account-routing and template identifiers.
var Scenes = []string{"registration_verify", "registration_success", "password_reset", "password_changed", "email_verify", "email_changed", "quota_changed", "review_status", "review_requested", "permission_changed", "account_banned", "account_unbanned", "collaboration_invitation", "super_team_invitation", "pending_reviews", "unusual_login", "security_changed", "account_retired", "notification", "test"}

// TemplateData contains text-only substitutions and a same-instance action URL.
type TemplateData struct {
	Locale   string `json:"locale,omitempty"`
	Username string `json:"username"`
	Code     string `json:"code"`
	URL      string `json:"url"`
	Detail   string `json:"detail"`
}

type templateCopy struct{ Title, Body string }

var englishTemplates = []templateCopy{
	{"Verify your email", "Use the verification code below to finish creating your account. The code expires in 10 minutes."},
	{"Welcome to RenoP", "Your account is ready. You can now sign in and manage your packages."},
	{"Reset your password", "Use the code below to set a new password. The code expires in 10 minutes. If you did not request this change, you can ignore this email."},
	{"Password changed", "Your account password has been changed. If you did not make this change, recover your account immediately."},
	{"Verify your new email", "Use the code below to confirm this security email address. The code expires in 10 minutes."},
	{"Security email changed", "Your account's security email address has been changed."},
	{"Publication quota updated", "Your publication quota has been updated. Open your account to view the current limits."},
	{"Ticket status updated", "Your ticket status has changed. Open tickets to view the result."},
	{"Ticket submitted", "Your ticket has been submitted. Open tickets to follow its progress."},
	{"Permissions updated", "Your account permissions have changed. Open your account to view your current access."},
	{"Account banned", "Your account has been banned. Sign-in and publication may be restricted."},
	{"Account unbanned", "Your account ban has been lifted. You can sign in again."},
	{"Collaboration invitation", "You have been invited to collaborate on a package. Open your messages to review the invitation."},
	{"Global team invitation", "You have been invited to join a global team. Open your messages to review the invitation."},
	{"Tickets need your attention", "A ticket is waiting. Open tickets to take it over and continue."},
	{"Sign-in from a new network", "Your account was used to sign in from a network that differs from your previous sign-in. If this was not you, revoke the session and change your password."},
	{"Account security updated", "Your account security settings have changed. Review your login methods if you did not make this change."},
	{"Account retired", "Your account has been retired. Existing sessions and third-party sign-in bindings have been revoked."},
	{"New notification", "You have a new notification. Open your messages to read it."},
	{"RenoP test email", "Your mail account successfully submitted this test email through the RenoP sending queue."},
}

var chineseTemplates = []templateCopy{
	{"验证注册邮箱", "请使用下方验证码完成账号注册。验证码在 10 分钟内有效。"},
	{"欢迎使用 RenoP", "您的账号已创建，现在可以登录并管理软件包。"},
	{"重置密码", "请使用下方验证码设置新密码。验证码在 10 分钟内有效。如果您没有申请重置密码，请忽略此邮件。"},
	{"密码已更改", "您的账号密码已更改。如果这不是您本人操作，请立即恢复账号。"},
	{"验证新邮箱", "请使用下方验证码确认此安全邮箱。验证码在 10 分钟内有效。"},
	{"安全邮箱已更改", "您的账号安全邮箱已更改。"},
	{"发布配额已更新", "您的发布配额已更新。请前往账号页面查看当前限额。"},
	{"工单状态已更新", "您的工单状态已变更，请前往工单页面查看结果。"},
	{"工单已提交", "您的工单已提交，请前往工单页面查看进度。"},
	{"账号权限已更新", "您的账号权限已更改。请前往账号页面查看当前权限。"},
	{"账号已封禁", "您的账号已被封禁，登录与发布操作可能受到限制。"},
	{"账号已解封", "您的账号封禁已解除，现在可以重新登录。"},
	{"协作邀请", "您收到了软件包协作邀请。请前往消息页面查看并处理邀请。"},
	{"全局团队邀请", "您收到了加入全局团队的邀请。请前往消息页面查看并处理邀请。"},
	{"有待处理的工单", "您有待处理的工单，请前往工单页面接管并处理。"},
	{"来自新网络的登录", "您的账号从与上次不同的网络登录。如果这不是您本人操作，请撤销该会话并更改密码。"},
	{"账号安全设置已更新", "您的账号安全设置已更改。如果这不是您本人操作，请检查登录方式。"},
	{"账号已注销", "您的账号已注销，现有会话与第三方登录绑定均已撤销。"},
	{"新通知", "您收到了一条新通知。请前往消息页面查看。"},
	{"RenoP 测试邮件", "邮件账号已通过 RenoP 发件队列成功提交此测试邮件。"},
}

// Inline colors mirror @renop/ui tokens because email clients may strip stylesheets.
var emailTemplate = template.Must(template.New("email").Parse(`<!doctype html>
<html lang="{{.Locale}}">
<head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<meta name="color-scheme" content="light dark"><title>{{.Title}}</title>
<style>
@media(max-width:480px){.mail-background{padding:24px 12px!important}.mail-content{padding:24px 20px!important}.mail-compact{padding:20px!important}.mail-title{font-size:24px!important}}
@media(prefers-color-scheme:dark){
.mail-background{background:#000000!important;color:#f9fafb!important}
.mail-card{background:#1f2937!important;border-color:#374151!important}
.mail-brand,.mail-code{background:#172b49!important;border-color:#28466f!important;color:#bfdbfe!important}
.mail-rule{border-color:#374151!important}.mail-notice{border-top-color:#f59e0b!important}
.mail-muted{color:#9ca3af!important}.mail-button{background:#2563eb!important;border-color:#3b82f6!important;color:#ffffff!important}
}
</style>
</head>
<body class="mail-background" style="margin:0;padding:40px 16px;background:#f3f4f6;color:#111827;font-family:system-ui,-apple-system,'Segoe UI',Arial,sans-serif;font-size:16px;line-height:1.65">
<table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="width:100%;max-width:600px;margin:0 auto;border-collapse:separate;border-spacing:0;table-layout:fixed">
<tr><td style="padding:0 4px 18px;overflow-wrap:anywhere;word-break:break-word">
<span class="mail-brand" style="display:inline-block;padding:6px 14px;border:1px solid #dbeafe;border-radius:999px;background:#eff6ff;color:#1d4ed8;font-size:14px;font-weight:700;letter-spacing:0.1px">{{.SiteName}}</span>
</td></tr>
<tr><td class="mail-card{{if eq .Style "notice"}} mail-notice{{end}}" style="background:#ffffff;border:1px solid #e5e7eb;border-radius:18px;{{if eq .Style "notice"}}border-top:4px solid #d97706;{{end}}">
<table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="width:100%;border-collapse:separate;border-spacing:0;table-layout:fixed">
<tr><td class="mail-content{{if eq .Style "compact"}} mail-compact{{end}}" style="padding:{{if eq .Style "compact"}}22px{{else}}32px{{end}};overflow-wrap:anywhere;word-break:break-word">
<h1 class="mail-title" style="margin:0 0 20px;font-size:{{if eq .Style "compact"}}24px{{else}}28px{{end}};font-weight:700;line-height:1.25;letter-spacing:-0.5px">{{.Title}}</h1>
{{if .Username}}<p class="mail-muted" style="margin:0 0 12px;color:#6b7280;font-size:14px">{{.Username}},</p>{{end}}
<p style="margin:0 0 20px">{{.Body}}</p>
{{if .Code}}<div class="mail-code" style="padding:18px 12px;margin:24px 0;border:1px solid #dbeafe;border-radius:12px;background:#eff6ff;color:#1d4ed8;text-align:center;font:700 30px ui-monospace,Menlo,Consolas,monospace;line-height:1.5;letter-spacing:4px;overflow-wrap:anywhere;word-break:break-word">{{.Code}}</div>{{end}}
{{if .Detail}}<div class="mail-rule" style="margin:24px 0;padding-top:20px;border-top:1px solid #e5e7eb;white-space:pre-wrap">{{.Detail}}</div>{{end}}
{{if .URL}}<a class="mail-button" href="{{.URL}}" style="display:inline-block;box-sizing:border-box;max-width:100%;padding:11px 24px;border:1px solid #2563eb;border-radius:999px;background:#2563eb;color:#ffffff;text-decoration:none;font-size:15px;font-weight:600;text-align:center;line-height:1.5">{{.OpenLabel}}</a>{{end}}
</td></tr></table>
</td></tr>
<tr><td class="mail-muted" style="padding:20px 4px 0;color:#6b7280;font-size:12px;line-height:1.6;overflow-wrap:anywhere;word-break:break-word">{{.Footer}}</td></tr>
</table>
</body></html>`))

// Render creates RenoUI-styled HTML and a matching plain-text alternative.
func (c Config) Render(scene string, data TemplateData) (Message, error) {
	index := slices.Index(Scenes, scene)
	if index < 0 {
		return Message{}, errors.New("invalid email template")
	}
	if len(data.Username) > 128 || len(data.Code) > 16 || len(data.Detail) > 4000 {
		return Message{}, errors.New("email template data is too large")
	}
	if strings.ContainsAny(data.Code, "\r\n\x00") {
		return Message{}, errors.New("invalid verification code")
	}
	if data.URL != "" {
		base, err := url.Parse(c.PublicURL)
		target, targetErr := url.Parse(data.URL)
		if err != nil || targetErr != nil || target.Scheme != "https" || target.User != nil || target.Host != base.Host {
			return Message{}, errors.New("invalid email action URL")
		}
	}
	code := locale.Resolve(data.Locale)
	catalog := templateCatalogs[code]
	copy := catalog.Messages[index]
	openLabel, footer := catalog.OpenLabel, catalog.Footer
	view := struct {
		TemplateData
		Title, Body, SiteName, Style, Locale, OpenLabel, Footer string
	}{data, copy.Title, copy.Body, c.SiteName, c.TemplateStyle, code, openLabel, footer}
	var html bytes.Buffer
	if err := emailTemplate.Execute(&html, view); err != nil {
		return Message{}, err
	}
	parts := []string{copy.Title, copy.Body}
	if data.Username != "" {
		parts = append(parts, data.Username)
	}
	if data.Code != "" {
		parts = append(parts, data.Code)
	}
	if data.Detail != "" {
		parts = append(parts, data.Detail)
	}
	if data.URL != "" {
		parts = append(parts, data.URL)
	}
	parts = append(parts, footer)
	return Message{Subject: copy.Title, HTML: html.String(), Text: strings.Join(parts, "\n\n")}, nil
}
