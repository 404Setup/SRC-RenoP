/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package repositorygate

import (
	"testing"
	"time"
)

func TestMigrationWaitsForRepositoryMutation(t *testing.T) {
	releaseMutation := AcquireMutation("releases")
	acquired := make(chan func(), 1)
	started := make(chan struct{})
	go func() {
		close(started)
		acquired <- AcquireMigration("releases")
	}()
	<-started
	select {
	case release := <-acquired:
		release()
		t.Fatal("migration acquired the gate during an active mutation")
	case <-time.After(25 * time.Millisecond):
	}
	releaseMutation()
	select {
	case release := <-acquired:
		release()
	case <-time.After(time.Second):
		t.Fatal("migration did not acquire the released repository gate")
	}
}

func TestMutationWaitsForRepositoryMigration(t *testing.T) {
	releaseMigration := AcquireMigration("releases")
	acquired := make(chan func(), 1)
	started := make(chan struct{})
	go func() {
		close(started)
		acquired <- AcquireMutation("releases")
	}()
	<-started
	select {
	case release := <-acquired:
		release()
		t.Fatal("mutation acquired the gate during migration")
	case <-time.After(25 * time.Millisecond):
	}
	releaseMigration()
	select {
	case release := <-acquired:
		release()
	case <-time.After(time.Second):
		t.Fatal("mutation did not acquire the released repository gate")
	}
}
