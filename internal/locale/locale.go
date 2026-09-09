/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

// Package locale matches account and request languages to RenoP's supported catalogs.
package locale

import (
	"errors"
	"strings"

	"golang.org/x/text/language"
)

// Default is the fallback language for accounts and mail.
const Default = "en-US"

// ErrUnsupported indicates a preference that has no matching catalog.
var ErrUnsupported = errors.New("unsupported account language")

// Codes follows the canonical identifiers used by the frontend catalogs.
var Codes = [...]string{"en-US", "zh-CN", "zh-HK", "zh-TW", "zh-YUE", "ko-KR", "ja-JP", "de-DE", "fr-FR", "ru-RU", "es-ES", "pt-PT"}

var matcher = language.NewMatcher(func() []language.Tag {
	tags := make([]language.Tag, len(Codes))
	for i, code := range Codes {
		tags[i] = language.Make(code)
	}
	return tags
}())

// Match canonicalizes a supported language or returns an empty string.
func Match(value string) string {
	if len(value) > 64 || strings.TrimSpace(value) == "" {
		return ""
	}
	value = strings.TrimSpace(value)
	for _, code := range Codes {
		if strings.EqualFold(value, code) {
			return code
		}
	}
	tag, err := language.Parse(value)
	if err != nil {
		return ""
	}
	_, index, confidence := matcher.Match(tag)
	if confidence == language.No {
		return ""
	}
	return Codes[index]
}

// Resolve uses English when no supported account preference is available.
func Resolve(value string) string {
	if code := Match(value); code != "" {
		return code
	}
	return Default
}

// FromHeader respects Accept-Language quality weights and bounds parsing work.
func FromHeader(value string) string {
	if len(value) > 1024 {
		return Default
	}
	tags, _, err := language.ParseAcceptLanguage(value)
	if err != nil {
		return Default
	}
	_, index, _ := matcher.Match(tags...)
	return Codes[index]
}
