/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

const minParallelPartSize = 8 << 20;
const maxDownloadWorkers = 4;
const maxUpdatePackageSize = 2 * 1024 * 1024 * 1024;
const maxUpdateExecutableSize = 512 * 1024 * 1024;
const workerURL = '/js/update-package-worker.js';

/**
 * Check whether hosted metadata identifies a raw Brotli update package.
 * @param {{file?: string, format?: string, packageFormat?: string}|null|undefined} target
 * @returns {boolean}
 */
export function isBrotliUpdateTarget(target) {
    const format = String(target?.format || target?.packageFormat || '').toLowerCase();
    return format === 'brotli' || String(target?.file || '').toLowerCase().endsWith('.br');
}

/**
 * Select one to four range workers from payload size and browser resource hints.
 * @param {number} size
 * @param {number} [hardwareConcurrency=globalThis.navigator?.hardwareConcurrency||1]
 * @param {number} [deviceMemory=globalThis.navigator?.deviceMemory||4]
 * @returns {number}
 */
export function chooseUpdateDownloadWorkers(
    size,
    hardwareConcurrency = globalThis.navigator?.hardwareConcurrency || 1,
    deviceMemory = globalThis.navigator?.deviceMemory || 4,
) {
    const sizeWorkers = Math.max(1, Math.ceil(Math.max(0, Number(size) || 0) / minParallelPartSize));
    const cpuWorkers = Math.max(1, Math.min(maxDownloadWorkers, Math.floor(Number(hardwareConcurrency) || 1) - 1 || 1));
    const memoryWorkers = Number(deviceMemory) <= 2 ? 1 : (Number(deviceMemory) <= 4 ? 2 : maxDownloadWorkers);
    return Math.max(1, Math.min(maxDownloadWorkers, sizeWorkers, cpuWorkers, memoryWorkers));
}

/**
 * Convert a Brotli filename to the legacy ZIP download name.
 * @param {string} filename
 * @returns {string}
 */
export function legacyZipFilename(filename) {
    const name = String(filename || 'renop-update.br');
    return name.toLowerCase().endsWith('.br') ? `${name.slice(0, -3)}.zip` : `${name}.zip`;
}

/**
 * Run one isolated update-package worker job.
 * @param {object} message
 * @param {(delta: number) => void} [onProgress]
 * @returns {Promise<Uint8Array>}
 */
function runWorkerJob(message, onProgress) {
    return new Promise((resolve, reject) => {
        const worker = new Worker(workerURL, {type: 'module', name: `renop-update-${message.jobId}`});
        const finish = () => worker.terminate();
        worker.addEventListener('message', event => {
            const response = event.data || {};
            if (response.jobId !== message.jobId) return;
            if (response.type === 'progress') {
                onProgress?.(Number(response.delta) || 0);
                return;
            }
            finish();
            if (response.type === 'error') reject(new Error(response.error || 'worker_failed'));
            else if (response.data instanceof ArrayBuffer) resolve(new Uint8Array(response.data));
            else reject(new Error('invalid_worker_response'));
        });
        worker.addEventListener('error', error => {
            finish();
            reject(new Error(error.message || 'worker_failed'));
        }, {once: true});
        const transfers = message.data instanceof ArrayBuffer ? [message.data] : [];
        worker.postMessage(message, transfers);
    });
}

/**
 * Split an inclusive byte length into near-equal non-overlapping ranges.
 * @param {number} size
 * @param {number} workers
 * @returns {Array<{start: number, end: number}>}
 */
function byteRanges(size, workers) {
    const ranges = [];
    const partSize = Math.ceil(size / workers);
    for (let index = 0; index < workers; index++) {
        const start = index * partSize;
        if (start >= size) break;
        ranges.push({start, end: Math.min(size - 1, start + partSize - 1)});
    }
    return ranges;
}

/**
 * Download a package with range workers when the official host supports byte ranges.
 * @param {string} url
 * @param {number} declaredSize
 * @param {(received: number, total: number) => void} onProgress
 * @returns {Promise<Uint8Array>}
 */
async function downloadPackage(url, declaredSize, onProgress) {
    let size = Math.max(0, Number(declaredSize) || 0);
    if (size > maxUpdatePackageSize) throw new Error('package_too_large');
    const workerCount = chooseUpdateDownloadWorkers(size);
    if (workerCount > 1 && size > 0) {
        const probe = await fetch(url, {
            headers: {Range: 'bytes=0-0'}, cache: 'no-store', credentials: 'omit'
        });
        if (probe.status === 206) {
            const contentRange = probe.headers.get('content-range') || '';
            const matched = contentRange.match(/\/(\d+)$/);
            if (matched && Number(matched[1]) > 0) size = Number(matched[1]);
            await probe.arrayBuffer();
            let received = 0;
            const parts = await Promise.all(byteRanges(size, workerCount).map((range, index) =>
                runWorkerJob({type: 'download', jobId: `range-${index}`, url, ...range}, delta => {
                    received += delta;
                    onProgress(received, size);
                })
            ));
            const output = new Uint8Array(parts.reduce((total, part) => total + part.length, 0));
            let offset = 0;
            for (const part of parts) {
                output.set(part, offset);
                offset += part.length;
            }
            if (output.length !== size) throw new Error('download_size_mismatch');
            return output;
        }
        if (!probe.ok) throw new Error(`download_http_${probe.status}`);
        await probe.body?.cancel();
        let received = 0;
        return runWorkerJob({type: 'download', jobId: 'complete-fallback', url}, delta => {
            received += delta;
            onProgress(received, size || received);
        });
    }
    let received = 0;
    return runWorkerJob({type: 'download', jobId: 'complete', url}, delta => {
        received += delta;
        onProgress(received, size || received);
    });
}

/**
 * Verify the compressed package against official metadata.
 * @param {Uint8Array} bytes
 * @param {string} expectedHex
 * @returns {Promise<boolean>}
 */
async function verifySHA256(bytes, expectedHex) {
    const expected = String(expectedHex || '').trim().toLowerCase();
    if (!/^[0-9a-f]{64}$/.test(expected)) return false;
    const digest = new Uint8Array(await crypto.subtle.digest('SHA-256', bytes));
    let actual = '';
    for (const value of digest) actual += value.toString(16).padStart(2, '0');
    return actual === expected;
}

/**
 * Download, authenticate, decompress, and repackage one raw Brotli update as ZIP.
 * @param {{url: string, size?: number, uncompressedSize?: number, sha256?: string, executableName: string, filename: string}} options
 * @param {(phase: string, progress?: number) => void} [onProgress]
 * @returns {Promise<{blob: Blob, filename: string}>}
 */
export async function convertBrotliUpdateToZip(options, onProgress = () => {
}) {
    const expectedSize = Number(options.uncompressedSize) || 0;
    if (expectedSize <= 0 || expectedSize > maxUpdateExecutableSize) throw new Error('invalid_uncompressed_size');
    const compressed = await downloadPackage(options.url, options.size || 0, (received, total) => {
        const progress = total > 0 ? Math.min(100, Math.round((received / total) * 100)) : 0;
        onProgress('download', progress);
    });
    if (!(await verifySHA256(compressed, options.sha256))) throw new Error('integrity_failed');
    onProgress('convert', 0);
    const zip = await runWorkerJob({
        type: 'convert', jobId: 'convert', data: compressed.buffer,
        executableName: options.executableName, expectedSize
    });
    onProgress('convert', 100);
    return {blob: new Blob([zip], {type: 'application/zip'}), filename: legacyZipFilename(options.filename)};
}
