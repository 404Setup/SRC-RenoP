/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {
    copyFileSync,
    existsSync,
    mkdirSync,
    readdirSync,
    readFileSync,
    rmSync,
    statSync,
    writeFileSync,
} from 'node:fs';
import {dirname, join, relative, sep} from 'node:path';
import {fileURLToPath, pathToFileURL} from 'node:url';
import {rolldown} from 'rolldown';
import {bundle} from 'lightningcss';

const root = dirname(fileURLToPath(import.meta.url));
const outDir = join(root, 'dist');
const contentDocs = join(root, 'content', 'docs');

const KNOWN_LOCALES = new Set([
    'en-US', 'zh-CN', 'zh-HK', 'zh-TW', 'zh-YUE',
    'ko-KR', 'ja-JP', 'de-DE', 'fr-FR', 'ru-RU', 'es-ES', 'pt-PT',
]);

function walk(dir, acc = []) {
    if (!existsSync(dir)) return acc;
    for (const name of readdirSync(dir)) {
        const p = join(dir, name);
        if (statSync(p).isDirectory()) walk(p, acc);
        else acc.push(p);
    }
    return acc;
}

function ensureDir(dir) {
    mkdirSync(dir, {recursive: true});
}

function copyDir(src, dest) {
    if (!existsSync(src)) return;
    ensureDir(dest);
    for (const name of readdirSync(src)) {
        const s = join(src, name);
        const d = join(dest, name);
        if (statSync(s).isDirectory()) copyDir(s, d);
        else {
            ensureDir(dirname(d));
            copyFileSync(s, d);
        }
    }
}

function parseFrontMatter(raw) {
    if (!raw.startsWith('---')) {
        return {meta: {}, body: raw};
    }
    const end = raw.indexOf('\n---', 3);
    if (end === -1) return {meta: {}, body: raw};
    const fm = raw.slice(3, end).trim();
    const body = raw.slice(end + 4).replace(/^\r?\n/, '');
    const meta = {};
    for (const line of fm.split(/\r?\n/)) {
        const m = line.match(/^([A-Za-z0-9_-]+)\s*:\s*(.*)$/);
        if (!m) continue;
        let val = m[2].trim();
        if ((val.startsWith('"') && val.endsWith('"')) || (val.startsWith("'") && val.endsWith("'"))) {
            val = val.slice(1, -1);
        }
        if (/^\d+$/.test(val)) val = Number(val);
        meta[m[1]] = val;
    }
    return {meta, body};
}

function titleFromMarkdown(body, fallback) {
    const m = body.match(/^#\s+(.+)$/m);
    return m ? m[1].trim() : fallback;
}

function humanize(name) {
    return name
        .replace(/[-_]/g, ' ')
        .replace(/\b\w/g, (c) => c.toUpperCase());
}

const categoryOrder = {
    'getting-started': 10,
    configuration: 20,
    api: 30,
    general: 90,
};

function buildLocaleBundle(docs) {
    docs.sort((a, b) => {
        const ca = categoryOrder[a.categorySlug] ?? 50;
        const cb = categoryOrder[b.categorySlug] ?? 50;
        if (ca !== cb) return ca - cb;
        if (a.categorySlug !== b.categorySlug) {
            return a.categorySlug.localeCompare(b.categorySlug);
        }
        if (a.order !== b.order) return a.order - b.order;
        return a.title.localeCompare(b.title);
    });

    const categories = [];
    const seen = new Map();
    for (const d of docs) {
        if (!seen.has(d.categorySlug)) {
            seen.set(d.categorySlug, {
                slug: d.categorySlug,
                name: d.category,
                docs: [],
            });
            categories.push(seen.get(d.categorySlug));
        }
        seen.get(d.categorySlug).docs.push(d);
    }
    return {docs, categories};
}

/**
 * content/docs/{locale}/{category}/file.md
 * slug is locale-relative: {category}/file
 */
function generateDocsIndex() {
    const files = walk(contentDocs).filter((f) => f.toLowerCase().endsWith('.md'));
    const byLocale = new Map();

    for (const file of files) {
        const rel = relative(contentDocs, file).replace(/\\/g, '/');
        const parts = rel.split('/');
        if (parts.length < 2) {
            console.warn('skip doc not under locale folder:', rel);
            continue;
        }

        const locale = parts[0];
        if (!KNOWN_LOCALES.has(locale)) {
            console.warn('skip unknown docs locale folder:', locale, rel);
            continue;
        }

        const rest = parts.slice(1);
        const raw = readFileSync(file, 'utf8');
        const {meta, body} = parseFrontMatter(raw);
        const fileName = rest[rest.length - 1].replace(/\.md$/i, '');
        const categorySlug = rest.length > 1 ? rest[0] : 'general';
        const category =
            meta.category ||
            (rest.length > 1 ? humanize(rest[0]) : 'General');
        const slug = rest.join('/').replace(/\.md$/i, '');
        const title = meta.title || titleFromMarkdown(body, humanize(fileName));
        const order = typeof meta.order === 'number' ? meta.order : 100;
        const description = meta.description || '';

        if (!byLocale.has(locale)) byLocale.set(locale, []);
        byLocale.get(locale).push({
            slug,
            path: `docs/${rel}`,
            title,
            category,
            categorySlug,
            order,
            description,
            locale,
        });
    }

    const locales = {};
    for (const [locale, docs] of byLocale) {
        locales[locale] = buildLocaleBundle(docs);
    }

    const defaultLocale = locales['en-US'] ? 'en-US' : Object.keys(locales)[0] || 'en-US';
    const totalDocs = Object.values(locales).reduce((n, b) => n + b.docs.length, 0);

    return {
        defaultLocale,
        locales,
        generatedAt: new Date().toISOString(),
        _totalDocs: totalDocs,
    };
}

if (existsSync(outDir)) {
    rmSync(outDir, {recursive: true, force: true});
}
ensureDir(outDir);

const docsIndex = generateDocsIndex();
const {_totalDocs, ...publicIndex} = docsIndex;
ensureDir(join(outDir, 'content'));
writeFileSync(
    join(outDir, 'content', 'docs-index.json'),
    JSON.stringify(publicIndex, null, 2),
);
copyDir(contentDocs, join(outDir, 'content', 'docs'));

const configUrl = pathToFileURL(join(root, 'rolldown.config.mjs')).href;
const {default: rolldownConfig} = await import(configUrl);
const build = await rolldown(rolldownConfig);
try {
    await build.write(rolldownConfig.output);
} finally {
    await build.close();
}

const mainJs = join(outDir, 'js', 'main.js');
if (!existsSync(mainJs)) {
    console.error('missing dist/js/main.js after Rolldown build');
    process.exit(1);
}

const styleEntry = join(root, 'css', 'style.css');
const cssDir = join(outDir, 'css');
ensureDir(cssDir);
const {code, warnings} = bundle({
    filename: styleEntry,
    minify: true,
});
if (warnings && warnings.length) {
    for (const w of warnings) console.warn('CSS warning:', w.message);
}
writeFileSync(join(cssDir, 'style.css'), code);

copyFileSync(join(root, 'index.html'), join(outDir, 'index.html'));
copyDir(join(root, 'svg'), join(outDir, 'svg'));
copyDir(join(root, 'assets'), join(outDir, 'assets'));

copyFileSync(join(root, 'index.html'), join(outDir, '404.html'));

const all = walk(outDir);
const localeList = Object.keys(publicIndex.locales || {}).join(', ') || '(none)';
console.log(`Website build OK (${all.length} files, ${_totalDocs} docs, locales: ${localeList}):`);
for (const f of all) {
    console.log('  ' + f.slice(outDir.length + 1).replaceAll(sep, '/'));
}
