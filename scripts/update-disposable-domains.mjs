/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {mkdir, readFile, rename, rm, writeFile} from 'node:fs/promises';
import {createHash, randomUUID} from 'node:crypto';
import {domainToASCII} from 'node:url';
import {setGlobalProxyFromEnv} from 'node:http';

const directory = new URL('../internal/mail/data/', import.meta.url);
const output = new URL('disposable_domains.txt', directory);
const outputHash = 'e0e0c6ae2e120e0309da143091a2967f77676added3064e866c97c634fea2ead';
const sources = [
    {
        repository: 'disposable/disposable-email-domains',
        commit: 'e4f846fdc7470c8ad127b54867ce2790bd4f9a45',
        path: 'domains.txt',
        sha256: 'c65415be8efc0802ba01d6382e0489df6df94ce1da674a3d00cf51284e6a1ed2',
    },
    {
        repository: 'disposable-email-domains/disposable-email-domains',
        commit: '8d5b14f53dff80842e841ee0ae14de2962cee66c',
        path: 'disposable_email_blocklist.conf',
        sha256: '9e94123b0f0ebd77ef9cd7c6e7dd39bf7d3b981f618a6d20632647e2de20649b',
    },
];

/** Hash source bytes and the generated snapshot without newline conversion. */
function sha256(data) {
    return createHash('sha256').update(data).digest('hex');
}

/** Fetch one pinned data file with a bounded body and deadline. */
async function download(url) {
    const response = await fetch(url, {signal: AbortSignal.timeout(45000)});
    if (!response.ok) throw new Error(`Download failed: ${response.status} ${url}`);
    const chunks = [];
    let size = 0;
    for await (const chunk of response.body) {
        size += chunk.length;
        if (size > 8 * 1024 * 1024) throw new Error('Disposable email source exceeds 8 MiB');
        chunks.push(chunk);
    }
    return Buffer.concat(chunks);
}

/** Reuse a verified snapshot or generate it before Go compiles embedded assets. */
async function ensureDisposableDomains() {
    try {
        if (sha256(await readFile(output)) === outputHash) return;
    } catch (error) {
        if (error.code !== 'ENOENT') throw error;
    }
    const domains = new Set();
    for (const source of sources) {
        const data = await download(`https://raw.githubusercontent.com/${source.repository}/${source.commit}/${source.path}`);
        if (sha256(data) !== source.sha256) throw new Error('Disposable email source checksum mismatch');
        for (const line of data.toString('utf8').split(/\r?\n/)) {
            if (!line.trim() || line.startsWith('#')) continue;
            const domain = domainToASCII(line.trim().toLowerCase().replace(/\.$/, ''));
            if (!domain || domain.length > 253 || !/^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/.test(domain)) {
                throw new Error(`Invalid disposable email domain: ${line}`);
            }
            domains.add(domain);
        }
    }
    const data = [...domains].sort().join('\n') + '\n';
    if (sha256(data) !== outputHash) throw new Error('Generated disposable email snapshot checksum mismatch');
    await mkdir(directory, {recursive: true});
    const temporary = new URL(`disposable_domains.${randomUUID()}.tmp`, directory);
    try {
        await writeFile(temporary, data, {flag: 'wx'});
        await rename(temporary, output);
    } finally {
        await rm(temporary, {force: true});
    }
    console.log(`Generated ${domains.size} disposable email domains from ${sources.length} pinned sources.`);
}

const restoreProxy = setGlobalProxyFromEnv();
try {
    await ensureDisposableDomains();
} finally {
    restoreProxy();
}
