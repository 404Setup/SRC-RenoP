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
import {canonicalPageURL, docURL} from '../js/lib/doc-routes.js';

test('API links leave general docs and preserve old bookmarks', () => {
    assert.equal(docURL('api/authentication'), '/api/authentication');
    assert.equal(docURL('security/oauth-login'), '/docs/security/oauth-login');
    assert.equal(canonicalPageURL(new URL('https://example.test/docs/api/settings?view=openapi#legal')),
        '/api/settings?view=openapi#legal');
    assert.equal(canonicalPageURL(new URL('https://example.test/docs/api-guide')), '/docs/api-guide');
});

test('navigation during an API render renders the latest URL and cleans up the previous page', async () => {
    const source = readFileSync(new URL('../js/router.js', import.meta.url), 'utf8')
        .replace(/^import .*;\r?\n/gm, '').replace(/^export /gm, '');
    const root = {childNodes: [], offsetWidth: 1};
    let onClick;
    const chain = {get: () => root, find: () => ({length: 0}), each() {}, removeClass() { return this; },
        addClass() { return this; }, empty() { return this; },
        on(event, selector, handler) { if (event === 'click') onClick = handler; }};
    const context = vm.createContext({
        URL, URLSearchParams, canonicalPageURL, location: new URL('https://example.test/api'),
        document: {documentElement: {scrollTop: 0}}, window: {scrollY: 0},
        $: () => chain, wait: async () => {}, smoothScrollToTop() {}, history: {},
    });
    vm.runInContext(source, context);
    const rendered = [];
    let release;
    const pending = new Promise(resolve => { release = resolve; });
    context.registerRoute('/api', async () => {
        rendered.push('api');
        await pending;
        return () => rendered.push('cleanup');
    });
    context.registerRoute('/docs', () => { rendered.push('docs'); });
    const initial = context.renderRoute();
    context.location = new URL('https://example.test/docs');
    await context.renderRoute();
    release();
    await initial;
    assert.deepEqual(rendered, ['api', 'cleanup', 'docs']);
    context.location = new URL('https://example.test/api?view=openapi');
    context.history.pushState = (_state, _title, path) => { context.location = new URL(path, context.location); };
    context.initRouter();
    onClick({currentTarget: {getAttribute: () => '/api'}, button: 0, preventDefault() {}});
    assert.equal(context.location.href, 'https://example.test/api');
});
