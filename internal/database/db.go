/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

// Package database provides dialect-aware persistence for RenoP domains.
package database

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"

	"renop/internal/config"
	"renop/internal/core"
)

type DB struct {
	SQLDB            *sql.DB
	Dialect          Dialect
	tokenCache       *TTLCache[string, *core.AccessToken]
	tokenSecretCache *TTLCache[string, *core.AccessToken]
	sessionCache     *TTLCache[string, *core.Session]
}

type Tx struct {
	*sql.Tx
	db *DB
}

func (db *DB) Rebind(query string) string {
	if db == nil || db.Dialect == nil {
		return query
	}
	return db.Dialect.Rebind(query)
}

func (db *DB) Exec(query string, args ...any) (sql.Result, error) {
	return db.SQLDB.Exec(db.Rebind(query), args...)
}

func (db *DB) Query(query string, args ...any) (*sql.Rows, error) {
	return db.SQLDB.Query(db.Rebind(query), args...)
}

func (db *DB) QueryRow(query string, args ...any) *sql.Row {
	return db.SQLDB.QueryRow(db.Rebind(query), args...)
}

func (db *DB) Begin() (*Tx, error) {
	tx, err := db.SQLDB.Begin()
	if err != nil {
		return nil, err
	}
	return &Tx{Tx: tx, db: db}, nil
}

func (tx *Tx) Exec(query string, args ...any) (sql.Result, error) {
	return tx.Tx.Exec(tx.db.Rebind(query), args...)
}

func (tx *Tx) Query(query string, args ...any) (*sql.Rows, error) {
	return tx.Tx.Query(tx.db.Rebind(query), args...)
}

func (tx *Tx) QueryRow(query string, args ...any) *sql.Row {
	return tx.Tx.QueryRow(tx.db.Rebind(query), args...)
}

func InitDB(cfg config.DatabaseConfig) (*DB, error) {
	driver := strings.TrimSpace(cfg.Driver)
	if driver == "" {
		driver = "sqlite3"
	}

	dsn := strings.TrimSpace(cfg.Dsn)
	if dsn == "" {
		if strings.HasPrefix(driver, "sqlite") {
			dsn = "renop.db"
		}
	}

	var sqlDB *sql.DB
	var err error
	actualDriver := driver

	lowerDriver := strings.ToLower(driver)
	if strings.HasPrefix(lowerDriver, "sqlite") {
		dsn = buildSQLiteDSN(dsn)
		sqlDB, err = openAndPing("sqlite3", dsn, cfg)
		if err != nil {
			actualDriver = "sqlite"
			sqlDB, err = openAndPing("sqlite", dsn, cfg)
		}
	} else if lowerDriver == "postgres" || lowerDriver == "postgresql" || lowerDriver == "pgx" || lowerDriver == "pg" {
		actualDriver = "postgres"
		sqlDB, err = openAndPing("pgx", dsn, cfg)
	} else {
		sqlDB, err = openAndPing(driver, dsn, cfg)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to initialize database (%s): %w", driver, err)
	}

	dialect := NewDialect(actualDriver)

	db := &DB{
		SQLDB:            sqlDB,
		Dialect:          dialect,
		tokenCache:       NewTTLCache[string, *core.AccessToken](10 * time.Minute),
		tokenSecretCache: NewTTLCache[string, *core.AccessToken](10 * time.Minute),
		sessionCache:     NewTTLCache[string, *core.Session](15 * time.Minute),
	}

	if err := db.Dialect.InitTables(sqlDB); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("failed to initialize database tables: %w", err)
	}
	if err := db.initializeUserIdentities(); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("failed to initialize stable user identities: %w", err)
	}
	if err := db.migrateLegacyAPITokens(); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("failed to migrate legacy API tokens: %w", err)
	}
	if err := db.migrateGlobalMavenDomains(); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("failed to migrate global Maven domains: %w", err)
	}

	log.Printf("Database initialized successfully (driver: %s, dsn: %s)", actualDriver, sanitizeDSN(dsn))
	return db, nil
}

func buildSQLiteDSN(dsn string) string {
	if strings.Contains(dsn, "_pragma") || strings.Contains(dsn, "mode=") {
		return dsn
	}
	pragmas := "_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)&_pragma=cache_size(-4096)&_pragma=temp_store(MEMORY)&_pragma=mmap_size(268435456)"
	if strings.Contains(dsn, "?") {
		return dsn + "&" + pragmas
	}
	return dsn + "?" + pragmas
}

func openAndPing(driver, dsn string, cfg config.DatabaseConfig) (*sql.DB, error) {
	sqlDB, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, err
	}

	isSQLite := strings.HasPrefix(strings.ToLower(driver), "sqlite")

	maxOpen := cfg.MaxOpenConns
	if maxOpen <= 0 {
		if isSQLite {
			maxOpen = 10
		} else {
			maxOpen = 25
		}
	}
	sqlDB.SetMaxOpenConns(maxOpen)

	maxIdle := cfg.MaxIdleConns
	if maxIdle <= 0 {
		if isSQLite {
			maxIdle = min(4, maxOpen)
		} else {
			maxIdle = min(5, maxOpen)
		}
	}
	sqlDB.SetMaxIdleConns(maxIdle)

	lifetimeSec := cfg.ConnMaxLifetimeSec
	if lifetimeSec <= 0 {
		lifetimeSec = 300
	}
	sqlDB.SetConnMaxLifetime(time.Duration(lifetimeSec) * time.Second)
	idleTime := min(time.Duration(lifetimeSec/2)*time.Second, 2*time.Minute)
	sqlDB.SetConnMaxIdleTime(idleTime)

	if err := sqlDB.Ping(); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}

	if isSQLite {
		pragmas := []string{
			"PRAGMA foreign_keys = ON;",
			"PRAGMA journal_mode = WAL;",
			"PRAGMA busy_timeout = 5000;",
			"PRAGMA synchronous = NORMAL;",
			"PRAGMA cache_size = -4096;",
			"PRAGMA temp_store = MEMORY;",
			"PRAGMA mmap_size = 268435456;",
			"PRAGMA trusted_schema = OFF;",
			"PRAGMA cell_size_check = ON;",
		}
		for _, pragma := range pragmas {
			if _, err := sqlDB.Exec(pragma); err != nil {
				log.Printf("[WARN] SQLite pragma failed (%s): %v", pragma, err)
			}
		}
	}

	return sqlDB, nil
}

func sanitizeDSN(dsn string) string {
	if at := strings.Index(dsn, "@"); at != -1 {
		if colon := strings.Index(dsn[:at], ":"); colon != -1 {
			return dsn[:colon+1] + "****" + dsn[at:]
		}
	}
	return dsn
}

func sessionTokenPrefix(s string) string {
	if len(s) <= 8 {
		return s
	}
	return s[:8]
}

// SanitizeInputString trims null bytes and invalid UTF-8 control characters from SQL inputs.
func SanitizeInputString(s string, maxLen int) string {
	if s == "" {
		return ""
	}
	if hasControlBytes(s) {
		s = sanitizeControlBytes(s)
	}
	if maxLen > 0 && len(s) > maxLen {
		s = truncateUTF8(s, maxLen)
	}
	return s
}

func hasControlBytes(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < 32 && c != '\t' && c != '\n' && c != '\r') || c == 127 {
			return true
		}
	}
	return false
}

func sanitizeControlBytes(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 32 || c == '\t' || c == '\n' || c == '\r') && c != 127 {
			b.WriteByte(c)
		}
	}
	return b.String()
}

func truncateUTF8(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	for maxLen > 0 && (s[maxLen]&0xC0) == 0x80 {
		maxLen--
	}
	return s[:maxLen]
}

func (db *DB) Close() error {
	if db == nil {
		return nil
	}
	if db.SQLDB == nil {
		return nil
	}
	if db.Dialect != nil && strings.HasPrefix(db.Dialect.Name(), "sqlite") {
		_, _ = db.SQLDB.Exec("PRAGMA wal_checkpoint(TRUNCATE);")
	}
	return db.SQLDB.Close()
}

// EvictExpiredCaches removes expired entries from all database lookup caches.
func (db *DB) EvictExpiredCaches() {
	if db == nil {
		return
	}
	if db.tokenCache != nil {
		db.tokenCache.EvictExpired()
	}
	if db.tokenSecretCache != nil {
		db.tokenSecretCache.EvictExpired()
	}
	if db.sessionCache != nil {
		db.sessionCache.EvictExpired()
	}
}
