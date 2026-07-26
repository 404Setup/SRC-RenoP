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
 * Empty list / no-data placeholder custom element.
 */
export class RenopEmptyState extends HTMLElement {
    /**
     * @returns {string[]}
     */
    static get observedAttributes() {
        return ['message', 'icon', 'subtext'];
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
     * Rebuild empty-state content from attributes.
     * @returns {void}
     */
    render() {
        const message = this.getAttribute('message') || '';
        const icon = this.getAttribute('icon');
        const subtext = this.getAttribute('subtext');

        this.className = 'renop-empty-state';
        this.style.textAlign = 'center';
        this.style.padding = '3rem 1rem';
        this.style.opacity = '0.55';
        this.style.fontSize = '0.9rem';
        this.innerHTML = '';

        if (icon) {
            const iconWrap = el('div', {class: 'empty-state-icon', style: {marginBottom: '0.5rem'}}, createIcon(icon));
            this.appendChild(iconWrap);
        }
        if (message) {
            this.appendChild(el('div', {class: 'empty-state-message'}, message));
        }
        if (subtext) {
            this.appendChild(el('div', {
                class: 'empty-state-subtext',
                style: {fontSize: '0.8rem', opacity: '0.8', marginTop: '0.25rem'}
            }, subtext));
        }
    }
}

if (!customElements.get('renop-empty-state')) {
    customElements.define('renop-empty-state', RenopEmptyState);
}

/**
 * Create an empty-state placeholder element.
 * @param {string|object} [props={}] - Message string or options object.
 * @param {string} [props.message] - Primary message text.
 * @param {string} [props.icon] - Icon name.
 * @param {string} [props.subtext] - Secondary text.
 * @returns {HTMLElement}
 */
export function createEmptyState(props = {}) {
    const emptyState = document.createElement('renop-empty-state');
    if (typeof props === 'string') {
        emptyState.setAttribute('message', props);
    } else {
        if (props.message) emptyState.setAttribute('message', props.message);
        if (props.icon) emptyState.setAttribute('icon', props.icon);
        if (props.subtext) emptyState.setAttribute('subtext', props.subtext);
    }
    return emptyState;
}
