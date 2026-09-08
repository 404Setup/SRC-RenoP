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
	"errors"
	"renop/internal/config"
	"renop/internal/core"
)

func resolveGitHubLogin(state *core.AppState, identity githubAPIIdentity,
	principals []core.GitHubPrincipal, authorizedAt int64) (*config.User, error) {
	state.Inner.TokenWriteLock.Lock()
	defer state.Inner.TokenWriteLock.Unlock()
	linked, err := state.GetDB().GetGitHubIdentityByProviderID(identity.ID)
	if err != nil {
		return nil, err
	}
	if linked == nil {
		return nil, core.ErrGitHubIdentityNotFound
	}
	if err := state.GetDB().StoreGitHubIdentity(linked.UserID, identity.ID, identity.Login,
		principals, authorizedAt); err != nil {
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
