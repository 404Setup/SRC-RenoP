/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {getAuthHeaders, PROTO_CONTENT_TYPE, decodeProtoResponse} from './api.js';
import {
    ChunkedUploadInitRequest,
    ChunkedUploadInitResponse,
    ChunkedUploadCompleteResponse,
} from './proto/index.js';

/** Files smaller than this use the original single-request upload path (no multi-part). */
export const CHUNK_THRESHOLD = 8 * 1024 * 1024;

/** Fallback part size when size-based suggestion is unavailable. */
export const PREFERRED_CHUNK_SIZE = 4 * 1024 * 1024;

/** Soft cap matching server MaxChunkSize (32 MiB). */
export const MAX_CHUNK_SIZE = 32 * 1024 * 1024;

/** Default parallel part uploads; overridden by suggestConcurrency for most files. */
export const CHUNK_CONCURRENCY = 4;

/** Retries per chunk after a failed attempt (network / 5xx / transient). */
export const CHUNK_MAX_RETRIES = 3;

/** Base delay (ms) before retry; grows with attempt index. */
export const CHUNK_RETRY_BASE_MS = 400;

/**
 * @typedef {'pending'|'uploading'|'done'|'error'|'retrying'} ChunkStatus
 * @typedef {{index: number, loaded: number, total: number, status: ChunkStatus, attempt: number}} ChunkState
 *
 * @typedef {object} ChunkedUploadOptions
 * @property {'storage'|'updater'} purpose
 * @property {string} [path] destination path for storage (e.g. "releases/com/foo/a.jar")
 * @property {boolean} [generateChecksums]
 * @property {boolean} [gpgSignatureExpected] whether a matching .asc is part of this browser batch
 * @property {Record<string, string>} [headers]
 * @property {(loaded: number, total: number) => void} [onProgress]
 * @property {(chunks: ChunkState[], meta: {chunkCount: number, concurrency: number}) => void} [onChunkProgress]
 * @property {number} [concurrency]
 * @property {number} [chunkSize]
 * @property {number} [maxRetries]
 * @property {AbortSignal} [signal]
 */

/**
 * Whether this file should use multi-part upload.
 * Small files are faster as a single PUT/POST (no init/complete round-trips).
 * @param {File|Blob} file
 */
export function shouldUseChunkedUpload(file) {
    return !!(file && file.size >= CHUNK_THRESHOLD);
}

/**
 * Suggest part size from file size (aligned with server SuggestChunkSize).
 * @param {number} fileSize
 * @returns {number}
 */
export function suggestChunkSize(fileSize) {
    const size = Number(fileSize) || 0;
    if (size <= 0) return PREFERRED_CHUNK_SIZE;
    if (size <= 8 * 1024 * 1024) return size;
    if (size <= 32 * 1024 * 1024) return 4 * 1024 * 1024;
    if (size <= 128 * 1024 * 1024) return 8 * 1024 * 1024;
    if (size <= 512 * 1024 * 1024) return 16 * 1024 * 1024;
    if (size <= 2 * 1024 * 1024 * 1024) return 24 * 1024 * 1024;
    return MAX_CHUNK_SIZE;
}

/**
 * Parallel workers based on file size and part count.
 * @param {number} fileSize
 * @param {number} chunkCount
 * @returns {number}
 */
export function suggestConcurrency(fileSize, chunkCount) {
    const n = Math.max(0, Number(chunkCount) || 0);
    if (n <= 1) return 1;
    const size = Number(fileSize) || 0;
    let limit;
    if (size < 32 * 1024 * 1024) limit = 3;
    else if (size < 256 * 1024 * 1024) limit = 5;
    else limit = 6;
    return Math.min(limit, n);
}

/**
 * Multi-part upload with concurrent chunk transfers, per-chunk progress, and retries.
 *
 * @param {File|Blob} file
 * @param {ChunkedUploadOptions} options
 * @returns {Promise<{ok: boolean, status: number, body: any, responseText: string, errorCode?: string, reviewID?: string}>}
 */
export async function uploadFileChunked(file, options = {}) {
    const purpose = options.purpose || 'storage';
    const headers = {...getAuthHeaders(), ...(options.headers || {})};
    const preferredChunk = options.chunkSize || suggestChunkSize(file.size);
    const maxRetries = Math.max(0, options.maxRetries ?? CHUNK_MAX_RETRIES);
    const onProgress = typeof options.onProgress === 'function' ? options.onProgress : () => {};
    const onChunkProgress = typeof options.onChunkProgress === 'function' ? options.onChunkProgress : () => {};
    const signal = options.signal;

    const initPayload = {
        purpose,
        filename: file.name || 'upload.bin',
        size: file.size,
        chunk_size: preferredChunk,
    };
    if (purpose === 'storage') {
        initPayload.path = options.path || '';
        initPayload.generate_checksums = !!options.generateChecksums;
		initPayload.gpg_signature_expected = !!options.gpgSignatureExpected;
    }

    const initBody = ChunkedUploadInitRequest.encode(initPayload).finish();

    const initResp = await fetch('/api/upload/chunked/', {
        method: 'POST',
        headers: {
            ...headers,
            'Content-Type': PROTO_CONTENT_TYPE,
            Accept: PROTO_CONTENT_TYPE,
        },
        body: initBody,
        signal,
    });

    if (!initResp.ok) {
        const text = await initResp.text().catch(() => '');
        return {
            ok: false,
            status: initResp.status,
            body: tryParseJsonError(text),
            responseText: text,
            errorCode: initResp.headers.get('X-Renop-Error-Code') || '',
        };
    }

    let session;
    try {
        session = await decodeProtoResponse(initResp, ChunkedUploadInitResponse);
    } catch (err) {
        return {ok: false, status: initResp.status, body: null, responseText: String(err)};
    }

    const uploadId = session.upload_id;
    const chunkSize = Number(session.chunk_size) || preferredChunk;
    const chunkCount = Number(session.chunk_count) || 0;
    const concurrency = Math.max(
        1,
        options.concurrency || suggestConcurrency(file.size, chunkCount),
    );

    /** @type {ChunkState[]} */
    const chunkStates = [];
    for (let i = 0; i < chunkCount; i++) {
        const start = i * chunkSize;
        const end = Math.min(start + chunkSize, file.size);
        chunkStates.push({
            index: i,
            loaded: 0,
            total: Math.max(0, end - start),
            status: 'pending',
            attempt: 0,
        });
    }

    let progressRaf = 0;
    let progressDirty = false;
    const flushProgress = () => {
        progressRaf = 0;
        progressDirty = false;
        onChunkProgress(chunkStates, {chunkCount, concurrency});
        const loaded = chunkStates.reduce((acc, s) => acc + s.loaded, 0);
        onProgress(Math.min(loaded, file.size), file.size);
    };
    const emitChunkProgress = (immediate = false) => {
        if (immediate) {
            if (progressRaf) {
                cancelAnimationFrame(progressRaf);
                progressRaf = 0;
            }
            flushProgress();
            return;
        }
        progressDirty = true;
        if (!progressRaf) {
            progressRaf = requestAnimationFrame(flushProgress);
        }
    };
    emitChunkProgress(true);

    const abortSession = async () => {
        try {
            await fetch(`/api/upload/chunked/${encodeURIComponent(uploadId)}`, {
                method: 'DELETE',
                headers,
            });
        } catch {
            /* ignore */
        }
    };

    if (signal?.aborted) {
        await abortSession();
        return {ok: false, status: 0, body: null, responseText: 'aborted'};
    }

    try {
        if (chunkCount > 0) {
            let nextIndex = 0;
            let firstError = null;

            const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

            const uploadOneChunk = async (index) => {
                const state = chunkStates[index];
                const start = index * chunkSize;
                const end = Math.min(start + chunkSize, file.size);
                const blob = file.slice(start, end);

                for (let attempt = 0; attempt <= maxRetries; attempt++) {
                    if (signal?.aborted) {
                        state.status = 'error';
                        emitChunkProgress(true);
                        return Object.assign(new Error('aborted'), {status: 0, responseText: 'aborted'});
                    }
                    if (firstError) {
                        return firstError;
                    }

                    state.attempt = attempt + 1;
                    state.status = attempt === 0 ? 'uploading' : 'retrying';
                    state.loaded = 0;
                    emitChunkProgress(true);

                    if (attempt > 0) {
                        const delay = CHUNK_RETRY_BASE_MS * Math.pow(2, attempt - 1);
                        await sleep(delay);
                        if (signal?.aborted || firstError) {
                            state.status = 'error';
                            emitChunkProgress(true);
                            return firstError || Object.assign(new Error('aborted'), {status: 0});
                        }
                    }

                    const result = await putChunkWithProgress(
                        uploadId,
                        index,
                        blob,
                        headers,
                        signal,
                        (loaded) => {
                            state.loaded = Math.min(loaded, state.total);
                            state.status = attempt === 0 ? 'uploading' : 'retrying';
                            emitChunkProgress(false);
                        },
                    );

                    if (result.ok) {
                        state.loaded = state.total;
                        state.status = 'done';
                        emitChunkProgress(true);
                        return null;
                    }

                    if (result.status >= 400 && result.status < 500 && result.status !== 408 && result.status !== 429) {
                        state.status = 'error';
                        emitChunkProgress(true);
                        return Object.assign(new Error(result.responseText || `chunk ${index} failed`), {
                            status: result.status,
                            body: result.body,
                            responseText: result.responseText,
                            errorCode: result.errorCode,
                        });
                    }

                    if (attempt === maxRetries) {
                        state.status = 'error';
                        emitChunkProgress(true);
                        return Object.assign(new Error(result.responseText || `chunk ${index} failed after retries`), {
                            status: result.status,
                            body: result.body,
                            responseText: result.responseText,
                            errorCode: result.errorCode,
                        });
                    }
                }
                return null;
            };

            const worker = async () => {
                while (true) {
                    if (signal?.aborted) {
                        firstError = firstError || Object.assign(new Error('aborted'), {status: 0});
                        return;
                    }
                    if (firstError) return;
                    const index = nextIndex++;
                    if (index >= chunkCount) return;

                    const err = await uploadOneChunk(index);
                    if (err) {
                        firstError = firstError || err;
                        return;
                    }
                }
            };

            const workers = [];
            const n = Math.min(concurrency, chunkCount);
            for (let i = 0; i < n; i++) {
                workers.push(worker());
            }
            await Promise.all(workers);

            if (progressRaf) {
                cancelAnimationFrame(progressRaf);
                progressRaf = 0;
            }
            if (progressDirty) flushProgress();

            if (firstError) {
                await abortSession();
                return {
                    ok: false,
                    status: firstError.status || 0,
                    body: firstError.body || null,
                    responseText: firstError.responseText || firstError.message || '',
                    errorCode: firstError.errorCode || '',
                };
            }
        } else {
            onProgress(0, 0);
        }

        const completeResp = await fetch(
            `/api/upload/chunked/${encodeURIComponent(uploadId)}/complete`,
            {
                method: 'POST',
                headers: {
                    ...headers,
                    Accept: PROTO_CONTENT_TYPE,
                },
                signal,
            },
        );

        if (!completeResp.ok) {
            const text = await completeResp.text().catch(() => '');
            await abortSession();
            return {
                ok: false,
                status: completeResp.status,
                body: tryParseJsonError(text),
                responseText: text,
                errorCode: completeResp.headers.get('X-Renop-Error-Code') || '',
            };
        }

        let body = null;
        try {
            body = await decodeProtoResponse(completeResp, ChunkedUploadCompleteResponse);
        } catch {
            body = null;
        }
        onProgress(file.size, file.size);
        chunkStates.forEach((s) => {
            s.loaded = s.total;
            s.status = 'done';
        });
        emitChunkProgress(true);

        return {
            ok: true,
            status: completeResp.status,
            body,
            responseText: '',
            reviewID: completeResp.headers.get('X-RenoP-Review-ID') || '',
        };
    } catch (err) {
        if (progressRaf) {
            cancelAnimationFrame(progressRaf);
            progressRaf = 0;
        }
        await abortSession();
        throw err;
    }
}

/**
 * Try to parse an error response body as JSON; return null on empty/invalid input.
 * @param {string} text
 * @returns {object|null}
 */
function tryParseJsonError(text) {
    if (!text) return null;
    try {
        return JSON.parse(text);
    } catch {
        return null;
    }
}

/**
 * PUT one chunk with XHR so upload progress events are available.
 * @param {string} uploadId
 * @param {number} index
 * @param {Blob} blob
 * @param {Record<string, string>} [headers]
 * @param {AbortSignal} [signal]
 * @param {(loaded: number, total: number) => void} [onChunkProgress]
 * @returns {Promise<{ok: boolean, status: number, body: any, responseText: string, errorCode?: string}>}
 */
function putChunkWithProgress(uploadId, index, blob, headers, signal, onChunkProgress) {
    return new Promise((resolve) => {
        const xhr = new XMLHttpRequest();
        xhr.open('PUT', `/api/upload/chunked/${encodeURIComponent(uploadId)}/${index}`, true);
        xhr.responseType = 'text';

        for (const [key, value] of Object.entries(headers || {})) {
            if (key.toLowerCase() === 'content-type') continue;
            xhr.setRequestHeader(key, value);
        }
        xhr.setRequestHeader('Content-Type', 'application/octet-stream');

        const onAbort = () => {
            try {
                xhr.abort();
            } catch {
                /* ignore */
            }
        };
        if (signal) {
            if (signal.aborted) {
                resolve({ok: false, status: 0, body: null, responseText: 'aborted'});
                return;
            }
            signal.addEventListener('abort', onAbort, {once: true});
        }

        xhr.upload.onprogress = (ev) => {
            if (ev.lengthComputable && onChunkProgress) {
                onChunkProgress(ev.loaded);
            }
        };

        xhr.onload = () => {
            if (signal) signal.removeEventListener('abort', onAbort);
            const text = xhr.responseText || '';
            resolve({
                ok: xhr.status >= 200 && xhr.status < 300,
                status: xhr.status,
                body: tryParseJsonError(text),
                responseText: text,
                errorCode: xhr.getResponseHeader('X-Renop-Error-Code') || '',
            });
        };

        xhr.onerror = () => {
            if (signal) signal.removeEventListener('abort', onAbort);
            resolve({ok: false, status: 0, body: null, responseText: 'network error'});
        };

        xhr.onabort = () => {
            if (signal) signal.removeEventListener('abort', onAbort);
            resolve({ok: false, status: 0, body: null, responseText: 'aborted'});
        };

        xhr.send(blob);
    });
}

/**
 * Single-shot PUT used by the browser upload panel for small files (original behavior).
 * @param {string} targetPath
 * @param {File|Blob} file
 * @param {Record<string, string>} headers
 * @param {(loaded: number, total: number) => void} [onProgress]
 * @returns {Promise<{ok: boolean, status: number, responseText: string, reviewID?: string}>}
 */
export function uploadFileSinglePut(targetPath, file, headers, onProgress) {
    return new Promise((resolve) => {
        const xhr = new XMLHttpRequest();
        xhr.open('PUT', targetPath, true);
        for (const [key, value] of Object.entries(headers || {})) {
            xhr.setRequestHeader(key, value);
        }
        xhr.upload.onprogress = (e) => {
            if (e.lengthComputable && onProgress) {
                onProgress(e.loaded, e.total);
            }
        };
        xhr.onload = () => {
            resolve({
                ok: xhr.status >= 200 && xhr.status < 300,
                status: xhr.status,
                responseText: xhr.responseText || '',
                reviewID: xhr.getResponseHeader('X-RenoP-Review-ID') || '',
            });
        };
        xhr.onerror = () => {
            resolve({ok: false, status: 0, responseText: ''});
        };
        xhr.send(file);
    });
}

/**
 * Single-shot multipart POST for offline updater (original behavior).
 */
export function uploadUpdaterSingle(file, onProgress) {
    return new Promise((resolve) => {
        const formData = new FormData();
        formData.append('file', file);

        const xhr = new XMLHttpRequest();
        xhr.open('POST', '/api/updater/upload', true);

        xhr.upload.onprogress = (ev) => {
            if (ev.lengthComputable && onProgress) {
                onProgress(ev.loaded, ev.total);
            }
        };

        xhr.onload = () => {
            const text = xhr.responseText || '';
            resolve({
                ok: xhr.status >= 200 && xhr.status < 300,
                status: xhr.status,
                body: tryParseJsonError(text),
                responseText: text,
                errorCode: xhr.getResponseHeader('X-Renop-Error-Code') || '',
            });
        };
        xhr.onerror = () => {
            resolve({ok: false, status: 0, body: null, responseText: ''});
        };
        xhr.send(formData);
    });
}

/**
 * Build/update a compact multi-chunk progress strip under an upload entry or container.
 * @param {HTMLElement} host
 * @param {ChunkState[]} chunks
 */
export function renderChunkProgressStrip(host, chunks) {
    if (!host) return null;
    let strip = host.querySelector('.upload-chunk-strip');
    if (!chunks || chunks.length === 0) {
        if (strip) strip.remove();
        return null;
    }
    if (!strip) {
        strip = document.createElement('div');
        strip.className = 'upload-chunk-strip';
        strip.setAttribute('aria-label', 'Chunk upload progress');
        host.appendChild(strip);
    }

    if (strip.childElementCount !== chunks.length) {
        strip.innerHTML = '';
        for (let i = 0; i < chunks.length; i++) {
            const cell = document.createElement('div');
            cell.className = 'upload-chunk-cell';
            cell.dataset.index = String(i);
            const fill = document.createElement('div');
            fill.className = 'upload-chunk-fill';
            cell.appendChild(fill);
            const tip = document.createElement('span');
            tip.className = 'upload-chunk-tip';
            cell.appendChild(tip);
            strip.appendChild(cell);
        }
    }

    chunks.forEach((c, i) => {
        const cell = strip.children[i];
        if (!cell) return;
        const pct = c.total > 0 ? Math.min(100, Math.round((c.loaded / c.total) * 100)) : 0;
        const fill = cell.querySelector('.upload-chunk-fill');
        if (fill) fill.style.width = pct + '%';
        cell.dataset.status = c.status || 'pending';
        cell.className = `upload-chunk-cell is-${c.status || 'pending'}`;
        const tip = cell.querySelector('.upload-chunk-tip');
        if (tip) {
            const label = c.status === 'retrying'
                ? `#${i + 1} retry ${c.attempt || 1}`
                : `#${i + 1} ${pct}%`;
            tip.textContent = label;
            cell.title = label;
        }
    });

    return strip;
}
