/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package mail

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/emmansun/base64"

	"github.com/goccy/go-json"
)

// OAuthToken is durable private state; rotated refresh credentials must survive restart.
type OAuthToken struct {
	Fingerprint string `json:"fingerprint"`
	Access      string `json:"access"`
	Refresh     string `json:"refresh"`
	ExpiresAt   int64  `json:"expires_at"`
}

// PrepareToken refreshes delegated OAuth or obtains an Entra application token.
// The caller persists returned changes before attempting to send.
func (c *Client) PrepareToken(ctx context.Context, a Account, previous OAuthToken) (OAuthToken, error) {
	fingerprint := hashHex([]byte(a.Provider + "\x00" + a.Endpoint + "\x00" + a.Tenant + "\x00" + a.ClientID + "\x00" + a.ClientSecret + "\x00" + a.RefreshToken + "\x00" + a.AccessToken))
	if previous.Fingerprint != fingerprint {
		previous = OAuthToken{Fingerprint: fingerprint, Access: a.AccessToken, Refresh: a.RefreshToken}
	}
	if previous.Access != "" && (previous.ExpiresAt == 0 && previous.Refresh == "" || previous.ExpiresAt > time.Now().Add(time.Minute).UnixMilli()) {
		return previous, nil
	}
	if a.Provider != "graph" && a.Provider != "gmail" && a.Provider != "feishu" && a.Provider != "smtp" {
		return previous, nil
	}
	if a.Provider == "smtp" && a.ClientID == "" {
		return previous, nil
	}
	if a.ClientID == "" {
		return previous, &SendError{Code: "mail_oauth_configuration_invalid", NotCharged: true}
	}
	var data map[string]any
	var err error
	if a.Provider == "feishu" {
		if previous.Refresh == "" {
			return previous, &SendError{Code: "mail_oauth_configuration_invalid", NotCharged: true}
		}
		data, _, err = c.jsonRequest(ctx, "POST", strings.TrimRight(a.Endpoint, "/")+"/authen/v2/oauth/token", map[string]string{"grant_type": "refresh_token", "client_id": a.ClientID, "client_secret": a.ClientSecret, "refresh_token": previous.Refresh}, nil)
	} else {
		endpoint := "https://oauth2.googleapis.com/token"
		form := url.Values{"client_id": {a.ClientID}, "client_secret": {a.ClientSecret}}
		if a.Provider == "graph" || a.Provider == "smtp" && strings.Contains(a.SMTPHost, "outlook") || a.Provider == "smtp" && strings.Contains(a.SMTPHost, "office365") {
			authority := "https://login.microsoftonline.com"
			scope := "https://graph.microsoft.com/.default"
			if strings.Contains(a.Endpoint, "microsoft.us") {
				authority = "https://login.microsoftonline.us"
				u, _ := url.Parse(a.Endpoint)
				scope = "https://" + u.Host + "/.default"
			}
			if strings.Contains(a.Endpoint, "chinacloudapi.cn") {
				authority = "https://login.chinacloudapi.cn"
				scope = "https://microsoftgraph.chinacloudapi.cn/.default"
			}
			if a.Provider == "smtp" {
				scope = "https://outlook.office365.com/.default"
			}
			tenant := a.Tenant
			if tenant == "" {
				tenant = "common"
			}
			endpoint = authority + "/" + url.PathEscape(tenant) + "/oauth2/v2.0/token"
			form.Set("scope", scope)
		}
		if previous.Refresh != "" {
			form.Set("grant_type", "refresh_token")
			form.Set("refresh_token", previous.Refresh)
		} else if a.Provider == "graph" {
			form.Set("grant_type", "client_credentials")
		} else {
			return previous, &SendError{Code: "mail_oauth_configuration_invalid", NotCharged: true}
		}
		data, _, err = c.request(ctx, "POST", endpoint, []byte(form.Encode()), http.Header{"Content-Type": {"application/x-www-form-urlencoded"}})
	}
	if err != nil {
		return previous, &SendError{Code: "mail_oauth_refresh_failed", NotCharged: true}
	}
	if code, ok := numberAt(data, "code"); ok && code != 0 {
		return previous, &SendError{Code: "mail_oauth_refresh_failed", NotCharged: true}
	}
	access := textAt(data, "access_token")
	if access == "" || len(access) > 16384 || strings.ContainsAny(access, "\r\n\x00") {
		return previous, &SendError{Code: "mail_oauth_response_invalid", NotCharged: true}
	}
	seconds, ok := numberAt(data, "expires_in")
	if !ok || seconds < 60 || seconds > 86400 {
		return previous, &SendError{Code: "mail_oauth_response_invalid", NotCharged: true}
	}
	previous.Access = access
	previous.ExpiresAt = time.Now().Add(time.Duration(seconds) * time.Second).UnixMilli()
	if refresh := textAt(data, "refresh_token"); refresh != "" {
		if len(refresh) > 16384 || strings.ContainsAny(refresh, "\r\n\x00") {
			return previous, &SendError{Code: "mail_oauth_response_invalid", NotCharged: true}
		}
		previous.Refresh = refresh
	}
	return previous, nil
}

// Send submits one message. Queueing, quotas, and token persistence belong to the caller.
func (c *Client) Send(ctx context.Context, a Account, m Message) (Result, error) {
	if err := m.Validate(); err != nil {
		return Result{}, &SendError{Code: "mail_message_invalid", NotCharged: true}
	}
	if err := a.Validate(); err != nil {
		return Result{}, &SendError{Code: "mail_configuration_invalid", NotCharged: true}
	}
	if a.Provider == "smtp" {
		return c.sendSMTP(ctx, a, m)
	}
	endpoint := strings.TrimRight(a.Endpoint, "/")
	headers := http.Header{}
	var data map[string]any
	var err error
	var responseHeaders http.Header
	result := Result{Status: "accepted", Check: true}
	switch a.Provider {
	case "cloudflare":
		headers.Set("Authorization", "Bearer "+a.APIKey)
		data, _, err = c.jsonRequest(ctx, "POST", endpoint+"/accounts/"+url.PathEscape(a.AccountID)+"/email/sending/send", map[string]any{"from": map[string]string{"address": a.From, "name": a.FromName}, "to": []string{m.To}, "subject": m.Subject, "html": m.HTML, "text": m.Text}, headers)
		if err == nil {
			if success, _ := data["success"].(bool); !success {
				return Result{}, cloudflareError(data)
			}
			result.MessageID = textAt(data, "result", "message_id")
			result.Check = false
			if len(arrayAt(data, "result", "permanent_bounces"))+len(arrayAt(data, "result", "suppressed_recipients")) > 0 {
				return Result{}, &SendError{Code: "mail_recipient_rejected", Detail: "Cloudflare recipient rejected"}
			}
			if len(arrayAt(data, "result", "delivered")) > 0 {
				result.Status = "delivered"
			} else {
				result.Status = "queued_provider"
			}
		}
	case "graph":
		headers.Set("Authorization", "Bearer "+a.AccessToken)
		message := map[string]any{"subject": m.Subject, "body": map[string]string{"contentType": "HTML", "content": m.HTML}, "toRecipients": []any{map[string]any{"emailAddress": map[string]string{"address": m.To}}}, "singleValueExtendedProperties": []any{map[string]string{"id": graphTrackingProperty, "value": m.ID}}}
		data, _, err = c.jsonRequest(ctx, "POST", graphMailbox(a)+"/sendMail", map[string]any{"message": message, "saveToSentItems": true}, headers)
		result.MessageID = m.ID
	case "gmail":
		headers.Set("Authorization", "Bearer "+a.AccessToken)
		raw, encodeErr := rawBase64(m, a)
		if encodeErr != nil {
			return Result{}, encodeErr
		}
		data, _, err = c.jsonRequest(ctx, "POST", endpoint+"/users/me/messages/send", map[string]string{"raw": raw}, headers)
		result.MessageID = textAt(data, "id")
	case "sendgrid":
		headers.Set("Authorization", "Bearer "+a.APIKey)
		data, responseHeaders, err = c.jsonRequest(ctx, "POST", endpoint+"/mail/send", map[string]any{"from": map[string]string{"email": a.From, "name": a.FromName}, "personalizations": []any{map[string]any{"to": []any{map[string]string{"email": m.To}}}}, "subject": m.Subject, "content": []any{map[string]string{"type": "text/plain", "value": m.Text}, map[string]string{"type": "text/html", "value": m.HTML}}}, headers)
		result.MessageID = responseHeaders.Get("X-Message-Id")
	case "ses":
		body := map[string]any{"FromEmailAddress": a.From, "Destination": map[string]any{"ToAddresses": []string{m.To}}, "Content": map[string]any{"Simple": map[string]any{"Subject": map[string]string{"Data": m.Subject, "Charset": "UTF-8"}, "Body": map[string]any{"Html": map[string]string{"Data": m.HTML, "Charset": "UTF-8"}, "Text": map[string]string{"Data": m.Text, "Charset": "UTF-8"}}}}}
		data, err = c.awsRequest(ctx, a, "POST", "/v2/email/outbound-emails", body)
		result.MessageID = textAt(data, "MessageId")
	case "aliyun":
		data, err = c.aliRequest(ctx, a, "2015-11-23", "SingleSendMail", url.Values{"AccountName": {a.From}, "AddressType": {"1"}, "ReplyToAddress": {"false"}, "ToAddress": {m.To}, "Subject": {m.Subject}, "HtmlBody": {m.HTML}, "TextBody": {m.Text}, "FromAlias": {a.FromName}})
		result.MessageID = textAt(data, "EnvId")
	case "tencent":
		data, err = c.tencentRequest(ctx, a, "ses", "2020-10-02", "SendEmail", map[string]any{"FromEmailAddress": a.From, "Destination": []string{m.To}, "Subject": m.Subject, "TriggerType": 1, "Simple": map[string]string{"Html": base64.StdEncoding.EncodeToString([]byte(m.HTML)), "Text": base64.StdEncoding.EncodeToString([]byte(m.Text))}})
		result.MessageID = textAt(data, "MessageId")
	case "feishu":
		headers.Set("Authorization", "Bearer "+a.AccessToken)
		raw, encodeErr := rawBase64(m, a)
		if encodeErr != nil {
			return Result{}, encodeErr
		}
		data, _, err = c.jsonRequest(ctx, "POST", feishuMailbox(a)+"/messages/send", map[string]any{"raw": raw, "dedupe_key": m.ID}, headers)
		if err == nil {
			err = feishuError(data)
		}
		result.MessageID = textAt(data, "data", "message_id")
	default:
		return Result{}, &SendError{Code: "mail_provider_invalid", NotCharged: true}
	}
	if err != nil {
		return Result{}, err
	}
	if result.MessageID == "" || len(result.MessageID) > 512 {
		return Result{}, &SendError{Code: "mail_response_invalid", Uncertain: true}
	}
	return result, nil
}

func graphMailbox(a Account) string {
	base := strings.TrimRight(a.Endpoint, "/")
	if a.Mailbox != "" && a.Mailbox != "me" {
		return base + "/users/" + url.PathEscape(a.Mailbox)
	}
	return base + "/me"
}
func feishuMailbox(a Account) string {
	mailbox := a.Mailbox
	if mailbox == "" {
		mailbox = a.From
	}
	return strings.TrimRight(a.Endpoint, "/") + "/mail/v1/user_mailboxes/" + url.PathEscape(mailbox)
}
func cloudflareError(data map[string]any) error {
	code := "mail_provider_error"
	if values := arrayAt(data, "errors"); len(values) > 0 {
		if value, ok := values[0].(map[string]any); ok {
			code = "CF_" + textAt(value, "code")
		}
	}
	return &SendError{Code: cleanCode(code), NotCharged: configurationError(code)}
}
func feishuError(data map[string]any) error {
	code, ok := numberAt(data, "code")
	if !ok {
		return &SendError{Code: "mail_response_invalid"}
	}
	if code != 0 {
		return &SendError{Code: "FEISHU_" + strconv.FormatFloat(code, 'f', 0, 64), NotCharged: code >= 99991600 && code <= 99991799}
	}
	return nil
}

func hashHex(value []byte) string { hash := sha256.Sum256(value); return hex.EncodeToString(hash[:]) }
func hmacSHA256(key []byte, value string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(value))
	return mac.Sum(nil)
}

func (c *Client) awsRequest(ctx context.Context, a Account, method, path string, data any) (map[string]any, error) {
	var body []byte
	var err error
	if data != nil {
		body, err = json.Marshal(data)
		if err != nil {
			return nil, err
		}
	}
	endpoint := strings.TrimRight(a.Endpoint, "/") + path
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	date := now.Format("20060102")
	timestamp := now.Format("20060102T150405Z")
	headers := http.Header{"Content-Type": {"application/json"}, "X-Amz-Date": {timestamp}}
	canonicalHeaders := "host:" + u.Host + "\n" + "x-amz-date:" + timestamp + "\n"
	signedHeaders := "host;x-amz-date"
	if a.SessionToken != "" {
		headers.Set("X-Amz-Security-Token", a.SessionToken)
		canonicalHeaders += "x-amz-security-token:" + a.SessionToken + "\n"
		signedHeaders += ";x-amz-security-token"
	}
	canonical := method + "\n" + u.EscapedPath() + "\n" + u.Query().Encode() + "\n" + canonicalHeaders + "\n" + signedHeaders + "\n" + hashHex(body)
	scope := date + "/" + a.Region + "/ses/aws4_request"
	key := hmacSHA256([]byte("AWS4"+a.APISecret), date)
	key = hmacSHA256(key, a.Region)
	key = hmacSHA256(key, "ses")
	key = hmacSHA256(key, "aws4_request")
	signature := hex.EncodeToString(hmacSHA256(key, "AWS4-HMAC-SHA256\n"+timestamp+"\n"+scope+"\n"+hashHex([]byte(canonical))))
	headers.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+a.APIKey+"/"+scope+", SignedHeaders="+signedHeaders+", Signature="+signature)
	result, _, err := c.request(ctx, method, endpoint, body, headers)
	return result, err
}

func (c *Client) tencentRequest(ctx context.Context, a Account, service, version, action string, data any) (map[string]any, error) {
	body, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(a.Endpoint)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	date := now.Format("2006-01-02")
	timestamp := strconv.FormatInt(now.Unix(), 10)
	headers := http.Header{"Content-Type": {"application/json; charset=utf-8"}, "X-TC-Action": {action}, "X-TC-Version": {version}, "X-TC-Timestamp": {timestamp}, "X-TC-Region": {a.Region}}
	if a.SessionToken != "" {
		headers.Set("X-TC-Token", a.SessionToken)
	}
	canonical := "POST\n/\n\ncontent-type:application/json; charset=utf-8\nhost:" + u.Host + "\nx-tc-action:" + strings.ToLower(action) + "\n\ncontent-type;host;x-tc-action\n" + hashHex(body)
	scope := date + "/" + service + "/tc3_request"
	key := hmacSHA256([]byte("TC3"+a.APISecret), date)
	key = hmacSHA256(key, service)
	key = hmacSHA256(key, "tc3_request")
	signature := hex.EncodeToString(hmacSHA256(key, "TC3-HMAC-SHA256\n"+timestamp+"\n"+scope+"\n"+hashHex([]byte(canonical))))
	headers.Set("Authorization", "TC3-HMAC-SHA256 Credential="+a.APIKey+"/"+scope+", SignedHeaders=content-type;host;x-tc-action, Signature="+signature)
	result, _, err := c.request(ctx, "POST", strings.TrimRight(a.Endpoint, "/")+"/", body, headers)
	if err != nil {
		return nil, err
	}
	response, ok := result["Response"].(map[string]any)
	if !ok {
		return nil, &SendError{Code: "mail_response_invalid"}
	}
	if code := textAt(response, "Error", "Code"); code != "" {
		return response, &SendError{Code: cleanCode(code), NotCharged: configurationError(code)}
	}
	return response, nil
}

func percent(value string) string { return strings.ReplaceAll(url.QueryEscape(value), "+", "%20") }

func (c *Client) aliRequest(ctx context.Context, a Account, version, action string, values url.Values) (map[string]any, error) {
	if values == nil {
		values = url.Values{}
	}
	values.Set("Action", action)
	values.Set("Version", version)
	values.Set("Format", "JSON")
	values.Set("AccessKeyId", a.APIKey)
	values.Set("SignatureMethod", "HMAC-SHA1")
	values.Set("SignatureVersion", "1.0")
	values.Set("SignatureNonce", rand.Text())
	values.Set("Timestamp", time.Now().UTC().Format("2006-01-02T15:04:05Z"))
	if a.Region != "" {
		values.Set("RegionId", a.Region)
	}
	if a.SessionToken != "" {
		values.Set("SecurityToken", a.SessionToken)
	}
	canonical := strings.ReplaceAll(values.Encode(), "+", "%20")
	mac := hmac.New(sha1.New, []byte(a.APISecret+"&"))
	_, _ = mac.Write([]byte("POST&%2F&" + percent(canonical)))
	values.Set("Signature", base64.StdEncoding.EncodeToString(mac.Sum(nil)))
	result, _, err := c.request(ctx, "POST", strings.TrimRight(a.Endpoint, "/")+"/", []byte(values.Encode()), http.Header{"Content-Type": {"application/x-www-form-urlencoded"}})
	if err != nil {
		return result, err
	}
	if code := textAt(result, "Code"); code != "" && code != "200" && code != "Success" {
		return result, &SendError{Code: cleanCode(code), NotCharged: configurationError(code)}
	}
	if success, ok := result["Success"].(bool); ok && !success {
		return result, &SendError{Code: "mail_provider_error"}
	}
	return result, nil
}

var errUnsupported = errors.New("provider capability unavailable")
