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
 * Lightweight client-side router (History API) with page enter/leave animation.
 * Soft-navigates within the same section (e.g. /docs → /docs/...) without remounting.
 */

import {smoothScrollToTop, wait} from '@renop/ui/scroll';

const routes = [];
let currentCleanup = null;
let firstRender = true;
let rendering = false;
let previousPath = null;

/**
 * Register a path pattern and its page renderer.
 * @param {string} pattern - Exact path (`/docs`) or prefix with splat (`/docs/*`).
 * @param {(ctx: { path: string, params: Record<string, string>, root: HTMLElement, soft: boolean }) => (void|function|Promise<void|function>)} handler
 *   Page render function. May return a cleanup function (sync or async).
 * @returns {void}
 */
export function registerRoute(pattern, handler) {
    routes.push({pattern, handler});
}

/**
 * Match a pathname against registered routes.
 * @param {string} path - URL pathname.
 * @returns {{ route: { pattern: string, handler: Function }, params: Record<string, string> } | null}
 */
function matchRoute(path) {
    let p = path || '/';
    if (!p.startsWith('/')) p = '/' + p;
    if (p.length > 1 && p.endsWith('/')) p = p.slice(0, -1);

    for (const route of routes) {
        if (route.pattern === p) {
            return {route, params: {}};
        }
        if (route.pattern.endsWith('/*')) {
            const prefix = route.pattern.slice(0, -2);
            if (p === prefix) {
                return {route, params: {splat: ''}};
            }
            if (p.startsWith(prefix + '/')) {
                return {route, params: {splat: p.slice(prefix.length + 1)}};
            }
        }
    }
    return null;
}

/**
 * Section key used to decide soft vs full navigation.
 * Soft navigation reuses the page shell when staying in the same section (e.g. docs).
 * @param {string} path - URL pathname.
 * @returns {string} Section identifier (`home`, `docs`, `download`, …) or the path itself.
 */
function sectionKey(path) {
    let p = path || '/';
    if (!p.startsWith('/')) p = '/' + p;
    if (p.length > 1 && p.endsWith('/')) p = p.slice(0, -1);
    if (p === '/docs' || p.startsWith('/docs/')) return 'docs';
    if (p === '/download') return 'download';
    if (p === '/pricing') return 'pricing';
    if (p === '/contributors') return 'contributors';
    if (p === '/') return 'home';
    return p;
}

/**
 * Push or replace history and render the matching route.
 * @param {string} path - Absolute or relative site path.
 * @param {{ replace?: boolean }} [options]
 * @param {boolean} [options.replace=false] - Use `history.replaceState` instead of `pushState`.
 * @returns {Promise<void>}
 */
export function navigate(path, {replace = false} = {}) {
    const url = path.startsWith('/') ? path : '/' + path;
    if (replace) history.replaceState({}, '', url);
    else history.pushState({}, '', url);
    return renderRoute();
}

/**
 * Resolve the current pathname, run leave animation / cleanup when needed,
 * invoke the matched route handler, and play the enter animation.
 * Concurrent calls while a render is in progress are ignored.
 * @returns {Promise<void>}
 */
export async function renderRoute() {
    if (rendering) return;
    rendering = true;

    try {
        const path = location.pathname || '/';
        const matched = matchRoute(path) || matchRoute('/');
        const root = document.getElementById('page-root');
        if (!root || !matched) return;

        const soft =
            !firstRender &&
            previousPath != null &&
            sectionKey(previousPath) === sectionKey(path) &&
            sectionKey(path) === 'docs' &&
            root.querySelector('.docs-layout');

        previousPath = path;

        if (!soft) {
            if (!firstRender && root.childNodes.length) {
                root.classList.remove('page-enter');
                root.classList.add('page-leave');
                await wait(180);
            }
            firstRender = false;

            if (typeof currentCleanup === 'function') {
                try {
                    currentCleanup();
                } catch {
                    /* ignore */
                }
                currentCleanup = null;
            }

            root.classList.remove('page-leave');
            root.innerHTML = '';

            if ((window.scrollY || document.documentElement.scrollTop) > 0) {
                smoothScrollToTop(350);
            }
        }

        document.querySelectorAll('.nav-links a[data-link]').forEach((a) => {
            const href = a.getAttribute('href') || '';
            const active = href === path || (href !== '/' && path.startsWith(href));
            a.classList.toggle('active', active);
        });

        const result = await matched.route.handler({
            path,
            params: matched.params,
            root,
            soft,
        });

        if (typeof result === 'function') {
            currentCleanup = result;
        }

        if (!soft) {
            root.classList.remove('page-enter');
            void root.offsetWidth;
            root.classList.add('page-enter');
        }
    } finally {
        rendering = false;
    }
}

/**
 * Wire client-side navigation: intercept `a[data-link]` clicks and handle `popstate`.
 * @returns {void}
 */
export function initRouter() {
    document.addEventListener('click', (e) => {
        const a = e.target.closest('a[data-link]');
        if (!a) return;
        if (e.metaKey || e.ctrlKey || e.shiftKey || e.altKey || e.button !== 0) return;
        const href = a.getAttribute('href');
        if (!href || href.startsWith('http') || href.startsWith('//') || href.startsWith('#')) return;
        e.preventDefault();
        if (href !== location.pathname) navigate(href);
        else renderRoute();
    });

    window.addEventListener('popstate', () => {
        renderRoute();
    });
}
