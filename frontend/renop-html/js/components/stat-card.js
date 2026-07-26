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
 * Dashboard statistic card custom element.
 */
export class RenopStatCard extends HTMLElement {
    /**
     * @returns {string[]}
     */
    static get observedAttributes() {
        return ['icon', 'label', 'value', 'subtext', 'variant', 'value-id'];
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
     * Rebuild stat card content from attributes.
     * @returns {void}
     */
    render() {
        const icon = this.getAttribute('icon');
        const label = this.getAttribute('label') || '';
        const value = this.getAttribute('value') || '-';
        const subtext = this.getAttribute('subtext') || '';
        const variant = this.getAttribute('variant') || 'primary';
        const valueId = this.getAttribute('value-id');

        this.className = `cfg-stat-card cfg-stat-card--${variant}`;
        this.innerHTML = '';

        if (icon) {
            const iconDiv = el('div', {class: 'cfg-stat-icon'}, createIcon(icon));
            this.appendChild(iconDiv);
        }

        const content = el('div', {class: 'cfg-stat-content'});
        const labelEl = el('div', {class: 'cfg-stat-label'}, label);
        const valueEl = el('div', {class: 'cfg-stat-value'}, value);
        if (valueId) {
            valueEl.id = valueId;
        }
        content.appendChild(labelEl);
        content.appendChild(valueEl);

        if (subtext) {
            const subEl = el('div', {class: 'cfg-stat-subtext'}, subtext);
            content.appendChild(subEl);
        }

        this.appendChild(content);
    }
}

if (!customElements.get('renop-stat-card')) {
    customElements.define('renop-stat-card', RenopStatCard);
}

/**
 * Create a statistic card element.
 * @param {object} [props={}] - Card configuration.
 * @param {string} [props.icon] - Icon name.
 * @param {string} [props.label] - Stat label.
 * @param {string} [props.value] - Stat value text.
 * @param {string} [props.subtext] - Optional subtext.
 * @param {string} [props.variant] - Visual variant (e.g. primary).
 * @param {string} [props.valueId] - Id applied to the value element.
 * @returns {HTMLElement}
 */
export function createStatCard(props = {}) {
    const card = document.createElement('renop-stat-card');
    if (props.icon) card.setAttribute('icon', props.icon);
    if (props.label) card.setAttribute('label', props.label);
    if (props.value) card.setAttribute('value', props.value);
    if (props.subtext) card.setAttribute('subtext', props.subtext);
    if (props.variant) card.setAttribute('variant', props.variant);
    if (props.valueId) card.setAttribute('value-id', props.valueId);
    return card;
}
