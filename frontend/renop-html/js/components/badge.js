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
 * Permission or status badge custom element.
 */
export class RenopBadge extends HTMLElement {
    /**
     * @returns {string[]}
     */
    static get observedAttributes() {
        return ['type', 'text'];
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
     * Rebuild badge content from attributes.
     * @returns {void}
     */
    render() {
        const type = this.getAttribute('type') || 'none';
        const text = this.getAttribute('text') || this.textContent;
        this.className = `permission-badge badge-${type}`;
        this.textContent = text;
    }
}

if (!customElements.get('renop-badge')) {
    customElements.define('renop-badge', RenopBadge);
}

/**
 * Create a badge element for labels and status chips.
 * @param {string} text - Badge label text.
 * @param {string} [type='none'] - Visual tone (e.g. admin, none).
 * @param {object} [props={}] - Extra element props passed to the host.
 * @param {string} [props.title] - Title tooltip.
 * @param {string} [props.class] - Additional CSS class names.
 * @returns {HTMLElement}
 */
export function createBadge(text, type = 'none', props = {}) {
    const badge = document.createElement('renop-badge');
    badge.setAttribute('type', type);
    badge.setAttribute('text', text);
    if (props.title) badge.title = props.title;
    if (props.class) badge.classList.add(...props.class.split(' '));
    return badge;
}
