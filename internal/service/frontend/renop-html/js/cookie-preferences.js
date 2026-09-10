/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

/** Read a bounded, unexpired choice for the exact policy revision. */
export function parseCookiePreferences(raw, revision, now = Date.now()) {
    if (typeof raw !== 'string' || raw.length > 4096) return null;
    try {
        const value = JSON.parse(raw);
        if (value?.revision !== revision || typeof value.optional !== 'boolean' ||
            !Number.isSafeInteger(value.expires) || value.expires <= now) return null;
        return {revision: value.revision, optional: value.optional, expires: value.expires};
    } catch {
        return null;
    }
}
