/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package database_test

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"renop/internal/core"
	"renop/internal/database"
)

func ticketSession(t *testing.T, db *database.DB, name string) string {
	t.Helper()
	now := time.Now().UnixMilli()
	session := &core.Session{PublicID: "ticket-" + name, Username: name, CreatedAt: now}
	session.LastActive.Store(now)
	secret := "ticket-session-" + name
	require.NoError(t, db.SaveSession(session, secret))
	return secret
}

func TestTicketClaimsEscalationAndIdentityPrivacy(t *testing.T) {
	db := newMavenDB(t)
	for name, roles := range map[string][]string{
		"mod1": {"canmoderate:releases"}, "mod2": {"canmoderate:releases"}, "outside": {"canmoderate:other"},
		"admin2": {"manager"}, "admin3": {"manager"}, "admin4": {"manager"},
	} {
		require.NoError(t, db.SaveToken(&core.AccessToken{Name: name, Permissions: roles}))
	}
	sessions := make(map[string]string)
	for _, name := range []string{"alice", "bob", "mod1", "mod2", "outside", "admin", "admin2", "admin3", "admin4"} {
		sessions[name] = ticketSession(t, db, name)
	}
	now := time.Now().UnixMilli()
	task, err := db.CreateTicket(core.TicketRequest{Kind: core.TicketKindFeedback, Repository: "releases",
		Title: "Download issue", Body: "First line\nSecond line"}, "alice", sessions["alice"], now)
	require.NoError(t, err)
	act := func(actor, action string, force bool) (*core.ReviewTask, error) {
		return db.TransitionTicket(task.ID, actor, sessions[actor], core.TicketAction{
			Action: action, Force: force, Outcome: "resolved", Response: "Resolved\nPlease try again."}, now+1)
	}
	_, err = act("outside", "claim", false)
	require.ErrorIs(t, err, core.ErrReviewPermissionDenied)
	_, err = act("mod1", "complete", false)
	require.ErrorIs(t, err, core.ErrTicketClaimRequired)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, actor := range []string{"mod1", "mod2"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := act(actor, "claim", false)
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	winners := 0
	for err := range results {
		if err == nil {
			winners++
		} else {
			require.ErrorIs(t, err, core.ErrTicketOccupied)
		}
	}
	require.Equal(t, 1, winners)
	_, err = act("admin", "claim", true)
	require.NoError(t, err)
	_, err = act("admin2", "claim", true)
	require.ErrorIs(t, err, core.ErrTicketOccupied)
	require.NoError(t, db.UpdateToken("admin", func(token *core.AccessToken) { token.Permissions = []string{"base"} }))
	available, err := db.GetTicket(task.ID, "admin2")
	require.NoError(t, err)
	require.Contains(t, available.Actions, "force_claim")
	_, err = act("admin2", "claim", true)
	require.NoError(t, err)
	_, err = act("admin2", "release", false)
	require.NoError(t, err)
	require.NoError(t, db.UpdateToken("admin", func(token *core.AccessToken) { token.Permissions = []string{"manager"} }))
	_, err = act("mod1", "claim", false)
	require.NoError(t, err)
	require.NoError(t, db.UpdateToken("mod1", func(token *core.AccessToken) { token.Permissions = []string{"manager"} }))
	available, err = db.GetTicket(task.ID, "admin2")
	require.NoError(t, err)
	require.NotContains(t, available.Actions, "force_claim")
	_, err = act("admin2", "claim", true)
	require.ErrorIs(t, err, core.ErrTicketOccupied)
	require.NoError(t, db.UpdateToken("mod1", func(token *core.AccessToken) { token.Permissions = []string{"canmoderate:releases"} }))
	_, err = act("mod2", "complete", false)
	require.ErrorIs(t, err, core.ErrTicketClaimRequired)
	_, err = act("mod1", "escalate", false)
	require.NoError(t, err)
	_, err = act("mod2", "claim", false)
	require.ErrorIs(t, err, core.ErrReviewPermissionDenied)
	for _, actor := range []string{"admin", "admin2"} {
		_, err = act(actor, "claim", false)
		require.NoError(t, err)
		_, err = act(actor, "escalate", false)
		require.NoError(t, err)
		_, err = act(actor, "claim", false)
		require.ErrorIs(t, err, core.ErrReviewPermissionDenied)
	}
	task, err = act("admin3", "claim", false)
	require.NoError(t, err)
	require.Equal(t, 3, task.Escalations)
	_, err = act("admin4", "claim", true)
	require.ErrorIs(t, err, core.ErrTicketOccupied)
	for _, action := range []string{"escalate", "release"} {
		_, err = act("admin3", action, false)
		require.ErrorIs(t, err, core.ErrTicketEscalationLimit)
	}
	task, err = act("admin3", "process", false)
	require.NoError(t, err)
	require.Equal(t, core.TicketProcessed, task.TicketState.Status)
	task, err = act("admin3", "complete", false)
	require.NoError(t, err)
	require.Equal(t, core.TicketCompleted, task.TicketState.Status)
	requester, err := db.GetTicket(task.ID, "alice")
	require.NoError(t, err)
	require.Empty(t, requester.Assignee)
	require.Empty(t, requester.DecidedBy)
	require.Empty(t, requester.EscalatedBy)
	staff, err := db.GetTicket(task.ID, "mod1")
	require.NoError(t, err)
	require.Equal(t, "admin3", staff.DecidedBy)
	_, count, err := db.ListReviewTasks(core.ReviewTaskListOptions{Username: "mod1", TicketStatus: core.TicketCompleted, Limit: 10})
	require.NoError(t, err)
	require.Equal(t, 1, count)
	_, count, err = db.ListReviewTasks(core.ReviewTaskListOptions{Username: "outside", Administrator: true, TicketStatus: "all", Limit: 10})
	require.NoError(t, err)
	require.Zero(t, count)
}

func TestTicketReportsRejectSelfDuplicatesAndHiddenResources(t *testing.T) {
	db := newMavenDB(t)
	alice, bob := ticketSession(t, db, "alice"), ticketSession(t, db, "bob")
	now := time.Now().UnixMilli()
	request := core.TicketRequest{Kind: core.TicketKindReport, Title: "Report", Body: "Please investigate.",
		Target: core.ResourceLockTarget{Format: "user", Name: "alice"}}
	_, err := db.CreateTicket(request, "alice", alice, now)
	require.ErrorIs(t, err, core.ErrReviewPermissionDenied)
	task, err := db.CreateTicket(request, "bob", bob, now)
	require.NoError(t, err)
	_, err = db.CreateTicket(request, "bob", bob, now)
	require.ErrorIs(t, err, core.ErrReviewTaskExists)
	_, err = db.GetTicket(task.ID, "alice")
	require.ErrorIs(t, err, core.ErrReviewPermissionDenied)
	report, err := db.GetTicket(task.ID, "bob")
	require.NoError(t, err)
	require.Empty(t, report.ResourceVersion)
	require.Equal(t, "alice", report.ResourceKey)
	require.NoError(t, db.CreateMavenDomain(&core.MavenDomain{Domain: "com.example", VerificationType: "dns",
		VerificationHost: "example.com", VerificationCode: "proof", CreatedAt: now}, "alice"))
	require.NoError(t, db.MarkMavenDomainVerified("com.example", "proof", now, nil))
	require.NoError(t, db.RecordMavenPublication(&core.MavenArtifact{Repository: "maven", Domain: "com.example",
		GroupID: "com.example", ArtifactID: "demo", Publisher: "alice", CreatedAt: now},
		&core.MavenVersion{Version: "1.0", Publisher: "alice", CreatedAt: now}))
	request.Target = core.ResourceLockTarget{Format: "maven", Repository: "maven", Name: "com.example:demo", Version: "1.0"}
	_, err = db.CreateTicket(request, "alice", alice, now)
	require.ErrorIs(t, err, core.ErrReviewPermissionDenied)
	task, err = db.CreateTicket(request, "bob", bob, now)
	require.NoError(t, err)
	report, err = db.GetTicket(task.ID, "bob")
	require.NoError(t, err)
	require.Equal(t, "1.0", report.ResourceVersion)
	require.NoError(t, db.SetResourceLock(&core.ResourceLock{ResourceLockTarget: request.Target,
		Source: core.ResourceLockSystem, Mode: core.ResourceLockRead, Reason: "trojan", LockedAt: now}, "", ""))
	_, err = db.CreateTicket(request, "bob", bob, now)
	require.ErrorIs(t, err, core.ErrReviewPermissionDenied)
}
