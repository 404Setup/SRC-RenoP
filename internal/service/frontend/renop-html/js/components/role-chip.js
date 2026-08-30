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
import {createIcon} from './icon.js';

/**
 * Selectable role/permission chip with checkbox semantics.
 */
export class RenopRoleChip extends HTMLElement {
    /**
     * @returns {string[]}
     */
    static get observedAttributes() {
        return ['value', 'tone', 'title-text', 'code-text', 'desc-text', 'checked', 'compact', 'for-id'];
    }

    /**
     * Whether the chip checkbox is checked.
     * @returns {boolean}
     */
    get checked() {
        const input = this.querySelector('input[type="checkbox"]');
        return input ? input.checked : this.hasAttribute('checked');
    }

    /**
     * Set the chip checked state.
     * @param {boolean} val - Checked state.
     */
    set checked(val) {
        if (val) this.setAttribute('checked', '');
        else this.removeAttribute('checked');
        this.syncCheckedState(!!val);
    }

    /**
     * Render when inserted into the DOM.
     * @returns {void}
     */
    connectedCallback() {
        this.render();
    }

    /**
     * Sync checked state or re-render on attribute changes.
     * @param {string} name - Attribute name.
     * @param {string|null} oldValue - Previous value.
     * @param {string|null} newValue - New value.
     * @returns {void}
     */
    attributeChangedCallback(name, oldValue, newValue) {
        if (oldValue === newValue) return;
        if (name === 'checked' && this.querySelector('input[type="checkbox"]')) {
            this.syncCheckedState(this.hasAttribute('checked'));
            return;
        }
        this.render();
    }

    /**
     * Sync the internal checkbox and CSS class with a checked flag.
     * @param {boolean} isChecked - Desired checked state.
     * @returns {void}
     */
    syncCheckedState(isChecked) {
        const input = this.querySelector('input[type="checkbox"]');
        if (input && input.checked !== isChecked) {
            input.checked = isChecked;
        }
        this.classList.toggle('is-checked', isChecked);
    }

    /**
     * Rebuild chip markup from attributes.
     * @returns {void}
     */
    render() {
        const value = this.getAttribute('value') || '';
        const tone = this.getAttribute('tone') || 'system';
        const titleText = this.getAttribute('title-text') || value;
        const codeText = this.getAttribute('code-text') || value;
        const descText = this.getAttribute('desc-text') || '';
        const isChecked = this.hasAttribute('checked');
        const isCompact = this.hasAttribute('compact');
        const forId = this.getAttribute('for-id') || ('role-' + value.replace(/[^a-zA-Z0-9_-]/g, '_'));

        this.className = `role-chip role-chip--${tone}${isCompact ? ' role-chip--compact' : ''}${isChecked ? ' is-checked' : ''}`;
        this.innerHTML = '';

        const label = el('label', {class: 'role-chip-label'});

        const input = el('input', {
            type: 'checkbox',
            id: forId,
            value,
            class: 'role-chip-input',
            checked: isChecked
        });

        input.addEventListener('change', () => {
            if (input.checked) {
                this.setAttribute('checked', '');
            } else {
                this.removeAttribute('checked');
            }
            this.classList.toggle('is-checked', input.checked);
            this.dispatchEvent(new CustomEvent('change', {bubbles: true, detail: {checked: input.checked, value}}));
        });

        const check = el('span', {class: 'role-chip-check', 'aria-hidden': 'true'}, createIcon('check', {
            width: '12',
            height: '12'
        }));
        const body = el('span', {class: 'role-chip-body'},
            el('span', {class: 'role-chip-title'}, titleText),
            el('span', {class: 'role-chip-code'}, codeText)
        );

        if (descText) {
            body.appendChild(el('span', {class: 'role-chip-desc'}, descText));
        }

        label.appendChild(input);
        label.appendChild(check);
        label.appendChild(body);
        this.appendChild(label);
    }
}

if (!customElements.get('renop-role-chip')) {
    customElements.define('renop-role-chip', RenopRoleChip);
}

/**
 * Create a role/permission selection chip.
 * @param {string} value - Chip value / permission code.
 * @param {object} [options={}] - Chip configuration.
 * @param {string} [options.title] - Display title.
 * @param {string} [options.desc] - Optional description.
 * @param {string} [options.tone='system'] - Visual tone (system, view, update, …).
 * @param {string} [options.code] - Code text shown under the title.
 * @param {boolean} [options.checked=false] - Initial checked state.
 * @param {boolean} [options.compact=false] - Compact layout.
 * @param {function(boolean, string): void} [options.onChange] - Change handler (checked, value).
 * @returns {HTMLElement}
 */
export function createRoleChip(value, {
    title,
    desc,
    tone = 'system',
    code,
    checked = false,
    compact = false,
    onChange
} = {}) {
    const chip = document.createElement('renop-role-chip');
    chip.setAttribute('value', value);
    if (tone) chip.setAttribute('tone', tone);
    if (title) chip.setAttribute('title-text', title);
    if (code) chip.setAttribute('code-text', code);
    if (desc) chip.setAttribute('desc-text', desc);
    if (checked) chip.setAttribute('checked', '');
    if (compact) chip.setAttribute('compact', '');
    if (onChange) {
        chip.addEventListener('change', (e) => onChange(e.detail.checked, e.detail.value));
    }
    return chip;
}
