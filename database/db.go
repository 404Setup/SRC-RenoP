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
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "modernc.org/sqlite"

	"renop/config"
	"renop/core"
)

type DB struct {
	SqlDB            *sql.DB
	Dialect          Dialect
	tokenCache       *TTLCache[string, *core.AccessToken]
	tokenSecretCache *TTLCache[string, *core.AccessToken]
	sessionCache     *TTLCache[string, *core.Session]
}

func InitDB(cfg config.DatabaseConfig) (*DB, error) {
	if !cfg.Enabled {
		return nil, nil
	}

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

	if driver == "sqlite3" || driver == "sqlite" {
		sqlDB, err = openAndPing("sqlite3", dsn, cfg)
		if err != nil {
			actualDriver = "sqlite"
			sqlDB, err = openAndPing("sqlite", dsn, cfg)
		}
	} else {
		sqlDB, err = openAndPing(driver, dsn, cfg)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to initialize database (%s): %w", driver, err)
	}

	dialect := NewDialect(actualDriver)

	db := &DB{
		SqlDB:            sqlDB,
		Dialect:          dialect,
		tokenCache:       NewTTLCache[string, *core.AccessToken](10 * time.Minute),
		tokenSecretCache: NewTTLCache[string, *core.AccessToken](10 * time.Minute),
		sessionCache:     NewTTLCache[string, *core.Session](15 * time.Minute),
	}

	if err := db.Dialect.InitTables(sqlDB); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("failed to initialize database tables: %w", err)
	}

	log.Printf("Database initialized successfully (driver: %s, dsn: %s)", actualDriver, sanitizeDSN(dsn))
	return db, nil
}

func openAndPing(driver, dsn string, cfg config.DatabaseConfig) (*sql.DB, error) {
	sqlDB, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, err
	}

	maxOpen := cfg.MaxOpenConns
	if maxOpen <= 0 {
		maxOpen = 25
	}
	sqlDB.SetMaxOpenConns(maxOpen)

	maxIdle := cfg.MaxIdleConns
	if maxIdle <= 0 {
		maxIdle = 25
	}
	sqlDB.SetMaxIdleConns(maxIdle)

	lifetimeSec := cfg.ConnMaxLifetimeSec
	if lifetimeSec <= 0 {
		lifetimeSec = 300
	}
	sqlDB.SetConnMaxLifetime(time.Duration(lifetimeSec) * time.Second)
	idleTime := min(time.Duration(lifetimeSec/2)*time.Second, 10*time.Minute)
	sqlDB.SetConnMaxIdleTime(idleTime)

	if err := sqlDB.Ping(); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}

	if strings.HasPrefix(driver, "sqlite") {
		pragmas := []string{
			"PRAGMA foreign_keys = ON;",
			"PRAGMA journal_mode = WAL;",
			"PRAGMA busy_timeout = 5000;",
			"PRAGMA synchronous = NORMAL;",
			"PRAGMA cache_size = -8192;",
			"PRAGMA temp_store = MEMORY;",
			"PRAGMA mmap_size = 268435456;",
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

func safePrefix(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func (db *DB) Close() error {
	if db == nil {
		return nil
	}
	if db.tokenCache != nil {
		db.tokenCache.Close()
	}
	if db.tokenSecretCache != nil {
		db.tokenSecretCache.Close()
	}
	if db.sessionCache != nil {
		db.sessionCache.Close()
	}
	if db.SqlDB == nil {
		return nil
	}
	return db.SqlDB.Close()
}
