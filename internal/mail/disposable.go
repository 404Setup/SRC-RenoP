/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package mail

import (
	_ "embed"
	"slices"
	"strings"
	"sync"
)

//go:generate node ../../scripts/update-disposable-domains.mjs
//go:embed data/disposable_domains.txt
var disposableDomainData string

var disposableDomains = sync.OnceValue(func() []string { return strings.Fields(disposableDomainData) })

func isDisposableDomain(domain string) bool {
	for strings.Contains(domain, ".") {
		if _, found := slices.BinarySearch(disposableDomains(), domain); found {
			return true
		}
		_, domain, _ = strings.Cut(domain, ".")
	}
	return false
}
