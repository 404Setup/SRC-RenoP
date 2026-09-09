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

test('ticket details use server actions and refresh only after successful claims', async () => {
    const requests = [], alerts = [];
    let accepted = false, refreshed = 0;
    const context = vm.createContext({
        el: (tag, attributes, ...children) => ({tag, ...attributes, children: children.filter(Boolean),
            append(...items) { this.children.push(...items); }, appendChild(item) { this.children.push(item); }}),
        createIcon: name => ({icon: name}), t: key => key, formatTimestamp: () => 'now',
        runButtonAction: (_button, action) => action(), showConfirm: async () => true,
        showAlert: value => alerts.push(value), caughtErrorMessage: () => 'safe error',
        apiRequest: async (url, options, policy) => {
            requests.push({url, options, policy}); return {ok: accepted};
        },
        localizedResponseError: async () => new Error('safe error'), REVIEW_ERROR_KEYS: {},
    });
    const source = readFileSync(new URL('../js/tickets.js', import.meta.url), 'utf8');
    vm.runInContext(source.replace(/^import .*;\r?\n/gm, '').replaceAll('export ', '') +
        '\nloadTasks = async () => {};', context);
    const buttons = node => [node, ...node.children?.flatMap(buttons) || []].filter(node => node.tag === 'button');
    const ticket = {id: 'one', kind: 'report', resource_type: 'npm', resource_name: 'demo',
        status: 'pending', ticket_status: 'unprocessed', title: '<script>bad()</script>', body: '<img onerror=bad()>', actions: ['claim']};
    const refresh = () => { refreshed++; };
    const list = context.taskCard(ticket);
    assert.deepEqual(buttons(list).map(button => button.children[0]), ['ticket.open']);
    const detail = context.taskCard(ticket, refresh);
    const claim = buttons(detail)[0];
    assert.equal(claim.children[0], 'ticket.action.claim');
    claim.onclick({currentTarget: claim});
    await new Promise(resolve => setImmediate(resolve));
    assert.equal(refreshed, 0);
    assert.deepEqual(alerts, ['safe error']);
    accepted = true;
    claim.onclick({currentTarget: claim});
    await new Promise(resolve => setImmediate(resolve));
    assert.equal(refreshed, 1);
    assert.equal(requests[1].url, '/api/tickets/one/action');
    assert.deepEqual(JSON.parse(requests[1].options.body), {action: 'claim', force: false});
    assert.equal(requests[1].policy.logoutOnForbidden, false);
    const occupied = context.taskCard({...ticket, actions: []}, refresh);
    assert.equal(buttons(occupied).length, 0);
    const final = context.taskCard({...ticket, escalations: 3, actions: ['process', 'complete', 'close']}, refresh);
    assert.deepEqual(buttons(final).map(button => button.children[0]),
        ['ticket.action.process', 'ticket.action.complete', 'ticket.action.close']);
    assert.equal(context.ticketRouteFromPath('/account/reviews/'), true);
    assert.equal(context.ticketRouteFromPath('/account/tickets'), true);
    assert.equal(context.ticketRouteFromPath('/account/tickets/unrelated'), false);
});
