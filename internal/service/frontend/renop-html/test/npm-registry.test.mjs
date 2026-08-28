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
const npmView = readFileSync(join(frontendRoot, 'js/browser/npm.js'), 'utf8');
const npmCSS = readFileSync(join(frontendRoot, 'css/browser/npm.css'), 'utf8');
const formats = readFileSync(join(frontendRoot, 'js/repository-formats.js'), 'utf8');
const repositorySettings = readFileSync(join(frontendRoot, 'js/repositories.js'), 'utf8');

test('npm repository UI uses shared routing, errors, clipboard, and team controls', () => {
    for (const required of [
        'ensureRepositoryView',
        'replaceRepositoryView',
        'createRepositoryMirrorBadge',
        'copyWithFeedback',
        'RepositoryUserSuggestions',
        'makeCustomSelect',
        'npmResponseError',
        'NPMRequestError',
        'packageDetails.member',
        'distTagsSection',
    ]) {
        assert.ok(npmView.includes(required), `npm repository view is missing ${required}`);
    }
    assert.doesNotMatch(npmView, /showAlert\(\s*error\.message/,
        'npm repository view exposes an untrusted runtime error');
    assert.doesNotMatch(npmView, /placeholder:\s*['"]@scope\/package/,
        'npm package placeholders must be localized');
});

test('npm repository format and mirror controls are canonical and responsive', () => {
    for (const required of [
        "id: 'npm'",
        "icon: 'repositoryNpm'",
        "snippetTabs: Object.freeze(['npm-config', 'npm-install', 'npm-publish'])",
    ]) {
        assert.ok(formats.includes(required), `npm format descriptor is missing ${required}`);
    }
    for (const required of [
        "const isNPM = format.id === 'npm'",
        "mirrorUrlPlaceholder = 'https://registry.npmjs.org/'",
        "addRulePlaceholder = 'repos.npmAddRulePlaceholder'",
    ]) {
        assert.ok(repositorySettings.includes(required), `npm mirror settings are missing ${required}`);
    }
    assert.match(npmCSS, /\.npm-command pre\s*\{[^}]*overflow-x:\s*auto/s);
    assert.match(npmCSS, /@media \(max-width: 700px\)[\s\S]*\.npm-invite\s*\{[^}]*grid-template-columns:\s*minmax\(0, 1fr\)/);
});
