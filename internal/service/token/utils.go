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
	"log"
	"os"
	"time"

	"github.com/3JoB/unsafeConvert"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"renop/internal/core"
)

func UpdateTokenSync(opChan chan<- TokenOp, name string, updateFn func(*core.AccessToken)) error {
	errChan := make(chan error, 1)
	opChan <- TokenOp{
		Type:     OpTokenUpdate,
		Name:     name,
		UpdateFn: updateFn,
		ErrChan:  errChan,
	}
	return <-errChan
}

func AutoRegisterAdmin(state *core.AppState, opChan chan<- TokenOp) {
	if state.Inner.TokensCount.Load() == 0 {
		defaultPassword := os.Getenv("RENOP_DEFAULT_ADMIN_PASSWORD")
		if defaultPassword == "" {
			defaultPassword = uuid.NewString()
			log.Printf("WARNING: RENOP_DEFAULT_ADMIN_PASSWORD is not set. Generated a random password: %s. Please secure your instance immediately.\n", defaultPassword)
		}

		hashBytes, err := bcrypt.GenerateFromPassword([]byte(defaultPassword), bcrypt.DefaultCost)
		if err != nil {
			return
		}
		encryptedSecret := unsafeConvert.StringPointer(hashBytes)

		token := &core.AccessToken{
			Identifier: core.AccessTokenIdentifier{
				Type:  core.Persistent,
				Value: 1,
			},
			Name:            "admin",
			EncryptedSecret: encryptedSecret,
			PasswordHash:    "",
			Tokens:          []string{},
			CreatedAt:       time.Now().UTC().Format(time.RFC3339),
			Description:     "Auto-registered admin",
			ExpiresAt:       nil,
			Permissions:     []string{"admin"},
		}

		errChan := make(chan error, 1)
		opChan <- TokenOp{
			Type:    OpTokenStore,
			Name:    "admin",
			Token:   token,
			ErrChan: errChan,
		}
		<-errChan
	}
}
