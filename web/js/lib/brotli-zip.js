/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

import {decompress} from 'brotli-compress/js';
import {zipSync} from 'fflate';

/**
 * Decode one raw Brotli executable and place it in a standard deflated ZIP archive.
 * @param {Uint8Array} compressed
 * @param {string} executableName
 * @param {number} [expectedSize=0]
 * @param {Object.<string, Uint8Array>} [additionalFiles={}]
 * @returns {Uint8Array}
 */
export function brotliExecutableToZip(compressed, executableName, expectedSize = 0, additionalFiles = {}) {
    const executable = decompress(compressed);
    if (expectedSize > 0 && executable.length !== expectedSize) throw new Error('uncompressed_size_mismatch');
    const windows = String(executableName).toLowerCase().endsWith('.exe');
    const entry = windows ? executable : [executable, {os: 3, attrs: 0o755 << 16}];
    return zipSync({[executableName]: entry, ...additionalFiles}, {level: 9});
}
