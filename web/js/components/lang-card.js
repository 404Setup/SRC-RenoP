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
 * Minimal local element factory for this component (no shared `lib/dom` dependency).
 * @param {string} tag - HTML tag name.
 * @param {Record<string, *>} [attrs={}] - Attributes; `class` → className, `text` → textContent.
 * @param {...(Node|string|null|false)} children
 * @returns {HTMLElement}
 */
function el(tag, attrs = {}, ...children) {
    const node = document.createElement(tag);
    for (const [k, v] of Object.entries(attrs)) {
        if (k === 'class') node.className = v;
        else if (k === 'text') node.textContent = v;
        else if (v != null) node.setAttribute(k, v);
    }
    for (const child of children) {
        if (child == null || child === false) continue;
        node.appendChild(typeof child === 'string' ? document.createTextNode(child) : child);
    }
    return node;
}

/**
 * Create a language picker card button.
 * @param {object} opts
 * @param {string} opts.code - Language code (e.g. `en-US`); used for `data-lang` and badge.
 * @param {string} [opts.name] - Primary display name.
 * @param {string} [opts.sub] - Secondary subtitle.
 * @param {boolean} [opts.active] - When true, card gets the `active` class.
 * @param {(ev: MouseEvent) => void} [opts.onClick] - Click handler.
 * @returns {HTMLButtonElement}
 */
export function createLangCard({ code, name, sub, active, onClick }) {
    const card = el('button', {
        type: 'button',
        class: `lang-card${active ? ' active' : ''}`,
        'data-lang': code,
        role: 'button',
    });

    const shortCode = (code.split('-')[0] || code).slice(0, 3).toUpperCase();
    const left = el('div', { class: 'lang-card-left' },
        el('span', { class: 'lang-code-badge' }, shortCode),
        el('div', { class: 'lang-card-info' },
            el('span', { class: 'lang-card-name' }, name || code),
            el('span', { class: 'lang-card-sub' }, sub || ''),
        ),
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

    card.appendChild(left);
    card.appendChild(checkSvg);

    if (onClick) {
        card.addEventListener('click', onClick);
    }
    return card;
}
