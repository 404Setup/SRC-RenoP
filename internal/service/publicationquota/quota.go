/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

// Package publicationquota coordinates bounded publication quota reservations.
package publicationquota

import (
	"errors"
	"log"
	"strings"
	"sync/atomic"
	"time"

	"renop/internal/config"
	"renop/internal/core"
)

const reservationTTL = 15 * time.Minute

// Reservation owns one durable quota claim until it is committed or released.
type Reservation struct {
	state *core.AppState
	db    core.StateDB
	id    string
	done  atomic.Bool
	free  bool
}

// Unmetered returns a completed no-op reservation for trusted internal publication paths.
func Unmetered(state *core.AppState) *Reservation {
	return &Reservation{state: state, free: true}
}

// Defaults returns the current global publication quota policy.
func Defaults(state *core.AppState) core.PublicationQuotaLimits {
	if state == nil || state.Inner == nil {
		return core.PublicationQuotaLimits{}
	}
	loaded := state.Inner.Config.Load()
	if loaded == nil {
		return core.PublicationQuotaLimits{}
	}
	quotaConfig := loaded.PublicationQuota
	if !core.ValidPublicationQuotaPeriod(quotaConfig.Period) {
		quotaConfig = config.DefaultPublicationQuotaConfig()
	}
	return core.PublicationQuotaLimits{
		FileLimit: quotaConfig.FileLimit, ByteLimit: quotaConfig.ByteLimit,
		PublicationLimit: quotaConfig.PublicationLimit, Period: quotaConfig.Period,
	}
}

// Enabled reports whether the loaded configuration explicitly initialized quota accounting.
func Enabled(state *core.AppState) bool {
	if state == nil || state.Inner == nil {
		return false
	}
	loaded := state.Inner.Config.Load()
	if loaded == nil {
		return false
	}
	quotaConfig := loaded.PublicationQuota
	return core.ValidPublicationQuotaPeriod(quotaConfig.Period)
}

// Subject selects the global team when a package is bound, otherwise the publishing account.
func Subject(username, superTeamPrefix string) core.PublicationQuotaSubject {
	if prefix, valid := core.NormalizeSuperTeamPrefix(superTeamPrefix); valid {
		return core.PublicationQuotaSubject{OwnerType: core.PublicationQuotaOwnerSuperTeam, OwnerKey: prefix}
	}
	return core.PublicationQuotaSubject{
		OwnerType: core.PublicationQuotaOwnerUser,
		OwnerKey:  strings.ToLower(strings.TrimSpace(username)),
	}
}

// Reserve creates a short-lived durable quota claim for one publication.
func Reserve(state *core.AppState, username, superTeamPrefix string,
	delta core.PublicationQuotaDelta,
) (*Reservation, error) {
	if !Enabled(state) {
		return Unmetered(state), nil
	}
	if state == nil || state.GetDB() == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	now := time.Now()
	record, err := state.GetDB().ReservePublicationQuota(
		Subject(username, superTeamPrefix), Defaults(state), delta,
		now.UnixMilli(), now.Add(reservationTTL).UnixMilli())
	if err != nil {
		return nil, err
	}
	return &Reservation{state: state, db: state.GetDB(), id: record.ID, free: record.Unlimited}, nil
}

// Commit converts a reservation into committed quota usage exactly once.
func (reservation *Reservation) Commit() error {
	if reservation == nil || !reservation.done.CompareAndSwap(false, true) {
		return nil
	}
	if reservation.free || reservation.id == "" {
		return nil
	}
	if err := reservation.db.CommitPublicationQuotaReservation(reservation.id, time.Now().UnixMilli()); err != nil {
		reservation.done.Store(false)
		return err
	}
	return nil
}

// Release removes an uncommitted reservation; it is safe to call from a defer.
func (reservation *Reservation) Release() {
	if reservation == nil || !reservation.done.CompareAndSwap(false, true) || reservation.free || reservation.id == "" {
		return
	}
	if err := reservation.db.ReleasePublicationQuotaReservation(reservation.id); err != nil {
		if reservation.state != nil && reservation.state.Inner != nil {
			reservation.state.Inner.FailuresCount.Add(1)
		}
		log.Printf("Failed to release publication quota reservation %s: %v", reservation.id, err)
	}
}

// ErrorCode maps quota failures to stable protocol-facing identifiers.
func ErrorCode(err error) string {
	switch {
	case errors.Is(err, core.ErrPublicationFileLimit):
		return "publication_file_quota"
	case errors.Is(err, core.ErrPublicationByteLimit):
		return "publication_byte_quota"
	case errors.Is(err, core.ErrPublicationCountLimit):
		return "publication_count_quota"
	default:
		return "publication_quota_unavailable"
	}
}

// Status returns effective limits and usage for an account or global team.
func Status(state *core.AppState, subject core.PublicationQuotaSubject) (*core.PublicationQuotaStatus, error) {
	if state == nil || state.GetDB() == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	return state.GetDB().GetPublicationQuotaStatus(subject, Defaults(state), time.Now().UnixMilli())
}
