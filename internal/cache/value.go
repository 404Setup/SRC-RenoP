/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package cache

import (
	"time"

	"github.com/goccy/go-json"
	"github.com/llxisdsh/pb"
)

// Value retains either an in-memory value or local invalidation metadata and an encrypted remote reference.
type Value[V any] struct {
	Index    V
	Blob     *Blob
	external bool
}

// NewValue stores a value, preserving only the supplied invalidation projection locally in external mode.
func NewValue[V any](remote *Remote, value V, ttl time.Duration, project func(V) V) Value[V] {
	if remote == nil {
		return Value[V]{Index: value}
	}
	entry := Value[V]{external: true}
	if project != nil {
		entry.Index = project(value)
	}
	if data, err := json.Marshal(value); err == nil {
		entry.Blob, _ = remote.Put(data, ttl)
	}
	return entry
}

// Read decodes the stored value without exposing the local projection as a cache hit.
func (v Value[V]) Read() (V, bool) {
	if !v.external {
		return v.Index, true
	}
	var value V
	data, err := v.Blob.Read()
	if err != nil {
		return value, false
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return value, false
	}
	return value, true
}

// IndexedMap preserves local, enumerable invalidation metadata while offloading cache values.
// The zero value uses memory. Bind must run before concurrent use.
type IndexedMap[K comparable, V any] struct {
	entries pb.MapOf[K, Value[V]]
	remote  *Remote
	project func(V) V
}

// Bind selects external storage and the fields needed for local invalidation.
func (m *IndexedMap[K, V]) Bind(remote *Remote, project func(V) V) {
	m.entries.Range(func(key K, entry Value[V]) bool { m.entries.Delete(key); entry.Blob.Delete(); return true })
	m.remote, m.project = remote, project
}

// Contains reports whether a local reference exists, even if its remote value is unavailable.
func (m *IndexedMap[K, V]) Contains(key K) bool { _, ok := m.entries.Load(key); return ok }

// Load returns a value only while its local reference is still current.
func (m *IndexedMap[K, V]) Load(key K) (V, bool) {
	entry, ok := m.entries.Load(key)
	if !ok {
		var zero V
		return zero, false
	}
	value, ok := entry.Read()
	if entry.external {
		current, exists := m.entries.Load(key)
		ok = ok && exists && current.Blob == entry.Blob
	}
	return value, ok
}

// Store retains a cache value for at most one hour in external storage.
func (m *IndexedMap[K, V]) Store(key K, value V) { m.StoreTTL(key, value, time.Hour) }

// StoreTTL stores one value with the caller's external lifetime.
func (m *IndexedMap[K, V]) StoreTTL(key K, value V, ttl time.Duration) {
	entry := NewValue(m.remote, value, ttl, m.project)
	old, loaded := m.entries.Swap(key, entry)
	if loaded {
		old.Blob.Delete()
	}
}

// LoadAndDelete removes one reference and returns its local invalidation metadata.
func (m *IndexedMap[K, V]) LoadAndDelete(key K) (V, bool) {
	entry, ok := m.entries.LoadAndDelete(key)
	if ok {
		entry.Blob.Delete()
	}
	return entry.Index, ok
}

// Delete removes one reference independently of remote availability.
func (m *IndexedMap[K, V]) Delete(key K) { _, _ = m.LoadAndDelete(key) }

// Range enumerates local invalidation metadata without remote lookups.
func (m *IndexedMap[K, V]) Range(fn func(K, V) bool) {
	m.entries.Range(func(key K, entry Value[V]) bool { return fn(key, entry.Index) })
}

// DeleteMatching invalidates selected references and batches remote deletion.
// A positive limit bounds the removal count. Callers must serialize concurrent writers.
func (m *IndexedMap[K, V]) DeleteMatching(predicate func(K, V) bool, limit int) int {
	var blobs []*Blob
	removed := 0
	m.entries.Range(func(key K, entry Value[V]) bool {
		if predicate(key, entry.Index) {
			if deleted, loaded := m.entries.LoadAndDelete(key); loaded {
				removed++
				if deleted.Blob != nil {
					blobs = append(blobs, deleted.Blob)
				}
			}
		}
		return limit <= 0 || removed < limit
	})
	DeleteBlobs(blobs)
	return removed
}
