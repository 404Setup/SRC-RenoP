/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package core

// MaxDownloadStatisticsOffset bounds offset pagination work for statistics queries.
const MaxDownloadStatisticsOffset = 1_000_000

// DownloadStatisticDelta is one aggregated increment waiting for persistence.
type DownloadStatisticDelta struct {
	Username   string `json:"username,omitempty"`
	Repository string `json:"repository"`
	Format     string `json:"format"`
	Namespace  string `json:"namespace,omitempty"`
	Package    string `json:"package,omitempty"`
	Version    string `json:"version,omitempty"`
	Count      int64  `json:"count"`
	Bytes      int64  `json:"bytes"`
	UpdatedAt  int64  `json:"updated_at"`
}

// DownloadStatisticRecord is one grouped statistics query result.
type DownloadStatisticRecord struct {
	Username   string `json:"username,omitempty"`
	Repository string `json:"repository,omitempty"`
	Format     string `json:"format,omitempty"`
	Namespace  string `json:"namespace,omitempty"`
	Package    string `json:"package,omitempty"`
	Version    string `json:"version,omitempty"`
	Count      int64  `json:"count"`
	Bytes      int64  `json:"bytes"`
	UpdatedAt  int64  `json:"updated_at"`
}

// DownloadStatisticsQuery defines exact filters and one bounded grouping level.
type DownloadStatisticsQuery struct {
	UserID     string
	Username   string
	Repository string
	Format     string
	Namespace  string
	Package    string
	Version    string
	GroupBy    string
	Limit      int
	Offset     int
}

// DownloadStatisticsPage is a paginated aggregate response.
type DownloadStatisticsPage struct {
	GroupBy string                     `json:"group_by"`
	Count   int64                      `json:"count"`
	Bytes   int64                      `json:"bytes"`
	Records []*DownloadStatisticRecord `json:"records"`
	Total   int                        `json:"total"`
	Limit   int                        `json:"limit"`
	Offset  int                        `json:"offset"`
}

// DownloadStatisticsCounter batches successful package-download statistics.
type DownloadStatisticsCounter interface {
	Record(event DownloadStatisticDelta)
	Flush() error
	ResetRepository(repository string) error
}
