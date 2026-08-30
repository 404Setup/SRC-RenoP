/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {brotliExecutableToZip} from '../lib/brotli-zip.js';

const maxUpdatePackageSize = 2 * 1024 * 1024 * 1024;

/**
 * Read a response body while reporting byte deltas to the owning page.
 * @param {Response} response
 * @param {string} jobId
 * @returns {Promise<Uint8Array>}
 */
async function readResponseBytes(response, jobId) {
    if (!response.body) return new Uint8Array(await response.arrayBuffer());
    const expected = Number(response.headers.get('content-length')) || 0;
    if (expected > maxUpdatePackageSize) throw new Error('package_too_large');
    const output = expected > 0 ? new Uint8Array(expected) : null;
    const chunks = output ? null : [];
    let received = 0;
    const reader = response.body.getReader();
    for (; ;) {
        const {done, value} = await reader.read();
        if (done) break;
        if (!(value instanceof Uint8Array) || value.length === 0) continue;
        if (output) {
            if (received + value.length > output.length) throw new Error('download_size_mismatch');
            output.set(value, received);
        } else {
            chunks.push(value);
        }
        received += value.length;
        if (received > maxUpdatePackageSize) {
            await reader.cancel();
            throw new Error('package_too_large');
        }
        self.postMessage({type: 'progress', jobId, delta: value.length, received, expected});
    }
    if (output) {
        if (received !== output.length) throw new Error('download_size_mismatch');
        return output;
    }
    const combined = new Uint8Array(received);
    let offset = 0;
    for (const chunk of chunks) {
        combined.set(chunk, offset);
        offset += chunk.length;
    }
    return combined;
}

/**
 * Download one complete package or one inclusive byte range.
 * @param {{jobId: string, url: string, start?: number, end?: number}} job
 * @returns {Promise<void>}
 */
async function download(job) {
    const ranged = Number.isInteger(job.start) && Number.isInteger(job.end);
    const headers = ranged ? {Range: `bytes=${job.start}-${job.end}`} : {};
    const response = await fetch(job.url, {headers, cache: 'no-store', credentials: 'omit'});
    if (!response.ok || (ranged && response.status !== 206)) {
        throw new Error(ranged && response.status === 200 ? 'range_unsupported' : `download_http_${response.status}`);
    }
    const bytes = await readResponseBytes(response, job.jobId);
    self.postMessage({type: 'downloaded', jobId: job.jobId, data: bytes.buffer}, [bytes.buffer]);
}

/**
 * Decompress the raw executable and wrap it in a standard deflated ZIP archive.
 * @param {{jobId: string, data: ArrayBuffer, executableName: string, expectedSize?: number}} job
 * @returns {Promise<void>}
 */
async function convert(job) {
    const compressed = new Uint8Array(job.data);
    const legalNames = ['LICENSE', 'README.md', 'THIRD_PARTY_NOTICES.md'];
    const legalEntries = {};
    const responses = await Promise.all(legalNames.map(name => fetch(`/assets/release/${name}`, {cache: 'force-cache'})));
    for (let index = 0; index < responses.length; index++) {
        if (!responses[index].ok) throw new Error('release_document_unavailable');
        legalEntries[legalNames[index]] = new Uint8Array(await responses[index].arrayBuffer());
    }
    const zip = brotliExecutableToZip(compressed, job.executableName, job.expectedSize, legalEntries);
    self.postMessage({type: 'converted', jobId: job.jobId, data: zip.buffer}, [zip.buffer]);
}

self.addEventListener('message', event => {
    const job = event.data || {};
    Promise.resolve()
        .then(() => {
            if (job.type === 'download') return download(job);
            if (job.type === 'convert') return convert(job);
            throw new Error('unsupported_worker_job');
        })
        .catch(error => {
            self.postMessage({type: 'error', jobId: job.jobId, error: String(error?.message || error)});
        });
});
