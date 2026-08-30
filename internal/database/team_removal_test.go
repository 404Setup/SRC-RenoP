/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package database_test

import (
	"math"
	"strings"
	"testing"

	"github.com/goccy/go-json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"renop/internal/database"
)

func requireTeamRemovalMessage(t *testing.T, db *database.DB, recipient, format, repository, resource, operator string) {
	t.Helper()
	messages, err := db.ListMessages(recipient, 10, 0, "package_team_removed", math.MaxInt64)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	message := messages[0]
	assert.Empty(t, message.Sender)
	var payload map[string]string
	require.NoError(t, json.Unmarshal(message.Payload, &payload))
	assert.Equal(t, map[string]string{
		"format": format, "repository": repository, "package": resource,
	}, payload)
	if operator != "" {
		operator = strings.ToLower(operator)
		assert.NotContains(t, strings.ToLower(message.Title), operator)
		assert.NotContains(t, strings.ToLower(message.Body), operator)
		assert.NotContains(t, strings.ToLower(string(message.Payload)), operator)
	}
}

func requireNoTeamRemovalMessage(t *testing.T, db *database.DB, recipient string) {
	t.Helper()
	messages, err := db.ListMessages(recipient, 10, 0, "package_team_removed", math.MaxInt64)
	require.NoError(t, err)
	assert.Empty(t, messages)
}
