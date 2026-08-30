/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import test from 'node:test';
import assert from 'node:assert/strict';

import {paginateRepositoryNames, sortedRepositoryNames} from '../js/repository-list.js';

test('repository list sorts naturally and filters classic Maven by its engine', () => {
    const repositories = {
        zeta: {format: 'docker'},
        'alpha-10': {format: 'cargo'},
        alpha: {format: 'maven-classic'},
        'alpha-2': {format: 'files'}
    };
    assert.deepEqual(sortedRepositoryNames(repositories), ['alpha', 'alpha-2', 'alpha-10', 'zeta']);
    assert.deepEqual(sortedRepositoryNames(repositories, ['maven']), ['alpha']);
    assert.deepEqual(sortedRepositoryNames(repositories, ['cargo', 'docker']), ['alpha-10', 'zeta']);
});

test('repository pagination clamps stale pages after filtering or deletion', () => {
    const names = Array.from({length: 23}, (_, index) => `repo-${index + 1}`);
    assert.deepEqual(paginateRepositoryNames(names, 99, 10), {
        names: ['repo-21', 'repo-22', 'repo-23'], page: 2, pages: 3, total: 23, start: 21, end: 23
    });
    assert.deepEqual(paginateRepositoryNames([], 3, 10), {
        names: [], page: 0, pages: 1, total: 0, start: 0, end: 0
    });
});
