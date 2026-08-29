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
import {$} from './jquery.js';

/**
 * Short badge label from a locale code (e.g. `zh-CN` → `ZH`).
 * @param {string} code
 * @returns {string}
 */
export function langShortCode(code) {
    return (String(code || '').split('-')[0] || String(code || '')).slice(0, 3).toUpperCase();
}

/**
 * Inline checkmark SVG used on language cards.
 * @returns {SVGSVGElement}
 */
function createLangCardCheckSvg() {
    const checkSvg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    checkSvg.setAttribute('class', 'lang-card-check');
    checkSvg.setAttribute('viewBox', '0 0 24 24');
    checkSvg.setAttribute('fill', 'none');
    checkSvg.setAttribute('stroke', 'currentColor');
    checkSvg.setAttribute('stroke-width', '2.5');
    checkSvg.setAttribute('stroke-linecap', 'round');
    checkSvg.setAttribute('stroke-linejoin', 'round');
    checkSvg.setAttribute('aria-hidden', 'true');
    const polyline = document.createElementNS('http://www.w3.org/2000/svg', 'polyline');
    polyline.setAttribute('points', '20 6 9 17 4 12');
    checkSvg.appendChild(polyline);
    return checkSvg;
}

/**
 * Build left cluster + check icon for a language card host.
 * @param {HTMLElement} host
 * @param {{ code: string, name?: string, sub?: string }} opts
 * @returns {void}
 */
export function fillLangCardBody(host, {code, name, sub}) {
    $(host).empty();

    const shortCode = langShortCode(code);
    const left = el('div', {class: 'lang-card-left'},
        el('span', {class: 'lang-code-badge'}, shortCode),
        el('div', {class: 'lang-card-info'},
            el('span', {class: 'lang-card-name'}, name || code),
            el('span', {class: 'lang-card-sub'}, sub || ''),
        ),
    );

    $(host).append(left, createLangCardCheckSvg());
}

/**
 * Wire click + keyboard activation (for non-button hosts).
 * @param {HTMLElement} card
 * @param {((ev: Event) => void)|null|undefined} onClick
 * @returns {void}
 */
function bindLangCardActivate(card, onClick) {
    if (!onClick) return;
    $(card).on('click', onClick);
    if (card.tagName !== 'BUTTON') {
        $(card).on('keydown', (e) => {
            if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                onClick(e);
            }
        });
    }
}

/**
 * Create a language picker card (`<button class="lang-card">`).
 * Preferred factory for both web and frontend.
 *
 * @param {object} opts
 * @param {string} opts.code - Locale code (e.g. `en-US`).
 * @param {string} [opts.name] - Primary display name.
 * @param {string} [opts.sub] - Secondary subtitle.
 * @param {boolean} [opts.active] - Selected state (`active` class).
 * @param {(ev: Event) => void} [opts.onClick] - Activation handler.
 * @returns {HTMLButtonElement}
 */
export function createLangCard({code, name, sub, active, onClick} = {}) {
    const card = el('button', {
        type: 'button',
        class: `lang-card${active ? ' active' : ''}`,
        'data-lang': code || '',
    });
    fillLangCardBody(card, {code: code || '', name, sub});
    bindLangCardActivate(card, onClick);
    return card;
}

/**
 * Language picker card custom element (attribute-driven).
 * Registered as `renop-lang-card`. Prefer {@link createLangCard} for new code.
 */
export class RenopLangCard extends HTMLElement {
    /**
     * @returns {string[]}
     */
    static get observedAttributes() {
        return ['code', 'name', 'sub', 'active'];
    }

    /**
     * @returns {void}
     */
    connectedCallback() {
        this.render();
    }

    /**
     * @returns {void}
     */
    attributeChangedCallback() {
        if (this.isConnected) this.render();
    }

    /**
     * Rebuild content from attributes.
     * @returns {void}
     */
    render() {
        const code = this.getAttribute('code') || '';
        const name = this.getAttribute('name') || '';
        const sub = this.getAttribute('sub') || '';
        const isActive = this.hasAttribute('active');

        this.className = `lang-card${isActive ? ' active' : ''}`;
        this.setAttribute('data-lang', code);
        this.setAttribute('role', 'button');
        this.setAttribute('tabindex', '0');
        fillLangCardBody(this, {code, name, sub});
    }
}

/**
 * Register `renop-lang-card` if not already defined.
 * @returns {void}
 */
export function defineLangCard() {
    if (typeof customElements === 'undefined') return;
    if (!customElements.get('renop-lang-card')) {
        customElements.define('renop-lang-card', RenopLangCard);
    }
}

defineLangCard();
