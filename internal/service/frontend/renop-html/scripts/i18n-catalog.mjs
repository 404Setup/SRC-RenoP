/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the terms of the Mozilla Public License, v. 2.0.
 */

import {existsSync, readFileSync, readdirSync, writeFileSync} from 'node:fs';
import {join} from 'node:path';
import {pathToFileURL} from 'node:url';

const localePattern = /^[A-Za-z]{2,3}(?:-[A-Za-z0-9]{2,8})+$/;
const fragmentPattern = /^[a-z][a-z0-9-]*\.js$/;
const placeholderPattern = /\{([A-Za-z0-9_]+)\}/g;

/**
 * List sorted translation fragment names for one locale.
 * @param {string} i18nDir - Locale catalog root.
 * @param {string} locale - Locale directory name.
 * @returns {string[]} Fragment file names.
 */
function listLocaleFragments(i18nDir, locale) {
    const localeDir = join(i18nDir, locale);
    const fragments = [];
    for (const entry of readdirSync(localeDir, {withFileTypes: true})) {
        if (!entry.isFile() || !fragmentPattern.test(entry.name)) {
            throw new Error(`invalid i18n fragment in ${locale}: ${entry.name}`);
        }
        fragments.push(entry.name);
    }
    fragments.sort();
    return fragments;
}

/**
 * Load and validate one locale fragment.
 * @param {string} i18nDir - Locale catalog root.
 * @param {string} locale - Locale directory name.
 * @param {string} fragment - Fragment file name.
 * @returns {Promise<{locale: string, fragment: string, translations: object, keys: Set<string>}>} Fragment descriptor.
 */
async function loadLocaleFragment(i18nDir, locale, fragment) {
    const fragmentPath = join(i18nDir, locale, fragment);
    const module = await import(pathToFileURL(fragmentPath).href);
    const translations = module.default;
    if (!translations || typeof translations !== 'object' || Array.isArray(translations)) {
        throw new Error(`i18n fragment must default-export an object: ${locale}/${fragment}`);
    }
    const keys = new Set();
    for (const [key, value] of Object.entries(translations)) {
        if (!key || typeof value !== 'string') {
            throw new Error(`invalid i18n entry in ${locale}/${fragment}: ${key || '<empty key>'}`);
        }
        keys.add(key);
    }
    return {locale, fragment, translations, keys};
}

/**
 * Collect interpolation placeholder names without allocating match arrays.
 * @param {string} value - Translation value.
 * @returns {Set<string>} Placeholder names.
 */
function placeholders(value) {
    const result = new Set();
    placeholderPattern.lastIndex = 0;
    let match;
    while ((match = placeholderPattern.exec(value)) !== null) result.add(match[1]);
    return result;
}

/**
 * Compare two small sets.
 * @param {Set<string>} left - First set.
 * @param {Set<string>} right - Second set.
 * @returns {boolean} Whether both sets contain the same values.
 */
function equalSets(left, right) {
    if (left.size !== right.size) return false;
    for (const value of left) if (!right.has(value)) return false;
    return true;
}

/**
 * Add duplicate-key diagnostics while building a locale-wide key index.
 * @param {string} locale - Locale name.
 * @param {Map<string, {keys: Set<string>}>} fragments - Fragment descriptors.
 * @param {string[]} issues - Diagnostic accumulator.
 * @returns {Set<string>} Combined keys.
 */
function indexLocale(locale, fragments, issues) {
    const combined = new Set();
    for (const [fragment, descriptor] of fragments) {
        for (const key of descriptor.keys) {
            if (combined.has(key)) issues.push(`duplicate key in ${locale}: ${key} (${fragment})`);
            else combined.add(key);
        }
    }
    return combined;
}

/**
 * Scan all locales in parallel against the English fragment/key baseline.
 * @param {object} options - Scan configuration.
 * @param {string} options.i18nDir - Locale catalog root.
 * @param {string} [options.referenceLocale='en-US'] - Canonical locale.
 * @param {string} [options.catalogName='catalog.generated.js'] - Generated catalog file name.
 * @returns {Promise<{locales: string[], referenceFragments: string[], fragmentsByLocale: Map<string, Map<string, object>>, keyCount: number, durationMs: number}>} Validated scan.
 */
export async function scanI18nCatalog({
    i18nDir,
    referenceLocale = 'en-US',
    catalogName = 'catalog.generated.js'
}) {
    const startedAt = performance.now();
    if (!existsSync(i18nDir)) throw new Error(`i18n directory not found: ${i18nDir}`);

    const locales = [];
    for (const entry of readdirSync(i18nDir, {withFileTypes: true})) {
        if (entry.isFile() && entry.name === catalogName) continue;
        if (!entry.isDirectory() || !localePattern.test(entry.name)) {
            throw new Error(`invalid entry in i18n directory: ${entry.name}`);
        }
        locales.push(entry.name);
    }
    locales.sort();
    if (!locales.includes(referenceLocale)) throw new Error(`reference locale is missing: ${referenceLocale}`);

    const fragmentNamesByLocale = new Map();
    const loadTasks = [];
    for (const locale of locales) {
        const fragments = listLocaleFragments(i18nDir, locale);
        fragmentNamesByLocale.set(locale, fragments);
        for (const fragment of fragments) loadTasks.push(loadLocaleFragment(i18nDir, locale, fragment));
    }
    const loaded = await Promise.all(loadTasks);
    const fragmentsByLocale = new Map(locales.map((locale) => [locale, new Map()]));
    for (const descriptor of loaded) fragmentsByLocale.get(descriptor.locale).set(descriptor.fragment, descriptor);

    const referenceFragments = fragmentNamesByLocale.get(referenceLocale);
    if (!referenceFragments.includes('common.js')) {
        throw new Error(`${referenceLocale} must provide common.js`);
    }
    const issues = [];
    const referenceDescriptors = fragmentsByLocale.get(referenceLocale);
    const referenceKeys = indexLocale(referenceLocale, referenceDescriptors, issues);
    if (issues.length > 0) throw new Error(`invalid reference locale:\n- ${issues.join('\n- ')}`);

    const referenceFragmentSet = new Set(referenceFragments);
    for (const locale of locales) {
        if (locale === referenceLocale) continue;
        const fragments = fragmentNamesByLocale.get(locale);
        const fragmentSet = new Set(fragments);
        const missingFragments = referenceFragments.filter((fragment) => !fragmentSet.has(fragment));
        const extraFragments = fragments.filter((fragment) => !referenceFragmentSet.has(fragment));
        if (missingFragments.length > 0) issues.push(`missing fragments in ${locale}: ${missingFragments.join(', ')}`);
        if (extraFragments.length > 0) issues.push(`extra fragments in ${locale}: ${extraFragments.join(', ')}`);

        const descriptors = fragmentsByLocale.get(locale);
        indexLocale(locale, descriptors, issues);
        for (const fragment of referenceFragments) {
            const expected = referenceDescriptors.get(fragment);
            const actual = descriptors.get(fragment);
            if (!actual) continue;
            const missingKeys = [];
            const extraKeys = [];
            const placeholderMismatches = [];
            for (const key of expected.keys) {
                if (!actual.keys.has(key)) {
                    missingKeys.push(key);
                    continue;
                }
                const expectedPlaceholders = placeholders(expected.translations[key]);
                const actualPlaceholders = placeholders(actual.translations[key]);
                if (!equalSets(expectedPlaceholders, actualPlaceholders)) {
                    placeholderMismatches.push(
                        `${key} ({${[...expectedPlaceholders].sort().join(',')}} != {${[...actualPlaceholders].sort().join(',')}})`,
                    );
                }
            }
            for (const key of actual.keys) if (!expected.keys.has(key)) extraKeys.push(key);
            if (missingKeys.length > 0) issues.push(`missing keys in ${locale}/${fragment}: ${missingKeys.sort().join(', ')}`);
            if (extraKeys.length > 0) issues.push(`extra keys in ${locale}/${fragment}: ${extraKeys.sort().join(', ')}`);
            if (placeholderMismatches.length > 0) {
                issues.push(`placeholder mismatches in ${locale}/${fragment}: ${placeholderMismatches.sort().join(', ')}`);
            }
        }
    }

    if (issues.length > 0) {
        const error = new Error(`i18n validation failed with ${issues.length} issue(s):\n- ${issues.join('\n- ')}`);
        error.code = 'I18N_VALIDATION_FAILED';
        error.issues = issues;
        throw error;
    }
    return {
        locales,
        referenceFragments,
        fragmentsByLocale,
        keyCount: referenceKeys.size,
        durationMs: performance.now() - startedAt
    };
}

/**
 * Create a stable JavaScript identifier for a locale fragment import.
 * @param {string} locale - Locale directory name.
 * @param {string} fragment - Fragment file name.
 * @returns {string} Import identifier.
 */
function localeImportIdentifier(locale, fragment) {
    return `${locale}_${fragment.slice(0, -3)}`.replaceAll('-', '_');
}

/**
 * Render the validated static locale catalog module.
 * @param {string[]} locales - Sorted locale names.
 * @param {string[]} fragments - Canonical fragment names.
 * @returns {string} Generated module source.
 */
function renderCatalog(locales, fragments) {
    const imports = [];
    const catalogEntries = [];
    for (const locale of locales) {
        const identifiers = [];
        for (const fragment of fragments) {
            const identifier = localeImportIdentifier(locale, fragment);
            identifiers.push(identifier);
            imports.push(`import ${identifier} from './${locale}/${fragment}';`);
        }
        catalogEntries.push(`    '${locale}': Object.freeze(Object.assign({}, ${identifiers.join(', ')})),`);
    }
    return [
        '/*',
        ' * Copyright (c) 2026 404Setup. All rights reserved.',
        ' *',
        ' * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.',
        ' *',
        ' * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.',
        ' *',
        ' * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.',
        ' */',
        '',
        '/* This file is generated by build.mjs. Do not edit it directly. */',
        '',
        ...imports,
        '',
        'const localeCatalog = Object.freeze({',
        ...catalogEntries,
        '});',
        '',
        'export default localeCatalog;',
        ''
    ].join('\n');
}

/**
 * Validate locales and update the generated browser catalog when content changed.
 * @param {object} options - Generator configuration.
 * @param {string} options.i18nDir - Locale catalog root.
 * @param {string} options.catalogFile - Generated catalog path.
 * @param {string} [options.referenceLocale='en-US'] - Canonical locale.
 * @param {string} [options.catalogName='catalog.generated.js'] - Generated file name.
 * @returns {Promise<void>}
 */
export async function generateI18nCatalog({
    i18nDir,
    catalogFile,
    referenceLocale = 'en-US',
    catalogName = 'catalog.generated.js'
}) {
    const scan = await scanI18nCatalog({i18nDir, referenceLocale, catalogName});
    const generated = renderCatalog(scan.locales, scan.referenceFragments);
    if (!existsSync(catalogFile) || readFileSync(catalogFile, 'utf8') !== generated) {
        writeFileSync(catalogFile, generated, 'utf8');
    }
    console.log(
        `i18n validation OK: ${scan.locales.length} locales, ${scan.referenceFragments.length} fragments, ` +
        `${scan.keyCount} English keys (${scan.durationMs.toFixed(1)} ms)`,
    );
    console.log('Generated i18n catalog:', catalogFile.replaceAll('\\', '/'));
}
