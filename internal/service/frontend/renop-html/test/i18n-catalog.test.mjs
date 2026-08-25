/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the terms of the Mozilla Public License, v. 2.0.
 */

import assert from 'node:assert/strict';
import {mkdtempSync, mkdirSync, rmSync, writeFileSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import test from 'node:test';

import {scanI18nCatalog} from '../scripts/i18n-catalog.mjs';

/**
 * Write a minimal locale fragment fixture.
 * @param {string} root - Fixture root.
 * @param {string} locale - Locale directory.
 * @param {string} fragment - Fragment file name.
 * @param {object} values - Translation values.
 * @returns {void}
 */
function writeFragment(root, locale, fragment, values) {
    const directory = join(root, locale);
    mkdirSync(directory, {recursive: true});
    writeFileSync(join(directory, fragment), `export default ${JSON.stringify(values)};\n`, 'utf8');
}

test('parallel i18n scan accepts complete English-key parity', async () => {
    const root = mkdtempSync(join(tmpdir(), 'renop-i18n-valid-'));
    try {
        writeFragment(root, 'en-US', 'common.js', {alpha: 'Hello {name}', beta: 'Ready'});
        writeFragment(root, 'fr-FR', 'common.js', {alpha: 'Bonjour {name}', beta: 'Prêt'});

        const result = await scanI18nCatalog({i18nDir: root});
        assert.equal(result.keyCount, 2);
        assert.deepEqual(result.referenceFragments, ['common.js']);
        assert.ok(result.durationMs >= 0);
    } finally {
        rmSync(root, {recursive: true, force: true});
    }
});

test('i18n scan reports all missing keys, fragments, extras, and placeholder drift', async () => {
    const root = mkdtempSync(join(tmpdir(), 'renop-i18n-invalid-'));
    try {
        writeFragment(root, 'en-US', 'browser.js', {'browser.open': 'Open'});
        writeFragment(root, 'en-US', 'common.js', {alpha: 'Hello {name}', beta: 'Ready'});
        writeFragment(root, 'fr-FR', 'common.js', {alpha: 'Bonjour {nom}', gamma: 'Supplément'});

        await assert.rejects(
            scanI18nCatalog({i18nDir: root}),
            (error) => {
                assert.equal(error.code, 'I18N_VALIDATION_FAILED');
                assert.match(error.message, /missing fragments in fr-FR: browser\.js/);
                assert.match(error.message, /missing keys in fr-FR\/common\.js: beta/);
                assert.match(error.message, /extra keys in fr-FR\/common\.js: gamma/);
                assert.match(error.message, /placeholder mismatches in fr-FR\/common\.js: alpha/);
                return true;
            }
        );
    } finally {
        rmSync(root, {recursive: true, force: true});
    }
});
