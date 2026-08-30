/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

export const MAX_PRIVACY_POLICY_BYTES = 512 << 10;

/**
 * Read one successful UTF-8 plain-text privacy policy within the shared size limit.
 * @param {Response} response - Same-origin policy response.
 * @returns {Promise<string>} Validated policy text.
 */
export async function readPrivacyPolicyResponse(response) {
    if (!response?.ok) throw new Error('privacy policy request failed');
    const contentType = String(response.headers.get('Content-Type') || '').split(';', 1)[0].trim().toLowerCase();
    if (contentType && contentType !== 'text/plain') throw new Error('privacy policy response is not plain text');
    const declaredLength = Number(response.headers.get('Content-Length'));
    if (Number.isFinite(declaredLength) && declaredLength > MAX_PRIVACY_POLICY_BYTES) {
        throw new Error('privacy policy response exceeds the size limit');
    }
    if (!response.body?.getReader) throw new Error('privacy policy response is not streamable');

    const reader = response.body.getReader();
    const decoder = new TextDecoder('utf-8', {fatal: true});
    let consumed = 0;
    let output = '';
    try {
        while (true) {
            const {done, value} = await reader.read();
            if (done) break;
            consumed += value.byteLength;
            if (consumed > MAX_PRIVACY_POLICY_BYTES) {
                await reader.cancel();
                throw new Error('privacy policy response exceeds the size limit');
            }
            output += decoder.decode(value, {stream: true});
        }
        output += decoder.decode();
    } finally {
        reader.releaseLock();
    }
    if (!output || output.includes('\0')) throw new Error('privacy policy response is invalid');
    return output;
}
