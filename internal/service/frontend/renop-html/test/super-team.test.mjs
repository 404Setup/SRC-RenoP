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

test('global teams use a routed account center and embedded profile limits', () => {
    const html = source('index.html');
    const main = source('js', 'main.js');
    const profile = source('js', 'profile.js');
    assert.match(html, /data-account-action="super-teams"/);
    assert.match(html, /id="tab-content-super-teams"/);
    assert.match(main, /superTeamRouteFromPath/);
    assert.match(main, /loadSuperTeamCenterPage/);
    assert.match(main, /isAccountTab\(tabId\)/);
    assert.match(profile, /createProfileSuperTeamLimits\(profile\.super_team_limits/);
    assert.match(profile, /editable: administrator/);
    assert.match(profile, /profile\.super_team_limits = saved/);
    const superTeams = source('js', 'super-teams.js');
    assert.match(superTeams, /api\/super-teams\/users\/\$\{encodeURIComponent\(username\)\}\/limits/);
    assert.match(superTeams, /Number\.isSafeInteger\(value\)/);
});

test('global teams expose a standalone read-only public route', () => {
    const frontendRoutes = readFileSync(join(repoRoot, 'internal', 'service', 'frontend', 'mod.go'), 'utf8');
    const mainSource = source('js', 'main.js');
    const superTeamSource = source('js', 'super-teams.js');
    assert.match(frontendRoutes, /app\.Get\("\/team\/:prefix"/);
    assert.match(mainSource, /publicSuperTeamRouteFromPath/);
    assert.match(superTeamSource, /const publicRouteRoot = '\/team'/);
    assert.match(superTeamSource, /export function publicSuperTeamRouteFromPath/);
    assert.match(superTeamSource, /export async function loadPublicSuperTeamPage/);
    assert.match(superTeamSource, /teamDetailContent\(details, route\.prefix, \{publicView: true\}\)/);
    assert.match(superTeamSource, /const canManage = !readOnly/);
    assert.doesNotMatch(superTeamSource, /loadPublicSuperTeamPage[\s\S]*?publication-quota\/super-teams/);
});

test('global team controls share bounded dialogs, user suggestions, errors, and mobile layout', () => {
    const script = source('js', 'super-teams.js');
    const messages = source('js', 'super-team-messages.js');
    const styles = source('css', 'manager', 'super-teams.css');
    assert.match(script, /new RepositoryUserSuggestions/);
    assert.match(script, /makeCustomSelect\(roleOptions/);
    assert.match(script, /RenopDialog\.show/);
    assert.match(script, /id: 'super-team-leave-dialog'/);
    assert.match(script, /\/membership`, \{method: 'DELETE'\}/);
    assert.doesNotMatch(script, /own \|\| canManage/);
    assert.match(script, /SUPER_TEAM_ERROR_KEYS/);
    assert.match(script, /morphElementHeight/);
    assert.match(script, /createUserIdentity\(member\.username, \{avatar: true\}\)/);
    assert.match(script, /createUserIdentity\(team\.created_by, \{template: 'superTeam\.createdBy'\}\)/);
    assert.doesNotMatch(script, /String\(member\.username \|\| '\?'\)\[0\]/);
    assert.match(messages, /registerMessageActionHandler\('super_team_invite'/);
    assert.match(styles, /@media \(max-width: 560px\)/);
    assert.match(styles, /\.super-team-member-row\s*\{[\s\S]*?grid-template-columns/);
    assert.match(styles, /\.super-team-member-controls \.custom-select-wrapper/);
    assert.match(styles, /\.super-team-member-controls \.icon-btn\.is-danger\s*\{[\s\S]*?width:\s*2rem[\s\S]*?border-radius:\s*50%/);
    assert.match(styles, /\.super-team-detail-hero \+ \.publication-quota-panel\s*\{[^}]*margin-top:/s);
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
    assert.match(schema, /super_team_members[\s\S]*?public_visible/);
    assert.match(clickhouse, /name: "super_team_members"[\s\S]*?public_visible/);
    assert.doesNotMatch(database, /INSERT INTO (?:npm|docker|cargo)_members/);
});

test('global-team membership visibility is self-managed and manager-aware', () => {
    const script = source('js', 'super-teams.js');
    const profile = source('js', 'profile.js');
    const database = readFileSync(join(repoRoot, 'internal', 'database', 'super_team.go'), 'utf8');
    const routes = readFileSync(join(repoRoot, 'internal', 'service', 'superteam', 'routes.go'), 'utf8');
    assert.match(script, /membership\/visibility/);
    assert.match(script, /\?manage=true/);
    assert.match(script, /member\.visible === false/);
    assert.match(script, /superTeam\.hideMembership/);
    assert.match(profile, /api\/users\/\$\{encodeURIComponent\(profile\.username\)\}\/super-teams/);
    assert.match(profile, /createProfileSuperTeamSection\(profile\)/);
    assert.match(database, /target\.public_visible = 1 OR \? = 1 OR/);
    assert.match(database, /viewer_member\.role_level, 0\) >= \?/);
    assert.match(database, /team\.RoleLevel < core\.SuperTeamRoleManage/);
    assert.match(routes, /SetSuperTeamMemberVisibility\(prefix, user\.Username/);
});

test('global-team profiles use a separate bounded cross-format resource module', () => {
    const teams = source('js', 'super-teams.js');
    const resources = source('js', 'super-team-resources.js');
    const links = source('js', 'profile-links.js');
    const database = readFileSync(join(repoRoot, 'internal', 'database', 'super_team_resources.go'), 'utf8');
    const routes = readFileSync(join(repoRoot, 'internal', 'service', 'superteam', 'routes.go'), 'utf8');
    assert.match(teams, /createSuperTeamResourcesSection\(team\.prefix \|\| prefix\)/);
    assert.match(resources, /const resourcePageSize = 8/);
    for (const format of ['maven', 'cargo', 'docker', 'npm']) {
        assert.match(resources, new RegExp(`\\['${format}'`));
    }
    assert.match(resources, /api\/super-teams\/\$\{encodeURIComponent\(prefix\)\}\/resources/);
    assert.match(resources, /panel\.remove\(\)/);
    assert.match(resources, /packageResourceTarget\(resource\)/);
    assert.match(links, /export function packageResourceTarget/);
    assert.match(database, /options\.Limit > 50/);
    assert.match(database, /resource\.private = 1/);
    assert.match(database, /team_member\.user_id IS NOT NULL/);
    assert.match(database, /SUBSTR\(resource\.description, 1, 4000\)/);
    assert.match(routes, /auth\.CurrentCredentialIsAPIToken\(c\)/);
});

test('package creation includes T2 approval while domain binding remains T3', () => {
    const selector = source('js', 'super-team-selector.js');
    const docker = source('js', 'browser', 'docker.js');
    const npm = source('js', 'browser', 'npm.js');
    const maven = source('js', 'browser', 'maven.js');
    const schema = readFileSync(join(repoRoot, 'internal', 'database', 'dialect.go'), 'utf8');
    const sqliteSchema = readFileSync(join(repoRoot, 'internal', 'database', 'dialect_sqlite.go'), 'utf8');
    assert.match(selector, /minimumRole = 3/);
    assert.match(selector, /super-teams\/eligible\?minimum_role=\$\{boundedRole\}/);
    assert.match(selector, /makeCustomSelect/);
    for (const script of [docker, npm, maven]) {
        assert.match(script, /createSuperTeamBindingField/);
        assert.match(script, /super_team_prefix/);
    }
    assert.match(docker, /createSuperTeamBindingField\(\{minimumRole: 2}\)/);
    assert.match(npm, /createSuperTeamBindingField\(\{minimumRole: 2}\)/);
    assert.doesNotMatch(maven, /createSuperTeamBindingField\(\{minimumRole: 2}\)/);
    for (const table of ['npm_packages', 'maven_domains', 'maven_artifacts']) {
        assert.match(schema, new RegExp(`${table}[\\s\\S]*?super_team_prefix`));
    }
    for (const table of ['cargo_packages', 'docker_images']) {
        assert.match(sqliteSchema, new RegExp(`${table}[\\s\\S]*?super_team_prefix`));
    }
});

test('global team settings remain inside the merged service domain', () => {
    const settings = source('js', 'settings.js');
    assert.match(settings, /SERVICE_DOMAINS[\s\S]*?'super_teams'/);
    assert.match(settings, /MERGED_SERVICE_DOMAINS/);
    assert.match(settings, /!MERGED_SERVICE_DOMAINS\.has\(domain\)/);
    assert.match(settings, /fetchSuperTeamSettings/);
    assert.match(settings, /renderSuperTeamSettings/);
    assert.doesNotMatch(settings, /DOMAIN_MESSAGE_TYPES[\s\S]*?super_teams:/);
});
