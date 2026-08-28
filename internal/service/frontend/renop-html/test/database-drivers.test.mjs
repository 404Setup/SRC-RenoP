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

const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..', '..', '..', '..', '..');
const frontendRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');

test('database settings expose structured native ClickHouse configuration', () => {
    const source = readFileSync(join(frontendRoot, 'js/settings.js'), 'utf8');
    for (const required of [
        "{value: 'clickhouse', label: 'ClickHouse'}",
        'parseClickHouseDsn(currentConfig.database.dsn)',
        'formatClickHouseDsn(clickHouseParts)',
        'return `clickhouse://',
        "secure=false&compress=lz4",
    ]) {
        assert.ok(source.includes(required), `ClickHouse settings are missing ${required}`);
    }
});

test('backend uses only the native ClickHouse API', () => {
    const nativeSource = readFileSync(join(repositoryRoot, 'internal/database/clickhouse.go'), 'utf8');
    const databaseSource = readFileSync(join(repositoryRoot, 'internal/database/db.go'), 'utf8');
    assert.ok(nativeSource.includes('clickhouse.Open(options)'));
    assert.doesNotMatch(nativeSource + databaseSource, /clickhouse\.OpenDB|sql\.Open\(["']clickhouse["']/,
        'ClickHouse must not use its database/sql compatibility API');
});
