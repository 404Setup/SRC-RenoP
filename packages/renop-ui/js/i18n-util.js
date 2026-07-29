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
 * Match a raw language tag against a registry of known keys.
 * Handles exact match, Chinese family heuristics, then base-language prefix.
 *
 * @param {string|null|undefined} lang - Raw tag (e.g. `zh`, `en-US`, `yue-HK`).
 * @param {Record<string, *>|string[]} languages - Map or list of accepted codes/aliases.
 * @returns {string|null} Matched registry key, or null.
 */
export function matchLocaleKey(lang, languages) {
    if (!lang || typeof lang !== 'string') return null;
    const clean = lang.trim();
    if (!clean) return null;

    const keys = Array.isArray(languages) ? languages : Object.keys(languages || {});
    if (!keys.length) return null;

    const cleanLower = clean.toLowerCase();

    for (const key of keys) {
        if (key.toLowerCase() === cleanLower) return key;
    }

    if (cleanLower.startsWith('zh') || cleanLower.startsWith('yue')) {
        if (keys.includes('zh-YUE') && (cleanLower.includes('yue') || cleanLower.includes('cantonese'))) {
            return 'zh-YUE';
        }
        if (keys.includes('zh-HK') && cleanLower.includes('hk')) return 'zh-HK';
        if (keys.includes('zh-TW') && (cleanLower.includes('tw') || cleanLower.includes('hant'))) {
            return 'zh-TW';
        }
        if (keys.includes('zh-CN')) {
            if (
                cleanLower.includes('cn')
                || cleanLower.includes('hans')
                || cleanLower.includes('sg')
                || cleanLower === 'zh'
                || cleanLower.startsWith('zh-')
            ) {
                return 'zh-CN';
            }
        }
        // Prefer first zh* key if present
        const zhKey = keys.find((k) => k.toLowerCase().startsWith('zh'));
        if (zhKey) return zhKey;
    }

    const base = clean.split(/[-_]/)[0].toLowerCase();
    if (!base) return null;
    for (const key of keys) {
        if (key.toLowerCase() === base || key.toLowerCase().startsWith(`${base}-`)) {
            return key;
        }
    }
    return null;
}

/**
 * Detect preferred language from localStorage, then `navigator.languages`, then default.
 *
 * @param {object} options
 * @param {string} options.storageKey - localStorage key.
 * @param {(raw: string) => string|null} options.resolve - Maps raw tags to a supported code.
 * @param {string} options.defaultLang - Fallback code.
 * @returns {{ lang: string, source: 'localStorage'|'browser'|'fallback' }}
 */
export function detectLanguage({storageKey, resolve, defaultLang}) {
    const saved = typeof localStorage !== 'undefined' ? localStorage.getItem(storageKey) : null;
    if (saved) {
        const resolved = resolve(saved);
        if (resolved) return {lang: resolved, source: 'localStorage'};
    }

    const browserLangs = (typeof navigator !== 'undefined' && navigator.languages?.length)
        ? navigator.languages
        : (typeof navigator !== 'undefined' ? [navigator.language] : []);

    for (const bLang of browserLangs) {
        if (!bLang) continue;
        const resolved = resolve(bLang);
        if (resolved) return {lang: resolved, source: 'browser'};
    }

    return {lang: defaultLang, source: 'fallback'};
}

/**
 * Interpolate `{name}` placeholders in a message string.
 * @param {string} text
 * @param {Record<string, *>} [params={}]
 * @returns {string}
 */
export function interpolateMessage(text, params = {}) {
    if (typeof text !== 'string' || !params || typeof params !== 'object') return text;
    let out = text;
    for (const [pKey, pVal] of Object.entries(params)) {
        out = out.replace(new RegExp(`\\{${pKey}\\}`, 'g'), String(pVal));
    }
    return out;
}

/**
 * Look up a message key in current then fallback dictionaries, with interpolation.
 *
 * @param {string} key
 * @param {Record<string, *>} [params={}]
 * @param {{ current?: Record<string, string>, fallback?: Record<string, string> }} dicts
 * @returns {string}
 */
export function translateKey(key, params = {}, {current = {}, fallback = {}} = {}) {
    const text = current[key] || fallback[key] || key;
    return interpolateMessage(text, params);
}

/**
 * Animate the header language button label width when the display name changes.
 *
 * @param {object} opts
 * @param {HTMLElement|null|undefined} opts.nameEl - `#current-lang-name`
 * @param {HTMLElement|null|undefined} opts.btn - `#lang-btn`
 * @param {string} opts.newName - Target label text.
 * @returns {void}
 */
export function animateLangButtonLabel({nameEl, btn, newName}) {
    if (!nameEl) return;

    const oldName = nameEl.textContent.trim();
    if (oldName && oldName !== newName && btn && document.readyState !== 'loading') {
        const startWidth = btn.getBoundingClientRect().width;

        btn.style.width = `${startWidth}px`;
        btn.classList.add('is-animating');
        nameEl.classList.add('lang-name-changing');

        setTimeout(() => {
            nameEl.textContent = newName;

            btn.style.width = 'auto';
            const targetWidth = btn.getBoundingClientRect().width;

            btn.style.width = `${startWidth}px`;
            void btn.offsetWidth;

            btn.style.width = `${targetWidth}px`;
            nameEl.classList.remove('lang-name-changing');
            nameEl.classList.add('lang-name-changed');

            setTimeout(() => {
                btn.style.width = '';
                btn.classList.remove('is-animating');
                nameEl.classList.remove('lang-name-changed');
            }, 300);
        }, 120);
    } else {
        nameEl.textContent = newName;
    }
}

/**
 * Toggle `.active` on `.lang-card` nodes to match the current language.
 * @param {ParentNode|null|undefined} grid - Usually `#language-grid`
 * @param {string} currentLang
 * @returns {void}
 */
export function syncLangCardsActive(grid, currentLang) {
    if (!grid) return;
    grid.querySelectorAll('.lang-card').forEach((card) => {
        const lKey = card.getAttribute('data-lang');
        card.classList.toggle('active', lKey === currentLang);
    });
}
