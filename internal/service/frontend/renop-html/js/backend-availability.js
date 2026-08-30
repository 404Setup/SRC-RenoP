/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

const probeRetryDelayMs = 650;
const probeTimeoutMs = 4000;

/**
 * Coordinate backend failure confirmation without trusting timers that ran while a page was suspended.
 */
export class BackendAvailabilityMonitor {
    /**
     * @param {{probe: () => Promise<boolean>, isVisible: () => boolean, onOffline: () => void, onOnline: () => void, delay?: (milliseconds: number) => Promise<void>}} options
     */
    constructor({
                    probe,
                    isVisible,
                    onOffline,
                    onOnline,
                    delay = milliseconds => new Promise(resolve => setTimeout(resolve, milliseconds))
                }) {
        this.probe = probe;
        this.isVisible = isVisible;
        this.onOffline = onOffline;
        this.onOnline = onOnline;
        this.delay = delay;
        this.pendingFailure = false;
        this.offline = false;
        this.confirmation = null;
    }

    /** Record a failed same-origin request and confirm it only while the page is visible. */
    noteFailure() {
        this.pendingFailure = true;
        if (this.isVisible()) void this.confirm();
    }

    /** Record backend reachability and clear any stale offline overlay. */
    noteSuccess() {
        this.pendingFailure = false;
        this.offline = false;
        this.onOnline();
    }

    /** Recheck pending or displayed failures after the document returns to the foreground. */
    resume() {
        if (!this.isVisible() || (!this.pendingFailure && !this.offline)) return;
        this.pendingFailure = true;
        void this.confirm();
    }

    /**
     * Explicitly retry the backend connection.
     * @returns {Promise<boolean>} Whether the backend responded.
     */
    retry() {
        this.pendingFailure = true;
        return this.confirm();
    }

    /**
     * Run a probe while treating provider exceptions as a failed confirmation.
     * @returns {Promise<boolean>}
     */
    async runProbe() {
        try {
            return await this.probe();
        } catch {
            return false;
        }
    }

    /**
     * Confirm a failure with two bounded probes before exposing the blocking offline state.
     * @returns {Promise<boolean>} Whether the backend responded.
     */
    confirm() {
        if (this.confirmation) return this.confirmation;
        this.confirmation = (async () => {
            if (!this.isVisible()) return false;
            if (await this.runProbe()) {
                this.noteSuccess();
                return true;
            }
            await this.delay(probeRetryDelayMs);
            if (!this.pendingFailure) return true;
            if (!this.isVisible()) return false;
            if (await this.runProbe()) {
                this.noteSuccess();
                return true;
            }
            if (!this.isVisible()) return false;
            this.pendingFailure = false;
            this.offline = true;
            this.onOffline();
            return false;
        })().finally(() => {
            this.confirmation = null;
            if (this.pendingFailure && this.isVisible()) queueMicrotask(() => void this.confirm());
        });
        return this.confirmation;
    }
}

/**
 * Test whether a fetch target resolves to the current application origin.
 * @param {Request|string|URL} input - Fetch input.
 * @param {string} baseURL - Current page URL.
 * @returns {boolean}
 */
export function isSameOriginRequest(input, baseURL) {
    try {
        const target = typeof input === 'object' && input !== null && 'url' in input ? input.url : input;
        const base = new URL(baseURL);
        return new URL(String(target), base).origin === base.origin;
    } catch {
        return false;
    }
}

/**
 * Install the same-origin fetch monitor and foreground resume hooks.
 * @param {Window} [windowObject=window]
 * @param {Document} [documentObject=document]
 * @returns {BackendAvailabilityMonitor}
 */
export function installBackendAvailabilityMonitor(windowObject = window, documentObject = document) {
    const originalFetch = windowObject.fetch.bind(windowObject);
    const offlineElement = documentObject.getElementById('backend-offline');
    const setOfflineVisible = visible => {
        if (offlineElement) offlineElement.style.display = visible ? 'flex' : 'none';
    };
    const probe = async () => {
        const controller = new windowObject.AbortController();
        const timer = windowObject.setTimeout(() => controller.abort(), probeTimeoutMs);
        try {
            const response = await originalFetch('/api/status/health', {
                method: 'GET', cache: 'no-store', credentials: 'same-origin', signal: controller.signal
            });
            return response.ok;
        } catch {
            return false;
        } finally {
            windowObject.clearTimeout(timer);
        }
    };
    const monitor = new BackendAvailabilityMonitor({
        probe,
        isVisible: () => documentObject.visibilityState === 'visible',
        onOffline: () => setOfflineVisible(true),
        onOnline: () => setOfflineVisible(false)
    });
    windowObject.fetch = async (...args) => {
        const tracked = isSameOriginRequest(args[0], windowObject.location.href);
        try {
            const response = await originalFetch(...args);
            if (tracked) monitor.noteSuccess();
            return response;
        } catch (error) {
            if (tracked && error instanceof TypeError) monitor.noteFailure();
            throw error;
        }
    };
    documentObject.addEventListener('visibilitychange', () => monitor.resume());
    windowObject.addEventListener('pageshow', () => monitor.resume());
    windowObject.addEventListener('online', () => monitor.resume());
    return monitor;
}
