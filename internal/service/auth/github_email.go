/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package auth

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"renop/internal/core"
)

func fetchGitHubVerifiedEmail(ctx context.Context, client *http.Client, provider githubOAuthProvider, accessToken string) (string, error) {
	fallback := ""
	for page := 1; page <= 10; page++ {
		var addresses []struct {
			Email    string `json:"email"`
			Verified bool   `json:"verified"`
			Primary  bool   `json:"primary"`
		}
		link, err := getGitHubAPI(ctx, client, strings.TrimRight(provider.APIURL, "/")+
			"/user/emails?per_page=100&page="+strconv.Itoa(page), accessToken, &addresses)
		if err != nil {
			return "", err
		}
		if len(addresses) > 100 {
			return "", errors.New("GitHub email list exceeds the page limit")
		}
		for _, address := range addresses {
			email, valid := core.NormalizeEmail(address.Email)
			if !address.Verified || !valid || email == "" || strings.HasSuffix(email, "@users.noreply.github.com") ||
				strings.HasSuffix(email, "@noreply.github.com") {
				continue
			}
			if address.Primary {
				return email, nil
			}
			if fallback == "" {
				fallback = email
			}
		}
		if len(addresses) < 100 || !strings.Contains(link, `rel="next"`) {
			return fallback, nil
		}
	}
	return "", errors.New("GitHub email list exceeds the supported limit")
}

func finishGitHubEmailVerification(c fiber.Ctx, state *core.AppState, record core.TransientAuthState,
	ctx context.Context, client *http.Client, provider githubOAuthProvider, accessToken string) error {
	profile, err := currentSessionProfile(c, state)
	session, _ := c.Locals("current_session_id").(string)
	if err != nil || profile == nil || profile.UserID != record.UserID || session == "" ||
		c.Cookies(sessionCookieName) != session || fmt.Sprintf("%x", sha256.Sum256([]byte(session))) != record.SessionHash {
		return oauthResultRedirect(c, record.ReturnTo, "session_changed")
	}
	email, err := fetchGitHubVerifiedEmail(ctx, client, provider, accessToken)
	if err != nil {
		return oauthResultRedirect(c, record.ReturnTo, "email_failed")
	}
	if email == "" {
		return oauthResultRedirect(c, record.ReturnTo, "email_missing")
	}
	if !state.Inner.Config.Load().Mail.Allows(email) {
		return oauthResultRedirect(c, record.ReturnTo, "email_blocked")
	}
	_, err = state.GetDB().UpdateAccountEmailFromSession(profile.Username, session, email, record.Snapshot, time.Now().UnixMilli())
	if errors.Is(err, core.ErrEmailAlreadyExists) {
		return oauthResultRedirect(c, record.ReturnTo, "email_conflict")
	}
	if errors.Is(err, core.ErrEmailCodeInvalid) || errors.Is(err, core.ErrAccountDeleted) || errors.Is(err, core.ErrAccountBanned) {
		return oauthResultRedirect(c, record.ReturnTo, "session_changed")
	}
	if err != nil {
		return oauthResultRedirect(c, record.ReturnTo, "email_failed")
	}
	recordPrivateEmailChange(c, state, profile.Username)
	return oauthResultRedirect(c, record.ReturnTo, "email_updated")
}
