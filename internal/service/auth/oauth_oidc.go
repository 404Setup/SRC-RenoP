/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"renop/internal/config"
)

type oauthJWK struct {
	KID    string `json:"kid"`
	KTY    string `json:"kty"`
	Alg    string `json:"alg"`
	Use    string `json:"use"`
	N      string `json:"n"`
	E      string `json:"e"`
	X      string `json:"x"`
	Y      string `json:"y"`
	Curve  string `json:"crv"`
	Issuer string `json:"issuer"`
}

func oauthSigningKey(jwk oauthJWK, algorithm string) (any, error) {
	invalid := errors.New("OAuth signing key is invalid")
	if jwk.Use != "" && jwk.Use != "sig" || jwk.Alg != "" && jwk.Alg != algorithm {
		return nil, invalid
	}
	if algorithm == "RS256" && jwk.KTY == "RSA" {
		n, err := base64.RawURLEncoding.DecodeString(jwk.N)
		if err != nil || len(n) < 256 || len(n) > 512 {
			return nil, invalid
		}
		e, err := base64.RawURLEncoding.DecodeString(jwk.E)
		if err != nil || len(e) == 0 || len(e) > 4 {
			return nil, invalid
		}
		exponent := new(big.Int).SetBytes(e).Int64()
		modulus := new(big.Int).SetBytes(n)
		if exponent < 3 || exponent > 2147483647 || exponent%2 == 0 || modulus.BitLen() < 2048 {
			return nil, invalid
		}
		return &rsa.PublicKey{N: modulus, E: int(exponent)}, nil
	}
	if algorithm == "ES256" && jwk.KTY == "EC" && jwk.Curve == "P-256" {
		x, xerr := base64.RawURLEncoding.DecodeString(jwk.X)
		y, yerr := base64.RawURLEncoding.DecodeString(jwk.Y)
		if xerr != nil || yerr != nil || len(x) != 32 || len(y) != 32 {
			return nil, invalid
		}
		key := &ecdsa.PublicKey{Curve: elliptic.P256(), X: new(big.Int).SetBytes(x), Y: new(big.Int).SetBytes(y)}
		if !key.Curve.IsOnCurve(key.X, key.Y) {
			return nil, invalid
		}
		return key, nil
	}
	return nil, invalid
}

func oauthIssuerAllowed(p config.OAuthProviderConfig, issuer, tenant string) bool {
	if p.Type == "google" && issuer == "accounts.google.com" {
		return true
	}
	if p.Type != "microsoft" {
		return issuer == p.Issuer
	}
	tenantID, err := uuid.Parse(tenant)
	if err != nil || tenantID.String() != strings.ToLower(tenant) || issuer != "https://login.microsoftonline.com/"+tenant+"/v2.0" {
		return false
	}
	const consumers = "9188040d-6c67-4c5b-b112-36a304b66dad"
	switch p.Tenant {
	case "", "common":
		return true
	case "organizations":
		return tenant != consumers
	case "consumers":
		return tenant == consumers
	default:
		return strings.EqualFold(p.Tenant, tenant)
	}
}

func verifyOAuthIDToken(ctx context.Context, client *http.Client, p config.OAuthProviderConfig, tokens oauthTokens, nonce string) (jwt.MapClaims, error) {
	invalid := errors.New("OAuth ID token verification failed")
	if tokens.IDToken == "" || len(tokens.IDToken) > 16384 || nonce == "" {
		return nil, invalid
	}
	var keys struct {
		Keys []oauthJWK `json:"keys"`
	}
	if err := getOAuthJSON(ctx, client, p.JWKSURL, "", &keys); err != nil {
		return nil, err
	}
	if len(keys.Keys) == 0 || len(keys.Keys) > 32 {
		return nil, invalid
	}
	claims := jwt.MapClaims{}
	token, err := jwt.ParseWithClaims(tokens.IDToken, claims, func(token *jwt.Token) (any, error) {
		issuer, _ := claims.GetIssuer()
		tenant, _ := claims["tid"].(string)
		if !oauthIssuerAllowed(p, issuer, tenant) {
			return nil, invalid
		}
		kid, _ := token.Header["kid"].(string)
		if kid == "" || len(kid) > 256 {
			return nil, invalid
		}
		for _, key := range keys.Keys {
			if key.KID != kid {
				continue
			}
			if p.Type == "microsoft" && key.Issuer != "" && strings.ReplaceAll(key.Issuer, "{tenantid}", tenant) != issuer {
				continue
			}
			return oauthSigningKey(key, token.Method.Alg())
		}
		return nil, invalid
	}, jwt.WithValidMethods([]string{"RS256", "ES256"}), jwt.WithAudience(p.ClientID), jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(), jwt.WithLeeway(30*time.Second), jwt.WithJSONNumber(), jwt.WithStrictDecoding())
	if err != nil || !token.Valid {
		return nil, invalid
	}
	actualNonce, _ := claims["nonce"].(string)
	subject, err := claims.GetSubject()
	iat, iatErr := claims.GetIssuedAt()
	if err != nil || subject == "" || len(subject) > 255 || iatErr != nil || iat == nil ||
		iat.Time.Before(time.Now().Add(-10*time.Minute)) || subtle.ConstantTimeCompare([]byte(actualNonce), []byte(nonce)) != 1 {
		return nil, invalid
	}
	audience, _ := claims.GetAudience()
	azp, hasAZP := claims["azp"]
	if len(audience) > 1 && !hasAZP || hasAZP && azp != p.ClientID {
		return nil, invalid
	}
	if hash, exists := claims["at_hash"]; exists {
		digest := sha256.Sum256([]byte(tokens.AccessToken))
		if hash != base64.RawURLEncoding.EncodeToString(digest[:16]) {
			return nil, invalid
		}
	}
	return claims, nil
}
