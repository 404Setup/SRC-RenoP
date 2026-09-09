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
import {createToggle} from './toggle.js';

let fieldLabelSequence = 0;

/**
 * Settings form field row with label, hint, and control slot.
 */
export class RenopFieldRow extends HTMLElement {
    /**
     * @returns {string[]}
     */
    static get observedAttributes() {
        return ['label', 'hint', 'modifier'];
    }

    /**
     * Render when inserted into the DOM.
     * @returns {void}
     */
    connectedCallback() {
        this.render();
        queueMicrotask(() => { if (this.isConnected) this.render(); });
    }

    /**
     * Re-render when observed attributes change.
     * @returns {void}
     */
    attributeChangedCallback() {
        this.render();
    }

    /**
     * Rebuild label/hint layout while preserving the control slot.
     * @returns {void}
     */
    render() {
        const labelText = this.getAttribute('label') || '';
        const hintText = this.getAttribute('hint') || '';
        const modifier = this.getAttribute('modifier') || '';
        const labelID = this._labelID ||= `cfg-field-label-${++fieldLabelSequence}`;

        this.className = `cfg-field-row ${modifier}`.trim();
        let labelWrap = this.querySelector('.cfg-field-label');
        let controlWrap = this.querySelector('.cfg-field-control');

        if (!labelWrap) {
            labelWrap = el('div', {class: 'cfg-field-label'});
            this.prepend(labelWrap);
        }
        labelWrap.innerHTML = '';
        labelWrap.appendChild(el('span', {class: 'cfg-label-text', id: labelID}, labelText));
        if (hintText) labelWrap.appendChild(el('span', {class: 'cfg-label-hint', id: `${labelID}-hint`}, hintText));

        if (!controlWrap) {
            controlWrap = el('div', {class: 'cfg-field-control'});
            this.appendChild(controlWrap);
        }
        for (const control of controlWrap.querySelectorAll('input, textarea, select, button[aria-haspopup="listbox"]')) {
            if (control.closest('renop-field-row') !== this) continue;
            if (labelText && !control.hasAttribute('aria-label') && !control.hasAttribute('aria-labelledby') &&
                ![...(control.labels || [])].some(label => label.textContent.trim())) {
                control.setAttribute('aria-labelledby', labelID);
            }
            if (hintText && !control.hasAttribute('aria-describedby')) control.setAttribute('aria-describedby', `${labelID}-hint`);
        }
    }
}

if (!customElements.get('renop-field-row')) {
    customElements.define('renop-field-row', RenopFieldRow);
}

/**
 * Create a labeled field row with an optional control element.
 * @param {string} label - Field label text.
 * @param {string} hint - Secondary hint under the label.
 * @param {HTMLElement|null} control - Control element placed in the control slot.
 * @param {string} [modifierClass=''] - Extra modifier class on the row.
 * @returns {HTMLElement}
 */
export function createFieldRow(label, hint, control, modifierClass = '') {
    const row = document.createElement('renop-field-row');
    if (label) row.setAttribute('label', label);
    if (hint) row.setAttribute('hint', hint);
    if (modifierClass) row.setAttribute('modifier', modifierClass);
    if (control) {
        let controlWrap = row.querySelector('.cfg-field-control');
        if (!controlWrap) {
            controlWrap = el('div', {class: 'cfg-field-control'});
            row.appendChild(controlWrap);
        }
        controlWrap.appendChild(control);
    }
    return row;
}

/**
 * Create a field row containing a toggle control.
 * @param {string} label - Field label text.
 * @param {string} hint - Secondary hint under the label.
 * @param {boolean} checked - Initial toggle state.
 * @param {function(boolean): void} onChange - Toggle change handler.
 * @returns {HTMLElement}
 */
export function createToggleRow(label, hint, checked, onChange) {
    const toggle = createToggle(checked, onChange);
    return createFieldRow(label, hint, toggle, 'cfg-field-row--toggle');
}
