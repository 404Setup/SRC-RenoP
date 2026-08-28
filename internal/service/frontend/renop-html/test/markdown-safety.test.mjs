/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {dirname, join, resolve} from 'node:path';
import test from 'node:test';
import {fileURLToPath, pathToFileURL} from 'node:url';

const frontendRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const markdownPath = join(frontendRoot, 'js/markdown.js');
const markdownSource = readFileSync(markdownPath, 'utf8');
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
