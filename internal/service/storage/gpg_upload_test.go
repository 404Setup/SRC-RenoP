/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package storage

import (
	"bytes"
	"context"
	"crypto"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/packet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
	"renop/internal/service/gpg"
	"renop/internal/service/index"
)

func setupGPGUploadState(t *testing.T) (*core.AppState, *database.DB, *config.Repository, string) {
	t.Helper()
	storagePath := storageTestTempDir(t)
	cfg := config.DefaultConfig()
	cfg.StoragePath = storagePath
	cfg.Database = config.DatabaseConfig{
		Driver:       "sqlite",
		Dsn:          filepath.Join(storagePath, "gpg-upload.db"),
		MaxOpenConns: 1,
		MaxIdleConns: 1,
	}
	repo := &config.Repository{
		Name:                "releases",
		Visibility:          "PUBLIC",
		RequireGPGSignature: true,
	}
	cfg.Maven.Repositories[repo.Name] = repo
	InitS3(cfg)

	db, err := database.InitDB(cfg.Database)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	state := core.NewAppState()
	state.Inner.Config.Store(cfg)
	state.Inner.DB = db
	state.Inner.FileIndex = index.NewFileIndexCustom(true)
	return state, db, repo, storagePath
}

func registerTestSigner(t *testing.T, db *database.DB, username string) *openpgp.Entity {
	t.Helper()
	entity, err := openpgp.NewEntity("Upload Signer", "", "uploader@example.test", &packet.Config{
		Algorithm:   packet.PubKeyAlgoEdDSA,
		DefaultHash: crypto.SHA256,
	})
	require.NoError(t, err)
	var serialized bytes.Buffer
	require.NoError(t, entity.Serialize(&serialized))
	publicEntities, err := openpgp.ReadKeyRing(bytes.NewReader(serialized.Bytes()))
	require.NoError(t, err)
	require.Len(t, publicEntities, 1)
	publicEntity := publicEntities[0]

	aliases := []string{
		strings.ToUpper(hex.EncodeToString(publicEntity.PrimaryKey.Fingerprint)),
		fmt.Sprintf("%016X", publicEntity.PrimaryKey.KeyId),
	}
	for i := range publicEntity.Subkeys {
		if publicEntity.Subkeys[i].PublicKey == nil {
			continue
		}
		aliases = append(aliases,
			strings.ToUpper(hex.EncodeToString(publicEntity.Subkeys[i].PublicKey.Fingerprint)),
			fmt.Sprintf("%016X", publicEntity.Subkeys[i].PublicKey.KeyId),
		)
	}
	key := &core.GPGPublicKey{
		Fingerprint:     aliases[0],
		KeyID:           aliases[1],
		PrimaryIdentity: "Upload Signer",
		PublicKey:       serialized.Bytes(),
		KeyCreatedAt:    publicEntity.PrimaryKey.CreationTime.UnixMilli(),
		FetchedAt:       time.Now().UnixMilli(),
	}
	require.NoError(t, db.RegisterUserGPGKey(username, key.KeyID, key, aliases))
	return entity
}

func preparedTestUpload(t *testing.T, finalPath, username string, content []byte) *PreparedUpload {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(finalPath), 0755))
	temp, err := os.CreateTemp(filepath.Dir(finalPath), ".gpg-upload-*")
	require.NoError(t, err)
	tempPath := temp.Name()
	t.Cleanup(func() { _ = os.Remove(tempPath) })
	_, err = temp.Write(content)
	require.NoError(t, err)
	require.NoError(t, temp.Close())
	info, err := os.Stat(tempPath)
	require.NoError(t, err)
	return &PreparedUpload{
		LocalFilePath: finalPath,
		TempPath:      tempPath,
		Username:      username,
		FileSize:      info.Size(),
		ModTime:       info.ModTime().UnixNano(),
	}
}

func signTestArtifact(t *testing.T, signer *openpgp.Entity, content []byte) []byte {
	t.Helper()
	var signature bytes.Buffer
	require.NoError(t, openpgp.ArmoredDetachSign(
		&signature,
		signer,
		bytes.NewReader(content),
		&packet.Config{DefaultHash: crypto.SHA256},
	))
	return signature.Bytes()
}

func getOnlyRelease(t *testing.T, db *database.DB, username string) *core.GPGRelease {
	t.Helper()
	releases, total, err := db.ListGPGReleases(username, 20, 0)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, releases, 1)
	return releases[0]
}

func TestRequiredGPGUploadPublishesOnlyFromBackgroundQueue(t *testing.T) {
	state, db, repo, storagePath := setupGPGUploadState(t)
	signer := registerTestSigner(t, db, "alice")
	artifactBytes := []byte("verified artifact payload")
	artifactPath := filepath.Join(storagePath, "releases", "org", "example", "demo", "1.0", "demo-1.0.jar")

	artifactUpload := preparedTestUpload(t, artifactPath, "alice", artifactBytes)
	artifactResult, err := ProcessUploadedFile(context.Background(), state, repo, artifactUpload)
	require.NoError(t, err)
	assert.True(t, artifactResult.Pending)
	assert.NotEmpty(t, artifactResult.ReleaseID)
	assert.Empty(t, artifactUpload.TempPath)
	assert.NoFileExists(t, artifactPath)
	assert.True(t, state.Inner.FileIndex.IsBlocked(artifactPath))

	checksumBytes := []byte("client supplied checksum")
	checksumUpload := preparedTestUpload(t, artifactPath+".sha1", "alice", checksumBytes)
	checksumResult, err := ProcessUploadedFile(context.Background(), state, repo, checksumUpload)
	require.NoError(t, err)
	assert.True(t, checksumResult.Pending)
	assert.Equal(t, artifactResult.ReleaseID, checksumResult.ReleaseID)
	assert.Empty(t, checksumUpload.TempPath)
	assert.NoFileExists(t, artifactPath+".sha1")

	signatureUpload := preparedTestUpload(t, artifactPath+".asc", "alice", signTestArtifact(t, signer, artifactBytes))
	signatureResult, err := ProcessUploadedFile(context.Background(), state, repo, signatureUpload)
	require.NoError(t, err)
	assert.True(t, signatureResult.Pending)
	assert.Equal(t, artifactResult.ReleaseID, signatureResult.ReleaseID)
	assert.NoFileExists(t, artifactPath)

	processGPGReleaseQueue(state)
	assert.FileExists(t, artifactPath)
	assert.FileExists(t, artifactPath+".asc")
	storedChecksum, err := os.ReadFile(artifactPath + ".sha1")
	require.NoError(t, err)
	assert.Equal(t, checksumBytes, storedChecksum)
	assert.True(t, state.Inner.FileIndex.HasFile(artifactPath))
	assert.False(t, state.Inner.FileIndex.IsBlocked(artifactPath))

	release := getOnlyRelease(t, db, "alice")
	assert.Equal(t, core.GPGReleaseSuccess, release.Status)
	assert.False(t, release.CleanupPending)
	record, err := db.GetGPGSignature(gpg.ArtifactKey("releases", "org/example/demo/1.0/demo-1.0.jar"))
	require.NoError(t, err)
	require.NotNil(t, record)
	assert.Equal(t, "alice", record.Uploader)
}

func TestSignedSnapshotRedeploymentKeepsNewDetachedSignature(t *testing.T) {
	state, db, repo, storagePath := setupGPGUploadState(t)
	repo.AllowRedeployment = true
	signer := registerTestSigner(t, db, "alice")
	artifactBytes := []byte("new snapshot payload")
	artifactPath := filepath.Join(storagePath, "releases", "org", "example", "demo", "1.0-SNAPSHOT", "demo-1.0-SNAPSHOT.jar")
	require.NoError(t, os.MkdirAll(filepath.Dir(artifactPath), 0755))
	require.NoError(t, os.WriteFile(artifactPath, []byte("old snapshot payload"), 0644))
	require.NoError(t, os.WriteFile(artifactPath+".asc", []byte("old signature"), 0644))
	state.Inner.FileIndex.EnsureParentDirs(artifactPath)
	state.Inner.FileIndex.InsertFile(artifactPath, index.FileInfo{Size: 20, ModTime: 1})
	state.Inner.FileIndex.InsertFile(artifactPath+".asc", index.FileInfo{Size: 13, ModTime: 1})

	artifactUpload := preparedTestUpload(t, artifactPath, "alice", artifactBytes)
	artifactUpload.Existed = true
	_, err := ProcessUploadedFile(context.Background(), state, repo, artifactUpload)
	require.NoError(t, err)
	signatureBytes := signTestArtifact(t, signer, artifactBytes)
	signatureUpload := preparedTestUpload(t, artifactPath+".asc", "alice", signatureBytes)
	signatureUpload.Existed = true
	_, err = ProcessUploadedFile(context.Background(), state, repo, signatureUpload)
	require.NoError(t, err)

	processGPGReleaseQueue(state)
	assert.FileExists(t, artifactPath+".asc")
	storedArtifact, err := os.ReadFile(artifactPath)
	require.NoError(t, err)
	assert.Equal(t, artifactBytes, storedArtifact)
	storedSignature, err := os.ReadFile(artifactPath + ".asc")
	require.NoError(t, err)
	assert.Equal(t, signatureBytes, storedSignature)
}

func TestRequiredGPGChecksumCanArriveBeforeArtifact(t *testing.T) {
	state, db, repo, storagePath := setupGPGUploadState(t)
	signer := registerTestSigner(t, db, "alice")
	artifactBytes := []byte("artifact after checksum")
	artifactPath := filepath.Join(storagePath, "releases", "org", "example", "demo", "1.0", "demo-1.0.pom")
	checksumBytes := []byte("checksum uploaded first")

	checksumUpload := preparedTestUpload(t, artifactPath+".sha512", "alice", checksumBytes)
	checksumResult, err := ProcessUploadedFile(context.Background(), state, repo, checksumUpload)
	require.NoError(t, err)
	assert.True(t, checksumResult.Pending)
	assert.Empty(t, checksumUpload.TempPath)
	assert.True(t, state.Inner.FileIndex.IsBlocked(artifactPath+".sha512"))
	assert.NoFileExists(t, artifactPath+".sha512")

	artifactResult, err := ProcessUploadedFile(context.Background(), state, repo,
		preparedTestUpload(t, artifactPath, "alice", artifactBytes))
	require.NoError(t, err)
	assert.Equal(t, checksumResult.ReleaseID, artifactResult.ReleaseID)
	signatureResult, err := ProcessUploadedFile(context.Background(), state, repo,
		preparedTestUpload(t, artifactPath+".asc", "alice", signTestArtifact(t, signer, artifactBytes)))
	require.NoError(t, err)
	assert.Equal(t, checksumResult.ReleaseID, signatureResult.ReleaseID)

	processGPGReleaseQueue(state)
	storedChecksum, err := os.ReadFile(artifactPath + ".sha512")
	require.NoError(t, err)
	assert.Equal(t, checksumBytes, storedChecksum)
	release := getOnlyRelease(t, db, "alice")
	assert.Equal(t, core.GPGReleaseSuccess, release.Status)
}

func TestRequiredGPGChecksumAfterPublicationDoesNotCreateRelease(t *testing.T) {
	state, db, repo, storagePath := setupGPGUploadState(t)
	signer := registerTestSigner(t, db, "alice")
	artifactBytes := []byte("published before checksum")
	artifactPath := filepath.Join(storagePath, "releases", "org", "example", "demo", "1.0", "demo-1.0.jar")

	_, err := ProcessUploadedFile(context.Background(), state, repo,
		preparedTestUpload(t, artifactPath, "alice", artifactBytes))
	require.NoError(t, err)
	_, err = ProcessUploadedFile(context.Background(), state, repo,
		preparedTestUpload(t, artifactPath+".asc", "alice", signTestArtifact(t, signer, artifactBytes)))
	require.NoError(t, err)
	processGPGReleaseQueue(state)
	require.FileExists(t, artifactPath)

	checksumBytes := []byte("checksum uploaded after publication")
	result, err := ProcessUploadedFile(context.Background(), state, repo,
		preparedTestUpload(t, artifactPath+".sha256", "alice", checksumBytes))
	require.NoError(t, err)
	assert.False(t, result.Pending)
	storedChecksum, err := os.ReadFile(artifactPath + ".sha256")
	require.NoError(t, err)
	assert.Equal(t, checksumBytes, storedChecksum)
	assert.False(t, state.Inner.FileIndex.IsBlocked(artifactPath))
	assert.True(t, state.Inner.FileIndex.HasFile(artifactPath))

	_, total, err := db.ListGPGReleases("alice", 20, 0)
	require.NoError(t, err)
	assert.Equal(t, 1, total)
}

func TestPendingGPGReleasePreventsStoragePathChange(t *testing.T) {
	state, _, repo, storagePath := setupGPGUploadState(t)
	artifactPath := filepath.Join(storagePath, "releases", "org", "example", "demo", "1.0", "demo-1.0.jar")

	_, err := ProcessUploadedFile(context.Background(), state, repo,
		preparedTestUpload(t, artifactPath, "alice", []byte("pending artifact")))
	require.NoError(t, err)
	unlock, err := AcquireGPGStoragePathChange(state)
	assert.ErrorIs(t, err, ErrGPGStoragePathChange)
	assert.Nil(t, unlock)
}

func TestRequiredGPGChecksumOnlyReleaseExpiresAndUnblocks(t *testing.T) {
	state, db, repo, storagePath := setupGPGUploadState(t)
	artifactPath := filepath.Join(storagePath, "releases", "org", "example", "demo", "1.0", "demo-1.0.pom")
	checksumPath := artifactPath + ".sha1"
	_, err := ProcessUploadedFile(context.Background(), state, repo,
		preparedTestUpload(t, checksumPath, "alice", []byte("orphaned checksum")))
	require.NoError(t, err)
	activeKey := gpg.ArtifactKey("releases", "org/example/demo/1.0/demo-1.0.pom")
	release, err := db.GetActiveGPGRelease(activeKey)
	require.NoError(t, err)
	require.NotNil(t, release)
	release.CreatedAt = time.Now().Add(-2 * gpgPendingTTL).UnixMilli()
	release.UpdatedAt = release.CreatedAt
	require.NoError(t, db.SaveGPGRelease(release))

	processGPGReleaseQueue(state)
	assert.NoFileExists(t, checksumPath)
	assert.False(t, state.Inner.FileIndex.IsBlocked(artifactPath))
	assert.False(t, state.Inner.FileIndex.IsBlocked(checksumPath))
	release = getOnlyRelease(t, db, "alice")
	assert.Equal(t, core.GPGReleaseFailed, release.Status)
	assert.Contains(t, release.FailureReason, "artifact")
}

func TestRequiredGPGUploadFailureDeletesQuarantineAndNeverIndexes(t *testing.T) {
	state, db, repo, storagePath := setupGPGUploadState(t)
	signer := registerTestSigner(t, db, "alice")
	artifactPath := filepath.Join(storagePath, "releases", "org", "example", "demo", "1.0", "demo-1.0.jar")

	artifactResult, err := ProcessUploadedFile(context.Background(), state, repo,
		preparedTestUpload(t, artifactPath, "alice", []byte("artifact payload")))
	require.NoError(t, err)
	checksumUpload := preparedTestUpload(t, artifactPath+".sha256", "alice", []byte("failed checksum"))
	checksumResult, err := ProcessUploadedFile(context.Background(), state, repo, checksumUpload)
	require.NoError(t, err)
	assert.True(t, checksumResult.Pending)
	assert.Equal(t, artifactResult.ReleaseID, checksumResult.ReleaseID)
	_, err = ProcessUploadedFile(context.Background(), state, repo,
		preparedTestUpload(t, artifactPath+".asc", "alice", signTestArtifact(t, signer, []byte("different payload"))))
	require.NoError(t, err)

	queued, err := db.GetActiveGPGRelease(gpg.ArtifactKey("releases", "org/example/demo/1.0/demo-1.0.jar"))
	require.NoError(t, err)
	require.NotNil(t, queued)
	stagedArtifact := queued.ArtifactStagingPath
	stagedSignature := queued.SignatureStagingPath
	processGPGReleaseQueue(state)

	release := getOnlyRelease(t, db, "alice")
	assert.Equal(t, artifactResult.ReleaseID, release.ID)
	assert.Equal(t, core.GPGReleaseFailed, release.Status)
	assert.Contains(t, release.FailureReason, "invalid")
	assert.False(t, release.CleanupPending)
	assert.NoFileExists(t, stagedArtifact)
	assert.NoFileExists(t, stagedSignature)
	assert.NoFileExists(t, artifactPath)
	assert.NoFileExists(t, artifactPath+".asc")
	assert.NoFileExists(t, artifactPath+".sha256")
	assert.False(t, state.Inner.FileIndex.HasFile(artifactPath))
	assert.False(t, state.Inner.FileIndex.IsBlocked(artifactPath))
}

func TestFailedGPGRedeploymentPreservesExistingArtifactCompanions(t *testing.T) {
	state, db, repo, storagePath := setupGPGUploadState(t)
	repo.AllowRedeployment = true
	signer := registerTestSigner(t, db, "alice")
	artifactPath := filepath.Join(storagePath, "releases", "org", "example", "demo", "1.0", "demo-1.0.jar")
	require.NoError(t, os.MkdirAll(filepath.Dir(artifactPath), 0755))
	require.NoError(t, os.WriteFile(artifactPath, []byte("existing artifact"), 0644))
	require.NoError(t, os.WriteFile(artifactPath+".sha1", []byte("existing checksum"), 0644))
	state.Inner.FileIndex.EnsureParentDirs(artifactPath)
	state.Inner.FileIndex.InsertFile(artifactPath, index.FileInfo{Size: 17, ModTime: 1})
	state.Inner.FileIndex.InsertFile(artifactPath+".sha1", index.FileInfo{Size: 17, ModTime: 1})

	artifactUpload := preparedTestUpload(t, artifactPath, "alice", []byte("replacement artifact"))
	artifactUpload.Existed = true
	_, err := ProcessUploadedFile(context.Background(), state, repo, artifactUpload)
	require.NoError(t, err)
	checksumUpload := preparedTestUpload(t, artifactPath+".sha1", "alice", []byte("replacement checksum"))
	checksumUpload.Existed = true
	_, err = ProcessUploadedFile(context.Background(), state, repo, checksumUpload)
	require.NoError(t, err)
	_, err = ProcessUploadedFile(context.Background(), state, repo,
		preparedTestUpload(t, artifactPath+".asc", "alice", signTestArtifact(t, signer, []byte("different artifact"))))
	require.NoError(t, err)

	processGPGReleaseQueue(state)
	storedArtifact, err := os.ReadFile(artifactPath)
	require.NoError(t, err)
	assert.Equal(t, []byte("existing artifact"), storedArtifact)
	storedChecksum, err := os.ReadFile(artifactPath + ".sha1")
	require.NoError(t, err)
	assert.Equal(t, []byte("existing checksum"), storedChecksum)
	assert.True(t, state.Inner.FileIndex.HasFile(artifactPath))
	assert.True(t, state.Inner.FileIndex.HasFile(artifactPath+".sha1"))
}

func TestGPGExpirationReloadsReleaseBeforeFailingIt(t *testing.T) {
	state, db, repo, storagePath := setupGPGUploadState(t)
	signer := registerTestSigner(t, db, "alice")
	artifactBytes := []byte("paired before expiration")
	artifactPath := filepath.Join(storagePath, "releases", "org", "example", "demo", "1.0", "demo-1.0.module")
	_, err := ProcessUploadedFile(context.Background(), state, repo,
		preparedTestUpload(t, artifactPath, "alice", artifactBytes))
	require.NoError(t, err)
	stale, err := db.GetActiveGPGRelease(gpg.ArtifactKey("releases", "org/example/demo/1.0/demo-1.0.module"))
	require.NoError(t, err)
	require.NotNil(t, stale)
	stale.CreatedAt = time.Now().Add(-2 * gpgPendingTTL).UnixMilli()
	stale.UpdatedAt = stale.CreatedAt
	require.NoError(t, db.SaveGPGRelease(stale))
	observed := *stale

	_, err = ProcessUploadedFile(context.Background(), state, repo,
		preparedTestUpload(t, artifactPath+".asc", "alice", signTestArtifact(t, signer, artifactBytes)))
	require.NoError(t, err)
	require.NoError(t, expireGPGReleaseIfIncomplete(state, &observed, time.Now()))
	current, err := db.GetActiveGPGRelease(observed.ActiveKey)
	require.NoError(t, err)
	require.NotNil(t, current)
	assert.Equal(t, core.GPGReleaseQueued, current.Status)
	assert.NotEmpty(t, current.SignatureStagingPath)

	processGPGReleaseQueue(state)
	assert.FileExists(t, artifactPath)
	release := getOnlyRelease(t, db, "alice")
	assert.Equal(t, core.GPGReleaseSuccess, release.Status)
}

func TestCancelledValidationCannotPublishAfterStatusChanges(t *testing.T) {
	state, db, repo, storagePath := setupGPGUploadState(t)
	signer := registerTestSigner(t, db, "alice")
	artifactBytes := []byte("cancelled artifact")
	artifactPath := filepath.Join(storagePath, "releases", "org", "example", "demo", "1.0", "demo-1.0.module")
	_, err := ProcessUploadedFile(context.Background(), state, repo,
		preparedTestUpload(t, artifactPath, "alice", artifactBytes))
	require.NoError(t, err)
	_, err = ProcessUploadedFile(context.Background(), state, repo,
		preparedTestUpload(t, artifactPath+".asc", "alice", signTestArtifact(t, signer, artifactBytes)))
	require.NoError(t, err)

	claimed, err := db.ClaimNextGPGRelease(time.Now().UnixMilli())
	require.NoError(t, err)
	require.NotNil(t, claimed)
	active, err := db.GetActiveGPGRelease(claimed.ActiveKey)
	require.NoError(t, err)
	require.NotNil(t, active)
	active.Status = core.GPGReleaseFailed
	active.FailureReason = "cancelled by test"
	active.CleanupPending = true
	active.CompletedAt = time.Now().UnixMilli()
	require.NoError(t, db.SaveGPGRelease(active))

	err = publishGPGRelease(state, claimed)
	assert.ErrorIs(t, err, errGPGReleaseInactive)
	assert.NoFileExists(t, artifactPath)
	assert.NoFileExists(t, artifactPath+".asc")
	assert.False(t, state.Inner.FileIndex.IsBlocked(artifactPath))
	active, err = db.GetActiveGPGRelease(claimed.ActiveKey)
	require.NoError(t, err)
	assert.Nil(t, active)
}

func TestOptionalUnsignedProtectedUploadPublishesImmediately(t *testing.T) {
	state, _, repo, storagePath := setupGPGUploadState(t)
	repo.RequireGPGSignature = false
	artifactPath := filepath.Join(storagePath, "releases", "org", "example", "demo", "1.0", "demo-1.0.pom")
	result, err := ProcessUploadedFile(context.Background(), state, repo,
		preparedTestUpload(t, artifactPath, "", []byte("<project/>")))
	require.NoError(t, err)
	assert.False(t, result.Pending)
	assert.FileExists(t, artifactPath)
	assert.True(t, state.Inner.FileIndex.HasFile(artifactPath))
}

func TestOptionalUnsignedUploadCannotReplaceActiveGPGRelease(t *testing.T) {
	state, _, repo, storagePath := setupGPGUploadState(t)
	artifactPath := filepath.Join(storagePath, "releases", "org", "example", "demo", "1.0", "demo-1.0.pom")
	_, err := ProcessUploadedFile(context.Background(), state, repo,
		preparedTestUpload(t, artifactPath, "alice", []byte("<project>queued</project>")))
	require.NoError(t, err)

	repo.RequireGPGSignature = false
	replacement := preparedTestUpload(t, artifactPath, "", []byte("<project>replacement</project>"))
	_, err = ProcessUploadedFile(context.Background(), state, repo, replacement)
	assert.ErrorIs(t, err, ErrGPGPendingConflict)
	assert.NotEmpty(t, replacement.TempPath)
	assert.NoFileExists(t, artifactPath)
}

func TestStandaloneSignatureTemporarilyHidesExistingArtifact(t *testing.T) {
	state, db, repo, storagePath := setupGPGUploadState(t)
	repo.RequireGPGSignature = false
	signer := registerTestSigner(t, db, "alice")
	artifactPath := filepath.Join(storagePath, "releases", "org", "example", "demo", "1.0", "demo-1.0.jar")
	_, err := ProcessUploadedFile(context.Background(), state, repo,
		preparedTestUpload(t, artifactPath, "", []byte("existing artifact")))
	require.NoError(t, err)
	require.True(t, state.Inner.FileIndex.HasFile(artifactPath))

	_, err = ProcessUploadedFile(context.Background(), state, repo,
		preparedTestUpload(t, artifactPath+".asc", "alice", signTestArtifact(t, signer, []byte("different artifact"))))
	require.NoError(t, err)
	assert.False(t, state.Inner.FileIndex.HasFile(artifactPath))
	assert.True(t, state.Inner.FileIndex.IsBlocked(artifactPath))

	release := getOnlyRelease(t, db, "alice")
	release.CreatedAt = time.Now().Add(-2 * gpgOptionalSignatureGrace).UnixMilli()
	release.UpdatedAt = release.CreatedAt
	require.NoError(t, db.SaveGPGRelease(release))
	processGPGReleaseQueue(state)

	assert.FileExists(t, artifactPath)
	assert.NoFileExists(t, artifactPath+".asc")
	assert.False(t, state.Inner.FileIndex.IsBlocked(artifactPath))
	assert.True(t, state.Inner.FileIndex.HasFile(artifactPath))
	release = getOnlyRelease(t, db, "alice")
	assert.Equal(t, core.GPGReleaseFailed, release.Status)
}

func TestGPGReleaseBlacklistRestoresAcrossRestartAndRebuild(t *testing.T) {
	state, db, repo, storagePath := setupGPGUploadState(t)
	artifactPath := filepath.Join(storagePath, "releases", "org", "example", "demo", "1.0", "demo-1.0.module")
	_, err := ProcessUploadedFile(context.Background(), state, repo,
		preparedTestUpload(t, artifactPath, "alice", []byte("module")))
	require.NoError(t, err)

	// Simulate an object that a watcher or rebuild could discover while the
	// durable job is still pending.
	require.NoError(t, os.WriteFile(artifactPath, []byte("must stay hidden"), 0644))
	orphanPath := filepath.Join(storagePath, gpgQuarantineDirName, "orphaned-upload", "artifact")
	require.NoError(t, os.MkdirAll(filepath.Dir(orphanPath), 0700))
	require.NoError(t, os.WriteFile(orphanPath, []byte("orphaned"), 0600))
	restarted := core.NewAppState()
	restarted.Inner.Config.Store(state.Inner.Config.Load())
	restarted.Inner.DB = db
	restarted.Inner.FileIndex = index.NewFileIndexCustom(true)
	require.NoError(t, RestoreGPGReleaseState(restarted))
	require.NoError(t, index.BuildIndexSync(storagePath, restarted.Inner.FileIndex))
	assert.NoFileExists(t, orphanPath)
	assert.True(t, restarted.Inner.FileIndex.IsBlocked(artifactPath))
	assert.False(t, restarted.Inner.FileIndex.HasFile(artifactPath))
}

func TestRequiredGPGPolicyDoesNotCoverOtherFileTypes(t *testing.T) {
	state, _, repo, storagePath := setupGPGUploadState(t)
	zipPath := filepath.Join(storagePath, "releases", "org", "example", "demo", "1.0", "demo-1.0.zip")
	upload := preparedTestUpload(t, zipPath, "", []byte("archive"))
	result, err := ProcessUploadedFile(context.Background(), state, repo, upload)
	require.NoError(t, err)
	assert.False(t, result.Pending)
	assert.FileExists(t, zipPath)
}

func TestGPGSignatureRequiresCanonicalSuffix(t *testing.T) {
	state, _, repo, storagePath := setupGPGUploadState(t)
	signaturePath := filepath.Join(storagePath, "releases", "org", "example", "demo", "1.0", "demo-1.0.jar.ASC")
	upload := preparedTestUpload(t, signaturePath, "alice", []byte("not a signature"))
	_, err := ProcessUploadedFile(context.Background(), state, repo, upload)
	assert.ErrorIs(t, err, ErrGPGSignatureSuffix)
	assert.NoFileExists(t, signaturePath)
}

func TestPreparedUploadRejectsConcurrentRepositoryEngineChange(t *testing.T) {
	state, _, repo, storagePath := setupGPGUploadState(t)
	originalRepo := repo.DeepCopy()
	updatedConfig := state.Inner.Config.Load().DeepCopy()
	filesRepo := repo.DeepCopy()
	filesRepo.Format = config.RepositoryFormatFiles
	filesRepo.AllowRedeployment = true
	filesRepo.RequireGPGSignature = false
	updatedConfig.Maven.Repositories[repo.Name] = filesRepo
	state.Inner.Config.Store(updatedConfig)

	target := filepath.Join(storagePath, repo.Name, "com", "example", "demo", "1.0", "demo-1.0.jar")
	upload := preparedTestUpload(t, target, "alice", []byte("stale upload"))
	_, err := ProcessUploadedFile(context.Background(), state, originalRepo, upload)
	assert.ErrorIs(t, err, ErrRepositoryFormatChanged)
	assert.NoFileExists(t, target)
}
