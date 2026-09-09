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

test('account recovery validates distinct codes, submits once, clears secrets, and restores sign-in', async () => {
    const elements = new Map();
    const field = id => {
        if (!elements.has(id)) elements.set(id, {
            value: '', textContent: '', labels: [{}], focus() {
            }
        });
        return elements.get(id);
    };
    const codes = Array.from({length: 4}, (_, i) => field('code-' + i));
    const button = {disabled: false};
    let submit, pagehide, pending, release, requests = 0, allowWeak = true, status = 500;
    const events = [];
    const form = {
        querySelectorAll: () => codes,
        querySelector: () => button,
        addEventListener: (name, handler) => {
            submit = handler;
        },
        reset: () => {
            for (const element of elements.values()) element.value = '';
        },
    };
    const context = vm.createContext({
        document: {getElementById: id => id === 'account-recovery-form' ? form : field(id)},
        window: {
            addEventListener: (name, handler) => {
                pagehide = handler;
            }
        },
        t: key => key,
        attachPasswordStrength: () => ({
            reset() {
            }
        }),
        getPasswordLengthError: value => value.length < 6 ? 'short' : '',
        confirmWeakPasswordIfNeeded: async () => allowWeak,
        loginReturnTo: () => '/account/reviews',
        navigateToLogin: (path, options) => {
            events.push(['login', path, options.replace]);
        },
        logout: async reason => {
            events.push(['logout', reason]);
        },
        showAlert: () => {
            events.push(['success']);
        },
        fetch: async (url, options) => {
            requests++;
            assert.equal(url, '/api/auth/recovery/password');
            assert.equal(options.credentials, 'include');
            assert.equal(options.cache, 'no-store');
            assert.equal(JSON.parse(options.body).new_password, 'a valid password');
            await new Promise(resolve => {
                release = resolve;
            });
            return {ok: status === 200, status, json: async () => ({username: 'alice'})};
        },
        console,
    });
    const buttonSource = readFileSync(new URL('../js/components/button.js', import.meta.url), 'utf8');
    const action = vm.runInContext(buttonSource.slice(buttonSource.indexOf('export async function runButtonAction')).replace('export ', '') + '; runButtonAction', context);
    context.runButtonAction = (button, task) => {
        const result = action(button, task);
        if (!pending) pending = result;
        return result;
    };
    const source = readFileSync(new URL('../js/account-recovery.js', import.meta.url), 'utf8');
    vm.runInContext(source.replace(/^import .*;$/gm, '').replace('export function ', 'function '), context);
    const send = () => submit({
        preventDefault() {
        }
    });
    const finish = async () => {
        await pending;
        pending = null;
    };
    const fill = () => {
        field('recovery-identifier').value = 'alice';
        field('recovery-password').value = field('recovery-password-confirmation').value = 'a valid password';
        codes.forEach((input, i) => {
            input.value = 'CODE-' + i;
        });
    };
    fill();
    field('recovery-password-confirmation').value = 'different';
    send();
    await finish();
    assert.equal(field('recovery-error').textContent, 'login.passwordsDoNotMatch');
    fill();
    codes[1].value = ' code-0 ';
    send();
    await finish();
    assert.equal(field('recovery-error').textContent, 'login.fourDistinctCodesRequired');
    fill();
    allowWeak = false;
    send();
    await finish();
    assert.equal(requests, 0);
    allowWeak = true;
    for (status of [500, 401, 429, 200]) {
        send();
        send();
        await new Promise(resolve => setImmediate(resolve));
        assert.equal(button.disabled, true);
        release();
        await finish();
        assert.equal(button.disabled, false);
        if (status !== 200) assert.equal(field('recovery-error').textContent,
            status === 500 ? 'login.recoveryFailed' : status === 401 ? 'login.recoveryInvalid' : 'login.recoveryRateLimited');
    }
    assert.equal(requests, 4);
    assert.deepEqual(events, [['logout', 'silent'], ['login', '/account/reviews', true], ['success']]);
    assert.equal(field('username').value, 'alice');
    assert.equal(field('recovery-password').value, '');
    assert.ok(codes.every(input => input.value === ''));
    fill();
    pagehide();
    assert.equal(field('recovery-password').value, '');
    assert.ok(codes.every(input => input.value === ''));
});
