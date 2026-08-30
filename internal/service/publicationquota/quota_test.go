/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package publicationquota

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"renop/internal/core"
)

func TestSubjectUsesBoundGlobalTeamOtherwiseAccount(t *testing.T) {
	assert.Equal(t, core.PublicationQuotaSubject{
		OwnerType: core.PublicationQuotaOwnerSuperTeam, OwnerKey: "platform",
	}, Subject("alice", "Platform"))
	assert.Equal(t, core.PublicationQuotaSubject{
		OwnerType: core.PublicationQuotaOwnerUser, OwnerKey: "alice",
	}, Subject(" Alice ", "invalid/team"))
}
