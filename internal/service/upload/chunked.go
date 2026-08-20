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
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	syncv2 "sync/v2"
	"time"

	"github.com/google/uuid"
)

const (
	// MinChunkSize is the smallest multi-part size (avoids tiny-part thrash).
	// Files at or below this size become a single part when forced through the API.
	MinChunkSize int64 = 256 * 1024

	// DefaultChunkSize is used when size is unknown and no client preference is set.
	DefaultChunkSize int64 = 4 * 1024 * 1024

	// MaxChunkSize caps a single part body (32 MiB).
	MaxChunkSize int64 = 32 * 1024 * 1024

	// maxPreferredChunks raises part size when a request would create too many parts.
	maxPreferredChunks int64 = 2048

	// SessionTTL for abandoned multipart sessions (max idle lifetime).
	SessionTTL = 15 * time.Minute

	// MaxSessions bounds concurrent in-memory sessions.
	MaxSessions = 256

	// maxUploadMetadataLength bounds client-controlled strings retained by a
	// session.  A chunked session may live for SessionTTL, so accepting the
	// full protobuf body for a filename would amplify a small request into
	// substantial process memory.
	maxUploadMetadataLength = 4 * 1024

	// chunkIOBufSize is the pooled read buffer for streamed part bodies.
	chunkIOBufSize = 128 * 1024

	PurposeStorage = "storage"
	PurposeUpdater = "updater"
)

// chunkBufPool reuses read buffers across concurrent WriteChunk calls.
var chunkBufPool = syncv2.Pool[*[]byte]{
	New: func() *[]byte {
		b := make([]byte, chunkIOBufSize)
		return &b
	},
}

// SuggestChunkSize picks a part size from total file size.
// Larger files get larger parts so HTTP/TLS overhead stays low and disk writes are sequential-friendly.
func SuggestChunkSize(totalSize int64) int64 {
	switch {
	case totalSize <= 0:
		return DefaultChunkSize
	case totalSize <= 8<<20:
		return totalSize
	case totalSize <= 32<<20:
		return 4 << 20
	case totalSize <= 128<<20:
		return 8 << 20
	case totalSize <= 512<<20:
		return 16 << 20
	case totalSize <= 2<<30:
		return 24 << 20
	default:
		return MaxChunkSize
	}
}

// NormalizeChunkSize clamps a client preference (or 0) to a safe, performant part size.
func NormalizeChunkSize(totalSize, requested int64) int64 {
	if totalSize <= 0 {
		if requested > 0 {
			if requested > MaxChunkSize {
				return MaxChunkSize
			}
			if requested < MinChunkSize {
				return MinChunkSize
			}
			return requested
		}
		return DefaultChunkSize
	}

	if totalSize <= MinChunkSize {
		return totalSize
	}

	size := requested
	if size <= 0 {
		size = SuggestChunkSize(totalSize)
	}
	if size > MaxChunkSize {
		size = MaxChunkSize
	}
	if size < MinChunkSize {
		size = MinChunkSize
	}
	if size >= totalSize {
		return totalSize
	}

	if n := 1 + (totalSize-1)/size; n > maxPreferredChunks {
		size = max(min(1+(totalSize-1)/maxPreferredChunks, MaxChunkSize), MinChunkSize)
		if size >= totalSize {
			return totalSize
		}
	}
	return size
}

// Session is an in-progress multi-part upload.
type Session struct {
	created       time.Time
	file          *os.File
	ID            string
	Purpose       string
	Filename      string
	LocalFilePath string
	RepoName      string
	TempPath      string
	owner         string

	// received[i]: 0 free, 1 writing, 2 done
	received []int32

	TotalSize         int64
	ChunkSize         int64
	ChunkCount        int
	mu                sync.Mutex
	done              int32
	closed            bool
	aborting          bool
	GenerateChecksums bool
	SignatureExpected bool
}

// Manager holds active chunked upload sessions.
type Manager struct {
	mu       sync.Mutex
	sessions map[string]*Session
	pending  int
	once     sync.Once
}

var defaultManager = &Manager{
	sessions: make(map[string]*Session),
}

// DefaultManager returns the process-wide chunked upload manager.
func DefaultManager() *Manager {
	return defaultManager
}

// StartBackgroundCleanup launches orphan-temp sweeps and session expiry.
// storagePath is the Maven storage root (may be empty to skip storage walks).
// Safe to call multiple times; only the first call starts the worker.
func StartBackgroundCleanup(storagePath string) {
	defaultManager.once.Do(func() {
		go func() {
			CleanupOrphanPartials(storagePath, 0)
			_ = CleanupStaleOSTempUploads(0, true)
		}()

		go func() {
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()
			for range ticker.C {
				defaultManager.CleanupExpired()
				_ = CleanupOrphanPartials(storagePath, SessionTTL)
				_ = CleanupStaleOSTempUploads(SessionTTL, false)
			}
		}()
	})
}

// CreateSession allocates a pre-sized temp file and registers the session.
func (m *Manager) CreateSession(
	purpose, filename, owner string,
	totalSize, chunkSize int64,
	generateChecksums bool,
	localFilePath, repoName string,
) (*Session, error) {
	if totalSize < 0 {
		return nil, errors.New("invalid size")
	}
	if len(filename) > maxUploadMetadataLength {
		return nil, errors.New("filename is too long")
	}
	if owner == "" {
		return nil, errors.New("missing upload owner")
	}
	chunkSize = NormalizeChunkSize(totalSize, chunkSize)

	var chunkCount64 int64
	if totalSize > 0 {
		chunkCount64 = 1 + (totalSize-1)/chunkSize
	}
	if chunkCount64 > 100_000 {
		return nil, errors.New("too many chunks")
	}
	chunkCount := int(chunkCount64)

	m.mu.Lock()
	if len(m.sessions)+m.pending >= MaxSessions {
		m.mu.Unlock()
		return nil, errors.New("too many concurrent uploads")
	}
	m.pending++
	m.mu.Unlock()
	reserved := true
	defer func() {
		if reserved {
			m.mu.Lock()
			m.pending--
			m.mu.Unlock()
		}
	}()

	id := uuid.New().String()
	var tempPath string
	var file *os.File
	var err error

	switch purpose {
	case PurposeStorage:
		if localFilePath == "" {
			return nil, errors.New("missing destination path")
		}
		parent := filepath.Dir(localFilePath)
		if err := os.MkdirAll(parent, 0755); err != nil {
			return nil, err
		}
		tempPath = localFilePath + ".chunk." + id
		file, err = os.Create(tempPath)
	case PurposeUpdater:
		file, err = os.CreateTemp("", "renop-chunk-upload-*.zip")
		if err == nil {
			tempPath = file.Name()
		}
	default:
		return nil, errors.New("invalid purpose")
	}
	if err != nil {
		return nil, err
	}

	if totalSize > 0 {
		if err := file.Truncate(totalSize); err != nil {
			_ = file.Close()
			_ = os.Remove(tempPath)
			return nil, err
		}
	}

	sess := &Session{
		ID:                id,
		Purpose:           purpose,
		Filename:          filename,
		TotalSize:         totalSize,
		ChunkSize:         chunkSize,
		ChunkCount:        chunkCount,
		GenerateChecksums: generateChecksums,
		LocalFilePath:     localFilePath,
		RepoName:          repoName,
		TempPath:          tempPath,
		file:              file,
		received:          make([]int32, chunkCount),
		created:           time.Now(),
		owner:             owner,
	}

	m.mu.Lock()
	m.pending--
	reserved = false
	if m.sessions == nil {
		m.sessions = make(map[string]*Session)
	}
	m.sessions[id] = sess
	m.mu.Unlock()
	return sess, nil
}

// Get returns a session by id.
func (m *Manager) Get(id string) (*Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	return s, ok
}

// CleanupExpired aborts sessions older than SessionTTL.
func (m *Manager) CleanupExpired() {
	now := time.Now()
	m.mu.Lock()
	var expired []*Session
	for id, s := range m.sessions {
		if now.Sub(s.created) > SessionTTL {
			expired = append(expired, s)
			delete(m.sessions, id)
		}
	}
	m.mu.Unlock()
	for _, s := range expired {
		s.Abort()
	}
}

// claimChunk validates index/size and marks the slot as in-progress.
// Returns offset, expected length, and the open file handle.
func (s *Session) claimChunk(index int, contentLength int64) (offset, expected int64, file *os.File, err error) {
	if atomic.LoadInt32(&s.done) != 0 {
		return 0, 0, nil, errors.New("session already completed")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.aborting {
		return 0, 0, nil, errors.New("session closed")
	}
	if index < 0 || (s.ChunkCount > 0 && index >= s.ChunkCount) {
		return 0, 0, nil, errors.New("invalid chunk index")
	}
	if s.ChunkCount == 0 {
		return 0, 0, nil, errors.New("no chunks expected for empty file")
	}
	if s.received[index] == 2 {
		return 0, 0, nil, errChunkAlreadyDone
	}
	if s.received[index] == 1 {
		return 0, 0, nil, errors.New("chunk already in progress")
	}

	offset = int64(index) * s.ChunkSize
	expected = s.ChunkSize
	if index == s.ChunkCount-1 {
		expected = s.TotalSize - offset
	}
	if contentLength >= 0 && contentLength != expected {
		return 0, 0, nil, fmt.Errorf("unexpected chunk size: want %d got %d", expected, contentLength)
	}
	s.received[index] = 1
	return offset, expected, s.file, nil
}

var errChunkAlreadyDone = errors.New("chunk already done")

var (
	errChunksIncomplete = errors.New("not all chunks received")
	errSessionFinished  = errors.New("session already completed")
	errSessionNotFound  = errors.New("upload session not found")
	errSessionNotOwned  = errors.New("upload session owned by another user")
)

// BeginSessionCompletion atomically removes a complete, owned session from the
// manager so cancellation cannot delete its temp file during finalization.
func (m *Manager) BeginSessionCompletion(id, owner string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sess, ok := m.sessions[id]
	if !ok {
		return nil, errSessionNotFound
	}
	if !sess.OwnedBy(owner) {
		return nil, errSessionNotOwned
	}
	if err := sess.BeginCompletion(); err != nil {
		return nil, err
	}
	delete(m.sessions, id)
	return sess, nil
}

// AbortOwned atomically claims an owned session for cancellation.
func (m *Manager) AbortOwned(id, owner string) (found, owned bool) {
	m.mu.Lock()
	sess, ok := m.sessions[id]
	if !ok {
		m.mu.Unlock()
		return false, false
	}
	if !sess.OwnedBy(owner) {
		m.mu.Unlock()
		return true, false
	}
	delete(m.sessions, id)
	m.mu.Unlock()

	sess.Abort()
	return true, true
}

func (s *Session) releaseClaim(index int) {
	s.mu.Lock()
	if s.received[index] == 1 {
		s.received[index] = 0
	}
	s.mu.Unlock()
}

func (s *Session) finishChunk(index int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.aborting {
		s.received[index] = 0
		return errors.New("session closed")
	}
	s.received[index] = 2
	return nil
}

// WriteChunkBytes writes one part from an in-memory buffer (single WriteAt).
func (s *Session) WriteChunkBytes(index int, data []byte) error {
	contentLength := int64(len(data))
	offset, expected, file, err := s.claimChunk(index, contentLength)
	if err != nil {
		if errors.Is(err, errChunkAlreadyDone) {
			return nil
		}
		return err
	}
	if contentLength != expected {
		s.releaseClaim(index)
		return fmt.Errorf("unexpected chunk size: want %d got %d", expected, contentLength)
	}

	n, werr := file.WriteAt(data, offset)
	if werr != nil {
		s.releaseClaim(index)
		return werr
	}
	if int64(n) != expected {
		s.releaseClaim(index)
		return fmt.Errorf("short chunk: want %d got %d", expected, n)
	}
	return s.finishChunk(index)
}

// WriteChunk writes one part at the expected offset. index is 0-based.
// Prefer WriteChunkBytes when the full body is already buffered.
func (s *Session) WriteChunk(index int, r io.Reader, contentLength int64) error {
	offset, expected, file, err := s.claimChunk(index, contentLength)
	if err != nil {
		if errors.Is(err, errChunkAlreadyDone) {
			return nil
		}
		return err
	}

	bufPtr := chunkBufPool.Get()
	buf := *bufPtr
	defer chunkBufPool.Put(bufPtr)

	lr := io.LimitReader(r, expected)
	w := io.NewOffsetWriter(file, offset)
	written, copyErr := io.CopyBuffer(w, lr, buf)
	if copyErr != nil {
		s.releaseClaim(index)
		return copyErr
	}
	if written != expected {
		s.releaseClaim(index)
		return fmt.Errorf("short chunk: want %d got %d", expected, written)
	}
	return s.finishChunk(index)
}

// AllReceived reports whether every chunk index was accepted.
func (s *Session) AllReceived() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, st := range s.received {
		if st != 2 {
			return false
		}
	}
	return true
}

// BeginCompletion grants exactly one caller ownership of finalization. If a
// chunk is still being written, the caller may retry after it finishes.
func (s *Session) BeginCompletion() error {
	if !atomic.CompareAndSwapInt32(&s.done, 0, 1) {
		return errSessionFinished
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.aborting {
		return errSessionFinished
	}
	for _, st := range s.received {
		if st != 2 {
			atomic.StoreInt32(&s.done, 0)
			return errChunksIncomplete
		}
	}
	return nil
}

// CloseFile flushes and closes the backing temp file without deleting it.
func (s *Session) CloseFile() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file == nil {
		return nil
	}
	err := s.file.Close()
	s.file = nil
	return err
}

// Abort closes and deletes the temp file. Safe to call multiple times.
func (s *Session) Abort() {
	s.mu.Lock()
	if s.aborting {
		s.mu.Unlock()
		return
	}
	s.aborting = true
	file := s.file
	s.file = nil
	path := s.TempPath
	s.TempPath = ""
	s.closed = true
	s.mu.Unlock()

	if file != nil {
		_ = file.Close()
	}
	if path != "" {
		_ = os.Remove(path)
	}
	atomic.StoreInt32(&s.done, 1)
}

// MarkCompleted marks the session finished (temp may have been renamed away).
func (s *Session) MarkCompleted() {
	s.mu.Lock()
	s.closed = true
	if s.file != nil {
		_ = s.file.Close()
		s.file = nil
	}
	s.mu.Unlock()
	atomic.StoreInt32(&s.done, 1)
}

// OwnedBy reports whether the session belongs to the given username.
func (s *Session) OwnedBy(username string) bool {
	return s.owner != "" && s.owner == username
}
