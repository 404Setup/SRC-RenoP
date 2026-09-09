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
import vm from 'node:vm';
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
        '/account/forgot-password', '/ACCOUNT/FORGOT-PASSWORD/', '/account/register', '/ACCOUNT/REGISTER/', '/account/recovery', '/ACCOUNT/RECOVERY/', '/account/%72ecovery', '/api', '/API/auth/logout', '/api/auth/logout', '/' + 'a'.repeat(1024), '/' + '例'.repeat(500)]) {
        assert.equal(safeLoginReturnTo(value), '/', String(value));
    }
    assert.equal(safeLoginReturnTo('/user/alice?ignored=value#fragment'), '/user/alice');
    assert.equal(accountPageFromPath('/account/register/'), 'registration');
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
        assert.match(catalog['fido.waiting'], /\{seconds\}/, locale.name);
        for (const key of ['timeout', 'cancelled', 'alreadyRegistered', 'insecure', 'verificationUnavailable']) {
            assert.ok(catalog['fido.' + key]?.length > 0, `${locale.name} ${key}`);
        }
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

test('native Passkey prompts bound waiting, restore buttons, and discard cancelled or malformed results', async () => {
    let now = 0, nextTimer = 0, request, resolveCredential, calls = 0;
    const timeouts = new Map(), intervals = new Map(), listeners = new Map();
    const buffer = new Uint8Array([1, 2, 3]).buffer;
    const valid = {
        id: 'key', rawId: buffer, type: 'public-key', response: {
            authenticatorData: buffer, clientDataJSON: buffer, signature: buffer, attestationObject: buffer,
        }
    };
    const original = [{label: 'Passkey'}, {icon: true}], attributes = new Map();
    const button = {
        childNodes: [...original],
        replaceChildren(...nodes) {
            this.childNodes = nodes;
        },
        getAttribute: key => attributes.get(key) ?? null,
        setAttribute: (key, value) => attributes.set(key, value), removeAttribute: key => attributes.delete(key),
    };
    const browser = {
        PublicKeyCredential: true, isSecureContext: true,
        addEventListener: (name, fn) => listeners.set(name, fn), removeEventListener: name => listeners.delete(name),
    };
    const native = options => {
        calls++;
        request = options;
        return new Promise(resolve => {
            resolveCredential = resolve;
        });
    };
    const context = vm.createContext({
        Uint8Array, atob, btoa, AbortController, DOMException,
        window: browser, navigator: {credentials: {get: native, create: native}}, Date: {now: () => now},
        t: (key, values) => values ? `${key} (${values.seconds}s)` : key,
        setTimeout: (fn, delay) => {
            timeouts.set(++nextTimer, {fn, delay});
            return nextTimer;
        },
        clearTimeout: id => timeouts.delete(id),
        setInterval: fn => {
            intervals.set(++nextTimer, fn);
            return nextTimer;
        }, clearInterval: id => intervals.delete(id),
    });
    vm.runInContext(readFileSync(join(frontendRoot, 'js/fido-utils.js'), 'utf8').replace(/^import .*;\r?\n/gm, '').replaceAll('export ', ''), context);
    const options = {publicKey: {challenge: 'AQID', timeout: 120000}};
    const pending = context.requestPasskeyAssertion(options, {button});
    const rejected = assert.rejects(pending, {name: 'TimeoutError'});
    assert.equal(request.publicKey.timeout, 60000);
    assert.equal(options.publicKey.timeout, 120000, 'server options stay unchanged');
    assert.equal(button.childNodes[0], 'fido.waiting (60s)');
    assert.equal(attributes.get('aria-busy'), 'true');
    now = 1000;
    for (const tick of intervals.values()) tick();
    assert.equal(button.childNodes[0], 'fido.waiting (59s)');
    for (const {fn} of timeouts.values()) fn();
    await rejected;
    assert.equal(request.signal.aborted, true);
    resolveCredential(valid);
    await Promise.resolve();
    assert.deepEqual(button.childNodes, original);
    assert.equal(attributes.has('aria-busy'), false);
    assert.equal(timeouts.size + intervals.size + listeners.size, 0);

    const controller = new AbortController();
    const aborted = context.requestPasskeyAssertion(options, {signal: controller.signal});
    controller.abort();
    await assert.rejects(aborted, {name: 'AbortError'});
    const callCount = calls;
    await assert.rejects(context.requestPasskeyAssertion(options, {signal: controller.signal}), {name: 'AbortError'});
    assert.equal(calls, callCount, 'pre-cancelled requests never invoke a native prompt');

    const navigated = context.requestPasskeyAssertion(options);
    listeners.get('popstate')();
    await assert.rejects(navigated, {name: 'AbortError'});
    const late = context.requestPasskeyAssertion({publicKey: {challenge: 'AQID', timeout: 1000}});
    now += 1001;
    resolveCredential(valid);
    await assert.rejects(late, {name: 'TimeoutError'}, 'deadline also applies when timers are throttled');

    for (const [credential, name] of [[null, 'NotAllowedError'], [{...valid, response: {}}, 'DataError'], [{
        ...valid,
        type: 'password'
    }, 'DataError']]) {
        const invalid = context.requestPasskeyAssertion(options);
        resolveCredential(credential);
        await assert.rejects(invalid, {name});
    }
    browser.isSecureContext = false;
    await assert.rejects(context.requestPasskeyAssertion(options), {name: 'SecurityError'});
    browser.isSecureContext = true;
    browser.PublicKeyCredential = undefined;
    await assert.rejects(context.requestPasskeyAssertion(options), {name: 'NotSupportedError'});
    browser.PublicKeyCredential = true;
    const registered = context.requestPasskeyRegistration({
        publicKey: {
            challenge: 'AQID',
            user: {id: 'AQID'},
            excludeCredentials: [{id: 'AQID', type: 'public-key'}],
            authenticatorSelection: {userVerification: 'required'}
        }
    }, {button});
    assert.equal(request.publicKey.authenticatorSelection.userVerification, 'required');
    assert.deepEqual([...new Uint8Array(request.publicKey.user.id)], [1, 2, 3]);
    resolveCredential(valid);
    assert.equal((await registered).response.attestationObject, 'AQID');
    assert.deepEqual(button.childNodes, original);
    assert.equal(timeouts.size + intervals.size + listeners.size, 0);
    for (const name of ['TimeoutError', 'NotAllowedError', 'AbortError', 'InvalidStateError', 'NotSupportedError', 'SecurityError', 'ConstraintError']) {
        assert.notEqual(context.passkeyErrorMessage({name, message: 'private exception'}), 'login.fidoFailed');
    }
    assert.equal(context.passkeyErrorMessage({message: 'private exception'}), 'login.fidoFailed');
});

test('primary Passkey login never submits timed-out credentials or completes after navigation', async () => {
    const requests = [], completions = [], listeners = new Map();
    let delayedBegin = false, releaseBegin, delayedFinish = false, releaseFinish;
    let failure = new DOMException('', 'TimeoutError');
    const context = vm.createContext({
        AbortController, DOMException,
        window: {PublicKeyCredential: true, addEventListener: (type, callback) => listeners.set(type, callback)},
        document: {getElementById: () => ({value: 'alice'})},
        loginError: {style: {}, textContent: ''}, btnFidoLogin: {}, t: key => key,
        passkeyErrorMessage: error => error.name === 'TimeoutError' ? 'fido.timeout' : 'login.fidoFailed',
        requestPasskeyAssertion: async (_, {signal, button}) => {
            assert.ok(signal);
            assert.ok(button);
            if (failure) throw failure;
            return {id: 'credential'};
        },
        completeLogin: session => completions.push(session),
        fetch: async (url, options) => {
            requests.push({url, options});
            if (url.endsWith('/begin')) {
                if (delayedBegin) await new Promise(resolve => {
                    releaseBegin = resolve;
                });
                return {
                    ok: true,
                    json: async () => ({session_id: 'challenge', options: {publicKey: {challenge: 'AQID'}}})
                };
            }
            if (delayedFinish) await new Promise(resolve => {
                releaseFinish = resolve;
            });
            return {ok: true, json: async () => ({access_token: {name: 'alice'}})};
        },
    });
    const auth = readFileSync(join(frontendRoot, 'js/auth.js'), 'utf8');
    vm.runInContext(auth.slice(auth.indexOf('let fidoLoginAbort;'), auth.indexOf('if (loginBtn) {')).replaceAll('export ', ''), context);
    await context.fidoLogin();
    assert.equal(context.loginError.textContent, 'fido.timeout');
    assert.equal(requests.length, 1);
    assert.equal(completions.length, 0);
    failure = undefined;
    delayedBegin = true;
    const abandoned = context.fidoLogin();
    listeners.get('popstate')();
    releaseBegin();
    await abandoned;
    assert.equal(requests.length, 2);
    assert.equal(completions.length, 0);
    delayedBegin = false;
    delayedFinish = true;
    const late = context.fidoLogin();
    await new Promise(resolve => setImmediate(resolve));
    listeners.get('pagehide')();
    releaseFinish();
    await late;
    assert.equal(completions.length, 0);
    delayedFinish = false;
    await context.fidoLogin();
    assert.equal(completions.length, 1);
});
