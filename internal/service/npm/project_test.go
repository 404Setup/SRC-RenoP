/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package npm

import (
	"strings"
	"testing"

	"github.com/goccy/go-json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"renop/internal/core"
)

func TestEnrichNPMProjectMetadataNormalizesReadmePeopleAndLinks(t *testing.T) {
	manifest, err := json.Marshal(map[string]any{
		"name": "@team/demo", "version": "1.2.3",
		"readme": "# Demo\n\nSafe **README**.", "readmeFilename": "README.md",
		"license": "MPL-2.0", "homepage": "javascript:alert(1)",
		"repository": map[string]any{"type": "git", "url": "git+https://github.com/example/demo.git"},
		"bugs":       map[string]any{"url": "https://github.com/example/demo/issues"},
		"author":     "Alice Example <alice@example.test> (https://example.test/alice)",
		"contributors": []any{
			map[string]any{"name": "Bob", "url": "https://example.test/bob"},
			"Carol <carol@example.test>",
		},
		"maintainers": []any{map[string]any{"name": "Maintainer"}},
		"funding": []any{
			map[string]any{"type": "individual", "url": "https://fund.example.test/demo"},
			"data:text/html,unsafe",
		},
		"keywords":       []any{"registry", "RenoP", "registry"},
		"engines":        map[string]any{"node": ">=20"},
		"packageManager": "pnpm@11.20.0",
	})
	require.NoError(t, err)
	details := &core.NPMPackageDetails{
		Package:  &core.NPMPackage{LatestVersion: "1.2.3"},
		Versions: []*core.NPMVersion{{Version: "1.2.3", ManifestJSON: string(manifest)}},
	}
	enrichNPMProjectMetadata(details)
	require.NotNil(t, details.Project)
	assert.Equal(t, "# Demo\n\nSafe **README**.", details.Project.Readme)
	assert.Equal(t, "README.md", details.Project.ReadmeFilename)
	assert.Equal(t, "MPL-2.0", details.Project.License)
	assert.Empty(t, details.Project.Homepage)
	assert.Equal(t, "https://github.com/example/demo.git", details.Project.Repository)
	assert.Equal(t, "https://github.com/example/demo/issues", details.Project.Bugs)
	require.NotNil(t, details.Project.Author)
	assert.Equal(t, "Alice Example", details.Project.Author.Name)
	assert.Equal(t, "alice@example.test", details.Project.Author.Email)
	assert.Equal(t, "https://example.test/alice", details.Project.Author.URL)
	require.Len(t, details.Project.Contributors, 2)
	require.Len(t, details.Project.Maintainers, 1)
	assert.Equal(t, []string{"https://fund.example.test/demo"}, details.Project.Funding)
	assert.Equal(t, []string{"registry", "RenoP"}, details.Project.Keywords)
	assert.Equal(t, ">=20", details.Project.NodeEngine)
	assert.Equal(t, "pnpm@11.20.0", details.Project.PackageManager)
}

func TestNPMReadmeIsBoundedAndMissingSentinelIsHidden(t *testing.T) {
	assert.Empty(t, boundedNPMReadme("ERROR: No README data found!"))
	readme := boundedNPMReadme(strings.Repeat("界", maxNPMReadmeBytes))
	assert.LessOrEqual(t, len(readme), maxNPMReadmeBytes+len("\n\n…"))
	assert.True(t, strings.HasSuffix(readme, "…"))
}
