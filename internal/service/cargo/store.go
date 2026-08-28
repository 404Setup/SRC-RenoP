/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the terms of the Mozilla Public License, v. 2.0.
 */

package cargo

import "renop/internal/service/packagestore"

// StagedFile is the shared streaming package staging contract.
type StagedFile = packagestore.StagedFile

// Store is the shared streaming package persistence contract.
type Store = packagestore.Store
