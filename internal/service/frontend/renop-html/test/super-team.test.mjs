/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
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

test('global teams use a routed account center and embedded profile limits', () => {
    const html = source('index.html');
    const main = source('js', 'main.js');
    const profile = source('js', 'profile.js');
    assert.match(html, /data-account-action="super-teams"/);
    assert.match(html, /id="tab-content-super-teams"/);
    assert.match(main, /superTeamRouteFromPath/);
    assert.match(main, /loadSuperTeamCenterPage/);
    assert.match(main, /isAccountTab\(tabId\)/);
    assert.match(profile, /renderProfileSuperTeamLimits\(profile\.super_team_limits\)/);
});

test('global team controls share bounded dialogs, user suggestions, errors, and mobile layout', () => {
    const script = source('js', 'super-teams.js');
    const messages = source('js', 'super-team-messages.js');
    const styles = source('css', 'manager', 'super-teams.css');
    assert.match(script, /new RepositoryUserSuggestions/);
    assert.match(script, /makeCustomSelect\(roleOptions/);
    assert.match(script, /RenopDialog\.show/);
    assert.match(script, /SUPER_TEAM_ERROR_KEYS/);
    assert.match(script, /morphElementHeight/);
    assert.match(messages, /registerMessageActionHandler\('super_team_invite'/);
    assert.match(styles, /@media \(max-width: 560px\)/);
    assert.match(styles, /\.super-team-member-row\s*\{[\s\S]*?grid-template-columns/);
    assert.match(styles, /\.super-team-member-controls \.icon-btn\.is-danger\s*\{[\s\S]*?width:\s*2rem[\s\S]*?border-radius:\s*50%/);
});

test('global teams persist independently from package membership tables', () => {
    const schema = readFileSync(join(repoRoot, 'internal', 'database', 'dialect.go'), 'utf8');
    const clickhouse = readFileSync(join(repoRoot, 'internal', 'database', 'clickhouse_schema.go'), 'utf8');
    const database = readFileSync(join(repoRoot, 'internal', 'database', 'super_team.go'), 'utf8');
    for (const table of ['super_teams', 'super_team_members', 'super_team_invitations', 'user_super_team_limits']) {
        assert.ok(schema.includes(table), `missing SQL schema ${table}`);
        assert.ok(clickhouse.includes(`name: "${table}"`), `missing ClickHouse schema ${table}`);
    }
    assert.match(database, /team_prefix, user_id, role_level/);
    assert.doesNotMatch(database, /INSERT INTO (?:npm|docker|cargo)_members/);
});
