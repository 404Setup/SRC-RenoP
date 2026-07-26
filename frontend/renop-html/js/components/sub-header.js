/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {el} from '../cfg-ui.js';
import {createIcon} from './icon.js';

/**
 * Subsection header with icon, title, and optional action slot.
 */
export class RenopSubHeader extends HTMLElement {
    /**
     * @returns {string[]}
     */
    static get observedAttributes() {
        return ['icon', 'title'];
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
     * Rebuild header left content while preserving non-left children.
     * @returns {void}
     */
    render() {
        const icon = this.getAttribute('icon');
        const titleText = this.getAttribute('title') || '';

        const extraChildren = Array.from(this.childNodes).filter(node =>
            !(node.nodeType === 1 && node.classList.contains('cfg-sub-header-left'))
        );

        this.className = 'cfg-sub-header';
        this.innerHTML = '';

        const left = el('div', {class: 'cfg-sub-header-left'});
        if (icon) {
            const iconDiv = el('div', {class: 'cfg-sub-header-icon'}, createIcon(icon, {width: '16', height: '16'}));
            left.appendChild(iconDiv);
        }
        if (titleText) {
            left.appendChild(el('span', {class: 'cfg-sub-header-title'}, titleText));
        }

        this.appendChild(left);
        extraChildren.forEach(child => this.appendChild(child));
    }
}

if (!customElements.get('renop-sub-header')) {
    customElements.define('renop-sub-header', RenopSubHeader);
}

/**
 * Create a subsection header element.
 * @param {string} icon - Icon name.
 * @param {string} titleText - Header title text.
 * @param {HTMLElement|null} [actionEl=null] - Optional action element appended as a child.
 * @returns {HTMLElement}
 */
export function createSubHeader(icon, titleText, actionEl = null) {
    const subHeader = document.createElement('renop-sub-header');
    if (icon) subHeader.setAttribute('icon', icon);
    if (titleText) subHeader.setAttribute('title', titleText);
    if (actionEl) subHeader.appendChild(actionEl);
    return subHeader;
}
