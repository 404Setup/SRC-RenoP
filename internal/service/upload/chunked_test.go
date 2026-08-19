/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package upload

import (
	"bytes"
	"errors"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestCreateSessionAndParallelWriteChunks(t *testing.T) {
	mgr := &Manager{sessions: make(map[string]*Session)}
	dir := t.TempDir()
	dest := filepath.Join(dir, "artifact.bin")

	const chunkSize = MinChunkSize
	const total = chunkSize * 4

	sess, err := mgr.CreateSession(PurposeStorage, "artifact.bin", "alice", total, chunkSize, false, dest, "releases")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if sess.ChunkCount != 4 {
		t.Fatalf("chunk count = %d, want 4 (size=%d count=%d)", sess.ChunkCount, sess.ChunkSize, sess.ChunkCount)
	}
	if sess.ChunkSize != chunkSize {
		t.Fatalf("chunk size = %d, want %d", sess.ChunkSize, chunkSize)
	}

	payload := make([]byte, total)
	for i := range payload {
		payload[i] = byte(i % 251)
	}

	var wg sync.WaitGroup
	errCh := make(chan error, sess.ChunkCount)
	for i := 0; i < sess.ChunkCount; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			start := int64(index) * sess.ChunkSize
			end := min(start+sess.ChunkSize, total)
			part := payload[start:end]
			if err := sess.WriteChunk(index, bytes.NewReader(part), int64(len(part))); err != nil {
				errCh <- err
			}
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("WriteChunk: %v", err)
	}

	if !sess.AllReceived() {
		t.Fatal("expected all chunks received")
	}
	if err := sess.CloseFile(); err != nil {
		t.Fatalf("CloseFile: %v", err)
	}

	got, err := os.ReadFile(sess.TempPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("assembled content mismatch (len got=%d want=%d)", len(got), len(payload))
	}

	path := sess.TempPath
	sess.Abort()
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("temp file should be removed after Abort")
	}
}

func TestWriteChunkIdempotentRetry(t *testing.T) {
	mgr := &Manager{sessions: make(map[string]*Session)}
	dir := t.TempDir()
	dest := filepath.Join(dir, "idem.bin")

	const part = MinChunkSize
	const total = part * 2
	sess, err := mgr.CreateSession(PurposeStorage, "idem.bin", "dave", total, part, false, dest, "releases")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	defer sess.Abort()

	buf := make([]byte, part)
	if err := sess.WriteChunk(0, bytes.NewReader(buf), part); err != nil {
		t.Fatalf("first WriteChunk: %v", err)
	}
	if err := sess.WriteChunk(0, bytes.NewReader(buf), part); err != nil {
		t.Fatalf("retry WriteChunk: %v", err)
	}
}

func TestWriteChunkRejectsWrongSize(t *testing.T) {
	mgr := &Manager{sessions: make(map[string]*Session)}
	dir := t.TempDir()
	dest := filepath.Join(dir, "a.bin")

	const part = MinChunkSize
	const total = part * 2
	sess, err := mgr.CreateSession(PurposeStorage, "a.bin", "bob", total, part, false, dest, "releases")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	defer sess.Abort()

	if err := sess.WriteChunk(0, bytes.NewReader(make([]byte, 10)), 10); err == nil {
		t.Fatal("expected size mismatch error")
	}
}

func TestZeroByteSession(t *testing.T) {
	mgr := &Manager{sessions: make(map[string]*Session)}
	dir := t.TempDir()
	dest := filepath.Join(dir, "empty.bin")

	sess, err := mgr.CreateSession(PurposeStorage, "empty.bin", "carol", 0, DefaultChunkSize, false, dest, "releases")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	defer sess.Abort()

	if sess.ChunkCount != 0 {
		t.Fatalf("chunk count = %d, want 0", sess.ChunkCount)
	}
	if !sess.AllReceived() {
		t.Fatal("empty session should report all received")
	}
}

func TestSuggestChunkSize(t *testing.T) {
	cases := []struct {
		size int64
		want int64
	}{
		{size: 0, want: DefaultChunkSize},
		{size: 100, want: 100},
		{size: 8 << 20, want: 8 << 20},
		{size: 8<<20 + 1, want: 4 << 20},
		{size: 32 << 20, want: 4 << 20},
		{size: 32<<20 + 1, want: 8 << 20},
		{size: 128 << 20, want: 8 << 20},
		{size: 128<<20 + 1, want: 16 << 20},
		{size: 512 << 20, want: 16 << 20},
		{size: 512<<20 + 1, want: 24 << 20},
		{size: 2 << 30, want: 24 << 20},
		{size: 2<<30 + 1, want: MaxChunkSize},
	}
	for _, tc := range cases {
		got := SuggestChunkSize(tc.size)
		if got != tc.want {
			t.Fatalf("SuggestChunkSize(%d) = %d, want %d", tc.size, got, tc.want)
		}
	}
}

func TestNormalizeChunkSize(t *testing.T) {
	if got := NormalizeChunkSize(100, 4<<20); got != 100 {
		t.Fatalf("tiny file: got %d want 100", got)
	}
	if got := NormalizeChunkSize(64<<20, 0); got != SuggestChunkSize(64<<20) {
		t.Fatalf("auto size: got %d want %d", got, SuggestChunkSize(64<<20))
	}
	if got := NormalizeChunkSize(1<<30, 64<<20); got != MaxChunkSize {
		t.Fatalf("cap: got %d want %d", got, MaxChunkSize)
	}
	if got := NormalizeChunkSize(64<<20, 1024); got != MinChunkSize {
		t.Fatalf("min multi-part: got %d want %d", got, MinChunkSize)
	}
	got := NormalizeChunkSize(2<<30, MinChunkSize)
	if n := (2<<30 + got - 1) / got; n > maxPreferredChunks {
		t.Fatalf("still too many parts: size=%d count=%d", got, n)
	}
	if got := NormalizeChunkSize(math.MaxInt64, MinChunkSize); got != MaxChunkSize {
		t.Fatalf("max int size: got %d want %d", got, MaxChunkSize)
	}
}

func TestCreateSessionCountsPendingReservations(t *testing.T) {
	mgr := &Manager{
		sessions: make(map[string]*Session),
		pending:  MaxSessions,
	}
	_, err := mgr.CreateSession(PurposeUpdater, "update.zip", "alice", 0, DefaultChunkSize, false, "", "")
	if err == nil || err.Error() != "too many concurrent uploads" {
		t.Fatalf("CreateSession error = %v, want session limit error", err)
	}
	if mgr.pending != MaxSessions {
		t.Fatalf("pending reservations = %d, want %d", mgr.pending, MaxSessions)
	}
}

func TestCreateSessionRejectsOversizedFilename(t *testing.T) {
	mgr := &Manager{sessions: make(map[string]*Session)}
	filename := string(bytes.Repeat([]byte{'x'}, maxUploadMetadataLength+1))
	_, err := mgr.CreateSession(PurposeUpdater, filename, "alice", 0, DefaultChunkSize, false, "", "")
	if err == nil || err.Error() != "filename is too long" {
		t.Fatalf("CreateSession error = %v, want filename length error", err)
	}
	if len(mgr.sessions) != 0 || mgr.pending != 0 {
		t.Fatalf("oversized metadata must not reserve a session: sessions=%d pending=%d", len(mgr.sessions), mgr.pending)
	}
}

func TestSessionOwnershipRequiresExactNonEmptyOwner(t *testing.T) {
	if (&Session{}).OwnedBy("alice") {
		t.Fatal("an ownerless session must not be claimable")
	}
	if !(&Session{owner: "alice"}).OwnedBy("alice") {
		t.Fatal("the exact owner should own the session")
	}
	if (&Session{owner: "alice"}).OwnedBy("bob") {
		t.Fatal("a different user must not own the session")
	}
}

func TestBeginCompletionHasSingleWinner(t *testing.T) {
	sess := &Session{owner: "alice"}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- sess.BeginCompletion()
		}()
	}
	wg.Wait()
	close(results)

	var succeeded, rejected int
	for err := range results {
		if err == nil {
			succeeded++
		} else if errors.Is(err, errSessionFinished) {
			rejected++
		} else {
			t.Fatalf("unexpected result: %v", err)
		}
	}
	if succeeded != 1 || rejected != 1 {
		t.Fatalf("succeeded=%d rejected=%d, want one of each", succeeded, rejected)
	}
}

func TestManagerCompletionAndAbortAreMutuallyExclusive(t *testing.T) {
	mgr := &Manager{sessions: map[string]*Session{
		"id": {ID: "id", owner: "alice"},
	}}
	start := make(chan struct{})
	results := make(chan bool, 2)
	go func() {
		<-start
		_, err := mgr.BeginSessionCompletion("id", "alice")
		results <- err == nil
	}()
	go func() {
		<-start
		_, owned := mgr.AbortOwned("id", "alice")
		results <- owned
	}()
	close(start)

	winners := 0
	for range 2 {
		if <-results {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("completion/abort winners = %d, want exactly one", winners)
	}
}

func TestWriteChunkBytes(t *testing.T) {
	mgr := &Manager{sessions: make(map[string]*Session)}
	dir := t.TempDir()
	dest := filepath.Join(dir, "bytes.bin")

	const chunkSize = MinChunkSize
	const total = chunkSize*2 + 123
	sess, err := mgr.CreateSession(PurposeStorage, "bytes.bin", "eve", total, chunkSize, false, dest, "releases")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	defer sess.Abort()
	if sess.ChunkCount != 3 {
		t.Fatalf("chunk count = %d, want 3", sess.ChunkCount)
	}

	payload := make([]byte, total)
	for i := range payload {
		payload[i] = byte(i)
	}
	for i := 0; i < sess.ChunkCount; i++ {
		start := int64(i) * sess.ChunkSize
		end := min(start+sess.ChunkSize, total)
		if err := sess.WriteChunkBytes(i, payload[start:end]); err != nil {
			t.Fatalf("WriteChunkBytes %d: %v", i, err)
		}
	}
	if !sess.AllReceived() {
		t.Fatal("expected all chunks")
	}
	if err := sess.CloseFile(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(sess.TempPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("content mismatch")
	}
}

func TestSmallFileSinglePart(t *testing.T) {
	mgr := &Manager{sessions: make(map[string]*Session)}
	dir := t.TempDir()
	dest := filepath.Join(dir, "small.bin")

	const total = int64(50 * 1024)
	sess, err := mgr.CreateSession(PurposeStorage, "small.bin", "frank", total, DefaultChunkSize, false, dest, "releases")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	defer sess.Abort()
	if sess.ChunkCount != 1 {
		t.Fatalf("chunk count = %d, want 1 for small file", sess.ChunkCount)
	}
	if sess.ChunkSize != total {
		t.Fatalf("chunk size = %d, want total %d", sess.ChunkSize, total)
	}
}
