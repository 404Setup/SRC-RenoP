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
import {readdirSync, readFileSync, statSync} from 'node:fs';
import {dirname, join, relative, resolve, sep} from 'node:path';
import test from 'node:test';
import {fileURLToPath} from 'node:url';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..', 'content', 'docs');
const locales = ['en-US', 'fr-FR', 'ja-JP', 'ru-RU', 'zh-CN'];

/**
 * Return every Markdown path below dir using forward slashes.
 * @param {string} dir
 * @param {string[]} [files]
 * @returns {string[]}
 */
function markdownFiles(dir, files = []) {
    for (const name of readdirSync(dir)) {
        const path = join(dir, name);
        if (statSync(path).isDirectory()) markdownFiles(path, files);
        else if (name.endsWith('.md')) files.push(relative(dirRoot(dir), path).split(sep).join('/'));
    }
    return files;
}

/**
 * Resolve the locale directory that owns a recursively visited path.
 * @param {string} dir
 * @returns {string}
 */
function dirRoot(dir) {
    let current = dir;
    while (!locales.includes(current.split(sep).at(-1))) current = dirname(current);
    return current;
}

/**
 * Remove YAML front matter from one Markdown source.
 * @param {string} source
 * @returns {string}
 */
function markdownBody(source) {
    return source.replace(/^---\r?\n[\s\S]*?\r?\n---\r?\n/, '').replace(/\r\n/g, '\n');
}

/**
 * Parse the flat front-matter fields used by website documentation.
 * @param {string} source
 * @returns {Record<string, string>}
 */
function frontMatter(source) {
    const match = source.match(/^---\r?\n([\s\S]*?)\r?\n---/);
    if (!match) return {};
    return Object.fromEntries(match[1].split(/\r?\n/).map(line => {
        const separator = line.indexOf(':');
        return separator < 0 ? [line.trim(), ''] : [line.slice(0, separator).trim(), line.slice(separator + 1).trim()];
    }));
}

/**
 * Extract ordered heading levels.
 * @param {string} source
 * @returns {number[]}
 */
function headingLevels(source) {
    const prose = markdownBody(source).replace(/^```[^\n]*\n[\s\S]*?^```\s*$/gm, '');
    return [...prose.matchAll(/^(#{1,6})\s+/gm)].map(match => match[1].length);
}

/**
 * Extract fenced code blocks including their language marker.
 * @param {string} source
 * @returns {string[]}
 */
function codeBlocks(source) {
    return [...markdownBody(source).matchAll(/^```([^\n]*)\n([\s\S]*?)^```\s*$/gm)]
        .map(match => `${match[1].trim()}\n${match[2].trimEnd()}`);
}

/**
 * Extract stable HTTP method/path contracts from prose and tables.
 * @param {string} source
 * @returns {string[]}
 */
function endpoints(source) {
    const found = [...markdownBody(source).matchAll(/\b(GET|POST|PUT|PATCH|DELETE|HEAD)\s+(\/[^`\s,)]+)/g)]
        .map(match => `${match[1]} ${match[2].replace(/[.;:]$/, '')}`);
    return [...new Set(found)].sort();
}

/**
 * Extract local Markdown link destinations.
 * @param {string} source
 * @returns {string[]}
 */
function localLinks(source) {
    return [...markdownBody(source).matchAll(/\[[^\]]+\]\((?!https?:|#)([^)]+)\)/g)]
        .map(match => match[1]).sort();
}

/**
 * Count non-empty documentation lines outside front matter.
 * @param {string} source
 * @returns {number}
 */
function meaningfulLines(source) {
    return markdownBody(source).split('\n').filter(line => line.trim() !== '').length;
}

test('documentation locales preserve canonical structure and contracts', () => {
    const canonicalFiles = markdownFiles(join(root, 'en-US')).sort();
    for (const locale of locales) {
        const localeRoot = join(root, locale);
        assert.deepEqual(markdownFiles(localeRoot).sort(), canonicalFiles, `${locale} file set`);
        if (locale === 'en-US') continue;
        for (const file of canonicalFiles) {
            const canonical = readFileSync(join(root, 'en-US', file), 'utf8');
            const translated = readFileSync(join(localeRoot, file), 'utf8');
            const canonicalMeta = frontMatter(canonical);
            const translatedMeta = frontMatter(translated);
            assert.equal(translatedMeta.order, canonicalMeta.order, `${locale}:${file} order`);
            for (const field of ['title', 'category', 'description']) {
                assert.ok(translatedMeta[field], `${locale}:${file} missing ${field}`);
            }
            assert.deepEqual(headingLevels(translated), headingLevels(canonical), `${locale}:${file} heading outline`);
            assert.deepEqual(codeBlocks(translated), codeBlocks(canonical), `${locale}:${file} code blocks`);
            assert.deepEqual(endpoints(translated), endpoints(canonical), `${locale}:${file} endpoints`);
            assert.deepEqual(localLinks(translated), localLinks(canonical), `${locale}:${file} local links`);
            const minimumLines = Math.max(1, Math.floor(meaningfulLines(canonical) * 0.72));
            assert.ok(meaningfulLines(translated) >= minimumLines,
                `${locale}:${file} is abbreviated (${meaningfulLines(translated)} < ${minimumLines} lines)`);
        }
    }
});

test('documentation headings use descriptive labels without numeric decoration', () => {
    const canonicalFiles = markdownFiles(join(root, 'en-US')).sort();
    for (const locale of locales) {
        for (const file of canonicalFiles) {
            const source = markdownBody(readFileSync(join(root, locale, file), 'utf8'))
                .replace(/^```[^\n]*\n[\s\S]*?^```\s*$/gm, '');
            assert.doesNotMatch(source, /^#{2,6}\s+\d+[.)]\s+/m, `${locale}:${file} numbered heading`);
        }
    }
});
