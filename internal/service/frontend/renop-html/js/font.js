/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

const customFontFamily = 'RenoP Custom';
const googleFontsStylesheetHost = 'fonts.googleapis.com';
const fontLoadTimeoutMs = 15000;
const fallbackFontStack = 'system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", "Noto Sans", sans-serif';

/**
 * Resolves and validates the administrator-provided font URL before it reaches CSS.
 * @param {string} value - Raw URL read from the server-rendered metadata.
 * @returns {string} Absolute safe URL, or an empty string when invalid.
 */
function resolveFontUrl(value) {
    value = String(value || '').trim();
    if (!value || value.startsWith('//') || /\s/.test(value)) return '';
    if (!value.startsWith('/') && !/^https?:\/\//i.test(value)) return '';
    try {
        const parsed = new URL(value, window.location.origin);
        if (!['http:', 'https:'].includes(parsed.protocol) || parsed.username || parsed.password) return '';
        return parsed.href;
    } catch {
        return '';
    }
}

/**
 * Extracts the ordered family list from a Google Fonts CSS endpoint.
 * @param {string} fontUrl - Validated absolute stylesheet URL.
 * @returns {string[]} Safe family names in URL order.
 */
function googleFontFamilies(fontUrl) {
    try {
        const parsed = new URL(fontUrl);
        if (parsed.hostname.toLowerCase() !== googleFontsStylesheetHost) return [];
        const families = [];
        const seen = new Set();
        for (const declaration of parsed.searchParams.getAll('family')) {
            const family = declaration.split(':', 1)[0].trim();
            if (!family || family.length > 128 || /[\u0000-\u001f\u007f]/.test(family) || seen.has(family)) continue;
            seen.add(family);
            families.push(family);
        }
        return families.slice(0, 8);
    } catch {
        return [];
    }
}

/**
 * Quotes one trusted font family for use in a CSS font-family value.
 * @param {string} family - Validated family name.
 * @returns {string} CSS string token.
 */
function quoteFontFamily(family) {
    return `"${family.replace(/\\/g, '\\\\').replace(/"/g, '\\"')}"`;
}

/**
 * Applies loaded families through an inline custom property so the computed root family changes immediately.
 * @param {string[]} families - Loaded family names ordered by preference.
 * @returns {void}
 */
function activateFontFamilies(families) {
    if (!families.length) return;
    const root = document.documentElement;
    root.style.setProperty('--font-sans', `${families.map(quoteFontFamily).join(', ')}, ${fallbackFontStack}`);
    root.dataset.customFontLoaded = 'true';
}

/**
 * Resolves a promise with null when a font provider does not finish promptly.
 * @template T
 * @param {Promise<T>} operation - Font loading operation.
 * @returns {Promise<T|null>} Result or null after the bounded timeout.
 */
function withFontTimeout(operation) {
    let timeoutId;
    const timeout = new Promise(resolve => {
        timeoutId = window.setTimeout(() => resolve(null), fontLoadTimeoutMs);
    });
    return Promise.race([operation, timeout]).finally(() => window.clearTimeout(timeoutId));
}

/**
 * Loads a Google Fonts stylesheet after first paint, then waits for its primary family before activation.
 * @param {string} fontUrl - Validated Google Fonts CSS endpoint.
 * @param {string[]} families - Ordered families declared by the endpoint URL.
 * @returns {Promise<boolean>} Whether at least the primary family became usable.
 */
function loadGoogleFontsStylesheet(fontUrl, families) {
    if (!document.fonts || !families.length) return Promise.resolve(false);
    return new Promise(resolve => {
        const link = document.createElement('link');
        let settled = false;
        const finish = value => {
            if (settled) return;
            settled = true;
            window.clearTimeout(stylesheetTimeoutId);
            resolve(value);
        };
        const stylesheetTimeoutId = window.setTimeout(() => {
            link.remove();
            finish(false);
        }, fontLoadTimeoutMs);
        link.rel = 'stylesheet';
        link.href = fontUrl;
        link.media = 'print';
        link.dataset.renopCustomFont = 'google';
        link.referrerPolicy = 'no-referrer';
        link.onerror = () => {
            link.remove();
            finish(false);
        };
        link.onload = async () => {
            if (settled) return;
            window.clearTimeout(stylesheetTimeoutId);
            link.media = 'all';
            await new Promise(next => requestAnimationFrame(next));
            const faces = await withFontTimeout(document.fonts.load(`1em ${quoteFontFamily(families[0])}`, 'Aa0'));
            if (settled) return;
            if (Array.isArray(faces) && faces.length > 0) {
                activateFontFamilies(families);
                finish(true);
                return;
            }
            finish(false);
        };
        document.head.appendChild(link);
    });
}

/**
 * Downloads one direct font file without participating in the render-blocking stylesheet path.
 * @param {string} fontUrl - Validated absolute font resource URL.
 * @returns {Promise<boolean>} Whether the font became usable.
 */
async function loadDirectFont(fontUrl) {
    if (typeof FontFace !== 'function' || !document.fonts) return false;
    const face = new FontFace(customFontFamily, `url(${JSON.stringify(fontUrl)})`);
    const loaded = await withFontTimeout(face.load());
    if (!loaded) return false;
    document.fonts.add(loaded);
    activateFontFamilies([customFontFamily]);
    return true;
}

/**
 * Loads either a direct font resource or a supported provider stylesheet.
 * @param {string} fontUrl - Validated absolute URL.
 * @returns {Promise<void>}
 */
async function loadConfiguredFont(fontUrl) {
    const root = document.documentElement;
    if (root.dataset.customFontLoading === 'true' || root.dataset.customFontLoaded === 'true') return;
    root.dataset.customFontLoading = 'true';
    try {
        const families = googleFontFamilies(fontUrl);
        if (families.length) {
            await loadGoogleFontsStylesheet(fontUrl, families);
        } else {
            await loadDirectFont(fontUrl);
        }
    } catch {
        delete root.dataset.customFontLoaded;
    } finally {
        delete root.dataset.customFontLoading;
    }
}

/**
 * Schedules the configured custom font after the initial application modules can render.
 * @returns {void}
 */
export function initConfiguredFont() {
    const root = document.documentElement;
    if (root.dataset.fontPreset !== 'custom') return;
    const meta = document.querySelector('meta[name="renop-font-url"]');
    const fontUrl = resolveFontUrl(meta?.getAttribute('content'));
    if (!fontUrl) return;
    const start = () => void loadConfiguredFont(fontUrl);
    if (typeof window.requestIdleCallback === 'function') {
        window.requestIdleCallback(start, {timeout: 1500});
    } else {
        window.setTimeout(start, 0);
    }
}
