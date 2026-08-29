/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

import assert from 'node:assert/strict';
import {readdirSync, readFileSync} from 'node:fs';
import {dirname, join, resolve} from 'node:path';
import test from 'node:test';
import {fileURLToPath, pathToFileURL} from 'node:url';

const frontendRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const repositoryRoot = resolve(frontendRoot, '..', '..', '..', '..');

test('Maven and npm package disclosures use one reversible height controller', () => {
    const uiPackage = JSON.parse(readFileSync(join(repositoryRoot, 'packages/renop-ui/package.json'), 'utf8'));
    const disclosure = readFileSync(join(repositoryRoot, 'packages/renop-ui/js/disclosure.js'), 'utf8');
    const maven = readFileSync(join(frontendRoot, 'js/browser/maven.js'), 'utf8');
    const npm = readFileSync(join(frontendRoot, 'js/browser/npm.js'), 'utf8');

    assert.equal(uiPackage.exports['./disclosure'], './js/disclosure.js');
    assert.match(disclosure, /desiredOpen = !desiredOpen/);
    assert.match(disclosure, /collapseElement\(content/);
    assert.match(disclosure, /expandElement\(content/);
    assert.match(disclosure, /currentOperation === operation/);
    assert.match(maven, /bindAnimatedDetails\(details, \{content: body, marginTop: '0\.55rem'\}\)/);
    assert.match(npm, /bindAnimatedDetails\(details, \{content: detailsBody, marginTop: '0\.7rem'\}\)/);
});

test('description dialogs use property-backed values and one shared save translation', async () => {
    const maven = readFileSync(join(frontendRoot, 'js/browser/maven.js'), 'utf8');
    const npm = readFileSync(join(frontendRoot, 'js/browser/npm.js'), 'utf8');
    assert.match(maven, /rows: '8', value: artifact\.description \|\| ''/);
    assert.match(npm, /rows: '5', value: packageDetails\.package\.description \|\| ''/);
    assert.match(maven, /text: t\('common\.save'\)/);
    assert.match(npm, /text: t\('common\.save'\)/);

    const localeRoot = join(frontendRoot, 'js/i18n');
    for (const locale of readdirSync(localeRoot, {withFileTypes: true}).filter(entry => entry.isDirectory())) {
        const common = await import(pathToFileURL(join(localeRoot, locale.name, 'common.js')).href);
        const npmCatalog = await import(pathToFileURL(join(localeRoot, locale.name, 'npm.js')).href);
        assert.notEqual(common.default['common.save'], 'common.save', `${locale.name} common.save`);
        assert.equal(Object.hasOwn(npmCatalog.default, 'npm.save'), false, `${locale.name} duplicate npm.save`);
    }
});

test('files repositories hide protocol snippets and npm metadata stays compact but interactive', () => {
    const snippets = readFileSync(join(frontendRoot, 'js/browser/snippets.js'), 'utf8');
    const npmStyles = readFileSync(join(frontendRoot, 'css/browser/npm.css'), 'utf8');
    const personRule = npmStyles.match(/\.npm-project-person\s*\{[^}]*\}/s)?.[0] || '';

    assert.match(snippets, /if \(format\.id === 'files'\) \{[\s\S]*?currentSnippets = \{\};[\s\S]*?return;/);
    assert.ok(snippets.indexOf("if (format.id === 'files')") < snippets.indexOf("if (card) card.style.display = '';"));
    assert.doesNotMatch(personRule, /background|border-radius|border:/);
    assert.match(npmStyles, /\.npm-metadata-copy:hover,[\s\S]*?transform: translateY\(-1px\)/);
    assert.match(npmStyles, /\.npm-metadata-copy\.copied/);
    assert.match(npmStyles, /\.npm-metadata-copy \.copy-toast/);
});
