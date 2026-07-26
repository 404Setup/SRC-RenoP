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
 * Extract platform-specific package from multi-arch nightly zip (browser-side).
 * Layout matches updater/installer.go nested zip support.
 */

import JSZip from 'jszip';

/**
 * Whether an archive entry name contains a `{goos}-{goarch}` platform marker.
 * @param {string} name - Zip entry path or filename.
 * @param {string} goos - GOOS value (e.g. `linux`).
 * @param {string} goarch - GOARCH value (e.g. `amd64`).
 * @returns {boolean}
 */
function archiveMatchesPlatform(name, goos, goarch) {
    const nameLower = String(name || '').toLowerCase().replace(/\\/g, '/');
    const marker = `${String(goos).toLowerCase()}-${String(goarch).toLowerCase()}`;

    for (const src of [nameLower.split('/').pop() || nameLower, nameLower]) {
        const idx = src.indexOf(marker);
        if (idx < 0) continue;
        const end = idx + marker.length;
        if (end >= src.length) return true;
        const ch = src[end];
        if (ch === '.' || ch === '-' || ch === '_' || ch === '/') return true;
    }
    return false;
}

/**
 * Extract the nested platform package zip from a multi-arch nightly outer zip.
 * @param {ArrayBuffer} outerZipBuffer - Outer `renop-nightly.zip` contents.
 * @param {string} goos - Target GOOS.
 * @param {string} goarch - Target GOARCH.
 * @returns {Promise<{ blob: Blob, filename: string }>} Nested package blob and basename.
 * @throws {Error} When no nested package matches the platform.
 */
export async function extractPlatformPackage(outerZipBuffer, goos, goarch) {
    const zip = await JSZip.loadAsync(outerZipBuffer);
    const entries = Object.values(zip.files).filter((f) => !f.dir);

    let platformInner = null;
    const anyInner = [];

    for (const entry of entries) {
        const base = entry.name.split('/').pop().toLowerCase();
        if (base.endsWith('.zip')) {
            anyInner.push(entry);
            if (archiveMatchesPlatform(entry.name, goos, goarch)) {
                platformInner = entry;
            }
        }
    }

    const target = platformInner || anyInner.find((e) =>
        archiveMatchesPlatform(e.name, goos, goarch),
    );

    if (!target) {
        throw new Error(`No nested package for ${goos}-${goarch}`);
    }

    const data = await target.async('blob');
    const filename = target.name.split('/').pop() || `renop-${goos}-${goarch}.zip`;
    return { blob: data, filename };
}

/**
 * Fetch the nightly multi-arch zip and extract the matching platform package.
 * @param {string} url - Nightly zip download URL.
 * @param {string} goos - Target GOOS.
 * @param {string} goarch - Target GOARCH.
 * @param {{ onProgress?: (ratio: number) => void }} [options]
 * @param {(ratio: number) => void} [options.onProgress] - Download progress from 0–1 (0 when Content-Length unknown).
 * @returns {Promise<{ blob: Blob, filename: string }>}
 * @throws {Error} On HTTP failure or missing platform package.
 */
export async function downloadAndExtractNightly(url, goos, goarch, { onProgress } = {}) {
    const res = await fetch(url, { mode: 'cors', credentials: 'omit' });
    if (!res.ok) {
        throw new Error(`Download failed: HTTP ${res.status}`);
    }

    const total = Number(res.headers.get('Content-Length')) || 0;
    if (!res.body || !onProgress) {
        const buf = await res.arrayBuffer();
        if (onProgress) onProgress(1);
        return extractPlatformPackage(buf, goos, goarch);
    }

    const reader = res.body.getReader();
    const chunks = [];
    let received = 0;
    for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        chunks.push(value);
        received += value.byteLength;
        if (total > 0) onProgress(received / total);
        else onProgress(0);
    }
    onProgress(1);

    const buf = new Uint8Array(received);
    let offset = 0;
    for (const chunk of chunks) {
        buf.set(chunk, offset);
        offset += chunk.byteLength;
    }
    return extractPlatformPackage(buf.buffer, goos, goarch);
}
