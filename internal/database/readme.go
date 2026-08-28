/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package database

import "strings"

const maxPackageReadmeBytes = 512 << 10

func sanitizePackageReadme(readme string) string {
	return strings.TrimSpace(SanitizeInputString(readme, maxPackageReadmeBytes))
}
