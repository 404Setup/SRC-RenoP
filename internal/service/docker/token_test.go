/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package docker

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/3JoB/unsafeConvert"
	"github.com/goccy/go-json"
)

func TestDockerTokenLifecycle(t *testing.T) {
	secret := []byte("a-very-secure-docker-secret-key-32bytes")
	access := []AccessEntry{
		{
			Type:    "repository",
			Name:    "docker-local/org/my-app",
			Actions: []string{"pull", "push"},
		},
	}

	token, err := GenerateDockerToken(secret, "admin", "renop-registry", access, 10*time.Minute)
	if err != nil {
		t.Fatalf("GenerateDockerToken failed: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	claims, err := ValidateDockerToken(secret, token)
	if err != nil {
		t.Fatalf("ValidateDockerToken failed: %v", err)
	}
	if claims.Subject != "admin" {
		t.Fatalf("expected subject 'admin', got '%s'", claims.Subject)
	}
	if claims.Audience != "renop-registry" {
		t.Fatalf("expected audience 'renop-registry', got '%s'", claims.Audience)
	}
	if len(claims.Access) != 1 || claims.Access[0].Name != "docker-local/org/my-app" {
		t.Fatalf("unexpected access entries: %+v", claims.Access)
	}
	if len(claims.Access[0].Actions) != 2 || claims.Access[0].Actions[0] != "pull" || claims.Access[0].Actions[1] != "push" {
		t.Fatalf("unexpected actions: %+v", claims.Access[0].Actions)
	}

	expiredToken, err := GenerateDockerToken(secret, "admin", "renop-registry", access, -5*time.Minute)
	if err != nil {
		t.Fatalf("GenerateDockerToken (expired) failed: %v", err)
	}
	_, err = ValidateDockerToken(secret, expiredToken)
	if err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expected token expired error, got: %v", err)
	}

	tamperedToken := token + "tamper"
	_, err = ValidateDockerToken(secret, tamperedToken)
	if err == nil || !strings.Contains(err.Error(), "signature") {
		t.Fatalf("expected invalid signature error, got: %v", err)
	}

	wrongSecret := []byte("wrong-secret-key-000000000000000000")
	_, err = ValidateDockerToken(wrongSecret, token)
	if err == nil || !strings.Contains(err.Error(), "signature") {
		t.Fatalf("expected signature mismatch with wrong secret, got: %v", err)
	}

	malformedCases := []string{
		"",
		"singlepart",
		"part1.part2",
		"part1.part2.part3.part4",
	}
	for _, mal := range malformedCases {
		if _, err := ValidateDockerToken(secret, mal); err == nil {
			t.Fatalf("expected error for malformed token '%s'", mal)
		}
	}

	parts := strings.Split(token, ".")
	fakeClaimsJSON, _ := json.Marshal(TokenClaims{
		Issuer:    "renop",
		Subject:   "root-attacker",
		Audience:  "renop-registry",
		ExpiresAt: time.Now().Add(1 * time.Hour).Unix(),
	})
	fakeClaimsEncoded := base64.RawURLEncoding.EncodeToString(fakeClaimsJSON)
	forgedToken := parts[0] + "." + fakeClaimsEncoded + "." + parts[2]
	if _, err := ValidateDockerToken(secret, forgedToken); err == nil {
		t.Fatal("expected signature mismatch for forged payload token")
	}

	msg := parts[0] + ".???invalidbase64???"
	mac := hmac.New(sha256.New, secret)
	mac.Write(unsafeConvert.BytePointer(msg))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if _, err := ValidateDockerToken(secret, msg+"."+sig); err == nil {
		t.Fatal("expected decode error for corrupted claims encoding")
	}
}

func TestGenerateDockerSecret(t *testing.T) {
	s1 := GenerateDockerSecret()
	s2 := GenerateDockerSecret()

	if len(s1) != 32 {
		t.Fatalf("expected 32 bytes secret, got %d", len(s1))
	}
	if len(s2) != 32 {
		t.Fatalf("expected 32 bytes secret, got %d", len(s2))
	}
	if hmac.Equal(s1, s2) {
		t.Fatal("two independently generated secrets should not be identical")
	}
}
