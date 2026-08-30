/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package database

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"renop/internal/config"
	"renop/internal/core"
)

func downloadStatisticID(event *core.DownloadStatisticDelta, userID string) string {
	identity := userID
	if identity == "" {
		identity = "username:" + event.Username
	}
	sum := sha256.Sum256([]byte(strings.Join([]string{
		identity, event.Repository, event.Format, event.Namespace, event.Package, event.Version,
	}, "\x00")))
	return hex.EncodeToString(sum[:])
}

func resolveDownloadStatisticUserID(tx *Tx, username string) (string, error) {
	if username == "" || strings.EqualFold(username, "guest") {
		return "", nil
	}
	var userID string
	err := tx.QueryRow(`SELECT user_id FROM user_profiles WHERE username = ?`, username).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("resolve download-statistics user: %w", err)
	}
	return userID, nil
}

func resolveDownloadStatisticNamespace(tx *Tx, event *core.DownloadStatisticDelta) (string, error) {
	if event.Format != config.RepositoryFormatMaven || event.Package == "" {
		return event.Namespace, nil
	}
	separator := strings.LastIndexByte(event.Package, ':')
	if separator <= 0 || separator == len(event.Package)-1 {
		return event.Namespace, nil
	}
	groupID, artifactID := event.Package[:separator], event.Package[separator+1:]
	var domain string
	err := tx.QueryRow(`SELECT domain FROM maven_artifacts
		WHERE repository = ? AND group_id = ? AND artifact_id = ?`,
		event.Repository, groupID, artifactID).Scan(&domain)
	if errors.Is(err, sql.ErrNoRows) {
		return event.Namespace, nil
	}
	if err != nil {
		return "", fmt.Errorf("resolve download-statistics Maven domain: %w", err)
	}
	return domain, nil
}

func incrementDownloadStatistic(tx *Tx, id, userID string, event *core.DownloadStatisticDelta) error {
	insertArgs := []any{
		id, userID, event.Username, event.Repository, event.Format, event.Namespace, event.Package,
		event.Version, event.Count, event.Bytes, event.UpdatedAt,
	}
	switch tx.db.Dialect.Name() {
	case "sqlite", "postgres":
		_, err := tx.Exec(`INSERT INTO download_statistics
			(id, user_id, username, repository, format, namespace, package_name, version,
			download_count, download_bytes, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET username = excluded.username,
			download_count = download_statistics.download_count + excluded.download_count,
			download_bytes = download_statistics.download_bytes + excluded.download_bytes,
			updated_at = CASE WHEN excluded.updated_at > download_statistics.updated_at
				THEN excluded.updated_at ELSE download_statistics.updated_at END`, insertArgs...)
		return err
	case "mysql":
		_, err := tx.Exec(`INSERT INTO download_statistics
			(id, user_id, username, repository, format, namespace, package_name, version,
			download_count, download_bytes, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE username = VALUES(username),
			download_count = download_count + VALUES(download_count),
			download_bytes = download_bytes + VALUES(download_bytes),
			updated_at = CASE WHEN VALUES(updated_at) > updated_at THEN VALUES(updated_at) ELSE updated_at END`,
			insertArgs...)
		return err
	}

	update := func() (int64, error) {
		result, err := tx.Exec(`UPDATE download_statistics SET username = ?,
			download_count = download_count + ?, download_bytes = download_bytes + ?,
			updated_at = CASE WHEN ? > updated_at THEN ? ELSE updated_at END WHERE id = ?`,
			event.Username, event.Count, event.Bytes, event.UpdatedAt, event.UpdatedAt, id)
		if err != nil {
			return 0, err
		}
		return result.RowsAffected()
	}
	changed, err := update()
	if err != nil {
		return err
	}
	if changed > 0 {
		return nil
	}
	_, insertErr := tx.Exec(`INSERT INTO download_statistics
		(id, user_id, username, repository, format, namespace, package_name, version,
		download_count, download_bytes, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		insertArgs...)
	if insertErr == nil {
		return nil
	}
	changed, retryErr := update()
	if retryErr == nil && changed > 0 {
		return nil
	}
	return errors.Join(insertErr, retryErr)
}

// BatchIncrementDownloadStatistics persists a bounded in-memory counter batch atomically.
func (db *DB) BatchIncrementDownloadStatistics(events []*core.DownloadStatisticDelta) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	if len(events) == 0 {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin download-statistics batch: %w", err)
	}
	defer tx.Rollback()
	userIDs := make(map[string]string)
	namespaces := make(map[string]string)
	for _, event := range events {
		if event == nil || event.Count <= 0 || event.Repository == "" || event.Format == "" {
			continue
		}
		userID, known := userIDs[event.Username]
		if !known {
			userID, err = resolveDownloadStatisticUserID(tx, event.Username)
			if err != nil {
				return err
			}
			userIDs[event.Username] = userID
		}
		if event.Format == config.RepositoryFormatMaven && event.Package != "" {
			namespaceKey := event.Repository + "\x00" + event.Package
			resolved, known := namespaces[namespaceKey]
			if !known {
				resolved, err = resolveDownloadStatisticNamespace(tx, event)
				if err != nil {
					return err
				}
				namespaces[namespaceKey] = resolved
			}
			event.Namespace = resolved
		}
		if err := incrementDownloadStatistic(tx, downloadStatisticID(event, userID), userID, event); err != nil {
			return fmt.Errorf("increment download statistics for %s: %w", event.Repository, err)
		}
		if event.Format == config.RepositoryFormatDocker && event.Package != "" {
			if _, err := tx.Exec(`UPDATE docker_images SET pull_count = pull_count + ?
				WHERE repository = ? AND image_name = ?`, event.Count, event.Repository, event.Package); err != nil {
				return fmt.Errorf("increment Docker pull count: %w", err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit download-statistics batch: %w", err)
	}
	return nil
}

// ResetDownloadStatistics deletes persisted counters for one repository.
func (db *DB) ResetDownloadStatistics(repository string) error {
	if db == nil || db.SQLDB == nil {
		return core.ErrDatabaseUnavailable
	}
	repository = strings.ToLower(strings.TrimSpace(repository))
	if repository == "" {
		return errors.New("download-statistics repository is required")
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin download-statistics reset: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM download_statistics WHERE repository = ?`, repository); err != nil {
		return fmt.Errorf("reset repository download statistics: %w", err)
	}
	if _, err := tx.Exec(`UPDATE docker_images SET pull_count = 0 WHERE repository = ?`, repository); err != nil {
		return fmt.Errorf("reset Docker pull counts: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit download-statistics reset: %w", err)
	}
	return nil
}

func downloadStatisticsFilters(query core.DownloadStatisticsQuery) (string, []any) {
	conditions := make([]string, 0, 7)
	args := make([]any, 0, 7)
	for _, filter := range []struct {
		column string
		value  string
	}{
		{column: "user_id", value: query.UserID},
		{column: "username", value: query.Username},
		{column: "repository", value: query.Repository},
		{column: "format", value: query.Format},
		{column: "namespace", value: query.Namespace},
		{column: "package_name", value: query.Package},
		{column: "version", value: query.Version},
	} {
		if filter.value == "" {
			continue
		}
		conditions = append(conditions, filter.column+" = ?")
		args = append(args, filter.value)
	}
	if len(conditions) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conditions, " AND "), args
}

type downloadStatisticsGrouping struct {
	selectColumns string
	groupColumns  string
	requireColumn string
}

func downloadStatisticsGroup(groupBy string) (downloadStatisticsGrouping, bool) {
	switch groupBy {
	case "user":
		return downloadStatisticsGrouping{
			selectColumns: "MAX(username)", groupColumns: "user_id", requireColumn: "user_id",
		}, true
	case "repository":
		return downloadStatisticsGrouping{
			selectColumns: "repository, format", groupColumns: "repository, format",
		}, true
	case "namespace":
		return downloadStatisticsGrouping{
			selectColumns: "repository, format, namespace", groupColumns: "repository, format, namespace",
			requireColumn: "namespace",
		}, true
	case "package":
		return downloadStatisticsGrouping{
			selectColumns: "repository, format, namespace, package_name",
			groupColumns:  "repository, format, namespace, package_name", requireColumn: "package_name",
		}, true
	case "version":
		return downloadStatisticsGrouping{
			selectColumns: "repository, format, namespace, package_name, version",
			groupColumns:  "repository, format, namespace, package_name, version", requireColumn: "version",
		}, true
	default:
		return downloadStatisticsGrouping{}, false
	}
}

func scanDownloadStatistic(rows rows, groupBy string) (*core.DownloadStatisticRecord, error) {
	record := &core.DownloadStatisticRecord{}
	var err error
	switch groupBy {
	case "user":
		err = rows.Scan(&record.Username, &record.Count, &record.Bytes, &record.UpdatedAt)
	case "repository":
		err = rows.Scan(&record.Repository, &record.Format, &record.Count, &record.Bytes, &record.UpdatedAt)
	case "namespace":
		err = rows.Scan(&record.Repository, &record.Format, &record.Namespace,
			&record.Count, &record.Bytes, &record.UpdatedAt)
	case "package":
		err = rows.Scan(&record.Repository, &record.Format, &record.Namespace, &record.Package,
			&record.Count, &record.Bytes, &record.UpdatedAt)
	case "version":
		err = rows.Scan(&record.Repository, &record.Format, &record.Namespace, &record.Package, &record.Version,
			&record.Count, &record.Bytes, &record.UpdatedAt)
	}
	return record, err
}

// QueryDownloadStatistics returns one bounded hierarchical aggregate page.
func (db *DB) QueryDownloadStatistics(query core.DownloadStatisticsQuery) (*core.DownloadStatisticsPage, error) {
	if db == nil || db.SQLDB == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	grouping, ok := downloadStatisticsGroup(query.GroupBy)
	if !ok || query.Limit < 1 || query.Limit > 100 ||
		query.Offset < 0 || query.Offset > core.MaxDownloadStatisticsOffset {
		return nil, errors.New("download-statistics query is invalid")
	}
	where, args := downloadStatisticsFilters(query)
	groupWhere := where
	groupArgs := append([]any(nil), args...)
	if grouping.requireColumn != "" {
		if groupWhere == "" {
			groupWhere = " WHERE " + grouping.requireColumn + " <> ''"
		} else {
			groupWhere += " AND " + grouping.requireColumn + " <> ''"
		}
	}
	page := &core.DownloadStatisticsPage{
		GroupBy: query.GroupBy, Limit: query.Limit, Offset: query.Offset,
		Records: make([]*core.DownloadStatisticRecord, 0, query.Limit),
	}
	if err := db.QueryRow(`SELECT COALESCE(SUM(download_count), 0), COALESCE(SUM(download_bytes), 0)
		FROM download_statistics`+groupWhere, groupArgs...).Scan(&page.Count, &page.Bytes); err != nil {
		return nil, fmt.Errorf("sum download statistics: %w", err)
	}
	countQuery := `SELECT COUNT(*) FROM (SELECT ` + grouping.groupColumns + ` FROM download_statistics` +
		groupWhere + ` GROUP BY ` + grouping.groupColumns + `) grouped_statistics`
	if err := db.QueryRow(countQuery, groupArgs...).Scan(&page.Total); err != nil {
		return nil, fmt.Errorf("count grouped download statistics: %w", err)
	}
	rows, err := db.Query(`SELECT `+grouping.selectColumns+`,
		SUM(download_count), SUM(download_bytes), MAX(updated_at) FROM download_statistics`+groupWhere+
		` GROUP BY `+grouping.groupColumns+
		` ORDER BY SUM(download_count) DESC, SUM(download_bytes) DESC, `+grouping.groupColumns+` LIMIT ? OFFSET ?`,
		append(groupArgs, query.Limit, query.Offset)...)
	if err != nil {
		return nil, fmt.Errorf("query grouped download statistics: %w", err)
	}
	for rows.Next() {
		record, err := scanDownloadStatistic(rows, query.GroupBy)
		if err != nil {
			return nil, errors.Join(fmt.Errorf("scan grouped download statistics: %w", err), rows.Close())
		}
		page.Records = append(page.Records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, errors.Join(fmt.Errorf("iterate grouped download statistics: %w", err), rows.Close())
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close grouped download statistics: %w", err)
	}
	return page, nil
}
