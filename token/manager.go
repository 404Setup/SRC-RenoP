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
	"os"
	"strings"

	"go.yaml.in/yaml/v3"

	"renop/core"
	"renop/utils"
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
			clonedToken.Name = safeName

			if db := state.GetDB(); db != nil {
				if existing, _ := db.GetTokenByName(safeName); existing == nil {
					state.Inner.TokensCount.Add(1)
				}
				_ = db.SaveToken(clonedToken)
			} else {
				for _, t := range clonedToken.Tokens {
					state.Inner.TokenIndex.Store(t, clonedToken)
				}
				if oldVal, loaded := state.Inner.TokenRepository.LoadOrStore(safeName, clonedToken); !loaded {
					state.Inner.TokensCount.Add(1)
				} else {
					state.Inner.TokenRepository.Store(safeName, clonedToken)
					if oldToken := oldVal; true {
						newTokens := make(map[string]bool)
						for _, t := range clonedToken.Tokens {
							newTokens[t] = true
						}
						for _, t := range oldToken.Tokens {
							if !newTokens[t] {
								state.Inner.TokenIndex.Delete(t)
							}
						}
					}
				}
			}
			saveOrClearCache(state, opChan)
			if op.ErrChan != nil {
				op.ErrChan <- nil
			}
		case OpTokenDelete:
			safeName := strings.Clone(op.Name)
			state.DeleteFidoDevicesByUsername(safeName)
			if db := state.GetDB(); db != nil {
				if existing, _ := db.GetTokenByName(safeName); existing != nil {
					_ = db.DeleteToken(safeName)
					state.Inner.TokensCount.Add(^uint64(0))
					saveOrClearCache(state, opChan)
				}
			} else {
				if oldVal, loaded := state.Inner.TokenRepository.LoadAndDelete(safeName); loaded {
					for _, t := range oldVal.Tokens {
						state.Inner.TokenIndex.Delete(t)
					}
					state.Inner.TokensCount.Add(^uint64(0))
					saveOrClearCache(state, opChan)
				}
			}
			if op.ErrChan != nil {
				op.ErrChan <- nil
			}
		case OpTokenUpdate:
			safeName := strings.Clone(op.Name)
			val := state.GetTokenByName(safeName)
			if val != nil {
				token := val
				tCopy := *token
				op.UpdateFn(&tCopy)

				clonedToken := cloneAccessToken(&tCopy)
				clonedToken.Name = safeName

				if db := state.GetDB(); db != nil {
					_ = db.SaveToken(clonedToken)
				} else {
					for _, t := range clonedToken.Tokens {
						state.Inner.TokenIndex.Store(t, clonedToken)
					}
					state.Inner.TokenRepository.Store(safeName, clonedToken)
					newTokens := make(map[string]bool)
					for _, t := range clonedToken.Tokens {
						newTokens[t] = true
					}
					for _, t := range token.Tokens {
						if !newTokens[t] {
							state.Inner.TokenIndex.Delete(t)
						}
					}
				}
				saveOrClearCache(state, opChan)
				if op.ErrChan != nil {
					op.ErrChan <- nil
				}
			} else {
				if op.ErrChan != nil {
					op.ErrChan <- errors.New("token not found")
				}
			}
		case OpTokenRename:
			oldName := strings.Clone(op.Name)
			newName := strings.Clone(op.NewName)
			clonedToken := cloneAccessToken(op.Token)
			clonedToken.Name = newName

			val := state.GetTokenByName(oldName)
			if val != nil {
				if existing := state.GetTokenByName(newName); existing != nil && newName != oldName {
					if op.ErrChan != nil {
						op.ErrChan <- errors.New("token name already exists")
					}
					continue
				}
				oldToken := val
				if db := state.GetDB(); db != nil {
					_ = db.RenameToken(oldName, newName, clonedToken)
				} else {
					state.Inner.TokenRepository.Store(newName, clonedToken)
					for _, t := range clonedToken.Tokens {
						state.Inner.TokenIndex.Store(t, clonedToken)
					}
					state.Inner.TokenRepository.Delete(oldName)
					for _, t := range oldToken.Tokens {
						state.Inner.TokenIndex.Delete(t)
					}
					for _, t := range clonedToken.Tokens {
						state.Inner.TokenIndex.Store(t, clonedToken)
					}
					if op.State != nil {
						op.State.Inner.Sessions.Range(func(key string, session *core.Session) bool {
							if session.Username == oldName {
								session.Username = newName
								op.State.Inner.SessionsIsDirty.Store(true)
							}
							return true
						})
					}
				}
				saveOrClearCache(state, opChan)
				if op.ErrChan != nil {
					op.ErrChan <- nil
				}
			} else {
				if op.ErrChan != nil {
					op.ErrChan <- errors.New("token not found")
				}
			}
		}
	}
}

func saveOrClearCache(state *core.AppState, opChan <-chan TokenOp) {
	state.ClearAuthCache()
	if len(opChan) == 0 {
		saveTokensSync(state)
	}
}

func saveTokensSync(state *core.AppState) {
	if state == nil || state.Inner == nil {
		return
	}
	if state.GetDB() != nil {
		state.ClearAuthCache()
		return
	}

	snapshot := make(map[string]*core.AccessToken)
	state.Inner.TokenRepository.Range(func(key string, value *core.AccessToken) bool {
		snapshot[strings.Clone(key)] = value
		return true
	})

	tokensPath := os.Getenv("RENOP_TOKENS")
	if tokensPath == "" {
		tokensPath = "tokens.yaml"
	}

	yamlData, err := yaml.Marshal(snapshot)
	if err == nil {
		tmpPath := tokensPath + ".tmp"
		if os.WriteFile(tmpPath, yamlData, 0600) == nil {
			_ = utils.SafeRename(tmpPath, tokensPath)
		}
	}

	state.ClearAuthCache()
}
