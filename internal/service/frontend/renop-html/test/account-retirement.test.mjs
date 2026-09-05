/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 */

import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import test from 'node:test';

/** Load a browser module with explicit DOM and network substitutes for its interaction test. */
function loadModule(relativePath, exportName, dependencies = {}) {
    const source = readFileSync(new URL(relativePath, import.meta.url), 'utf8')
        .replace(/^import .*;\r?\n/gm, '')
        .replace(/^export /gm, '');
    return new Function(...Object.keys(dependencies), `${source}\nreturn ${exportName};`)(
        ...Object.values(dependencies));
}

test('retention actions survive asynchronous confirmation and do not run when cancelled', async () => {
    const runButtonAction = loadModule('../js/components/button.js', 'runButtonAction');
    const requests = [];
    const alerts = [];
    const closed = [];
    let options;
    let confirmed = true;
    let failed = false;
    const openDialog = loadModule('../js/users/retention.js', 'openAccountRetentionDialog', {
        apiRequest: async (url, request = {}) => {
            requests.push({url, method: request.method || 'GET'});
            if (failed) throw new Error('network failure');
            return {ok: true, json: async () => ({deleted_at: 1, email_release_at: 2, audit_purge_at: 3})};
        },
        showAlert: message => alerts.push(message),
        showConfirm: async () => confirmed,
        RenopDialog: {show: value => { options = value; }},
        runButtonAction,
        t: key => key,
        responseErrorMessage: async (_response, key) => key,
        formatTimestamp: value => String(value),
        el: (tag, props, ...children) => ({tag, props, children}),
    });
    await openDialog({name: 'alice'});
    const button = {disabled: false};
    const dialog = {close: result => closed.push(result)};
    for (const [index, resource] of [[0, 'email'], [1, 'audit']]) {
        const event = {currentTarget: button};
        const pending = options.footer[index].onClick(event, dialog);
        assert.equal(button.disabled, true);
        event.currentTarget = null;
        await pending;
        assert.equal(button.disabled, false);
        assert.deepEqual(requests.at(-1), {url: `/api/tokens/alice/retention/${resource}`, method: 'DELETE'});
    }
    assert.deepEqual(closed, [true, true]);
    confirmed = false;
    const previous = requests.length;
    await options.footer[0].onClick({currentTarget: button}, dialog);
    assert.equal(requests.length, previous);
    confirmed = true;
    failed = true;
    await options.footer[0].onClick({currentTarget: button}, dialog);
    assert.equal(alerts.at(-1), 'users.releaseEmailFailed');
    assert.equal(button.disabled, false);
});
