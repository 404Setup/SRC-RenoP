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

const flush = () => new Promise(resolve => setImmediate(resolve));

/** Run the real synchronizer against controllable request and language events. */
function languageRuntime(storage = new Map()) {
    const events = new Map(), requests = [], applied = [], errors = [];
    let current = 'en-US', revision = 0, username = '';
    const emit = (type, detail) => events.get(type)?.({detail});
    const context = vm.createContext({
        AbortController, AbortSignal,
        localStorage: {getItem: key => storage.get(key) ?? null, setItem: (key, value) => storage.set(key, value), removeItem: key => storage.delete(key)},
        window: {addEventListener: (type, handler) => events.set(type, handler)},
        document: {visibilityState: 'visible', addEventListener: (type, handler) => events.set(type, handler)},
        getAvailableLanguages: () => ['en-US', 'fr-FR', 'ja-JP', 'de-DE'],
        getLanguage: () => ({current, revision}),
        setLanguage: async (language, source, signal) => {
            if (signal?.aborted) return current;
            current = language; revision++; applied.push(language);
            emit('languageChanged', {lang: language, source});
            return current;
        },
        t: key => key, showAlert: message => errors.push(message),
        apiRequest: (path, options = {}) => {
            const userID = 'id-' + username;
            const account = username;
            return new Promise(resolve => requests.push({path, ...options,
                complete: (locale, ok = true, identity = userID, name = account) => resolve({ok, json: async () => ({locale, user_id: identity, username: name})})}));
        },
    });
    vm.runInContext(readFileSync(new URL('../js/account-language.js', import.meta.url), 'utf8')
        .replace(/^import .*;\r?\n/gm, '').replaceAll('export ', ''), context);
    context.installAccountLanguageSync();
    return {requests, applied, errors, storage, emit,
        auth: name => { username = name; emit('authChanged', {isLoggedIn: !!name, username: name}); },
        choose: language => { current = language; revision++; emit('languageChanged', {lang: language, source: 'user-set'}); },
        current: () => current};
}

test('account language restores without an echo and serializes rapid selections', async () => {
    const runtime = languageRuntime();
    runtime.auth('alice');
    runtime.requests[0].complete('fr-FR');
    await flush();
    assert.equal(runtime.current(), 'fr-FR');
    assert.equal(runtime.requests.length, 1);
    runtime.choose('ja-JP');
    runtime.choose('de-DE');
    runtime.choose('en-US');
    assert.equal(runtime.requests.length, 2);
    runtime.requests[1].complete('ja-JP');
    await flush();
    assert.equal(JSON.parse(runtime.requests[2].body).locale, 'en-US');
    runtime.requests[2].complete('en-US');
    await flush();
    assert.equal(runtime.storage.size, 0);
    runtime.auth('bobby');
    runtime.auth('charlie');
    assert.equal(runtime.requests[3].signal.aborted, true);
    runtime.requests[3].complete('ja-JP');
    runtime.requests[4].complete('de-DE');
    await flush();
    assert.equal(runtime.current(), 'de-DE');
    assert.deepEqual(runtime.applied, ['fr-FR', 'de-DE']);
    assert.equal(runtime.requests.length, 5);
    runtime.auth('alice');
    runtime.requests[5].complete('ja-JP', true, 'id-bobby', 'bobby');
    await flush();
    assert.equal(runtime.current(), 'de-DE');
    assert.equal(runtime.requests.length, 6);
    assert.deepEqual(runtime.errors, ['profile.languageLoadFailed']);
});

test('new choices beat pending hydration and failed saves survive reloads', async () => {
    const runtime = languageRuntime();
    runtime.auth('alice');
    runtime.choose('ja-JP');
    runtime.requests[0].complete('fr-FR');
    await flush();
    assert.equal(runtime.current(), 'ja-JP');
    assert.equal(JSON.parse(runtime.requests[1].body).locale, 'ja-JP');
    runtime.requests[1].complete('', false);
    await flush();
    assert.deepEqual(runtime.errors, ['profile.languageSaveFailed']);
    assert.equal(runtime.storage.size, 1);
    const reclaimed = languageRuntime(new Map(runtime.storage));
    reclaimed.auth('alice');
    reclaimed.requests[0].complete('de-DE', true, 'reclaimed-alice');
    await flush();
    assert.equal(reclaimed.current(), 'de-DE');
    assert.equal(reclaimed.requests.length, 1);
    const restored = languageRuntime(runtime.storage);
    restored.auth('alice');
    restored.requests[0].complete('fr-FR');
    await flush();
    assert.equal(restored.current(), 'ja-JP');
    assert.equal(JSON.parse(restored.requests[1].body).locale, 'ja-JP');
    restored.requests[1].complete('ja-JP');
    await flush();
    assert.equal(restored.storage.size, 0);
    restored.auth('bobby');
    restored.requests[2].complete('');
    await flush();
    assert.equal(JSON.parse(restored.requests[3].body).locale, 'ja-JP');
});

test('cancelled catalog loading cannot apply the previous account language', async () => {
    const source = readFileSync(new URL('../js/i18n.js', import.meta.url), 'utf8');
    const change = source.match(/^export async function setLanguage\([\s\S]*?^}/m)[0];
    let finish;
    const events = [];
    const context = vm.createContext({
        currentLang: 'en-US', currentSource: 'default', languageRequestID: 0,
        DEFAULT_LANG: 'en-US', STORAGE_KEY: 'language',
        resolveLanguage: value => value,
        getAvailableLanguages: () => ['en-US', 'fr-FR'],
        setLanguageLoading() {}, ensureLanguage: () => new Promise(resolve => { finish = resolve; }),
        localStorage: {setItem() { assert.fail('cancelled language was persisted'); }},
        document: {getElementById: () => null}, updatePageTranslations() {},
        window: {dispatchEvent: event => events.push(event)}, console,
    });
    vm.runInContext(change.replace('export ', ''), context);
    const controller = new AbortController();
    const pending = context.setLanguage('fr-FR', 'account', controller.signal);
    controller.abort();
    finish();
    assert.equal(await pending, 'en-US');
    assert.deepEqual(events, []);
});
