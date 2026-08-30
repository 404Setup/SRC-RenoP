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
import {readFileSync} from 'node:fs';
import {dirname, join, resolve} from 'node:path';
import test from 'node:test';
import {fileURLToPath, pathToFileURL} from 'node:url';

const frontendRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const markdownPath = join(frontendRoot, 'js/markdown.js');
const markdownSource = readFileSync(markdownPath, 'utf8');
const markdownStyles = readFileSync(join(frontendRoot, 'css/components/markdown.css'), 'utf8');
const {safeMarkdownURL} = await import(pathToFileURL(markdownPath));

test('package Markdown accepts only credential-free absolute HTTP links', () => {
    assert.equal(safeMarkdownURL('https://example.test/project?q=1'), 'https://example.test/project?q=1');
    assert.equal(safeMarkdownURL('http://example.test/readme'), 'http://example.test/readme');
    for (const unsafe of [
        'javascript:alert(1)', 'data:text/html,unsafe', '/relative', '//example.test/path',
        'https://user:secret@example.test/path', 'file:///etc/passwd'
    ]) {
        assert.equal(safeMarkdownURL(unsafe), '', `unsafe Markdown URL was accepted: ${unsafe}`);
    }
});

test('all package formats share the safe Markdown renderer and neutral layout', () => {
    for (const format of ['npm', 'docker', 'cargo', 'maven']) {
        const source = readFileSync(join(frontendRoot, `js/browser/${format}.js`), 'utf8');
        assert.ok(source.includes('setSafeMarkdown'), `${format} does not use the safe Markdown renderer`);
        assert.ok(source.includes('repository-markdown'), `${format} does not use the shared Markdown layout`);
    }
    for (const required of ['overflow-wrap: anywhere', 'overflow-x: auto', 'max-width: 100%', 'var(--text-color) 5%']) {
        assert.ok(markdownStyles.includes(required), `shared Markdown styles are missing ${required}`);
    }
});

test('package Markdown uses an inert allowlist and strips active attributes', () => {
    for (const required of [
        "html(token)", "document.createElement('template')", "querySelectorAll('*')",
        'element.removeAttribute(attribute.name)', "rel', 'noopener noreferrer nofollow'",
        "referrerpolicy', 'no-referrer'", 'allowedMarkdownElements'
    ]) {
        assert.ok(markdownSource.includes(required), `Markdown boundary is missing ${required}`);
    }
    assert.doesNotMatch(markdownSource, /target\.innerHTML\s*=/,
        'rendered Markdown must not be assigned directly to an active element');
});
