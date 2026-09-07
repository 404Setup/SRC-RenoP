/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

// Package cache provides bounded external storage for application cache values.
package cache

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

// Config selects the shared backend for serializable server caches.
type Config struct {
	Mode      string `json:"mode" yaml:"mode"`
	Address   string `json:"address" yaml:"address"`
	Username  string `json:"username" yaml:"username"`
	Password  string `json:"password" yaml:"password"`
	Database  int    `json:"database" yaml:"database"`
	TLS       bool   `json:"tls" yaml:"tls"`
	TimeoutMS int    `json:"timeout_ms" yaml:"timeout_ms"`
}

// Normalize fills defaults without replacing unsupported values.
func (c *Config) Normalize() {
	c.Mode = strings.ToLower(strings.TrimSpace(c.Mode))
	if c.Mode == "" {
		c.Mode = "memory"
	}
	c.Address = strings.TrimSpace(c.Address)
	if c.Address == "" {
		c.Address = "localhost:6379"
	}
	if c.TimeoutMS == 0 {
		c.TimeoutMS = 1000
	}
}

// Validate checks connection settings before persistence or startup.
func (c Config) Validate() error {
	c.Normalize()
	host, port, err := net.SplitHostPort(c.Address)
	portNumber, portErr := strconv.Atoi(port)
	if c.Mode != "memory" && c.Mode != "redis" && c.Mode != "valkey" ||
		err != nil || portErr != nil || host == "" || portNumber < 1 || portNumber > 65535 ||
		len(c.Address) > 512 || strings.ContainsAny(c.Address, "\r\n\t /?#@") ||
		len(c.Username) > 256 || len(c.Password) > 4096 || c.Database < 0 || c.Database > 65535 ||
		c.TimeoutMS < 10 || c.TimeoutMS > 10000 {
		return errors.New("invalid cache settings")
	}
	return nil
}

const maxValueBytes = 2 << 20

// ErrMiss indicates that an external value is unavailable or no longer trusted.
var ErrMiss = errors.New("cache value unavailable")

// Remote owns one bounded connection pool and a process-private encryption key.
type Remote struct {
	client   *redis.Client
	aead     cipher.AEAD
	prefix   string
	sequence atomic.Uint64
	retryAt  atomic.Int64
	timeout  time.Duration
}

// Blob identifies an immutable encrypted value; its reference stays in a local invalidation index.
type Blob struct {
	remote *Remote
	key    string
	size   int
}

// Open connects to Redis or Valkey. Memory mode returns a nil remote backend.
func Open(cfg Config) (*Remote, error) {
	cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if cfg.Mode == "memory" {
		return nil, nil
	}
	timeout := time.Duration(cfg.TimeoutMS) * time.Millisecond
	options := &redis.Options{
		Addr: cfg.Address, Username: cfg.Username, Password: cfg.Password, DB: cfg.Database,
		Protocol: 2, MaxRetries: -1, DialerRetries: 1,
		DialTimeout: timeout, ReadTimeout: timeout, WriteTimeout: timeout, PoolTimeout: timeout,
		ContextTimeoutEnabled: true, PoolSize: 8, MaxActiveConns: 8, MaxIdleConns: 2,
		ReadBufferSize: 4096, WriteBufferSize: 4096, DisableIdentity: true,
	}
	if cfg.TLS {
		options.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	client := redis.NewClient(options)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, errors.New("cache connection failed")
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		_ = client.Close()
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		_ = client.Close()
		return nil, err
	}
	aead, err := cipher.NewGCMWithRandomNonce(block)
	if err != nil {
		_ = client.Close()
		return nil, err
	}
	return &Remote{client: client, aead: aead, prefix: "renop:" + rand.Text() + ":", timeout: timeout}, nil
}

func (r *Remote) operationContext() (context.Context, context.CancelFunc, error) {
	if r.retryAt.Load() > time.Now().UnixMilli() {
		return nil, nil, ErrMiss
	}
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	return ctx, cancel, nil
}

func (r *Remote) failed(err error) error {
	if err != nil && !errors.Is(err, redis.Nil) {
		now := time.Now().UnixMilli()
		if previous := r.retryAt.Load(); previous <= now && r.retryAt.CompareAndSwap(previous, now+5000) {
			log.Print("External cache unavailable; bypassing cache for five seconds")
		}
	}
	return ErrMiss
}

// Put encrypts a bounded value under a new opaque key with a finite lifetime.
func (r *Remote) Put(data []byte, ttl time.Duration) (*Blob, error) {
	if r == nil || len(data) > maxValueBytes || ttl <= 0 {
		return nil, ErrMiss
	}
	ctx, cancel, err := r.operationContext()
	if err != nil {
		return nil, err
	}
	defer cancel()
	sequence := r.sequence.Add(1)
	// Random-nonce GCM allows at most 2^32 messages per key.
	if sequence > 1<<32 {
		return nil, ErrMiss
	}
	key := r.prefix + strconv.FormatUint(sequence, 16)
	sealed := r.aead.Seal(nil, nil, data, []byte(key))
	if err := r.client.Set(ctx, key, sealed, min(ttl, 24*time.Hour)).Err(); err != nil {
		return nil, r.failed(err)
	}
	return &Blob{remote: r, key: key, size: len(sealed)}, nil
}

// Read retrieves and authenticates the exact bounded value named by the reference.
func (b *Blob) Read() ([]byte, error) {
	if b == nil {
		return nil, ErrMiss
	}
	r := b.remote
	ctx, cancel, err := r.operationContext()
	if err != nil {
		return nil, err
	}
	defer cancel()
	sealed, err := r.client.GetRange(ctx, b.key, 0, int64(b.size)).Bytes()
	if err != nil {
		return nil, r.failed(err)
	}
	if len(sealed) != b.size {
		return nil, ErrMiss
	}
	data, err := r.aead.Open(nil, nil, sealed, []byte(b.key))
	if err != nil {
		return nil, ErrMiss
	}
	return data, nil
}

// Delete releases a value. A failed deletion cannot restore its discarded local reference.
func (b *Blob) Delete() {
	if b == nil {
		return
	}
	b.remote.deleteKeys([]string{b.key})
}

func (r *Remote) deleteKeys(keys []string) {
	ctx, cancel, err := r.operationContext()
	if err != nil {
		return
	}
	defer cancel()
	if err := r.client.Del(ctx, keys...).Err(); err != nil {
		r.failed(err)
	}
}

// DeleteBlobs releases values in bounded native DEL batches after local invalidation.
func DeleteBlobs(blobs []*Blob) {
	if len(blobs) == 0 {
		return
	}
	groups := make(map[*Remote][]string)
	for _, blob := range blobs {
		if blob == nil {
			continue
		}
		keys := append(groups[blob.remote], blob.key)
		if len(keys) == 128 {
			blob.remote.deleteKeys(keys)
			keys = keys[:0]
		}
		groups[blob.remote] = keys
	}
	for remote, keys := range groups {
		if len(keys) > 0 {
			remote.deleteKeys(keys)
		}
	}
}

// Close releases the external cache connection pool.
func (r *Remote) Close() error {
	if r == nil {
		return nil
	}
	if err := r.client.Close(); err != nil {
		return fmt.Errorf("close external cache: %w", err)
	}
	return nil
}
