/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"log"
	"strconv"
	"strings"
	"time"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/token"
)

func githubUsernameBase(login string) string {
	var builder strings.Builder
	builder.Grow(min(len(login), core.MaxUsernameLength))
	for _, character := range strings.ToLower(login) {
		if builder.Len() >= core.MaxUsernameLength {
			break
		}
		switch {
		case character >= 'a' && character <= 'z', character >= '0' && character <= '9', character == '_':
			builder.WriteRune(character)
		case character == '-':
			builder.WriteByte('_')
		}
	}
	result := strings.Trim(builder.String(), "_")
	if len(result) < core.MinUsernameLength {
		result = "gh_" + result
	}
	if len(result) > core.MaxUsernameLength {
		result = result[:core.MaxUsernameLength]
	}
	if normalized, ok := core.NormalizeUsername(result); ok {
		return normalized
	}
	return "github_user"
}

func randomGitHubUsername() (string, error) {
	var value [10]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return "gh_" + base64.RawURLEncoding.EncodeToString(value[:]), nil
}

func availableGitHubUsername(state *core.AppState, login string, githubID int64) (string, error) {
	base := githubUsernameBase(login)
	candidates := []string{base}
	suffix := "_" + strconv.FormatInt(githubID, 36)
	prefixLength := core.MaxUsernameLength - len(suffix)
	if prefixLength < 1 {
		prefixLength = 1
	}
	prefix := base
	if len(prefix) > prefixLength {
		prefix = prefix[:prefixLength]
	}
	candidates = append(candidates, prefix+suffix)
	for _, candidate := range candidates {
		if normalized, ok := core.NormalizeUsername(candidate); ok && state.GetTokenByName(normalized) == nil {
			return normalized, nil
		}
	}
	for attempts := 0; attempts < 8; attempts++ {
		candidate, err := randomGitHubUsername()
		if err != nil {
			return "", err
		}
		if state.GetTokenByName(candidate) == nil {
			return candidate, nil
		}
	}
	return "", errors.New("could not allocate a GitHub account username")
}

func createGitHubAccount(state *core.AppState, opChan chan<- token.TokenOp,
	identity githubAPIIdentity) (*core.UserProfile, error) {
	if opChan == nil {
		return nil, errors.New("token operation channel is unavailable")
	}
	username, err := availableGitHubUsername(state, identity.Login, identity.ID)
	if err != nil {
		return nil, err
	}
	nickname := strings.TrimSpace(identity.Name)
	if normalized, ok := core.NormalizeNickname(nickname); ok {
		nickname = normalized
	} else if normalized, ok := core.NormalizeNickname(identity.Login); ok {
		nickname = normalized
	} else {
		nickname = ""
	}
	account := &core.AccessToken{
		Identifier: core.AccessTokenIdentifier{Type: core.Persistent},
		Name:       username, Tokens: []string{}, CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Description: "Created through GitHub OAuth", Permissions: []string{"base"},
	}
	errChannel := make(chan error, 1)
	opChan <- token.TokenOp{
		Type: token.OpTokenCreate, Name: username, Token: account, Nickname: nickname,
		ChangedAt: time.Now().UnixMilli(), ErrChan: errChannel,
	}
	if err := <-errChannel; err != nil {
		return nil, err
	}
	return state.GetDB().GetUserProfile(username)
}

func deleteGitHubCreatedAccount(opChan chan<- token.TokenOp, username string) {
	if opChan == nil || username == "" {
		return
	}
	errChannel := make(chan error, 1)
	opChan <- token.TokenOp{Type: token.OpTokenDelete, Name: username, ErrChan: errChannel}
	if err := <-errChannel; err != nil {
		log.Printf("Failed to roll back GitHub-created account %s: %v", username, err)
	}
}

func resolveGitHubLogin(state *core.AppState, opChan chan<- token.TokenOp, identity githubAPIIdentity,
	principals []core.GitHubPrincipal, authorizedAt int64) (*config.User, error) {
	state.Inner.TokenWriteLock.Lock()
	defer state.Inner.TokenWriteLock.Unlock()
	linked, err := state.GetDB().GetGitHubIdentityByProviderID(identity.ID)
	if err != nil {
		return nil, err
	}
	createdUsername := ""
	if linked == nil {
		profile, err := createGitHubAccount(state, opChan, identity)
		if err != nil {
			return nil, err
		}
		createdUsername = profile.Username
		linked = &core.GitHubIdentity{UserID: profile.UserID, Username: profile.Username}
	}
	if err := state.GetDB().StoreGitHubIdentity(linked.UserID, identity.ID, identity.Login,
		principals, authorizedAt); err != nil {
		if createdUsername != "" {
			deleteGitHubCreatedAccount(opChan, createdUsername)
		}
		return nil, err
	}
	accessToken := state.GetTokenByName(linked.Username)
	if accessToken == nil {
		return nil, errors.New("linked RenoP account is unavailable")
	}
	if err := accountAccessError(accessToken); err != nil {
		return nil, err
	}
	return buildSynthUser(accessToken), nil
}
