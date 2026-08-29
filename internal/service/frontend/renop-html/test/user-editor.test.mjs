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

test('user editor uses semantic account fields and a responsive permission layout', () => {
    const index = readFileSync(join(frontendRoot, 'index.html'), 'utf8');
    const modal = readFileSync(join(frontendRoot, 'js/users/modal.js'), 'utf8');
    const permissions = readFileSync(join(frontendRoot, 'js/users/permissions.js'), 'utf8');
    const styles = readFileSync(join(frontendRoot, 'css/manager/users.css'), 'utf8');
    const appUI = readFileSync(join(frontendRoot, 'js/app-ui.js'), 'utf8');

    assert.match(index, /id="btn-create-user"/);
    assert.doesNotMatch(index, /id="btn-show-create-token"/);
    for (const id of ['user-editor-modal', 'user-editor-form', 'user-name', 'user-nickname', 'user-password']) {
        assert.ok(modal.includes(`'${id}'`), `missing semantic editor id ${id}`);
    }
    assert.match(modal, /payload\.secret = password/);
    assert.match(modal, /autocomplete: 'new-password'/);
    assert.match(modal, /aria-describedby/);
    assert.doesNotMatch(modal, /token-form|token-secret|create-token-modal|editToken/);
    assert.ok(appUI.includes("'user-editor-modal'"));
    assert.ok(appUI.includes("'user-password-result-modal'"));

    assert.match(permissions, /Object\.keys\(data\.repositories \|\| \{\}\)\.sort/);
    assert.match(permissions, /sequence !== permissionLoadSequence/);
    assert.match(styles, /\.user-editor-body\s*\{[^}]*grid-template-columns:/s);
    assert.match(styles, /@media \(max-width: 780px\)[\s\S]*?\.user-editor-body\s*\{[^}]*grid-template-columns: minmax\(0, 1fr\)/);
    assert.match(styles, /max-height: calc\(100dvh/);
});

test('user management locales describe account passwords rather than security tokens', async () => {
    const localeRoot = join(frontendRoot, 'js/i18n');
    const keys = [
        'users.subtitle',
        'users.passwordPlaceholder',
        'users.passwordCreateHint',
        'users.copySecretWarning',
        'users.confirmDeleteToken',
        'users.failedDeleteToken',
    ];
    const legacyCredentialTerms = /\btoken\b|secret|密钥|金鑰|シークレット|토큰|비밀\s*키|секрет/i;

    for (const locale of readdirSync(localeRoot, {withFileTypes: true}).filter(entry => entry.isDirectory())) {
        const module = await import(pathToFileURL(join(localeRoot, locale.name, 'management.js')).href);
        for (const key of keys) {
            assert.equal(typeof module.default[key], 'string', `${locale.name} ${key}`);
            assert.doesNotMatch(module.default[key], legacyCredentialTerms, `${locale.name} ${key}`);
        }
    }
});
