/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

import test from 'node:test';
import assert from 'node:assert/strict';

import {BackendAvailabilityMonitor, isSameOriginRequest} from '../js/backend-availability.js';

test('background request failures wait for a visible confirmation probe', async () => {
    let visible = false;
    let probes = 0;
    let offline = 0;
    const monitor = new BackendAvailabilityMonitor({
        probe: async () => {
            probes++;
            return true;
        },
        isVisible: () => visible,
        onOffline: () => offline++,
        onOnline: () => {},
        delay: async () => {}
    });
    monitor.noteFailure();
    await Promise.resolve();
    assert.equal(probes, 0);
    assert.equal(offline, 0);
    visible = true;
    monitor.resume();
    await monitor.confirm();
    assert.equal(probes, 1);
    assert.equal(offline, 0);
});

test('visible failures require two failed probes and recover on later success', async () => {
    const probeResults = [false, false, true];
    let offline = 0;
    let online = 0;
    const monitor = new BackendAvailabilityMonitor({
        probe: async () => probeResults.shift() ?? false,
        isVisible: () => true,
        onOffline: () => offline++,
        onOnline: () => online++,
        delay: async () => {}
    });
    monitor.noteFailure();
    assert.equal(await monitor.confirm(), false);
    assert.equal(offline, 1);
    monitor.resume();
    assert.equal(await monitor.confirm(), true);
    assert.equal(online, 1);
});

test('only same-origin fetches participate in backend availability', () => {
    const base = 'https://packages.example/settings';
    assert.equal(isSameOriginRequest('/api/status/health', base), true);
    assert.equal(isSameOriginRequest('https://packages.example/api/status/health', base), true);
    assert.equal(isSameOriginRequest('https://cdn.example/assets/app.js', base), false);
    assert.equal(isSameOriginRequest({url: 'https://packages.example/api/status/hash'}, base), true);
});
