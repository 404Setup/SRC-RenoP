/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {t} from '../i18n.js';
import {el} from '../cfg-ui.js';
import {createIcon} from './icon.js';

/**
 * Compact selected-file summary card with optional remove action.
 */
export class RenopFileCard extends HTMLElement {
    /**
     * @returns {string[]}
     */
    static get observedAttributes() {
        return ['filename', 'filesize', 'icon', 'removable'];
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
     * Rebuild card content from attributes.
     * @returns {void}
     */
    render() {
        const filename = this.getAttribute('filename') || '';
        const filesize = this.getAttribute('filesize') || '';
        const iconName = this.getAttribute('icon') || 'box';
        const removable = this.getAttribute('removable') !== 'false';

        this.className = 'file-card-item';
        this.style.display = 'flex';
        this.style.alignItems = 'center';
        this.style.justifyContent = 'space-between';
        this.style.width = '100%';
        this.innerHTML = '';

        const main = el('div', {class: 'file-info-main'},
            createIcon(iconName, {class: 'file-info-icon'}),
            el('div', {class: 'file-info-details'},
                el('span', {class: 'file-info-name'}, filename),
                el('span', {class: 'file-info-size'}, filesize)
            )
        );
        this.appendChild(main);

        if (removable) {
            const removeBtn = el('button', {
                type: 'button',
                class: 'file-info-remove',
                ariaLabel: t('common.remove') || 'Remove'
            }, '×');
            removeBtn.onclick = (e) => {
                e.stopPropagation();
                this.dispatchEvent(new CustomEvent('remove', {bubbles: true}));
            };
            this.appendChild(removeBtn);
        }
    }
}

if (!customElements.get('renop-file-card')) {
    customElements.define('renop-file-card', RenopFileCard);
}

/**
 * Create a file summary card element.
 * @param {string} filename - Display file name.
 * @param {string} filesize - Formatted size string.
 * @param {object} [options={}] - Extra options.
 * @param {string} [options.icon='box'] - Icon name.
 * @param {Function} [options.onRemove] - Handler for the remove event.
 * @returns {HTMLElement}
 */
export function createFileCard(filename, filesize, {icon = 'box', onRemove} = {}) {
    const card = document.createElement('renop-file-card');
    card.setAttribute('filename', filename);
    card.setAttribute('filesize', filesize);
    card.setAttribute('icon', icon);
    if (onRemove) card.addEventListener('remove', onRemove);
    return card;
}
