/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import test from 'node:test';
import vm from 'node:vm';

test('email recovery gates mail, submits once, polls privately, and clears stale page state', async () => {
    const elements = new Map(), timers = new Map(), requests = [], navigation = [], links = [{hidden: true}];
    let timerID = 0, pagehide, enabled = false, status = 'queued', allowWeak = true, failConfirm = true, failPoll = false, delaySend = false, release;
    const field = id => {
        if (!elements.has(id)) elements.set(id, {value: '', textContent: '', disabled: false, hidden: false, handlers: {}, focus() {}, reportValidity() { return this.value.includes('@'); }, addEventListener(name, fn) { this.handlers[name] = fn; }});
        return elements.get(id);
    };
    const form = field('password-reset-form');
    const submit = field('submit');
    form.querySelector = () => submit;
    form.reset = () => { for (const element of elements.values()) element.value = ''; };
    field('password-reset-fields').disabled = true;
    class LocalizedResponseError extends Error {}
    const context = vm.createContext({
        document: {getElementById: field, querySelectorAll: () => links},
        window: {addEventListener: (name, fn) => { pagehide = fn; }},
        AbortSignal: {timeout: () => undefined},
        setTimeout: fn => { timers.set(++timerID, fn); return timerID; },
        clearTimeout: id => { timers.delete(id); },
        t: key => key, LocalizedResponseError,
        responseErrorMessage: async () => 'login.emailCodeInvalid',
        mailStatusLabel: status => 'mail.status.' + status,
        attachPasswordStrength: () => ({reset() {}}),
        getPasswordLengthError: value => value.length < 6 ? 'short' : '',
        confirmWeakPasswordIfNeeded: async () => allowWeak,
        loginReturnTo: () => '/account/reviews',
        navigateToLogin: (path, options) => { navigation.push(['login', path, options.replace]); },
        logout: async reason => { navigation.push(['logout', reason]); context.updatePasswordRecoveryPage(false); },
        showAlert: () => { navigation.push(['success']); },
        fetch: async (url, options) => {
            requests.push({url, options});
            assert.equal(options.credentials, 'include');
            assert.equal(options.cache, 'no-store');
            let body;
            if (url.endsWith('/status')) body = {enabled};
            else if (url.endsWith('/request')) {
                if (delaySend) await new Promise(resolve => { release = resolve; });
                body = {id: 'job', ticket: 'private-ticket', status: 'queued'};
            } else if (url.endsWith('/confirm')) {
                if (failConfirm) return {ok: false, status: 401};
                body = {username: 'alice'};
            } else {
                assert.equal(url, '/api/auth/mail/job');
                assert.equal(options.headers['X-Renop-Mail-Ticket'], 'private-ticket');
                if (failPoll) throw new Error('network');
                body = {status};
            }
            return {ok: true, json: async () => body};
        },
    });
    const buttonSource = readFileSync(new URL('../js/components/button.js', import.meta.url), 'utf8');
    const action = vm.runInContext(buttonSource.slice(buttonSource.indexOf('export async function runButtonAction')).replace('export ', '') + '; runButtonAction', context);
    const pending = [];
    context.runButtonAction = (button, fn) => { const promise = action(button, fn); pending.push(promise); return promise; };
    const source = readFileSync(new URL('../js/password-recovery.js', import.meta.url), 'utf8');
    vm.runInContext(source.replace(/^import .*;$/gm, '').replace(/^export /gm, ''), context);
    const drain = async () => { await Promise.all(pending.splice(0)); await new Promise(resolve => setImmediate(resolve)); };
    const click = id => field(id).handlers.click();
    const send = () => form.handlers.submit({preventDefault() {}});
    const fill = () => {
        field('password-reset-email').value = 'alice@example.com';
        field('password-reset-code').value = '12345678';
        field('password-reset-password').value = field('password-reset-confirmation').value = 'a valid password';
    };
    context.updatePasswordRecoveryPage(true, true);
    await context.refreshPasswordRecoveryAvailability();
    fill(); click('password-reset-send'); send(); await drain();
    assert.equal(requests.length, 1);
    assert.equal(links[0].hidden, true);
    enabled = true;
    await context.refreshPasswordRecoveryAvailability();
    assert.equal(field('password-reset-fields').disabled, false);
    assert.equal(links[0].hidden, false);
    field('password-reset-confirmation').value = 'different'; send(); await drain();
    assert.equal(field('password-reset-error').textContent, 'login.passwordsDoNotMatch');
    fill(); allowWeak = false; send(); await drain();
    assert.equal(requests.filter(request => request.url.endsWith('/confirm')).length, 0);
    allowWeak = true;
    click('password-reset-send'); click('password-reset-send'); await drain();
    assert.equal(requests.filter(request => request.url.endsWith('/request')).length, 1);
    assert.equal(field('password-reset-delivery').textContent, 'mail.status.queued');
    assert.equal(timers.size, 1);
    [...timers.values()][0](); await drain();
    assert.equal(timers.size, 1);
    failPoll = true; [...timers.values()][0](); await drain();
    assert.equal(timers.size, 0);
    assert.equal(field('password-reset-refresh').hidden, false);
    failPoll = false; status = 'failed'; click('password-reset-refresh'); await drain();
    assert.equal(field('password-reset-delivery').textContent, 'mail.status.failed');
    assert.equal(timers.size, 0);
    send(); send(); await drain();
    assert.equal(requests.filter(request => request.url.endsWith('/confirm')).length, 1);
    assert.equal(field('password-reset-error').textContent, 'login.emailCodeInvalid');
    assert.deepEqual(navigation, []);
    failConfirm = false; send(); await drain();
    assert.deepEqual(navigation, [['logout', 'silent'], ['login', '/account/reviews', true], ['success']]);
    assert.equal(field('username').value, 'alice');
    assert.equal(field('password-reset-code').value, '');
    assert.equal(field('password-reset-password').value, '');
    context.updatePasswordRecoveryPage(true, true); fill();
    delaySend = true; click('password-reset-send'); await new Promise(resolve => setImmediate(resolve));
    pagehide(); release(); await drain();
    assert.equal(field('password-reset-delivery').textContent, '');
    assert.equal(timers.size, 0);
    assert.equal(field('password-reset-email').value, '');
});
