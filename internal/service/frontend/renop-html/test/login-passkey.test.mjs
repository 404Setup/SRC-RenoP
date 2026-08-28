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

test('login alternatives place Passkey before optional GitHub below the divider', () => {
    const index = readFileSync(join(frontendRoot, 'index.html'), 'utf8');
    const submit = index.indexOf('type="submit" class="submit-btn"');
    const divider = index.indexOf('class="login-provider-divider"');
    const passkey = index.indexOf('id="btn-fido-login"');
    const github = index.indexOf('id="btn-github-login"');
    assert.ok(submit >= 0 && divider > submit && passkey > divider && github > passkey);
    assert.match(index, /<button[^>]+id="btn-fido-login"[^>]+login-provider-btn/);
    assert.doesNotMatch(index, /Username, email, or token name|Password \/ Secret|password or secret/i);

    const auth = readFileSync(join(frontendRoot, 'js/auth.js'), 'utf8');
    assert.ok(auth.includes("runButtonAction(btnFidoLogin, fidoLogin)"));
    const styles = readFileSync(join(frontendRoot, 'css/components/button.css'), 'utf8');
    assert.ok(styles.includes('.login-provider-section'));
    assert.ok(styles.includes('.passkey-login-btn'));
});

test('all locales use Passkey copy and password-only login fields', async () => {
    const localeRoot = join(frontendRoot, 'js/i18n');
    for (const locale of readdirSync(localeRoot, {withFileTypes: true}).filter(entry => entry.isDirectory())) {
        const authModule = await import(pathToFileURL(join(localeRoot, locale.name, 'auth.js')).href);
        const catalog = authModule.default;
        assert.match(catalog['login.fidoLogin'], /Passkey/i, `${locale.name} Passkey action`);
        assert.doesNotMatch(catalog['login.fidoLogin'], /FIDO/i, `${locale.name} legacy FIDO action`);
        for (const key of ['login.usernameLabel', 'login.usernamePlaceholder']) {
            assert.doesNotMatch(catalog[key], /token|secret|密钥|金鑰|シークレット|비밀키|секрет/i,
                `${locale.name} ${key}`);
        }
        for (const key of ['login.passwordLabel', 'login.passwordPlaceholder']) {
            assert.doesNotMatch(catalog[key], /secret|密钥|金鑰|シークレット|비밀키|секрет/i,
                `${locale.name} ${key}`);
        }
        for (const fragment of ['auth.js', 'profile.js', 'management.js']) {
            const module = await import(pathToFileURL(join(localeRoot, locale.name, fragment)).href);
            for (const [key, value] of Object.entries(module.default)) {
                if (key.toLowerCase().includes('fido')) {
                    assert.doesNotMatch(value, /FIDO/i, `${locale.name} ${fragment} ${key}`);
                }
            }
        }
    }
});
