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

/**
 * Build a feature-card SVG icon from a list of path/circle/rect/line descriptors.
 * @param {Array<
 *   | { type: 'path', d: string }
 *   | { type: 'circle', cx: string, cy: string, r: string }
 *   | { type: 'rect', x: string, y: string, w: string, h: string, rx?: string }
 *   | { type: 'line', x1: string, y1: string, x2: string, y2: string }
 * >} paths - Shape descriptors.
 * @returns {SVGSVGElement}
 */
function featureIcon(paths) {
    const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    svg.setAttribute('viewBox', '0 0 24 24');
    svg.setAttribute('width', '20');
    svg.setAttribute('height', '20');
    svg.setAttribute('fill', 'none');
    svg.setAttribute('stroke', 'currentColor');
    svg.setAttribute('stroke-width', '2');
    svg.setAttribute('stroke-linecap', 'round');
    svg.setAttribute('stroke-linejoin', 'round');
    svg.setAttribute('aria-hidden', 'true');
    for (const d of paths) {
        if (d.type === 'path') {
            const p = document.createElementNS('http://www.w3.org/2000/svg', 'path');
            p.setAttribute('d', d.d);
            svg.appendChild(p);
        } else if (d.type === 'circle') {
            const c = document.createElementNS('http://www.w3.org/2000/svg', 'circle');
            c.setAttribute('cx', d.cx);
            c.setAttribute('cy', d.cy);
            c.setAttribute('r', d.r);
            svg.appendChild(c);
        } else if (d.type === 'rect') {
            const r = document.createElementNS('http://www.w3.org/2000/svg', 'rect');
            r.setAttribute('x', d.x);
            r.setAttribute('y', d.y);
            r.setAttribute('width', d.w);
            r.setAttribute('height', d.h);
            r.setAttribute('rx', d.rx || '0');
            svg.appendChild(r);
        } else if (d.type === 'line') {
            const l = document.createElementNS('http://www.w3.org/2000/svg', 'line');
            l.setAttribute('x1', d.x1);
            l.setAttribute('y1', d.y1);
            l.setAttribute('x2', d.x2);
            l.setAttribute('y2', d.y2);
            svg.appendChild(l);
        }
    }
    return svg;
}

/**
 * Render the marketing home page (hero, feature grid, CTA).
 * @param {{ root: HTMLElement }} ctx - Route context; `root` is `#page-root`.
 * @returns {Promise<void>}
 */
export async function renderHome({root}) {
    root.innerHTML = '';

    const features = [
        {
            title: t('home.feature1.title'),
            desc: t('home.feature1.desc'),
            icon: [
                {type: 'path', d: 'M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z'},
            ],
        },
        {
            title: t('home.feature2.title'),
            desc: t('home.feature2.desc'),
            icon: [
                {type: 'circle', cx: '12', cy: '12', r: '10'},
                {type: 'line', x1: '2', y1: '12', x2: '22', y2: '12'},
                {
                    type: 'path',
                    d: 'M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z'
                },
            ],
        },
        {
            title: t('home.feature3.title'),
            desc: t('home.feature3.desc'),
            icon: [
                {type: 'rect', x: '2', y: '3', w: '20', h: '14', rx: '2'},
                {type: 'line', x1: '8', y1: '21', x2: '16', y2: '21'},
                {type: 'line', x1: '12', y1: '17', x2: '12', y2: '21'},
            ],
        },
    ];

    const hero = el('section', {class: 'home-hero'},
        el('div', {class: 'home-hero-copy'},
            el('h1', {}, t('home.title')),
            el('p', {class: 'lead'}, t('home.lead')),
            el('div', {class: 'home-hero-actions'},
                el('a', {
                    class: 'pill-btn pill-btn--primary pill-btn--lg',
                    href: '/download',
                    'data-link': ''
                }, t('home.download')),
                el('a', {
                    class: 'pill-btn pill-btn--soft pill-btn--lg',
                    href: '/docs',
                    'data-link': ''
                }, t('home.readDocs')),
            ),
        ),
        el('div', {class: 'home-hero-visual'},
            el('div', {class: 'home-screenshot-wrap'},
                el('img', {
                    class: 'home-screenshot',
                    src: '/assets/mainscreen.png',
                    alt: 'RenoP main screen',
                    loading: 'eager',
                }),
            ),
        ),
    );

    const grid = el('div', {class: 'feature-grid'});
    for (const f of features) {
        const iconWrap = el('div', {class: 'feature-icon'});
        iconWrap.appendChild(featureIcon(f.icon));
        grid.appendChild(
            el('article', {class: 'card feature-card'},
                iconWrap,
                el('h3', {}, f.title),
                el('p', {}, f.desc),
            ),
        );
    }

    const cta = el('section', {class: 'card home-cta'},
        el('div', {},
            el('h2', {}, t('home.cta.title')),
            el('p', {}, t('home.cta.desc')),
        ),
        el('a', {class: 'pill-btn pill-btn--primary', href: '/pricing', 'data-link': ''}, t('home.cta.pricing')),
    );

    root.append(hero, grid, cta);
    document.title = `RenoP — ${t('home.title')}`;
}
