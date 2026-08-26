/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {t} from './i18n.js';
import {LocalizedResponseError, localizedResponseError} from './response-errors.js';

const PROFILE_CACHE_TTL_MS = 5 * 60 * 1000;
const profileCache = new Map();
const profileRequests = new Map();
const profileBatchQueue = new Map();
let profileBatchScheduled = false;

/**
 * Fetch one profile without batching, used for route refreshes.
 * @param {string} username - Normalized username.
 * @returns {Promise<object>} Public profile payload.
 */
async function fetchUserProfile(username) {
    const response = await fetch(`/api/users/${encodeURIComponent(username)}/profile`, {
        credentials: 'include', cache: 'no-store'
    });
    if (!response.ok) {
        throw await localizedResponseError(response, 'profile.loadFailed');
    }
    return response.json();
}

/**
 * Resolve queued identity components in batches of at most 50 usernames.
 * @returns {Promise<void>}
 */
async function flushUserProfileBatch() {
    profileBatchScheduled = false;
    const entries = Array.from(profileBatchQueue.entries());
    profileBatchQueue.clear();
    for (let offset = 0; offset < entries.length; offset += 50) {
        const batch = entries.slice(offset, offset + 50);
        const usernames = batch.map(([username]) => username);
        try {
            const response = await fetch(`/api/users/profiles?names=${encodeURIComponent(usernames.join(','))}`, {
                credentials: 'include', cache: 'no-store'
            });
            if (!response.ok) throw await localizedResponseError(response, 'profile.loadFailed');
            const payload = await response.json();
            const profiles = new Map((payload.profiles || []).map(profile => [profile.username, profile]));
            for (const [username, handlers] of batch) {
                const profile = profiles.get(username);
                if (!profile) {
                    const error = new LocalizedResponseError(t('profile.notFound'), 404);
                    handlers.reject(error);
                    continue;
                }
                handlers.resolve(profile);
            }
        } catch (error) {
            for (const [, handlers] of batch) handlers.reject(error);
        }
    }
}

/**
 * Queue one component profile lookup for the current microtask batch.
 * @param {string} username - Normalized username.
 * @returns {Promise<object>} Public profile payload.
 */
function queueUserProfile(username) {
    const request = new Promise((resolve, reject) => {
        profileBatchQueue.set(username, {resolve, reject});
    });
    if (!profileBatchScheduled) {
        profileBatchScheduled = true;
        queueMicrotask(() => void flushUserProfileBatch());
    }
    return request;
}

/**
 * Read one public profile with bounded shared caching.
 * @param {string} username - Account username.
 * @param {{refresh?: boolean}} [options] - Cache behavior.
 * @returns {Promise<object>} Public profile payload.
 */
export async function getUserProfile(username, {refresh = false} = {}) {
    const normalized = String(username || '').trim().toLowerCase();
    if (!normalized) throw new Error(t('profile.notFound'));
    const now = Date.now();
    const cached = profileCache.get(normalized);
    if (!refresh && cached && cached.expiresAt > now) return cached.profile;
    if (!refresh && profileRequests.has(normalized)) return profileRequests.get(normalized);
    const request = (async () => {
        const profile = refresh
            ? await fetchUserProfile(normalized)
            : await queueUserProfile(normalized);
        profileCache.set(normalized, {profile, expiresAt: Date.now() + PROFILE_CACHE_TTL_MS});
        return profile;
    })();
    profileRequests.set(normalized, request);
    try {
        return await request;
    } finally {
        profileRequests.delete(normalized);
    }
}

/**
 * Return a public-facing name without exposing the account username.
 * @param {object|null} profile - User profile.
 * @returns {string} Nickname or localized anonymous label.
 */
export function profileDisplayName(profile) {
    return String(profile?.nickname || '').trim() || t('profile.unnamedUser');
}

/**
 * Resolve one username to its nickname-first public label.
 * @param {string} username - Account username.
 * @returns {Promise<string>} Public display label.
 */
export async function resolveUserDisplayName(username) {
    try {
        return profileDisplayName(await getUserProfile(username));
    } catch {
        return t('profile.unnamedUser');
    }
}

/**
 * Invalidate cached identity after a profile change.
 * @param {...string} usernames - Current or previous usernames to invalidate.
 * @returns {void}
 */
export function invalidateUserProfiles(...usernames) {
    for (const username of usernames) {
        const normalized = String(username || '').trim().toLowerCase();
        if (normalized) profileCache.delete(normalized);
    }
}

/**
 * Parse a username-based profile route and its optional package section.
 * @param {string} path - Browser path.
 * @returns {{username: string, section: ''|'edit'|'maven'|'cargo'|'docker'}|null} Parsed route.
 */
export function profileRouteFromPath(path) {
    const match = /^\/user\/([^/]+)(?:\/(edit|maven|cargo|docker))?\/?$/.exec(String(path || ''));
    if (!match) return null;
    try {
        const username = decodeURIComponent(match[1]).trim().toLowerCase();
        return username ? {username, section: match[2] || ''} : null;
    } catch {
        return null;
    }
}

/**
 * Navigate to a user profile through browser history.
 * @param {string} username - Current account username used by the public route.
 * @param {''|'edit'|'maven'|'cargo'|'docker'} [section=''] - Optional profile section.
 * @param {boolean} [replace=false] - Replace the current history entry.
 * @returns {void}
 */
export function navigateToUserProfile(username, section = '', replace = false) {
    const normalized = String(username || '').trim().toLowerCase();
    if (!normalized) return;
    const normalizedSection = section === 'edit' || section === 'maven' || section === 'cargo' || section === 'docker'
        ? section
        : '';
    const path = `/user/${encodeURIComponent(normalized)}${normalizedSection ? `/${normalizedSection}` : ''}`;
    if (!replace && window.location.pathname.replace(/\/$/, '') === path) return;
    const currentRoute = profileRouteFromPath(window.location.pathname);
    const returnPath = window.history.state?.renopProfileReturnPath
        || (currentRoute ? '/' : window.location.pathname);
    const state = {
        renopProfileReturnPath: returnPath,
        renopProfileCanGoBack: replace
            ? Boolean(window.history.state?.renopProfileCanGoBack)
            : true
    };
    if (replace) window.history.replaceState(state, '', path);
    else window.history.pushState(state, '', path);
    window.dispatchEvent(new PopStateEvent('popstate'));
}

/**
 * Navigate to a profile using its current username.
 * @param {string} username - Current account username.
 * @param {boolean} [replace=false] - Replace the current history entry.
 * @returns {void}
 */
export function navigateToUsernameProfile(username, replace = false) {
    navigateToUserProfile(username, '', replace);
}
