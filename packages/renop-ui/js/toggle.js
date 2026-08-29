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
import {$} from './jquery.js';

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
        const input = $(this).find('input[type="checkbox"]').get(0);
        return input ? input.checked : this.hasAttribute('checked');
    }

    /**
     * Set the toggle checked state.
     * @param {boolean} val - Checked state.
     */
    set checked(val) {
        if (val) this.setAttribute('checked', '');
        else this.removeAttribute('checked');
        const input = $(this).find('input[type="checkbox"]').get(0);
        if (input) $(input).prop('checked', !!val);
    }

    /**
     * Build or sync the toggle markup and input state.
     * @returns {void}
     */
    render() {
        if (this._rendered) {
            const input = $(this).find('input[type="checkbox"]').get(0);
            if (input) {
                $(input).prop({
                    checked: this.hasAttribute('checked'),
                    disabled: this.hasAttribute('disabled'),
                });
            }
            return;
        }
        this._rendered = true;
        $(this).empty();
        const label = el('label', {class: 'cfg-toggle'});
        const input = el('input', {
            type: 'checkbox',
            checked: this.hasAttribute('checked'),
            disabled: this.hasAttribute('disabled'),
        });
        $(input).on('change', (e) => {
            e.stopPropagation();
            if (input.checked) this.setAttribute('checked', '');
            else this.removeAttribute('checked');
            this.dispatchEvent(new CustomEvent('change', {bubbles: true, detail: {checked: input.checked}}));
        });
        const track = el('span', {class: 'cfg-toggle-track'}, el('span', {class: 'cfg-toggle-thumb'}));
        $(label).append(input, track);
        $(this).append(label);
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
    const toggle = $('<renop-toggle>').get(0);
    if (checked) $(toggle).attr('checked', '');
    if (onChange) {
        $(toggle).on('change', (e) => {
            const detail = e.originalEvent?.detail || e.detail;
            if (detail && typeof detail.checked === 'boolean') {
                onChange(detail.checked);
            }
        });
    }
    return toggle;
}
