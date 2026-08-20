/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package gpg

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"path"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/ProtonMail/go-crypto/openpgp"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/outboundproxy"
	"renop/internal/utils"
)

const (
	MaxKeysPerUser       = 10
	MaxKeyServers        = 8
	MaxPublicKeySize     = 2 * 1024 * 1024
	PublicKeyCacheTTL    = 24 * time.Hour
	keyFetchTotalTimeout = 18 * time.Second
	keyFetchTimeout      = 6 * time.Second
	maxPinnedAddresses   = 4
	pinnedDialTimeout    = 4 * time.Second
)

var (
	errInvalidKeyReference = errors.New("invalid GPG key ID or fingerprint")
	errKeyNotFound         = errors.New("GPG key was not found on configured key servers")
	errAmbiguousKey        = errors.New("GPG key ID matches multiple public keys; use a full fingerprint")
	keyFetchSemaphore      = make(chan struct{}, 8)
)

var nonPublicAddressRanges = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
}

func isPublicIP(ip net.IP) bool {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	addr = addr.Unmap()
	if !addr.IsGlobalUnicast() || addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() {
		return false
	}
	for _, prefix := range nonPublicAddressRanges {
		if prefix.Contains(addr) {
			return false
		}
	}
	return true
}

func NormalizeKeyReference(value string) (string, error) {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(strings.TrimPrefix(value, "0x"), "0X")
	value = strings.ReplaceAll(value, " ", "")
	if len(value) != 16 && len(value) != 40 && len(value) != 64 {
		return "", errInvalidKeyReference
	}
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded)*2 != len(value) {
		return "", errInvalidKeyReference
	}
	return strings.ToUpper(value), nil
}

func normalizeFingerprint(value string) (string, error) {
	value, err := NormalizeKeyReference(value)
	if err != nil || (len(value) != 40 && len(value) != 64) {
		return "", errInvalidKeyReference
	}
	return value, nil
}

func ValidateKeyServers(servers []string) ([]string, error) {
	if len(servers) == 0 {
		return nil, errors.New("at least one GPG key server is required")
	}
	if len(servers) > MaxKeyServers {
		return nil, fmt.Errorf("at most %d GPG key servers are allowed", MaxKeyServers)
	}
	normalized := make([]string, 0, len(servers))
	seen := make(map[string]struct{}, len(servers))
	for _, raw := range servers {
		raw = strings.TrimSpace(raw)
		if raw == "" || len(raw) > 512 || strings.IndexFunc(raw, unicode.IsSpace) >= 0 {
			return nil, errors.New("invalid GPG key server URL")
		}
		parsed, err := url.ParseRequestURI(raw)
		if err != nil || !strings.EqualFold(parsed.Scheme, "https") || parsed.Host == "" || parsed.Hostname() == "" {
			return nil, errors.New("GPG key server URLs must use HTTPS")
		}
		if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
			return nil, errors.New("GPG key server URLs must contain only an HTTPS origin")
		}
		parsed.Scheme = "https"
		parsed.Path = ""
		value := strings.TrimSuffix(parsed.String(), "/")
		key := strings.ToLower(value)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, value)
	}
	if len(normalized) == 0 {
		return nil, errors.New("at least one GPG key server is required")
	}
	return normalized, nil
}

func ArtifactKey(repository, artifactPath string) string {
	artifactPath = strings.TrimPrefix(filepathSlashClean(artifactPath), "/")
	digest := sha256.Sum256([]byte(repository + "\x00" + artifactPath))
	return hex.EncodeToString(digest[:])
}

func filepathSlashClean(value string) string {
	cleaned := path.Clean(strings.ReplaceAll(value, "\\", "/"))
	if cleaned == "." {
		return ""
	}
	return cleaned
}

// IsProtectedArtifact reports whether OpenPGP policy applies to a Maven file.
// Detached signatures and checksum companions are deliberately excluded.
func IsProtectedArtifact(artifactPath string) bool {
	lower := strings.ToLower(filepathSlashClean(artifactPath))
	return strings.HasSuffix(lower, ".jar") || strings.HasSuffix(lower, ".pom") || strings.HasSuffix(lower, ".module")
}

// ArtifactForDetachedSignature maps foo.jar.asc to foo.jar when the base file
// is one of the Maven artifact types covered by GPG policy.
func ArtifactForDetachedSignature(signaturePath string) (string, bool) {
	cleaned := filepathSlashClean(signaturePath)
	if !strings.HasSuffix(strings.ToLower(cleaned), ".asc") {
		return "", false
	}
	artifactPath := cleaned[:len(cleaned)-len(".asc")]
	return artifactPath, IsProtectedArtifact(artifactPath)
}

func entityAliases(entity *openpgp.Entity) []string {
	if entity == nil || entity.PrimaryKey == nil {
		return nil
	}
	aliases := make([]string, 0, 2+len(entity.Subkeys)*2)
	appendKey := func(fingerprint []byte, keyID uint64) {
		aliases = append(aliases, strings.ToUpper(hex.EncodeToString(fingerprint)))
		aliases = append(aliases, fmt.Sprintf("%016X", keyID))
	}
	appendKey(entity.PrimaryKey.Fingerprint, entity.PrimaryKey.KeyId)
	for i := range entity.Subkeys {
		if entity.Subkeys[i].PublicKey != nil {
			appendKey(entity.Subkeys[i].PublicKey.Fingerprint, entity.Subkeys[i].PublicKey.KeyId)
		}
	}
	return aliases
}

func entityMatchesReference(entity *openpgp.Entity, reference string) bool {
	return slices.Contains(entityAliases(entity), reference)
}

func entityContainsPrivateKey(entity *openpgp.Entity) bool {
	if entity == nil {
		return false
	}
	if entity.PrivateKey != nil {
		return true
	}
	for i := range entity.Subkeys {
		if entity.Subkeys[i].PrivateKey != nil {
			return true
		}
	}
	return false
}

func entityToPublicKey(entity *openpgp.Entity, fetchedAt time.Time) (*core.GPGPublicKey, []string, error) {
	if entity == nil || entity.PrimaryKey == nil || entityContainsPrivateKey(entity) {
		return nil, nil, errors.New("invalid public key response")
	}
	now := time.Now()
	if !entityHasAcceptedSigningKey(entity, now) {
		return nil, nil, errors.New("GPG key is revoked, expired, or does not have an accepted signing key")
	}
	var serialized bytes.Buffer
	if err := entity.Serialize(&serialized); err != nil {
		return nil, nil, fmt.Errorf("failed to serialize GPG public key: %w", err)
	}
	if serialized.Len() == 0 || serialized.Len() > MaxPublicKeySize {
		return nil, nil, errors.New("GPG public key exceeds the size limit")
	}
	identity := ""
	primarySelfSignature, _ := entity.PrimarySelfSignature()
	if primary := entity.PrimaryIdentity(); primary != nil {
		identity = primary.Name
	}
	var expiresAt int64
	if primarySelfSignature != nil && primarySelfSignature.KeyLifetimeSecs != nil && *primarySelfSignature.KeyLifetimeSecs != 0 {
		expiresAt = entity.PrimaryKey.CreationTime.Add(time.Duration(*primarySelfSignature.KeyLifetimeSecs) * time.Second).UnixMilli()
	}
	key := &core.GPGPublicKey{
		Fingerprint:     strings.ToUpper(hex.EncodeToString(entity.PrimaryKey.Fingerprint)),
		KeyID:           fmt.Sprintf("%016X", entity.PrimaryKey.KeyId),
		PrimaryIdentity: identity,
		PublicKey:       serialized.Bytes(),
		KeyCreatedAt:    entity.PrimaryKey.CreationTime.UnixMilli(),
		KeyExpiresAt:    expiresAt,
		FetchedAt:       fetchedAt.UnixMilli(),
	}
	return key, entityAliases(entity), nil
}

func parsePublicKey(data []byte, reference string, fetchedAt time.Time) (*core.GPGPublicKey, []string, error) {
	entities, err := openpgp.ReadArmoredKeyRing(bytes.NewReader(data))
	if err != nil {
		return nil, nil, fmt.Errorf("invalid public key response: %w", err)
	}
	var matched *openpgp.Entity
	for _, entity := range entities {
		if !entityMatchesReference(entity, reference) {
			continue
		}
		if matched != nil && !bytes.Equal(matched.PrimaryKey.Fingerprint, entity.PrimaryKey.Fingerprint) {
			return nil, nil, errAmbiguousKey
		}
		matched = entity
	}
	if matched == nil {
		return nil, nil, errKeyNotFound
	}
	return entityToPublicKey(matched, fetchedAt)
}

func resolveKeyServerAddresses(ctx context.Context, parsed *url.URL) ([]string, error) {
	host := parsed.Hostname()
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil || len(ips) == 0 {
		return nil, errors.New("could not resolve GPG key server")
	}
	port := parsed.Port()
	if port == "" {
		port = "443"
	}
	return validatedKeyServerAddresses(ips, port)
}

func validatedKeyServerAddresses(ips []net.IP, port string) ([]string, error) {
	ipv4 := make([]string, 0, len(ips))
	ipv6 := make([]string, 0, len(ips))
	seen := make(map[string]struct{}, len(ips))
	for _, ip := range ips {
		if !isPublicIP(ip) {
			return nil, errors.New("GPG key server resolves to a non-public address")
		}
		address := net.JoinHostPort(ip.String(), port)
		if _, exists := seen[address]; exists {
			continue
		}
		seen[address] = struct{}{}
		if ip.To4() != nil {
			ipv4 = append(ipv4, address)
		} else {
			ipv6 = append(ipv6, address)
		}
	}
	addresses := append(ipv4, ipv6...)
	if len(addresses) == 0 {
		return nil, errors.New("GPG key server resolves to a non-public address")
	}
	if len(addresses) > maxPinnedAddresses {
		addresses = addresses[:maxPinnedAddresses]
	}
	return addresses, nil
}

func dialPinnedAddresses(ctx context.Context, network string, addresses []string) (net.Conn, error) {
	if len(addresses) == 0 {
		return nil, errors.New("no public GPG key server address is available")
	}
	perAddressTimeout := max(pinnedDialTimeout/time.Duration(len(addresses)), 750*time.Millisecond)
	dialer := &net.Dialer{Timeout: perAddressTimeout, KeepAlive: -1}
	errList := make([]error, 0, len(addresses))
	for _, address := range addresses {
		attemptCtx, cancel := context.WithTimeout(ctx, perAddressTimeout)
		conn, err := dialer.DialContext(attemptCtx, network, address)
		cancel()
		if err == nil {
			return conn, nil
		}
		errList = append(errList, err)
		if ctx.Err() != nil {
			break
		}
	}
	return nil, errors.Join(errList...)
}

func makeKeyServerClient(ctx context.Context, parsed *url.URL, proxyConfig *config.OutboundProxy) (*http.Client, error) {
	var addresses []string
	if proxyConfig == nil {
		var err error
		addresses, err = resolveKeyServerAddresses(ctx, parsed)
		if err != nil {
			return nil, err
		}
	}
	transport := &http.Transport{
		Proxy: nil,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
		DisableCompression:     true,
		DisableKeepAlives:      true,
		TLSHandshakeTimeout:    4 * time.Second,
		ResponseHeaderTimeout:  5 * time.Second,
		MaxResponseHeaderBytes: 128 << 10,
	}
	if proxyConfig == nil {
		transport.DialContext = func(dialCtx context.Context, network, _ string) (net.Conn, error) {
			return dialPinnedAddresses(dialCtx, network, addresses)
		}
	} else if err := outboundproxy.ConfigureTransport(transport, proxyConfig); err != nil {
		return nil, err
	}
	return &http.Client{
		Timeout:   keyFetchTimeout,
		Transport: transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}, nil
}

func fetchKeyFromServer(ctx context.Context, server, reference string, proxyConfig *config.OutboundProxy) (*core.GPGPublicKey, []string, error) {
	select {
	case keyFetchSemaphore <- struct{}{}:
		defer func() { <-keyFetchSemaphore }()
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}

	base, err := url.Parse(server)
	if err != nil {
		return nil, nil, err
	}
	client, err := makeKeyServerClient(ctx, base, proxyConfig)
	if err != nil {
		return nil, nil, err
	}
	if transport, ok := client.Transport.(*http.Transport); ok {
		defer transport.CloseIdleConnections()
	}
	lookup := *base
	lookup.Path = "/pks/lookup"
	query := lookup.Query()
	query.Set("op", "get")
	query.Set("options", "mr")
	query.Set("search", "0x"+reference)
	lookup.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, lookup.String(), nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Accept", "application/pgp-keys, application/pgp, text/plain;q=0.8")
	req.Header.Set("User-Agent", "RenoP-GPG-Key-Resolver/1")
	req.Close = true

	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer utils.DiscardHTTPBody(resp.Body, resp.ContentLength)
	if resp.StatusCode != http.StatusOK {
		return nil, nil, errKeyNotFound
	}
	if resp.ContentLength > MaxPublicKeySize {
		return nil, nil, errors.New("GPG public key exceeds the size limit")
	}
	limited := &io.LimitedReader{R: resp.Body, N: MaxPublicKeySize + 1}
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, nil, err
	}
	if limited.N <= 0 {
		return nil, nil, errors.New("GPG public key exceeds the size limit")
	}
	return parsePublicKey(data, reference, time.Now())
}

func fetchPublicKey(ctx context.Context, cfg *config.Config, reference string) (*core.GPGPublicKey, []string, error) {
	if cfg == nil {
		return nil, nil, errors.New("configuration unavailable")
	}
	servers, err := ValidateKeyServers(cfg.Server.GPG.KeyServers)
	if err != nil {
		return nil, nil, err
	}
	proxyConfig, err := outboundproxy.Selected(cfg.Proxy)
	if err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, keyFetchTotalTimeout)
	defer cancel()
	var lastErr error
	for _, server := range servers {
		key, aliases, fetchErr := fetchKeyFromServer(ctx, server, reference, proxyConfig)
		if fetchErr == nil {
			return key, aliases, nil
		}
		if errors.Is(fetchErr, errAmbiguousKey) {
			return nil, nil, fetchErr
		}
		lastErr = fetchErr
		if ctx.Err() != nil {
			break
		}
	}
	if lastErr == nil {
		lastErr = errKeyNotFound
	}
	return nil, nil, fmt.Errorf("%w: %v", errKeyNotFound, lastErr)
}

func isFresh(key *core.GPGPublicKey, now time.Time) bool {
	return key != nil && key.FetchedAt > 0 && now.UnixMilli()-key.FetchedAt < PublicKeyCacheTTL.Milliseconds()
}

func RegisterUserKey(ctx context.Context, state *core.AppState, username, reference string) (*core.UserGPGKey, error) {
	if state == nil || state.Inner == nil || strings.TrimSpace(username) == "" {
		return nil, core.ErrDatabaseUnavailable
	}
	reference, err := NormalizeKeyReference(reference)
	if err != nil {
		return nil, err
	}
	db := state.GetDB()
	if db == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	userLockKey := "gpg-user:" + strings.ToLower(strings.TrimSpace(username))
	var userUpdate *core.InFlightDownload
	for {
		var loaded bool
		userUpdate, loaded = state.Inner.GPGUserKeyUpdates.LockPath(userLockKey)
		if !loaded {
			break
		}
		state.Inner.GPGUserKeyUpdates.Wait(userUpdate)
	}
	registrationSucceeded := false
	defer func() {
		state.Inner.GPGUserKeyUpdates.UnlockPath(userLockKey, userUpdate, registrationSucceeded)
	}()

	registeredKeys, err := db.ListUserGPGKeys(username)
	if err != nil {
		return nil, err
	}
	if len(registeredKeys) >= MaxKeysPerUser {
		cachedMatches, findErr := db.FindGPGPublicKeys(reference)
		if findErr != nil {
			return nil, findErr
		}
		alreadyRegistered := false
		for _, registered := range registeredKeys {
			for _, cached := range cachedMatches {
				if registered.Fingerprint == cached.Fingerprint {
					alreadyRegistered = true
					break
				}
			}
			if alreadyRegistered {
				break
			}
		}
		if !alreadyRegistered {
			return nil, core.ErrGPGKeyLimit
		}
	}

	findFresh := func() (*core.GPGPublicKey, error) {
		keys, findErr := db.FindGPGPublicKeys(reference)
		if findErr != nil {
			return nil, findErr
		}
		if len(keys) > 1 {
			return nil, errAmbiguousKey
		}
		if len(keys) == 1 && isFresh(keys[0], time.Now()) {
			return keys[0], nil
		}
		return nil, nil
	}

	key, err := findFresh()
	if err != nil {
		return nil, err
	}
	var aliases []string
	if key == nil {
		lockKey := "gpg-key:" + reference
		for {
			fetch, loaded := state.Inner.GPGKeyFetches.LockPath(lockKey)
			if !loaded {
				fetchSucceeded := false
				defer func() {
					state.Inner.GPGKeyFetches.UnlockPath(lockKey, fetch, fetchSucceeded)
				}()
				key, err = findFresh()
				if err != nil {
					return nil, err
				}
				if key == nil {
					key, aliases, err = fetchPublicKey(ctx, state.Inner.Config.Load(), reference)
					if err != nil {
						return nil, err
					}
					if err = db.RefreshGPGPublicKey(key, aliases); err != nil {
						return nil, err
					}
				}
				fetchSucceeded = true
				break
			}
			state.Inner.GPGKeyFetches.Wait(fetch)
			key, err = findFresh()
			if err != nil {
				return nil, err
			}
			if key != nil {
				break
			}
		}
	}
	if len(aliases) == 0 {
		entityList, parseErr := openpgp.ReadKeyRing(bytes.NewReader(key.PublicKey))
		if parseErr != nil || len(entityList) != 1 {
			return nil, errors.New("cached GPG public key is invalid")
		}
		aliases = entityAliases(entityList[0])
	}
	if err := db.RegisterUserGPGKey(username, reference, key, aliases); err != nil {
		return nil, err
	}
	keys, err := db.ListUserGPGKeys(username)
	if err != nil {
		return nil, err
	}
	for _, userKey := range keys {
		if userKey.Fingerprint == key.Fingerprint {
			registrationSucceeded = true
			return userKey, nil
		}
	}
	return nil, errors.New("registered GPG key could not be loaded")
}

func refreshPublicKey(ctx context.Context, state *core.AppState, fingerprint string) (*core.GPGPublicKey, error) {
	db := state.GetDB()
	if db == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	lockKey := "gpg-key:" + fingerprint
	for {
		fetch, loaded := state.Inner.GPGKeyFetches.LockPath(lockKey)
		if loaded {
			state.Inner.GPGKeyFetches.Wait(fetch)
			cached, err := db.GetGPGPublicKey(fingerprint)
			if err != nil {
				return nil, err
			}
			if isFresh(cached, time.Now()) {
				return cached, nil
			}
			continue
		}
		fetchSucceeded := false
		key, aliases, err := fetchPublicKey(ctx, state.Inner.Config.Load(), fingerprint)
		if err == nil && key.Fingerprint != fingerprint {
			err = errors.New("GPG key server returned a different fingerprint")
		}
		if err == nil {
			err = db.RefreshGPGPublicKey(key, aliases)
		}
		if err == nil {
			fetchSucceeded = true
		}
		state.Inner.GPGKeyFetches.UnlockPath(lockKey, fetch, fetchSucceeded)
		if err != nil {
			return nil, err
		}
		return key, nil
	}
}
