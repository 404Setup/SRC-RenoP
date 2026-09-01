/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {getUserProfile, navigateToUserProfile, profileDisplayName} from '../user-profiles.js';
import {t} from '../i18n.js';
import {createUserAvatar} from './user-avatar.js';

/**
 * Nickname-first linked identity with shared profile loading and truncation.
 */
export class RenopUserIdentity extends HTMLElement {
    /**
     * @returns {string[]} Attributes that trigger a new profile render.
     */
    static get observedAttributes() {
        return ['username', 'avatar', 'template'];
    }

    /**
     * Load identity when inserted into the document.
     * @returns {void}
     */
    connectedCallback() {
        this.render();
    }

    /**
     * Reload identity when routing or presentation attributes change.
     * @returns {void}
     */
    attributeChangedCallback() {
        if (this.isConnected) this.render();
    }

    /**
     * Load and render the current username attribute.
     * @returns {void}
     */
    render() {
        const username = String(this.getAttribute('username') || '').trim().toLowerCase();
        const version = String((Number(this.dataset.renderVersion) || 0) + 1);
        this.dataset.renderVersion = version;
        this.className = 'user-identity';
        this.replaceChildren();
        if (!username) return;
        const label = document.createElement('span');
        label.className = 'user-identity-name is-loading';
        label.textContent = '…';
        const link = document.createElement('a');
        link.className = 'user-identity-link';
        link.href = `/user/${encodeURIComponent(username)}`;
        link.addEventListener('click', event => {
            if (event.button !== 0 || event.ctrlKey || event.metaKey || event.shiftKey || event.altKey) return;
            event.preventDefault();
            navigateToUserProfile(username);
        });
        if (this.hasAttribute('avatar')) link.appendChild(createUserAvatar('?'));
        link.appendChild(label);
        this.appendChild(link);
        void getUserProfile(username).then(profile => {
            if (!this.isConnected || this.dataset.renderVersion !== version) return;
            this.applyProfile(profile);
        }).catch(() => {
            if (!this.isConnected || this.dataset.renderVersion !== version) return;
            label.textContent = '—';
            label.classList.remove('is-loading');
        });
    }

    /**
     * Apply an already resolved shared profile without replacing the identity DOM.
     * @param {object} profile - Public profile payload.
     * @returns {void}
     */
    applyProfile(profile) {
        this._profile = profile;
        const displayName = profileDisplayName(profile);
        const template = this.getAttribute('template') || '';
        const label = this.querySelector('.user-identity-name');
        if (label) {
            label.textContent = template ? t(template, {name: displayName}) : displayName;
            label.title = displayName;
            label.classList.remove('is-loading');
        }
        const avatar = this.querySelector('renop-user-avatar');
        if (avatar) {
            avatar.setAttribute('name', displayName);
            avatar.setProfile(profile);
        }
    }
}

if (!customElements.get('renop-user-identity')) {
    customElements.define('renop-user-identity', RenopUserIdentity);
}

window.addEventListener('languageChanged', () => {
    document.querySelectorAll('renop-user-identity').forEach(identity => {
        if (identity._profile) identity.applyProfile(identity._profile);
    });
});

window.addEventListener('userProfileChanged', event => {
    const detail = event instanceof CustomEvent ? event.detail : null;
    const oldUsername = String(detail?.oldUsername || '').toLowerCase();
    const newUsername = String(detail?.username || '').toLowerCase();
    document.querySelectorAll('renop-user-identity').forEach(identity => {
        if (oldUsername && identity.getAttribute('username') === oldUsername) {
            identity.setAttribute('username', newUsername);
        }
        if (identity.getAttribute('username') === newUsername) identity.applyProfile(detail.profile);
    });
});

window.addEventListener('userProfilesInvalidated', event => {
    const usernames = event instanceof CustomEvent ? event.detail?.usernames : null;
    if (!Array.isArray(usernames)) return;
    document.querySelectorAll('renop-user-identity').forEach(identity => {
        if (usernames.includes(String(identity.getAttribute('username') || '').toLowerCase())) identity.render();
    });
});

/**
 * Create a nickname-first user identity element.
 * @param {string} username - Account username used only for profile lookup and routing.
 * @param {{avatar?: boolean, template?: string}} [options] - Visual options.
 * @returns {HTMLElement} User identity element.
 */
export function createUserIdentity(username, {avatar = false, template = ''} = {}) {
    const identity = document.createElement('renop-user-identity');
    identity.setAttribute('username', username || '');
    if (avatar) identity.setAttribute('avatar', '');
    if (template) identity.setAttribute('template', template);
    return identity;
}
