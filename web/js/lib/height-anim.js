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
 * Height morph animations (ported from frontend/renop-html browser/utils.js).
 * Used when content swaps would otherwise snap layout height.
 *
 * Shrink animations use the Web Animations API when available — CSS
 * `height` transitions are unreliable when going from tall → short after a
 * DOM swap (browser may skip the transition or measure incorrectly).
 */

const HEIGHT_EASE = 'cubic-bezier(0.16, 1, 0.3, 1)';
const heightAnimTokens = new WeakMap();

/**
 * Whether the user prefers reduced motion (system accessibility setting).
 * @returns {boolean}
 */
export function prefersReducedMotion() {
    return typeof window !== 'undefined'
        && window.matchMedia
        && window.matchMedia('(prefers-reduced-motion: reduce)').matches;
}

/**
 * Bump the per-element animation generation token so older animations can self-abort.
 * @param {HTMLElement} el
 * @returns {number} New token for this element.
 */
function nextHeightAnimToken(el) {
    const token = (heightAnimTokens.get(el) || 0) + 1;
    heightAnimTokens.set(el, token);
    return token;
}

/**
 * @param {HTMLElement} el
 * @param {number} token
 * @returns {boolean} True if `token` is still the latest for `el`.
 */
function isHeightAnimCurrent(el, token) {
    return heightAnimTokens.get(el) === token;
}

/**
 * Clear inline styles applied during height morphing.
 * @param {HTMLElement} el
 * @returns {void}
 */
function clearHeightInlineStyles(el) {
    el.style.height = '';
    el.style.overflow = '';
    el.style.opacity = '';
    el.style.marginTop = '';
    el.style.transition = '';
    el.style.boxSizing = '';
}

/**
 * Measure an element's natural (content) height, restoring prior inline height.
 * Forces `height: auto` so shrink targets are not stuck on a previous locked height.
 * @param {HTMLElement|null|undefined} el
 * @returns {number} Content height in CSS pixels.
 */
export function measureNaturalHeight(el) {
    if (!el) return 0;
    const prevHeight = el.style.height;
    const prevOverflow = el.style.overflow;
    const prevMinHeight = el.style.minHeight;
    el.style.minHeight = '0';
    el.style.height = 'auto';
    el.style.overflow = 'hidden';
    void el.offsetHeight;
    const height = el.getBoundingClientRect().height;
    el.style.height = prevHeight;
    el.style.overflow = prevOverflow;
    el.style.minHeight = prevMinHeight;
    return height;
}

/**
 * Lock current height so later content swaps don't jump the layout.
 * @param {HTMLElement|null|undefined} el
 * @returns {number} Locked height in CSS pixels, or 0 if `el` is missing.
 */
export function lockElementHeight(el) {
    if (!el) return 0;
    const height = el.getBoundingClientRect().height;
    el.style.boxSizing = 'border-box';
    el.style.height = `${Math.max(height, 0)}px`;
    el.style.overflow = 'hidden';
    return height;
}

/**
 * Cancel any in-flight Web Animations on the element.
 * @param {HTMLElement} el
 * @returns {void}
 */
function cancelWaapi(el) {
    try {
        const anims = typeof el.getAnimations === 'function' ? el.getAnimations() : [];
        for (const a of anims) {
            try {
                a.cancel();
            } catch {
                /* ignore */
            }
        }
    } catch {
        /* ignore */
    }
}

/**
 * Animate element height from its current locked/computed height to the natural
 * height of its current content. Optionally run `mutate` before measuring.
 * Works for both grow and shrink. Prefers the Web Animations API; falls back to CSS transitions.
 * @param {HTMLElement|null|undefined} el - Element to animate (no-op morph if missing; `mutate` still runs).
 * @param {(() => void)|null|undefined} mutate - DOM mutation run after locking start height.
 * @param {{ duration?: number, easing?: string, alsoOpacity?: boolean }} [options]
 * @param {number} [options.duration=340] - Animation duration in ms.
 * @param {string} [options.easing] - CSS easing string.
 * @param {boolean} [options.alsoOpacity=false] - Also fade opacity to 1 during the morph.
 * @returns {Promise<void>} Resolves when the animation finishes or is superseded.
 */
export function morphElementHeight(el, mutate, {
    duration = 340,
    easing = HEIGHT_EASE,
    alsoOpacity = false,
} = {}) {
    if (!el) {
        if (typeof mutate === 'function') mutate();
        return Promise.resolve();
    }

    const token = nextHeightAnimToken(el);
    cancelWaapi(el);

    if (prefersReducedMotion()) {
        if (typeof mutate === 'function') mutate();
        clearHeightInlineStyles(el);
        return Promise.resolve();
    }

    el.style.boxSizing = 'border-box';

    const from = el.getBoundingClientRect().height || parseFloat(el.style.height) || 0;
    el.style.height = `${from}px`;
    el.style.overflow = 'hidden';
    void el.offsetHeight;

    if (typeof mutate === 'function') mutate();

    const to = measureNaturalHeight(el);
    el.style.height = `${from}px`;
    void el.offsetHeight;

    if (Math.abs(from - to) < 0.5) {
        clearHeightInlineStyles(el);
        return Promise.resolve();
    }

    if (typeof el.animate === 'function') {
        const keyframes = alsoOpacity
            ? [
                { height: `${from}px`, opacity: el.style.opacity || '1' },
                { height: `${to}px`, opacity: '1' },
            ]
            : [
                { height: `${from}px` },
                { height: `${to}px` },
            ];

        let anim;
        try {
            anim = el.animate(keyframes, {
                duration,
                easing,
                fill: 'forwards',
            });
        } catch {
            anim = null;
        }

        if (anim) {
            return new Promise((resolve) => {
                let settled = false;
                /** Settle WAAPI morph: lock end height, cancel animation, clear styles. */
                const finish = () => {
                    if (settled) return;
                    settled = true;
                    if (!isHeightAnimCurrent(el, token)) {
                        resolve();
                        return;
                    }
                    el.style.height = `${to}px`;
                    el.style.overflow = 'hidden';
                    el.style.boxSizing = 'border-box';
                    try {
                        anim.cancel();
                    } catch {
                    }
                    requestAnimationFrame(() => {
                        if (!isHeightAnimCurrent(el, token)) {
                            resolve();
                            return;
                        }
                        clearHeightInlineStyles(el);
                        resolve();
                    });
                };

                if (anim.finished && typeof anim.finished.then === 'function') {
                    anim.finished.then(finish).catch(finish);
                } else {
                    anim.onfinish = finish;
                    anim.oncancel = () => {
                        if (!settled) {
                            settled = true;
                            resolve();
                        }
                    };
                }
                setTimeout(finish, duration + 80);
            });
        }
    }

    const props = [`height ${duration}ms ${easing}`];
    if (alsoOpacity) props.push(`opacity ${Math.min(duration, 280)}ms ease`);
    el.style.transition = props.join(', ');
    void el.offsetHeight;

    return new Promise((resolve) => {
        let settled = false;
        /** Settle CSS-transition morph and clear listeners. */
        const finish = () => {
            if (settled) return;
            settled = true;
            el.removeEventListener('transitionend', onEnd);
            if (!isHeightAnimCurrent(el, token)) {
                resolve();
                return;
            }
            clearHeightInlineStyles(el);
            resolve();
        };
        /** @param {TransitionEvent} e */
        const onEnd = (e) => {
            if (e.target === el && e.propertyName === 'height') finish();
        };
        el.addEventListener('transitionend', onEnd);

        requestAnimationFrame(() => {
            if (!isHeightAnimCurrent(el, token)) {
                finish();
                return;
            }
            requestAnimationFrame(() => {
                if (!isHeightAnimCurrent(el, token)) {
                    finish();
                    return;
                }
                el.style.height = `${to}px`;
                if (alsoOpacity) el.style.opacity = '1';
            });
        });
        setTimeout(finish, duration + 80);
    });
}
