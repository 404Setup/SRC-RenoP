/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {$} from './jquery.js';

let scrollToTopTimer = null;
const dragScrollContainers = new WeakSet();
const DRAG_DISTANCE_PX = 8;
const MOUSE_DRAG_HOLD_MS = 180;

/**
 * Smoothly scroll the window to the top while temporarily locking main content
 * min-height so the page does not collapse under the scroll position mid-animation.
 *
 * @param {number} [duration=350] - Approximate animation duration in ms (for min-height restore).
 * @param {{ mainSelectors?: string[] }} [options]
 * @param {string[]} [options.mainSelectors] - Query selectors tried in order for the height lock target.
 * @returns {void}
 */
export function smoothScrollToTop(duration = 350, {
    mainSelectors = ['main', '#page-root', '.container', 'body'],
} = {}) {
    const startY = window.scrollY || window.pageYOffset || document.documentElement.scrollTop || 0;
    if (startY <= 0) return;

    let mainEl = document.body;
    for (const sel of mainSelectors) {
        const found = $(sel).get(0);
        if (found) {
            mainEl = found;
            break;
        }
    }

    const originalMinHeight = mainEl.style.minHeight;
    const currentTotalHeight = Math.max(
        document.body.scrollHeight,
        document.documentElement.scrollHeight,
        startY + window.innerHeight,
    );

    mainEl.style.minHeight = `${currentTotalHeight}px`;

    try {
        window.scrollTo({top: 0, behavior: 'smooth'});
    } catch {
        window.scrollTo(0, 0);
    }

    if (scrollToTopTimer) clearTimeout(scrollToTopTimer);
    scrollToTopTimer = setTimeout(() => {
        mainEl.style.minHeight = originalMinHeight;
    }, duration + 100);
}

/**
 * Promise-based delay.
 * @param {number} ms - Milliseconds to wait.
 * @returns {Promise<void>}
 */
export function wait(ms) {
    return new Promise((resolve) => setTimeout(resolve, ms));
}

/**
 * Enable click-and-drag horizontal scrolling on an overflow container.
 * Suppresses click events that follow a drag so nested links are not activated.
 * @param {HTMLElement|null|undefined} container - Scrollable element.
 * @returns {void}
 */
export function enableDragToScroll(container) {
    if (!container || dragScrollContainers.has(container)) return;
    dragScrollContainers.add(container);

    let isDown = false;
    let isDragging = false;
    let startX;
    let startY;
    let scrollLeft;
    let activePointerId = null;
    let activeScrollTarget = null;
    let dragReady = false;
    let holdTimer = null;
    let dragFrame = null;
    let pendingScrollLeft = 0;
    const $container = $(container);

    const getScrollTarget = () => {
        const tabsContainer = $container.closest('.tabs-container').get(0);
        if (tabsContainer && tabsContainer !== container
            && tabsContainer.scrollWidth > tabsContainer.clientWidth + 1) {
            return tabsContainer;
        }
        if (container.scrollWidth > container.clientWidth + 1) {
            return container;
        }
        const parent = tabsContainer || container.parentElement;
        if (parent && parent.scrollWidth > parent.clientWidth + 1) {
            return parent;
        }
        return container;
    };

    const stopDragging = (e) => {
        if (!isDown) return;
        isDown = false;
        if (holdTimer !== null) {
            clearTimeout(holdTimer);
            holdTimer = null;
        }
        if (activeScrollTarget) {
            if (dragFrame !== null) {
                cancelAnimationFrame(dragFrame);
                dragFrame = null;
                if (isDragging) activeScrollTarget.scrollLeft = pendingScrollLeft;
            }
            $(activeScrollTarget).css('cursor', '').removeClass('is-dragging');
        }
        if (e && activePointerId !== null && container.releasePointerCapture) {
            try {
                if (!container.hasPointerCapture || container.hasPointerCapture(activePointerId)) {
                    container.releasePointerCapture(activePointerId);
                }
            } catch {}
        }
        activePointerId = null;
        activeScrollTarget = null;
        dragReady = false;
        setTimeout(() => {
            isDragging = false;
        }, 0);
    };

    container.addEventListener('pointerdown', (e) => {
        if (e.button !== 0) return;
        const scrollTarget = getScrollTarget();
        if (scrollTarget.scrollWidth <= scrollTarget.clientWidth + 1) return;
        isDown = true;
        isDragging = false;
        startX = e.clientX;
        startY = e.clientY;
        scrollLeft = scrollTarget.scrollLeft;
        pendingScrollLeft = scrollLeft;
        activePointerId = e.pointerId;
        activeScrollTarget = scrollTarget;
        dragReady = e.pointerType !== 'mouse';
        if (!dragReady) {
            holdTimer = setTimeout(() => {
                dragReady = true;
                holdTimer = null;
            }, MOUSE_DRAG_HOLD_MS);
        }
    });

    container.addEventListener('pointerleave', (e) => {
        if (isDown && !isDragging) stopDragging(e);
    });
    container.addEventListener('pointerup', stopDragging);
    container.addEventListener('pointercancel', stopDragging);

    container.addEventListener('pointermove', (e) => {
        if (!isDown || e.pointerId !== activePointerId || !activeScrollTarget) return;
        const deltaX = e.clientX - startX;
        const deltaY = e.clientY - startY;

        if (!isDragging && Math.abs(deltaY) > Math.abs(deltaX) && Math.abs(deltaY) > DRAG_DISTANCE_PX) {
            stopDragging(e);
            return;
        }

        if (!isDragging && dragReady && Math.abs(deltaX) > DRAG_DISTANCE_PX) {
            isDragging = true;
            startX = e.clientX;
            startY = e.clientY;
            scrollLeft = activeScrollTarget.scrollLeft;
            pendingScrollLeft = scrollLeft;
            if (container.setPointerCapture) {
                try {
                    container.setPointerCapture(e.pointerId);
                } catch {}
            }
            if (window.getSelection) {
                window.getSelection().removeAllRanges();
            }
            $(activeScrollTarget).css('cursor', 'grabbing').addClass('is-dragging');
        }

        if (isDragging) {
            e.preventDefault();
            pendingScrollLeft = scrollLeft - (e.clientX - startX);
            if (dragFrame === null) {
                dragFrame = requestAnimationFrame(() => {
                    dragFrame = null;
                    if (activeScrollTarget) activeScrollTarget.scrollLeft = pendingScrollLeft;
                });
            }
        }
    });

    container.addEventListener('click', (e) => {
        if (isDragging) {
            e.preventDefault();
            e.stopPropagation();
        }
    }, true);

    container.addEventListener('dragstart', (e) => {
        if (isDown) e.preventDefault();
    });
}
