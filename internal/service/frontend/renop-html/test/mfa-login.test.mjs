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
import test from 'node:test';
import vm from 'node:vm';

test('second-factor login keeps rejected and stale challenges out of authenticated UI', async () => {
    const elements = new Map(), requests = [], events = [], timers = new Map();
    let timerID = 0, accept = false, available = true, delayed = false, release, passkeyError;
    const field = id => {
        if (!elements.has(id)) elements.set(id, {
            value: '', hidden: true, disabled: false, textContent: '', style: {}, handlers: {},
            focus() {
            }, checkValidity() {
                return /^\d{6}$/.test(this.value);
            },
            reset() {
                field('mfa-login-code').value = '';
            }, addEventListener(name, fn) {
                this.handlers[name] = fn;
            }
        });
        return elements.get(id);
    };
    field('login-form').hidden = false;
    const context = vm.createContext({
        document: {getElementById: field},
        window: {
            addEventListener() {
            }, dispatchEvent: event => events.push(event)
        },
        AbortController, Date, CustomEvent: class {
            constructor(type, {detail}) {
                this.type = type;
                this.detail = detail;
            }
        },
        setTimeout: fn => {
            timers.set(++timerID, fn);
            return timerID;
        }, clearTimeout: id => timers.delete(id),
        t: key => key, responseErrorMessage: async () => 'mfa.invalid',
        passkeyErrorMessage: error => error.name === 'TimeoutError' ? 'fido.timeout' : 'login.fidoFailed',
        requestPasskeyAssertion: async options => {
            assert.equal(options.publicKey.userVerification, 'required');
            if (passkeyError) throw passkeyError;
            return {id: 'credential'};
        },
        fetch: async (url, options) => {
            assert.equal(options.credentials, 'include');
            assert.equal(options.cache, 'no-store');
            requests.push({url, options});
            if (options.method === 'GET') return {
                ok: available,
                json: async () => ({totp: true, passkey: true, expires_at: Date.now() + 300000})
            };
            if (options.method === 'DELETE') return {ok: true};
            if (url.endsWith('/begin')) return {
                ok: true,
                json: async () => ({options: {publicKey: {userVerification: 'required'}}})
            };
            if (delayed) await new Promise(resolve => {
                release = resolve;
            });
            return {ok: accept, json: async () => ({access_token: {name: 'alice'}})};
        },
    });
    const button = readFileSync(new URL('../js/components/button.js', import.meta.url), 'utf8');
    context.runButtonAction = vm.runInContext(button.slice(button.indexOf('export async function runButtonAction')).replace('export ', '') + '; runButtonAction', context);
    const source = readFileSync(new URL('../js/mfa-login.js', import.meta.url), 'utf8').replace(/^import .*;\r?\n/gm, '').replaceAll('export ', '');
    vm.runInContext(source, context);
    const settle = async () => {
        for (let i = 0; i < 8; i++) await new Promise(resolve => setImmediate(resolve));
    };
    available = false;
    await context.showMFALogin();
    assert.equal(field('login-error').textContent, 'mfa.invalid');
    assert.equal(field('login-error').style.display, 'block');
    available = true;
    await context.showMFALogin();
    assert.equal(field('login-form').hidden, true);
    assert.equal(field('mfa-login-form').hidden, false);
    field('mfa-login-code').value = '123456';
    await context.verifyFactor('totp');
    assert.equal(events.length, 0);
    assert.equal(field('mfa-login-code').value, '');
    assert.equal(field('mfa-login-error').hidden, false);
    accept = true;
    await context.verifyFactor('passkey');
    assert.equal(events.length, 1);
    assert.equal(events[0].detail.access_token.name, 'alice');
    assert.equal(field('mfa-login-form').hidden, true);
    assert.equal(field('login-form').hidden, false);
    assert.equal(timers.size, 0);
    await context.showMFALogin();
    delayed = true;
    field('mfa-login-code').value = '123456';
    const pending = context.verifyFactor('totp');
    await settle();
    context.updateMFALoginPage(false);
    release();
    await pending;
    assert.equal(events.length, 1, 'late success must not navigate after leaving');
    assert.equal(field('mfa-login-code').value, '');
    assert.ok(requests.some(request => request.options.method === 'DELETE'));
    delayed = false;
    await context.showMFALogin();
    const finishes = requests.filter(request => request.url.endsWith('/finish')).length;
    passkeyError = new DOMException('', 'TimeoutError');
    await context.verifyFactor('passkey');
    assert.equal(field('login-form').hidden, false);
    assert.equal(field('login-error').textContent, 'fido.timeout');
    assert.equal(timers.size, 0);
    assert.equal(requests.filter(request => request.url.endsWith('/finish')).length, finishes);
    assert.equal(events.length, 1);
    await context.showMFALogin();
    for (const expire of [...timers.values()]) expire();
    assert.equal(field('login-error').textContent, 'mfa.invalid');
    assert.equal(field('mfa-login-form').hidden, true);
});

test('Passkey assertion conversion preserves required user verification', async () => {
    let request;
    const buffer = new Uint8Array([1, 2, 3]).buffer;
    const context = vm.createContext({
        Uint8Array, atob, btoa, AbortController, DOMException, setTimeout, clearTimeout, setInterval, clearInterval,
        window: {
            PublicKeyCredential: true, addEventListener() {
            }, removeEventListener() {
            }
        },
        navigator: {
            credentials: {
                get: async options => {
                    request = options;
                    return {
                        id: 'key',
                        rawId: buffer,
                        type: 'public-key',
                        response: {
                            authenticatorData: buffer,
                            clientDataJSON: buffer,
                            signature: buffer,
                            userHandle: null
                        }
                    };
                }
            }
        },
    });
    vm.runInContext(readFileSync(new URL('../js/fido-utils.js', import.meta.url), 'utf8').replace(/^import .*;\r?\n/gm, '').replaceAll('export ', ''), context);
    const result = await context.requestPasskeyAssertion({
        publicKey: {
            challenge: 'AQID',
            allowCredentials: [{type: 'public-key', id: 'AQID'}],
            userVerification: 'required'
        }
    });
    assert.equal(request.publicKey.userVerification, 'required');
    assert.deepEqual([...new Uint8Array(request.publicKey.challenge)], [1, 2, 3]);
    assert.deepEqual([...new Uint8Array(request.publicKey.allowCredentials[0].id)], [1, 2, 3]);
    assert.equal(result.response.signature, 'AQID');
    assert.equal(result.response.userHandle, null);
});
