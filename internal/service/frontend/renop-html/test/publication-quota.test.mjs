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
const repositoryRoot = resolve(frontendRoot, '..', '..', '..', '..');

/**
 * Read one embedded frontend source file.
 * @param {string} relativePath - Frontend-relative path.
 * @returns {string} UTF-8 source.
 */
function frontendSource(relativePath) {
    return readFileSync(join(frontendRoot, relativePath), 'utf8');
}

test('publication quotas share one responsive account and global-team component', () => {
    const quota = frontendSource('js/publication-quota.js');
    const profile = frontendSource('js/profile.js');
    const users = frontendSource('js/users.js');
    const teams = frontendSource('js/super-teams.js');
    const settings = frontendSource('js/settings.js');
    const css = frontendSource('css/manager/publication-quota.css');
    assert.match(quota, /makeCustomSelect/);
    assert.match(quota, /createToggle/);
    assert.match(quota, /formatBytes/);
    assert.match(quota, /\/api\/publication-quota\/\$\{segment}/);
    assert.match(quota, /inherited \? \{\} :/);
    assert.match(quota, /unlimited/);
    assert.doesNotMatch(quota, /innerHTML|\.text\(\)/);
    assert.match(profile, /renderProfilePublicationQuota\(profile\.publication_quota\)/);
    assert.match(users, /ownerType: 'user'/);
    assert.match(teams, /ownerType: 'super_team'/);
    assert.match(settings, /'publication_quota'/);
    assert.match(settings, /renderPublicationQuotaSettings/);
    assert.match(css, /@media \(max-width: 680px\)/);
    assert.match(css, /grid-template-columns: 1fr/);
});

test('publication quota persistence and every local publication engine use the shared reservation boundary', () => {
    const schema = readFileSync(join(repositoryRoot, 'internal/database/dialect.go'), 'utf8');
    const database = readFileSync(join(repositoryRoot, 'internal/database/publication_quota.go'), 'utf8');
    for (const table of ['publication_quota_overrides', 'publication_quota_usage', 'publication_quota_reservations']) {
        assert.ok(schema.includes(table), `missing quota table ${table}`);
    }
    assert.match(database, /ReservePublicationQuota/);
    assert.match(database, /CommitPublicationQuotaReservation/);
    assert.match(database, /expires_at > \?/);
    for (const relativePath of [
        'internal/service/cargo/publish.go',
        'internal/service/npm/publish.go',
        'internal/service/docker/handler.go',
        'internal/service/storage/publication_quota.go',
    ]) {
        const source = readFileSync(join(repositoryRoot, relativePath), 'utf8');
        assert.match(source, /publicationquota\.(?:Reserve|Reservation)/,
            `${relativePath} does not use publication quota reservations`);
    }
});
