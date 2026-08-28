/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

const customFontFamily = 'RenoP Custom';

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
 * Downloads a custom font without participating in the render-blocking stylesheet path.
 * The active family changes only after the complete font has loaded successfully.
 * @param {string} fontUrl - Validated absolute font resource URL.
 * @returns {Promise<void>}
 */
async function loadCustomFont(fontUrl) {
    if (typeof FontFace !== 'function' || !document.fonts) return;
    const root = document.documentElement;
    if (root.dataset.customFontLoading === 'true' || root.dataset.customFontLoaded === 'true') return;
    root.dataset.customFontLoading = 'true';
    try {
        const face = new FontFace(customFontFamily, `url(${JSON.stringify(fontUrl)})`);
        const loaded = await face.load();
        document.fonts.add(loaded);
        root.dataset.customFontLoaded = 'true';
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
    const start = () => void loadCustomFont(fontUrl);
    if (typeof window.requestIdleCallback === 'function') {
        window.requestIdleCallback(start, {timeout: 1500});
    } else {
        window.setTimeout(start, 0);
    }
}
