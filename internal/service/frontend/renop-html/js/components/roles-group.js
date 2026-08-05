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

/**
 * Grouped roles section with title and subtitle header.
 */
export class RenopRolesGroup extends HTMLElement {
    /**
     * @returns {string[]}
     */
    static get observedAttributes() {
        return ['title', 'subtitle'];
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
     * Rebuild the group header from attributes.
     * @returns {void}
     */
    render() {
        const titleText = this.getAttribute('title') || '';
        const subtitleText = this.getAttribute('subtitle') || '';

        this.className = 'roles-group';
        let header = this.querySelector('.roles-group-header');
        if (!header) {
            header = el('div', {class: 'roles-group-header'});
            this.prepend(header);
        }
        header.innerHTML = '';
        header.appendChild(el('h4', {class: 'roles-group-title'}, titleText));
        if (subtitleText) {
            header.appendChild(el('p', {class: 'roles-group-subtitle'}, subtitleText));
        }
    }
}

if (!customElements.get('renop-roles-group')) {
    customElements.define('renop-roles-group', RenopRolesGroup);
}

/**
 * Create a roles group container with header text.
 * @param {string} title - Group title.
 * @param {string} [subtitle] - Optional group subtitle.
 * @returns {HTMLElement}
 */
export function createRolesGroup(title, subtitle) {
    const group = document.createElement('renop-roles-group');
    if (title) group.setAttribute('title', title);
    if (subtitle) group.setAttribute('subtitle', subtitle);
    return group;
}
