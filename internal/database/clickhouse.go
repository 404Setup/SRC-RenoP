/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	chdriver "github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/google/uuid"

	"renop/internal/config"
)

type clickHouseBackend struct {
	conn     chdriver.Conn
	database string
	lock     sync.RWMutex
}

type clickHouseTransaction struct {
	backend   *clickHouseBackend
	id        string
	snapshots map[string]string
	closed    bool
}

type clickHouseResult struct {
	affected int64
}

func (result clickHouseResult) LastInsertId() (int64, error) {
	return 0, nil
}

func (result clickHouseResult) RowsAffected() (int64, error) {
	return result.affected, nil
}

type clickHouseLockedRows struct {
	chdriver.Rows
	unlock sync.Once
	done   func()
}

func (rows *clickHouseLockedRows) Scan(dest ...any) error {
	return scanClickHouseValues(rows.Rows, dest...)
}

func (rows *clickHouseLockedRows) release() {
	rows.unlock.Do(rows.done)
}

func (rows *clickHouseLockedRows) Next() bool {
	next := rows.Rows.Next()
	if !next {
		rows.release()
	}
	return next
}

func (rows *clickHouseLockedRows) Close() error {
	err := rows.Rows.Close()
	rows.release()
	return err
}

type clickHouseLockedRow struct {
	rows   chdriver.Rows
	err    error
	unlock sync.Once
	done   func()
}

func (row *clickHouseLockedRow) Scan(dest ...any) error {
	defer row.unlock.Do(row.done)
	if row.err != nil {
		return row.err
	}
	defer row.rows.Close()
	if !row.rows.Next() {
		if err := row.rows.Err(); err != nil {
			return err
		}
		return sql.ErrNoRows
	}
	return scanClickHouseValues(row.rows, dest...)
}

func openClickHouse(cfg config.DatabaseConfig, dsn string) (*clickHouseBackend, error) {
	options, err := clickhouse.ParseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse ClickHouse DSN: %w", err)
	}
	if options.Auth.Database == "" {
		options.Auth.Database = "default"
	}
	if options.MaxOpenConns <= 0 {
		options.MaxOpenConns = max(cfg.MaxOpenConns, 4)
	}
	if options.MaxIdleConns <= 0 {
		options.MaxIdleConns = max(cfg.MaxIdleConns, 2)
	}
	if cfg.ConnMaxLifetimeSec > 0 {
		options.ConnMaxLifetime = time.Duration(cfg.ConnMaxLifetimeSec) * time.Second
	}
	if options.Settings == nil {
		options.Settings = clickhouse.Settings{}
	}
	options.Settings["mutations_sync"] = 2
	options.Settings["lightweight_deletes_sync"] = 2
	options.Settings["join_use_nulls"] = 1
	options.Settings["optimize_trivial_approximate_count_query"] = 0
	if options.Compression == nil {
		options.Compression = &clickhouse.Compression{Method: clickhouse.CompressionLZ4}
	}
	conn, err := clickhouse.Open(options)
	if err != nil {
		return nil, fmt.Errorf("open native ClickHouse connection: %w", err)
	}
	pingCtx, pingCancel := context.WithTimeout(context.Background(), 30*time.Second)
	if err := conn.Ping(pingCtx); err != nil {
		pingCancel()
		_ = conn.Close()
		return nil, fmt.Errorf("ping native ClickHouse connection: %w", err)
	}
	pingCancel()
	migrationCtx, migrationCancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer migrationCancel()
	if err := initializeClickHouseSchema(migrationCtx, conn, options.Auth.Database); err != nil {
		_ = conn.Close()
		return nil, err
	}
	backend := &clickHouseBackend{conn: conn, database: options.Auth.Database}
	if err := backend.recoverTransactions(migrationCtx); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("recover ClickHouse transaction journal: %w", err)
	}
	return backend, nil
}

func (backend *clickHouseBackend) close() error {
	backend.lock.Lock()
	defer backend.lock.Unlock()
	return backend.conn.Close()
}

func (backend *clickHouseBackend) exec(query string, args ...any) (result, error) {
	backend.lock.Lock()
	defer backend.lock.Unlock()
	return backend.execUnlocked(query, args...)
}

func (backend *clickHouseBackend) execUnlocked(query string, args ...any) (result, error) {
	args = normalizeClickHouseArguments(args)
	affected, affectedKnown, err := backend.countMutationRows(query, args)
	if err != nil {
		return nil, err
	}
	var wroteRows atomic.Uint64
	ctx := clickhouse.Context(context.Background(), clickhouse.WithProgress(func(progress *clickhouse.Progress) {
		wroteRows.Add(progress.WroteRows)
	}))
	if err := backend.conn.Exec(ctx, query, args...); err != nil {
		return nil, err
	}
	if affectedKnown {
		return clickHouseResult{affected: affected}, nil
	}
	written := int64(wroteRows.Load())
	if written == 0 && strings.HasPrefix(strings.ToUpper(stripLeadingSQLComments(query)), "INSERT ") &&
		!strings.Contains(query, "renop:ignore-if-exists") {
		written = 1
	}
	return clickHouseResult{affected: written}, nil
}

func (backend *clickHouseBackend) query(query string, args ...any) (rows, error) {
	backend.lock.RLock()
	result, err := backend.conn.Query(context.Background(), query, normalizeClickHouseArguments(args)...)
	if err != nil {
		backend.lock.RUnlock()
		return nil, err
	}
	return &clickHouseLockedRows{Rows: result, done: backend.lock.RUnlock}, nil
}

func (backend *clickHouseBackend) queryRow(query string, args ...any) row {
	backend.lock.RLock()
	result, err := backend.conn.Query(context.Background(), query, normalizeClickHouseArguments(args)...)
	return &clickHouseLockedRow{rows: result, err: err, done: backend.lock.RUnlock}
}

func (backend *clickHouseBackend) begin() (*clickHouseTransaction, error) {
	backend.lock.Lock()
	return &clickHouseTransaction{
		backend: backend, id: strings.ReplaceAll(uuid.NewString(), "-", ""), snapshots: make(map[string]string),
	}, nil
}

func (tx *clickHouseTransaction) exec(query string, args ...any) (result, error) {
	if tx.closed {
		return nil, sql.ErrTxDone
	}
	if table := clickHouseMutationTable(query); table != "" && table != clickHouseTransactionJournal &&
		table != clickHouseTransactionKeys &&
		!strings.HasPrefix(table, "_renop_tx_") {
		if err := tx.ensureSnapshot(table, query, args); err != nil {
			return nil, err
		}
	}
	return tx.backend.execUnlocked(query, args...)
}

func (tx *clickHouseTransaction) query(query string, args ...any) (rows, error) {
	if tx.closed {
		return nil, sql.ErrTxDone
	}
	return tx.backend.conn.Query(context.Background(), query, normalizeClickHouseArguments(args)...)
}

func (tx *clickHouseTransaction) queryRow(query string, args ...any) row {
	if tx.closed {
		return errorRow{err: sql.ErrTxDone}
	}
	result, err := tx.backend.conn.Query(context.Background(), query, normalizeClickHouseArguments(args)...)
	return &clickHouseTransactionRow{rows: result, err: err}
}

type clickHouseTransactionRow struct {
	rows chdriver.Rows
	err  error
}

func (row *clickHouseTransactionRow) Scan(dest ...any) error {
	if row.err != nil {
		return row.err
	}
	defer row.rows.Close()
	if !row.rows.Next() {
		if err := row.rows.Err(); err != nil {
			return err
		}
		return sql.ErrNoRows
	}
	return scanClickHouseValues(row.rows, dest...)
}

type errorRow struct {
	err error
}

func (row errorRow) Scan(_ ...any) error {
	return row.err
}
