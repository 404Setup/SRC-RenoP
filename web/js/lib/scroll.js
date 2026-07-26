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
 * Smooth scroll helpers (from frontend/renop-html app-ui.js).
 */

let scrollToTopTimer = null;

/**
 * Smoothly scroll the window to the top while temporarily locking main content min-height
 * so the page does not collapse under the scroll position mid-animation.
 * @param {number} [duration=350] - Approximate animation duration in ms (for min-height restore).
 * @returns {void}
 */
export function smoothScrollToTop(duration = 350) {
    const startY = window.scrollY || window.pageYOffset || document.documentElement.scrollTop || 0;
    if (startY <= 0) return;

    const mainEl = document.querySelector('main') || document.querySelector('#page-root') || document.body;
    const originalMinHeight = mainEl.style.minHeight;
    const currentTotalHeight = Math.max(
        document.body.scrollHeight,
        document.documentElement.scrollHeight,
        startY + window.innerHeight,
    );

    mainEl.style.minHeight = `${currentTotalHeight}px`;

    try {
        window.scrollTo({ top: 0, behavior: 'smooth' });
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
