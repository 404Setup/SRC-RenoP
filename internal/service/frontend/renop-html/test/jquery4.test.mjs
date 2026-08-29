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

test('jQuery 4 is self-hosted through one shared runtime', () => {
    const uiPackage = JSON.parse(readFileSync(join(repositoryRoot, 'packages/renop-ui/package.json'), 'utf8'));
    const runtime = readFileSync(join(repositoryRoot, 'packages/renop-ui/js/jquery.js'), 'utf8');
    const appMain = readFileSync(join(frontendRoot, 'js/main.js'), 'utf8');
    const webMain = readFileSync(join(repositoryRoot, 'web/js/main.js'), 'utf8');
    const lockfile = readFileSync(join(repositoryRoot, 'pnpm-lock.yaml'), 'utf8');

    assert.equal(uiPackage.dependencies.jquery, '4.0.0');
    assert.equal(uiPackage.exports['./jquery'], './js/jquery.js');
    assert.match(runtime, /await import\('jquery'\)/);
    assert.match(runtime, /startsWith\('4\.'\)/);
    assert.match(runtime, /export const \$ = jquery/);
    assert.match(runtime, /installJQueryGlobals\(window\)/);
    assert.match(runtime, /target\.\$ == null \|\| target\.\$ === existing/);
    assert.match(runtime, /new target\.CustomEvent\('jqueryReady'/);
    assert.match(appMain, /import \{\$\} from '@renop\/ui\/jquery'/);
    assert.match(appMain, /\$\(initializeApplication\)/);
    assert.match(webMain, /import \{\$\} from '@renop\/ui\/jquery'/);
    assert.match(webMain, /\$\(renderRoute\)/);
    assert.match(lockfile, /jquery@4\.0\.0:/);
    assert.doesNotMatch(lockfile, /jquery-migrate/i);
});

test('shared UI and application interaction layers use the jQuery 4 boundary', () => {
    const migratedFiles = [
        'packages/renop-ui/js/dom.js',
        'packages/renop-ui/js/theme.js',
        'packages/renop-ui/js/modal.js',
        'packages/renop-ui/js/tabs.js',
        'packages/renop-ui/js/custom-select.js',
        'packages/renop-ui/js/toggle.js',
        'packages/renop-ui/js/button.js',
        'packages/renop-ui/js/lang-card.js',
        'packages/renop-ui/js/scroll.js',
        'packages/renop-ui/js/i18n-util.js',
        'internal/service/frontend/renop-html/js/account-security.js',
        'internal/service/frontend/renop-html/js/github-auth.js',
        'web/js/router.js',
    ];
    const removedAPIs = /\$\.(?:trim|isArray|parseJSON|now|camelCase|isFunction|isNumeric|type|proxy|holdReady)\s*\(|\.(?:size|andSelf|bind|unbind|delegate|undelegate)\s*\(/;

    for (const relativePath of migratedFiles) {
        const source = readFileSync(join(repositoryRoot, relativePath), 'utf8');
        assert.match(source, /import \{\$\} from ['"](?:\.\/jquery\.js|@renop\/ui\/jquery)['"]/, relativePath);
        assert.match(source, /\$\(/, relativePath);
        assert.doesNotMatch(source, removedAPIs, relativePath);
    }
});

test('both browser builds isolate deferred libraries in hashed chunks', () => {
    for (const configPath of [
        join(frontendRoot, 'rolldown.config.mjs'),
        join(repositoryRoot, 'web/rolldown.config.mjs'),
    ]) {
        const config = readFileSync(configPath, 'utf8');
        assert.match(config, /codeSplitting: true/);
        assert.match(config, /chunkFileNames: 'js\/chunks\/\[name\]-\[hash\]\.js'/);
    }
});
