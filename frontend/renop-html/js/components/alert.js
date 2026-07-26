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
 * Dismissible alert banner custom element.
 */
export class RenopAlert extends HTMLElement {
    /**
     * @returns {string[]}
     */
    static get observedAttributes() {
        return ['type', 'message'];
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
     * @param {string} name - Attribute name.
     * @param {string|null} oldValue - Previous value.
     * @param {string|null} newValue - New value.
     * @returns {void}
     */
    attributeChangedCallback(name, oldValue, newValue) {
        if (oldValue !== newValue) {
            this.render();
        }
    }

    /**
     * Rebuild alert content from attributes.
     * @returns {void}
     */
    render() {
        const type = this.getAttribute('type') || 'info';
        const message = this.getAttribute('message') || '';

        this.className = `alert alert-${type}`;
        this.innerHTML = '';

        const textSpan = el('span', {}, message);
        const closeBtn = el('button', {class: 'alert-close', ariaLabel: t('modal.close') || 'Close'});
        closeBtn.appendChild(createIcon('close'));
        closeBtn.onclick = () => this.dismiss();

        this.appendChild(textSpan);
        this.appendChild(closeBtn);
    }

    /**
     * Animate out and remove the alert from the DOM.
     * @returns {void}
     */
    dismiss() {
        this.classList.add('alert-leaving');
        setTimeout(() => {
            if (this.parentNode) {
                this.parentNode.removeChild(this);
            }
        }, 300);
    }
}

if (!customElements.get('renop-alert')) {
    customElements.define('renop-alert', RenopAlert);
}

/**
 * Create a dismissible alert element.
 * @param {string} message - Alert message text.
 * @param {string} [type='info'] - Visual tone (e.g. info, success, warning, danger).
 * @returns {HTMLElement}
 */
export function createAlert(message, type = 'info') {
    const alertEl = document.createElement('renop-alert');
    alertEl.setAttribute('type', type);
    alertEl.setAttribute('message', message);
    return alertEl;
}
