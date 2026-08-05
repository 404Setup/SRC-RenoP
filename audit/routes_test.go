/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package audit

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"

	"renop/config"
	"renop/core"
	"renop/database"
)

func newTestAuditDB(t *testing.T) *database.DB {
	t.Helper()
	dbFile := t.TempDir() + "/audit_test.db"
	cfg := config.DatabaseConfig{
		Driver:       "sqlite3",
		Dsn:          dbFile,
		MaxOpenConns: 5,
		MaxIdleConns: 5,
	}
	db, err := database.InitDB(cfg)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func drainAuditChan(state *core.AppState, db *database.DB) {
	for len(state.Inner.AuditLogChan) > 0 {
		e := <-state.Inner.AuditLogChan
		if e.CreatedAt <= 0 {
			e.CreatedAt = time.Now().UnixMilli()
		}
		_ = db.SaveAuditLog(e)
	}
}

func TestAuditLogFlow(t *testing.T) {
	db := newTestAuditDB(t)

	state := core.NewAppState()
	state.Inner.DB = db
	state.Inner.Config.Store(config.DefaultConfig())

	now := time.Now().UnixMilli()
	Log(state, &core.AuditLogEntry{
		Username:   "user1",
		Operator:   "user1",
		Action:     "LOGIN",
		Details:    "Logged in",
		AuthMethod: "Password",
		IP:         "127.0.0.1",
		CreatedAt:  now - 1000,
	})

	Log(state, &core.AuditLogEntry{
		Username:   "user1",
		Operator:   "admin",
		Action:     "USER_PERMISSION_UPDATE",
		Details:    "Roles updated by admin",
		AuthMethod: "Session",
		IP:         "127.0.0.1",
		CreatedAt:  now,
	})

	// Drain the async channel into DB synchronously for test determinism
	drainAuditChan(state, db)

	app := fiber.New()
	apiGroup := app.Group("/api/auth")

	app.Use(func(c fiber.Ctx) error {
		if uHeader := c.Get("X-Test-User"); uHeader != "" {
			u := &config.User{
				Username: uHeader,
				Roles:    []string{"base"},
			}
			if uHeader == "admin" {
				u.Roles = []string{"manager"}
			}
			c.Locals("user", u)
		}
		return c.Next()
	})

	SetupAuditRoutes(apiGroup, state)

	t.Run("Self View Audit Logs - Operator Masked", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/auth/profile/audit-logs", nil)
		req.Header.Set("X-Test-User", "user1")
		resp, err := app.Test(req)
		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		var res AuditLogListResponse
		err = json.NewDecoder(resp.Body).Decode(&res)
		assert.NoError(t, err)
		assert.Equal(t, 2, res.Total)

		for _, l := range res.Logs {
			if l.Action == "USER_PERMISSION_UPDATE" {
				assert.Equal(t, "Administrator", l.Operator)
			}
		}
	})

	t.Run("User Cannot Delete Own Audit Logs", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/api/auth/profile/audit-logs", nil)
		req.Header.Set("X-Test-User", "user1")
		resp, err := app.Test(req)
		assert.NoError(t, err)
		assert.Equal(t, 403, resp.StatusCode)
	})

	t.Run("Admin Views Full User Audit Logs", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/auth/users/user1/audit-logs", nil)
		req.Header.Set("X-Test-User", "admin")
		resp, err := app.Test(req)
		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		var res AuditLogListResponse
		err = json.NewDecoder(resp.Body).Decode(&res)
		assert.NoError(t, err)
		assert.Equal(t, 2, res.Total)

		foundAdminOp := false
		for _, l := range res.Logs {
			if l.Action == "USER_PERMISSION_UPDATE" {
				assert.Equal(t, "admin", l.Operator)
				foundAdminOp = true
			}
		}
		assert.True(t, foundAdminOp)
	})

	t.Run("Admin Clears User Audit Logs", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/api/auth/users/user1/audit-logs", nil)
		req.Header.Set("X-Test-User", "admin")
		resp, err := app.Test(req)
		assert.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		req2 := httptest.NewRequest("GET", "/api/auth/users/user1/audit-logs", nil)
		req2.Header.Set("X-Test-User", "admin")
		resp2, err := app.Test(req2)
		assert.NoError(t, err)
		assert.Equal(t, 200, resp2.StatusCode)

		var res2 AuditLogListResponse
		_ = json.NewDecoder(resp2.Body).Decode(&res2)
		// After clearing, only the LOG_CLEAR entry itself should remain (added by DeleteUserAuditLogs)
		for _, l := range res2.Logs {
			assert.NotEqual(t, "LOGIN", l.Action)
			assert.NotEqual(t, "USER_PERMISSION_UPDATE", l.Action)
		}
	})
}
