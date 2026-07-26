/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package core

import "go.yaml.in/yaml/v3"

type AccessTokenType string

const Persistent AccessTokenType = "PERSISTENT"

type AccessTokenIdentifier struct {
	Type  AccessTokenType `json:"type" yaml:"type"`
	Value int32           `json:"value" yaml:"value"`
}

type AccessToken struct {
	Identifier      AccessTokenIdentifier `json:"identifier" yaml:"identifier"`
	Name            string                `json:"name" yaml:"name"`
	EncryptedSecret string                `json:"encrypted_secret" yaml:"encrypted_secret"`
	PasswordHash    string                `json:"password_hash" yaml:"password_hash"`
	Tokens          []string              `json:"tokens" yaml:"tokens"`
	CreatedAt       string                `json:"created_at" yaml:"created_at"`
	Description     string                `json:"description" yaml:"description"`
	ExpiresAt       *int64                `json:"expires_at" yaml:"expires_at"`
	Permissions     []string              `json:"permissions" yaml:"permissions"`
}

func (t *AccessToken) UnmarshalYAML(value *yaml.Node) error {
	type alias AccessToken
	var aux alias
	if err := value.Decode(&aux); err != nil {
		return err
	}
	*t = AccessToken(aux)

	var rawMap map[string]any
	if err := value.Decode(&rawMap); err == nil {
		if val, ok := rawMap["encryptedsecret"]; ok {
			if s, ok := val.(string); ok {
				t.EncryptedSecret = s
			}
		}
		if val, ok := rawMap["passwordhash"]; ok {
			if s, ok := val.(string); ok {
				t.PasswordHash = s
			}
		}
		if val, ok := rawMap["createdat"]; ok {
			if s, ok := val.(string); ok {
				t.CreatedAt = s
			}
		}
		if val, ok := rawMap["expiresat"]; ok {
			if i, ok := val.(int64); ok {
				t.ExpiresAt = &i
			} else if i, ok := val.(int); ok {
				i64 := int64(i)
				t.ExpiresAt = &i64
			}
		}
	}
	return nil
}

type AccessTokenDto struct {
	Identifier  AccessTokenIdentifier `json:"identifier"`
	Name        string                `json:"name"`
	CreatedAt   string                `json:"created_at"`
	Description string                `json:"description"`
	ExpiresAt   *int64                `json:"expires_at"`
	Tokens      []string              `json:"tokens"`
	Permissions []string              `json:"permissions"`
}

type CreateAccessTokenRequest struct {
	Permissions []string `json:"permissions"`
	NewName     *string  `json:"new_name"`
	Secret      *string  `json:"secret"`
	IsCreate    bool     `json:"is_create"`
}

type CreateAccessTokenResponse struct {
	AccessToken AccessTokenDto `json:"access_token"`
	Secret      string         `json:"secret"`
}
