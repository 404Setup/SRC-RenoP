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
 * Create a DOM element with attributes and children.
 * Special attrs: `class`/`className`, `text`, `html`, object `style`, object `dataset`,
 * and `on*` handlers (e.g. `onClick` → `click` listener).
 * @param {string} tag - HTML tag name.
 * @param {Record<string, *>} [attrs={}] - Attributes / props.
 * @param {...(Node|string|number|null|false|Array)} children - Nested content (arrays are flattened).
 * @returns {HTMLElement}
 */
export function el(tag, attrs = {}, ...children) {
    const node = document.createElement(tag);
    for (const [k, v] of Object.entries(attrs || {})) {
        if (v == null || v === false) continue;
        if (k === 'class') node.className = v;
        else if (k === 'className') node.className = v;
        else if (k === 'text') node.textContent = v;
        else if (k === 'html') node.innerHTML = v;
        else if (k === 'style' && typeof v === 'object') Object.assign(node.style, v);
        else if (k.startsWith('on') && typeof v === 'function') {
            node.addEventListener(k.slice(2).toLowerCase(), v);
        } else if (k === 'dataset' && typeof v === 'object') {
            Object.assign(node.dataset, v);
        } else {
            node.setAttribute(k, v === true ? '' : String(v));
        }
    }
    for (const child of children.flat()) {
        if (child == null || child === false) continue;
        node.appendChild(typeof child === 'string' || typeof child === 'number'
            ? document.createTextNode(String(child))
            : child);
    }
    return node;
}

/**
 * Remove all child nodes from an element.
 * @param {Node} node - Parent to empty.
 * @returns {Node} The same node for chaining.
 */
export function clear(node) {
    while (node.firstChild) node.removeChild(node.firstChild);
    return node;
}

/**
 * Build a small inline SVG checkmark icon (stroke, currentColor).
 * @returns {SVGSVGElement}
 */
export function iconCheck() {
    const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    svg.setAttribute('viewBox', '0 0 24 24');
    svg.setAttribute('width', '16');
    svg.setAttribute('height', '16');
    svg.setAttribute('fill', 'none');
    svg.setAttribute('stroke', 'currentColor');
    svg.setAttribute('stroke-width', '2.5');
    svg.setAttribute('stroke-linecap', 'round');
    svg.setAttribute('stroke-linejoin', 'round');
    svg.setAttribute('aria-hidden', 'true');
    const p = document.createElementNS('http://www.w3.org/2000/svg', 'polyline');
    p.setAttribute('points', '20 6 9 17 4 12');
    svg.appendChild(p);
    return svg;
}
