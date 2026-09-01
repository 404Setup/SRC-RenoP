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
const source = relative => readFileSync(join(repositoryRoot, relative), 'utf8');

test('administrator account bans are modular, bounded, and reversible', () => {
    const ban = source('internal/service/frontend/renop-html/js/users/ban.js');
    const users = source('internal/service/frontend/renop-html/js/users.js');
    const row = source('internal/service/frontend/renop-html/js/components/user-row.js');
    const routes = source('internal/service/token/routes.go');

    assert.match(users, /from '\.\/users\/ban\.js'/);
    assert.match(ban, /type: 'datetime-local'/);
    assert.match(ban, /maxBanReasonLength = 512/);
    assert.match(ban, /method: 'PUT'/);
    assert.match(ban, /method: 'DELETE'/);
    assert.match(row, /token\.ban/);
    assert.match(row, /options\.onBan/);
    assert.match(routes, /ReadJSONLimited\(c, &request, 4096\)/);
    assert.match(routes, /ForgetUserSessions\(name\)/);
});

test('every authentication method shares the durable account-ban boundary', () => {
    for (const relative of [
        'internal/service/auth/api_token.go',
        'internal/service/auth/fido.go',
        'internal/service/auth/github_account.go',
        'internal/service/auth/middleware.go',
        'internal/service/auth/routes.go',
        'internal/service/auth/session_issue.go',
    ]) {
        assert.match(source(relative), /accountAccessError\(/, relative);
    }
    for (const relative of [
        'internal/database/dialect_sqlite.go',
        'internal/database/dialect_postgres.go',
        'internal/database/dialect_mysql.go',
        'internal/database/clickhouse_schema.go',
    ]) {
        const schema = source(relative);
        assert.match(schema, /ban_reason/);
        assert.match(schema, /banned_at/);
        assert.match(schema, /banned_until/);
    }
    assert.match(source('proto/api/v1/api.proto'), /AccountBan ban = 8;/);
});
