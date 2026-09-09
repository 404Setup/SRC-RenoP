/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package maven

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/goccy/go-json"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/outboundproxy"
	"renop/internal/service/repositorygate"
)

const domainHealthInterval = 6 * time.Hour

var rdapBootstrap struct {
	sync.Mutex
	endpoints map[string]string
	expires   time.Time
	err       error
}

func rdapEndpoint(ctx context.Context, client *http.Client, host string) (string, error) {
	rdapBootstrap.Lock()
	defer rdapBootstrap.Unlock()
	if time.Now().After(rdapBootstrap.expires) {
		var document struct {
			Services [][][]string `json:"services"`
		}
		err := getRDAPJSON(ctx, client, "https://data.iana.org/rdap/dns.json", &document)
		endpoints := make(map[string]string)
		if err == nil {
			for _, service := range document.Services {
				if len(service) != 2 {
					continue
				}
				for _, endpoint := range service[1] {
					parsed, parseErr := url.Parse(endpoint)
					if parseErr != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil ||
						parsed.RawQuery != "" || parsed.Fragment != "" {
						continue
					}
					for _, suffix := range service[0] {
						endpoints[strings.ToLower(suffix)] = strings.TrimRight(endpoint, "/") + "/"
					}
					break
				}
			}
			if len(endpoints) == 0 {
				err = errors.New("RDAP bootstrap contains no HTTPS services")
			}
		}
		rdapBootstrap.err = err
		rdapBootstrap.expires = time.Now().Add(5 * time.Minute)
		if err == nil {
			rdapBootstrap.endpoints = endpoints
			rdapBootstrap.expires = time.Now().Add(24 * time.Hour)
		}
	}
	if rdapBootstrap.err != nil {
		return "", rdapBootstrap.err
	}
	for suffix := host; suffix != ""; {
		if endpoint := rdapBootstrap.endpoints[suffix]; endpoint != "" {
			return endpoint + "domain/" + url.PathEscape(host), nil
		}
		_, suffix, _ = strings.Cut(suffix, ".")
	}
	return "", errors.New("domain registry has no RDAP service")
}

func getRDAPJSON(ctx context.Context, client *http.Client, endpoint string, destination any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/rdap+json, application/json")
	request.Header.Set("User-Agent", "RenoP-Maven-Domain-Verifier/1")
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return errVerificationAccountMissing
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("RDAP provider returned HTTP %d", response.StatusCode)
	}
	const limit = 2 << 20
	if response.ContentLength > limit {
		return errors.New("RDAP response exceeds the size limit")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return err
	}
	if len(body) > limit {
		return errors.New("RDAP response exceeds the size limit")
	}
	return json.Unmarshal(body, destination)
}

type rdapDomain struct {
	ObjectClassName string   `json:"objectClassName"`
	LDHName         string   `json:"ldhName"`
	Status          []string `json:"status"`
	Events          []struct {
		Action string `json:"eventAction"`
		Date   string `json:"eventDate"`
	} `json:"events"`
}

func rdapDomainHealth(document rdapDomain, host string, now time.Time) (*core.MavenDomainHealth, error) {
	if document.ObjectClassName != "domain" || !strings.EqualFold(strings.TrimSuffix(document.LDHName, "."), host) {
		return nil, errors.New("RDAP response does not describe the requested domain")
	}
	health := &core.MavenDomainHealth{Status: "active"}
	for _, event := range document.Events {
		if event.Action != "expiration" {
			continue
		}
		expires, err := time.Parse(time.RFC3339, event.Date)
		if err != nil {
			return nil, errors.New("RDAP expiration date is invalid")
		}
		if health.ExpiresAt == 0 || expires.UnixMilli() < health.ExpiresAt {
			health.ExpiresAt = expires.UnixMilli()
		}
	}
	for _, status := range document.Status {
		switch strings.NewReplacer(" ", "", "-", "", "_", "").Replace(strings.ToLower(status)) {
		case "serverhold", "clienthold", "hold":
			if health.Status == "active" {
				health.Status = "hold"
			}
		case "clientrenewprohibited", "serverrenewprohibited", "renewprohibited":
			if health.Status != "expired" {
				health.Status = "prohibited"
			}
		case "pendingdelete", "restorable", "redemptionperiod", "pendingrestore":
			health.Status = "expired"
		}
	}
	if health.ExpiresAt > 0 && health.ExpiresAt <= now.UnixMilli() {
		health.Status = "expired"
	}
	return health, nil
}

func inspectDomainHealth(ctx context.Context, client *http.Client, domain *core.MavenDomain, proof string) (*core.MavenDomainHealth, error) {
	if domain.VerificationType == core.MavenVerificationDNS {
		endpoint, err := rdapEndpoint(ctx, client, domain.VerificationHost)
		if err != nil {
			return nil, err
		}
		var document rdapDomain
		if err := getRDAPJSON(ctx, client, endpoint, &document); errors.Is(err, errVerificationAccountMissing) {
			return &core.MavenDomainHealth{Status: "expired"}, nil
		} else if err != nil {
			return nil, err
		}
		health, err := rdapDomainHealth(document, domain.VerificationHost, time.Now())
		if err == nil && health.ExpiresAt == 0 && domain.Health != nil {
			health.ExpiresAt = domain.Health.ExpiresAt
			if health.ExpiresAt > 0 && health.ExpiresAt <= time.Now().UnixMilli() {
				health.Status = "expired"
			}
		}
		return health, err
	}
	expected := domain.Health
	accountType := ""
	if expected != nil {
		accountType = expected.ProviderType
	}
	health, err := providerIdentity(ctx, client, domain.VerificationType, domain.VerificationHost, accountType, proof)
	if errors.Is(err, errVerificationAccountMissing) {
		return &core.MavenDomainHealth{Status: "expired"}, nil
	}
	if err != nil {
		return nil, err
	}
	if expected != nil && expected.ProviderID != "" &&
		(expected.ProviderID != health.ProviderID || expected.ProviderType != health.ProviderType) {
		health.Status = "prohibited"
	} else if proof == "" && (expected == nil || expected.ProviderID == "") && domain.Verified {
		// A legacy login name alone cannot establish the original account's identity.
		health.Status = "hold"
	}
	return health, nil
}

func checkDomainHealth(ctx context.Context, cfg *config.Config, domain *core.MavenDomain) (*core.MavenDomainHealth, error) {
	ctx, cancel := context.WithTimeout(ctx, verificationTimeout)
	defer cancel()
	select {
	case verificationSemaphore <- struct{}{}:
		defer func() { <-verificationSemaphore }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	proxy, err := outboundproxy.Selected(cfg.Proxy)
	if err != nil {
		return nil, err
	}
	client, err := verificationClient(proxy)
	if err != nil {
		return nil, err
	}
	defer client.CloseIdleConnections()
	return inspectDomainHealth(ctx, client, domain, "")
}

// CheckDomainHealth checks a bounded batch without holding repository gates across external requests.
func CheckDomainHealth(ctx context.Context, state *core.AppState) {
	domains, err := state.GetDB().ListMavenDomainHealthChecks(time.Now().UnixMilli(), 16)
	if err != nil {
		log.Printf("Failed to list Maven domain health checks: %v", err)
		return
	}
	for _, domain := range domains {
		if ctx.Err() != nil {
			return
		}
		cfg := state.Inner.Config.Load()
		health, err := checkDomainHealth(ctx, cfg, domain)
		now := time.Now()
		if err != nil {
			health = &core.MavenDomainHealth{Status: "unavailable"}
			if domain.VerificationType == core.MavenVerificationDNS && domain.Health.ExpiresAt > 0 && domain.Health.ExpiresAt <= now.UnixMilli() {
				health.Status, health.ExpiresAt = "expired", domain.Health.ExpiresAt
			}
		}
		health.CheckedAt, health.NextCheckAt = now.UnixMilli(), now.Add(domainHealthInterval).UnixMilli()
		if health.Status == "unavailable" {
			health.NextCheckAt = now.Add(15 * time.Minute).UnixMilli()
		}
		code := ""
		if health.Status != "active" && health.Status != "unavailable" && domain.Health.LockedAt == 0 {
			code, err = NewVerificationCode()
			if err != nil {
				log.Printf("Failed to create Maven domain redemption proof: %v", err)
				continue
			}
		}
		release := repositorygate.AcquireAllMigrations()
		err = state.GetDB().RecordMavenDomainHealth(domain, health, cfg.MavenDomains.ReleaseAt(now.UnixMilli()), code)
		release()
		if err != nil {
			log.Printf("Failed to record Maven domain health for %s: %v", domain.Domain, err)
		}
	}
}
