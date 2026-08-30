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

import {
    formatClickHouseDsn,
    formatMysqlDsn,
    formatPostgresDsn,
    parseClickHouseDsn,
    parseMysqlDsn,
    parsePostgresDsn
} from '../js/settings/database-dsn.js';

const repositoryRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..', '..', '..', '..', '..');
const frontendRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');

test('database settings expose structured native ClickHouse configuration', () => {
    const source = readFileSync(join(frontendRoot, 'js/settings.js'), 'utf8');
    const dsnModule = readFileSync(join(frontendRoot, 'js/settings/database-dsn.js'), 'utf8');
    for (const required of [
        "{value: 'clickhouse', label: 'ClickHouse'}",
        'parseClickHouseDsn(currentConfig.database.dsn)',
        'formatClickHouseDsn(clickHouseParts)',
        "secure=false&compress=lz4",
    ]) {
        assert.ok(source.includes(required), `ClickHouse settings are missing ${required}`);
    }
    assert.match(source, /from '\.\/settings\/database-dsn\.js'/);
    assert.match(dsnModule, /return `clickhouse:\/\//);
});

test('database DSN editors preserve credentials, parameters, and IPv6 hosts', () => {
    const mysql = parseMysqlDsn('renop:p@ss@tcp([::1]:3307)/packages?parseTime=true');
    assert.deepEqual(mysql, {
        user: 'renop', password: 'p@ss', host: '[::1]', port: '3307', database: 'packages',
        params: 'parseTime=true'
    });
    assert.equal(formatMysqlDsn(mysql), 'renop:p@ss@tcp([::1]:3307)/packages?parseTime=true');

    const postgres = parsePostgresDsn(
        'postgresql://renop%40user:p%3Ass@[::1]:5433/package%20db?sslmode=require');
    assert.deepEqual(postgres, {
        user: 'renop@user', password: 'p:ss', host: '[::1]', port: '5433', database: 'package db',
        params: 'sslmode=require'
    });
    assert.equal(formatPostgresDsn(postgres),
        'postgres://renop%40user:p%3Ass@[::1]:5433/package%20db?sslmode=require');

    const clickHouse = parseClickHouseDsn(
        'clickhouse://renop%40user:p%3Ass@[::1]:9440/package%20db?secure=true&compress=lz4');
    assert.deepEqual(clickHouse, {
        user: 'renop@user', password: 'p:ss', host: '[::1]', port: '9440', database: 'package db',
        params: 'secure=true&compress=lz4'
    });
    assert.equal(formatClickHouseDsn(clickHouse),
        'clickhouse://renop%40user:p%3Ass@[::1]:9440/package%20db?secure=true&compress=lz4');
});

test('database DSN parsers return independent defaults and normalize PostgreSQL keywords', () => {
    const first = parseMysqlDsn('');
    first.user = 'changed';
    assert.equal(parseMysqlDsn('').user, 'root');
    assert.equal(parseClickHouseDsn('invalid').user, 'default');
    assert.equal(parsePostgresDsn('postgres://[').user, 'postgres');

    const keywords = parsePostgresDsn(
        "host=db.local port=5433 user='renop user' password='p\\'ass' dbname=packages " +
        "application_name='RenoP Worker' sslmode=require");
    assert.deepEqual(keywords, {
        user: 'renop user', password: "p'ass", host: 'db.local', port: '5433', database: 'packages',
        params: 'application_name=RenoP%20Worker&sslmode=require'
    });
});

test('backend uses only the native ClickHouse API', () => {
    const nativeSource = readFileSync(join(repositoryRoot, 'internal/database/clickhouse.go'), 'utf8');
    const databaseSource = readFileSync(join(repositoryRoot, 'internal/database/db.go'), 'utf8');
    assert.ok(nativeSource.includes('clickhouse.Open(options)'));
    assert.doesNotMatch(nativeSource + databaseSource, /clickhouse\.OpenDB|sql\.Open\(["']clickhouse["']/,
        'ClickHouse must not use its database/sql compatibility API');
});
