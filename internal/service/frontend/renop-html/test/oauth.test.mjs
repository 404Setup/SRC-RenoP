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

test('OAuth controls preserve intent, enforce last-login state, and discard private results after logout', async () => {
    const elements = new Map(), events = {}, destinations = [], alerts = [];
    const node = (tag, attributes = {}, ...children) => ({tag, ...attributes, children, handlers: {},
        appendChild(child) { this.children.push(child); }, replaceChildren(...next) { this.children = next; },
        addEventListener(name, fn) { this.handlers[name] = fn; }});
    const element = id => { if (!elements.has(id)) elements.set(id, node('div')); return elements.get(id); };
    const text = value => typeof value === 'string' ? value : value.children.map(text).join('');
    const buttons = value => [value, ...value.children.flatMap(child => typeof child === 'string' ? [] : buttons(child))].filter(child => child.tag === 'button');
    const translate = (key, args = {}) => key + Object.values(args).join('');
    let release, delay = false;
    const publicChoices = [{id: 'demo', name: '<Example>'}];
    const context = vm.createContext({el: node, t: translate, URL, URLSearchParams, AbortSignal,
        document: {getElementById: element}, createIcon: () => node('icon'),
        window: {location: {href: 'https://renop.example/account/login?oauth=success&provider=demo', pathname: '/user/alice'},
            history: {replaceState: (_state, _title, path) => destinations.push(path)},
            addEventListener: (name, fn) => { events[name] = fn; }, showConfirm: async () => true},
        loginReturnTo: () => '/packages', showAlert: (...args) => alerts.push(args),
        refreshAccountSecurity: async () => {}, runButtonAction: (_button, fn) => fn(),
        LocalizedResponseError: Error, responseErrorMessage: async () => 'oauth.failed',
        fetch: async () => ({ok: true, json: async () => ({providers: publicChoices})}),
        apiRequest: async () => {
            if (delay) await new Promise(resolve => { release = resolve; });
            return {ok: true, json: async () => ({providers: [{id: 'demo', name: '<Example>', login: '<Alice>',
                configured: true, linked: true, can_disconnect: false, can_verify_email: true, can_import_avatar: true}]})};
        },
    });
    context.window.location.assign = value => destinations.push(value);
    const source = readFileSync(new URL('../js/oauth.js', import.meta.url), 'utf8');
    vm.runInContext(source.replace(/^import .*;$/gm, '').replace(/^export /gm, ''), context);
    await context.initializeOAuth();
    assert.equal(destinations[0], '/account/login');
    assert.equal(alerts[0][0], 'oauth.success');
    for (const [id, intent] of [['oauth-login-providers', 'login'], ['oauth-register-providers', 'register']]) {
        const button = buttons(element(id))[0];
        assert.equal(text(button), 'oauth.continue<Example>');
        button.onclick();
        const url = new URL(destinations.at(-1), 'https://renop.example');
        assert.equal(url.pathname, '/api/auth/oauth/demo/start');
        assert.equal(url.searchParams.get('intent'), intent);
        assert.equal(url.searchParams.get('return_to'), '/packages');
    }
    publicChoices.push({id: 'new', name: 'New provider'});
    await events.oauthProvidersChanged();
    assert.equal(buttons(element('oauth-login-providers')).length, 2);
    assert.equal(buttons(element('oauth-register-providers')).length, 2);
    await context.refreshOAuthProfile('alice');
    assert.ok(text(element('profile-oauth-providers')).includes('<Alice>'));
    assert.ok(!buttons(element('profile-oauth-providers')).some(button => text(button) === 'oauth.disconnect'));
    events.accountSecurityUpdated({detail: {password_configured: true, password_login_enabled: true}});
    assert.ok(buttons(element('profile-oauth-providers')).some(button => text(button) === 'oauth.disconnect'));
    const email = buttons(element('profile-oauth-providers')).find(button => text(button) === 'oauth.useEmail');
    email.onclick();
    assert.equal(new URL(destinations.at(-1), 'https://renop.example').searchParams.get('intent'), 'email');
    delay = true;
    const loading = context.refreshOAuthProfile('alice');
    events.authChanged({detail: {isLoggedIn: false}});
    release(); await loading;
    assert.equal(element('profile-oauth-providers').hidden, true);
    assert.equal(element('profile-oauth-providers').children.length, 0);
});

test('session labels retain OAuth provider and second-factor method', () => {
    const source = readFileSync(new URL('../js/sessions.js', import.meta.url), 'utf8');
    const declaration = source.match(/^function formatLoginMethod\([\s\S]*?^}/m)[0];
    const format = vm.runInNewContext(declaration + '; formatLoginMethod', {t: (key, args = {}) => key + (args.provider || '')});
    assert.equal(format('oauth:entra+totp'), 'oauth.sessionMethodentra + mfa.authenticator');
    assert.equal(format('github+passkey'), 'sessions.methodGithub + sessions.methodFido');
    assert.equal(format('password'), 'sessions.methodPassword');
});
