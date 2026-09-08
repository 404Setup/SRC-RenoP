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
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/http"
	"net/http/httptrace"
	netmail "net/mail"
	"net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/emmansun/base64"

	"github.com/goccy/go-json"
)

// MaxBodyBytes bounds each rendered message and each provider response body.
const MaxBodyBytes = 128 << 10

// Message contains one recipient and already rendered, bounded content.
type Message struct {
	ID        string `json:"id"`
	To        string `json:"to"`
	Subject   string `json:"subject"`
	Text      string `json:"text"`
	HTML      string `json:"html"`
	CreatedAt int64  `json:"created_at"`
}

// Result preserves submission and delivery as distinct provider outcomes.
type Result struct {
	MessageID  string `json:"message_id"`
	Status     string `json:"status"`
	Code       string `json:"code"`
	Detail     string `json:"detail"`
	Check      bool   `json:"check"`
	NotCharged bool   `json:"-"`
}

// SendError identifies failures that never reached the provider's sending stage.
type SendError struct {
	Code       string
	Detail     string
	NotCharged bool
	Uncertain  bool
}

func (e *SendError) Error() string { return e.Code }

// Chargeable excludes invalid credentials/configuration and failed connections.
func Chargeable(err error) bool {
	var failure *SendError
	return !errors.As(err, &failure) || !failure.NotCharged
}

// Validate prevents header injection and limits durable payload size.
func (m Message) Validate() error {
	if !identifier.MatchString(m.ID) || len(m.Subject) == 0 || len(m.Subject) > 240 || strings.ContainsAny(m.Subject, "\r\n\x00") || len(m.Text)+len(m.HTML) > MaxBodyBytes {
		return errors.New("invalid email message")
	}
	_, err := Address(m.To)
	return err
}

// MIME builds a multipart alternative with encoded headers and CRLF body lines.
func (m Message) MIME(a Account) ([]byte, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	if _, err := Address(a.From); err != nil {
		return nil, err
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, part := range []struct{ kind, body string }{{"text/plain", m.Text}, {"text/html", m.HTML}} {
		header := textproto.MIMEHeader{"Content-Type": {part.kind + "; charset=utf-8"}, "Content-Transfer-Encoding": {"quoted-printable"}}
		out, err := writer.CreatePart(header)
		if err != nil {
			return nil, err
		}
		encoded := quotedprintable.NewWriter(out)
		if _, err = io.WriteString(encoded, part.body); err != nil {
			return nil, err
		}
		if err = encoded.Close(); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	_, domain, _ := strings.Cut(a.From, "@")
	date := time.UnixMilli(m.CreatedAt)
	if m.CreatedAt == 0 {
		date = time.Now()
	}
	var output bytes.Buffer
	fmt.Fprintf(&output, "From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMessage-ID: <%s@%s>\r\nMIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=%q\r\n\r\n",
		(&netmail.Address{Name: a.FromName, Address: a.From}).String(), (&netmail.Address{Address: m.To}).String(), mime.QEncoding.Encode("utf-8", m.Subject), date.Format(time.RFC1123Z), m.ID, domain, writer.Boundary())
	output.Write(body.Bytes())
	return output.Bytes(), nil
}

// Client shares one bounded HTTP connection pool across serial provider operations.
type Client struct{ HTTP *http.Client }

// NewClient accepts the application's proxy-configured transport.
func NewClient(transport http.RoundTripper) *Client {
	if transport == nil {
		transport = &http.Transport{Proxy: http.ProxyFromEnvironment, MaxIdleConns: 4, MaxIdleConnsPerHost: 1, MaxConnsPerHost: 1, IdleConnTimeout: time.Minute, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 20 * time.Second, MaxResponseHeaderBytes: 16 << 10, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}}
	}
	return &Client{HTTP: &http.Client{Transport: transport, Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

// Close releases idle connections when the queue worker exits or proxy settings change.
func (c *Client) Close() {
	if c != nil && c.HTTP != nil {
		c.HTTP.CloseIdleConnections()
	}
}

func (c *Client) request(ctx context.Context, method, endpoint string, body []byte, headers http.Header) (map[string]any, http.Header, error) {
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, nil, &SendError{Code: "mail_configuration_invalid", NotCharged: true}
	}
	req.Header = headers.Clone()
	if req.Header == nil {
		req.Header = make(http.Header)
	}
	if len(body) > 0 && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	var submitted atomic.Bool
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), &httptrace.ClientTrace{WroteHeaders: func() { submitted.Store(true) }}))
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, nil, &SendError{Code: "mail_connection_failed", NotCharged: !submitted.Load(), Uncertain: submitted.Load()}
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, MaxBodyBytes+1))
	if err != nil || len(raw) > MaxBodyBytes {
		return nil, resp.Header, &SendError{Code: "mail_response_invalid", Uncertain: true}
	}
	data := map[string]any{}
	if len(raw) > 0 {
		if err = json.Unmarshal(raw, &data); err != nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil, resp.Header, &SendError{Code: "mail_response_invalid", Uncertain: true}
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		code := textAt(data, "error", "code")
		if code == "" {
			code = textAt(data, "code")
		}
		if code == "" {
			code = textAt(data, "Code")
		}
		if code == "" {
			if entries := arrayAt(data, "errors"); len(entries) > 0 {
				if entry, ok := entries[0].(map[string]any); ok {
					code = textAt(entry, "code")
				}
			}
		}
		if code == "" {
			code = strings.Split(resp.Header.Get("X-Amzn-Errortype"), ":")[0]
		}
		if code == "" {
			code = textAt(data, "__type")
		}
		if code == "" {
			code = "HTTP_" + strconv.Itoa(resp.StatusCode)
		}
		invalidEndpoint := resp.StatusCode >= 300 && resp.StatusCode < 400 || resp.StatusCode == 404 || resp.StatusCode == 405
		diagnostic := textAt(data, "message") + " " + textAt(data, "Message") + " " + textAt(data, "error", "message")
		invalidConfiguration := configurationError(code) || strings.Contains(strings.ToLower(diagnostic), "email address is not verified")
		return data, resp.Header, &SendError{Code: cleanCode(code), Detail: "HTTP " + strconv.Itoa(resp.StatusCode), NotCharged: invalidEndpoint || resp.StatusCode == 401 || resp.StatusCode == 403 || invalidConfiguration}
	}
	return data, resp.Header, nil
}

func (c *Client) jsonRequest(ctx context.Context, method, endpoint string, data any, headers http.Header) (map[string]any, http.Header, error) {
	var body []byte
	var err error
	if data != nil {
		body, err = json.Marshal(data)
		if err != nil {
			return nil, nil, err
		}
	}
	return c.request(ctx, method, endpoint, body, headers)
}

func textAt(data map[string]any, path ...string) string {
	value := valueAt(data, path...)
	switch v := value.(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	return ""
}
func valueAt(data map[string]any, path ...string) any {
	var value any = data
	for _, part := range path {
		object, ok := value.(map[string]any)
		if !ok {
			return nil
		}
		value = object[part]
	}
	return value
}
func numberAt(data map[string]any, path ...string) (float64, bool) {
	v, ok := valueAt(data, path...).(float64)
	return v, ok
}
func arrayAt(data map[string]any, path ...string) []any {
	values, _ := valueAt(data, path...).([]any)
	return values
}
func cleanCode(value string) string {
	var out strings.Builder
	for _, ch := range value {
		if out.Len() >= 128 {
			break
		}
		if ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || strings.ContainsRune("_.-", ch) {
			out.WriteRune(ch)
		}
	}
	if out.Len() == 0 {
		return "mail_provider_error"
	}
	return out.String()
}
func configurationError(code string) bool {
	s := strings.ToLower(code)
	for _, part := range []string{"invalidclient", "invalid_client", "invalidgrant", "invalid_grant", "invalidaccesskey", "signaturedoesnotmatch", "signaturefailure", "authfailure", "invalidcredential", "unauthorized", "permissiondenied", "sendernotverified", "mailfromdomainnotverified", "invalidfromaddress", "invalidfromalias", "invalidmailaddress.notfound", "invalidaccountname", "invalidregion"} {
		if strings.Contains(s, part) {
			return true
		}
	}
	return false
}

type limitedSMTPConn struct {
	net.Conn
	remaining int64
}

func (c *limitedSMTPConn) Read(p []byte) (int, error) {
	if c.remaining <= 0 {
		return 0, errors.New("SMTP response limit exceeded")
	}
	if int64(len(p)) > c.remaining {
		p = p[:c.remaining]
	}
	n, err := c.Conn.Read(p)
	c.remaining -= int64(n)
	return n, err
}

type oauthSMTPAuth struct{ username, token string }

func (a oauthSMTPAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	if !server.TLS {
		return "", nil, errors.New("SMTP OAuth requires TLS")
	}
	return "XOAUTH2", []byte("user=" + a.username + "\x01auth=Bearer " + a.token + "\x01\x01"), nil
}
func (oauthSMTPAuth) Next(_ []byte, more bool) ([]byte, error) {
	if more {
		return nil, errors.New("SMTP OAuth rejected")
	}
	return nil, nil
}

type loginSMTPAuth struct {
	username, password string
	step               int
}

type explicitPlainSMTPAuth struct{ smtp.Auth }

func (a explicitPlainSMTPAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	// Plain authentication is permitted only when the administrator explicitly selects plain SMTP.
	copy := *server
	copy.TLS = true
	return a.Auth.Start(&copy)
}

func (a *loginSMTPAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	if !server.TLS && server.Name != "localhost" {
		return "", nil, errors.New("SMTP authentication requires TLS")
	}
	return "LOGIN", nil, nil
}
func (a *loginSMTPAuth) Next(_ []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	a.step++
	switch a.step {
	case 1:
		return []byte(a.username), nil
	case 2:
		return []byte(a.password), nil
	}
	return nil, errors.New("unexpected SMTP authentication challenge")
}

func (c *Client) sendSMTP(ctx context.Context, a Account, m Message) (Result, error) {
	body, err := m.MIME(a)
	if err != nil {
		return Result{}, &SendError{Code: "mail_message_invalid", NotCharged: true}
	}
	dialer := net.Dialer{Timeout: 10 * time.Second}
	address := net.JoinHostPort(a.SMTPHost, strconv.Itoa(a.SMTPPort))
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return Result{}, &SendError{Code: "mail_connection_failed", NotCharged: true}
	}
	defer conn.Close()
	deadline := time.Now().Add(30 * time.Second)
	if value, ok := ctx.Deadline(); ok && value.Before(deadline) {
		deadline = value
	}
	if err = conn.SetDeadline(deadline); err != nil {
		return Result{}, err
	}
	rawConn := conn
	stop := context.AfterFunc(ctx, func() { _ = rawConn.Close() })
	defer stop()
	tlsConfig := &tls.Config{ServerName: a.SMTPHost, MinVersion: tls.VersionTLS12}
	if transport, ok := c.HTTP.Transport.(*http.Transport); ok && transport.TLSClientConfig != nil {
		tlsConfig.RootCAs = transport.TLSClientConfig.RootCAs
	}
	conn = &limitedSMTPConn{Conn: conn, remaining: 256 << 10}
	if a.SMTPSecurity == "tls" {
		secure := tls.Client(conn, tlsConfig)
		if err = secure.HandshakeContext(ctx); err != nil {
			return Result{}, &SendError{Code: "mail_tls_failed", NotCharged: true}
		}
		conn = secure
	}
	client, err := smtp.NewClient(conn, a.SMTPHost)
	if err != nil {
		return Result{}, smtpError(err, true)
	}
	defer client.Close()
	if a.SMTPSecurity == "starttls" {
		if err = client.StartTLS(tlsConfig); err != nil {
			return Result{}, smtpError(err, true)
		}
	}
	if a.Username != "" {
		var auth smtp.Auth
		if a.AccessToken != "" {
			auth = oauthSMTPAuth{a.Username, a.AccessToken}
		} else {
			_, methods := client.Extension("AUTH")
			if strings.Contains(methods, "PLAIN") {
				auth = smtp.PlainAuth("", a.Username, a.Password, a.SMTPHost)
			} else {
				auth = &loginSMTPAuth{username: a.Username, password: a.Password}
			}
		}
		if a.SMTPSecurity == "plain" {
			auth = explicitPlainSMTPAuth{auth}
		}
		if err = client.Auth(auth); err != nil {
			return Result{}, smtpError(err, true)
		}
	}
	if err = client.Mail(a.From); err != nil {
		reply, ok := errors.AsType[*textproto.Error](err)
		invalidSender := ok && (reply.Code == 530 || reply.Code == 535 || reply.Code == 550 || reply.Code == 553)
		return Result{}, smtpError(err, invalidSender)
	}
	if err = client.Rcpt(m.To); err != nil {
		return Result{}, smtpError(err, false)
	}
	id, err := client.Text.Cmd("DATA")
	if err != nil {
		return Result{}, smtpError(err, false)
	}
	client.Text.StartResponse(id)
	_, _, err = client.Text.ReadResponse(354)
	client.Text.EndResponse(id)
	if err != nil {
		return Result{}, smtpError(err, false)
	}
	out := client.Text.DotWriter()
	if _, err = out.Write(body); err != nil {
		return Result{}, smtpError(err, false)
	}
	if err = out.Close(); err != nil {
		return Result{}, smtpError(err, false)
	}
	code, _, err := client.Text.ReadResponse(250)
	if err != nil {
		return Result{}, smtpError(err, false)
	}
	return Result{MessageID: m.ID, Status: "accepted", Code: strconv.Itoa(code)}, nil
}

func smtpError(err error, notCharged bool) error {
	code := "mail_smtp_failed"
	reply, replyReceived := errors.AsType[*textproto.Error](err)
	if replyReceived {
		code = "SMTP_" + strconv.Itoa(reply.Code)
	}
	return &SendError{Code: code, NotCharged: notCharged, Uncertain: !notCharged && !replyReceived}
}

func rawBase64(m Message, a Account) (string, error) {
	data, err := m.MIME(a)
	return base64.URLEncoding.EncodeToString(data), err
}
