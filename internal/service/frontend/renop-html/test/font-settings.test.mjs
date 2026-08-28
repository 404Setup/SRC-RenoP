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
const repositoryRoot = resolve(frontendRoot, '..', '..', '..', '..');

test('frontend font settings use the shared styled select and bounded custom URL field', () => {
    const settings = readFileSync(join(frontendRoot, 'js/settings.js'), 'utf8');
    for (const required of [
        "{value: 'system', label: t('settings.fontSystem')}",
        "{value: 'custom', label: t('settings.fontCustom')}",
        'makeCustomSelect(fontOptions',
        "buildInput('url', currentConfig.font_url",
        'fontUrlInput.required = custom',
    ]) {
        assert.ok(settings.includes(required), `font settings are missing ${required}`);
    }
});

test('custom font loading is asynchronous and activates only after a complete load', () => {
    const source = readFileSync(join(frontendRoot, 'js/font.js'), 'utf8');
    const idle = source.indexOf('requestIdleCallback');
    const createFace = source.indexOf('new FontFace');
    const awaitLoad = source.indexOf('await withFontTimeout(face.load())');
    const addFace = source.indexOf('document.fonts.add(loaded)', createFace);
    assert.ok(idle >= 0 && createFace >= 0 && awaitLoad > createFace && addFace > awaitLoad);
    assert.ok(source.includes("root.dataset.customFontLoaded = 'true'"));
    assert.ok(source.includes("googleFontsStylesheetHost = 'fonts.googleapis.com'"));
    assert.ok(source.includes("parsed.searchParams.getAll('family')"));
    assert.ok(source.includes("link.media = 'print'"));
    assert.ok(source.includes('document.fonts.load('));
    assert.ok(source.includes("root.style.setProperty('--font-sans'"));
    assert.doesNotMatch(source, /document\.write|<link[^>]+stylesheet/i);
});

test('the system font baseline is shared and the H5 shell carries only inert font metadata', () => {
    const tokens = readFileSync(join(repositoryRoot, 'packages/renop-ui/css/tokens.css'), 'utf8');
    const base = readFileSync(join(repositoryRoot, 'packages/renop-ui/css/base.css'), 'utf8');
    const index = readFileSync(join(frontendRoot, 'index.html'), 'utf8');
    assert.ok(tokens.includes('--font-sans: system-ui'));
    assert.ok(base.includes('font-family: var(--font-sans'));
    assert.doesNotMatch(base, /font-family:\s*['"]Open Sans/);
    assert.ok(index.includes('data-font-preset="{{RENOP.FONT_PRESET}}"'));
    assert.ok(index.includes('meta name="renop-font-url" content="{{RENOP.FONT_URL}}"'));
    assert.doesNotMatch(index, /RENOP\.FONT_URL[^>]+(?:stylesheet|preload)/);
});
