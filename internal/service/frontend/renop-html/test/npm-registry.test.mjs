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
import {fileURLToPath} from 'node:url';

const frontendRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const npmView = readFileSync(join(frontendRoot, 'js/browser/npm.js'), 'utf8');
const npmCSS = readFileSync(join(frontendRoot, 'css/browser/npm.css'), 'utf8');
const markdownCSS = readFileSync(join(frontendRoot, 'css/components/markdown.css'), 'utf8');
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
        'packageDetails.member_count',
        'distTagsSection',
        'setInviteValidation',
        'npm-invite-error',
        'createUserIdentity',
        'setSafeMarkdown',
        'packageDetails.project',
        'projectMetadataSection',
        'readmeSection',
        'version.integrity',
        'version.shasum',
        'version.review_status',
        'npm.reviewPending',
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
    assert.match(npmCSS, /\.npm-command-header strong\s*\{[^}]*color:\s*var\(--text-color\)[^}]*font-weight:\s*700/s,
        'npm command titles must remain visually distinct from muted command content');
    assert.match(npmCSS, /\.npm-command pre code\s*\{[^}]*color:\s*color-mix/s,
        'npm command content must retain its secondary text color');
    assert.match(npmCSS, /\.npm-command\s*\{[^}]*background:\s*color-mix\(in srgb, var\(--text-color\) 2\.5%, transparent\)/s,
        'npm command cards must use Maven-style neutral surfaces');
    assert.match(npmCSS, /\.npm-package-card\s*\{[^}]*background:\s*color-mix\(in srgb, var\(--text-color\) 2\.5%, transparent\)/s,
        'npm package cards must use Maven-style neutral surfaces');
    assert.match(npmCSS, /\.npm-invite-input\s*\{[^}]*border:[^}]*background:[^}]*color:/s,
        'npm invitation input must have complete application styling');
    assert.match(npmCSS, /\.npm-member-remove\s*\{[^}]*border:[^}]*background:[^}]*color:\s*#dc2626/s,
        'npm member removal must have an explicit destructive style');
    assert.match(npmCSS, /\.npm-information-grid\s*\{[^}]*grid-template-columns:/s,
        'npm package metadata must use the responsive information grid');
    assert.match(markdownCSS, /\.repository-markdown\s*\{[^}]*overflow-wrap:\s*anywhere/s,
        'npm README content must remain within the package layout');
    assert.match(npmCSS, /@media \(max-width: 700px\)[\s\S]*\.npm-invite-controls\s*\{[^}]*grid-template-columns:\s*minmax\(0, 1fr\)/);
});
