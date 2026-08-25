/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the terms of the Mozilla Public License, v. 2.0.
 */

package tasks

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestSchedulerRunsTasksAndStops(t *testing.T) {
	scheduler := NewScheduler()
	var first atomic.Int64
	var second atomic.Int64
	if err := scheduler.Schedule("first", 10*time.Millisecond, 0, func(context.Context) {
		first.Add(1)
	}); err != nil {
		t.Fatal(err)
	}
	if err := scheduler.Schedule("second", 15*time.Millisecond, 0, func(context.Context) {
		second.Add(1)
	}); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(time.Second)
	for (first.Load() < 2 || second.Load() < 2) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if first.Load() < 2 || second.Load() < 2 {
		t.Fatalf("scheduled counts = first %d, second %d", first.Load(), second.Load())
	}
	scheduler.Close()
	firstAfterClose := first.Load()
	secondAfterClose := second.Load()
	time.Sleep(30 * time.Millisecond)
	if first.Load() != firstAfterClose || second.Load() != secondAfterClose {
		t.Fatal("scheduler executed tasks after Close")
	}
}

func TestSchedulerCoalescesNonReentrantTask(t *testing.T) {
	scheduler := NewScheduler()
	defer scheduler.Close()
	var active atomic.Int64
	var maxActive atomic.Int64
	var runs atomic.Int64
	started := make(chan struct{}, 1)
	if err := scheduler.Schedule("slow", time.Hour, 0, func(ctx context.Context) {
		current := active.Add(1)
		for {
			observed := maxActive.Load()
			if current <= observed || maxActive.CompareAndSwap(observed, current) {
				break
			}
		}
		runs.Add(1)
		select {
		case started <- struct{}{}:
		default:
		}
		select {
		case <-ctx.Done():
		case <-time.After(35 * time.Millisecond):
		}
		active.Add(-1)
	}); err != nil {
		t.Fatal(err)
	}

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("slow task did not start")
	}
	for range 8 {
		scheduler.Trigger("slow")
	}
	deadline := time.Now().Add(time.Second)
	for runs.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	if maxActive.Load() != 1 {
		t.Fatalf("maximum concurrent executions = %d, want 1", maxActive.Load())
	}
	if runs.Load() != 2 {
		t.Fatalf("coalesced task executions = %d, want 2", runs.Load())
	}
}

func TestSchedulerRejectsInvalidAndDuplicateTasks(t *testing.T) {
	scheduler := NewScheduler()
	if err := scheduler.Schedule("", time.Second, 0, func(context.Context) {}); !errors.Is(err, errInvalidScheduledTask) {
		t.Fatalf("invalid task error = %v", err)
	}
	if err := scheduler.Schedule("task", time.Second, 0, func(context.Context) {}); err != nil {
		t.Fatal(err)
	}
	if err := scheduler.Schedule("task", time.Second, 0, func(context.Context) {}); !errors.Is(err, errDuplicateScheduledTask) {
		t.Fatalf("duplicate task error = %v", err)
	}
	scheduler.Close()
	if err := scheduler.Schedule("after-close", time.Second, 0, func(context.Context) {}); !errors.Is(err, errSchedulerClosed) {
		t.Fatalf("closed scheduler error = %v", err)
	}
}
