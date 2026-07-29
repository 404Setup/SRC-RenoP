/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

let scrollToTopTimer = null;

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
        const found = document.querySelector(sel);
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
    if (!container) return;
    let isDown = false;
    let isDragging = false;
    let startX;
    let scrollLeft;

    const stopDragging = () => {
        isDown = false;
        container.style.cursor = '';
        container.classList.remove('is-dragging');
        setTimeout(() => {
            isDragging = false;
        }, 0);
    };

    container.addEventListener('pointerdown', (e) => {
        if (e.button !== 0) return;
        if (container.scrollWidth <= container.clientWidth) return;
        isDown = true;
        isDragging = false;
        startX = e.pageX - container.offsetLeft;
        scrollLeft = container.scrollLeft;
    });

    container.addEventListener('pointerleave', stopDragging);
    container.addEventListener('pointerup', stopDragging);
    container.addEventListener('pointercancel', stopDragging);

    container.addEventListener('pointermove', (e) => {
        if (!isDown) return;
        const x = e.pageX - container.offsetLeft;
        const walk = (x - startX) * 1.5;
        if (Math.abs(walk) > 5) {
            if (!isDragging) {
                isDragging = true;
                if (window.getSelection) {
                    window.getSelection().removeAllRanges();
                }
            }
            container.style.cursor = 'grabbing';
            container.classList.add('is-dragging');
            e.preventDefault();
        }
        if (isDragging) {
            container.scrollLeft = scrollLeft - walk;
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
