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
const PROFILE_CACHE_MAX_ENTRIES = 256;
const profileCache = new Map();
const profileRequests = new Map();
const profileBatchQueue = new Map();
let profileBatchScheduled = false;
let profileCacheGeneration = 0;

/**
 * Store one profile while pruning expired and oldest entries.
 * @param {string} username - Normalized username.
 * @param {object} profile - Profile payload.
 * @returns {void}
 */
function cacheUserProfile(username, profile) {
    const now = Date.now();
    profileCache.delete(username);
    for (const [cachedUsername, cached] of profileCache) {
        if (cached.expiresAt <= now) profileCache.delete(cachedUsername);
    }
    if (profileCache.size >= PROFILE_CACHE_MAX_ENTRIES) {
        profileCache.delete(profileCache.keys().next().value);
    }
    profileCache.set(username, {profile, expiresAt: now + PROFILE_CACHE_TTL_MS});
}

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
    if (cached) profileCache.delete(normalized);
    if (!refresh && profileRequests.has(normalized)) return profileRequests.get(normalized);
    const generation = profileCacheGeneration;
    const request = (async () => {
        const profile = refresh
            ? await fetchUserProfile(normalized)
            : await queueUserProfile(normalized);
        if (generation === profileCacheGeneration) cacheUserProfile(normalized, profile);
        return profile;
    })();
    profileRequests.set(normalized, request);
    try {
        return await request;
    } finally {
        if (profileRequests.get(normalized) === request) profileRequests.delete(normalized);
    }
}

/** Clear all cached profile payloads when the authenticated account changes. */
export function clearUserProfileCache() {
    profileCacheGeneration++;
    profileCache.clear();
    profileRequests.clear();
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
 * Return a shared initials fallback for identity avatars.
 * @param {object|null} profile - User profile.
 * @param {number} [length=2] - Maximum visible characters.
 * @returns {string} Uppercase avatar fallback.
 */
export function profileAvatarText(profile, length = 2) {
    const boundedLength = Math.max(1, Math.min(2, Math.trunc(Number(length) || 2)));
    return Array.from(profileDisplayName(profile)).slice(0, boundedLength).join('').toUpperCase() || '?';
}

/**
 * Return a same-origin cached avatar URL from a public profile payload.
 * @param {object|null} profile - User profile.
 * @returns {string} Safe local avatar URL or an empty string.
 */
export function profileAvatarURL(profile) {
    const value = String(profile?.avatar_url || '').trim();
    if (!value.startsWith('/api/users/')) return '';
    try {
        const url = new URL(value, window.location.origin);
        return url.origin === window.location.origin && url.pathname.startsWith('/api/users/') ? url.href : '';
    } catch {
        return '';
    }
}

/**
 * Render a cached avatar image with the shared initials fallback.
 * @param {HTMLElement|null} element - Avatar host.
 * @param {object|null} profile - User profile.
 * @param {{length?: number}} [options] - Initials length.
 * @returns {void}
 */
export function renderProfileAvatar(element, profile, {length = 2} = {}) {
    if (!element) return;
    const fallback = profileAvatarText(profile, length);
    const avatarURL = profileAvatarURL(profile);
    if (avatarURL && element.dataset.avatarUrl === avatarURL && element.querySelector('img')) return;
    element.dataset.avatarUrl = avatarURL;
    element.replaceChildren(document.createTextNode(fallback));
    element.classList.toggle('has-profile-avatar', Boolean(avatarURL));
    if (!avatarURL) return;
    const image = document.createElement('img');
    image.src = avatarURL;
    image.alt = '';
    image.decoding = 'async';
    image.draggable = false;
    image.addEventListener('error', () => {
        if (element.dataset.avatarUrl !== avatarURL || !element.contains(image)) return;
        image.remove();
        element.classList.remove('has-profile-avatar');
        element.dataset.avatarUrl = '';
    }, {once: true});
    element.appendChild(image);
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
    const invalidated = [];
    for (const username of usernames) {
        const normalized = String(username || '').trim().toLowerCase();
        if (normalized) {
            profileCache.delete(normalized);
            invalidated.push(normalized);
        }
    }
    if (invalidated.length > 0) {
        window.dispatchEvent(new CustomEvent('userProfilesInvalidated', {detail: {usernames: invalidated}}));
    }
}

/**
 * Publish one authoritative profile update to the shared cache and every mounted identity.
 * @param {object} profile - Updated public profile payload.
 * @param {{oldUsername?: string}} [options] - Previous route identity after a rename.
 * @returns {void}
 */
export function syncUserProfile(profile, {oldUsername = ''} = {}) {
    const username = String(profile?.username || '').trim().toLowerCase();
    if (!username) return;
    const previous = String(oldUsername || '').trim().toLowerCase();
    if (previous && previous !== username) profileCache.delete(previous);
    cacheUserProfile(username, profile);
    window.dispatchEvent(new CustomEvent('userProfileChanged', {
        detail: {profile, username, oldUsername: previous}
    }));
}

/**
 * Parse a username-based profile route and its optional package section.
 * @param {string} path - Browser path.
 * @returns {{username: string, section: ''|'edit'|'maven'|'cargo'|'docker'|'npm'}|null} Parsed route.
 */
export function profileRouteFromPath(path) {
    const match = /^\/user\/([^/]+)(?:\/(edit|maven|cargo|docker|npm))?\/?$/.exec(String(path || ''));
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
 * @param {''|'edit'|'maven'|'cargo'|'docker'|'npm'} [section=''] - Optional profile section.
 * @param {boolean} [replace=false] - Replace the current history entry.
 * @returns {void}
 */
export function navigateToUserProfile(username, section = '', replace = false) {
    const normalized = String(username || '').trim().toLowerCase();
    if (!normalized) return;
    const normalizedSection = section === 'edit' || section === 'maven' || section === 'cargo' ||
    section === 'docker' || section === 'npm'
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
