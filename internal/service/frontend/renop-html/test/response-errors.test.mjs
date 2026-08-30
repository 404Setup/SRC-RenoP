/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

import assert from 'node:assert/strict';
import {readFileSync, readdirSync} from 'node:fs';
import {dirname, join, resolve} from 'node:path';
import test from 'node:test';
import {fileURLToPath} from 'node:url';

const frontendRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');

/**
 * Discover every handwritten frontend JavaScript module.
 * @param {string} directory - Absolute directory to scan.
 * @param {string} relative - Repository-relative label.
 * @returns {string[]} Module labels.
 */
function discoverUserFacingModules(directory, relative = 'js') {
    const modules = [];
    for (const entry of readdirSync(directory, {withFileTypes: true})) {
        if (entry.isDirectory()) {
            if (entry.name === 'i18n' || entry.name === 'proto') continue;
            modules.push(...discoverUserFacingModules(join(directory, entry.name), `${relative}/${entry.name}`));
        } else if (entry.isFile() && entry.name.endsWith('.js') && !entry.name.endsWith('.generated.js')) {
            modules.push(`${relative}/${entry.name}`);
        }
    }
    return modules.sort();
}

const userFacingModules = discoverUserFacingModules(join(frontendRoot, 'js'));

test('user-facing request failures never expose raw response or runtime error text', () => {
    for (const relativePath of userFacingModules) {
        const source = readFileSync(join(frontendRoot, relativePath), 'utf8');
        assert.doesNotMatch(source, /\b(?:response|res|beginRes|finishRes|deleteResponse|verifyResponse)\.text\(\)/,
            `${relativePath} reads a raw failed response`);
        assert.doesNotMatch(source, /showAlert\(\s*(?:error|err|e)(?:\?\.|\.)message/,
            `${relativePath} displays an untrusted runtime error`);
        assert.doesNotMatch(source, /textContent\s*=\s*(?:errText|translatedErr|translatedMsg|responseMessage)/,
            `${relativePath} assigns untrusted error text to the DOM`);
    }
});

test('shared response errors use bounded registered localization', () => {
    const source = readFileSync(join(frontendRoot, 'js/response-errors.js'), 'utf8');
    for (const required of [
        'MAX_ERROR_BODY_BYTES',
        'X-Renop-Error-Code',
        'translateKnownError',
        'reader.read()',
        'statusErrorKeys',
        'caughtErrorMessage',
    ]) {
        assert.ok(source.includes(required), `response error mapping is missing ${required}`);
    }
    assert.doesNotMatch(source, /response\.text\(\)/,
        'response error mapping can allocate an unbounded response body');
});

test('authorization denials do not invalidate a browser session by default', () => {
    const source = readFileSync(join(frontendRoot, 'js/api.js'), 'utf8');
    assert.match(source, /logoutOnForbidden = false/);
    assert.match(source, /response\.status === 401/);
    assert.match(source, /response\.status === 403 && logoutOnForbidden/);
});
