/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import { t, getDocsLocale } from '../i18n.js';
import { el, clear } from '../lib/dom.js';
import { renderMarkdown } from '../lib/markdown.js';
import { morphElementHeight, prefersReducedMotion } from '@renop/ui/height-anim';

let indexCache = null;
let tocObserver = null;

/** In-memory shell refs for soft navigation within /docs*. */
let shell = null;

/**
 * Load and cache the docs index JSON (`/content/docs-index.json`).
 * @returns {Promise<object>} Parsed index document.
 * @throws {Error} When the index cannot be fetched.
 */
async function loadIndex() {
    if (indexCache) return indexCache;
    const res = await fetch('/content/docs-index.json', { cache: 'no-cache' });
    if (!res.ok) throw new Error('docs index missing');
    indexCache = await res.json();
    return indexCache;
}

/**
 * Resolve docs + categories for a locale from the index (supports multi-locale and legacy shapes).
 * @param {object|null|undefined} index - Docs index JSON.
 * @param {string} locale - Locale key (e.g. `en-US`).
 * @returns {{ docs: Array<object>, categories: Array<object> }}
 */
function localeBundle(index, locale) {
    if (!index) return { docs: [], categories: [] };
    if (index.locales) {
        return index.locales[locale] || index.locales[index.defaultLocale || 'en-US'] || { docs: [], categories: [] };
    }
    return { docs: index.docs || [], categories: index.categories || [] };
}

/**
 * Find document metadata by slug, preferring the primary bundle then fallback.
 * @param {{ docs: Array<{ slug: string }> }} bundle
 * @param {{ docs: Array<{ slug: string }> }} fallbackBundle
 * @param {string} slug - Doc slug path.
 * @returns {object|null}
 */
function resolveDocMeta(bundle, fallbackBundle, slug) {
    return bundle.docs.find((d) => d.slug === slug)
        || fallbackBundle.docs.find((d) => d.slug === slug)
        || null;
}

/**
 * Sidebar expand/collapse chevron SVG.
 * @returns {SVGSVGElement}
 */
function chevronSvg() {
    const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    svg.setAttribute('class', 'docs-sidebar-chevron');
    svg.setAttribute('viewBox', '0 0 24 24');
    svg.setAttribute('width', '16');
    svg.setAttribute('height', '16');
    svg.setAttribute('fill', 'none');
    svg.setAttribute('stroke', 'currentColor');
    svg.setAttribute('stroke-width', '2.2');
    svg.setAttribute('stroke-linecap', 'round');
    svg.setAttribute('stroke-linejoin', 'round');
    svg.setAttribute('aria-hidden', 'true');
    const p = document.createElementNS('http://www.w3.org/2000/svg', 'polyline');
    p.setAttribute('points', '6 9 12 15 18 9');
    svg.appendChild(p);
    return svg;
}

/**
 * @returns {boolean} True when the docs layout is in the mobile breakpoint (≤720px).
 */
function isMobileDocs() {
    return window.matchMedia('(max-width: 720px)').matches;
}

/**
 * Animated height expand/collapse for the categories body (mobile).
 * @param {HTMLElement} sidebar - Sidebar root (receives `is-expanded` / `is-collapsed`).
 * @param {HTMLElement} body - Collapsible categories body.
 * @param {boolean} expanded - Target open state.
 * @param {{ animate?: boolean }} [options]
 * @param {boolean} [options.animate=true] - When false, snap height/opacity without transition.
 * @returns {void}
 */
function setSidebarExpanded(sidebar, body, expanded, { animate = true } = {}) {
    sidebar.classList.toggle('is-expanded', expanded);
    sidebar.classList.toggle('is-collapsed', !expanded);
    const toggle = sidebar.querySelector('.docs-sidebar-toggle');
    if (toggle) {
        toggle.setAttribute('aria-expanded', expanded ? 'true' : 'false');
    }

    if (!animate) {
        body.style.height = expanded ? 'auto' : '0px';
        body.style.opacity = expanded ? '1' : '0';
        return;
    }

    if (expanded) {
        body.style.opacity = '1';
        body.style.height = '0px';
        void body.offsetHeight;
        const target = body.scrollHeight;
        body.style.height = `${target}px`;
        /** After expand transition, unlock height so content can grow freely. @param {TransitionEvent} e */
        const onEnd = (e) => {
            if (e.propertyName !== 'height') return;
            body.removeEventListener('transitionend', onEnd);
            if (sidebar.classList.contains('is-expanded')) {
                body.style.height = 'auto';
            }
        };
        body.addEventListener('transitionend', onEnd);
    } else {
        const current = body.scrollHeight;
        body.style.height = `${current}px`;
        void body.offsetHeight;
        body.style.opacity = '0';
        body.style.height = '0px';
    }
}

/**
 * Collapse the mobile category drawer after picking a page.
 * @param {HTMLElement|null|undefined} sidebar
 * @returns {void}
 */
function collapseSidebarIfMobile(sidebar) {
    if (!sidebar || !isMobileDocs()) return;
    if (!sidebar.classList.contains('is-expanded')) return;
    const body = sidebar.querySelector('.docs-sidebar-body');
    if (!body) return;
    setSidebarExpanded(sidebar, body, false, { animate: true });
}

/**
 * Build the docs category sidebar with mobile toggle and breakpoint listener.
 * @param {Array<{ name: string, docs: Array<{ slug: string, title: string }> }>} categories
 * @param {string|null} activeSlug - Currently viewed doc slug, or null on index.
 * @returns {HTMLElement} Sidebar element (stores `_docsMql` / `_docsOnBreakpoint` for cleanup).
 */
function buildSidebar(categories, activeSlug) {
    const aside = el('aside', { class: 'card docs-sidebar is-collapsed' });

    const toggle = el('button', {
        type: 'button',
        class: 'docs-sidebar-toggle',
        'aria-expanded': 'false',
        'aria-controls': 'docs-sidebar-body',
        title: t('docs.toggleCategories'),
        'data-i18n-title': 'docs.toggleCategories',
        'data-i18n-aria-label': 'docs.toggleCategories',
        'aria-label': t('docs.toggleCategories'),
    });
    toggle.append(
        el('span', { class: 'docs-sidebar-toggle-label', 'data-i18n': 'docs.categories' }, t('docs.categories')),
        (() => {
            const wrap = el('span', { class: 'docs-sidebar-chevron-wrap' });
            wrap.appendChild(chevronSvg());
            return wrap;
        })(),
    );
    aside.appendChild(toggle);

    aside.appendChild(el('h2', { class: 'docs-sidebar-heading', 'data-i18n': 'docs.categories' }, t('docs.categories')));

    const body = el('div', {
        class: 'docs-sidebar-body',
        id: 'docs-sidebar-body',
    });

    for (const cat of categories) {
        const block = el('div', { class: 'docs-cat' },
            el('div', { class: 'docs-cat-title' }, cat.name),
        );
        const ul = el('ul', { class: 'docs-nav' });
        for (const doc of cat.docs) {
            ul.appendChild(
                el('li', {},
                    el('a', {
                        href: `/docs/${doc.slug}`,
                        'data-link': '',
                        class: doc.slug === activeSlug ? 'active' : '',
                        'data-docs-slug': doc.slug,
                    }, doc.title),
                ),
            );
        }
        block.appendChild(ul);
        body.appendChild(block);
    }
    aside.appendChild(body);

    if (isMobileDocs()) {
        setSidebarExpanded(aside, body, false, { animate: false });
    } else {
        setSidebarExpanded(aside, body, true, { animate: false });
        body.style.height = 'auto';
        body.style.opacity = '1';
    }

    toggle.addEventListener('click', () => {
        const expanded = !aside.classList.contains('is-expanded');
        setSidebarExpanded(aside, body, expanded, { animate: true });
    });

    body.addEventListener('click', (e) => {
        const a = e.target.closest('a[data-docs-slug]');
        if (!a) return;
        collapseSidebarIfMobile(aside);
    });

    const mql = window.matchMedia('(max-width: 720px)');
    /** Sync sidebar open/closed state when crossing the mobile breakpoint. */
    const onBreakpoint = () => {
        if (mql.matches) {
            setSidebarExpanded(aside, body, false, { animate: false });
        } else {
            setSidebarExpanded(aside, body, true, { animate: false });
            body.style.height = 'auto';
            body.style.opacity = '1';
        }
    };
    mql.addEventListener('change', onBreakpoint);
    aside._docsMql = mql;
    aside._docsOnBreakpoint = onBreakpoint;

    return aside;
}

/**
 * Highlight the active doc link in an existing sidebar.
 * @param {HTMLElement|null|undefined} sidebar
 * @param {string|null} activeSlug
 * @returns {void}
 */
function updateSidebarActive(sidebar, activeSlug) {
    if (!sidebar) return;
    sidebar.querySelectorAll('.docs-nav a').forEach((a) => {
        const slug = a.getAttribute('data-docs-slug') || '';
        a.classList.toggle('active', slug === activeSlug);
    });
}

/**
 * Build the on-this-page TOC aside from markdown heading entries.
 * @param {Array<{ id: string, text: string, level: number }>} toc
 * @returns {HTMLElement}
 */
function buildToc(toc) {
    const aside = el('aside', { class: 'card docs-toc' },
        el('h2', { 'data-i18n': 'docs.onThisPage' }, t('docs.onThisPage')),
    );
    if (!toc.length) {
        aside.appendChild(el('p', { class: 'muted docs-toc-empty' }, '—'));
        return aside;
    }
    const ul = el('ul', { class: 'docs-toc-list' });
    for (const item of toc) {
        ul.appendChild(
            el('li', {},
                el('a', {
                    href: `#${item.id}`,
                    class: item.level >= 3 ? 'toc-h3' : '',
                }, item.text),
            ),
        );
    }
    aside.appendChild(ul);
    return aside;
}

/**
 * Render the docs index listing (all categories and pages).
 * @param {Array<{ name: string, docs: Array<{ slug: string, title: string, description?: string }> }>} categories
 * @returns {HTMLElement}
 */
function renderDocsIndex(categories) {
    const wrap = el('div', { class: 'docs-index-list' });
    if (!categories.length) {
        wrap.appendChild(el('p', { class: 'docs-empty' }, t('docs.empty')));
        return wrap;
    }
    for (const cat of categories) {
        const card = el('section', { class: 'card docs-index-cat' },
            el('h2', {}, cat.name),
        );
        const ul = el('ul', {});
        for (const doc of cat.docs) {
            ul.appendChild(
                el('li', {},
                    el('a', {
                        href: `/docs/${doc.slug}`,
                        'data-link': '',
                        'data-docs-slug': doc.slug,
                    }, doc.title),
                    doc.description
                        ? el('span', { class: 'muted', style: { display: 'block', fontSize: '0.85rem' } }, doc.description)
                        : null,
                ),
            );
        }
        card.appendChild(ul);
        wrap.appendChild(card);
    }
    return wrap;
}

/**
 * Disconnect the scroll-spy IntersectionObserver for TOC highlighting.
 * @returns {void}
 */
function disconnectTocObserver() {
    if (tocObserver) {
        tocObserver.disconnect();
        tocObserver = null;
    }
}

/**
 * Watch article headings and toggle the active class on matching TOC links.
 * @param {HTMLElement|null|undefined} article - Article containing `h2[id]` / `h3[id]`.
 * @param {HTMLElement|null|undefined} tocEl - TOC root with `.docs-toc-list a`.
 * @returns {void}
 */
function attachTocObserver(article, tocEl) {
    disconnectTocObserver();
    const links = tocEl?.querySelectorAll('.docs-toc-list a') || [];
    if (!links.length || !article) return;

    tocObserver = new IntersectionObserver(
        (entries) => {
            for (const entry of entries) {
                if (!entry.isIntersecting) continue;
                const id = entry.target.id;
                links.forEach((a) => {
                    a.classList.toggle('active', a.getAttribute('href') === `#${id}`);
                });
            }
        },
        { rootMargin: '-20% 0px -70% 0px', threshold: 0 },
    );
    article.querySelectorAll('h2[id], h3[id]').forEach((h) => tocObserver.observe(h));
}

/**
 * Soft fade for content column when switching docs.
 * Opacity-only — height is handled separately via `morphElementHeight`.
 * @param {HTMLElement} main - Content column element.
 * @returns {void}
 */
function flashContent(main) {
    main.classList.remove('docs-content-enter');
    void main.offsetWidth;
    main.classList.add('docs-content-enter');
}

/**
 * Replace all children of `container` with the given nodes.
 * @param {HTMLElement} container
 * @param {Array<Node|null|undefined>} nodes
 * @returns {void}
 */
function applyNodes(container, nodes) {
    clear(container);
    for (const node of nodes) {
        if (node) container.appendChild(node);
    }
}

/**
 * Swap children and morph height like the product frontend file list
 * (`morphElementHeight` from `@renop/ui/height-anim`).
 * Soft navigations animate; first paint / hard mount skips height tween.
 * @param {HTMLElement|null|undefined} container
 * @param {Array<Node|null|undefined>} nodes - New children.
 * @param {{ soft?: boolean, flash?: boolean }} [options]
 * @param {boolean} [options.soft=false] - Enable height morph when true.
 * @param {boolean} [options.flash=false] - Play content enter fade after swap.
 * @returns {Promise<void>}
 */
function replaceWithHeightMorph(container, nodes, { soft = false, flash = false } = {}) {
    if (!container) return Promise.resolve();

    if (!soft || prefersReducedMotion()) {
        applyNodes(container, nodes);
        if (flash && soft) flashContent(container);
        return Promise.resolve();
    }

    container.classList.add('is-height-morphing');
    return morphElementHeight(container, () => {
        applyNodes(container, nodes);
        if (flash) flashContent(container);
    }, { duration: 340 }).finally(() => {
        container.classList.remove('is-height-morphing');
    });
}

/**
 * Replace the TOC slot contents with a new TOC element (or clear it).
 * @param {HTMLElement|null|undefined} tocSlot
 * @param {HTMLElement|null|undefined} tocEl
 * @param {{ soft?: boolean }} [options]
 * @returns {Promise<void>}
 */
function replaceTocSlot(tocSlot, tocEl, { soft = false } = {}) {
    if (!tocSlot) return Promise.resolve();
    const nodes = tocEl ? [tocEl] : [];
    return replaceWithHeightMorph(tocSlot, nodes, { soft, flash: false });
}

/**
 * Load and render either the docs index or a single markdown article into the content column.
 * @param {object} opts
 * @param {HTMLElement} opts.main - Main content column.
 * @param {HTMLElement|null|undefined} opts.tocSlot - Right-hand TOC container.
 * @param {HTMLElement} opts.layout - Docs layout root.
 * @param {string} opts.slug - Doc slug, or empty for index.
 * @param {Array<object>} opts.categories - Category tree for the index view.
 * @param {{ docs: Array<object> }} opts.bundle - Locale docs bundle.
 * @param {{ docs: Array<object> }} opts.fallbackBundle - Fallback locale bundle (e.g. en-US).
 * @param {boolean} opts.soft - Whether this is a soft in-section navigation.
 * @returns {Promise<void>}
 */
async function fillContent({ main, tocSlot, layout, slug, categories, bundle, fallbackBundle, soft }) {
    disconnectTocObserver();

    if (!slug) {
        document.title = `RenoP — ${t('docs.title')}`;
        await Promise.all([
            replaceWithHeightMorph(main, [
                el('h2', { class: 'docs-index-heading', 'data-i18n': 'docs.indexTitle' }, t('docs.indexTitle')),
                renderDocsIndex(categories),
            ], { soft, flash: soft }),
            replaceTocSlot(tocSlot, null, { soft }),
        ]);
        return;
    }

    const meta = resolveDocMeta(bundle, fallbackBundle, slug);
    if (!meta) {
        await Promise.all([
            replaceWithHeightMorph(main, [
                el('p', { class: 'docs-error' }, t('docs.notFound')),
            ], { soft, flash: soft }),
            replaceTocSlot(tocSlot, null, { soft }),
        ]);
        return;
    }

    if (!soft || !main.childNodes.length) {
        await replaceWithHeightMorph(main, [
            el('p', { class: 'docs-loading' }, t('docs.loading')),
        ], { soft: false });
        if (tocSlot && !soft) clear(tocSlot);
    }

    try {
        let res = await fetch(`/content/${meta.path}`, { cache: 'no-cache' });
        if (!res.ok && meta.locale !== 'en-US') {
            const fb = fallbackBundle.docs.find((d) => d.slug === slug);
            if (fb) res = await fetch(`/content/${fb.path}`, { cache: 'no-cache' });
        }
        if (!res.ok) throw new Error('missing');
        const raw = await res.text();
        const { html, toc } = renderMarkdown(raw);

        const article = el('article', { class: 'card docs-article' });
        article.innerHTML = html;
        if (!article.querySelector('h1')) {
            article.prepend(el('h1', {}, meta.title));
        }
        wrapOverflowBlocks(article);
        rewriteDocLinks(article, meta.slug);

        const tocEl = buildToc(toc);

        await Promise.all([
            replaceWithHeightMorph(main, [article], { soft, flash: soft }),
            replaceTocSlot(tocSlot, tocEl, { soft }),
        ]);

        if (!tocSlot && layout && !layout.querySelector('.docs-toc')) {
            layout.appendChild(tocEl);
        }

        document.title = `${meta.title} — RenoP`;
        attachTocObserver(article, tocEl);
    } catch {
        await Promise.all([
            replaceWithHeightMorph(main, [
                el('p', { class: 'docs-error' }, t('docs.loadError')),
            ], { soft, flash: soft }),
            replaceTocSlot(tocSlot, null, { soft }),
        ]);
    }
}

/**
 * Tear down the persistent docs shell (TOC observer, breakpoint listener, shell refs).
 * @returns {void}
 */
function destroyShell() {
    disconnectTocObserver();
    if (shell?.sidebar?._docsMql && shell.sidebar._docsOnBreakpoint) {
        try {
            shell.sidebar._docsMql.removeEventListener('change', shell.sidebar._docsOnBreakpoint);
        } catch {
            /* ignore */
        }
    }
    shell = null;
}

/**
 * Render the documentation section: sidebar + article (or index) + TOC.
 * Soft navigations reuse the existing shell when staying under `/docs`.
 * @param {object} ctx
 * @param {HTMLElement} ctx.root - `#page-root`.
 * @param {{ splat?: string }} ctx.params - Route params; `splat` is the doc path under `/docs/`.
 * @param {boolean} [ctx.soft=false] - Soft navigation flag from the router.
 * @returns {Promise<() => void>} Cleanup that destroys the docs shell.
 */
export async function renderDocs({ root, params, soft = false }) {
    const locale = getDocsLocale();
    const fallbackLocale = 'en-US';
    const slug = (params.splat || '').replace(/^\/+|\/+$/g, '');
    const activeSlug = slug || null;

    if (soft && shell && root.contains(shell.layout)) {
        let index;
        try {
            index = await loadIndex();
        } catch {
            clear(shell.main);
            shell.main.appendChild(el('p', { class: 'docs-error' }, t('docs.loadError')));
            return () => destroyShell();
        }

        const bundle = localeBundle(index, locale);
        const fallbackBundle = localeBundle(index, fallbackLocale);
        const categories = bundle.categories.length ? bundle.categories : fallbackBundle.categories;

        if (shell.locale !== locale) {
            const newSidebar = buildSidebar(categories, activeSlug);
            shell.sidebar.replaceWith(newSidebar);
            shell.sidebar = newSidebar;
            shell.locale = locale;
        } else {
            updateSidebarActive(shell.sidebar, activeSlug);
        }

        collapseSidebarIfMobile(shell.sidebar);

        await fillContent({
            main: shell.main,
            tocSlot: shell.tocSlot,
            layout: shell.layout,
            slug,
            categories,
            bundle,
            fallbackBundle,
            soft: true,
        });

        const top = shell.layout.getBoundingClientRect().top + window.scrollY;
        const navOffset = 16;
        if (Math.abs((window.scrollY || 0) - Math.max(0, top - navOffset)) > 8) {
            try {
                window.scrollTo({ top: Math.max(0, top - navOffset), behavior: 'smooth' });
            } catch {
                window.scrollTo(0, Math.max(0, top - navOffset));
            }
        }

        return () => destroyShell();
    }

    destroyShell();
    root.innerHTML = '';
    document.title = `RenoP — ${t('docs.title')}`;

    const hero = el('header', { class: 'page-hero' },
        el('h1', { 'data-i18n': 'docs.title' }, t('docs.title')),
        el('p', { 'data-i18n': 'docs.lead' }, t('docs.lead')),
    );
    root.appendChild(hero);

    const layout = el('div', { class: 'docs-layout' });
    const main = el('div', { class: 'docs-content' },
        el('p', { class: 'docs-loading' }, t('docs.loading')),
    );
    const tocSlot = el('div', { class: 'docs-toc-slot' });

    root.appendChild(layout);

    let index;
    try {
        index = await loadIndex();
    } catch {
        clear(main);
        layout.appendChild(main);
        main.appendChild(el('p', { class: 'docs-error' }, t('docs.loadError')));
        return () => destroyShell();
    }

    const bundle = localeBundle(index, locale);
    const fallbackBundle = localeBundle(index, fallbackLocale);
    const categories = bundle.categories.length ? bundle.categories : fallbackBundle.categories;

    const sidebar = buildSidebar(categories, activeSlug);
    layout.append(sidebar, main, tocSlot);

    shell = {
        layout,
        sidebar,
        main,
        tocSlot,
        hero,
        locale,
    };

    await fillContent({
        main,
        tocSlot,
        layout,
        slug,
        categories,
        bundle,
        fallbackBundle,
        soft: false,
    });

    return () => destroyShell();
}

/**
 * Drop the in-memory docs index so the next render reloads it (e.g. after language change).
 * @returns {void}
 */
export function invalidateDocsCache() {
    indexCache = null;
}

/**
 * Ensure wide blocks stay inside the article: wrap bare tables for horizontal scroll.
 * Markdown already wraps tables; this is a safety net for fallback parse paths.
 * @param {HTMLElement} article - Rendered article root.
 * @returns {void}
 */
function wrapOverflowBlocks(article) {
    article.querySelectorAll('table').forEach((table) => {
        if (table.parentElement?.classList.contains('docs-table-wrap')) return;
        const wrap = document.createElement('div');
        wrap.className = 'docs-table-wrap';
        table.replaceWith(wrap);
        wrap.appendChild(table);
    });
}

/**
 * Rewrite relative `.md` links inside an article to in-app `/docs/...` routes with `data-link`.
 * @param {HTMLElement} article - Rendered article root.
 * @param {string} currentSlug - Slug of the current document (for resolving relative paths).
 * @returns {void}
 */
function rewriteDocLinks(article, currentSlug) {
    const baseParts = currentSlug.split('/').slice(0, -1);
    article.querySelectorAll('a[href]').forEach((a) => {
        const href = a.getAttribute('href') || '';
        if (!href || href.startsWith('http') || href.startsWith('#') || href.startsWith('/')) return;
        if (!/\.md($|#)/i.test(href)) return;

        const [pathPart, hash] = href.split('#');
        const joined = [...baseParts, ...pathPart.split('/')];
        const resolved = [];
        for (const part of joined) {
            if (!part || part === '.') continue;
            if (part === '..') resolved.pop();
            else resolved.push(part);
        }
        const nextSlug = resolved.join('/').replace(/\.md$/i, '');
        a.setAttribute('href', `/docs/${nextSlug}${hash ? `#${hash}` : ''}`);
        a.setAttribute('data-link', '');
        a.setAttribute('data-docs-slug', nextSlug);
    });
}
