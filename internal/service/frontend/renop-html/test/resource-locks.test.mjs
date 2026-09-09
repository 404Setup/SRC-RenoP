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

test('Maven version locks hide deletion while retaining staff lock controls and metadata', () => {
    const actions = [];
    const context = vm.createContext({
        el: (tag, attributes, ...children) => {
            const node = {tag, ...attributes, children, appendChild(child) { this.children.push(child); }};
            if (attributes.class === 'maven-version-actions') actions.push(node);
            return node;
        },
        t: key => key, formatDate: value => value, formatBytes: value => value,
        createIcon: name => ({icon: name}), createResourceLockNotices: locks => ({locks}),
        mavenVersionFiles: version => ({files: version.files}),
    });
    const locks = readFileSync(new URL('../js/resource-locks.js', import.meta.url), 'utf8');
    const maven = readFileSync(new URL('../js/browser/maven.js', import.meta.url), 'utf8');
    vm.runInContext(locks.match(/export function resourceWriteLocked\([^]*?\n}/)[0].replace('export ', '') + '\n' +
        maven.match(/function mavenVersionEntry\([^]*?\n}(?=\r?\n)/)[0], context);
    const locked = {version: '2.0', locks: [{mode: 'read', reason: 'trojan'}], files: [{name: 'demo.jar'}]};
    const options = {canManageVersions: true, manageLock: () => ({lockButton: true}), artifact: {}};
    const row = context.mavenVersionEntry(locked, options);
    assert.equal(actions.at(-1).children.length, 1);
    assert.equal(actions.at(-1).children[0].lockButton, true);
    assert.equal(row.children.at(-1).files, locked.files);
    context.mavenVersionEntry({version: '1.0'}, options);
    assert.equal(actions.at(-1).children.length, 2);
    context.mavenVersionEntry(locked, {...options, manageLock: null});
    assert.equal(actions.at(-1).children.length, 0);
    context.mavenVersionEntry({version: '3.0', review_status: 'pending'}, options);
    assert.equal(actions.at(-1).children.length, 0);
});

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
        locks: [system, {mode: 'read', source: 'manual', inherited: true}], name: 'demo', request: async (mode, reason) => {
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

test('Docker lock refreshes discard responses after navigation or view dismissal', async () => {
    const requests = [], payloads = [];
    const view = {hidden: false, querySelector: () => null};
    let renders = 0;
    const context = vm.createContext({
        view, apiRequest: () => new Promise(resolve => requests.push(resolve)),
        dockerUserSuggestions: {detach() {}}, setRepositoryViewBusy() {},
        replaceRepositoryView: () => { renders++; },
        hideRepositoryView: container => { container.hidden = true; },
    });
    const source = readFileSync(new URL('../js/browser/docker.js', import.meta.url), 'utf8');
    vm.runInContext(`let dockerLoadSequence = 1, dockerViewContainer = view;
        ${['renderImageDetailsView', 'hideDockerRepositoryView'].map(name =>
            source.match(new RegExp(`(?:export )?(?:async )?function ${name}\\([^]*?\\n}`))[0].replace('export ', '')
        ).join('\n')}`, context);
    const first = context.renderImageDetailsView(view, 'docker', 'demo', 1);
    let started;
    const jsonStarted = new Promise(resolve => { started = resolve; });
    requests[0]({ok: true, json: () => new Promise(resolve => { payloads.push(resolve); started(); })});
    await jsonStarted;
    context.hideDockerRepositoryView();
    payloads[0]({image: {image_name: 'demo'}});
    await first;
    await context.renderImageDetailsView(view, 'docker', 'demo', 2);
    assert.equal(renders, 0);
    assert.equal(requests.length, 1);
});


test('team and domain locks retain members and staff actions while hiding mutation controls', () => {
    const nodes = [];
    const context = vm.createContext({
        el: (tag, attributes, ...children) => {
            const node = {tag, ...attributes, children, appendChild(child) { this.children.push(child); },
                append(...items) { this.children.push(...items); }, get childElementCount() { return this.children.length; }};
            nodes.push(node);
            return node;
        },
        t: key => key, roleLabel: level => `T${level}`, permissionLabel: level => `L${level}`,
        createIcon: name => ({icon: name}), createResourceLockNotices: locks => ({locks}),
        createResourceLockButton: () => ({tag: 'button', staffLock: true}),
        createUserIdentity: name => ({name}), createPublicProfileLinks: () => null,
        createSuperTeamResourcesSection: () => null,
        createPublicationQuotaPanel: (_status, options) => ({quotaEditable: options.editable}),
        makeCustomSelect: () => ({tag: 'select'}), roleOptions: () => [],
        localStorage: {getItem: () => 'alice'}, cachedIsLoggedIn: true, loadGeneration: 1,
    });
    const team = readFileSync(new URL('../js/super-teams.js', import.meta.url), 'utf8');
    const maven = readFileSync(new URL('../js/browser/maven.js', import.meta.url), 'utf8');
    const locks = readFileSync(new URL('../js/resource-locks.js', import.meta.url), 'utf8');
    const extract = (source, name) => source.match(new RegExp(`(?:export )?function ${name}\\([^]*?\\n}`))[0].replace('export ', '');
    vm.runInContext([extract(locks, 'resourceWriteLocked'), extract(team, 'memberRow'),
        extract(team, 'teamDetailContent'), extract(maven, 'teamPanel')].join('\n'), context);
    const restriction = {mode: 'read', reason: 'abuse'};
    const members = [{username: 'alice', level: 3}, {username: 'bob', level: 2}];
    const content = context.teamDetailContent({team: {prefix: 'demo', role_level: 4, locks: [restriction]},
        members, administrator: true, moderator: true}, 'demo', {quotaStatus: {}});
    assert.equal(content[1].locks[0], restriction);
    assert.equal(content[2].quotaEditable, false);
    assert.equal(nodes.filter(node => node.tag === 'select').length, 0);
    assert.equal(nodes.filter(node => node.tag === 'button').length, 1); // Back navigation.
    assert.equal(nodes.find(node => node.class === 'super-team-detail-actions').children[0].staffLock, true);
    nodes.length = 0;
    context.teamPanel({domain: {domain: 'com.example', permission_level: 4, locks: [restriction]},
        members, administrator: true}, () => {});
    assert.equal(nodes.filter(node => ['button', 'select', 'form'].includes(node.tag)).length, 0);
    assert.equal(nodes.filter(node => node.class === 'maven-team-row').length, 2);
});
