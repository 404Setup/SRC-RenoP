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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	netmail "net/mail"
	"net/textproto"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSMTPPlainTLSSTARTTLSAndFailureCharging(t *testing.T) {
	certificate := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer certificate.Close()
	for _, scenario := range []struct {
		mode   string
		reject int
		auth   string
	}{{"plain", 0, "PLAIN"}, {"plain", 0, "LOGIN"}, {"tls", 0, "PLAIN"}, {"starttls", 0, "LOGIN"}, {"plain", 535, "PLAIN"}, {"plain", 550, "PLAIN"}, {"plain", 451, "PLAIN"}} {
		t.Run(fmt.Sprintf("%s-%s-%d", scenario.mode, scenario.auth, scenario.reject), func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			defer listener.Close()
			done := make(chan error, 1)
			go func() {
				done <- func() error {
					conn, err := listener.Accept()
					if err != nil {
						return err
					}
					defer conn.Close()
					_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
					secure := scenario.mode == "tls"
					if secure {
						conn = tls.Server(conn, certificate.TLS)
					}
					wire := textproto.NewConn(conn)
					if err = wire.PrintfLine("220 smtp.example ESMTP"); err != nil {
						return err
					}
					for {
						line, err := wire.ReadLine()
						if err != nil {
							return err
						}
						switch {
						case strings.HasPrefix(line, "EHLO "):
							if err = wire.PrintfLine("250-smtp.example"); err != nil {
								return err
							}
							if !secure && scenario.mode == "starttls" {
								if err = wire.PrintfLine("250-STARTTLS"); err != nil {
									return err
								}
							}
							if err = wire.PrintfLine("250 AUTH %s", scenario.auth); err != nil {
								return err
							}
						case line == "STARTTLS":
							if err = wire.PrintfLine("220 Ready for TLS"); err != nil {
								return err
							}
							conn = tls.Server(conn, certificate.TLS)
							wire = textproto.NewConn(conn)
							secure = true
						case strings.HasPrefix(line, "AUTH "):
							if scenario.reject == 535 {
								_ = wire.PrintfLine("535 Invalid credentials")
								return nil
							}
							if scenario.auth == "LOGIN" {
								_ = wire.PrintfLine("334 VXNlcm5hbWU6")
								if _, err = wire.ReadLine(); err != nil {
									return err
								}
								_ = wire.PrintfLine("334 UGFzc3dvcmQ6")
								if _, err = wire.ReadLine(); err != nil {
									return err
								}
							} else {
								encoded := strings.TrimPrefix(line, "AUTH PLAIN ")
								raw, err := base64.StdEncoding.DecodeString(encoded)
								if err != nil || string(raw) != "\x00smtp-user\x00smtp-password" {
									return fmt.Errorf("incorrect SMTP credential framing")
								}
							}
							if err = wire.PrintfLine("235 Authenticated"); err != nil {
								return err
							}
						case strings.HasPrefix(line, "MAIL FROM:"):
							if scenario.reject == 451 {
								_ = wire.PrintfLine("451 Temporary local error")
								return nil
							}
							if err = wire.PrintfLine("250 Sender accepted"); err != nil {
								return err
							}
						case strings.HasPrefix(line, "RCPT TO:"):
							if scenario.reject == 550 {
								_ = wire.PrintfLine("550 Recipient rejected")
								return nil
							}
							if err = wire.PrintfLine("250 Recipient accepted"); err != nil {
								return err
							}
						case line == "DATA":
							if err = wire.PrintfLine("354 Send message"); err != nil {
								return err
							}
							body, err := wire.ReadDotBytes()
							if err != nil {
								return err
							}
							if !strings.Contains(string(body), "MIME-Version: 1.0") || !strings.Contains(string(body), "private body") {
								return fmt.Errorf("SMTP message body missing")
							}
							return wire.PrintfLine("250 Queued")
						default:
							return fmt.Errorf("unexpected SMTP command %q", line)
						}
					}
				}()
			}()
			host, port, _ := net.SplitHostPort(listener.Addr().String())
			a := testAccount()
			a.Provider, a.SMTPHost, a.SMTPSecurity, a.Username, a.Password = "smtp", host, scenario.mode, "smtp-user", "smtp-password"
			a.SMTPPort, _ = strconv.Atoi(port)
			client := NewClient(certificate.Client().Transport)
			result, err := client.Send(context.Background(), a, Message{ID: "smtp-test", To: "receiver@example.com", Subject: "SMTP check", Text: "private body"})
			if scenario.reject == 0 {
				require.NoError(t, err)
				require.Equal(t, "accepted", result.Status)
				require.Equal(t, "250", result.Code)
			} else {
				require.Error(t, err)
				require.Equal(t, scenario.reject != 535, Chargeable(err))
			}
			require.NoError(t, <-done)
		})
	}
}

func TestMailProviderStatusCorrelationAndFailures(t *testing.T) {
	for _, scenario := range []struct{ provider, body, status string }{
		{"ses", `{"Insights":[{"Destination":"other@example.com","Events":[{"Type":"DELIVERY"}]},{"Destination":"receiver@example.com","Events":[{"Type":"BOUNCE"}]}]}`, "failed"},
		{"sendgrid", `{"messages":[{"msg_id":"provider-id.remote","to_email":"other@example.com","status":"delivered"},{"msg_id":"provider-id.remote","to_email":"receiver@example.com","status":"not_delivered"}]}`, "failed"},
		{"tencent", `{"Response":{"EmailStatusList":[{"MessageId":"provider-id","ToEmailAddress":"receiver@example.com","SendStatus":0,"DeliverStatus":1}]}}`, "delivered"},
		{"graph", `{"value":[{"id":"sent-message","isDraft":false}]}`, "sent"},
		{"gmail", `{"id":"provider-id","labelIds":["SENT"]}`, "sent"},
		{"feishu", `{"code":0,"data":{"message":{"message_state":2}}}`, "sent"},
		{"aliyun", `{"data":{"mailDetail":[{"accountName":"receiver@example.com","status":"1"}]}}`, "unknown"},
	} {
		t.Run(scenario.provider, func(t *testing.T) {
			called := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called++
				if scenario.provider == "ses" {
					require.Equal(t, "/v2/email/insights/provider-id/", r.URL.Path)
				}
				if scenario.provider == "graph" {
					require.Contains(t, r.URL.Query().Get("$filter"), "local-message")
				}
				if scenario.provider == "tencent" {
					require.Equal(t, "GetSendEmailStatus", r.Header.Get("X-TC-Action"))
				}
				if called == 1 {
					_, _ = io.WriteString(w, scenario.body)
				} else {
					w.WriteHeader(403)
					_, _ = io.WriteString(w, `{"error":{"code":"PermissionDenied"}}`)
				}
			}))
			defer server.Close()
			a := testAccount()
			a.Provider, a.Endpoint, a.AccessToken = scenario.provider, server.URL, "access"
			client := NewClient(server.Client().Transport)
			defer client.Close()
			message := Message{ID: "local-message", To: "receiver@example.com", CreatedAt: time.Now().UnixMilli()}
			previous := Result{MessageID: "provider-id", Status: "accepted", Check: true}
			result, err := client.Check(context.Background(), a, message, previous)
			require.NoError(t, err)
			require.Equal(t, scenario.status, result.Status)
			require.True(t, result.Terminal())
			result, err = client.Check(context.Background(), a, message, previous)
			require.Error(t, err)
			require.Equal(t, previous, result)
		})
	}
}

func TestMailBudgetReconfigurationKeepsUsageAndResetsRemoteState(t *testing.T) {
	now := time.Now()
	a := testAccount()
	rate := DefaultConfig().AccountRate
	state := AccountState{}
	_, reason := state.Reserve(a, rate, now)
	require.Empty(t, reason)
	remaining, balance := int64(0), int64(0)
	state.Calibrate(a, Budget{Remaining: &remaining, BalanceMicros: &balance, Currency: "USD"}, now)
	a.Provider = "sendgrid"
	a.Endpoint = "https://api.sendgrid.com/v3"
	state.Normalize(a, rate, now)
	require.Nil(t, state.Budget.Remaining)
	require.Nil(t, state.BalanceMicros)
	require.Zero(t, state.CalibrationAt)
	require.EqualValues(t, 1, state.Usage["day"].Used)
	require.EqualValues(t, 1, state.Rate.Used)
}

func TestMailLateRefundDoesNotCreditCalibratedProviderBalanceTwice(t *testing.T) {
	now := time.Now()
	a := testAccount()
	a.Quota.Limit = 0
	a.FetchBalance = true
	state := AccountState{}
	state.Normalize(a, DefaultConfig().AccountRate, now)
	remaining, balance := int64(10), int64(1000000)
	state.Calibrate(a, Budget{Remaining: &remaining, BalanceMicros: &balance, Currency: "USD"}, now)
	reservation, reason := state.Reserve(a, DefaultConfig().AccountRate, now)
	require.Empty(t, reason)
	require.EqualValues(t, 9, *state.Budget.Remaining)
	state.Calibrate(a, Budget{Remaining: &remaining, BalanceMicros: &balance, Currency: "USD"}, now.Add(time.Second))
	state.Refund(reservation)
	require.EqualValues(t, 10, *state.Budget.Remaining)
	require.EqualValues(t, 1000000, *state.BalanceMicros)
	require.Zero(t, state.Charged)
	require.EqualValues(t, 1, state.Rate.Used)
}

type mailRoundTrip func(*http.Request) (*http.Response, error)

func (f mailRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestMailOAuthRefreshRotationSurvivesEncryptedReload(t *testing.T) {
	a := testAccount()
	a.Provider, a.Endpoint, a.Tenant = "graph", "https://graph.microsoft.com/v1.0", "organizations"
	a.ClientID, a.ClientSecret, a.RefreshToken = "client", "client-secret", "initial-refresh"
	requests := 0
	client := NewClient(mailRoundTrip(func(r *http.Request) (*http.Response, error) {
		requests++
		require.Equal(t, "login.microsoftonline.com", r.URL.Host)
		require.NoError(t, r.ParseForm())
		require.Equal(t, "refresh_token", r.Form.Get("grant_type"))
		want := "initial-refresh"
		if requests > 1 {
			want = "rotated-refresh"
		}
		require.Equal(t, want, r.Form.Get("refresh_token"))
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"access_token":"access","refresh_token":"rotated-refresh","expires_in":3600}`))}, nil
	}))
	first, err := client.PrepareToken(context.Background(), a, OAuthToken{})
	require.NoError(t, err)
	cfg := DefaultConfig()
	require.NoError(t, cfg.EnsureKey())
	sealed, err := Seal(cfg.EncryptionKey, "account:primary", first)
	require.NoError(t, err)
	var restored OAuthToken
	require.NoError(t, Open(cfg.EncryptionKey, "account:primary", sealed, &restored))
	_, err = client.PrepareToken(context.Background(), a, restored)
	require.NoError(t, err)
	require.Equal(t, 1, requests)
	restored.ExpiresAt = time.Now().Add(-time.Minute).UnixMilli()
	_, err = client.PrepareToken(context.Background(), a, restored)
	require.NoError(t, err)
	require.Equal(t, 2, requests)
}

func TestSESSenderConfigurationFailureDoesNotConsumeQuota(t *testing.T) {
	client := NewClient(mailRoundTrip(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 400, Header: http.Header{"X-Amzn-Errortype": {"MessageRejected"}}, Body: io.NopCloser(strings.NewReader(`{"message":"Email address is not verified. Sender identity failed verification."}`))}, nil
	}))
	a := testAccount()
	a.Provider = "ses"
	a.Region = "us-east-1"
	a.Endpoint = "https://email.us-east-1.amazonaws.com"
	_, err := client.Send(context.Background(), a, Message{ID: "ses-test", To: "receiver@example.com", Subject: "Test", Text: "Test"})
	require.Error(t, err)
	require.Equal(t, "MessageRejected", err.Error())
	require.False(t, Chargeable(err))
}

func testAccount() Account {
	cfg := DefaultConfig()
	cfg.Accounts = []Account{{ID: "primary", Name: "Primary", Enabled: true, Provider: "cloudflare", From: "sender@example.com", Endpoint: "https://api.cloudflare.com/client/v4", APIKey: "secret-api-key", AccountID: "account", Pricing: Pricing{Currency: "USD", Rounding: "proportional", Tiers: []PriceTier{{AmountMicros: 100000, BatchSize: 1000}}}, Quota: Quota{Limit: 1, Period: "day"}, Overage: Quota{Limit: 2, Period: "day"}}}
	cfg.Normalize()
	return cfg.Accounts[0]
}

func TestMailConfigurationAndRouting(t *testing.T) {
	cfg := DefaultConfig()
	require.Equal(t, 2*time.Minute, cfg.ManualRate.Interval.Duration())
	require.EqualValues(t, 50, cfg.AccountRate.Limit)
	require.Equal(t, 5*time.Minute, cfg.Calibration.Duration())
	cfg.Accounts = []Account{testAccount()}
	cfg.Enabled = true
	cfg.PublicURL = "https://renop.example"
	require.NoError(t, cfg.Validate())
	require.Equal(t, "primary", cfg.SelectAccount("test").ID)
	cfg.Accounts[0].Scenes = []string{"test"}
	second := testAccount()
	second.ID = "secondary"
	second.Scenes = []string{"*"}
	cfg.Accounts = append(cfg.Accounts, second)
	require.Equal(t, "primary", cfg.SelectAccount("test").ID)
	require.Equal(t, "secondary", cfg.SelectAccount("password_reset").ID)
	cfg.Accounts[1].Scenes = []string{"test"}
	require.Error(t, cfg.Validate())
	cfg.Accounts = cfg.Accounts[:1]
	for _, zero := range []int64{0, -1} {
		changed := cfg.Clone()
		changed.ManualRate.Limit = zero
		require.Error(t, changed.Validate())
		changed = cfg.Clone()
		changed.AccountRate.Interval.Value = zero
		require.Error(t, changed.Validate())
	}
	for _, disabled := range []int64{0, -1} {
		changed := cfg.Clone()
		changed.Calibration.Value = disabled
		require.NoError(t, changed.Validate())
		require.Zero(t, changed.Calibration.Duration())
	}
	for _, invalid := range []string{"a@example.com\r\nBcc: b@example.com", "Name <a@example.com>", "a@example.com,b@example.com"} {
		_, err := Address(invalid)
		require.Error(t, err)
	}
	cfg.ListMode = "whitelist"
	cfg.Addresses = []string{"@example.com"}
	require.True(t, cfg.Allows("a@EXAMPLE.com"))
	require.False(t, cfg.Allows("a@evil-example.com"))
	cfg.ListMode = "blacklist"
	require.False(t, cfg.Allows("a@example.com"))
}

func TestMailQuotaPricingAndRefund(t *testing.T) {
	now := time.Date(2026, 9, 7, 23, 59, 0, 0, time.UTC)
	a := testAccount()
	balance := int64(150)
	a.BalanceMicros = &balance
	a.ForceSend = true
	s := AccountState{}
	rate := DefaultConfig().AccountRate
	first, reason := s.Reserve(a, rate, now)
	require.Empty(t, reason)
	require.False(t, first.Paid)
	second, reason := s.Reserve(a, rate, now)
	require.Empty(t, reason)
	require.True(t, second.Paid)
	require.EqualValues(t, 100, second.CostMicros)
	require.EqualValues(t, 50, *s.BalanceMicros)
	_, reason = s.Reserve(a, rate, now)
	require.Equal(t, "mail_balance_exhausted", reason)
	s.Refund(second)
	require.EqualValues(t, 150, *s.BalanceMicros)
	require.EqualValues(t, 1, s.Usage["day"].Used)
	require.EqualValues(t, 2, s.Rate.Used)
	_, reason = s.Reserve(a, rate, now.Add(2*time.Minute))
	require.Empty(t, reason)
	require.EqualValues(t, 1, s.Usage["day"].Used)
	require.EqualValues(t, 2, s.Usage["week"].Used)
	a.Quota.Limit = -1
	a.BalanceMicros = nil
	s = AccountState{}
	zero := int64(0)
	s.Budget.HardRemaining = &zero
	_, reason = s.Reserve(a, rate, now)
	require.Equal(t, "mail_provider_quota_exhausted", reason)
	a.Quota.Limit = 0
	s = AccountState{}
	_, reason = s.Reserve(a, rate, now)
	require.Empty(t, reason)
	tiers := Pricing{Currency: "USD", Rounding: "proportional", Tiers: []PriceTier{{UpTo: 10, AmountMicros: 100, BatchSize: 10}, {AmountMicros: 50, BatchSize: 10}}}
	for count, want := range map[int64]int64{0: 0, 1: 10, 10: 100, 11: 105, 20: 150} {
		cost, err := tiers.Cost(count)
		require.NoError(t, err)
		require.Equal(t, want, cost)
	}
	tiers.Rounding = "batch"
	cost, err := tiers.Cost(11)
	require.NoError(t, err)
	require.EqualValues(t, 150, cost)
	_, err = tiers.Cost(1000000001)
	require.Error(t, err)
	start, end := Window("month", time.Date(2028, 2, 29, 9, 0, 0, 0, time.UTC))
	require.Equal(t, "2028-02-01", start.Format("2006-01-02"))
	require.Equal(t, "2028-03-01", end.Format("2006-01-02"))
}

func TestMailTemplatesMIMEAndEncryptedPayload(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PublicURL = "https://renop.example"
	require.NoError(t, cfg.EnsureKey())
	for _, style := range []string{"card", "compact", "notice"} {
		cfg.TemplateStyle = style
		for _, scene := range Scenes {
			m, err := cfg.Render(scene, TemplateData{Username: "<script>alert(1)</script>", Code: "123456", URL: "https://renop.example/account", Detail: "<img src=x onerror=alert(1)>"})
			require.NoError(t, err)
			require.Contains(t, m.HTML, "&lt;script&gt;")
			require.NotContains(t, m.HTML, "<img src=x")
			require.Contains(t, m.Text, "123456")
			m.ID = "message-1"
			m.To = "receiver@example.com"
			raw, err := m.MIME(testAccount())
			require.NoError(t, err)
			parsed, err := netmail.ReadMessage(bytes.NewReader(raw))
			require.NoError(t, err)
			_, params, err := mime.ParseMediaType(parsed.Header.Get("Content-Type"))
			require.NoError(t, err)
			reader := multipart.NewReader(parsed.Body, params["boundary"])
			for range 2 {
				part, err := reader.NextPart()
				require.NoError(t, err)
				data, err := io.ReadAll(part)
				require.NoError(t, err)
				require.Contains(t, string(data), "123456")
			}
			_, err = reader.NextPart()
			require.ErrorIs(t, err, io.EOF)
			sealed, err := Seal(cfg.EncryptionKey, "job:message-1", m)
			require.NoError(t, err)
			require.NotContains(t, sealed, "123456")
			var restored Message
			require.NoError(t, Open(cfg.EncryptionKey, "job:message-1", sealed, &restored))
			require.Equal(t, m, restored)
			require.Error(t, Open(cfg.EncryptionKey, "job:other", sealed, &restored))
		}
	}
	_, err := cfg.Render("password_reset", TemplateData{URL: "https://attacker.example/reset"})
	require.Error(t, err)
}

func TestMailProviderSubmissionAndStatusContracts(t *testing.T) {
	for _, provider := range []string{"cloudflare", "graph", "ses", "sendgrid", "gmail", "aliyun", "tencent", "feishu"} {
		t.Run(provider, func(t *testing.T) {
			a := testAccount()
			a.Provider = provider
			a.Region = "us-east-1"
			a.APISecret = "signing-secret"
			a.AccessToken = "access-token"
			message := Message{ID: "message-1", To: "receiver@example.com", Subject: "Test", Text: "plain text", HTML: "<p>Test</p>", CreatedAt: time.Now().UnixMilli()}
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "POST", r.Method)
				if provider != "aliyun" {
					require.NotEmpty(t, r.Header.Get("Authorization"))
				}
				switch provider {
				case "aliyun":
					require.NoError(t, r.ParseForm())
					require.Equal(t, "SingleSendMail", r.Form.Get("Action"))
					require.NotEmpty(t, r.Form.Get("Signature"))
					require.Equal(t, message.To, r.Form.Get("ToAddress"))
					_, _ = io.WriteString(w, `{"EnvId":"provider-id"}`)
				default:
					var body map[string]any
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					switch provider {
					case "cloudflare":
						require.Equal(t, message.Subject, body["subject"])
						_, _ = io.WriteString(w, `{"success":true,"result":{"message_id":"provider-id","delivered":["receiver@example.com"]}}`)
					case "graph":
						require.True(t, body["saveToSentItems"].(bool))
						require.NotNil(t, valueAt(body, "message", "singleValueExtendedProperties"))
						w.WriteHeader(202)
					case "ses":
						require.Contains(t, r.Header.Get("Authorization"), "/ses/aws4_request")
						require.NotNil(t, body["Destination"])
						_, _ = io.WriteString(w, `{"MessageId":"provider-id"}`)
					case "sendgrid":
						require.NotNil(t, body["personalizations"])
						w.Header().Set("X-Message-Id", "provider-id")
						w.WriteHeader(202)
					case "gmail", "feishu":
						raw, err := base64.RawURLEncoding.DecodeString(textAt(body, "raw"))
						require.NoError(t, err)
						require.Contains(t, string(raw), "MIME-Version: 1.0")
						if provider == "gmail" {
							_, _ = io.WriteString(w, `{"id":"provider-id"}`)
						} else {
							_, _ = io.WriteString(w, `{"code":0,"data":{"message_id":"provider-id"}}`)
						}
					case "tencent":
						require.Equal(t, "SendEmail", r.Header.Get("X-TC-Action"))
						require.Contains(t, r.Header.Get("Authorization"), "TC3-HMAC-SHA256")
						require.Equal(t, base64.StdEncoding.EncodeToString([]byte(message.HTML)), textAt(body, "Simple", "Html"))
						_, _ = io.WriteString(w, `{"Response":{"MessageId":"provider-id"}}`)
					}
				}
			}))
			defer server.Close()
			a.Endpoint = server.URL
			client := NewClient(server.Client().Transport)
			defer client.Close()
			result, err := client.Send(context.Background(), a, message)
			require.NoError(t, err)
			require.NotEmpty(t, result.MessageID)
			if provider == "cloudflare" {
				require.Equal(t, "delivered", result.Status)
				require.False(t, result.Check)
			} else {
				require.Equal(t, "accepted", result.Status)
				require.True(t, result.Check)
			}
		})
	}
}

func TestMailFailureChargingAndBudget(t *testing.T) {
	status := http.StatusUnauthorized
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if status == 200 {
			_, _ = io.WriteString(w, `{"remain":0,"total":100}`)
		} else {
			_, _ = io.WriteString(w, `{"error":{"code":"failure"}}`)
		}
	}))
	defer server.Close()
	client := NewClient(server.Client().Transport)
	a := testAccount()
	a.Provider = "sendgrid"
	a.Endpoint = server.URL
	_, _, err := client.jsonRequest(context.Background(), "POST", server.URL, map[string]string{}, nil)
	require.Error(t, err)
	require.False(t, Chargeable(err))
	status = 500
	_, _, err = client.jsonRequest(context.Background(), "POST", server.URL, map[string]string{}, nil)
	require.Error(t, err)
	require.True(t, Chargeable(err))
	status = 200
	budget, err := client.FetchBudget(context.Background(), a)
	require.NoError(t, err)
	require.NotNil(t, budget.Remaining)
	require.Zero(t, *budget.Remaining)
	require.Nil(t, budget.BalanceMicros)
	value, err := decimalMicros("-123.004567")
	require.NoError(t, err)
	require.EqualValues(t, -123004567, value)
	for _, invalid := range []string{"NaN", "1e9", "1.0000001", "9223372036854775807"} {
		_, err = decimalMicros(invalid)
		require.Error(t, err)
	}
	oversize := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, strings.Repeat("x", MaxBodyBytes+1))
	}))
	defer oversize.Close()
	client = NewClient(oversize.Client().Transport)
	_, _, err = client.jsonRequest(context.Background(), "GET", oversize.URL, nil, nil)
	require.Error(t, err)
	require.Equal(t, "mail_response_invalid", err.Error())
}
