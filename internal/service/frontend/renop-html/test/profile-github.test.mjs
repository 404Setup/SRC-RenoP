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
import {dirname, join, resolve} from 'node:path';
import test from 'node:test';
import {fileURLToPath} from 'node:url';

const frontendRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const repositoryRoot = resolve(frontendRoot, '..', '..', '..', '..');

test('own profile payload renders GitHub state without a delayed profile request', () => {
    const profile = readFileSync(join(frontendRoot, 'js/profile.js'), 'utf8');
    const github = readFileSync(join(frontendRoot, 'js/github-auth.js'), 'utf8');
    const backend = readFileSync(join(repositoryRoot, 'internal/service/auth/user_profile.go'), 'utf8');

    assert.match(backend, /GitHub\s+\*githubProfileStatus\s+`json:"github,omitempty"`/);
    assert.match(backend, /profileResponseWithConnections/);
    assert.match(profile, /renderGitHubConnection\(profile\.github\)/);
    assert.ok(profile.indexOf('renderGitHubConnection(profile.github)') <
        profile.indexOf('void refreshAccountSecurity()', profile.indexOf('function showProfileEdit')));
    assert.doesNotMatch(github, /apiRequest\('\/api\/auth\/profile\/github'\s*\)/);
    assert.match(github, /renderGitHubConnection\(nextStatus\)/);
    assert.match(github, /\$\(window\)\.on\('languageChanged'/);
    assert.match(github, /\$\(window\)\.on\('accountSecurityUpdated'/);
    assert.match(github, /security\.fido_device_count/);
});

test('profile edit cards use a compact responsive grid', () => {
    const styles = readFileSync(join(frontendRoot, 'css/manager/profile.css'), 'utf8');
    assert.match(styles, /\.profile-settings-card\s*\{[^}]*display: grid;[^}]*grid-template-columns: repeat\(2,/s);
    assert.match(styles, /\.profile-identity-card,[\s\S]*?#profile-github-section\s*\{[^}]*grid-column: 1 \/ -1;/);
    assert.match(styles, /@media \(max-width: 820px\)[\s\S]*?\.profile-settings-card\s*\{[^}]*grid-template-columns: minmax\(0, 1fr\)/);
    assert.match(styles, /#profile-edit-view \.profile-hero\s*\{[^}]*margin-bottom: 1\.25rem;/s);
});
