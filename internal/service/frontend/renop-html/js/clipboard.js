/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the terms of the Mozilla Public License, v. 2.0.
 */

/**
 * Copy plain text with the modern Clipboard API and a bounded legacy fallback.
 * @param {unknown} value - Value to copy.
 * @returns {Promise<void>}
 */
export async function writeClipboardText(value) {
    const text = String(value ?? '');
    if (!text) throw new TypeError('Clipboard text must not be empty');
    if (navigator.clipboard?.writeText) {
        try {
            await navigator.clipboard.writeText(text);
            return;
        } catch {
        }
    }
    const textarea = document.createElement('textarea');
    textarea.value = text;
    textarea.readOnly = true;
    textarea.setAttribute('aria-hidden', 'true');
    Object.assign(textarea.style, {
        position: 'fixed',
        inset: '0 auto auto -9999px',
        opacity: '0',
        pointerEvents: 'none'
    });
    document.body.appendChild(textarea);
    textarea.select();
    let copied = false;
    try {
        copied = typeof document.execCommand === 'function' && document.execCommand('copy');
    } finally {
        textarea.remove();
    }
    if (!copied) throw new Error('Clipboard API is unavailable');
}
