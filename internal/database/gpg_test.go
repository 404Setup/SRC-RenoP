/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package database_test

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
)

func newGPGDatabase(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.InitDB(config.DatabaseConfig{
		Driver:       "sqlite",
		Dsn:          filepath.Join(t.TempDir(), "gpg.db"),
		MaxOpenConns: 1,
		MaxIdleConns: 1,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return db
}

func TestUserGPGKeyLimit(t *testing.T) {
	db := newGPGDatabase(t)
	for i := 1; i <= 10; i++ {
		fingerprint := fmt.Sprintf("%040X", i)
		keyID := fingerprint[len(fingerprint)-16:]
		err := db.RegisterUserGPGKey("Alice", keyID, &core.GPGPublicKey{
			Fingerprint: fingerprint,
			KeyID:       keyID,
			PublicKey:   []byte{byte(i)},
			FetchedAt:   time.Now().UnixMilli(),
		}, []string{fingerprint, keyID})
		require.NoError(t, err)
	}

	overflowFingerprint := fmt.Sprintf("%040X", 11)
	err := db.RegisterUserGPGKey("alice", overflowFingerprint[len(overflowFingerprint)-16:], &core.GPGPublicKey{
		Fingerprint: overflowFingerprint,
		KeyID:       overflowFingerprint[len(overflowFingerprint)-16:],
		PublicKey:   []byte("overflow"),
		FetchedAt:   time.Now().UnixMilli(),
	}, []string{overflowFingerprint})
	assert.ErrorIs(t, err, core.ErrGPGKeyLimit)

	keys, err := db.ListUserGPGKeys("ALICE")
	require.NoError(t, err)
	assert.Len(t, keys, 10)

	require.NoError(t, db.RegisterUserGPGKey("alice", keys[0].KeyID, &keys[0].GPGPublicKey, []string{keys[0].Fingerprint, keys[0].KeyID}))
	require.NoError(t, db.DeleteUserGPGKey("alice", keys[0].Fingerprint))
	require.NoError(t, db.RegisterUserGPGKey("alice", overflowFingerprint[len(overflowFingerprint)-16:], &core.GPGPublicKey{
		Fingerprint: overflowFingerprint,
		KeyID:       overflowFingerprint[len(overflowFingerprint)-16:],
		PublicKey:   []byte("overflow"),
		FetchedAt:   time.Now().UnixMilli(),
	}, []string{overflowFingerprint}))
}

func TestFindGPGPublicKeysByFingerprintAndKeyID(t *testing.T) {
	db := newGPGDatabase(t)
	const fingerprint = "1462C0512352DEC38A39D0793586B4EB0FDA2EA9"
	const keyID = "3586B4EB0FDA2EA9"
	key := &core.GPGPublicKey{
		Fingerprint: fingerprint,
		KeyID:       keyID,
		PublicKey:   []byte("public-key"),
		FetchedAt:   time.Now().UnixMilli(),
	}
	require.NoError(t, db.RegisterUserGPGKey("alice", fingerprint, key, []string{fingerprint, keyID}))

	for _, identifier := range []string{fingerprint, keyID} {
		found, err := db.FindGPGPublicKeys(identifier)
		require.NoError(t, err)
		require.Len(t, found, 1)
		assert.Equal(t, fingerprint, found[0].Fingerprint)
		assert.Equal(t, keyID, found[0].KeyID)
	}
}

func TestGPGSignatureBatchLookupAndPrefixDelete(t *testing.T) {
	db := newGPGDatabase(t)
	keys := make([]string, 0, 503)
	for i := 1; i <= 501; i++ {
		artifactKey := fmt.Sprintf("%064x", i)
		keys = append(keys, artifactKey)
		require.NoError(t, db.SaveGPGSignature(&core.GPGSignature{
			ArtifactKey:        artifactKey,
			Repository:         "releases",
			ArtifactPath:       fmt.Sprintf("org/example/demo/1.0/demo-%03d.jar", i),
			Fingerprint:        fmt.Sprintf("%040X", i),
			KeyID:              fmt.Sprintf("%016X", i),
			Uploader:           "alice",
			SignatureCreatedAt: time.Now().UnixMilli(),
			VerifiedAt:         time.Now().UnixMilli(),
			HashAlgorithm:      "SHA-256",
			PublicKeyAlgorithm: "EdDSA",
		}))
	}
	keys = append(keys, keys[0], "invalid")

	signatures, err := db.GetGPGSignatures(keys)
	require.NoError(t, err)
	assert.Len(t, signatures, 501)

	require.NoError(t, db.DeleteGPGSignaturesByPrefix("releases", "org/example/demo/1.0"))
	signatures, err = db.GetGPGSignatures(keys)
	require.NoError(t, err)
	assert.Empty(t, signatures)

	signature, err := db.GetGPGSignature(keys[0])
	require.NoError(t, err)
	assert.Nil(t, signature)
}

func TestGPGReleaseQueueLifecycleAndRecovery(t *testing.T) {
	db := newGPGDatabase(t)
	now := time.Now().UnixMilli()
	required := &core.GPGRelease{
		ID:                  "00000000-0000-0000-0000-000000000001",
		ActiveKey:           fmt.Sprintf("%064x", 1),
		Repository:          "releases",
		ArtifactPath:        "org/example/demo/1.0/demo-1.0.jar",
		Uploader:            "Alice",
		Status:              core.GPGReleaseQueued,
		RequireSignature:    true,
		ArtifactStagingPath: "quarantine/artifact",
		CreatedAt:           now - 10_000,
		UpdatedAt:           now - 10_000,
	}
	require.NoError(t, db.SaveGPGRelease(required))

	active, err := db.GetActiveGPGRelease(required.ActiveKey)
	require.NoError(t, err)
	require.NotNil(t, active)
	assert.Equal(t, "alice", active.Uploader)

	claimed, err := db.ClaimNextGPGRelease(now)
	require.NoError(t, err)
	assert.Nil(t, claimed, "required artifact must wait for its detached signature")

	duplicate := *required
	duplicate.ID = "00000000-0000-0000-0000-000000000002"
	assert.Error(t, db.SaveGPGRelease(&duplicate), "active artifact keys must be unique")

	required.SignatureStagingPath = "quarantine/signature"
	require.NoError(t, db.SaveGPGRelease(required))
	claimed, err = db.ClaimNextGPGRelease(now)
	require.NoError(t, err)
	require.NotNil(t, claimed)
	assert.Equal(t, required.ID, claimed.ID)
	assert.Equal(t, core.GPGReleaseValidating, claimed.Status)

	require.NoError(t, db.ResetValidatingGPGReleases())
	active, err = db.GetActiveGPGRelease(required.ActiveKey)
	require.NoError(t, err)
	require.NotNil(t, active)
	assert.Equal(t, core.GPGReleaseQueued, active.Status)

	active.Status = core.GPGReleaseSuccess
	active.ActiveKey = ""
	active.CompletedAt = now
	active.UpdatedAt = now
	require.NoError(t, db.SaveGPGRelease(active))

	standalone := &core.GPGRelease{
		ID:                   "00000000-0000-0000-0000-000000000003",
		ActiveKey:            fmt.Sprintf("%064x", 3),
		Repository:           "releases",
		ArtifactPath:         "org/example/demo/1.0/demo-1.0.pom",
		Uploader:             "alice",
		Status:               core.GPGReleaseQueued,
		RequireSignature:     true,
		SignatureStagingPath: "quarantine/signature-only",
		ArtifactExisted:      true,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	require.NoError(t, db.SaveGPGRelease(standalone))
	claimed, err = db.ClaimNextGPGRelease(now - 5_000)
	require.NoError(t, err)
	assert.Nil(t, claimed, "standalone signature must observe the artifact pairing grace period")

	standalone.CreatedAt = now - 10_000
	standalone.UpdatedAt = standalone.CreatedAt
	require.NoError(t, db.SaveGPGRelease(standalone))
	claimed, err = db.ClaimNextGPGRelease(now - 5_000)
	require.NoError(t, err)
	require.NotNil(t, claimed)
	assert.Equal(t, standalone.ID, claimed.ID)

	totalPending, userPending, err := db.CountPendingGPGReleases("ALICE")
	require.NoError(t, err)
	assert.Equal(t, 1, totalPending)
	assert.Equal(t, 1, userPending)

	releases, total, err := db.ListGPGReleases("alice", 1, 0)
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	require.Len(t, releases, 1)
	assert.Equal(t, standalone.ID, releases[0].ID)

	releases, total, err = db.ListGPGReleases("alice", 1, 1)
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	require.Len(t, releases, 1)
	assert.Equal(t, required.ID, releases[0].ID)
}
