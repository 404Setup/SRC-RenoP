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
import {mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {dirname, join, resolve} from 'node:path';
import {fileURLToPath, pathToFileURL} from 'node:url';
import test from 'node:test';

import {generateI18nCatalog, scanI18nCatalog} from '../scripts/i18n-catalog.mjs';

const frontendRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');

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
        const catalogRoot = join(root, 'catalog');
        writeFragment(catalogRoot, 'en-US', 'common.js', {alpha: 'Hello {name}', beta: 'Ready'});
        writeFragment(catalogRoot, 'fr-FR', 'common.js', {alpha: 'Bonjour {name}', beta: 'Prêt'});
        const sourceRoot = join(root, 'src');
        mkdirSync(sourceRoot);
        writeFileSync(join(sourceRoot, 'app.js'), "const label = t('alpha');\n", 'utf8');

        const result = await scanI18nCatalog({i18nDir: catalogRoot, sourceRoots: [sourceRoot]});
        assert.equal(result.keyCount, 2);
        assert.equal(result.referenceCount, 1);
        assert.deepEqual(result.referenceFragments, ['common.js']);
        assert.ok(result.durationMs >= 0);
    } finally {
        rmSync(root, {recursive: true, force: true});
    }
});

test('i18n scan reports missing English keys referenced by JavaScript and HTML', async () => {
    const root = mkdtempSync(join(tmpdir(), 'renop-i18n-references-'));
    try {
        const catalogRoot = join(root, 'catalog');
        const sourceRoot = join(root, 'src');
        mkdirSync(sourceRoot, {recursive: true});
        writeFragment(catalogRoot, 'en-US', 'common.js', {alpha: 'Ready'});
        writeFragment(catalogRoot, 'fr-FR', 'common.js', {alpha: 'Prêt'});
        writeFileSync(join(sourceRoot, 'app.js'), "t('missing.javascript');\n", 'utf8');
        writeFileSync(join(sourceRoot, 'index.html'), '<span data-i18n="missing.html"></span>\n', 'utf8');

        await assert.rejects(
            scanI18nCatalog({i18nDir: catalogRoot, sourceRoots: [sourceRoot]}),
            (error) => {
                assert.equal(error.code, 'I18N_VALIDATION_FAILED');
                assert.match(error.message, /missing English key referenced by source: missing\.javascript \(app\.js:1\)/);
                assert.match(error.message, /missing English key referenced by source: missing\.html \(index\.html:1\)/);
                return true;
            }
        );
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

test('generated catalog keeps English eager and loads other locales on demand', async () => {
    const root = mkdtempSync(join(tmpdir(), 'renop-i18n-lazy-'));
    try {
        writeFragment(root, 'en-US', 'common.js', {alpha: 'Ready'});
        writeFragment(root, 'fr-FR', 'common.js', {alpha: 'Prêt'});
        const catalogFile = join(root, 'catalog.generated.js');
        await generateI18nCatalog({i18nDir: root, catalogFile});

        const catalog = await import(`${pathToFileURL(catalogFile).href}?test=${Date.now()}`);
        assert.deepEqual(catalog.availableLocales, ['en-US', 'fr-FR']);
        assert.equal(catalog.default.alpha, 'Ready');
        const first = await catalog.loadLocale('fr-FR');
        const second = await catalog.loadLocale('fr-FR');
        assert.equal(first.alpha, 'Prêt');
        assert.equal(first, second);
        await assert.rejects(catalog.loadLocale('invalid'), /unsupported locale: invalid/);
    } finally {
        rmSync(root, {recursive: true, force: true});
    }
});

test('initial translations and language changes expose deterministic loading state', () => {
    const runtime = readFileSync(join(frontendRoot, 'js/i18n.js'), 'utf8');
    const page = readFileSync(join(frontendRoot, 'index.html'), 'utf8');
    const baseStyles = readFileSync(join(frontendRoot, 'css/layout/base.css'), 'utf8');
    const settingsStyles = readFileSync(join(frontendRoot, 'css/manager/settings.css'), 'utf8');

    assert.match(page, /id="language-load-progress"[^>]*role="progressbar"/);
    assert.match(runtime, /setLanguageLoading\(true\)/);
    assert.match(runtime, /finally \{[\s\S]*?setLanguageLoading\(false\)/);
    assert.match(runtime, /document\.documentElement\.dataset\.i18nReady = 'true'/);
    assert.match(runtime, /if \(!document\.body\)[\s\S]*?else \{\s*setupLanguageModal\(\)/);
    assert.match(baseStyles, /html:not\(\[data-i18n-ready="true"\]\) \[data-i18n\][^}]*visibility: hidden/s);
    assert.match(settingsStyles, /\.language-load-progress > span[^}]*animation: languageLoadProgress/s);
    for (const key of ['users.thUser', 'users.thPermissions', 'users.thCreatedAt']) {
        assert.match(page, new RegExp(`data-i18n="${key}"`));
    }
});
