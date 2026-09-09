/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package tickettest

import (
	"testing"
	"time"

	"renop/internal/core"
)

// ClaimTicket establishes a browser session and returns a workflow with live assignment.
func ClaimTicket(t *testing.T, db core.StateDB, task *core.ReviewTask, actor string) *core.ReviewTask {
	t.Helper()
	now := time.Now().UnixMilli()
	secret := "test-ticket-" + task.ID + "-" + actor
	session := &core.Session{PublicID: secret, Username: actor, CreatedAt: now}
	session.LastActive.Store(now)
	if err := db.SaveSession(session, secret); err != nil {
		t.Fatal(err)
	}
	claimed, err := db.TransitionTicket(task.ID, actor, secret, core.TicketAction{Action: "claim"}, now)
	if cleanupErr := db.DeleteSession(secret); cleanupErr != nil {
		t.Fatal(cleanupErr)
	}
	if err != nil {
		t.Fatal(err)
	}
	return claimed
}
