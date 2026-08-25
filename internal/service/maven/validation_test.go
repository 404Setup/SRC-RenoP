/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package maven

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"renop/internal/core"
)

func TestVerificationTarget(t *testing.T) {
	tests := []struct {
		domain, kind, target string
	}{
		{"com.example", core.MavenVerificationDNS, "example.com"},
		{"uk.co.example.project", core.MavenVerificationDNS, "example.co.uk"},
		{"io.github.example-user", core.MavenVerificationGitHub, "example-user"},
		{"io.gitlab.example_group", core.MavenVerificationGitLab, "example_group"},
	}
	for _, test := range tests {
		t.Run(test.domain, func(t *testing.T) {
			kind, target, err := VerificationTarget(test.domain)
			require.NoError(t, err)
			assert.Equal(t, test.kind, kind)
			assert.Equal(t, test.target, target)
		})
	}
	for _, invalid := range []string{"example", "com..example", "io.github.bad_name", "localhost.local", ".com.example"} {
		t.Run("invalid_"+invalid, func(t *testing.T) {
			_, _, err := VerificationTarget(invalid)
			assert.Error(t, err)
		})
	}
}

func TestParseArtifactPath(t *testing.T) {
	coordinate, ok := ParseArtifactPath("com/example/library/1.2.3/library-1.2.3.jar.sha256")
	require.True(t, ok)
	assert.Equal(t, "com.example", coordinate.GroupID)
	assert.Equal(t, "library", coordinate.ArtifactID)
	assert.Equal(t, "1.2.3", coordinate.Version)
	_, ok = ParseArtifactPath("com/example/library/maven-metadata.xml")
	assert.False(t, ok)
	_, ok = ParseArtifactPath("com/example/other/1.0/library-1.0.jar")
	assert.False(t, ok)
	assert.True(t, isMavenPublicationPath("com/example/library/maven-metadata.xml.sha256"))
	assert.True(t, isMavenPublicationPath("com/example/library/1.0/library-1.0.pom.asc"))
	assert.False(t, isMavenPublicationPath("com/example/readme.txt"))
	assert.False(t, isMavenPublicationPath("com/example/library/notes.txt"))
	assert.True(t, isMirroredMavenCompanion("com/example/library/1.0/library-1.0.jar.sha256"))
	assert.True(t, isMirroredMavenCompanion("com/example/library/1.0/library-1.0.pom.asc"))
	assert.False(t, isMirroredMavenCompanion("com/example/library/1.0/library-1.0.jar"))
}

func TestNewVerificationCodeIsUniqueAndBounded(t *testing.T) {
	first, err := NewVerificationCode()
	require.NoError(t, err)
	second, err := NewVerificationCode()
	require.NoError(t, err)
	assert.NotEqual(t, first, second)
	assert.Contains(t, first, "renop-verification=")
	assert.LessOrEqual(t, len(first), 128)
}

func TestVerificationTXTMatchesCurrentRecordAmongMultipleValues(t *testing.T) {
	records := []string{
		"google-site-verification=unrelated",
		"renop-verification=previous",
		"  renop-verification=current  ",
		"v=spf1 include:_spf.example.com ~all",
	}
	assert.True(t, verificationTXTMatches(records, "renop-verification=current"))
	assert.False(t, verificationTXTMatches(records, "renop-verification=missing"))
}
