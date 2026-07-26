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
 * Key/value meta grid host custom element for modals.
 */
export class RenopMetaGrid extends HTMLElement {
    /**
     * Ensure the meta-grid base class is applied when connected.
     * @returns {void}
     */
    connectedCallback() {
        if (!this.classList.contains('modal-meta-grid')) {
            this.classList.add('modal-meta-grid');
        }
    }
}

if (!customElements.get('renop-meta-grid')) {
    customElements.define('renop-meta-grid', RenopMetaGrid);
}

/**
 * Create a meta grid of labeled rows.
 * @param {Array<{label: string, value: string, isCode?: boolean, colon?: boolean}>} [items=[]] - Grid items.
 * @returns {HTMLElement}
 */
export function createMetaGrid(items = []) {
    const grid = document.createElement('renop-meta-grid');
    items.forEach(item => {
        if (!item) return;
        const row = el('div');
        const strong = el('strong', {class: 'badge-muted'}, item.label + (item.colon !== false ? '：' : ''));
        row.appendChild(strong);
        if (item.isCode) {
            row.appendChild(el('code', {class: 'code-badge'}, item.value));
        } else {
            row.appendChild(el('span', {}, item.value));
        }
        grid.appendChild(row);
    });
    return grid;
}
