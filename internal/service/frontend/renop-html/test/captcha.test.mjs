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

test('CAPTCHA transport retries only a same-origin pre-mutation denial', async () => {
    const calls = [], events = new Map();
    let status = 428, code = 'captcha_required', challenges = 0;
    const context = vm.createContext({
        window: {addEventListener: (name, fn) => events.set(name, fn)},
        location: {origin: 'https://example.test', href: 'https://example.test/'},
        Headers, ReadableStream, URL,
        LocalizedResponseError: Error, t: key => key,
        fetch: async (url, options) => {
            calls.push({url, options});
            return new Response('', {status: calls.length === 1 ? status : 201, headers: {
                'X-Renop-Error-Code': code, 'X-Renop-Captcha-Scope': 'package_create'
            }});
        },
    });
    const source = readFileSync(new URL('../js/captcha.js', import.meta.url), 'utf8');
    vm.runInContext(source.replace(/^import .*;$/gm, '').replace(/^export /gm, '') +
        '; challenge = async () => { countChallenge(); return "verified-proof"; };', context);
    context.countChallenge = () => { challenges++; };
    let result = await context.captchaFetch('/create', {method: 'POST', body: 'payload', headers: {'Content-Type': 'application/json'}});
    assert.equal(result.status, 201);
    assert.equal(calls.length, 2);
    assert.equal(challenges, 1);
    assert.equal(calls[1].options.headers.get('X-Renop-Captcha'), 'verified-proof');
    assert.equal(calls[1].options.body, 'payload');
    for (const next of [400, 401, 403, 500, 503]) {
        status = next; calls.length = 0;
        result = await context.captchaFetch('/create', {method: 'POST', body: 'payload'});
        assert.equal(result.status, next);
        assert.equal(calls.length, 1);
    }
    status = 428; calls.length = 0;
    await context.captchaFetch('https://other.test/create', {method: 'POST'});
    assert.equal(calls.length, 1);
    code = 'legal_consent_required'; calls.length = 0;
    await context.captchaFetch('/create', {method: 'POST'});
    assert.equal(calls.length, 1);
    assert.equal(challenges, 1);
    code = 'captcha_required'; calls.length = 0;
    for (const navigation of ['popstate', 'appNavigation']) {
        calls.length = 0;
        context.countChallenge = () => events.get(navigation)();
        await assert.rejects(context.captchaFetch('/create', {method: 'POST'}), /captcha.cancelled/);
        assert.equal(calls.length, 1);
    }
});


test('widget adapters initialize once, use compact controls, and return nonce-bound completions', async () => {
    const source = readFileSync(new URL('../js/captcha-widget.js', import.meta.url), 'utf8');
    for (const provider of ['recaptcha_v2', 'recaptcha_invisible', 'recaptcha_v3', 'turnstile', 'hcaptcha', 'friendlycaptcha']) {
        let onMessage, scriptCount = 0, rendered;
        const messages = [], handlers = {};
        const widget = {dataset: {}, addEventListener: (event, fn) => { handlers[event] = fn; }};
        const parent = {postMessage: value => messages.push(value)};
        const window = {innerWidth: 300, addEventListener: (_, fn) => { onMessage = fn; }};
        const complete = () => rendered.callback('provider-response');
        window.hcaptcha = window.turnstile = {render: (_, options) => { rendered = options; complete(); }};
        window.grecaptcha = {
            render: (_, options) => { rendered = options; if (options.size !== 'invisible') complete(); return 3; },
            ready: fn => fn(),
            execute: (key, options) => { if (options) return Promise.resolve('provider-response'); complete(); },
        };
        const context = vm.createContext({window, parent, location: {origin: 'https://example.test'},
            URLSearchParams, encodeURIComponent, setTimeout, clearTimeout,
            MutationObserver: class { observe() {} },
            document: {documentElement: {}, body: {style: {}}, getElementById: () => widget,
                createElement: () => ({}), head: {appendChild: script => {
                    scriptCount++;
                    if (script.type === 'module') { script.onload(); handlers['frc:widget.complete']({detail: {response: 'provider-response'}}); }
                    else window.renopCaptchaReady();
                }}},
        });
        vm.runInContext(source, context);
        const event = {source: parent, origin: 'https://example.test', data: {kind: 'renop-captcha-init',
            provider, nonce: 'nonce', site_key: 'site', action: 'renop_registration', language: 'en-US', theme: 'light'}};
        await onMessage({...event, origin: 'https://other.test'});
        assert.equal(scriptCount, 0);
        await onMessage(event);
        await onMessage(event);
        assert.equal(scriptCount, 1, provider);
        assert.equal(messages.length, 1, provider);
        assert.equal(messages[0].kind, 'renop-captcha-complete');
        assert.equal(messages[0].response, 'provider-response');
        assert.equal(messages[0].nonce, 'nonce');
        if (rendered) assert.equal(rendered.size, provider === 'recaptcha_invisible' ? 'invisible' : 'compact');
    }
});
