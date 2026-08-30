/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {Marked} from 'marked';

const allowedMarkdownElements = new Set([
    'A', 'B', 'BLOCKQUOTE', 'BR', 'CODE', 'DEL', 'EM', 'H1', 'H2', 'H3', 'H4', 'H5', 'H6',
    'HR', 'IMG', 'LI', 'OL', 'P', 'PRE', 'STRONG', 'TABLE', 'TBODY', 'TD', 'TH', 'THEAD', 'TR', 'UL'
]);

/**
 * Escape raw HTML so Markdown cannot create active DOM nodes.
 * @param {string} value - Raw HTML token text.
 * @returns {string} Escaped text.
 */
function escapeMarkdownHTML(value) {
    return String(value || '')
        .replaceAll('&', '&amp;')
        .replaceAll('<', '&lt;')
        .replaceAll('>', '&gt;')
        .replaceAll('"', '&quot;')
        .replaceAll("'", '&#39;');
}

const safeMarkdownParser = new Marked({
    gfm: true,
    breaks: false,
    renderer: {
        html(token) {
            return escapeMarkdownHTML(token?.text || token?.raw || '');
        }
    }
});

/**
 * Accept only absolute HTTP(S) links for user-controlled Markdown and metadata.
 * @param {unknown} value - Candidate URL.
 * @returns {string} Normalized URL or an empty string.
 */
export function safeMarkdownURL(value) {
    const raw = String(value || '').trim();
    if (!raw) return '';
    try {
        const parsed = new URL(raw);
        if (!['http:', 'https:'].includes(parsed.protocol) || !parsed.hostname || parsed.username || parsed.password) {
            return '';
        }
        return parsed.href;
    } catch {
        return '';
    }
}

/**
 * Remove unsupported elements and every attribute not produced by the safe Markdown contract.
 * @param {DocumentFragment} fragment - Parsed inert template fragment.
 * @returns {void}
 */
function sanitizeMarkdownFragment(fragment) {
    for (const element of Array.from(fragment.querySelectorAll('*'))) {
        if (!allowedMarkdownElements.has(element.tagName)) {
            element.replaceWith(document.createTextNode(element.textContent || ''));
            continue;
        }
        const href = element.tagName === 'A' ? safeMarkdownURL(element.getAttribute('href')) : '';
        const source = element.tagName === 'IMG' ? safeMarkdownURL(element.getAttribute('src')) : '';
        const title = String(element.getAttribute('title') || '').slice(0, 512);
        const alternative = String(element.getAttribute('alt') || '').slice(0, 512);
        const codeClass = element.tagName === 'CODE' && /^language-[a-z0-9_-]{1,40}$/i.test(element.className)
            ? element.className
            : '';
        for (const attribute of Array.from(element.attributes)) element.removeAttribute(attribute.name);
        if (element.tagName === 'A' && href) {
            element.setAttribute('href', href);
            element.setAttribute('target', '_blank');
            element.setAttribute('rel', 'noopener noreferrer nofollow');
            if (title) element.setAttribute('title', title);
        }
        if (element.tagName === 'IMG') {
            if (!source) {
                element.replaceWith(document.createTextNode(alternative));
                continue;
            }
            element.setAttribute('src', source);
            element.setAttribute('alt', alternative);
            element.setAttribute('loading', 'lazy');
            element.setAttribute('referrerpolicy', 'no-referrer');
            if (title) element.setAttribute('title', title);
        }
        if (codeClass) element.className = codeClass;
    }
}

/**
 * Render bounded user-controlled Markdown into one element through an inert allowlist boundary.
 * @param {HTMLElement} target - Destination element.
 * @param {string} source - Markdown source.
 * @returns {void}
 */
export function setSafeMarkdown(target, source) {
    if (!(target instanceof HTMLElement)) return;
    const template = document.createElement('template');
    template.innerHTML = safeMarkdownParser.parse(String(source || ''));
    sanitizeMarkdownFragment(template.content);
    target.replaceChildren(template.content.cloneNode(true));
}
