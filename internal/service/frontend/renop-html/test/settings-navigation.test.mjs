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

test('settings pages preserve drafts and isolate loads, saves, credentials, and account changes', async () => {
    const requests = [], alerts = [], rendered = [], listeners = new Map();
    let delayedLoad, delayedSave, rejectSave = false, invalid = null;
    const element = {
        replaceChildren() {
        }, setAttribute() {
        }, addEventListener() {
        }
    };
    const response = data => ({response: {ok: true, status: 200}, data});
    const context = vm.createContext({
        structuredClone,
        Event,
        URLSearchParams,
        document: {getElementById: () => element, querySelector: () => invalid, querySelectorAll: () => []},
        window: {
            addEventListener: (name, callback) => listeners.set(name, callback), dispatchEvent() {
            }
        },
        t: key => key,
        showAlert: text => alerts.push(text),
        showConfirm: async () => true,
        createSkeleton: () => ({}),
        exitProtectedRouteOnDenial: () => false,
        responseErrorMessage: async () => 'safe error',
        caughtErrorMessage: () => 'safe error',
        LocalizedResponseError: Error,
        renderLegalSettings() {},
        renderCacheSettings() {
        },
        renderRegistrationSettings() {
        },
        renderOAuthSettings() {
        },
        renderMailSettings() {
        },
        renderMavenDomainSettings() {
        },
        FrontendConfig: {},
        ServerConfig: {},
        StorageConfig: {},
        ProxyConfig: {},
        UpdaterConfig: {},
        IndexDomainSettings: {},
        fetchProto: async url => {
            requests.push({url, method: 'GET'});
            if (url.endsWith('/server') && delayedLoad) return delayedLoad;
            return response({title: 'Saved', channel: 'stable'});
        },
        putProto: async (url, _type, data) => {
            requests.push({url, method: 'PUT', data});
            return response(null);
        },
        apiRequest: async (url, options = {}, policy) => {
            requests.push({
                url,
                method: options.method || 'GET',
                data: options.body && JSON.parse(options.body),
                policy
            });
            if (options.method === 'PUT') {
                if (delayedSave) return delayedSave;
                if (rejectSave) return {ok: false, status: 422};
                return {ok: true, status: 200, json: async () => ({mode: 'redis', password_configured: true})};
            }
            return {ok: true, status: 200, json: async () => ({mode: 'memory', password: '', clear_password: false})};
        },
    });
    const source = readFileSync(new URL('../js/settings.js', import.meta.url), 'utf8')
        .replace(/^import [\s\S]*? from '[^']+';\r?\n/gm, '').replaceAll('export ', '');
    vm.runInContext(source, context);
    context.rendered = rendered;
    vm.runInContext("renderDomainNavigation = () => {}; enableSave = () => {}; renderSettingsForm = (domain, data) => rendered.push({domain, data}); domainsList = ['frontend', 'server', 'cache'];", context);
    await context.loadDomainSettings('frontend');
    vm.runInContext("currentConfig.title = 'Draft';", context);
    await context.loadDomainSettings('cache');
    assert.equal(requests.filter(request => request.method === 'GET').length, 2);
    await context.loadDomainSettings('frontend');
    assert.equal(vm.runInContext('currentConfig.title', context), 'Draft');
    assert.equal(requests.length, 2, 'dirty page must not be fetched again');

    let finishLoad;
    delayedLoad = new Promise(resolve => {
        finishLoad = resolve;
    });
    const pending = context.loadDomainSettings('server');
    assert.equal(vm.runInContext('currentConfig', context), null);
    await context.saveDomainSettings();
    assert.equal(requests.filter(request => request.method === 'PUT').length, 0);
    await context.loadDomainSettings('cache');
    finishLoad(response({title: 'Late server response'}));
    await pending;
    assert.equal(rendered.at(-1).domain, 'cache');
    assert.equal(vm.runInContext('currentDomain', context), 'cache');
    delayedLoad = null;

    vm.runInContext("currentConfig.password = 'draft-secret'; currentConfig.clear_password = true;", context);
    invalid = {
        reportValidity() {
            this.reported = true;
        }
    };
    await context.saveDomainSettings();
    assert.equal(invalid.reported, true);
    assert.equal(requests.filter(request => request.method === 'PUT').length, 0);
    invalid = null;
    rejectSave = true;
    await context.saveDomainSettings();
    assert.equal(vm.runInContext('dirtyDraft(drafts.get("cache"))', context), true);
    assert.equal(vm.runInContext('currentConfig.password', context), 'draft-secret');
    assert.equal(alerts.at(-1), 'safe error');
    rejectSave = false;
    let finishSave;
    delayedSave = new Promise(resolve => {
        finishSave = resolve;
    });
    const saving = context.saveDomainSettings();
    await context.loadDomainSettings('frontend');
    await context.saveDomainSettings();
    assert.equal(vm.runInContext('currentDomain', context), 'cache');
    assert.equal(requests.filter(request => request.method === 'PUT').length, 2, 'duplicate save is suppressed');
    finishSave({ok: true, status: 200, json: async () => ({mode: 'redis', password_configured: true})});
    await saving;
    assert.equal(vm.runInContext('currentConfig.password', context), '');
    assert.equal(vm.runInContext('currentConfig.clear_password', context), false);
    assert.equal(vm.runInContext('dirtyDraft(drafts.get("cache"))', context), false);
    assert.equal(vm.runInContext('dirtyDraft(drafts.get("frontend"))', context), true);
    assert.equal(requests.at(-1).url, '/api/settings/cache');
    assert.equal(requests.at(-1).policy.logoutOnForbidden, false);

    delayedSave = new Promise(resolve => {
        finishSave = resolve;
    });
    vm.runInContext("currentConfig.password = 'another-secret';", context);
    const previousAccountSave = context.saveDomainSettings();
    listeners.get('authChanged')({detail: {isLoggedIn: false}});
    finishSave({ok: true, status: 200, json: async () => ({mode: 'redis'})});
    await previousAccountSave;
    assert.equal(vm.runInContext('drafts.size', context), 0);
    assert.equal(vm.runInContext('currentConfig', context), null);
    assert.equal(vm.runInContext('saving', context), false);
});
