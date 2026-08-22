/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the terms of the Mozilla Public License, v. 2.0.
 */

package cargo

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestIndexPath(t *testing.T) {
	for name, expected := range map[string]string{
		"a": "1/a", "ab": "2/ab", "abc": "3/a/abc",
		"Serde": "se/rd/serde", "my-crate": "my/-c/my-crate",
	} {
		if actual := indexPath(name); actual != expected {
			t.Errorf("indexPath(%q) = %q, want %q", name, actual, expected)
		}
	}
}

func TestRewriteIndexStreamsAppendAndRejectsEquivalentVersion(t *testing.T) {
	entry := IndexEntry{Name: "demo", Version: "1.0.0+build.1", Deps: []IndexDependency{}, Checksum: "abc", Features: map[string][]string{}}
	var first bytes.Buffer
	if err := rewriteIndex(nil, &first, entry); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(first.String(), "\n") || !strings.Contains(first.String(), `"vers":"1.0.0+build.1"`) {
		t.Fatalf("unexpected first index: %q", first.String())
	}

	entry.Version = "1.0.0+build.2"
	var duplicate bytes.Buffer
	if err := rewriteIndex(bytes.NewReader(first.Bytes()), &duplicate, entry); !errors.Is(err, errVersionExists) {
		t.Fatalf("equivalent version error = %v, want %v", err, errVersionExists)
	}
}

func TestRewriteYankedPreservesOtherEntries(t *testing.T) {
	existing := "{\"name\":\"demo\",\"vers\":\"1.0.0\",\"deps\":[],\"cksum\":\"a\",\"features\":{},\"yanked\":false}\n" +
		"{\"name\":\"demo\",\"vers\":\"2.0.0\",\"deps\":[],\"cksum\":\"b\",\"features\":{},\"yanked\":false}\n"
	var updated bytes.Buffer
	found, err := rewriteYanked(strings.NewReader(existing), &updated, "1.0.0", true)
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if !strings.Contains(updated.String(), `"vers":"1.0.0"`) || !strings.Contains(updated.String(), `"yanked":true`) ||
		!strings.Contains(updated.String(), `"vers":"2.0.0"`) {
		t.Fatalf("unexpected yanked index: %s", updated.String())
	}
}
