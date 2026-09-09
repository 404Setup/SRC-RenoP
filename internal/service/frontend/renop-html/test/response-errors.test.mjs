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
import {readdirSync, readFileSync} from 'node:fs';
import {dirname, join, resolve} from 'node:path';
import test from 'node:test';
import vm from 'node:vm';
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

test('session restoration preserves credentials on forbidden responses and expires only unauthorized sessions', async () => {
    const storage = new Map([['username', 'alice']]), logouts = [], updates = [];
    let status = 403;
    const context = vm.createContext({
        localStorage: {
            getItem: key => storage.get(key),
            setItem: (key, value) => storage.set(key, value),
            removeItem: key => storage.delete(key)
        },
        fetchProto: async () => ({response: {ok: false, status}, data: null}), SessionDetails: {},
        logout: reason => logouts.push(reason), updateAuthUI: (...args) => updates.push(args),
    });
    const auth = readFileSync(join(frontendRoot, 'js/auth.js'), 'utf8');
    const start = auth.indexOf('export async function initializeSession()');
    const end = auth.indexOf('\n/**', start);
    vm.runInContext(auth.slice(start, end).replace('export ', ''), context);
    await context.initializeSession();
    assert.deepEqual(logouts, []);
    assert.equal(storage.get('username'), 'alice');
    assert.deepEqual(updates.at(-1), [true, 'alice', false]);
    status = 401;
    await context.initializeSession();
    assert.deepEqual(logouts, ['expired']);
});
