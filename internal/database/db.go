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
	clickHouse       *clickHouseBackend
	Dialect          Dialect
	tokenCache       *TTLCache[string, *core.AccessToken]
	tokenSecretCache *TTLCache[string, *core.AccessToken]
	sessionCache     *TTLCache[string, *core.Session]
	userIDCache      *TTLCache[string, string]
	profileCache     *TTLCache[string, core.UserProfile]
}

type Tx struct {
	sqlTx      *sql.Tx
	clickHouse *clickHouseTransaction
	db         *DB
}

type result interface {
	LastInsertId() (int64, error)
	RowsAffected() (int64, error)
}

type rows interface {
	Next() bool
	Scan(dest ...any) error
	Close() error
	Err() error
}

type row interface {
	Scan(dest ...any) error
}

func (db *DB) Rebind(query string) string {
	if db == nil || db.Dialect == nil {
		return query
	}
	return db.Dialect.Rebind(query)
}

func (db *DB) Exec(query string, args ...any) (result, error) {
	if db != nil && db.clickHouse != nil {
		return db.clickHouse.exec(db.Rebind(query), args...)
	}
	return db.SQLDB.Exec(db.Rebind(query), args...)
}

func (db *DB) Query(query string, args ...any) (rows, error) {
	if db != nil && db.clickHouse != nil {
		return db.clickHouse.query(db.Rebind(query), args...)
	}
	return db.SQLDB.Query(db.Rebind(query), args...)
}

func (db *DB) QueryRow(query string, args ...any) row {
	if db != nil && db.clickHouse != nil {
		return db.clickHouse.queryRow(db.Rebind(query), args...)
	}
	return db.SQLDB.QueryRow(db.Rebind(query), args...)
}

func (db *DB) Begin() (*Tx, error) {
	if db != nil && db.clickHouse != nil {
		tx, err := db.clickHouse.begin()
		if err != nil {
			return nil, err
		}
		return &Tx{clickHouse: tx, db: db}, nil
	}
	tx, err := db.SQLDB.Begin()
	if err != nil {
		return nil, err
	}
	return &Tx{sqlTx: tx, db: db}, nil
}

func (tx *Tx) Exec(query string, args ...any) (result, error) {
	if tx.clickHouse != nil {
		return tx.clickHouse.exec(tx.db.Rebind(query), args...)
	}
	return tx.sqlTx.Exec(tx.db.Rebind(query), args...)
}

func (tx *Tx) Query(query string, args ...any) (rows, error) {
	if tx.clickHouse != nil {
		return tx.clickHouse.query(tx.db.Rebind(query), args...)
	}
	return tx.sqlTx.Query(tx.db.Rebind(query), args...)
}

func (tx *Tx) QueryRow(query string, args ...any) row {
	if tx.clickHouse != nil {
		return tx.clickHouse.queryRow(tx.db.Rebind(query), args...)
	}
	return tx.sqlTx.QueryRow(tx.db.Rebind(query), args...)
}

func (tx *Tx) Commit() error {
	if tx.clickHouse != nil {
		return tx.clickHouse.commit()
	}
	return tx.sqlTx.Commit()
}

func (tx *Tx) Rollback() error {
	if tx.clickHouse != nil {
		return tx.clickHouse.rollback()
	}
	return tx.sqlTx.Rollback()
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
		} else if strings.EqualFold(driver, "clickhouse") || strings.EqualFold(driver, "ch") {
			dsn = "clickhouse://default@localhost:9000/default"
		}
	}

	lowerDriver := strings.ToLower(driver)
	if lowerDriver == "clickhouse" || lowerDriver == "ch" {
		backend, err := openClickHouse(cfg, dsn)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize database (%s): %w", driver, err)
		}
		db := newDatabaseCaches(&DB{
			SQLDB: new(sql.DB), clickHouse: backend, Dialect: NewDialect("clickhouse"),
		})
		if err := db.initializePersistentMigrations(); err != nil {
			_ = backend.close()
			return nil, err
		}
		log.Printf("Database initialized successfully (driver: clickhouse, dsn: %s)", sanitizeDSN(dsn))
		return db, nil
	}

	var sqlDB *sql.DB
	var err error
	actualDriver := driver

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

	db := newDatabaseCaches(&DB{
		SQLDB:   sqlDB,
		Dialect: dialect,
	})

	if err := db.Dialect.InitTables(sqlDB); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("failed to initialize database tables: %w", err)
	}
	if err := db.initializePersistentMigrations(); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}

	log.Printf("Database initialized successfully (driver: %s, dsn: %s)", actualDriver, sanitizeDSN(dsn))
	return db, nil
}

func newDatabaseCaches(db *DB) *DB {
	db.tokenCache = NewTTLCacheWithCapacity[string, *core.AccessToken](10*time.Minute, 4096)
	db.tokenSecretCache = NewTTLCacheWithCapacity[string, *core.AccessToken](10*time.Minute, 8192)
	db.sessionCache = NewTTLCacheWithCapacity[string, *core.Session](15*time.Minute, 32768)
	db.userIDCache = NewTTLCacheWithCapacity[string, string](30*time.Minute, 8192)
	db.profileCache = NewTTLCacheWithCapacity[string, core.UserProfile](10*time.Minute, 8192)
	return db
}

func (db *DB) initializePersistentMigrations() error {
	if err := db.initializeUserIdentities(); err != nil {
		return fmt.Errorf("failed to initialize stable user identities: %w", err)
	}
	if err := db.migrateLegacyAPITokens(); err != nil {
		return fmt.Errorf("failed to migrate legacy API tokens: %w", err)
	}
	if err := db.migrateGlobalMavenDomains(); err != nil {
		return fmt.Errorf("failed to migrate global Maven domains: %w", err)
	}
	return nil
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
	if db.clickHouse != nil {
		return db.clickHouse.close()
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
	if db.userIDCache != nil {
		db.userIDCache.EvictExpired()
	}
	if db.profileCache != nil {
		db.profileCache.EvictExpired()
	}
}
