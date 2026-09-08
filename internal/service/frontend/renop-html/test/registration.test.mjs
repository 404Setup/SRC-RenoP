/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import test from 'node:test';
import vm from 'node:vm';

test('registration gates confirmation, verifies email, imports optional profile, and clears private state', async () => {
    const elements = new Map(), timers = new Map(), requests = [], navigation = [], links = [{}], actions = [];
    let timerID = 0, pagehide, enabled = false, mail = false, pending, delayPending = false, release;
    const field = id => {
        if (!elements.has(id)) elements.set(id, {value: '', checked: false, textContent: '', handlers: {},
            focus() {}, reportValidity() { return this.value.includes('@'); },
            addEventListener(name, fn) { this.handlers[name] = fn; }, closest() { return field(id + '-wrapper'); }});
        return elements.get(id);
    };
    const form = field('registration-form');
    form.querySelector = () => field('submit');
    form.reset = () => { for (const element of elements.values()) { element.value = ''; element.checked = false; } };
    class LocalizedResponseError extends Error {
        constructor(message, status) { super(message); this.status = status; }
    }
    const context = vm.createContext({
        document: {getElementById: field, querySelectorAll: () => links},
        window: {location: {search: '', assign: value => navigation.push(value)}, addEventListener: (_, fn) => { pagehide = fn; }},
        URLSearchParams, AbortSignal: {timeout: () => undefined},
        setTimeout: (fn, delay) => { timers.set(++timerID, {fn, delay}); return timerID; },
        clearTimeout: id => { timers.delete(id); },
        t: key => key, LocalizedResponseError, responseErrorMessage: async () => 'registration.invalid',
        mailStatusLabel: status => 'mail.' + status, attachPasswordStrength: () => ({reset() {}}),
        getPasswordLengthError: value => value.length < 6 ? 'short' : '', confirmWeakPasswordIfNeeded: async () => true,
        loginReturnTo: () => '/packages', navigateToLogin: path => navigation.push(path), showAlert() {},
        fetch: async (url, options) => {
            requests.push({url, options});
            assert.equal(options.credentials, 'include');
            assert.equal(options.cache, 'no-store');
            let body;
            if (url.endsWith('/status')) body = {enabled, email_required: mail};
            else if (url.endsWith('/pending')) {
                if (delayPending) await new Promise(resolve => { release = resolve; });
                if (!pending) return {ok: false, status: 400};
                body = pending;
            } else if (url.endsWith('/code')) body = {id: 'job', ticket: 'private-ticket', status: 'queued'};
            else if (url.endsWith('/mail/job')) {
                assert.equal(options.headers['X-Renop-Mail-Ticket'], 'private-ticket');
                body = {status: 'failed'};
            } else { assert.equal(url, '/api/auth/registration'); body = {username: 'alice'}; }
            return {ok: true, json: async () => body};
        },
    });
    const buttonSource = readFileSync(new URL('../js/components/button.js', import.meta.url), 'utf8');
    const action = vm.runInContext(buttonSource.slice(buttonSource.indexOf('export async function runButtonAction')).replace('export ', '') + '; runButtonAction', context);
    context.runButtonAction = (button, fn) => { const promise = action(button, fn); actions.push(promise); return promise; };
    const source = readFileSync(new URL('../js/registration.js', import.meta.url), 'utf8');
    vm.runInContext(source.replace(/^import .*;$/gm, '').replace(/^export /gm, ''), context);
    const drain = async () => { await Promise.all(actions.splice(0)); await new Promise(resolve => setImmediate(resolve)); };
    const submit = () => form.handlers.submit({preventDefault() {}});
    const fill = () => {
        field('registration-username').value = 'alice'; field('registration-email').value = 'alice@example.com';
        field('registration-password').value = field('registration-confirmation').value = 'A long password!';
    };
    const submissions = () => requests.filter(request => request.url === '/api/auth/registration');
    context.updateRegistrationPage(true, true); await drain(); fill(); submit(); await drain();
    assert.equal(submissions().length, 0);
    assert.equal(links[0].hidden, true);
    enabled = true; await context.refreshRegistrationAvailability();
    assert.equal(field('registration-code').required, false);
    assert.equal(field('registration-email').required, false);
    field('registration-confirmation').value = 'mismatch'; submit(); await drain();
    assert.equal(field('registration-error').textContent, 'login.passwordsDoNotMatch');
    mail = true; await context.refreshRegistrationAvailability();
    assert.equal(field('registration-code').required, true);
    fill(); field('registration-send').handlers.click(); field('registration-send').handlers.click(); await drain();
    assert.equal(requests.filter(request => request.url.endsWith('/code')).length, 1);
    assert.equal(field('registration-delivery').textContent, 'mail.queued');
    [...timers.values()][0].fn(); await drain();
    assert.equal(field('registration-delivery').textContent, 'mail.failed');
    assert.equal(timers.size, 0);
    field('registration-code').value = '12345678'; submit(); submit(); await drain();
    assert.equal(submissions().length, 1);
    assert.equal(JSON.parse(submissions()[0].options.body).code, '12345678');
    assert.deepEqual(navigation, ['/packages']);
    assert.equal(field('registration-password').value, '');
    assert.equal(field('registration-code').value, '');

    pending = {provider: 'github', email: 'verified@example.com', expires_at: Date.now() + 600000,
        username: 'occupied', username_available: false, nickname: 'Provider Name'};
    context.window.location.search = '?provider=github'; delayPending = true;
    context.updateRegistrationPage(true, true); fill(); submit(); await drain();
    assert.equal(field('registration-fields').disabled, true);
    assert.equal(submissions().length, 1);
    release(); await drain(); delayPending = false;
    assert.equal(field('registration-email').readOnly, true);
    assert.equal(field('registration-code').required, false);
    assert.equal(field('registration-error').textContent, 'registration.chooseUsername');
    field('registration-import').checked = true; field('registration-import').handlers.change();
    assert.equal(field('registration-username').value, '');
    assert.equal(field('registration-nickname').value, 'Provider Name');
    field('registration-username').value = 'alice'; submit(); await drain();
    const body = JSON.parse(submissions()[1].options.body);
    assert.equal(body.provider, 'github');
    assert.equal(body.import_avatar, true);
    assert.equal(body.email, 'verified@example.com');
    assert.equal(timers.size, 0);
    pending = {provider: 'stackexchange', provider_name: 'Stack Exchange', email_required: true,
        email: '', expires_at: Date.now() + 600000, username_available: true, avatar_available: false};
    context.window.location.search = '?provider=stackexchange';
    mail = false; context.updateRegistrationPage(true, true); await drain();
    assert.equal(field('registration-fields').disabled, true);
    assert.equal(field('registration-availability').textContent, 'oauth.mailRequired');
    mail = true; await context.refreshRegistrationAvailability();
    assert.equal(field('registration-fields').disabled, false);
    assert.equal(field('registration-email').readOnly, false);
    assert.equal(field('registration-code').required, true);
    fill(); field('registration-send').handlers.click(); await drain();
    assert.equal(JSON.parse(requests.filter(request => request.url.endsWith('/code')).at(-1).options.body).provider, 'stackexchange');
    field('registration-code').value = '12345678'; field('registration-import').checked = true;
    submit(); await drain();
    assert.equal(JSON.parse(submissions()[2].options.body).import_avatar, false);
    assert.equal(JSON.parse(submissions()[2].options.body).provider, 'stackexchange');
    pending = undefined; context.updateRegistrationPage(true, true); await drain();
    assert.equal(field('registration-fields').disabled, true);
    assert.equal(field('registration-availability').textContent, 'registration.expired');
    fill(); pagehide();
    assert.equal(field('registration-password').value, '');
    assert.equal(timers.size, 0);
});
