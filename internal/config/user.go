/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package config

import (
	"strings"
	"sync"

	"github.com/goccy/go-json"
	"go.yaml.in/yaml/v3"
)

type User struct {
	Username         string   `json:"username" yaml:"username"`
	PasswordHash     string   `json:"password_hash" yaml:"password_hash"`
	Tokens           []string `json:"tokens" yaml:"tokens"`
	Roles            []string `json:"roles" yaml:"roles"`
	ReadPermissions  []string `json:"read_permissions" yaml:"read_permissions"`
	WritePermissions []string `json:"write_permissions" yaml:"write_permissions"`

	once           sync.Once       `json:"-" yaml:"-"`
	isAdmin        bool            `json:"-" yaml:"-"`
	isAllView      bool            `json:"-" yaml:"-"`
	isShowing      bool            `json:"-" yaml:"-"`
	canUpdateAll   bool            `json:"-" yaml:"-"`
	updateRepos    map[string]bool `json:"-" yaml:"-"`
	canModerateAll bool            `json:"-" yaml:"-"`
	moderateRepos  map[string]bool `json:"-" yaml:"-"`
	canViewAll     bool            `json:"-" yaml:"-"`
	viewRepos      map[string]bool `json:"-" yaml:"-"`
	isManager      bool            `json:"-"  yaml:"-"`
}

func (u *User) IsManager() bool {
	u.initPermissions()
	return u.isManager
}

func (u *User) setDefaults() {
	if u.Tokens == nil {
		u.Tokens = []string{}
	}
	if u.Roles == nil {
		u.Roles = []string{}
	}
	if u.ReadPermissions == nil {
		u.ReadPermissions = []string{}
	}
	if u.WritePermissions == nil {
		u.WritePermissions = []string{}
	}
}

func (u *User) UnmarshalJSON(data []byte) error {
	u.setDefaults()
	type alias User
	aux := (*alias)(u)
	return json.Unmarshal(data, aux)
}

func (u *User) UnmarshalYAML(value *yaml.Node) error {
	u.setDefaults()
	type alias User
	aux := (*alias)(u)
	return value.Decode(aux)
}

func (u *User) DeepCopy() *User {
	if u == nil {
		return nil
	}
	cloned := &User{
		Username:     strings.Clone(u.Username),
		PasswordHash: strings.Clone(u.PasswordHash),
	}
	if u.Tokens != nil {
		cloned.Tokens = make([]string, len(u.Tokens))
		for i, t := range u.Tokens {
			cloned.Tokens[i] = strings.Clone(t)
		}
	}
	if u.Roles != nil {
		cloned.Roles = make([]string, len(u.Roles))
		for i, r := range u.Roles {
			cloned.Roles[i] = strings.Clone(r)
		}
	}
	if u.ReadPermissions != nil {
		cloned.ReadPermissions = make([]string, len(u.ReadPermissions))
		for i, p := range u.ReadPermissions {
			cloned.ReadPermissions[i] = strings.Clone(p)
		}
	}
	if u.WritePermissions != nil {
		cloned.WritePermissions = make([]string, len(u.WritePermissions))
		for i, p := range u.WritePermissions {
			cloned.WritePermissions[i] = strings.Clone(p)
		}
	}
	return cloned
}
