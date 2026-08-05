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
 * Single vertical bar for simple charts.
 */
export class RenopChartBar extends HTMLElement {
    /**
     * @returns {string[]}
     */
    static get observedAttributes() {
        return ['height-pct', 'title'];
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
     * @param {string} name - Attribute name.
     * @param {string|null} oldValue - Previous value.
     * @param {string|null} newValue - New value.
     * @returns {void}
     */
    attributeChangedCallback(name, oldValue, newValue) {
        if (oldValue !== newValue) {
            this.render();
        }
    }

    /**
     * Apply height and class from attributes.
     * @returns {void}
     */
    render() {
        const heightPct = this.getAttribute('height-pct') || '0';
        this.className = 'chart-bar';
        this.style.height = `${heightPct}%`;
    }
}

if (!customElements.get('renop-chart-bar')) {
    customElements.define('renop-chart-bar', RenopChartBar);
}

/**
 * Create a chart bar element with a given height percentage.
 * @param {number|string} heightPct - Bar height as a percentage (0–100).
 * @param {string} [title=''] - Optional title attribute/tooltip.
 * @returns {HTMLElement}
 */
export function createChartBar(heightPct, title = '') {
    const bar = document.createElement('renop-chart-bar');
    bar.setAttribute('height-pct', String(heightPct));
    if (title) bar.setAttribute('title', title);
    return bar;
}
