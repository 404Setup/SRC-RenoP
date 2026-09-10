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
import test from 'node:test';
import {readFileSync} from 'node:fs';
import vm from 'node:vm';
import {readLegalTextResponse} from '../js/legal-response.js';
import {parseCookiePreferences} from '../js/cookie-preferences.js';

test('optional verification requires an explicit current unexpired cookie choice', () => {
    const choice = {revision: 'current', optional: false, expires: 200};
    assert.equal(parseCookiePreferences(JSON.stringify(choice), 'current', 100).optional, false);
    assert.equal(parseCookiePreferences(JSON.stringify({...choice, optional: true}), 'current', 100).optional, true);
    assert.equal(parseCookiePreferences(JSON.stringify(choice), 'new', 100), null);
    assert.equal(parseCookiePreferences(JSON.stringify(choice), 'current', 200), null);
    for (const raw of [null, '', 'invalid', '{}', JSON.stringify({...choice, optional: 'true'}), ' '.repeat(4097)]) {
        assert.equal(parseCookiePreferences(raw, 'current', 100), null);
    }
});

test('account entry requires explicit current consent and cancels checks after navigation', async () => {
    let revision = 'a'.repeat(64), release;
    const input = {checked: false, dataset: {}, focus() {}, reportValidity() {}};
    const error = {dataset: {}, hidden: true};
    const document = {
        cookie: '', querySelectorAll: () => [input],
        getElementById: id => id.endsWith('-error') ? error : input,
    };
    const context = vm.createContext({
        document, AbortSignal, CustomEvent, readLegalTextResponse,
        window: {dispatchEvent() {}}, t: key => key,
        fetch: async () => {
            if (release) await new Promise(resolve => { release = resolve; });
            return new Response(JSON.stringify({revision, cookie_banner: true}), {headers: {'Content-Type': 'application/json'}});
        },
    });
    const source = readFileSync(new URL('../js/legal-consent.js', import.meta.url), 'utf8');
    vm.runInContext(source.replace(/^import .*;$/gm, '').replace(/^export /gm, ''), context);
    assert.equal(await context.ensureLegalConsent(), false);
    input.checked = true;
    document.cookie = 'renop_legal_consent=' + revision;
    assert.equal(await context.ensureLegalConsent(), true);
    revision = 'b'.repeat(64);
    assert.equal(await context.ensureLegalConsent(), false);
    assert.equal(input.checked, false);
    input.checked = true;
    document.cookie = 'renop_legal_consent=' + revision;
    release = true;
    const pending = context.ensureLegalConsent();
    context.resetLegalConsent('login');
    release();
    assert.equal(await pending, false);
});
