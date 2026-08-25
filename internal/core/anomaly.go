/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package core

import (
	"sync"
	"time"
)

// anomalyFailureTTL matches the previous bigcache LifeWindow for auth-failure counters.
const anomalyFailureTTL = 5 * time.Minute

// maxAnomalyFailureEntries bounds distinct client IPs tracked for 401/403 counters.
const maxAnomalyFailureEntries = 10_000

type anomalyFailure struct {
	Count     uint64
	ExpiresAt int64
}

type AnomalyFailureStore struct {
	mu      sync.Mutex
	entries map[string]anomalyFailure
}

func NewAnomalyFailureStore() *AnomalyFailureStore {
	return &AnomalyFailureStore{
		entries: make(map[string]anomalyFailure),
	}
}

// PruneExpired removes stale authentication failure counters and returns the
// number of entries removed.
func (s *AnomalyFailureStore) PruneExpired() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UnixMilli()
	removed := 0
	for ip, e := range s.entries {
		if e.ExpiresAt <= now {
			delete(s.entries, ip)
			removed++
		}
	}
	return removed
}

// Count returns the live failure count for ip, or 0 if missing/expired.
func (s *AnomalyFailureStore) Count(ip string) uint64 {
	if s == nil || ip == "" {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UnixMilli()
	e, ok := s.entries[ip]
	if !ok {
		return 0
	}
	if e.ExpiresAt <= now {
		delete(s.entries, ip)
		return 0
	}
	return e.Count
}

// Increment bumps the failure counter for ip (refreshing TTL) and returns the new count.
func (s *AnomalyFailureStore) Increment(ip string) uint64 {
	if s == nil || ip == "" {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UnixMilli()
	expire := now + anomalyFailureTTL.Milliseconds()

	if e, ok := s.entries[ip]; ok && e.ExpiresAt > now {
		e.Count++
		e.ExpiresAt = expire
		s.entries[ip] = e
		return e.Count
	}

	if len(s.entries) >= maxAnomalyFailureEntries {
		for k, e := range s.entries {
			if e.ExpiresAt <= now {
				delete(s.entries, k)
			}
		}
		if len(s.entries) >= maxAnomalyFailureEntries {
			for k := range s.entries {
				delete(s.entries, k)
				if len(s.entries) < maxAnomalyFailureEntries*3/4 {
					break
				}
			}
		}
	}

	s.entries[ip] = anomalyFailure{Count: 1, ExpiresAt: expire}
	return 1
}
