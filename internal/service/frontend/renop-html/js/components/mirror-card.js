/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {t} from '../i18n.js';
import {el} from '@renop/ui/dom';
import {createIcon} from './icon.js';
import {createMetaChip} from './meta-chip.js';

/**
 * Upstream mirror summary card custom element.
 */
export class RenopMirrorCard extends HTMLElement {
    /**
     * @returns {string[]}
     */
    static get observedAttributes() {
        return ['name', 'url', 'persist', 'enabled-date', 'cache-ttl', 'negative-cache', 'detailed'];
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
     * Rebuild mirror card content from attributes.
     * @returns {void}
     */
    render() {
        const nameText = this.getAttribute('name') || (t('details.unnamedMirror') || 'Unnamed Mirror');
        const urlText = this.getAttribute('url') || 'N/A';
        const persist = this.getAttribute('persist') === 'true';
        const enabledDate = this.getAttribute('enabled-date') || 'N/A';
        const isDetailed = this.hasAttribute('detailed');

        this.className = 'mirror-card' + (isDetailed ? ' mirror-card--detailed' : '');
        this.innerHTML = '';

        const header = el('div', {class: 'mirror-card-header'});
        const icon = el('div', {class: 'mirror-card-icon', 'aria-hidden': 'true'});
        icon.appendChild(createIcon('network'));

        const titleWrap = el('div', {class: 'mirror-card-title-wrap'},
            el('div', {class: 'mirror-card-name'}, nameText),
            el('div', {class: 'mirror-card-url', title: urlText}, urlText)
        );

        header.appendChild(icon);
        header.appendChild(titleWrap);
        this.appendChild(header);

        const meta = el('div', {class: 'mirror-card-meta'});
        meta.appendChild(createMetaChip(t('stats.persist') || 'Persist', persist ? (t('common.yes') || 'Yes') : (t('common.no') || 'No'), persist ? 'yes' : 'no'));
        meta.appendChild(createMetaChip(t('stats.enabledDate') || 'Enabled Date', enabledDate));
        this.appendChild(meta);

        if (isDetailed) {
            const cacheTtl = this.getAttribute('cache-ttl') ?? 'N/A';
            const negCache = this.getAttribute('negative-cache') === 'true';
            const cacheMeta = el('div', {class: 'mirror-card-meta mirror-card-meta--secondary'});
            cacheMeta.appendChild(createMetaChip(t('stats.cacheTtl') || 'Cache TTL', `${cacheTtl}s`));
            cacheMeta.appendChild(createMetaChip(t('stats.negativeCache') || 'Negative Cache', negCache ? (t('common.yes') || 'Yes') : (t('common.no') || 'No'), negCache ? 'yes' : 'no'));
            this.appendChild(cacheMeta);
        }
    }
}

if (!customElements.get('renop-mirror-card')) {
    customElements.define('renop-mirror-card', RenopMirrorCard);
}

/**
 * Create a mirror card from a mirror config object.
 * @param {object} mirror - Mirror configuration.
 * @param {string} [mirror.name] - Mirror display name.
 * @param {string} [mirror.url] - Mirror URL.
 * @param {boolean} [mirror.persist] - Whether content is persisted.
 * @param {string} [mirror.enabled_date] - Enabled date string.
 * @param {number|string} [mirror.cache_ttl] - Cache TTL in seconds.
 * @param {boolean} [mirror.negative_cache] - Negative cache enabled.
 * @param {boolean} [includeCacheDetails=false] - Include cache TTL / negative-cache chips.
 * @returns {HTMLElement}
 */
export function createMirrorCard(mirror, includeCacheDetails = false) {
    const card = document.createElement('renop-mirror-card');
    if (mirror.name) card.setAttribute('name', mirror.name);
    if (mirror.url) card.setAttribute('url', mirror.url);
    card.setAttribute('persist', mirror.persist ? 'true' : 'false');
    if (mirror.enabled_date) card.setAttribute('enabled-date', mirror.enabled_date);
    if (includeCacheDetails) {
        card.setAttribute('detailed', '');
        if (mirror.cache_ttl !== undefined) card.setAttribute('cache-ttl', String(mirror.cache_ttl));
        card.setAttribute('negative-cache', mirror.negative_cache ? 'true' : 'false');
    }
    return card;
}
