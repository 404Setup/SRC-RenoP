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

const frontendRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const userFacingModules = [
    'js/auth.js',
    'js/browser.js',
    'js/browser/cargo.js',
    'js/browser/maven.js',
    'js/browser/npm.js',
    'js/cargo-messages.js',
    'js/dashboard.js',
    'js/maven-messages.js',
    'js/npm-messages.js',
    'js/messages.js',
    'js/profile.js',
    'js/publication-quota.js',
    'js/repositories.js',
    'js/reviews.js',
    'js/sessions.js',
    'js/settings.js',
    'js/user-profiles.js',
    'js/users.js',
    'js/users/modal.js',
];

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
