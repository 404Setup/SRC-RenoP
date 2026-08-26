/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the terms of the Mozilla Public License, v. 2.0.
 */

import assert from 'node:assert/strict';
import {readFileSync, readdirSync} from 'node:fs';
import {dirname, join} from 'node:path';
import test from 'node:test';
import {fileURLToPath} from 'node:url';

globalThis.HTMLElement = class {};
globalThis.customElements = {get: () => undefined, define: () => {}};

const testRoot = dirname(fileURLToPath(import.meta.url));
const frontendRoot = join(testRoot, '..');
const {ICONS} = await import('../js/components/icon.js');
const {getRepositoryFormat, listRepositoryFormats} = await import('../js/repository-formats.js');

/**
 * Recursively collect handwritten JavaScript modules below one directory.
 * @param {string} root - Directory to traverse.
 * @returns {string[]} JavaScript source paths.
 */
function javascriptSources(root) {
    const paths = [];
    for (const entry of readdirSync(root, {withFileTypes: true})) {
        if (entry.name === 'proto' || entry.name === 'i18n') continue;
        const path = join(root, entry.name);
        if (entry.isDirectory()) paths.push(...javascriptSources(path));
        else if (entry.isFile() && entry.name.endsWith('.js')) paths.push(path);
    }
    return paths;
}

/**
 * Return every literal icon name referenced by one source string.
 * @param {string} source - JavaScript or HTML source.
 * @returns {Set<string>} Literal icon names.
 */
function literalIconNames(source) {
    const names = new Set();
    for (const pattern of [
        /createIcon\(\s*['"]([A-Za-z0-9]+)['"]/g,
        /\bicon:\s*['"]([A-Za-z0-9]+)['"]/g,
        /<renop-icon\b[^>]*\bname="([A-Za-z0-9]+)"/g,
    ]) {
        for (const match of source.matchAll(pattern)) names.add(match[1]);
    }
    return names;
}

test('icon aliases substantially reduce unique SVG markup', () => {
    assert.ok(Object.keys(ICONS).length >= 190, 'legacy icon call sites must remain compatible');
    assert.ok(new Set(Object.values(ICONS)).size <= 60, 'file icons should collapse into bounded visual families');
});

test('every repository format has one distinct canonical icon', () => {
    const formats = listRepositoryFormats();
    const icons = formats.map(format => format.icon);
    assert.equal(new Set(icons).size, formats.length);
    for (const format of formats) assert.ok(ICONS[format.icon], `missing ${format.id} repository icon`);
    assert.equal(getRepositoryFormat('maven-classic').icon, getRepositoryFormat('maven').icon);
});

test('every literal frontend icon reference resolves', () => {
    const sources = javascriptSources(join(frontendRoot, 'js'));
    sources.push(join(frontendRoot, 'index.html'));
    for (const path of sources) {
        const source = readFileSync(path, 'utf8');
        for (const name of literalIconNames(source)) {
            assert.ok(ICONS[name], `${path} references missing icon ${name}`);
        }
    }
});

test('repository views consume the shared format descriptors', () => {
    const expected = new Map([
        ['js/repositories.js', "createIcon(format.icon || 'repositoryFiles')"],
        ['js/components/file-item.js', "iconName = format.icon || 'repositoryFiles'"],
        ['js/profile.js', 'createIcon(getRepositoryFormat(format).icon'],
        ['js/browser/maven.js', "getRepositoryFormat('maven').icon"],
        ['js/browser/cargo.js', "getRepositoryFormat('cargo').icon"],
        ['js/browser/docker.js', "getRepositoryFormat('docker').icon"],
        ['js/browser/search.js', 'activeRepositoryIcon = getRepositoryFormat(activeFormat).icon'],
    ]);
    for (const [relativePath, required] of expected) {
        const source = readFileSync(join(frontendRoot, ...relativePath.split('/')), 'utf8');
        assert.ok(source.includes(required), `${relativePath} does not use the canonical repository icon`);
    }
});
