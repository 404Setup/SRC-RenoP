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

const endpoint = process.env.RENOP_TEST_BROWSER_CDP;
const selector = process.env.RENOP_TEST_SELECT_SELECTOR || '[data-mail-select="template_style"] button';

test('shared select supports keyboard selection, Escape, and focus restoration', {skip: !endpoint}, async () => {
    const pages = await (await fetch(endpoint + '/json/list')).json();
    const page = pages.find(value => value.type === 'page' && value.url.startsWith(process.env.RENOP_TEST_BROWSER_URL || 'http://127.0.0.1:18080'));
    assert.ok(page, 'Open the configured page with its select visible.');
    const socket = new WebSocket(page.webSocketDebuggerUrl);
    await new Promise((resolve, reject) => {
        socket.onopen = resolve;
        socket.onerror = reject;
    });
    let next = 0;
    const pending = new Map();
    socket.onmessage = event => {
        const value = JSON.parse(event.data);
        if (!value.id) return;
        const entry = pending.get(value.id);
        pending.delete(value.id);
        clearTimeout(entry.timer);
        value.error ? entry.reject(value.error) : entry.resolve(value.result);
    };
    const call = (method, params = {}) => new Promise((resolve, reject) => {
        const id = ++next;
        pending.set(id, {
            resolve,
            reject,
            timer: setTimeout(() => reject(new Error('Browser command timed out')), 10000)
        });
        socket.send(JSON.stringify({id, method, params}));
    });
    const evaluate = async expression => {
        const value = await call('Runtime.evaluate', {expression, returnByValue: true});
        assert.equal(value.exceptionDetails, undefined);
        return value.result.value;
    };
    const key = async value => {
        await call('Input.dispatchKeyEvent', {type: 'keyDown', key: value});
        await call('Input.dispatchKeyEvent', {type: 'keyUp', key: value});
    };
    try {
        await evaluate('window.selectUnderTest = document.querySelector(' + JSON.stringify(selector) + '); selectUnderTest.focus()');
        const sectionPresent = await evaluate('window.sectionUnderTest = selectUnderTest.closest(".cfg-section"); Boolean(sectionUnderTest)');
        if (sectionPresent) {
            await evaluate('window.sectionHeader = sectionUnderTest.querySelector(".cfg-section-header"); if(sectionUnderTest.classList.contains("is-collapsed")) sectionHeader.click(); sectionHeader.focus()');
            await key('Enter');
            assert.equal(await evaluate('sectionUnderTest.querySelector(".cfg-section-body").inert'), true);
            await evaluate('selectUnderTest.focus()');
            assert.equal(await evaluate('document.activeElement === sectionHeader'), true);
            await key('Enter');
            assert.equal(await evaluate('sectionUnderTest.querySelector(".cfg-section-body").inert'), false);
            await evaluate('selectUnderTest.focus()');
        }
        const original = await evaluate('selectUnderTest.textContent.trim()');
        await key('ArrowDown');
        assert.equal(await evaluate('document.activeElement.getAttribute("role")'), 'option');
        await key('ArrowDown');
        const expected = await evaluate('document.activeElement.textContent.trim()');
        await key('Enter');
        assert.equal(await evaluate('selectUnderTest.textContent.trim()'), expected);
        assert.equal(await evaluate('document.activeElement === selectUnderTest'), true);
        await key('ArrowDown');
        await key('Escape');
        assert.equal(await evaluate('selectUnderTest.getAttribute("aria-expanded")'), 'false');
        assert.equal(await evaluate('document.activeElement === selectUnderTest'), true);
        await key('ArrowDown');
        await evaluate('Array.from(document.getElementById(selectUnderTest.getAttribute("aria-controls")).children).find(item => item.textContent.trim() === ' + JSON.stringify(original) + ').click()');
        assert.equal(await evaluate('selectUnderTest.textContent.trim()'), original);
    } finally {
        socket.close();
        for (const entry of pending.values()) clearTimeout(entry.timer);
    }
});
