/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package pb

// Regenerate from the repository root (preferred):
//
//	protoc -I proto --go_out=. --go_opt=module=renop \
//	  proto/api/v1/api.proto \
//	  proto/storage/v1/session.proto
//
// Or via go generate from this package directory:
//
//go:generate protoc -I ../proto --go_out=.. --go_opt=module=renop ../proto/api/v1/api.proto ../proto/storage/v1/session.proto
