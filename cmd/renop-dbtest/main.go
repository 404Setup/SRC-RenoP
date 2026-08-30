/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

// Command renop-dbtest validates one RenoP database driver against an isolated database.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"renop/internal/config"
	"renop/internal/database"
)

func main() {
	os.Exit(run())
}

func run() (exitCode int) {
	driver := flag.String("driver", "", "database driver: sqlite3, postgres, mysql, or clickhouse")
	dsn := flag.String("dsn", "", "connection string for an isolated test database")
	confirmIsolated := flag.Bool("confirm-isolated", false, "confirm the target database is disposable and isolated")
	timeout := flag.Duration("timeout", 5*time.Minute, "overall validation timeout")
	jsonOutput := flag.Bool("json", false, "write the result as JSON")
	flag.Parse()
	if *driver == "" || *dsn == "" || !*confirmIsolated {
		fmt.Fprintln(os.Stderr, "driver, dsn, and -confirm-isolated are required; the tool creates and mutates RenoP tables")
		return 2
	}
	db, err := database.InitDB(config.DatabaseConfig{
		Driver: *driver, Dsn: *dsn, MaxOpenConns: 4, MaxIdleConns: 2, ConnMaxLifetimeSec: 300,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer func() {
		if err := db.Close(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			if exitCode == 0 {
				exitCode = 1
			}
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	results, err := database.RunDriverCheck(ctx, db)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if *jsonOutput {
		if err := json.NewEncoder(os.Stdout).Encode(struct {
			Driver  string                       `json:"driver"`
			Results []database.DriverCheckResult `json:"results"`
		}{Driver: db.Dialect.Name(), Results: results}); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	}
	for _, result := range results {
		fmt.Printf("PASS %-24s %s\n", result.Name, result.Duration.Round(time.Millisecond))
	}
	return 0
}
