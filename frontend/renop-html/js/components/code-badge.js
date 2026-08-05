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

/**
 * Monospace code snippet badge custom element.
 */
export class RenopCodeBadge extends HTMLElement {
    /**
     * @returns {string[]}
     */
    static get observedAttributes() {
        return ['code'];
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
     * Rebuild the code badge content from attributes.
     * @returns {void}
     */
    render() {
        const code = this.getAttribute('code') || this.textContent;
        this.innerHTML = '';
        const codeEl = el('code', {
            style: {
                fontFamily: 'monospace',
                background: 'var(--bg-color)',
                padding: '0.2rem 0.4rem',
                borderRadius: '4px',
                border: '1px solid var(--border-color)',
                fontSize: '0.85rem'
            }
        }, code);
        this.appendChild(codeEl);
    }
}

if (!customElements.get('renop-code-badge')) {
    customElements.define('renop-code-badge', RenopCodeBadge);
}

/**
 * Create a monospace code badge element.
 * @param {string} code - Code or token text to display.
 * @returns {HTMLElement}
 */
export function createCodeBadge(code) {
    const badge = document.createElement('renop-code-badge');
    badge.setAttribute('code', code);
    return badge;
}
