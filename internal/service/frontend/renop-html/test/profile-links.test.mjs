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

const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), '..');
const repoRoot = resolve(frontendRoot, '..', '..', '..', '..');

/**
 * Read one embedded frontend source file.
 * @param {...string} parts - Path below the frontend root.
 * @returns {string} UTF-8 source.
 */
function source(...parts) {
    return readFileSync(join(frontendRoot, ...parts), 'utf8');
}

test('user and global-team profiles share bounded safe public links', () => {
    const links = source('js', 'profile-links.js');
    const profile = source('js', 'profile.js');
    const teams = source('js', 'super-teams.js');
    const core = readFileSync(join(repoRoot, 'internal', 'core', 'public_links.go'), 'utf8');
    assert.match(links, /target: '_blank', rel: 'noopener noreferrer nofollow'/);
    assert.match(links, /type, value, maxLength = 2048/);
    assert.match(links, /Boolean\(value\.custom_name\) === Boolean\(value\.custom_url\)/);
    assert.match(profile, /api\/auth\/profile\/links/);
    assert.match(profile, /createPublicProfileLinks\(profile\.links\)/);
    assert.match(teams, /createPublicProfileLinksEditor\(details\.team\.links\)/);
    assert.match(teams, /createPublicProfileLinks\(team\.links\)/);
    assert.match(core, /url\.ParseRequestURI/);
    assert.match(core, /parsed\.User != nil/);
    assert.match(core, /"github\.com"/);
    assert.match(core, /"discord\.com", "discord\.gg"/);
});

test('all bound package pages link to the global-team public route', () => {
    const links = source('js', 'profile-links.js');
    assert.match(links, /`\/team\/\$\{encodeURIComponent\(prefix\)\}`/);
    for (const format of ['cargo', 'docker', 'npm', 'maven']) {
        const script = source('js', 'browser', `${format}.js`);
        assert.match(script, /createSuperTeamPublicLink/);
        assert.match(script, /super_team_prefix/);
    }
});

test('every database schema persists the shared profile-link fields', () => {
    const schemas = [
        'dialect_sqlite.go', 'dialect_mysql.go', 'dialect_postgres.go', 'dialect.go', 'clickhouse_schema.go'
    ].map(file => readFileSync(join(repoRoot, 'internal', 'database', file), 'utf8'));
    for (const field of ['website_url', 'github_url', 'discord_url', 'custom_link_name', 'custom_link_url']) {
        for (const schema of schemas) assert.ok(schema.includes(field), `${field} missing from a database schema`);
    }
});
