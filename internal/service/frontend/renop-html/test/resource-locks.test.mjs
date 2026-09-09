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

test('lock controls submit only a manual restriction and retain the dialog on failure', async () => {
    let dialog, closed = false, refreshed = 0, accepted = true;
    const selections = [], requests = [], alerts = [];
    const context = vm.createContext({
        el: (tag, attributes, ...children) => ({tag, ...attributes, children}),
        makeCustomSelect: (_options, current, change) => {
            const id = 'select-' + selections.length;
            selections.push({current, change});
            return {querySelector: () => ({id})};
        },
        RenopDialog: {show: options => { dialog = options; return Promise.resolve(); }},
        runButtonAction: (_button, action) => action(),
        t: key => key, showAlert: message => alerts.push(message),
        localizedResponseError: async () => new Error('safe'), caughtErrorMessage: () => 'safe',
    });
    vm.runInContext(readFileSync(new URL('../js/resource-locks.js', import.meta.url), 'utf8')
        .replace(/^import .*;\r?\n/gm, '').replaceAll('export ', ''), context);
    const system = {mode: 'write', source: 'system', reason: 'hold'};
    assert.equal(context.resourceWriteLocked({locks: [system]}, {}), true);
    assert.equal(context.resourceReadLocked({locks: [system]}, {locks: [{mode: 'read'}]}), true);
    const button = context.createResourceLockButton({
        locks: [system], name: 'demo', request: async (mode, reason) => {
            requests.push([mode, reason]); return {ok: accepted};
        }, onSuccess: () => { refreshed++; },
    });
    button.onclick();
    assert.equal(selections[0].current, '');
    selections[0].change('read');
    selections[1].change('trojan');
    await dialog.footer[1].onClick({currentTarget: {}}, {close: () => { closed = true; }});
    assert.deepEqual(requests, [['read', 'trojan']]);
    assert.equal(closed, true);
    assert.equal(refreshed, 1);
    accepted = false; closed = false;
    await dialog.footer[1].onClick({currentTarget: {}}, {close: () => { closed = true; }});
    assert.equal(closed, false);
    assert.equal(refreshed, 1);
    assert.equal(alerts.at(-1), 'safe');
});
