/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
)

func (tx *clickHouseTransaction) ensureSnapshot(table, query string, args []any) error {
	if !validClickHouseIdentifier(table) {
		return fmt.Errorf("invalid ClickHouse transaction table %q", table)
	}
	snapshot, exists := tx.snapshots[table]
	if !exists {
		snapshot = fmt.Sprintf("_renop_tx_%s_%d", tx.id, len(tx.snapshots))
		if err := tx.backend.conn.Exec(context.Background(), fmt.Sprintf(
			"CREATE TABLE `%s` AS `%s` ENGINE = MergeTree ORDER BY tuple()", snapshot, table)); err != nil {
			return fmt.Errorf("create ClickHouse transaction snapshot for %s: %w", table, err)
		}
		if err := tx.backend.conn.Exec(context.Background(), `INSERT INTO `+clickHouseTransactionJournal+
			` (transaction_id, table_name, snapshot_name, created_at) VALUES (?, ?, ?, ?)`,
			tx.id, table, snapshot, time.Now().UnixMilli()); err != nil {
			_ = tx.backend.conn.Exec(context.Background(), "DROP TABLE IF EXISTS `"+snapshot+"`")
			return fmt.Errorf("record ClickHouse transaction snapshot for %s: %w", table, err)
		}
		tx.snapshots[table] = snapshot
	}
	keys, err := tx.mutationKeys(table, query, args)
	if err != nil {
		return err
	}
	for _, key := range keys {
		var recorded uint64
		if err := tx.backend.conn.QueryRow(context.Background(), `SELECT COUNT(*) FROM `+clickHouseTransactionKeys+
			` WHERE transaction_id = ? AND table_name = ? AND row_key = ?`, tx.id, table, key).Scan(&recorded); err != nil {
			return fmt.Errorf("inspect ClickHouse transaction key for %s: %w", table, err)
		}
		if recorded != 0 {
			continue
		}
		if err := tx.backend.conn.Exec(context.Background(), fmt.Sprintf(
			"INSERT INTO `%s` SELECT * FROM `%s` WHERE _renop_key = ?", snapshot, table), key); err != nil {
			return fmt.Errorf("snapshot ClickHouse row in %s: %w", table, err)
		}
		if err := tx.backend.conn.Exec(context.Background(), `INSERT INTO `+clickHouseTransactionKeys+
			` (transaction_id, table_name, row_key, created_at) VALUES (?, ?, ?, ?)`,
			tx.id, table, key, time.Now().UnixMilli()); err != nil {
			return fmt.Errorf("record ClickHouse transaction key for %s: %w", table, err)
		}
	}
	return nil
}

func (tx *clickHouseTransaction) commit() error {
	if tx.closed {
		return sql.ErrTxDone
	}
	defer tx.finish()
	for table, snapshot := range tx.snapshots {
		if err := tx.backend.conn.Exec(context.Background(), `DELETE FROM `+clickHouseTransactionJournal+
			` WHERE transaction_id = ? AND table_name = ?`, tx.id, table); err != nil {
			return errors.Join(err, tx.restoreSnapshots())
		}
		if err := tx.backend.conn.Exec(context.Background(), `DELETE FROM `+clickHouseTransactionKeys+
			` WHERE transaction_id = ? AND table_name = ?`, tx.id, table); err != nil {
			log.Printf("Failed to clean committed ClickHouse transaction keys for %s: %v", table, err)
		}
		if err := tx.backend.conn.Exec(context.Background(), "DROP TABLE IF EXISTS `"+snapshot+"`"); err != nil {
			log.Printf("Failed to remove committed ClickHouse transaction snapshot %s: %v", snapshot, err)
		}
	}
	return nil
}

func (tx *clickHouseTransaction) rollback() error {
	if tx.closed {
		return sql.ErrTxDone
	}
	defer tx.finish()
	return tx.restoreSnapshots()
}

func (tx *clickHouseTransaction) restoreSnapshots() error {
	var restoreErr error
	for table, snapshot := range tx.snapshots {
		if err := tx.backend.restoreSnapshot(context.Background(), tx.id, table, snapshot); err != nil {
			restoreErr = errors.Join(restoreErr, err)
		}
	}
	return restoreErr
}

func (tx *clickHouseTransaction) finish() {
	tx.closed = true
	tx.backend.lock.Unlock()
}

func (backend *clickHouseBackend) restoreSnapshot(ctx context.Context, transactionID, table, snapshot string) error {
	if !validClickHouseIdentifier(table) || !validClickHouseIdentifier(snapshot) {
		return fmt.Errorf("invalid ClickHouse transaction journal entry")
	}
	var exists uint8
	if err := backend.conn.QueryRow(ctx, `SELECT count() > 0 FROM system.tables WHERE database = ? AND name = ?`,
		backend.database, snapshot).Scan(&exists); err != nil {
		return err
	}
	if exists != 0 {
		if err := backend.conn.Exec(ctx, `DELETE FROM `+table+` WHERE _renop_key IN (
			SELECT row_key FROM `+clickHouseTransactionKeys+` WHERE transaction_id = ? AND table_name = ?)`,
			transactionID, table); err != nil {
			return fmt.Errorf("remove changed ClickHouse rows from %s during rollback: %w", table, err)
		}
		if err := backend.conn.Exec(ctx, fmt.Sprintf("INSERT INTO `%s` SELECT * FROM `%s`", table, snapshot)); err != nil {
			return fmt.Errorf("restore ClickHouse table %s: %w", table, err)
		}
	}
	if err := backend.conn.Exec(ctx, `DELETE FROM `+clickHouseTransactionKeys+
		` WHERE transaction_id = ? AND table_name = ?`, transactionID, table); err != nil {
		return err
	}
	if err := backend.conn.Exec(ctx, `DELETE FROM `+clickHouseTransactionJournal+
		` WHERE transaction_id = ? AND table_name = ?`, transactionID, table); err != nil {
		return err
	}
	if exists != 0 {
		if err := backend.conn.Exec(ctx, "DROP TABLE IF EXISTS `"+snapshot+"`"); err != nil {
			return fmt.Errorf("remove restored ClickHouse transaction snapshot %s: %w", snapshot, err)
		}
	}
	return nil
}

func (backend *clickHouseBackend) recoverTransactions(ctx context.Context) error {
	rows, err := backend.conn.Query(ctx, `SELECT transaction_id, table_name, snapshot_name FROM `+clickHouseTransactionJournal)
	if err != nil {
		return err
	}
	entries := make([][3]string, 0)
	for rows.Next() {
		var entry [3]string
		if err := rows.Scan(&entry[0], &entry[1], &entry[2]); err != nil {
			_ = rows.Close()
			return err
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	activeSnapshots := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		activeSnapshots[entry[2]] = struct{}{}
		if err := backend.restoreSnapshot(ctx, entry[0], entry[1], entry[2]); err != nil {
			return err
		}
	}
	orphans, err := backend.conn.Query(ctx, `SELECT name FROM system.tables WHERE database = ? AND startsWith(name, '_renop_tx_')`, backend.database)
	if err != nil {
		return err
	}
	for orphans.Next() {
		var name string
		if err := orphans.Scan(&name); err != nil {
			_ = orphans.Close()
			return err
		}
		if _, active := activeSnapshots[name]; !active && validClickHouseIdentifier(name) {
			if err := backend.conn.Exec(ctx, "DROP TABLE IF EXISTS `"+name+"`"); err != nil {
				_ = orphans.Close()
				return fmt.Errorf("remove orphaned ClickHouse transaction snapshot %s: %w", name, err)
			}
		}
	}
	if err := orphans.Err(); err != nil {
		_ = orphans.Close()
		return err
	}
	if err := orphans.Close(); err != nil {
		return err
	}
	if err := backend.conn.Exec(ctx, `DELETE FROM `+clickHouseTransactionKeys+` WHERE transaction_id NOT IN (
		SELECT transaction_id FROM `+clickHouseTransactionJournal+`)`); err != nil {
		return fmt.Errorf("remove orphaned ClickHouse transaction keys: %w", err)
	}
	return nil
}

func (backend *clickHouseBackend) countMutationRows(query string, args []any) (int64, bool, error) {
	table := clickHouseMutationTable(query)
	if table == "" {
		return 0, false, nil
	}
	upper := strings.ToUpper(stripLeadingSQLComments(query))
	if !strings.HasPrefix(upper, "ALTER TABLE ") && !strings.HasPrefix(upper, "DELETE FROM ") {
		return 0, false, nil
	}
	whereIndex := findTopLevelSQLKeyword(query, "WHERE")
	countQuery := "SELECT count() FROM `" + table + "`"
	whereArgs := args
	if whereIndex >= 0 {
		countQuery += " " + strings.TrimSpace(query[whereIndex:])
		whereArgs = args[min(countSQLPlaceholders(query[:whereIndex]), len(args)):]
	}
	var count uint64
	if err := backend.conn.QueryRow(context.Background(), countQuery, whereArgs...).Scan(&count); err != nil {
		return 0, false, err
	}
	if count > uint64(^uint64(0)>>1) {
		return 0, false, fmt.Errorf("ClickHouse affected-row count exceeds int64")
	}
	return int64(count), true, nil
}

func (tx *clickHouseTransaction) mutationKeys(table, query string, args []any) ([]string, error) {
	operation := strings.ToUpper(stripLeadingSQLComments(query))
	if strings.HasPrefix(operation, "INSERT ") {
		return clickHouseInsertKeys(table, query, normalizeClickHouseArguments(args))
	}
	whereIndex := findTopLevelSQLKeyword(query, "WHERE")
	keyQuery := "SELECT _renop_key FROM `" + table + "`"
	whereArgs := normalizeClickHouseArguments(args)
	if whereIndex >= 0 {
		keyQuery += " " + strings.TrimSpace(query[whereIndex:])
		whereArgs = whereArgs[min(countSQLPlaceholders(query[:whereIndex]), len(whereArgs)):]
	}
	rows, err := tx.backend.conn.Query(context.Background(), keyQuery, whereArgs...)
	if err != nil {
		return nil, fmt.Errorf("list ClickHouse transaction keys for %s: %w", table, err)
	}
	defer rows.Close()
	keys := make([]string, 0)
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return keys, nil
}
