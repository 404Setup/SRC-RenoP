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
 * Navigation tab custom element.
 */
export class RenopTab extends HTMLElement {
    /**
     * @returns {string[]}
     */
    static get observedAttributes() {
        return ['text', 'active', 'loading'];
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
     * Apply tab text and active/loading classes.
     * @returns {void}
     */
    render() {
        const text = this.getAttribute('text') || '';
        const active = this.hasAttribute('active');
        const loading = this.hasAttribute('loading');

        this.className = 'tab' + (active ? ' active' : '') + (loading ? ' is-loading' : '');
        this.textContent = text;
    }
}

if (!customElements.get('renop-tab')) {
    customElements.define('renop-tab', RenopTab);
}

/**
 * Create a navigation tab link.
 * @param {string} text - Tab label.
 * @param {object} [options={}] - Tab configuration.
 * @param {string} [options.href='#'] - Link href.
 * @param {boolean} [options.active=false] - Active state.
 * @param {boolean} [options.loading=false] - Loading state.
 * @param {Function} [options.onClick] - Click handler.
 * @returns {HTMLElement}
 */
export function createTab(text, {href = '#', active = false, loading = false, onClick} = {}) {
    const tab = el('a', {
        href,
        class: 'tab' + (active ? ' active' : '') + (loading ? ' is-loading' : '')
    }, text);
    if (onClick) tab.onclick = onClick;
    return tab;
}

/**
 * Create a breadcrumb path separator element.
 * @param {number} [index=0] - Segment index used for animation CSS variables.
 * @returns {HTMLElement}
 */
export function createBreadcrumbSep(index = 0) {
    const sep = document.createElement('span');
    sep.className = 'breadcrumb-sep';
    sep.setAttribute('aria-hidden', 'true');
    sep.textContent = '/';
    sep.style.setProperty('--seg-index', String(index));
    return sep;
}

/**
 * Create a breadcrumb segment link.
 * @param {object} options - Segment configuration.
 * @param {string} options.href - Link href.
 * @param {string} options.text - Visible label.
 * @param {boolean} [options.isCurrent] - Marks the current page segment.
 * @param {string} [options.i18nKey] - Optional data-i18n key.
 * @param {number} [options.index=0] - Segment index for animation CSS variables.
 * @param {Function} [options.onClick] - Click handler.
 * @returns {HTMLElement}
 */
export function createBreadcrumbLink({href, text, isCurrent, i18nKey, index = 0, onClick}) {
    const link = document.createElement('a');
    link.href = href;
    link.className = 'breadcrumb-seg' + (isCurrent ? ' is-current' : '');
    link.textContent = text;
    if (i18nKey) link.setAttribute('data-i18n', i18nKey);
    link.title = href;
    link.style.setProperty('--seg-index', String(index));
    if (isCurrent) link.setAttribute('aria-current', 'page');
    if (onClick) link.addEventListener('click', onClick);
    return link;
}
