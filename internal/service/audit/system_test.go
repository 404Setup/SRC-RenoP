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
	"bytes"
	"errors"
	"io"
	"log"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"renop/internal/config"
	"renop/internal/core"
	"renop/pkg/pb"
)

func TestGlobalLogFiltersAndHiddenOperators(t *testing.T) {
	db := newTestAuditDB(t)
	state := core.NewAppState()
	state.Inner.DB = db
	now := time.Now().UnixMilli()
	for _, entry := range []*core.AuditLogEntry{
		{Username: "alice", Operator: "alice", Action: "LOGIN", AuthMethod: "Web", CreatedAt: now},
		{Username: "alice", Operator: "hidden-admin", Initiator: "hidden-initiator", Action: "USER_BAN", Trigger: "api", CreatedAt: now},
		{Username: "alice", Operator: "system", Action: ActionSystemHTTPError, Kind: "system", Trigger: "http", Severity: "error", CreatedAt: now},
		{Username: "bob", Operator: "bob", Action: "LOGIN", AuthMethod: "Web", CreatedAt: now},
	} {
		require.NoError(t, db.SaveAuditLog(entry))
	}
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		user := &config.User{Username: c.Get("X-User"), Roles: []string{"base"}}
		if user.Username == "admin" {
			user.Roles = []string{"manager"}
		}
		if user.Username == "moderator" {
			user.Roles = []string{"canmoderate:*"}
		}
		c.Locals("user", user)
		return c.Next()
	})
	SetupAuditRoutes(app.Group("/api/auth"), state)
	for _, tc := range []struct {
		user, path    string
		status, total int
	}{
		{"admin", "/logs", 200, 4},
		{"admin", "/logs?kind=system&severity=error&trigger=http&operator=system", 200, 1},
		{"admin", "/logs?kind=audit&action=LOGIN&username=bob", 200, 1},
		{"alice", "/logs", 403, 0}, {"moderator", "/logs", 403, 0},
		{"alice", "/profile/audit-logs?kind=system&username=bob", 200, 2},
		{"alice", "/profile/audit-logs?operator=@administrator", 200, 1},
		{"alice", "/profile/audit-logs?operator=hidden-admin", 400, 0},
		{"alice", "/profile/audit-logs?initiator=@administrator", 200, 1},
		{"alice", "/profile/audit-logs?initiator=hidden-initiator", 400, 0},
		{"alice", "/profile/audit-logs?initiator=nonexistent", 400, 0},
		{"admin", "/logs?operator=hidden-admin&initiator=hidden-initiator", 200, 1},
		{"alice", "/profile/audit-logs?operator=nonexistent", 400, 0},
		{"admin", "/logs?from=2&until=1", 400, 0},
		{"admin", "/logs?until=invalid", 400, 0},
		{"admin", "/logs?kind=invalid", 400, 0},
		{"admin", "/logs?trigger=" + strings.Repeat("x", 65), 400, 0},
	} {
		request := httptest.NewRequest("GET", "/api/auth"+tc.path, nil)
		request.Header.Set("X-User", tc.user)
		response, err := app.Test(request)
		require.NoError(t, err)
		body, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		require.NoError(t, response.Body.Close())
		require.Equal(t, tc.status, response.StatusCode, tc.path)
		if tc.status != 200 {
			continue
		}
		var result pb.AuditLogList
		require.NoError(t, proto.Unmarshal(body, &result))
		require.EqualValues(t, tc.total, result.Total, tc.path)
		if tc.user == "alice" {
			for _, entry := range result.Logs {
				require.Equal(t, "audit", entry.Kind)
				require.Equal(t, "alice", entry.Username)
				require.NotEqual(t, "hidden-admin", entry.Operator)
				require.NotEqual(t, "hidden-initiator", entry.Initiator)
			}
		}
	}
}

func TestSystemLogCaptureRedactionDrainAndFailure(t *testing.T) {
	db := newTestAuditDB(t)
	state := core.NewAppState()
	state.Inner.DB = db
	output := &bytes.Buffer{}
	previous := log.Writer()
	log.SetOutput(output)
	t.Cleanup(func() { log.SetOutput(previous) })
	stop := StartAuditLogConsumer(state)
	log.Print(`Failed request Authorization: Bearer bearer-secret password="password-secret" url=https://alice:url-secret@example.test`)
	stop()
	stop()
	require.Contains(t, output.String(), "bearer-secret", "the existing process log remains available")
	entries, total, err := db.FilterAuditLogs(core.AuditLogFilter{Kind: "system"}, 10, 0)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.NotContains(t, entries[0].Details, "bearer-secret")
	require.NotContains(t, entries[0].Details, "password-secret")
	require.NotContains(t, entries[0].Details, "url-secret")
	require.Equal(t, "error", entries[0].Severity)
	_, total, err = db.GetAuditLogs("", 10, 0)
	require.NoError(t, err)
	require.Zero(t, total)
	require.NoError(t, db.Close())
	stop = StartAuditLogConsumer(state)
	log.Print("Failed after database shutdown")
	stop()
	require.EqualValues(t, 1, state.Inner.FailuresCount.Load())
	require.Empty(t, state.Inner.AuditLogChan, "persistence errors must not recursively enqueue diagnostics")
}

func TestSystemLogQueueBoundAndHTTPPrivacy(t *testing.T) {
	state := core.NewAppState()
	state.Inner.AuditLogChan = make(chan *core.AuditLogEntry, 1)
	writer := &systemLogWriter{output: io.Discard, state: state}
	_, err := writer.Write([]byte(strings.Repeat("x", 100000)))
	require.NoError(t, err)
	_, err = writer.Write([]byte("overflow"))
	require.NoError(t, err)
	require.EqualValues(t, 1, writer.dropped.Load())
	require.LessOrEqual(t, len((<-state.Inner.AuditLogChan).Details), 4096)
	app := fiber.New(fiber.Config{ErrorHandler: ErrorHandler(state)})
	app.Use(HTTPDiagnostics(state))
	app.Get("/failure", func(c fiber.Ctx) error { return errors.New("database password=private-secret") })
	app.Get("/returned", func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusBadGateway) })
	response, err := app.Test(httptest.NewRequest("GET", "/failure?token=request-secret", nil))
	require.NoError(t, err)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, 500, response.StatusCode)
	require.NotContains(t, string(body), "private-secret")
	entry := <-state.Inner.AuditLogChan
	require.Equal(t, "http", entry.Trigger)
	require.NotContains(t, entry.Details, "private-secret")
	require.NotContains(t, entry.Details, "request-secret")
	require.Empty(t, state.Inner.AuditLogChan, "returned errors must be logged exactly once")
	response, err = app.Test(httptest.NewRequest("GET", "/returned", nil))
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	entry = <-state.Inner.AuditLogChan
	require.Contains(t, entry.Details, "HTTP 502")
}
