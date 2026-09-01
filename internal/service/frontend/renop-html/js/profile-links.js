/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {el} from '@renop/ui/dom';
import {createIcon} from './components.js';
import {t} from './i18n.js';

/**
 * Build one safe external profile link.
 * @param {string} label - Visible label.
 * @param {string} href - Server-validated HTTP(S) URL.
 * @returns {HTMLAnchorElement|null} External link or null.
 */
function externalProfileLink(label, href) {
    if (!label || !href) return null;
    return el('a', {
        class: 'public-profile-link', href, target: '_blank', rel: 'noopener noreferrer nofollow'
    }, createIcon('network'), el('span', {}, label));
}

/**
 * Render the non-empty public links shared by user and global-team profiles.
 * @param {object|null|undefined} links - Public-link payload.
 * @returns {HTMLElement|null} Link collection or null.
 */
export function createPublicProfileLinks(links) {
    const items = [
        externalProfileLink(t('profile.linkWebsite'), links?.website),
        externalProfileLink(t('profile.linkGitHub'), links?.github),
        externalProfileLink(t('profile.linkDiscord'), links?.discord),
        externalProfileLink(String(links?.custom_name || ''), links?.custom_url),
    ].filter(Boolean);
    return items.length ? el('div', {class: 'public-profile-links'}, ...items) : null;
}

/**
 * Build the shared public-link editor used by user and global-team profiles.
 * @param {object|null|undefined} links - Existing public links.
 * @returns {{element: HTMLElement, value: function(): object|null}} Editor and validated value reader.
 */
export function createPublicProfileLinksEditor(links) {
    /**
     * Build one bounded editor input.
     * @param {'text'|'url'} type - Native input type.
     * @param {string} value - Existing field value.
     * @param {number} [maxLength=2048] - Maximum accepted characters.
     * @returns {HTMLInputElement} Editor input.
     */
    const input = (type, value, maxLength = 2048) => el('input', {
        class: 'profile-input', type, value: value || '', maxlength: String(maxLength), autocomplete: 'url'
    });
    const website = input('url', links?.website);
    const github = input('url', links?.github);
    const discord = input('url', links?.discord);
    const customName = input('text', links?.custom_name, 40);
    customName.autocomplete = 'off';
    const customURL = input('url', links?.custom_url);
    const inputs = [website, github, discord, customName, customURL];
    return {
        element: el('div', {class: 'profile-links-editor'},
            el('label', {}, el('span', {}, t('profile.linkWebsite')), website),
            el('label', {}, el('span', {}, t('profile.linkGitHub')), github),
            el('label', {}, el('span', {}, t('profile.linkDiscord')), discord),
            el('label', {}, el('span', {}, t('profile.customLinkName')), customName),
            el('label', {}, el('span', {}, t('profile.customLinkURL')), customURL)
        ),
        /** @returns {object|null} Validated link payload. */
        value() {
            if (inputs.some(field => !field.reportValidity())) return null;
            const value = {
                website: website.value.trim(), github: github.value.trim(), discord: discord.value.trim(),
                custom_name: customName.value.trim(), custom_url: customURL.value.trim()
            };
            return Boolean(value.custom_name) === Boolean(value.custom_url) ? value : null;
        }
    };
}

/**
 * Build an internal link to one global-team public profile.
 * @param {string} prefix - Immutable global-team prefix.
 * @returns {HTMLAnchorElement|null} Routed team link or null.
 */
export function createSuperTeamPublicLink(prefix) {
    prefix = String(prefix || '').trim().toLowerCase();
    if (!prefix) return null;
    const href = `/team/${encodeURIComponent(prefix)}`;
    const link = el('a', {class: 'super-team-public-link', href}, el('code', {}, prefix), createIcon('chevron'));
    link.addEventListener('click', event => {
        if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
        event.preventDefault();
        if (window.location.pathname !== href || window.location.search || window.location.hash) {
            window.history.pushState(null, '', href);
        }
        window.dispatchEvent(new PopStateEvent('popstate'));
    });
    return link;
}
