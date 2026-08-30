/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {el} from './dom.js';
import {morphElementHeight} from './height-anim.js';
import {$} from './jquery.js';

/**
 * Clamp a zero-based page to a bounded collection.
 * @param {number} page - Requested zero-based page.
 * @param {number} totalItems - Collection size.
 * @param {number} pageSize - Items per page.
 * @returns {number} Safe zero-based page.
 */
export function clampCollectionPage(page, totalItems, pageSize) {
    const pages = Math.max(1, Math.ceil(Math.max(0, totalItems) / Math.max(1, pageSize)));
    return Math.max(0, Math.min(Math.trunc(Number(page) || 0), pages - 1));
}

/**
 * Create a responsive previous/summary/next pager without dense numbered buttons.
 * @param {object} options - Pager options.
 * @param {number} options.page - Zero-based current page.
 * @param {number} options.totalItems - Collection size.
 * @param {number} options.pageSize - Items per page.
 * @param {string} options.previousLabel - Previous button label.
 * @param {string} options.nextLabel - Next button label.
 * @param {(state: {page: number, pages: number, total: number}) => string} options.summary - Summary formatter.
 * @param {(page: number) => void} options.onPageChange - Page selection callback.
 * @returns {HTMLElement|null} Pager, or null for a single page.
 */
function createCollectionPager({
    page,
    totalItems,
    pageSize,
    previousLabel,
    nextLabel,
    summary,
    onPageChange,
}) {
    const pages = Math.max(1, Math.ceil(totalItems / pageSize));
    if (pages <= 1) return null;
    const previous = el('button', {
        type: 'button', class: 'renop-pagination-btn', disabled: page === 0,
        'aria-label': previousLabel,
    }, previousLabel);
    const next = el('button', {
        type: 'button', class: 'renop-pagination-btn', disabled: page >= pages - 1,
        'aria-label': nextLabel,
    }, nextLabel);
    $(previous).on('click', () => onPageChange(page - 1));
    $(next).on('click', () => onPageChange(page + 1));
    const status = summary({page: page + 1, pages, total: totalItems});
    return el('nav', {class: 'renop-pagination', 'aria-label': status},
        previous,
        el('span', {class: 'renop-pagination-summary', 'aria-live': 'polite'}, status),
        next
    );
}

/**
 * Render an in-memory collection in bounded animated pages.
 * @template T
 * @param {object} options - Collection options.
 * @param {HTMLElement} options.list - Page item host.
 * @param {HTMLElement} options.pager - Pager host.
 * @param {T[]} options.items - Initial items.
 * @param {number} [options.pageSize=8] - Items per page.
 * @param {number} [options.initialPage=0] - Initial zero-based page.
 * @param {(item: T, index: number) => Node|null} options.renderItem - Item renderer.
 * @param {() => Node|null} [options.renderEmpty] - Empty-state renderer.
 * @param {string} options.previousLabel - Previous button label.
 * @param {string} options.nextLabel - Next button label.
 * @param {(state: {page: number, pages: number, total: number}) => string} options.summary - Summary formatter.
 * @param {(page: number) => void} [options.onPageChanged] - Optional page-state observer.
 * @returns {{page: () => number, setPage: (page: number) => Promise<void>, setItems: (items: T[], preservePage?: boolean) => Promise<void>}}
 *   Collection controller.
 */
export function createPaginatedCollection({
    list,
    pager,
    items,
    pageSize = 8,
    initialPage = 0,
    renderItem,
    renderEmpty,
    previousLabel,
    nextLabel,
    summary,
    onPageChanged,
}) {
    let collection = Array.isArray(items) ? items.slice() : [];
    const boundedPageSize = Math.max(1, Math.min(100, Math.trunc(Number(pageSize) || 8)));
    let currentPage = clampCollectionPage(initialPage, collection.length, boundedPageSize);

    const replacePage = () => {
        const start = currentPage * boundedPageSize;
        const pageItems = collection.slice(start, start + boundedPageSize);
        const nodes = pageItems.map((item, index) => renderItem(item, start + index)).filter(Boolean);
        if (nodes.length === 0 && typeof renderEmpty === 'function') {
            const empty = renderEmpty();
            if (empty) nodes.push(empty);
        }
        list.replaceChildren(...nodes);
        $(list).removeClass('renop-page-enter');
        void list.offsetWidth;
        $(list).addClass('renop-page-enter');
        $(pager).empty();
        const control = createCollectionPager({
            page: currentPage,
            totalItems: collection.length,
            pageSize: boundedPageSize,
            previousLabel,
            nextLabel,
            summary,
            onPageChange: page => void setPage(page),
        });
        if (control) $(pager).append(control);
    };

    const render = animate => animate
        ? morphElementHeight(list, replacePage, {duration: 260})
        : (replacePage(), Promise.resolve());

    /**
     * Select and render a page.
     * @param {number} page - Requested zero-based page.
     * @returns {Promise<void>} Render completion.
     */
    function setPage(page) {
        const nextPage = clampCollectionPage(page, collection.length, boundedPageSize);
        if (nextPage === currentPage) return Promise.resolve();
        currentPage = nextPage;
        if (typeof onPageChanged === 'function') onPageChanged(currentPage);
        return render(true);
    }

    if (typeof onPageChanged === 'function') onPageChanged(currentPage);
    replacePage();
    return {
        page: () => currentPage,
        setPage,
        setItems(nextItems, preservePage = true) {
            collection = Array.isArray(nextItems) ? nextItems.slice() : [];
            currentPage = clampCollectionPage(preservePage ? currentPage : 0,
                collection.length, boundedPageSize);
            if (typeof onPageChanged === 'function') onPageChanged(currentPage);
            return render(true);
        },
    };
}
