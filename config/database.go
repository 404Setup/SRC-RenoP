/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package config

type DatabaseConfig struct {
	Driver             string `json:"driver" yaml:"driver"` // "sqlite3" or "mysql"
	Dsn                string `json:"dsn" yaml:"dsn"`
	MaxOpenConns       int    `json:"max_open_conns" yaml:"max_open_conns"`
	MaxIdleConns       int    `json:"max_idle_conns" yaml:"max_idle_conns"`
	ConnMaxLifetimeSec int    `json:"conn_max_lifetime_sec" yaml:"conn_max_lifetime_sec"`
}

func DefaultDatabaseConfig() DatabaseConfig {
	return DatabaseConfig{
		Driver:             "sqlite3",
		Dsn:                "renop.db",
		MaxOpenConns:       25,
		MaxIdleConns:       25,
		ConnMaxLifetimeSec: 300,
	}
}

func (d *DatabaseConfig) setDefaults() {
	if d.Driver == "" {
		d.Driver = "sqlite3"
	}
	if d.Dsn == "" && (d.Driver == "sqlite3" || d.Driver == "sqlite") {
		d.Dsn = "renop.db"
	}
	if d.MaxOpenConns <= 0 {
		d.MaxOpenConns = 25
	}
	if d.MaxIdleConns <= 0 {
		d.MaxIdleConns = 25
	}
	if d.ConnMaxLifetimeSec <= 0 {
		d.ConnMaxLifetimeSec = 300
	}
}
