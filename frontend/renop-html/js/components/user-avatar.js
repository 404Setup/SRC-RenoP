/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

/**
 * Initials avatar custom element with deterministic gradient color.
 */
export class RenopUserAvatar extends HTMLElement {
    /**
     * @returns {string[]}
     */
    static get observedAttributes() {
        return ['name'];
    }

    /**
     * Render when inserted into the DOM.
     * @returns {void}
     */
    connectedCallback() {
        this.render();
    }

    /**
     * Re-render when observed attributes change.
     * @returns {void}
     */
    attributeChangedCallback() {
        this.render();
    }

    /**
     * Derive initials and background color from the name attribute.
     * @returns {void}
     */
    render() {
        const name = this.getAttribute('name') || '';
        this.className = 'user-avatar';
        this.textContent = name.substring(0, 2).toUpperCase();

        const colors = [
            'linear-gradient(135deg, #6366f1, #a855f7)',
            'linear-gradient(135deg, #ec4899, #f43f5e)',
            'linear-gradient(135deg, #3b82f6, #06b6d4)',
            'linear-gradient(135deg, #10b981, #3b82f6)',
            'linear-gradient(135deg, #f59e0b, #e11d48)'
        ];
        let hash = 0;
        for (let i = 0; i < name.length; i++) {
            hash = name.charCodeAt(i) + ((hash << 5) - hash);
        }
        const colorIndex = Math.abs(hash) % colors.length;
        this.style.background = colors[colorIndex];
    }
}

if (!customElements.get('renop-user-avatar')) {
    customElements.define('renop-user-avatar', RenopUserAvatar);
}

/**
 * Create a user avatar element from a display name.
 * @param {string} name - User name used for initials and color hash.
 * @returns {HTMLElement}
 */
export function createUserAvatar(name) {
    const avatar = document.createElement('renop-user-avatar');
    avatar.setAttribute('name', name || '');
    return avatar;
}
