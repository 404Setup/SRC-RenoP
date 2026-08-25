/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the terms of the Mozilla Public License, v. 2.0.
 */

// Package tasks coordinates non-reentrant periodic application maintenance.
package tasks

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

var (
	errSchedulerClosed        = errors.New("background scheduler is closed")
	errInvalidScheduledTask   = errors.New("scheduled task requires a unique name, positive interval, and callback")
	errDuplicateScheduledTask = errors.New("scheduled task name already exists")
)

type periodicTask struct {
	name      string
	interval  time.Duration
	next      time.Time
	run       func(context.Context)
	running   atomic.Bool
	triggered atomic.Bool
}

// Scheduler coordinates all coalescible periodic application work from one
// timer loop while running each task non-reentrantly.
type Scheduler struct {
	mu       sync.Mutex
	tasks    map[string]*periodicTask
	wake     chan struct{}
	stop     chan struct{}
	done     chan struct{}
	ctx      context.Context
	cancel   context.CancelFunc
	closed   atomic.Bool
	stopOnce sync.Once
	workers  sync.WaitGroup
}

// NewScheduler starts an empty process-wide periodic scheduler.
func NewScheduler() *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())
	scheduler := &Scheduler{
		tasks:  make(map[string]*periodicTask),
		wake:   make(chan struct{}, 1),
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
		ctx:    ctx,
		cancel: cancel,
	}
	go scheduler.loop()
	return scheduler
}

// Schedule registers a non-reentrant task. The first execution occurs after
// initialDelay; subsequent executions retain interval cadence without catch-up bursts.
func (scheduler *Scheduler) Schedule(
	name string,
	interval time.Duration,
	initialDelay time.Duration,
	run func(context.Context),
) error {
	if scheduler == nil || scheduler.closed.Load() {
		return errSchedulerClosed
	}
	if name == "" || interval <= 0 || run == nil {
		return errInvalidScheduledTask
	}
	if initialDelay < 0 {
		initialDelay = 0
	}
	task := &periodicTask{name: name, interval: interval, next: time.Now().Add(initialDelay), run: run}
	scheduler.mu.Lock()
	if scheduler.closed.Load() {
		scheduler.mu.Unlock()
		return errSchedulerClosed
	}
	if _, exists := scheduler.tasks[name]; exists {
		scheduler.mu.Unlock()
		return errDuplicateScheduledTask
	}
	scheduler.tasks[name] = task
	scheduler.mu.Unlock()
	scheduler.notify()
	return nil
}

// Trigger advances a registered task to the next scheduler turn. Repeated
// triggers coalesce, and a currently running task is never entered again.
func (scheduler *Scheduler) Trigger(name string) bool {
	if scheduler == nil || scheduler.closed.Load() {
		return false
	}
	scheduler.mu.Lock()
	task := scheduler.tasks[name]
	if task != nil {
		task.triggered.Store(true)
		if !task.running.Load() {
			task.next = time.Now()
		}
	}
	scheduler.mu.Unlock()
	if task == nil {
		return false
	}
	scheduler.notify()
	return true
}

// Done returns a channel closed after the timer loop and active task callbacks stop.
func (scheduler *Scheduler) Done() <-chan struct{} {
	if scheduler == nil {
		closed := make(chan struct{})
		close(closed)
		return closed
	}
	return scheduler.done
}

// Close cancels the scheduler and waits for active callbacks to return.
func (scheduler *Scheduler) Close() {
	if scheduler == nil {
		return
	}
	scheduler.stopOnce.Do(func() {
		scheduler.mu.Lock()
		scheduler.closed.Store(true)
		scheduler.cancel()
		close(scheduler.stop)
		scheduler.mu.Unlock()
	})
	<-scheduler.done
}

func (scheduler *Scheduler) notify() {
	select {
	case scheduler.wake <- struct{}{}:
	default:
	}
}

func (scheduler *Scheduler) loop() {
	defer close(scheduler.done)
	for {
		next, available := scheduler.nextRun()
		if !available {
			select {
			case <-scheduler.stop:
				scheduler.workers.Wait()
				return
			case <-scheduler.wake:
				continue
			}
		}

		delay := time.Until(next)
		if delay < 0 {
			delay = 0
		}
		timer := time.NewTimer(delay)
		select {
		case <-scheduler.stop:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			scheduler.workers.Wait()
			return
		case <-scheduler.wake:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			continue
		case now := <-timer.C:
			for _, task := range scheduler.takeDue(now) {
				scheduler.startTask(task)
			}
		}
	}
}

func (scheduler *Scheduler) nextRun() (time.Time, bool) {
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	var next time.Time
	for _, task := range scheduler.tasks {
		if next.IsZero() || task.next.Before(next) {
			next = task.next
		}
	}
	return next, !next.IsZero()
}

func (scheduler *Scheduler) takeDue(now time.Time) []*periodicTask {
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	due := make([]*periodicTask, 0, len(scheduler.tasks))
	for _, task := range scheduler.tasks {
		if task.next.After(now) {
			continue
		}
		elapsed := now.Sub(task.next)
		steps := elapsed/task.interval + 1
		task.next = task.next.Add(steps * task.interval)
		due = append(due, task)
	}
	return due
}

func (scheduler *Scheduler) startTask(task *periodicTask) {
	if task == nil || scheduler.closed.Load() {
		return
	}
	scheduler.mu.Lock()
	if scheduler.closed.Load() || !task.running.CompareAndSwap(false, true) {
		scheduler.mu.Unlock()
		return
	}
	task.triggered.Store(false)
	scheduler.mu.Unlock()
	if scheduler.closed.Load() {
		task.running.Store(false)
		return
	}
	scheduler.workers.Add(1)
	go func() {
		defer scheduler.workers.Done()
		defer scheduler.finishTask(task)
		task.run(scheduler.ctx)
	}()
}

func (scheduler *Scheduler) finishTask(task *periodicTask) {
	task.running.Store(false)
	if !task.triggered.Swap(false) || scheduler.closed.Load() {
		return
	}
	scheduler.mu.Lock()
	if !scheduler.closed.Load() {
		task.next = time.Now()
	}
	scheduler.mu.Unlock()
	scheduler.notify()
}
