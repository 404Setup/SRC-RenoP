/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {apiRequest} from './api.js';
import {getAvailableLanguages, getLanguage, setLanguage, t} from './i18n.js';
import {showAlert} from './alert.js';

const endpoint = '/api/auth/profile/locale';
const pendingKey = 'renop_account_language_pending';

/** Restore each signed-in account's language and serialize subsequent preference writes. */
export function installAccountLanguageSync() {
    let active = null;

    /** Preserve a single unfinished choice across reloads without applying it to another account. */
    function remember(state, language) {
        state.pending = language;
        if (state.userID) localStorage.setItem(pendingKey, JSON.stringify({user_id: state.userID, locale: language}));
    }

    /** A completed request must not discard a newer choice from another tab. */
    function forget(state, language) {
        if (localStorage.getItem(pendingKey) === JSON.stringify({user_id: state.userID, locale: language})) localStorage.removeItem(pendingKey);
    }

    /** Save the newest choice after an older request completes. */
    async function save(state) {
        if (active !== state || !state.ready || state.saving || !state.pending) return;
        if (state.pending === state.saved) {
            forget(state, state.pending);
            state.pending = '';
            return;
        }
        const language = state.pending;
        state.saving = true;
        try {
            const response = await apiRequest(endpoint, {
                method: 'PUT', headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({user_id: state.userID, locale: language}), keepalive: true,
                signal: AbortSignal.any([state.controller.signal, AbortSignal.timeout(10000)]),
            });
            if (!response.ok) throw new Error('Account language update failed');
            if (active !== state) return;
            state.saved = language;
            state.saveFailed = false;
            if (state.pending === language) {
                state.pending = '';
                forget(state, language);
            }
        } catch {
            if (active === state && !state.saveFailed) {
                state.saveFailed = true;
                showAlert(t('profile.languageSaveFailed'), 'error');
            }
        } finally {
            state.saving = false;
            if (active === state && state.pending && state.pending !== language) void save(state);
        }
    }

    /** Account hydration must not overwrite a newer selection or an unfinished local write. */
    async function hydrate(state) {
        if (active !== state || state.loading) return;
        state.loading = true;
        const revision = getLanguage().revision;
        try {
            const response = await apiRequest(endpoint, {signal: AbortSignal.any([state.controller.signal, AbortSignal.timeout(10000)])});
            if (!response.ok) throw new Error('Account language lookup failed');
            const data = await response.json();
            if (active !== state) return;
            if (data.username !== state.username) throw new Error('Active account changed');
            if (typeof data.user_id !== 'string' || !data.user_id || data.user_id.length > 64) throw new Error('Invalid account identity');
            if (data.locale !== '' && !getAvailableLanguages().includes(data.locale)) throw new Error('Invalid account language');
            state.userID = data.user_id;
            if (state.pending) remember(state, state.pending);
            else {
                try {
                    const pending = JSON.parse(localStorage.getItem(pendingKey));
                    if (pending?.user_id === state.userID && getAvailableLanguages().includes(pending.locale)) state.pending = pending.locale;
                } catch {
                    localStorage.removeItem(pendingKey);
                }
            }
            state.saved = data.locale;
            state.ready = true;
            state.loadFailed = false;
            if (state.pending) {
                if (getLanguage().revision === revision && getLanguage().current !== state.pending) await setLanguage(state.pending, 'account', state.controller.signal);
                void save(state);
            } else if (getLanguage().revision === revision) {
                if (data.locale) {
                    if (getLanguage().current !== data.locale) await setLanguage(data.locale, 'account', state.controller.signal);
                } else {
                    remember(state, getLanguage().current);
                    void save(state);
                }
            }
        } catch {
            if (active === state && !state.loadFailed) {
                state.loadFailed = true;
                showAlert(t('profile.languageLoadFailed'), 'error');
            }
        } finally {
            state.loading = false;
        }
    }

    window.addEventListener('authChanged', event => {
        const username = event.detail?.isLoggedIn ? event.detail.username : '';
        if (active?.username === username) return;
        active?.controller.abort();
        active = null;
        if (!username) return;
        const state = {username, controller: new AbortController(), ready: false, pending: ''};
        active = state;
        void hydrate(state);
    });
    window.addEventListener('languageChanged', event => {
        if (!active || event.detail?.source !== 'user-set') return;
        remember(active, event.detail.lang);
        void save(active);
    });
    const retry = () => {
        if (active?.ready) void save(active);
        else if (active) void hydrate(active);
    };
    window.addEventListener('online', retry);
    window.addEventListener('pageshow', retry);
    document.addEventListener('visibilitychange', () => {
        if (document.visibilityState === 'visible') retry();
    });
}
