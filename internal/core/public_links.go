/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package core

import (
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	MaxPublicLinkURLLength       = 2048
	MaxPublicCustomLinkNameRunes = 40
)

// PublicLinks contains the bounded external links shared by user and global-team profiles.
type PublicLinks struct {
	Website    string `json:"website"`
	GitHub     string `json:"github"`
	Discord    string `json:"discord"`
	CustomName string `json:"custom_name"`
	CustomURL  string `json:"custom_url"`
}

func normalizePublicLinkURL(raw string, allowedHosts ...string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", true
	}
	if len(raw) > MaxPublicLinkURLLength || !utf8.ValidString(raw) || strings.IndexFunc(raw, unicode.IsSpace) >= 0 {
		return "", false
	}
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || parsed.Host == "" || parsed.Hostname() == "" || parsed.User != nil ||
		!strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https") {
		return "", false
	}
	if len(allowedHosts) > 0 {
		host := strings.ToLower(parsed.Hostname())
		allowed := false
		for _, suffix := range allowedHosts {
			if host == suffix || strings.HasSuffix(host, "."+suffix) {
				allowed = true
				break
			}
		}
		if !allowed {
			return "", false
		}
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	return parsed.String(), true
}

// NormalizePublicLinks validates profile links and returns their canonical representation.
func NormalizePublicLinks(links PublicLinks) (PublicLinks, bool) {
	var valid bool
	if links.Website, valid = normalizePublicLinkURL(links.Website); !valid {
		return PublicLinks{}, false
	}
	if links.GitHub, valid = normalizePublicLinkURL(links.GitHub, "github.com"); !valid {
		return PublicLinks{}, false
	}
	if links.Discord, valid = normalizePublicLinkURL(links.Discord, "discord.com", "discord.gg", "discordapp.com"); !valid {
		return PublicLinks{}, false
	}
	if links.CustomURL, valid = normalizePublicLinkURL(links.CustomURL); !valid {
		return PublicLinks{}, false
	}
	links.CustomName = strings.TrimSpace(links.CustomName)
	if !utf8.ValidString(links.CustomName) || utf8.RuneCountInString(links.CustomName) > MaxPublicCustomLinkNameRunes ||
		strings.IndexFunc(links.CustomName, unicode.IsControl) >= 0 || (links.CustomName == "") != (links.CustomURL == "") {
		return PublicLinks{}, false
	}
	return links, true
}
