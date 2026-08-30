/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package database_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"renop/internal/core"
)

func TestPublicationQuotaReservationLifecycleAndOverrides(t *testing.T) {
	db := newMavenDB(t)
	now := time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC).UnixMilli()
	defaults := core.PublicationQuotaLimits{FileLimit: 2, ByteLimit: 100, PublicationLimit: 1, Period: "day"}
	subject := core.PublicationQuotaSubject{OwnerType: core.PublicationQuotaOwnerUser, OwnerKey: "alice"}

	status, err := db.GetPublicationQuotaStatus(subject, defaults, now)
	require.NoError(t, err)
	assert.True(t, status.Inherited)
	assert.EqualValues(t, 2, status.FileLimit)
	assert.Zero(t, status.FilesUsed)

	reservation, err := db.ReservePublicationQuota(subject, defaults,
		core.PublicationQuotaDelta{Files: 1, Bytes: 60, Publications: 1}, now, now+60_000)
	require.NoError(t, err)
	require.NotEmpty(t, reservation.ID)
	status, err = db.GetPublicationQuotaStatus(subject, defaults, now+1)
	require.NoError(t, err)
	assert.EqualValues(t, 1, status.FilesReserved)
	assert.EqualValues(t, 60, status.BytesReserved)
	assert.EqualValues(t, 1, status.PublicationsReserved)

	_, err = db.ReservePublicationQuota(subject, defaults,
		core.PublicationQuotaDelta{Files: 2}, now+2, now+60_000)
	require.ErrorIs(t, err, core.ErrPublicationFileLimit)
	_, err = db.ReservePublicationQuota(subject, defaults,
		core.PublicationQuotaDelta{Files: 1, Bytes: 41}, now+2, now+60_000)
	require.ErrorIs(t, err, core.ErrPublicationByteLimit)
	_, err = db.ReservePublicationQuota(subject, defaults,
		core.PublicationQuotaDelta{Files: 1, Publications: 1}, now+2, now+60_000)
	require.ErrorIs(t, err, core.ErrPublicationCountLimit)

	require.NoError(t, db.CommitPublicationQuotaReservation(reservation.ID, now+3))
	status, err = db.GetPublicationQuotaStatus(subject, defaults, now+4)
	require.NoError(t, err)
	assert.EqualValues(t, 1, status.FilesUsed)
	assert.EqualValues(t, 60, status.BytesUsed)
	assert.EqualValues(t, 1, status.PublicationsUsed)
	assert.Zero(t, status.FilesReserved)

	weekly := "week"
	files := int64(8)
	unlimited := false
	require.NoError(t, db.SetPublicationQuotaOverride(subject, core.PublicationQuotaOverride{
		FileLimit: &files, Period: &weekly, Unlimited: &unlimited,
	}, now+5))
	status, err = db.GetPublicationQuotaStatus(subject, defaults, now+6)
	require.NoError(t, err)
	assert.False(t, status.Inherited)
	assert.EqualValues(t, 8, status.FileLimit)
	assert.Equal(t, "week", status.Period)
	assert.Zero(t, status.FilesUsed)

	unlimited = true
	require.NoError(t, db.SetPublicationQuotaOverride(subject, core.PublicationQuotaOverride{Unlimited: &unlimited}, now+7))
	reservation, err = db.ReservePublicationQuota(subject, defaults,
		core.PublicationQuotaDelta{Files: 1_000_000, Bytes: 1 << 40, Publications: 1_000_000}, now+8, now+60_000)
	require.NoError(t, err)
	assert.True(t, reservation.Unlimited)
	assert.Empty(t, reservation.ID)
}

func TestPublicationQuotaConcurrentReservationsRespectLimit(t *testing.T) {
	db := newMavenDB(t)
	now := time.Now().UnixMilli()
	defaults := core.PublicationQuotaLimits{FileLimit: 1, ByteLimit: 100, PublicationLimit: 10, Period: "month"}
	subject := core.PublicationQuotaSubject{OwnerType: core.PublicationQuotaOwnerUser, OwnerKey: "bob"}
	errorsByAttempt := make([]error, 2)
	reservations := make([]*core.PublicationQuotaReservation, 2)
	var wait sync.WaitGroup
	for index := range errorsByAttempt {
		wait.Add(1)
		go func() {
			defer wait.Done()
			reservations[index], errorsByAttempt[index] = db.ReservePublicationQuota(subject, defaults,
				core.PublicationQuotaDelta{Files: 1, Bytes: 10}, now, now+60_000)
		}()
	}
	wait.Wait()
	successes, limited := 0, 0
	for index, err := range errorsByAttempt {
		if err == nil {
			successes++
			require.NoError(t, db.ReleasePublicationQuotaReservation(reservations[index].ID))
		} else if errors.Is(err, core.ErrPublicationFileLimit) {
			limited++
		} else {
			t.Fatalf("unexpected reservation error: %v", err)
		}
	}
	assert.Equal(t, 1, successes)
	assert.Equal(t, 1, limited)
}
