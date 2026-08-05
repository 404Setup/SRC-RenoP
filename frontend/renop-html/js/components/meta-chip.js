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
 * Label/value meta chip custom element.
 */
export class RenopMetaChip extends HTMLElement {
    /**
     * @returns {string[]}
     */
    static get observedAttributes() {
        return ['label', 'value', 'tone'];
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
     * Rebuild chip content from attributes.
     * @returns {void}
     */
    render() {
        const label = this.getAttribute('label') || '';
        const value = this.getAttribute('value') || '';
        const tone = this.getAttribute('tone') || '';

        this.className = 'mirror-chip' + (tone ? ` mirror-chip--${tone}` : '');
        this.innerHTML = '';

        const labelEl = el('span', {class: 'mirror-chip-label'}, label);
        const valueEl = el('span', {class: 'mirror-chip-value'}, value);
        this.appendChild(labelEl);
        this.appendChild(valueEl);
    }
}

if (!customElements.get('renop-meta-chip')) {
    customElements.define('renop-meta-chip', RenopMetaChip);
}

/**
 * Create a label/value meta chip element.
 * @param {string} label - Chip label text.
 * @param {string} value - Chip value text.
 * @param {string} [tone=''] - Optional tone modifier (e.g. yes, no).
 * @returns {HTMLElement}
 */
export function createMetaChip(label, value, tone = '') {
    const chip = document.createElement('renop-meta-chip');
    chip.setAttribute('label', label);
    chip.setAttribute('value', value);
    if (tone) chip.setAttribute('tone', tone);
    return chip;
}
