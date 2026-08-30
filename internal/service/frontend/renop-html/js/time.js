/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

/**
 * Normalize Unix seconds, Unix milliseconds, Date instances, or date strings.
 * @param {unknown} value - Timestamp candidate.
 * @returns {number} Milliseconds since epoch, or NaN when invalid.
 */
export function timestampMilliseconds(value) {
    if (value instanceof Date) return value.getTime();
    if (value === null || value === undefined || value === '') return Number.NaN;
    const numeric = Number(value);
    if (Number.isFinite(numeric)) return numeric > 1e11 ? numeric : numeric * 1000;
    return Date.parse(String(value));
}

/**
 * Format a supported timestamp using the current locale.
 * @param {unknown} value - Timestamp candidate.
 * @param {object} [options={}] - Formatting options.
 * @param {boolean} [options.dateOnly=false] - Return date without time.
 * @param {boolean} [options.timeOnly=false] - Return time without date.
 * @param {string} [options.fallback=''] - Value returned for invalid timestamps.
 * @returns {string} Localized timestamp or fallback.
 */
export function formatTimestamp(value, {dateOnly = false, timeOnly = false, fallback = ''} = {}) {
    const timestamp = timestampMilliseconds(value);
    if (!Number.isFinite(timestamp) || timestamp <= 0) return fallback;
    const date = new Date(timestamp);
    if (timeOnly) return date.toLocaleTimeString();
    return dateOnly ? date.toLocaleDateString() : date.toLocaleString();
}
