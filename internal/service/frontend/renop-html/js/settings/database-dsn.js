/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

const mysqlDefaults = Object.freeze({
    user: 'root', password: '', host: '127.0.0.1', port: '3306', database: 'renop',
    params: 'charset=utf8mb4&parseTime=True&loc=Local'
});
const postgresDefaults = Object.freeze({
    user: 'postgres', password: '', host: '127.0.0.1', port: '5432', database: 'renop',
    params: 'sslmode=disable'
});
const clickHouseDefaults = Object.freeze({
    user: 'default', password: '', host: '127.0.0.1', port: '9000', database: 'default', params: ''
});

/**
 * Return an independent editable copy of immutable DSN defaults.
 * @param {object} defaults - Driver defaults.
 * @returns {object} Editable fields.
 */
function copyDefaults(defaults) {
    return {...defaults};
}

/**
 * Format a host for a URL authority while preserving already bracketed IPv6 literals.
 * @param {unknown} value - User-supplied host.
 * @param {string} fallback - Driver default host.
 * @returns {string} URL-safe authority host.
 */
function formatURLHost(value, fallback) {
    const host = String(value || fallback).trim() || fallback;
    return host.includes(':') && !(host.startsWith('[') && host.endsWith(']')) ? `[${host}]` : host;
}

/**
 * Parse a MySQL DSN into editable connection fields.
 * @param {string} dsnStr - MySQL DSN.
 * @returns {object} Parsed fields.
 */
export function parseMysqlDsn(dsnStr) {
    if (typeof dsnStr !== 'string' || !dsnStr.trim()) return copyDefaults(mysqlDefaults);
    let rest = dsnStr.trim();
    let user = '';
    let password = '';
    let host = mysqlDefaults.host;
    let port = mysqlDefaults.port;
    let database = '';
    let params = '';

    const atIndex = rest.lastIndexOf('@');
    if (atIndex >= 0) {
        const credentials = rest.slice(0, atIndex);
        rest = rest.slice(atIndex + 1);
        const separator = credentials.indexOf(':');
        if (separator >= 0) {
            user = credentials.slice(0, separator);
            password = credentials.slice(separator + 1);
        } else {
            user = credentials;
        }
    }
    const queryIndex = rest.indexOf('?');
    if (queryIndex >= 0) {
        params = rest.slice(queryIndex + 1);
        rest = rest.slice(0, queryIndex);
    }
    const slashIndex = rest.indexOf('/');
    if (slashIndex >= 0) {
        database = rest.slice(slashIndex + 1);
        let address = rest.slice(0, slashIndex);
        const openParen = address.indexOf('(');
        const closeParen = address.lastIndexOf(')');
        if (openParen >= 0 && closeParen > openParen) address = address.slice(openParen + 1, closeParen);
        if (address) {
            const portSeparator = address.lastIndexOf(':');
            if (portSeparator >= 0) {
                host = address.slice(0, portSeparator);
                port = address.slice(portSeparator + 1);
            } else {
                host = address;
            }
        }
    } else {
        database = rest;
    }
    return {
        user: user || mysqlDefaults.user,
        password,
        host: host || mysqlDefaults.host,
        port: port || mysqlDefaults.port,
        database: database || mysqlDefaults.database,
        params
    };
}

/**
 * Format editable fields as a MySQL DSN.
 * @param {object} parts - MySQL connection fields.
 * @returns {string} MySQL DSN.
 */
export function formatMysqlDsn(parts = {}) {
    const user = String(parts.user || '').trim();
    const password = String(parts.password || '');
    const host = String(parts.host || mysqlDefaults.host).trim() || mysqlDefaults.host;
    const port = String(parts.port || mysqlDefaults.port).trim();
    const database = String(parts.database || '').trim();
    const params = String(parts.params || '').trim().replace(/^\?/, '');
    const credentials = user ? `${user}${password ? `:${password}` : ''}@` : '';
    return `${credentials}tcp(${host}${port ? `:${port}` : ''})/${database}${params ? `?${params}` : ''}`;
}

/**
 * Parse a native ClickHouse URL into editable connection fields.
 * @param {string} dsnStr - ClickHouse connection URL.
 * @returns {object} Parsed fields.
 */
export function parseClickHouseDsn(dsnStr) {
    if (typeof dsnStr !== 'string' || !dsnStr.trim().startsWith('clickhouse://')) {
        return copyDefaults(clickHouseDefaults);
    }
    try {
        const parsed = new URL(dsnStr.trim());
        return {
            user: decodeURIComponent(parsed.username || '') || clickHouseDefaults.user,
            password: decodeURIComponent(parsed.password || ''),
            host: parsed.hostname || clickHouseDefaults.host,
            port: parsed.port || clickHouseDefaults.port,
            database: decodeURIComponent(parsed.pathname.replace(/^\//, '')) || clickHouseDefaults.database,
            params: parsed.search.slice(1)
        };
    } catch {
        return copyDefaults(clickHouseDefaults);
    }
}

/**
 * Format editable fields as a native ClickHouse URL.
 * @param {object} parts - ClickHouse connection fields.
 * @returns {string} ClickHouse connection URL.
 */
export function formatClickHouseDsn(parts = {}) {
    const user = encodeURIComponent(String(parts.user || clickHouseDefaults.user).trim());
    const password = String(parts.password || '');
    const credentials = password ? `${user}:${encodeURIComponent(password)}` : user;
    const host = formatURLHost(parts.host, clickHouseDefaults.host);
    const port = String(parts.port || clickHouseDefaults.port).trim();
    const database = encodeURIComponent(String(parts.database || clickHouseDefaults.database).trim());
    const params = String(parts.params || '').trim().replace(/^\?/, '');
    return `clickhouse://${credentials}@${host}${port ? `:${port}` : ''}/${database}${params ? `?${params}` : ''}`;
}

/**
 * Parse a PostgreSQL URL or keyword DSN into editable connection fields.
 * @param {string} dsnStr - PostgreSQL DSN.
 * @returns {object} Parsed fields.
 */
export function parsePostgresDsn(dsnStr) {
    if (typeof dsnStr !== 'string' || !dsnStr.trim()) return copyDefaults(postgresDefaults);
    const value = dsnStr.trim();
    if (value.startsWith('postgres://') || value.startsWith('postgresql://')) {
        try {
            const parsed = new URL(value);
            return {
                user: decodeURIComponent(parsed.username || '') || postgresDefaults.user,
                password: decodeURIComponent(parsed.password || ''),
                host: parsed.hostname || postgresDefaults.host,
                port: parsed.port || postgresDefaults.port,
                database: decodeURIComponent(parsed.pathname.replace(/^\//, '')) || postgresDefaults.database,
                params: parsed.search.slice(1) || postgresDefaults.params
            };
        } catch {
            return copyDefaults(postgresDefaults);
        }
    }
    if (!value.includes('=')) return copyDefaults(postgresDefaults);

    const result = {...postgresDefaults, params: ''};
    const extraParams = [];
    const fields = /([a-zA-Z_]+)=('(?:\\'|[^'])*'|\S+)/g;
    let match;
    while ((match = fields.exec(value)) !== null) {
        const key = match[1].toLowerCase();
        let fieldValue = match[2];
        if (fieldValue.startsWith("'") && fieldValue.endsWith("'")) {
            fieldValue = fieldValue.slice(1, -1).replace(/\\'/g, "'");
        }
        if (key === 'user') result.user = fieldValue;
        else if (key === 'password') result.password = fieldValue;
        else if (key === 'host') result.host = fieldValue;
        else if (key === 'port') result.port = fieldValue;
        else if (key === 'dbname' || key === 'database') result.database = fieldValue;
        else extraParams.push(`${encodeURIComponent(key)}=${encodeURIComponent(fieldValue)}`);
    }
    result.params = extraParams.join('&');
    return result;
}

/**
 * Format editable fields as a PostgreSQL URL.
 * @param {object} parts - PostgreSQL connection fields.
 * @returns {string} PostgreSQL connection URL.
 */
export function formatPostgresDsn(parts = {}) {
    const user = encodeURIComponent(String(parts.user || postgresDefaults.user).trim());
    const password = String(parts.password || '');
    const credentials = password ? `${user}:${encodeURIComponent(password)}` : user;
    const host = formatURLHost(parts.host, postgresDefaults.host);
    const port = String(parts.port || postgresDefaults.port).trim();
    const database = encodeURIComponent(String(parts.database || postgresDefaults.database).trim());
    const params = String(parts.params || '').trim().replace(/^\?/, '');
    return `postgres://${credentials}@${host}${port ? `:${port}` : ''}/${database}${params ? `?${params}` : ''}`;
}
