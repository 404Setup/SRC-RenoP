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
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Budget contains only capabilities actually returned by a provider.
type Budget struct {
	Remaining     *int64 `json:"remaining"`
	HardRemaining *int64 `json:"hard_remaining"`
	BalanceMicros *int64 `json:"balance_micros"`
	Currency      string `json:"currency"`
	Disabled      *bool  `json:"disabled"`
	ResetAt       int64  `json:"reset_at"`
}

// FetchBudget calibrates supported sending credits and optional account balances.
func (c *Client) FetchBudget(ctx context.Context, a Account) (Budget, error) {
	var budget Budget
	var quotaErr, balanceErr error
	switch a.Provider {
	case "ses":
		var data map[string]any
		data, quotaErr = c.awsRequest(ctx, a, "GET", "/v2/email/account", nil)
		if quotaErr == nil {
			maximum, hasMax := numberAt(data, "SendQuota", "Max24HourSend")
			sent, hasSent := numberAt(data, "SendQuota", "SentLast24Hours")
			if hasMax && hasSent {
				budget.Remaining = countPointer(maximum - sent)
				budget.HardRemaining = countPointer(maximum - sent)
			}
			if enabled, ok := data["SendingEnabled"].(bool); ok {
				disabled := !enabled
				budget.Disabled = &disabled
			}
		}
	case "sendgrid":
		data, _, err := c.jsonRequest(ctx, "GET", strings.TrimRight(a.Endpoint, "/")+"/user/credits", nil, http.Header{"Authorization": {"Bearer " + a.APIKey}})
		quotaErr = err
		if err == nil {
			if remain, ok := numberAt(data, "remain"); ok {
				budget.Remaining = countPointer(remain)
			}
			for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
				if reset, err := time.Parse(layout, textAt(data, "next_reset")); err == nil {
					budget.ResetAt = reset.UnixMilli()
					break
				}
			}
		}
	case "aliyun":
		data, err := c.aliRequest(ctx, a, "2015-11-23", "DescAccountSummary", nil)
		quotaErr = err
		if err == nil {
			remain, ok := numberAt(data, "RemainFreeQuota")
			daily, hasDaily := numberAt(data, "DailyRemainFreeQuota")
			if ok && hasDaily {
				remain = min(remain, daily)
			} else if hasDaily {
				remain = daily
				ok = true
			}
			if ok {
				budget.Remaining = countPointer(remain)
			}
			if status, ok := numberAt(data, "UserStatus"); ok {
				disabled := status != 0
				budget.Disabled = &disabled
			}
		}
	}
	if a.FetchBalance {
		billing := a
		billing.Endpoint = a.BillingEndpoint
		if HTTPSURL(billing.Endpoint) {
			switch a.Provider {
			case "aliyun":
				data, err := c.aliRequest(ctx, billing, "2017-12-14", "QueryAccountBalance", nil)
				balanceErr = err
				if err == nil {
					budget.Currency = textAt(data, "Data", "Currency")
					value, err := decimalMicros(strings.ReplaceAll(textAt(data, "Data", "AvailableAmount"), ",", ""))
					if err == nil {
						budget.BalanceMicros = &value
					} else {
						balanceErr = err
					}
				}
			case "tencent":
				data, err := c.tencentRequest(ctx, billing, "billing", "2018-07-09", "DescribeAccountBalance", map[string]any{})
				balanceErr = err
				if err == nil {
					amount, ok := numberAt(data, "RealBalance")
					if !ok {
						amount, ok = numberAt(data, "Balance")
					}
					if ok && math.Abs(amount) <= 1e11 {
						value := int64(math.Round(amount * 10000))
						budget.BalanceMicros = &value
					}
					budget.Currency = "CNY"
					if strings.Contains(a.BillingEndpoint, ".intl.") {
						budget.Currency = "USD"
					}
				}
			}
		}
	}
	return budget, errors.Join(quotaErr, balanceErr)
}

func countPointer(value float64) *int64 {
	if math.IsNaN(value) || math.IsInf(value, 0) || value > 1e9 {
		return nil
	}
	count := max(int64(math.Floor(value)), 0)
	return &count
}
func decimalMicros(value string) (int64, error) {
	negative := strings.HasPrefix(value, "-")
	if negative {
		value = value[1:]
	}
	whole, fraction, _ := strings.Cut(value, ".")
	if len(fraction) > 6 {
		return 0, errors.New("invalid provider balance")
	}
	for _, ch := range whole + fraction {
		if ch < '0' || ch > '9' {
			return 0, errors.New("invalid provider balance")
		}
	}
	number, err := strconv.ParseInt(whole, 10, 64)
	if err != nil || number > 1000000000 {
		return 0, errors.New("invalid provider balance")
	}
	frac, err := strconv.ParseInt(fraction+strings.Repeat("0", 6-len(fraction)), 10, 64)
	if err != nil {
		return 0, err
	}
	number = number*1000000 + frac
	if negative {
		number = -number
	}
	return number, nil
}

// Counter tracks one resettable accounting bucket.
type Counter struct {
	Start int64 `json:"start"`
	Used  int64 `json:"used"`
}

// AccountState is persisted with each claimed job before network transmission.
type AccountState struct {
	Configuration    string             `json:"configuration"`
	Currency         string             `json:"currency"`
	Usage            map[string]Counter `json:"usage"`
	Overage          map[string]Counter `json:"overage"`
	Rate             Counter            `json:"rate"`
	Budget           Budget             `json:"budget"`
	ManualBalance    *int64             `json:"manual_balance"`
	BalanceMicros    *int64             `json:"balance_micros"`
	Token            OAuthToken         `json:"token"`
	CalibrationAt    int64              `json:"calibration_at"`
	CalibrationError string             `json:"calibration_error"`
	Attempts         int64              `json:"attempts"`
	Charged          int64              `json:"charged"`
	SpentMicros      int64              `json:"spent_micros"`
}

func copyInt(value *int64) *int64 {
	if value == nil {
		return nil
	}
	v := *value
	return &v
}
func equalInt(a, b *int64) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }

// Normalize rolls UTC buckets forward and applies an explicitly edited balance.
func (s *AccountState) Normalize(a Account, rate Rate, now time.Time) {
	fingerprint := hashHex([]byte(strings.Join([]string{a.Provider, a.Endpoint, a.Region, a.AccountID, a.APIKey, a.APISecret, a.BillingEndpoint, a.Pricing.Currency}, "\x00")))
	if s.Configuration != "" && s.Configuration != fingerprint {
		s.Budget = Budget{}
		s.BalanceMicros = copyInt(a.BalanceMicros)
		s.CalibrationAt = 0
		s.CalibrationError = ""
	}
	s.Configuration = fingerprint
	if s.Currency != a.Pricing.Currency {
		s.Currency = a.Pricing.Currency
		s.SpentMicros = 0
	}
	if s.Usage == nil {
		s.Usage = map[string]Counter{}
	}
	if s.Overage == nil {
		s.Overage = map[string]Counter{}
	}
	for _, period := range []string{"hour", "day", "week", "month"} {
		start, _ := Window(period, now)
		if s.Usage[period].Start != start.UnixMilli() {
			s.Usage[period] = Counter{Start: start.UnixMilli()}
		}
		if s.Overage[period].Start != start.UnixMilli() {
			s.Overage[period] = Counter{Start: start.UnixMilli()}
		}
	}
	if duration := rate.Interval.Duration().Milliseconds(); duration > 0 {
		start := now.UnixMilli() / duration * duration
		if s.Rate.Start != start {
			s.Rate = Counter{Start: start}
		}
	}
	if !equalInt(s.ManualBalance, a.BalanceMicros) {
		s.ManualBalance = copyInt(a.BalanceMicros)
		s.BalanceMicros = copyInt(a.BalanceMicros)
		if a.BalanceMicros == nil && a.FetchBalance {
			s.BalanceMicros = copyInt(s.Budget.BalanceMicros)
		}
	}
	if a.BalanceMicros == nil && !a.FetchBalance {
		s.BalanceMicros = nil
	}
}

// Calibrate updates supported values; a failed lookup never erases a previously known limit.
func (s *AccountState) Calibrate(a Account, b Budget, now time.Time) {
	if b.Remaining != nil {
		s.Budget.Remaining = copyInt(b.Remaining)
	}
	if b.HardRemaining != nil {
		s.Budget.HardRemaining = copyInt(b.HardRemaining)
	}
	if b.Disabled != nil {
		s.Budget.Disabled = b.Disabled
	}
	if b.ResetAt > 0 {
		s.Budget.ResetAt = b.ResetAt
	}
	if b.BalanceMicros != nil && strings.EqualFold(b.Currency, a.Pricing.Currency) {
		s.Budget.BalanceMicros = copyInt(b.BalanceMicros)
		s.Budget.Currency = b.Currency
		s.BalanceMicros = copyInt(b.BalanceMicros)
	}
	s.CalibrationAt = now.UnixMilli()
}

// Reservation records the exact deduction so nonchargeable failures can refund it once.
type Reservation struct {
	Periods       map[string]int64 `json:"periods"`
	CostMicros    int64            `json:"cost_micros"`
	Paid          bool             `json:"paid"`
	Remote        bool             `json:"remote"`
	Hard          bool             `json:"hard"`
	CalibrationAt int64            `json:"calibration_at"`
	ManualBalance *int64           `json:"manual_balance"`
	Currency      string           `json:"currency"`
}

// Reserve validates local and provider limits and debits one attempted send before I/O.
func (s *AccountState) Reserve(a Account, rate Rate, now time.Time) (Reservation, string) {
	s.Normalize(a, rate, now)
	if s.Budget.Disabled != nil && *s.Budget.Disabled {
		return Reservation{}, "mail_account_disabled"
	}
	if s.BalanceMicros != nil && *s.BalanceMicros <= 0 {
		return Reservation{}, "mail_balance_exhausted"
	}
	if s.Rate.Used >= rate.Limit {
		return Reservation{}, "mail_account_rate_limited"
	}
	if s.Budget.HardRemaining != nil && *s.Budget.HardRemaining <= 0 {
		return Reservation{}, "mail_provider_quota_exhausted"
	}
	exhausted := a.Quota.Limit > 0 && s.Usage[a.Quota.Period].Used >= a.Quota.Limit || a.Quota.Limit == 0 && s.Budget.Remaining != nil && *s.Budget.Remaining <= 0
	r := Reservation{Paid: exhausted, Periods: map[string]int64{}, CalibrationAt: s.CalibrationAt, ManualBalance: copyInt(s.ManualBalance), Currency: s.Currency}
	if exhausted {
		if !a.ForceSend {
			return Reservation{}, "mail_quota_exhausted"
		}
		used := s.Overage[a.Overage.Period].Used
		if a.Overage.Limit >= 0 && used >= a.Overage.Limit {
			return Reservation{}, "mail_overage_exhausted"
		}
		before, err := a.Pricing.Cost(used)
		if err != nil {
			return Reservation{}, "mail_pricing_invalid"
		}
		after, err := a.Pricing.Cost(used + 1)
		if err != nil {
			return Reservation{}, "mail_pricing_invalid"
		}
		r.CostMicros = after - before
		if r.CostMicros < 0 || s.BalanceMicros != nil && *s.BalanceMicros < r.CostMicros {
			return Reservation{}, "mail_balance_exhausted"
		}
	}
	for period, counter := range s.Usage {
		r.Periods[period] = counter.Start
		counter.Used++
		s.Usage[period] = counter
		if r.Paid {
			paid := s.Overage[period]
			paid.Used++
			s.Overage[period] = paid
		}
	}
	s.Rate.Used++
	s.Attempts++
	s.Charged++
	s.SpentMicros += r.CostMicros
	if s.BalanceMicros != nil {
		*s.BalanceMicros -= r.CostMicros
	}
	if s.Budget.Remaining != nil && *s.Budget.Remaining > 0 {
		*s.Budget.Remaining--
		r.Remote = true
	}
	if s.Budget.HardRemaining != nil && *s.Budget.HardRemaining > 0 {
		*s.Budget.HardRemaining--
		r.Hard = true
	}
	return r, ""
}

// Refund restores sending credits and cost, while retaining the attempt rate-limit debit.
func (s *AccountState) Refund(r Reservation) {
	for period, start := range r.Periods {
		if count := s.Usage[period]; count.Start == start && count.Used > 0 {
			count.Used--
			s.Usage[period] = count
		}
		if r.Paid {
			if count := s.Overage[period]; count.Start == start && count.Used > 0 {
				count.Used--
				s.Overage[period] = count
			}
		}
	}
	if s.Charged > 0 {
		s.Charged--
	}
	if s.Currency == r.Currency {
		s.SpentMicros -= r.CostMicros
	}
	if s.BalanceMicros != nil && s.Currency == r.Currency && equalInt(s.ManualBalance, r.ManualBalance) && (s.ManualBalance != nil || s.CalibrationAt == r.CalibrationAt) {
		*s.BalanceMicros += r.CostMicros
	}
	if r.Remote && s.Budget.Remaining != nil && s.CalibrationAt == r.CalibrationAt {
		*s.Budget.Remaining++
	}
	if r.Hard && s.Budget.HardRemaining != nil && s.CalibrationAt == r.CalibrationAt {
		*s.Budget.HardRemaining++
	}
}
