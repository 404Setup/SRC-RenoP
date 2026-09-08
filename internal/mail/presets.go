/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package mail

// Preset supplies editable endpoints and a dated, source-linked marginal price.
type Preset struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Account      Account `json:"account"`
	PriceSource  string  `json:"price_source"`
	PriceChecked string  `json:"price_checked"`
	QuotaAPI     bool    `json:"quota_api"`
	BalanceAPI   bool    `json:"balance_api"`
	StatusAPI    bool    `json:"status_api"`
	PaidOverage  bool    `json:"paid_overage"`
}

// Presets lists supported provider endpoints; credentials and purchased allowances are never inferred.
func Presets() []Preset {
	price := func(currency string, micros, batch int64) Pricing {
		return Pricing{Currency: currency, Rounding: "proportional", Tiers: []PriceTier{{AmountMicros: micros, BatchSize: batch}}}
	}
	cf := price("USD", 350000, 1000)
	ses := price("USD", 100000, 1000)
	sg := price("USD", 1330, 1)
	ali := price("USD", 290000, 1000)
	tc := price("CNY", 1900, 1)
	included := price("USD", 0, 1)
	const cfSource = "https://developers.cloudflare.com/email-service/platform/pricing/"
	const sesSource = "https://aws.amazon.com/ses/pricing/"
	const sgSource = "https://sendgrid.com/content/dam/sendgrid/global/en/other/sendgrid-pricing/twi121--sendgrid-pricing-pdf-st1.pdf"
	const aliSource = "https://www.alibabacloud.com/help/en/direct-mail/billing-methods"
	const tcSource = "https://cloud.tencent.com/document/product/1288/47930"
	result := []Preset{
		{ID: "cloudflare", Name: "Cloudflare Email", Account: Account{Provider: "cloudflare", Endpoint: "https://api.cloudflare.com/client/v4", Pricing: cf}, PriceSource: cfSource, PaidOverage: true},
		{ID: "graph-global", Name: "Microsoft Graph · Global / Outlook.com", Account: Account{Provider: "graph", Endpoint: "https://graph.microsoft.com/v1.0", Tenant: "common", Pricing: included}, PriceSource: "https://learn.microsoft.com/en-us/graph/metered-api-list", StatusAPI: true},
		{ID: "graph-usgov", Name: "Microsoft Graph · US Government L4", Account: Account{Provider: "graph", Endpoint: "https://graph.microsoft.us/v1.0", Pricing: included}, PriceSource: "https://learn.microsoft.com/en-us/graph/deployments", StatusAPI: true},
		{ID: "graph-dod", Name: "Microsoft Graph · US Government L5", Account: Account{Provider: "graph", Endpoint: "https://dod-graph.microsoft.us/v1.0", Pricing: included}, PriceSource: "https://learn.microsoft.com/en-us/graph/deployments", StatusAPI: true},
		{ID: "graph-china", Name: "Microsoft Graph · China (21Vianet)", Account: Account{Provider: "graph", Endpoint: "https://microsoftgraph.chinacloudapi.cn/v1.0", Pricing: included}, PriceSource: "https://learn.microsoft.com/en-us/graph/deployments", StatusAPI: true},
		{ID: "sendgrid-global", Name: "Twilio SendGrid · Global · Essentials 50K", Account: Account{Provider: "sendgrid", Endpoint: "https://api.sendgrid.com/v3", Pricing: sg}, PriceSource: sgSource, QuotaAPI: true, StatusAPI: true, PaidOverage: true},
		{ID: "sendgrid-eu", Name: "Twilio SendGrid · EU", Account: Account{Provider: "sendgrid", Endpoint: "https://api.eu.sendgrid.com/v3", Pricing: price("USD", 1100, 1)}, PriceSource: sgSource, QuotaAPI: true, StatusAPI: true, PaidOverage: true},
		{ID: "gmail", Name: "Google Gmail / Workspace", Account: Account{Provider: "gmail", Endpoint: "https://gmail.googleapis.com/gmail/v1", Pricing: included}, PriceSource: "https://developers.google.com/workspace/gmail/api/reference/quota", StatusAPI: true},
		{ID: "aliyun-hangzhou", Name: "Alibaba Cloud Direct Mail · Hangzhou", Account: Account{Provider: "aliyun", Endpoint: "https://dm.aliyuncs.com", Region: "cn-hangzhou", BillingEndpoint: "https://business.aliyuncs.com", Pricing: ali}, PriceSource: aliSource, QuotaAPI: true, BalanceAPI: true, StatusAPI: true, PaidOverage: true},
		{ID: "aliyun-singapore", Name: "Alibaba Cloud Direct Mail · Singapore", Account: Account{Provider: "aliyun", Endpoint: "https://dm.ap-southeast-1.aliyuncs.com", Region: "ap-southeast-1", BillingEndpoint: "https://business.ap-southeast-1.aliyuncs.com", Pricing: ali}, PriceSource: aliSource, QuotaAPI: true, BalanceAPI: true, StatusAPI: true, PaidOverage: true},
		{ID: "aliyun-virginia", Name: "Alibaba Cloud Direct Mail · Virginia", Account: Account{Provider: "aliyun", Endpoint: "https://dm.us-east-1.aliyuncs.com", Region: "us-east-1", BillingEndpoint: "https://business.ap-southeast-1.aliyuncs.com", Pricing: ali}, PriceSource: aliSource, QuotaAPI: true, BalanceAPI: true, StatusAPI: true, PaidOverage: true},
		{ID: "aliyun-frankfurt", Name: "Alibaba Cloud Direct Mail · Frankfurt", Account: Account{Provider: "aliyun", Endpoint: "https://dm.eu-central-1.aliyuncs.com", Region: "eu-central-1", BillingEndpoint: "https://business.ap-southeast-1.aliyuncs.com", Pricing: ali}, PriceSource: aliSource, QuotaAPI: true, BalanceAPI: true, StatusAPI: true, PaidOverage: true},
		{ID: "tencent-china", Name: "Tencent Cloud SES · China", Account: Account{Provider: "tencent", Endpoint: "https://ses.tencentcloudapi.com", Region: "ap-guangzhou", BillingEndpoint: "https://billing.tencentcloudapi.com", Pricing: tc}, PriceSource: tcSource, BalanceAPI: true, StatusAPI: true, PaidOverage: true},
		{ID: "tencent-international", Name: "Tencent Cloud SES · International", Account: Account{Provider: "tencent", Endpoint: "https://ses.intl.tencentcloudapi.com", Region: "ap-singapore", BillingEndpoint: "https://billing.intl.tencentcloudapi.com", Pricing: price("USD", 280, 1)}, PriceSource: "https://www-sg.tencentcloud.com/document/product/1084/39335", BalanceAPI: true, StatusAPI: true, PaidOverage: true},
		{ID: "feishu", Name: "Feishu Mail", Account: Account{Provider: "feishu", Endpoint: "https://open.feishu.cn/open-apis", Pricing: included}, PriceSource: "https://www.feishu.cn/service", StatusAPI: true},
		{ID: "lark", Name: "Lark Mail", Account: Account{Provider: "feishu", Endpoint: "https://open.larksuite.com/open-apis", Pricing: included}, PriceSource: "https://www.larksuite.com/en_us/plans", StatusAPI: true},
	}
	for _, preset := range result {
		if preset.Account.Provider == "aliyun" {
			preset.ID += "-vpc"
			preset.Name += " · VPC"
			preset.Account.Endpoint = "https://dm-vpc." + preset.Account.Region + ".aliyuncs.com"
			result = append(result, preset)
		}
	}
	for _, region := range []string{"us-east-1", "us-east-2", "us-west-1", "us-west-2", "ca-central-1", "ca-west-1", "eu-west-1", "eu-west-2", "eu-west-3", "eu-central-1", "eu-central-2", "eu-north-1", "eu-south-1", "ap-south-1", "ap-south-2", "ap-northeast-1", "ap-northeast-2", "ap-northeast-3", "ap-southeast-1", "ap-southeast-2", "ap-southeast-3", "ap-southeast-5", "sa-east-1", "af-south-1", "me-south-1", "me-central-1", "il-central-1", "us-gov-east-1", "us-gov-west-1"} {
		result = append(result, Preset{ID: "ses-" + region, Name: "Amazon SES · " + region, Account: Account{Provider: "ses", Endpoint: "https://email." + region + ".amazonaws.com", Region: region, Pricing: ses}, PriceSource: sesSource, QuotaAPI: true, StatusAPI: true, PaidOverage: true})
		result = append(result, Preset{ID: "ses-dual-" + region, Name: "Amazon SES · " + region + " · IPv4/IPv6", Account: Account{Provider: "ses", Endpoint: "https://email." + region + ".api.aws", Region: region, Pricing: ses}, PriceSource: sesSource, QuotaAPI: true, StatusAPI: true, PaidOverage: true})
		if len(region) >= 3 && (region[:3] == "us-" || region[:3] == "ca-") {
			for _, suffix := range []string{"amazonaws.com", "api.aws"} {
				result = append(result, Preset{ID: "ses-fips-" + region + "-" + suffix, Name: "Amazon SES · " + region + " · FIPS · " + suffix, Account: Account{Provider: "ses", Endpoint: "https://email-fips." + region + "." + suffix, Region: region, Pricing: ses}, PriceSource: sesSource, QuotaAPI: true, StatusAPI: true, PaidOverage: true})
			}
		}
	}
	for _, entry := range []struct {
		id, name, host, security, username, source string
		port                                       int
		pricing                                    Pricing
		paid                                       bool
	}{
		{"smtp-custom", "SMTP", "", "starttls", "", "", 587, included, false},
		{"smtp-gmail", "Gmail / Google Workspace", "smtp.gmail.com", "tls", "", "https://support.google.com/a/answer/176600", 465, included, false},
		{"smtp-outlook", "Outlook.com", "smtp-mail.outlook.com", "starttls", "", "https://support.microsoft.com/office/d088b986-291d-42b8-9564-9c414e2aa040", 587, included, false},
		{"smtp-microsoft365", "Microsoft 365", "smtp.office365.com", "starttls", "", "https://learn.microsoft.com/en-us/exchange/mail-flow-best-practices/how-to-set-up-a-multifunction-device-or-application-to-send-email-using-microsoft-365-or-office-365", 587, included, false},
		{"smtp-qq", "QQ Mail", "smtp.qq.com", "tls", "", "https://service.mail.qq.com/", 465, included, false},
		{"smtp-163", "NetEase 163", "smtp.163.com", "tls", "", "https://help.mail.163.com/", 465, included, false},
		{"smtp-126", "NetEase 126", "smtp.126.com", "tls", "", "https://help.mail.163.com/", 465, included, false},
		{"smtp-cloudflare", "Cloudflare Email SMTP", "smtp.mx.cloudflare.net", "tls", "api_token", cfSource, 465, cf, true},
		{"smtp-sendgrid", "Twilio SendGrid SMTP · Essentials 50K", "smtp.sendgrid.net", "starttls", "apikey", sgSource, 587, sg, true},
		{"smtp-ses", "Amazon SES SMTP · us-east-1", "email-smtp.us-east-1.amazonaws.com", "starttls", "", sesSource, 587, ses, true},
		{"smtp-aliyun", "Alibaba Cloud Direct Mail SMTP", "smtpdm.aliyun.com", "tls", "", aliSource, 465, ali, true},
		{"smtp-tencent", "Tencent Cloud SES SMTP", "smtp.qcloudmail.com", "tls", "", tcSource, 465, tc, true},
		{"smtp-feishu", "Feishu Mail SMTP", "smtp.feishu.cn", "tls", "", "https://www.feishu.cn/service", 465, included, false},
	} {
		result = append(result, Preset{ID: entry.id, Name: entry.name, Account: Account{Provider: "smtp", SMTPHost: entry.host, SMTPPort: entry.port, SMTPSecurity: entry.security, Username: entry.username, Pricing: entry.pricing}, PriceSource: entry.source, PaidOverage: entry.paid})
	}
	for i := range result {
		p := &result[i]
		p.PriceChecked = "2026-09-08"
		p.Account.Preset = p.ID
		p.Account.Enabled = true
		p.Account.Quota.Period = "month"
		p.Account.Overage = Quota{Limit: -1, Period: "month"}
	}
	return result
}
