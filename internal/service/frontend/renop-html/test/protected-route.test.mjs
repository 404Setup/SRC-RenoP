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

/**
 * Read one source file below the embedded frontend root.
 * @param {string} relativePath - Frontend-relative file path.
 * @returns {string} UTF-8 source.
 */
const source = relativePath => readFileSync(join(frontendRoot, relativePath), 'utf8');

test('protected route denial has one shell-owned home replacement boundary', () => {
    const boundary = source('js/protected-route.js');
    const main = source('js/main.js');
    const auth = source('js/auth.js');
    assert.match(boundary, /protectedRouteDeniedEvent = 'renop:protected-route-denied'/);
    assert.match(boundary, /status !== 401 && status !== 403/);
    assert.match(main, /window\.addEventListener\(protectedRouteDeniedEvent/);
    assert.match(main, /navigateHome\(\{replace: true}\)/);
    assert.match(main, /if \(replace\) window\.history\.replaceState\(null, '', '\/'\)/);
    assert.match(auth, /let logoutPromise = null/);
    assert.match(auth, /requestProtectedRouteExit\(401\)[\s\S]*?if \(logoutPromise\) return logoutPromise/);
});

test('protected account loaders abandon denied pages without logging out valid sessions', () => {
    for (const relativePath of ['js/reviews.js', 'js/super-teams.js', 'js/browser/maven.js']) {
        const moduleSource = source(relativePath);
        assert.match(moduleSource, /exitProtectedRouteOnDenial/,
            `${relativePath} does not apply the protected route boundary`);
        assert.match(moduleSource, /error\?\.message === 'Unauthorized'/,
            `${relativePath} can render an error after session invalidation`);
    }
    const main = source('js/main.js');
    assert.match(main, /const routedAccountTab = accountTabFromPath\(\)/);
    assert.match(main, /!cachedIsLoggedIn && \(isAccountTab\(tabId\) \|\| routedAccountTab\)/);
    assert.match(main, /if \(routedAccountTab\) window\.history\.replaceState\(null, '', '\/'\)/);
    for (const relativePath of ['js/dashboard.js', 'js/repositories.js', 'js/settings.js', 'js/users.js']) {
        const moduleSource = source(relativePath);
        assert.match(moduleSource, /exitProtectedRouteOnDenial/,
            `${relativePath} does not leave a denied manager section`);
        assert.match(moduleSource, /response\.status === 401\) void logout\('kicked'\)/,
            `${relativePath} still conflates permission denial with session expiry`);
    }
});
