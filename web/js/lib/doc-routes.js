/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

/** Map a locale-relative source slug to its public page. @param {string} slug */
export function docURL(slug) {
    return slug.startsWith('api/') ? `/${slug}` : `/docs/${slug}`;
}

/** Preserve old API bookmarks, including their query and fragment. @param {URL} url */
export function canonicalPageURL(url) {
    const path = url.pathname.replace(/^\/docs\/api(?=\/|$)/, '/api');
    return path + url.search + url.hash;
}
