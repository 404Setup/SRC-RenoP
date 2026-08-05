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
import {createIcon, ICONS} from './icon.js';

/**
 * Settings section with icon header and a fields container.
 */
export class RenopSection extends HTMLElement {
    /**
     * @returns {string[]}
     */
    static get observedAttributes() {
        return ['icon', 'title', 'subtitle'];
    }

    /**
     * Render when inserted into the DOM.
     * @returns {void}
     */
    connectedCallback() {
        this.render();
    }

    /**
     * Rebuild section header and empty fields container.
     * @returns {void}
     */
    render() {
        const icon = this.getAttribute('icon');
        const title = this.getAttribute('title') || '';
        const subtitle = this.getAttribute('subtitle') || '';

        this.className = 'cfg-section';
        this.innerHTML = '';

        const header = el('div', {class: 'cfg-section-header'});
        const iconDiv = el('div', {class: 'cfg-section-icon'});
        if (icon) {
            if (ICONS[icon]) {
                iconDiv.appendChild(createIcon(icon));
            } else {
                iconDiv.innerHTML = icon;
            }
        }
        header.appendChild(iconDiv);

        const metaDiv = el('div', {class: 'cfg-section-meta'});
        metaDiv.appendChild(el('h3', {class: 'cfg-section-title'}, title));
        if (subtitle) {
            metaDiv.appendChild(el('p', {class: 'cfg-section-subtitle'}, subtitle));
        }
        header.appendChild(metaDiv);

        const fieldsDiv = el('div', {class: 'cfg-fields'});
        this.appendChild(header);
        this.appendChild(fieldsDiv);
    }

    /**
     * The fields container element (or the host if missing).
     * @returns {Element}
     */
    get fields() {
        return this.querySelector('.cfg-fields') || this;
    }
}

if (!customElements.get('renop-section')) {
    customElements.define('renop-section', RenopSection);
}
