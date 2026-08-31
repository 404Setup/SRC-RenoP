/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {dirname, join} from 'node:path';
import test from 'node:test';
import {fileURLToPath} from 'node:url';

const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), '..');

test('profile and prefetch caches have explicit lifecycle and capacity bounds', () => {
    const profiles = readFileSync(join(frontendRoot, 'js/user-profiles.js'), 'utf8');
    const auth = readFileSync(join(frontendRoot, 'js/auth.js'), 'utf8');
    const browser = readFileSync(join(frontendRoot, 'js/browser.js'), 'utf8');

    assert.match(profiles, /PROFILE_CACHE_MAX_ENTRIES = 256/);
    assert.match(profiles, /cached\.expiresAt <= now/);
    assert.match(profiles, /profileCache\.delete\(profileCache\.keys\(\)\.next\(\)\.value\)/);
    assert.match(profiles, /generation === profileCacheGeneration/);
    assert.match(profiles, /profileRequests\.get\(normalized\) === request/);
    assert.match(profiles, /export function clearUserProfileCache\(\)/);
    assert.match(profiles, /profileRequests\.clear\(\)/);
    assert.match(auth, /clearUserProfileCache\(\)/);
    assert.match(auth, /navProfileLoadSequence\+\+/);

    assert.match(browser, /MAX_PREFETCH_CACHE_ENTRIES = 128/);
    assert.match(browser, /prefetchCache\.get\(oldest\)\?\.remove\(\)/);
    assert.match(browser, /link\.addEventListener\('load',[\s\S]*?link\.remove\(\)/);
    assert.match(browser, /link\.addEventListener\('error',[\s\S]*?prefetchCache\.delete\(url\)/);
});
