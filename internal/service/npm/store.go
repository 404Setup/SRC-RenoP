/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package npm

import "renop/internal/service/packagestore"

// Store is the shared streaming package persistence contract.
type Store = packagestore.Store

// StagedFile is the shared atomic package staging contract.
type StagedFile = packagestore.StagedFile
