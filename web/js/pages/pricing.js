/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import { t } from '../i18n.js';
import { el, iconCheck } from '../lib/dom.js';

/**
 * Build a pricing tier card.
 * @param {object} opts
 * @param {string} opts.tier - Tier name.
 * @param {string} opts.price - Price label.
 * @param {string} opts.desc - Short description.
 * @param {string[]} opts.features - Feature bullet strings.
 * @param {boolean} [opts.featured] - Highlighted card style.
 * @param {string} [opts.badge] - Optional corner badge text.
 * @returns {HTMLElement}
 */
function tierCard({ tier, price, desc, features, featured, badge }) {
    const card = el('article', {
        class: `card pricing-card${featured ? ' pricing-card--featured' : ''}`,
    });

    if (badge) {
        card.appendChild(el('span', { class: 'pricing-badge' }, badge));
    }

    card.append(
        el('p', { class: 'pricing-tier' }, tier),
        el('p', { class: 'pricing-price' }, price, el('span', {}, t('pricing.forever'))),
        el('p', { class: 'pricing-desc' }, desc),
    );

    const ul = el('ul', { class: 'pricing-features' });
    for (const f of features) {
        const li = el('li', {});
        li.appendChild(iconCheck());
        li.appendChild(document.createTextNode(f));
        ul.appendChild(li);
    }
    card.appendChild(ul);

    card.appendChild(
        el('a', {
            class: `pill-btn ${featured ? 'pill-btn--primary' : 'pill-btn--soft'}`,
            href: '/download',
            'data-link': '',
        }, t('pricing.getStarted')),
    );

    return card;
}

/**
 * Render the pricing page (personal / nonprofit / enterprise tiers).
 * @param {{ root: HTMLElement }} ctx - Route context.
 * @returns {Promise<void>}
 */
export async function renderPricing({ root }) {
    root.innerHTML = '';

    const shared = [
        t('pricing.feat.allFeatures'),
        t('pricing.feat.selfHost'),
        t('pricing.feat.noTelemetry'),
        t('pricing.feat.noAds'),
        t('pricing.feat.unlimited'),
        t('pricing.feat.support'),
    ];

    root.append(
        el('header', { class: 'page-hero' },
            el('h1', {}, t('pricing.title')),
            el('p', {}, t('pricing.lead')),
        ),
        el('div', { class: 'pricing-grid' },
            tierCard({
                tier: t('pricing.personal'),
                price: t('pricing.free'),
                desc: t('pricing.personal.desc'),
                features: shared,
            }),
            tierCard({
                tier: t('pricing.nonprofit'),
                price: t('pricing.free'),
                desc: t('pricing.nonprofit.desc'),
                features: [...shared, t('pricing.feat.sla')],
                featured: true,
                badge: t('pricing.featured'),
            }),
            tierCard({
                tier: t('pricing.enterprise'),
                price: t('pricing.free'),
                desc: t('pricing.enterprise.desc'),
                features: [...shared, t('pricing.feat.invoice'), t('pricing.feat.priority')],
            }),
        ),
        el('div', { class: 'card pricing-note' },
            el('p', {}, t('pricing.note')),
            el('p', {}, el('strong', {}, t('pricing.footnote'))),
        ),
    );

    document.title = `RenoP — ${t('pricing.title')}`;
}
