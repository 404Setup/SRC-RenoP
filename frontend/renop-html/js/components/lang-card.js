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

/**
 * Language picker card custom element.
 */
export class RenopLangCard extends HTMLElement {
    /**
     * @returns {string[]}
     */
    static get observedAttributes() {
        return ['code', 'name', 'sub', 'active'];
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
     * Rebuild language card content from attributes.
     * @returns {void}
     */
    render() {
        const code = this.getAttribute('code') || '';
        const name = this.getAttribute('name') || '';
        const sub = this.getAttribute('sub') || '';
        const isActive = this.hasAttribute('active');
        const shortCode = (code.substring(0, 2) || '').toUpperCase();

        this.className = `lang-card ${isActive ? 'active' : ''}`;
        this.setAttribute('data-lang', code);
        this.setAttribute('role', 'button');
        this.setAttribute('tabindex', '0');
        this.innerHTML = '';

        const left = el('div', {class: 'lang-card-left'},
            el('span', {class: 'lang-code-badge'}, shortCode),
            el('div', {class: 'lang-card-info'},
                el('span', {class: 'lang-card-name'}, name),
                el('span', {class: 'lang-card-sub'}, sub)
            )
        );

        const checkSvg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
        checkSvg.setAttribute('class', 'lang-card-check');
        checkSvg.setAttribute('viewBox', '0 0 24 24');
        checkSvg.setAttribute('fill', 'none');
        checkSvg.setAttribute('stroke', 'currentColor');
        checkSvg.setAttribute('stroke-width', '2.5');
        checkSvg.setAttribute('stroke-linecap', 'round');
        checkSvg.setAttribute('stroke-linejoin', 'round');
        const polyline = document.createElementNS('http://www.w3.org/2000/svg', 'polyline');
        polyline.setAttribute('points', '20 6 9 17 4 12');
        checkSvg.appendChild(polyline);

        this.appendChild(left);
        this.appendChild(checkSvg);
    }
}

if (!customElements.get('renop-lang-card')) {
    customElements.define('renop-lang-card', RenopLangCard);
}

/**
 * Create a language selection card.
 * @param {object} options - Card configuration.
 * @param {string} options.code - Locale code (e.g. en-US).
 * @param {string} [options.name] - Display language name.
 * @param {string} [options.sub] - Secondary subtitle text.
 * @param {boolean} [options.active] - Whether the card is selected.
 * @param {Function} [options.onClick] - Click/keyboard activation handler.
 * @returns {HTMLElement}
 */
export function createLangCard({code, name, sub, active, onClick}) {
    const card = document.createElement('renop-lang-card');
    card.setAttribute('code', code);
    if (name) card.setAttribute('name', name);
    if (sub) card.setAttribute('sub', sub);
    if (active) card.setAttribute('active', '');
    if (onClick) {
        card.addEventListener('click', onClick);
        card.addEventListener('keydown', (e) => {
            if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                onClick(e);
            }
        });
    }
    return card;
}
