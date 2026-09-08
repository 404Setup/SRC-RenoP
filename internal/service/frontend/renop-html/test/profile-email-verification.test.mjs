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

test('email verification preserves rejected input state and discards responses after closing', async () => {
    const elements = new Map(), timers = new Map(), listeners = new Map();
    let options, accept = false, delayed = false, release, timerID = 0, closed = 0;
    const context = vm.createContext({
        AbortController, Date,
        document: {getElementById: id => elements.get(id)},
        window: {location: {pathname: '/user/alice/edit'},
            addEventListener: (name, fn) => listeners.set(name, fn), removeEventListener: name => listeners.delete(name)},
        el: (tag, props, ...children) => {
            const node = {tag, ...props, children, value: '', handlers: {}, textContent: typeof children[0] === 'string' ? children[0] : '',
                addEventListener(name, fn) { this.handlers[name] = fn; }, focus() {}};
            if (node.id) elements.set(node.id, node);
            return node;
        },
        t: key => key,
        mailStatusLabel: status => 'mail.' + status,
        responseErrorMessage: async () => 'invalid-code',
        setTimeout: fn => { timers.set(++timerID, fn); return timerID; }, clearTimeout: id => timers.delete(id),
        apiRequest: async (path, request) => {
            assert.ok(request.signal instanceof AbortSignal);
            if (path.includes('/mail/')) {
                assert.equal(request.headers['X-Renop-Mail-Ticket'], 'ticket');
                return {ok: true, json: async () => ({status: 'failed'})};
            }
            assert.equal(path, '/api/auth/profile/email/confirm');
            assert.deepEqual(JSON.parse(request.body), {email: 'new@example.com', code: '12345678'});
            if (delayed) await new Promise(resolve => { release = resolve; });
            return {ok: accept, status: accept ? 200 : 400, json: async () => ({email: 'new@example.com'})};
        },
        RenopDialog: {show: current => {
            options = current;
            return new Promise(resolve => {
                elements.set(current.id, {close: result => {
                    closed++; current.onClose(); elements.delete(current.id); resolve(result);
                }});
                for (const button of current.footer) if (button.id) elements.set(button.id, {disabled: false});
            });
        }},
    });
    const button = readFileSync(new URL('../js/components/button.js', import.meta.url), 'utf8');
    context.runButtonAction = vm.runInContext(button.slice(button.indexOf('export async function runButtonAction')).replace('export ', '') + '; runButtonAction', context);
    const source = readFileSync(new URL('../js/profile-email-verification.js', import.meta.url), 'utf8').replace(/^import .*;\r?\n/gm, '').replace('export async function', 'async function');
    vm.runInContext(source, context);
    const settle = async () => { for (let index = 0; index < 8; index++) await new Promise(resolve => setImmediate(resolve)); };
    const receipt = {id: 'job', status: 'queued', ticket: 'ticket'};
    const first = context.verifyProfileEmail('new@example.com', receipt);
    const code = elements.get('profile-email-code');
    code.value = '12345678';
    options.body.handlers.submit({preventDefault() {}});
    await settle();
    assert.equal(closed, 0);
    assert.equal(code.value, '');
    assert.equal(options.body.children.find(node => node.role === 'alert').textContent, 'invalid-code');
    await [...timers.values()][0]();
    await settle();
    assert.equal(options.body.children.find(node => node.role === 'status').textContent, 'mail.failed');
    assert.equal(timers.size, 0);
    accept = true;
    code.value = '12345678';
    options.body.handlers.submit({preventDefault() {}});
    assert.equal((await first).email, 'new@example.com');
    assert.equal(receipt.ticket, '');
    assert.equal(listeners.size, 0);

    delayed = true;
    const second = context.verifyProfileEmail('new@example.com', {id: 'job', status: 'queued', ticket: 'ticket'});
    elements.get('profile-email-code').value = '12345678';
    options.body.handlers.submit({preventDefault() {}});
    await settle();
    listeners.get('pagehide')();
    assert.equal(await second, null);
    release();
    await settle();
    assert.equal(closed, 2);
    assert.equal(timers.size, 0);
    assert.equal(elements.has('profile-email-verification-dialog'), false);
});
