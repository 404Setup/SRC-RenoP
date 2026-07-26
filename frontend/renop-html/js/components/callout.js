/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {createIcon} from './icon.js';

/**
 * Inline callout banner custom element with icon and message.
 */
export class RenopCallout extends HTMLElement {
    /**
     * @returns {string[]}
     */
    static get observedAttributes() {
        return ['type', 'icon', 'message'];
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
     * Rebuild callout content from attributes.
     * @returns {void}
     */
    render() {
        const type = this.getAttribute('type') || 'info';
        const iconName = this.getAttribute('icon') || (type === 'danger' || type === 'warning' ? 'warning' : 'info');
        const message = this.getAttribute('message') || '';

        this.className = `cfg-callout cfg-callout--${type}`;
        this.innerHTML = '';

        const iconEl = createIcon(iconName, {class: 'cfg-callout-icon'});
        this.appendChild(iconEl);

        const textDiv = document.createElement('div');
        if (message) {
            textDiv.textContent = message;
        }
        this.appendChild(textDiv);
    }
}

if (!customElements.get('renop-callout')) {
    customElements.define('renop-callout', RenopCallout);
}

/**
 * Create a callout element for notices and warnings.
 * @param {string} [type='info'] - Visual tone (e.g. info, danger, warning).
 * @param {string|Node} [message=''] - Message text or a DOM node to append.
 * @param {string|null} [iconName=null] - Optional icon name override.
 * @returns {HTMLElement}
 */
export function createCallout(type = 'info', message = '', iconName = null) {
    const callout = document.createElement('renop-callout');
    callout.setAttribute('type', type);
    if (iconName) callout.setAttribute('icon', iconName);
    if (typeof message === 'string') {
        callout.setAttribute('message', message);
    } else if (message instanceof Node) {
        callout.appendChild(message);
    }
    return callout;
}
