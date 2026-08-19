/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package token

import (
	"errors"
	"strings"

	"renop/internal/core"
)

type TokenOpType int

const (
	OpTokenStore TokenOpType = iota
	OpTokenDelete
	OpTokenUpdate
	OpTokenRename
)

type TokenOp struct {
	Type     TokenOpType
	Name     string
	NewName  string // used by OpTokenRename
	Token    *core.AccessToken
	UpdateFn func(*core.AccessToken)
	ErrChan  chan error
	State    *core.AppState // used by OpTokenRename to update sessions
}

func cloneAccessToken(token *core.AccessToken) *core.AccessToken {
	if token == nil {
		return nil
	}
	cloned := *token
	cloned.Name = strings.Clone(token.Name)
	cloned.EncryptedSecret = strings.Clone(token.EncryptedSecret)
	cloned.PasswordHash = strings.Clone(token.PasswordHash)
	cloned.CreatedAt = strings.Clone(token.CreatedAt)
	cloned.Description = strings.Clone(token.Description)

	if token.ExpiresAt != nil {
		expires := *token.ExpiresAt
		cloned.ExpiresAt = &expires
	}

	if token.Tokens != nil {
		cloned.Tokens = make([]string, len(token.Tokens))
		for i, t := range token.Tokens {
			cloned.Tokens[i] = strings.Clone(t)
		}
	}

	if token.Permissions != nil {
		cloned.Permissions = make([]string, len(token.Permissions))
		for i, p := range token.Permissions {
			cloned.Permissions[i] = strings.Clone(p)
		}
	}

	return &cloned
}

func StartTokenConsumer(state *core.AppState, opChan <-chan TokenOp) {
	for op := range opChan {
		switch op.Type {
		case OpTokenStore:
			safeName := strings.Clone(op.Name)
			clonedToken := cloneAccessToken(op.Token)
			if clonedToken == nil {
				completeTokenOp(op, errors.New("token is required"))
				continue
			}
			clonedToken.Name = safeName

			db := state.GetDB()
			if db == nil {
				completeTokenOp(op, core.ErrDatabaseUnavailable)
				continue
			}
			existing, err := db.GetTokenByName(safeName)
			if err == nil {
				err = db.SaveToken(clonedToken)
			}
			if err != nil {
				completeTokenOp(op, err)
				continue
			}
			if existing == nil {
				state.Inner.TokensCount.Add(1)
			}
			state.ClearAuthCache()
			completeTokenOp(op, nil)

		case OpTokenDelete:
			safeName := strings.Clone(op.Name)
			db := state.GetDB()
			if db == nil {
				completeTokenOp(op, core.ErrDatabaseUnavailable)
				continue
			}
			existing, err := db.GetTokenByName(safeName)
			if err == nil && existing != nil {
				err = db.DeleteToken(safeName)
			}
			if err != nil {
				completeTokenOp(op, err)
				continue
			}
			if existing != nil {
				state.Inner.TokensCount.Add(^uint64(0))
			}
			state.Inner.Sessions.Range(func(sessionToken string, session *core.Session) bool {
				if session != nil && strings.EqualFold(session.Username, safeName) {
					state.DeleteAuthCache("Session " + sessionToken)
					state.Inner.Sessions.Delete(sessionToken)
				}
				return true
			})
			state.ClearAuthCache()
			completeTokenOp(op, nil)

		case OpTokenUpdate:
			safeName := strings.Clone(op.Name)
			if op.UpdateFn == nil {
				completeTokenOp(op, errors.New("token update function is required"))
				continue
			}
			db := state.GetDB()
			if db == nil {
				completeTokenOp(op, core.ErrDatabaseUnavailable)
				continue
			}
			val, err := db.GetTokenByName(safeName)
			if err != nil {
				completeTokenOp(op, err)
				continue
			}
			if val != nil {
				token := val
				tCopy := *token
				op.UpdateFn(&tCopy)

				clonedToken := cloneAccessToken(&tCopy)
				clonedToken.Name = safeName

				if err := db.SaveToken(clonedToken); err != nil {
					completeTokenOp(op, err)
					continue
				}
				state.ClearAuthCache()
				completeTokenOp(op, nil)
			} else {
				completeTokenOp(op, errors.New("token not found"))
			}

		case OpTokenRename:
			oldName := strings.Clone(op.Name)
			newName := strings.Clone(op.NewName)
			clonedToken := cloneAccessToken(op.Token)
			if clonedToken == nil {
				completeTokenOp(op, errors.New("token is required"))
				continue
			}
			clonedToken.Name = newName
			db := state.GetDB()
			if db == nil {
				completeTokenOp(op, core.ErrDatabaseUnavailable)
				continue
			}
			val, err := db.GetTokenByName(oldName)
			if err != nil {
				completeTokenOp(op, err)
				continue
			}
			if val != nil {
				if newName != oldName {
					existing, err := db.GetTokenByName(newName)
					if err != nil {
						completeTokenOp(op, err)
						continue
					}
					if existing != nil {
						completeTokenOp(op, errors.New("token name already exists"))
						continue
					}
				}
				if err := db.RenameToken(oldName, newName, clonedToken); err != nil {
					completeTokenOp(op, err)
					continue
				}
				if op.State != nil {
					op.State.Inner.Sessions.Range(func(key string, session *core.Session) bool {
						if strings.EqualFold(session.Username, oldName) {
							session.Username = newName
						}
						return true
					})
				}
				state.ClearAuthCache()
				completeTokenOp(op, nil)
			} else {
				completeTokenOp(op, errors.New("token not found"))
			}
		}
	}
}

func completeTokenOp(op TokenOp, err error) {
	if op.ErrChan != nil {
		op.ErrChan <- err
	}
}
