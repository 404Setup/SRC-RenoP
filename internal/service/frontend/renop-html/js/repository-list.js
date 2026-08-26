/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

import {getRepositoryFormat} from './repository-formats.js';

const repositoryNameCollator = new Intl.Collator('en', {numeric: true, sensitivity: 'base'});

/**
 * Return repository names in stable natural-name order, optionally limited to selected engines.
 * @param {Object.<string, object>} repositories - Repository settings keyed by route name.
 * @param {Iterable<string>} [selectedEngines=[]] - Canonical engine protocols to include.
 * @returns {string[]} Sorted repository names.
 */
export function sortedRepositoryNames(repositories, selectedEngines = []) {
    const selected = new Set(selectedEngines);
    return Object.keys(repositories || {})
        .filter(name => selected.size === 0 || selected.has(getRepositoryFormat(repositories[name]?.format).protocol))
        .sort(repositoryNameCollator.compare);
}

/**
 * Clamp and slice a repository-name list into one bounded page.
 * @param {string[]} names - Already sorted and filtered repository names.
 * @param {number} requestedPage - Zero-based requested page.
 * @param {number} [pageSize=10] - Maximum names per page.
 * @returns {{names: string[], page: number, pages: number, total: number, start: number, end: number}}
 */
export function paginateRepositoryNames(names, requestedPage, pageSize = 10) {
    const safeNames = Array.isArray(names) ? names : [];
    const safePageSize = Number.isInteger(pageSize) && pageSize > 0 ? pageSize : 10;
    const pages = Math.max(1, Math.ceil(safeNames.length / safePageSize));
    const requested = Number.isInteger(requestedPage) && requestedPage >= 0 ? requestedPage : 0;
    const page = Math.min(requested, pages - 1);
    const offset = page * safePageSize;
    const pageNames = safeNames.slice(offset, offset + safePageSize);
    return {
        names: pageNames,
        page,
        pages,
        total: safeNames.length,
        start: pageNames.length > 0 ? offset + 1 : 0,
        end: offset + pageNames.length
    };
}
