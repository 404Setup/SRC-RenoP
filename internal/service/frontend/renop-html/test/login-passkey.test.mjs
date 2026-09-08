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
import {readdirSync, readFileSync} from 'node:fs';
import {dirname, join, resolve} from 'node:path';
import test from 'node:test';
import {fileURLToPath, pathToFileURL} from 'node:url';
import {accountPageFromPath, isLoginPath, loginReturnTo, safeLoginReturnTo} from '../js/login-route.js';

const frontendRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');

test('sign-in return paths stay local and cannot reenter authentication endpoints', () => {
    for (const value of ['/account/reviews', '/user/alice/edit', '/packages/%E4%BB%A3%E7%A0%81/']) {
        assert.equal(safeLoginReturnTo(value), value);
        assert.equal(loginReturnTo('?return_to=' + encodeURIComponent(value)), value);
    }
    for (const value of [null, '', 'https://evil.example', '//evil.example', '/\\evil.example',
        '/%2f%2fevil.example', '/%5cevil.example', '/bad%path', '/\nwrong', '/%0awrong',
        '/account/login', '/account/login/', '/ACCOUNT/LOGIN', '/account/%6cogin', '/foo/../account/login',
        '/account/recovery', '/ACCOUNT/RECOVERY/', '/account/%72ecovery', '/api', '/API/auth/logout', '/api/auth/logout', '/' + 'a'.repeat(1024), '/' + '例'.repeat(500)]) {
        assert.equal(safeLoginReturnTo(value), '/', String(value));
    }
    assert.equal(safeLoginReturnTo('/user/alice?ignored=value#fragment'), '/user/alice');
    assert.equal(isLoginPath('/account/login/'), true);
    assert.equal(isLoginPath('/account/login/extra'), false);
    assert.equal(accountPageFromPath('/ACCOUNT/RECOVERY/'), 'recovery');
    assert.equal(accountPageFromPath('/account/recovery/extra'), '');
    const index = readFileSync(join(frontendRoot, 'index.html'), 'utf8');
    assert.match(index, /<section[^>]*id="tab-content-login"/);
    assert.ok(index.indexOf('id="login-form"') < index.indexOf('</main>'));
    assert.doesNotMatch(index, /id="login-modal"|id="close-login-modal"/);
    const github = readFileSync(join(frontendRoot, 'js/github-auth.js'), 'utf8');
    assert.match(github, /isLoginPath\(\) \? loginReturnTo\(\)/);
});

test('login alternatives place Passkey before optional GitHub below the divider', () => {
    const index = readFileSync(join(frontendRoot, 'index.html'), 'utf8');
    const submit = index.indexOf('class="account-submit"');
    const divider = index.indexOf('class="account-provider-divider"');
    const passkey = index.indexOf('id="btn-fido-login"');
    const github = index.indexOf('id="btn-github-login"');
    assert.ok(submit >= 0 && divider > submit && passkey > divider && github > passkey);
    assert.match(index, /<button(?=[^>]*id="btn-fido-login")(?=[^>]*class="account-provider")[^>]*>/);
    assert.doesNotMatch(index, /Username, email, or token name|Password \/ Secret|password or secret/i);

    const auth = readFileSync(join(frontendRoot, 'js/auth.js'), 'utf8');
    assert.ok(auth.includes("runButtonAction(btnFidoLogin, fidoLogin)"));
    const styles = readFileSync(join(frontendRoot, 'css/account-pages.css'), 'utf8');
    assert.ok(styles.includes('.account-page'));
    assert.ok(styles.includes('.account-providers'));
    const loginPage = index.slice(index.indexOf('id="tab-content-login"'), index.indexOf('id="tab-content-overview"'));
    assert.doesNotMatch(loginPage, /modal-|form-group|submit-btn|login-provider-btn/);
    assert.match(index, /<button[^>]*id="login-btn"[^>]*type="button"/);
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
