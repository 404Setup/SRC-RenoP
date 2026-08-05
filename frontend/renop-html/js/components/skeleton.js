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
 * Loading skeleton placeholder custom element.
 */
export class RenopSkeleton extends HTMLElement {
    /**
     * @returns {string[]}
     */
    static get observedAttributes() {
        return ['type', 'count'];
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
     * Rebuild skeleton cards from type and count attributes.
     * @returns {void}
     */
    render() {
        const type = this.getAttribute('type') || 'form';
        const count = parseInt(this.getAttribute('count') || '2', 10);
        this.className = 'renop-skeleton-wrapper';
        this.innerHTML = '';

        for (let i = 0; i < count; i++) {
            const card = document.createElement('div');
            card.className = 'skeleton-loader-card';
            card.style.animationDelay = `${i * 0.04}s`;

            if (type === 'form') {
                card.appendChild(el('div', {
                    class: 'skeleton-bone',
                    style: {width: '30%', height: '22px', marginBottom: '1.25rem'}
                }));
                card.appendChild(el('div', {
                        style: {
                            display: 'flex',
                            gap: '1rem',
                            alignItems: 'center',
                            marginBottom: '1rem'
                        }
                    },
                    el('div', {class: 'skeleton-bone', style: {width: '25%', height: '16px'}}),
                    el('div', {class: 'skeleton-bone', style: {flex: '1', height: '38px', borderRadius: '8px'}})
                ));
                card.appendChild(el('div', {
                        style: {
                            display: 'flex',
                            gap: '1rem',
                            alignItems: 'center',
                            marginBottom: '1rem'
                        }
                    },
                    el('div', {class: 'skeleton-bone', style: {width: '25%', height: '16px'}}),
                    el('div', {class: 'skeleton-bone', style: {flex: '1', height: '38px', borderRadius: '8px'}})
                ));
            } else if (type === 'repo') {
                card.appendChild(el('div', {
                        style: {
                            display: 'flex',
                            alignItems: 'center',
                            gap: '12px',
                            marginBottom: '1.2rem'
                        }
                    },
                    el('div', {class: 'skeleton-bone', style: {width: '42px', height: '42px', borderRadius: '10px'}}),
                    el('div', {style: {flex: '1'}},
                        el('div', {
                            class: 'skeleton-bone',
                            style: {width: '140px', height: '20px', marginBottom: '6px'}
                        }),
                        el('div', {class: 'skeleton-bone', style: {width: '100px', height: '14px'}})
                    )
                ));
                card.appendChild(el('div', {
                        style: {
                            display: 'flex',
                            gap: '1rem',
                            alignItems: 'center',
                            marginBottom: '1rem'
                        }
                    },
                    el('div', {class: 'skeleton-bone', style: {width: '20%', height: '16px'}}),
                    el('div', {class: 'skeleton-bone', style: {flex: '1', height: '38px', borderRadius: '8px'}})
                ));
            } else if (type === 'table') {
                card.appendChild(el('div', {
                        style: {
                            display: 'flex',
                            gap: '1rem',
                            alignItems: 'center',
                            padding: '0.5rem 0'
                        }
                    },
                    el('div', {class: 'skeleton-bone', style: {width: '40px', height: '40px', borderRadius: '50%'}}),
                    el('div', {class: 'skeleton-bone', style: {width: '30%', height: '18px'}}),
                    el('div', {class: 'skeleton-bone', style: {flex: '1', height: '24px', borderRadius: '6px'}})
                ));
            } else {
                card.appendChild(el('div', {
                    class: 'skeleton-bone',
                    style: {width: '40%', height: '20px', marginBottom: '1rem'}
                }));
                card.appendChild(el('div', {
                    class: 'skeleton-bone',
                    style: {width: '100%', height: '60px', borderRadius: '6px'}
                }));
            }
            this.appendChild(card);
        }
    }
}

if (!customElements.get('renop-skeleton')) {
    customElements.define('renop-skeleton', RenopSkeleton);
}

/**
 * Create a loading skeleton placeholder.
 * @param {string} [type='form'] - Layout type (form, repo, table, or default).
 * @param {number} [count=2] - Number of skeleton cards to render.
 * @returns {HTMLElement}
 */
export function createSkeleton(type = 'form', count = 2) {
    const skel = document.createElement('renop-skeleton');
    skel.setAttribute('type', type);
    skel.setAttribute('count', String(count));
    return skel;
}
