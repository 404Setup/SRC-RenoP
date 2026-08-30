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
	"errors"
	"time"
)

const (
	PublicationQuotaOwnerUser      = "user"
	PublicationQuotaOwnerSuperTeam = "super_team"
	PublicationQuotaPeriodDay      = "day"
	PublicationQuotaPeriodWeek     = "week"
	PublicationQuotaPeriodMonth    = "month"
)

var (
	ErrPublicationQuotaInvalid     = errors.New("publication quota request is invalid")
	ErrPublicationFileLimit        = errors.New("publication file quota exceeded")
	ErrPublicationByteLimit        = errors.New("publication byte quota exceeded")
	ErrPublicationCountLimit       = errors.New("publication count quota exceeded")
	ErrPublicationQuotaReservation = errors.New("publication quota reservation was not found")
)

// PublicationQuotaLimits defines one effective bounded publication policy.
type PublicationQuotaLimits struct {
	FileLimit        int64  `json:"file_limit"`
	ByteLimit        int64  `json:"byte_limit"`
	PublicationLimit int64  `json:"publication_limit"`
	Period           string `json:"period"`
}

// PublicationQuotaSubject identifies an account or global-team quota owner.
type PublicationQuotaSubject struct {
	OwnerType string `json:"owner_type"`
	OwnerKey  string `json:"owner_key"`
}

// PublicationQuotaDelta is the bounded cost of one completed publication operation.
type PublicationQuotaDelta struct {
	Files        int64 `json:"files"`
	Bytes        int64 `json:"bytes"`
	Publications int64 `json:"publications"`
}

// PublicationQuotaOverride contains nullable per-owner policy overrides.
type PublicationQuotaOverride struct {
	FileLimit        *int64  `json:"file_limit"`
	ByteLimit        *int64  `json:"byte_limit"`
	PublicationLimit *int64  `json:"publication_limit"`
	Period           *string `json:"period"`
	Unlimited        *bool   `json:"unlimited"`
}

// PublicationQuotaStatus reports effective limits, committed usage, and active reservations.
type PublicationQuotaStatus struct {
	OwnerType            string `json:"owner_type"`
	OwnerKey             string `json:"owner_key"`
	FileLimit            int64  `json:"file_limit"`
	ByteLimit            int64  `json:"byte_limit"`
	PublicationLimit     int64  `json:"publication_limit"`
	Period               string `json:"period"`
	PeriodStart          int64  `json:"period_start"`
	PeriodEnd            int64  `json:"period_end"`
	FilesUsed            int64  `json:"files_used"`
	BytesUsed            int64  `json:"bytes_used"`
	PublicationsUsed     int64  `json:"publications_used"`
	FilesReserved        int64  `json:"files_reserved"`
	BytesReserved        int64  `json:"bytes_reserved"`
	PublicationsReserved int64  `json:"publications_reserved"`
	Inherited            bool   `json:"inherited"`
	Unlimited            bool   `json:"unlimited"`
}

// PublicationQuotaReservation is a durable short-lived quota claim.
type PublicationQuotaReservation struct {
	ID        string
	Unlimited bool
}

// ValidPublicationQuotaPeriod reports whether a period is supported.
func ValidPublicationQuotaPeriod(period string) bool {
	return period == PublicationQuotaPeriodDay || period == PublicationQuotaPeriodWeek ||
		period == PublicationQuotaPeriodMonth
}

// PublicationQuotaWindow returns UTC period boundaries for a supported policy.
func PublicationQuotaWindow(period string, now time.Time) (time.Time, time.Time, bool) {
	now = now.UTC()
	var start time.Time
	switch period {
	case PublicationQuotaPeriodDay:
		start = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		return start, start.AddDate(0, 0, 1), true
	case PublicationQuotaPeriodWeek:
		start = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		daysSinceMonday := (int(start.Weekday()) + 6) % 7
		start = start.AddDate(0, 0, -daysSinceMonday)
		return start, start.AddDate(0, 0, 7), true
	case PublicationQuotaPeriodMonth:
		start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		return start, start.AddDate(0, 1, 0), true
	default:
		return time.Time{}, time.Time{}, false
	}
}
