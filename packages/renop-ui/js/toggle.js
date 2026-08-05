/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {el} from './dom.js';

/**
 * Boolean toggle switch custom element.
 */
export class RenopToggle extends HTMLElement {
    /**
     * @returns {string[]}
     */
    static get observedAttributes() {
        return ['checked', 'disabled'];
    }

    /**
     * Render when inserted into the DOM.
     * @returns {void}
     */
    connectedCallback() {
        this.render();
    }

    /**
     * Re-render or sync state when observed attributes change.
     * @returns {void}
     */
    attributeChangedCallback() {
        this.render();
    }

    /**
     * Whether the toggle is checked.
     * @returns {boolean}
     */
    get checked() {
        const input = this.querySelector('input[type="checkbox"]');
        return input ? input.checked : this.hasAttribute('checked');
    }

    /**
     * Set the toggle checked state.
     * @param {boolean} val - Checked state.
     */
    set checked(val) {
        if (val) this.setAttribute('checked', '');
        else this.removeAttribute('checked');
        const input = this.querySelector('input[type="checkbox"]');
        if (input) input.checked = !!val;
    }

    /**
     * Build or sync the toggle markup and input state.
     * @returns {void}
     */
    render() {
        if (this._rendered) {
            const input = this.querySelector('input[type="checkbox"]');
            if (input) {
                input.checked = this.hasAttribute('checked');
                input.disabled = this.hasAttribute('disabled');
            }
            return;
        }
        this._rendered = true;
        this.innerHTML = '';
        const label = el('label', {class: 'cfg-toggle'});
        const input = el('input', {
            type: 'checkbox',
            checked: this.hasAttribute('checked'),
            disabled: this.hasAttribute('disabled'),
        });
        input.addEventListener('change', (e) => {
            e.stopPropagation();
            if (input.checked) this.setAttribute('checked', '');
            else this.removeAttribute('checked');
            this.dispatchEvent(new CustomEvent('change', {bubbles: true, detail: {checked: input.checked}}));
        });
        const track = el('span', {class: 'cfg-toggle-track'}, el('span', {class: 'cfg-toggle-thumb'}));
        label.appendChild(input);
        label.appendChild(track);
        this.appendChild(label);
    }
}

if (typeof customElements !== 'undefined' && !customElements.get('renop-toggle')) {
    customElements.define('renop-toggle', RenopToggle);
}

/**
 * Create a toggle switch element.
 * @param {boolean} [checked=false] - Initial checked state.
 * @param {function(boolean): void|null} [onChange=null] - Change handler receiving checked state.
 * @returns {HTMLElement}
 */
export function createToggle(checked = false, onChange = null) {
    const toggle = document.createElement('renop-toggle');
    if (checked) toggle.setAttribute('checked', '');
    if (onChange) {
        toggle.addEventListener('change', (e) => {
            if (e.detail && typeof e.detail.checked === 'boolean') {
                onChange(e.detail.checked);
            }
        });
    }
    return toggle;
}
