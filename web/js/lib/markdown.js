/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

/*
 * Markdown rendering + automatic heading IDs / TOC extraction.
 */

import {marked} from 'marked';

marked.setOptions({
    gfm: true,
    breaks: false,
});

/**
 * Convert heading text into a URL-safe fragment id.
 * @param {string} text - Heading plain text.
 * @returns {string} Slug, or `section` if empty after sanitizing.
 */
function slugify(text) {
    return String(text)
        .replace(/<[^>]*>/g, '')
        .trim()
        .toLowerCase()
        .replace(/[^\p{L}\p{N}\s\-_/]/gu, '')
        .replace(/[\s_/]+/g, '-')
        .replace(/-+/g, '-')
        .replace(/^-|-$/g, '') || 'section';
}

/**
 * Strip optional YAML front matter for display.
 * @param {string} raw - Full markdown source.
 * @returns {string} Body without leading `---` front matter block.
 */
export function stripFrontMatter(raw) {
    if (!raw.startsWith('---')) return raw;
    const end = raw.indexOf('\n---', 3);
    if (end === -1) return raw;
    return raw.slice(end + 4).replace(/^\r?\n/, '');
}

/**
 * Extract visible heading text from Marked inline tokens without retaining
 * emphasis delimiters, link destinations, or inline HTML tags.
 * @param {Array<object>} tokens - Marked inline tokens.
 * @param {string} fallback - Raw heading text when tokens are unavailable.
 * @returns {string} Plain heading text for TOC labels and anchors.
 */
function headingText(tokens, fallback) {
    const visit = (items) => items.map((token) => {
        if (Array.isArray(token.tokens) && token.tokens.length > 0) {
            return visit(token.tokens);
        }
        if (typeof token.text === 'string') return token.text;
        return typeof token.raw === 'string' ? token.raw : '';
    }).join('');
    const text = Array.isArray(tokens) && tokens.length > 0 ? visit(tokens) : String(fallback || '');
    return text.replace(/<[^>]*>/g, '').trim();
}

/**
 * Render GitHub-flavored markdown to HTML and collect h2/h3 TOC entries with stable ids.
 * @param {string} [source] - Markdown source (front matter is stripped).
 * @returns {{ html: string, toc: Array<{ id: string, text: string, level: number }> }}
 */
export function renderMarkdown(source) {
    const body = stripFrontMatter(source || '');
    const toc = [];
    const used = new Map();

    const renderer = new marked.Renderer();

    renderer.heading = function heading({tokens, depth, text}) {
        const plain = headingText(tokens, text);
        let id = slugify(plain);
        const n = used.get(id) || 0;
        used.set(id, n + 1);
        if (n > 0) id = `${id}-${n}`;

        if (depth >= 2 && depth <= 3) {
            toc.push({id, text: plain, level: depth});
        }

        const inner = this.parser.parseInline(tokens);
        return `<h${depth} id="${id}">${inner}</h${depth}>\n`;
    };

    const originTable = renderer.table.bind(renderer);
    renderer.table = function table(token) {
        const inner = originTable(token);
        return `<div class="docs-table-wrap">${inner}</div>\n`;
    };

    let html;
    try {
        html = marked.parse(body, {renderer});
    } catch {
        html = marked.parse(body);
    }

    return {html: typeof html === 'string' ? html : String(html), toc};
}
