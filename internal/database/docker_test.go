/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package database_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
	"renop/internal/testutil"
)

func newTestDockerDB(t *testing.T) *database.DB {
	t.Helper()
	dir := testutil.TempDir(t)
	db, err := database.InitDB(config.DatabaseConfig{
		Driver: "sqlite",
		Dsn:    filepath.Join(dir, "docker_test.db"),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}

func TestDockerListsHydrateMetadataInBatches(t *testing.T) {
	db := newTestDockerDB(t)
	for _, imageName := range []string{"alpha", "bravo"} {
		_, err := db.CreateDockerImage("docker-local", imageName, "admin", false, 1_700_000_000_000)
		require.NoError(t, err)
	}
	alphaManifest := &core.DockerManifest{
		Repository: "docker-local", ImageName: "alpha",
		Digest:    "sha256:1111111111111111111111111111111111111111111111111111111111111111",
		MediaType: "application/vnd.docker.distribution.manifest.v2+json", RawJSON: []byte(`{"schemaVersion":2}`),
	}
	require.NoError(t, db.PutDockerManifest(alphaManifest, "v1", "admin"))
	alphaManifest.Digest = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
	require.NoError(t, db.PutDockerManifest(alphaManifest, "v2", "admin"))
	bravoManifest := &core.DockerManifest{
		Repository: "docker-local", ImageName: "bravo",
		Digest:    "sha256:3333333333333333333333333333333333333333333333333333333333333333",
		MediaType: "application/vnd.docker.distribution.manifest.v2+json", RawJSON: []byte(`{"schemaVersion":2}`),
	}
	require.NoError(t, db.PutDockerManifest(bravoManifest, "stable", "admin"))
	_, err := db.Exec(`UPDATE docker_tags SET updated_at = CASE tag WHEN 'v2' THEN 2 ELSE 1 END
		WHERE repository = 'docker-local' AND image_name = 'alpha'`)
	require.NoError(t, err)
	_, err = db.Exec(`UPDATE docker_images SET publisher = '' WHERE repository = 'docker-local'`)
	require.NoError(t, err)
	_, err = db.Exec(`DELETE FROM docker_members WHERE repository = 'docker-local'`)
	require.NoError(t, err)
	_, err = db.Exec(`DELETE FROM docker_tags WHERE repository = 'docker-local' AND image_name = 'bravo'`)
	require.NoError(t, err)

	assertMetadata := func(images []*core.DockerRepositoryImage) {
		t.Helper()
		require.Len(t, images, 2)
		require.Equal(t, "alpha", images[0].ImageName)
		require.Equal(t, 2, images[0].TagCount)
		require.Equal(t, "v2", images[0].LatestTag)
		require.Equal(t, "admin", images[0].Publisher)
		require.Equal(t, "bravo", images[1].ImageName)
		require.Zero(t, images[1].TagCount)
		require.Empty(t, images[1].LatestTag)
		require.Equal(t, "admin", images[1].Publisher)
	}
	images, err := db.ListDockerImages("docker-local", "", 10)
	require.NoError(t, err)
	assertMetadata(images)
	images, total, err := db.SearchDockerImages("docker-local", "a", 10, 0)
	require.NoError(t, err)
	require.Equal(t, 2, total)
	assertMetadata(images)
}

func TestDockerDatabaseOperations(t *testing.T) {
	db := newTestDockerDB(t)
	_, err := db.CreateDockerImage("docker-local", "ubuntu", "admin", false, 1_700_000_000_000)
	require.NoError(t, err)

	manifest := &core.DockerManifest{
		Repository:   "docker-local",
		ImageName:    "ubuntu",
		Digest:       "sha256:abc1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
		MediaType:    "application/vnd.docker.distribution.manifest.v2+json",
		Size:         1024,
		ConfigDigest: "sha256:cfg1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
		RawJSON:      []byte(`{"schemaVersion":2}`),
	}

	err = db.PutDockerManifest(manifest, "latest", "admin")
	if err != nil {
		t.Fatalf("PutDockerManifest failed: %v", err)
	}

	err = db.PutDockerManifest(manifest, "22.04", "admin")
	if err != nil {
		t.Fatalf("PutDockerManifest with tag 22.04 failed: %v", err)
	}

	img, err := db.GetDockerImage("docker-local", "ubuntu")
	if err != nil {
		t.Fatalf("GetDockerImage failed: %v", err)
	}
	if img == nil {
		t.Fatal("expected image to be found")
	}
	if img.TagCount != 2 {
		t.Fatalf("expected 2 tags, got %d", img.TagCount)
	}
	if img.Publisher != "admin" {
		t.Fatalf("expected image publisher to be 'admin', got '%s'", img.Publisher)
	}

	tag, err := db.GetDockerTag("docker-local", "ubuntu", "latest")
	if err != nil {
		t.Fatalf("GetDockerTag failed: %v", err)
	}
	if tag == nil || tag.Digest != manifest.Digest {
		t.Fatalf("unexpected tag result: %+v", tag)
	}
	if tag.Publisher != "admin" {
		t.Fatalf("expected tag publisher to be 'admin', got '%s'", tag.Publisher)
	}

	tags, err := db.ListDockerTags("docker-local", "ubuntu", "", 50)
	if err != nil {
		t.Fatalf("ListDockerTags failed: %v", err)
	}
	if len(tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(tags))
	}

	storedManifest, err := db.GetDockerManifest("docker-local", "ubuntu", manifest.Digest)
	if err != nil {
		t.Fatalf("GetDockerManifest failed: %v", err)
	}
	if storedManifest == nil || storedManifest.Size != 1024 {
		t.Fatalf("unexpected manifest result: %+v", storedManifest)
	}
	if storedManifest.Publisher != "admin" {
		t.Fatalf("expected manifest publisher to be 'admin', got '%s'", storedManifest.Publisher)
	}

	details, err := db.GetDockerImageDetails("docker-local", "ubuntu")
	if err != nil || details == nil {
		t.Fatalf("GetDockerImageDetails failed: %v", err)
	}
	if len(details.Tags) != 2 || details.Image.Publisher != "admin" {
		t.Fatalf("unexpected details: %+v", details)
	}

	blobDigest := "sha256:blob1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
	if err := db.RecordDockerBlob("docker-local", blobDigest, 5000); err != nil {
		t.Fatalf("RecordDockerBlob failed: %v", err)
	}
	exists, size, err := db.HasDockerBlob("docker-local", blobDigest)
	if err != nil || !exists || size != 5000 {
		t.Fatalf("HasDockerBlob failed: exists=%v, size=%d, err=%v", exists, size, err)
	}

	results, total, err := db.SearchDockerImages("docker-local", "ubun", 10, 0)
	if err != nil {
		t.Fatalf("SearchDockerImages failed: %v", err)
	}
	if total != 1 || len(results) != 1 || results[0].ImageName != "ubuntu" {
		t.Fatalf("unexpected search results: total=%d, len=%d", total, len(results))
	}

	totalImages, totalTags, totalSize, err := db.GetDockerRepositoryStats("docker-local")
	if err != nil {
		t.Fatalf("GetDockerRepositoryStats failed: %v", err)
	}
	if totalImages != 1 || totalTags != 2 || totalSize != 5000 {
		t.Fatalf("unexpected stats: images=%d, tags=%d, size=%d", totalImages, totalTags, totalSize)
	}

	if err := db.DeleteDockerTag("docker-local", "ubuntu", "22.04"); err != nil {
		t.Fatalf("DeleteDockerTag failed: %v", err)
	}
	deletedTag, err := db.GetDockerTag("docker-local", "ubuntu", "22.04")
	if err != nil || deletedTag != nil {
		t.Fatalf("expected tag to be deleted, got: %v", deletedTag)
	}

	if err := db.IncrementDockerPullCount("docker-local", "ubuntu"); err != nil {
		t.Fatalf("IncrementDockerPullCount failed: %v", err)
	}
	imgAfterPull, err := db.GetDockerImage("docker-local", "ubuntu")
	if err != nil || imgAfterPull == nil || imgAfterPull.PullCount != 1 {
		t.Fatalf("expected pull count 1, got %+v", imgAfterPull)
	}

	invitationID := "inv-docker-001"
	now := int64(1700000000000)
	invMsg := &core.UserMessage{
		ID:           invitationID,
		Recipient:    "bob",
		Sender:       "admin",
		Kind:         "docker_image_invite",
		Severity:     "info",
		Title:        "Docker Invitation",
		Body:         "admin invited you to collaborate on ubuntu",
		Payload:      []byte(`{"repository":"docker-local","image":"ubuntu","inviter":"admin","level":0}`),
		ActionKind:   "docker_image_invite",
		ActionStatus: core.MessageActionPending,
		CreatedAt:    now,
		ExpiresAt:    now + 86400000,
	}
	inv := &core.DockerInvitation{
		ID:         invitationID,
		Repository: "docker-local",
		ImageName:  "ubuntu",
		Inviter:    "admin",
		Recipient:  "bob",
		Level:      0,
		CreatedAt:  now,
	}

	if err := db.CreateDockerInvitations([]*core.DockerInvitation{inv}, []*core.UserMessage{invMsg}); err != nil {
		t.Fatalf("CreateDockerInvitations failed: %v", err)
	}

	if err := db.RespondDockerInvitation(invitationID, "bob", "docker-local", true, now+1000); err != nil {
		t.Fatalf("RespondDockerInvitation failed: %v", err)
	}

	bobLevel, err := db.GetDockerMemberLevel("docker-local", "ubuntu", "bob")
	if err != nil || bobLevel != 0 {
		t.Fatalf("expected bob level 0, got %d (err: %v)", bobLevel, err)
	}
	exists, private, pushEnabled, member, accessLevel, err := db.GetDockerImageAccess("docker-local", "ubuntu", "bob")
	require.NoError(t, err)
	require.True(t, exists)
	require.False(t, private)
	require.True(t, pushEnabled)
	require.True(t, member)
	require.Equal(t, core.DockerPermissionRead, accessLevel)

	if err := db.SetDockerMemberLevel("docker-local", "ubuntu", "admin", "bob", 1); err != nil {
		t.Fatalf("SetDockerMemberLevel failed: %v", err)
	}
	bobLevel, _ = db.GetDockerMemberLevel("docker-local", "ubuntu", "bob")
	if bobLevel != 1 {
		t.Fatalf("expected bob level 1, got %d", bobLevel)
	}

	if err := db.RemoveDockerMember("docker-local", "ubuntu", "admin", "bob"); err != nil {
		t.Fatalf("RemoveDockerMember failed: %v", err)
	}
	requireTeamRemovalMessage(t, db, "bob", "docker", "docker-local", "ubuntu", "admin")
	bobLevel, _ = db.GetDockerMemberLevel("docker-local", "ubuntu", "bob")
	if bobLevel != 0 {
		t.Fatalf("expected bob level 0, got %d", bobLevel)
	}
	_, _, _, member, _, err = db.GetDockerImageAccess("docker-local", "ubuntu", "bob")
	require.NoError(t, err)
	require.False(t, member)

	if err := db.DeleteDockerManifest("docker-local", "ubuntu", manifest.Digest); err != nil {
		t.Fatalf("DeleteDockerManifest failed: %v", err)
	}

	if err := db.DeleteDockerRepository("docker-local"); err != nil {
		t.Fatalf("DeleteDockerRepository failed: %v", err)
	}
}

func TestDockerManifestRequiresPrecreatedImageAndMirrorCanImport(t *testing.T) {
	db := newTestDockerDB(t)
	manifest := &core.DockerManifest{
		Repository: "docker-local", ImageName: "manual/app",
		Digest:    "sha256:abc1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdff",
		MediaType: "application/vnd.oci.image.manifest.v1+json", RawJSON: []byte(`{"schemaVersion":2}`),
		BlobDigests: []string{"sha256:bbbb234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"},
	}
	require.ErrorIs(t, db.PutDockerManifest(manifest, "latest", "alice"), core.ErrDockerImageNotFound)
	created, err := db.CreateDockerImage("docker-local", "manual/app", "alice", true, 1_700_000_000_000)
	require.NoError(t, err)
	require.True(t, created.Private)
	_, err = db.CreateDockerImage("docker-local", "manual/app", "alice", false, 1_700_000_000_001)
	require.ErrorIs(t, err, core.ErrDockerImageExists)
	require.NoError(t, db.PutDockerManifest(manifest, "latest", "alice"))
	require.ErrorIs(t, db.CacheDockerManifest(manifest, "upstream"), core.ErrDockerImageExists)
	referenced, err := db.DockerImageReferencesBlob("docker-local", "manual/app", manifest.BlobDigests[0])
	require.NoError(t, err)
	require.True(t, referenced)

	mirrorManifest := *manifest
	mirrorManifest.ImageName = "upstream/app"
	mirrorManifest.Digest = "sha256:abc1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdfe"
	require.NoError(t, db.CacheDockerManifest(&mirrorManifest, "latest"))
	mirrored, err := db.GetDockerImage("docker-local", "upstream/app")
	require.NoError(t, err)
	require.NotNil(t, mirrored)
	require.False(t, mirrored.Private)
	require.True(t, mirrored.Mirrored)
	require.False(t, created.Mirrored)
	_, _, pushEnabled, member, _, err := db.GetDockerImageAccess("docker-local", "upstream/app", "mirror")
	require.NoError(t, err)
	require.False(t, pushEnabled)
	require.False(t, member)
	_, err = db.CreateDockerImage("docker-local", "upstream/app", "alice", true, 1_700_000_000_002)
	require.ErrorIs(t, err, core.ErrDockerImageExists)
}

func TestDockerTeamTransferPreservesRolesAndRejectsForceOverwrite(t *testing.T) {
	db := newTestDockerDB(t)
	_, err := db.CreateDockerImage("docker-local", "team/demo", "alice", false, 1_700_000_000_000)
	require.NoError(t, err)
	manifest := &core.DockerManifest{
		Repository: "docker-local",
		ImageName:  "team/demo",
		Digest:     "sha256:abc1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdea",
		MediaType:  "application/vnd.oci.image.manifest.v1+json",
		RawJSON:    []byte(`{"schemaVersion":2}`),
	}
	require.NoError(t, db.PutDockerManifest(manifest, "latest", "alice"))
	require.NoError(t, db.ForceAddDockerMembers("docker-local", "team/demo", "administrator", []string{"bob"}, core.DockerPermissionManage))
	require.ErrorIs(t,
		db.ForceAddDockerMembers("docker-local", "team/demo", "administrator", []string{"bob"}, core.DockerPermissionTeam),
		core.ErrDockerMemberExists,
	)

	bobLevel, err := db.GetDockerMemberLevel("docker-local", "team/demo", "bob")
	require.NoError(t, err)
	require.Equal(t, core.DockerPermissionManage, bobLevel)
	require.ErrorIs(t,
		db.SetDockerMemberLevel("docker-local", "team/demo", "bob", "bob", core.DockerPermissionOwner),
		core.ErrDockerPermissionDenied,
	)

	require.NoError(t, db.SetDockerMemberLevel("docker-local", "team/demo", "alice", "bob", core.DockerPermissionOwner))
	aliceLevel, err := db.GetDockerMemberLevel("docker-local", "team/demo", "alice")
	require.NoError(t, err)
	require.Equal(t, core.DockerPermissionManage, aliceLevel)
	members, err := db.ListDockerMembers("docker-local", "team/demo")
	require.NoError(t, err)
	ownerCount := 0
	for _, member := range members {
		if member.Level == core.DockerPermissionOwner {
			ownerCount++
			require.Equal(t, "bob", member.Username)
		}
	}
	require.Equal(t, 1, ownerCount)
	require.ErrorIs(t,
		db.RemoveDockerMember("docker-local", "team/demo", "bob", "bob"),
		core.ErrDockerOwnerCannotLeave,
	)
	require.NoError(t, db.RemoveDockerMember("docker-local", "team/demo", "alice", "alice"))
	requireNoTeamRemovalMessage(t, db, "alice")
}
