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
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

const graphTrackingProperty = "String {ad312278-f68a-419c-a047-bb5c91897271} Name RenoPMessageID"

// Check queries one previously submitted message without sending it again.
func (c *Client) Check(ctx context.Context, a Account, m Message, previous Result) (Result, error) {
	result := previous
	result.Check = true
	endpoint := strings.TrimRight(a.Endpoint, "/")
	headers := http.Header{"Authorization": {"Bearer " + a.AccessToken}}
	var data map[string]any
	var err error
	switch a.Provider {
	case "ses":
		data, err = c.awsRequest(ctx, a, "GET", "/v2/email/insights/"+url.PathEscape(previous.MessageID)+"/", nil)
		if err == nil {
			for _, raw := range arrayAt(data, "Insights") {
				item, ok := raw.(map[string]any)
				if !ok || !strings.EqualFold(textAt(item, "Destination"), m.To) {
					continue
				}
				for _, rawEvent := range arrayAt(item, "Events") {
					event, ok := rawEvent.(map[string]any)
					if !ok {
						continue
					}
					kind := strings.ToUpper(textAt(event, "Type"))
					switch kind {
					case "BOUNCE", "REJECT", "COMPLAINT", "RENDERING_FAILURE":
						result.Status = "failed"
						result.Code = kind
						result.Check = false
						return result, nil
					case "DELIVERY", "OPEN", "CLICK":
						result.Status = "delivered"
						result.Check = false
					}
				}
			}
		}
	case "sendgrid":
		headers.Set("Authorization", "Bearer "+a.APIKey)
		// SendGrid appends a recipient-specific suffix to the X-Message-Id identifier.
		query := url.Values{"limit": {"10"}, "query": {"msg_id LIKE " + strconv.Quote(previous.MessageID+"%")}}
		data, _, err = c.jsonRequest(ctx, "GET", endpoint+"/messages?"+query.Encode(), nil, headers)
		if err == nil {
			for _, raw := range arrayAt(data, "messages") {
				item, ok := raw.(map[string]any)
				if !ok || !strings.EqualFold(textAt(item, "to_email"), m.To) || !strings.HasPrefix(textAt(item, "msg_id"), previous.MessageID) {
					continue
				}
				status := textAt(item, "status")
				switch status {
				case "delivered":
					result.Status = "delivered"
					result.Check = false
				case "not_delivered", "dropped", "bounced", "blocked":
					result.Status = "failed"
					result.Code = cleanCode(status)
					result.Check = false
				}
			}
		}
	case "tencent":
		date := time.UnixMilli(m.CreatedAt).In(time.FixedZone("UTC+8", 8*3600)).Format("2006-01-02")
		data, err = c.tencentRequest(ctx, a, "ses", "2020-10-02", "GetSendEmailStatus", map[string]any{"RequestDate": date, "Offset": 0, "Limit": 10, "MessageId": previous.MessageID, "ToEmailAddress": m.To})
		if err == nil {
			for _, raw := range arrayAt(data, "EmailStatusList") {
				item, ok := raw.(map[string]any)
				if !ok || textAt(item, "MessageId") != previous.MessageID || !strings.EqualFold(textAt(item, "ToEmailAddress"), m.To) {
					continue
				}
				status, hasStatus := numberAt(item, "SendStatus")
				delivery, hasDelivery := numberAt(item, "DeliverStatus")
				if hasStatus && status != 0 && status != 2001 {
					result.Status = "failed"
					result.Code = "TENCENT_" + strconv.Itoa(int(status))
					result.NotCharged = status == 1011 || status == 3007 || status == 3009 || status == 3010 || status == 3014
					result.Check = false
					return result, nil
				}
				if hasDelivery {
					switch delivery {
					case 1:
						result.Status = "delivered"
						result.Check = false
					case 2, 3:
						result.Status = "failed"
						result.Code = "TENCENT_DELIVERY_" + strconv.Itoa(int(delivery))
						result.Check = false
					}
				}
			}
		}
	case "graph":
		query := url.Values{"$top": {"1"}, "$select": {"id,isDraft"}, "$filter": {"singleValueExtendedProperties/Any(ep: ep/id eq '" + graphTrackingProperty + "' and ep/value eq '" + m.ID + "')"}}
		data, _, err = c.jsonRequest(ctx, "GET", graphMailbox(a)+"/mailFolders/sentitems/messages?"+query.Encode(), nil, headers)
		if err == nil && len(arrayAt(data, "value")) > 0 {
			result.Status = "sent"
			result.Check = false
		}
	case "gmail":
		data, _, err = c.jsonRequest(ctx, "GET", endpoint+"/users/me/messages/"+url.PathEscape(previous.MessageID)+"?format=minimal&fields=id,labelIds", nil, headers)
		if err == nil {
			for _, label := range arrayAt(data, "labelIds") {
				if label == "SENT" {
					result.Status = "sent"
					result.Check = false
				}
			}
		}
	case "feishu":
		data, _, err = c.jsonRequest(ctx, "GET", feishuMailbox(a)+"/messages/"+url.PathEscape(previous.MessageID)+"/send_status", nil, headers)
		if err == nil {
			err = feishuError(data)
			if err == nil && textAt(data, "data", "message_id") == previous.MessageID {
				for _, raw := range arrayAt(data, "data", "details") {
					item, ok := raw.(map[string]any)
					if !ok || !strings.EqualFold(textAt(item, "recipient", "mail_address"), m.To) {
						continue
					}
					if state, ok := numberAt(item, "status"); ok {
						switch state {
						case 4:
							result.Status, result.Check = "delivered", false
						case 3, 6:
							result.Status, result.Check = "failed", false
							result.Code = "FEISHU_DELIVERY_" + strconv.Itoa(int(state))
						}
					}
				}
			}
		}
	case "aliyun":
		start := time.UnixMilli(m.CreatedAt).UTC()
		end := start.Add(10 * time.Minute)
		_, err = c.aliRequest(ctx, a, "2015-11-23", "SenderStatisticsDetailByParam", url.Values{"ToAddress": {m.To}, "StartTime": {start.Format("2006-01-02 15:04")}, "EndTime": {end.Format("2006-01-02 15:04")}, "Length": {"10"}})
		if err == nil {
			// Direct Mail's public response omits message IDs, so another send cannot be safely correlated.
			result.Status = "unknown"
			result.Code = "mail_status_uncorrelated"
			result.Check = false
		}
	default:
		return previous, errUnsupported
	}
	if err != nil {
		return previous, err
	}
	return result, nil
}

// Terminal reports whether further provider polling could change the tracked result.
func (r Result) Terminal() bool {
	return !r.Check || slices.Contains([]string{"delivered", "failed", "sent", "unknown"}, r.Status)
}
