/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package database_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
)

func TestDownloadStatisticsPersistStableUsersDomainsAndDockerPulls(t *testing.T) {
	db := newMavenDB(t)
	now := time.Now().UnixMilli()
	domain := &core.MavenDomain{
		Domain: "com.example", VerificationType: core.MavenVerificationDNS,
		VerificationHost: "example.com", VerificationCode: "code", CreatedAt: now,
	}
	require.NoError(t, db.CreateMavenDomain(domain, "alice"))
	require.NoError(t, db.MarkMavenDomainVerified(domain.Domain, domain.VerificationCode, now))
	require.NoError(t, db.RecordMavenPublication(&core.MavenArtifact{
		Repository: "releases", Domain: domain.Domain, GroupID: "com.example.tools", ArtifactID: "demo",
		LatestVersion: "1.0", CreatedAt: now, UpdatedAt: now,
	}, &core.MavenVersion{
		Repository: "releases", GroupID: "com.example.tools", ArtifactID: "demo", Version: "1.0", CreatedAt: now,
	}))
	_, err := db.CreateDockerImage("containers", "team/demo", "alice", false, now)
	require.NoError(t, err)

	require.NoError(t, db.BatchIncrementDownloadStatistics([]*core.DownloadStatisticDelta{
		{Username: "alice", Repository: "releases", Format: config.RepositoryFormatMaven,
			Namespace: "com.example.tools", Package: "com.example.tools:demo", Version: "1.0",
			Count: 2, Bytes: 2048, UpdatedAt: now},
		{Username: "alice", Repository: "containers", Format: config.RepositoryFormatDocker,
			Package: "team/demo", Version: "latest", Count: 3, Bytes: 4096, UpdatedAt: now},
	}))
	require.NoError(t, db.BatchIncrementDownloadStatistics([]*core.DownloadStatisticDelta{{
		Username: "alice", Repository: "releases", Format: config.RepositoryFormatMaven,
		Namespace: "com.example.tools", Package: "com.example.tools:demo", Version: "1.0",
		Count: 1, Bytes: 512, UpdatedAt: now + 1,
	}}))
	profile, err := db.GetUserProfile("alice")
	require.NoError(t, err)
	rows, err := db.Query(`SELECT user_id, repository, namespace, package_name, download_count, download_bytes
		FROM download_statistics ORDER BY repository`)
	require.NoError(t, err)
	type statistic struct {
		userID, repository, namespace, packageName string
		count, bytes                               int64
	}
	records := make([]statistic, 0, 2)
	for rows.Next() {
		var record statistic
		require.NoError(t, rows.Scan(&record.userID, &record.repository, &record.namespace,
			&record.packageName, &record.count, &record.bytes))
		records = append(records, record)
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	require.Len(t, records, 2)
	assert.Equal(t, profile.UserID, records[0].userID)
	assert.Equal(t, "containers", records[0].repository)
	assert.Equal(t, int64(3), records[0].count)
	assert.Equal(t, domain.Domain, records[1].namespace)
	assert.Equal(t, "com.example.tools:demo", records[1].packageName)
	assert.Equal(t, int64(3), records[1].count)
	assert.Equal(t, int64(2560), records[1].bytes)
	image, err := db.GetDockerImage("containers", "team/demo")
	require.NoError(t, err)
	assert.Equal(t, int64(3), image.PullCount)
	page, err := db.QueryDownloadStatistics(core.DownloadStatisticsQuery{
		UserID: profile.UserID, GroupBy: "repository", Limit: 20,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(6), page.Count)
	assert.Equal(t, int64(6656), page.Bytes)
	assert.Equal(t, 2, page.Total)
	require.Len(t, page.Records, 2)
	versionPage, err := db.QueryDownloadStatistics(core.DownloadStatisticsQuery{
		Repository: "releases", GroupBy: "version", Limit: 20,
	})
	require.NoError(t, err)
	require.Len(t, versionPage.Records, 1)
	assert.Equal(t, "1.0", versionPage.Records[0].Version)
	assert.Equal(t, domain.Domain, versionPage.Records[0].Namespace)

	require.NoError(t, db.ResetDownloadStatistics("containers"))
	image, err = db.GetDockerImage("containers", "team/demo")
	require.NoError(t, err)
	assert.Zero(t, image.PullCount)
	var remaining int
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM download_statistics WHERE repository = ?`, "containers").Scan(&remaining))
	assert.Zero(t, remaining)
}

func TestDownloadStatisticsPaginationUsesStableTieBreakers(t *testing.T) {
	db := newMavenDB(t)
	now := time.Now().UnixMilli()
	require.NoError(t, db.BatchIncrementDownloadStatistics([]*core.DownloadStatisticDelta{
		{Repository: "zeta", Format: config.RepositoryFormatFiles, Package: "zeta.zip", Count: 1, Bytes: 10, UpdatedAt: now},
		{Repository: "alpha", Format: config.RepositoryFormatFiles, Package: "alpha.zip", Count: 1, Bytes: 10, UpdatedAt: now},
	}))

	first, err := db.QueryDownloadStatistics(core.DownloadStatisticsQuery{GroupBy: "repository", Limit: 1})
	require.NoError(t, err)
	second, err := db.QueryDownloadStatistics(core.DownloadStatisticsQuery{GroupBy: "repository", Limit: 1, Offset: 1})
	require.NoError(t, err)
	require.Len(t, first.Records, 1)
	require.Len(t, second.Records, 1)
	assert.Equal(t, "alpha", first.Records[0].Repository)
	assert.Equal(t, "zeta", second.Records[0].Repository)
}

func TestPostgresDownloadStatisticsIntegration(t *testing.T) {
	dsn, admin, schema := newPostgresTestSchema(t, "renop_statistics_test")
	db, err := database.InitDB(config.DatabaseConfig{
		Driver: "postgres", Dsn: dsn, MaxOpenConns: 2, MaxIdleConns: 1,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	require.NoError(t, db.SaveToken(&core.AccessToken{Name: "statistics_pg", Permissions: []string{"base"}}))
	now := time.Now().UnixMilli()
	require.NoError(t, db.BatchIncrementDownloadStatistics([]*core.DownloadStatisticDelta{
		{Username: "statistics_pg", Repository: "releases", Format: config.RepositoryFormatMaven,
			Namespace: "org.example", Package: "org.example:demo", Version: "1.0.0",
			Count: 2, Bytes: 2048, UpdatedAt: now},
	}))
	require.NoError(t, db.BatchIncrementDownloadStatistics([]*core.DownloadStatisticDelta{{
		Username: "statistics_pg", Repository: "releases", Format: config.RepositoryFormatMaven,
		Namespace: "org.example", Package: "org.example:demo", Version: "1.0.0",
		Count: 1, Bytes: 512, UpdatedAt: now + 1,
	}}))
	profile, err := db.GetUserProfile("statistics_pg")
	require.NoError(t, err)
	page, err := db.QueryDownloadStatistics(core.DownloadStatisticsQuery{
		UserID: profile.UserID, GroupBy: "version", Limit: 10,
	})
	require.NoError(t, err)
	require.Len(t, page.Records, 1)
	assert.Equal(t, int64(3), page.Count)
	assert.Equal(t, int64(2560), page.Bytes)
	assert.Equal(t, "1.0.0", page.Records[0].Version)

	var indexCount int
	require.NoError(t, admin.QueryRow(`SELECT COUNT(*) FROM pg_indexes
		WHERE schemaname = $1 AND indexname LIKE 'idx_download_statistics_%'`, schema).Scan(&indexCount))
	assert.Equal(t, 4, indexCount)
	require.NoError(t, db.ResetDownloadStatistics("releases"))
}
