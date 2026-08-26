/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

// Package version exposes build-time RenoP version metadata.
package version

import "strconv"

// Version is the release version or the full source revision used for a
// development build.
var Version = "dev"

// Development is set to "true" for development builds. It is a string so it
// can be replaced by the Go linker's -X flag.
var Development = "true"

// Commit is the full source revision embedded by the release build.
var Commit = "dev"

// PreviousCommit is the preceding formal release revision when one exists.
var PreviousCommit = ""

// IsDevelopment reports whether this binary was built as a development build.
func IsDevelopment() bool {
	value, err := strconv.ParseBool(Development)
	return err == nil && value
}
