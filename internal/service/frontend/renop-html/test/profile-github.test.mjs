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
    assert.match(backend, /profileResponseWithPrivateDetails/);
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
    assert.match(styles, /\.profile-settings-card\s*\{[^}]*grid-auto-flow: dense;/s);
    assert.match(styles, /\.profile-identity-card,[\s\S]*?#profile-github-section\s*\{[^}]*grid-column: 1 \/ -1;/);
    assert.match(styles, /@media \(max-width: 820px\)[\s\S]*?\.profile-settings-card\s*\{[^}]*grid-template-columns: minmax\(0, 1fr\)/);
    assert.match(styles, /#profile-edit-view \.profile-hero\s*\{[^}]*margin-bottom: 1\.25rem;/s);
});

test('mobile public profiles keep identity copy above the edit action', () => {
    const profile = readFileSync(join(frontendRoot, 'js/profile.js'), 'utf8');
    const styles = readFileSync(join(frontendRoot, 'css/manager/profile.css'), 'utf8');
    const heading = profile.slice(profile.indexOf("class: 'profile-public-heading'"),
        profile.indexOf("class: 'profile-public-meta'"));
    assert.ok(heading.indexOf("class: 'profile-public-username'") < heading.lastIndexOf('actions'));
    assert.ok(heading.indexOf("class: 'profile-public-description'") < heading.lastIndexOf('actions'));
    assert.ok(heading.indexOf('createPublicProfileLinks(profile.links)') < heading.lastIndexOf('actions'));
    assert.match(styles, /\.profile-public-heading\s*\{[^}]*display: grid;[^}]*grid-template-columns:/s);
    assert.match(styles, /@media \(max-width: 640px\)[\s\S]*?\.profile-public-actions\s*\{[^}]*grid-row: auto;/);
});

test('password controls live inside the account security card', () => {
    const page = readFileSync(join(frontendRoot, 'index.html'), 'utf8');
    const security = readFileSync(join(frontendRoot, 'js/account-security.js'), 'utf8');
    const securityStart = page.indexOf('id="profile-account-security-section"');
    const securityEnd = page.indexOf('id="profile-api-token-section"');
    assert.ok(securityStart >= 0 && securityEnd > securityStart);
    assert.match(page.slice(securityStart, securityEnd), /id="profile-password-form"/);
    assert.equal(page.indexOf('id="profile-password-form"'), page.lastIndexOf('id="profile-password-form"'));
    assert.doesNotMatch(page.slice(securityStart - 120, securityStart), /hidden/);
    assert.match(security, /\$\(section\)\.prop\('hidden', false\)/);
    assert.doesNotMatch(security, /catch[\s\S]*?\$\(section\)\.prop\('hidden', true\)/);
});

test('authorized private panels render on the profile home instead of the editor', () => {
    const page = readFileSync(join(frontendRoot, 'index.html'), 'utf8');
    const profile = readFileSync(join(frontendRoot, 'js/profile.js'), 'utf8');
    const styles = readFileSync(join(frontendRoot, 'css/manager/profile.css'), 'utf8');
    const backend = readFileSync(join(repositoryRoot, 'internal/service/auth/user_profile.go'), 'utf8');
    assert.match(backend, /PrivateDetails\s+bool\s+`json:"private_details,omitempty"`/);
    assert.match(backend, /own \|\| administrator/);
    assert.match(profile, /if \(profile\.private_details\)/);
    assert.match(profile, /class: 'profile-private-grid'/);
    assert.match(profile, /createPublicationQuotaPanel\(profile\.publication_quota/);
    assert.match(profile, /createProfileSuperTeamLimits\(profile\.super_team_limits/);
    assert.match(profile, /query\.set\('username', view\.username\)/);
    assert.match(styles, /\.profile-private-grid\s*\{[^}]*grid-template-columns: repeat\(2,/s);
    assert.match(styles, /@media \(max-width: 820px\)[\s\S]*?\.profile-private-grid\s*\{[^}]*grid-template-columns: minmax\(0, 1fr\)/);
    assert.doesNotMatch(page, /id="btn-profile-gpg(?:-releases)?"/);
});
