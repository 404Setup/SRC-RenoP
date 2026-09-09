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

test('private email aliases add through confirmation and remove without replacing the primary', async () => {
    const nodes = new Map(), requests = [], updates = [];
    const node = (tag, props = {}, ...children) => {
        const result = {
            tag, ...props, children, value: '', disabled: false, dataset: {}, handlers: {},
            replaceChildren(...next) {
                this.children = next;
            },
            addEventListener(type, fn) {
                this.handlers[type] = fn;
            },
            reportValidity() {
                return this.value.includes('@');
            },
            querySelector() {
                return nodes.get('submit');
            },
        };
        if (props.id) nodes.set(props.id, result);
        return result;
    };
    const list = node('ul', {id: 'profile-email-aliases'});
    const form = node('form', {id: 'profile-email-alias-form'});
    const input = node('input', {id: 'profile-email-alias'});
    node('button', {id: 'submit'});
    node('p', {id: 'profile-email-alias-mail-disabled'});
    const next = {email: 'primary@example.com', email_aliases: ['new@example.com'], email_verification_required: true};
    const context = vm.createContext({
        document: {getElementById: id => nodes.get(id)}, window: {location: {pathname: '/user/alice/edit'}},
        el: node, t: key => key, showAlert() {
        }, responseErrorMessage: async () => 'error',
        runButtonAction: async (button, action) => {
            if (!button.disabled) await action();
        },
        apiRequest: async (path, options) => {
            requests.push({path, ...options});
            return {
                ok: true,
                json: async () => options.method === 'PUT' ? {id: 'job', ticket: 'private-ticket'} : next
            };
        },
        verifyProfileEmail: async (email, receipt) => {
            assert.equal(email, 'new@example.com');
            assert.equal(receipt.ticket, 'private-ticket');
            return next;
        },
    });
    vm.runInContext(readFileSync(new URL('../js/account-emails.js', import.meta.url), 'utf8')
        .replace(/^import .*;\r?\n/gm, '').replaceAll('export ', ''), context);
    const update = security => updates.push(security);
    context.renderAccountEmailAliases({
        email_aliases: ['old@example.com'],
        email_verification_required: true
    }, update, assert.fail);
    assert.equal(list.children[0].children[0].children[0], 'old@example.com');
    list.children[0].children[1].handlers.click();
    await new Promise(resolve => setImmediate(resolve));
    assert.equal(requests[0].method, 'DELETE');
    assert.equal(JSON.parse(requests[0].body).email, 'old@example.com');
    input.value = 'new@example.com';
    form.handlers.submit({
        preventDefault() {
        }
    });
    await new Promise(resolve => setImmediate(resolve));
    assert.deepEqual(JSON.parse(requests.at(-1).body), {email: 'new@example.com', alias: true});
    assert.equal(updates.at(-1).email, 'primary@example.com');
    assert.deepEqual(updates.at(-1).email_aliases, ['new@example.com']);
    assert.equal(input.value, '');
    context.renderAccountEmailAliases({email_aliases: [], email_verification_required: false}, update, assert.fail);
    assert.equal(input.disabled, true);
    assert.equal(nodes.get('submit').disabled, true);
    assert.equal(nodes.get('profile-email-alias-mail-disabled').hidden, false);
});
