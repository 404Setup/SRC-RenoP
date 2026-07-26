/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

export const GITHUB_REPO = '404Setup/SRC-RenoP';
export const NIGHTLY_ARTIFACT_NAME = 'dist-artifacts';
export const NIGHTLY_ZIP_URL =
    `https://nightly.link/${GITHUB_REPO}/workflows/build/main/${NIGHTLY_ARTIFACT_NAME}.zip`;

const UA_HEADERS = {
    Accept: 'application/vnd.github+json',
};

export const PLATFORMS = {
    os: [
        {value: 'windows', label: 'Windows'},
        {value: 'linux', label: 'Linux'},
        {value: 'freebsd', label: 'FreeBSD'},
        {value: 'netbsd', label: 'NetBSD'},
        {value: 'openbsd', label: 'OpenBSD'},
    ],
    arch: [
        {value: 'amd64', label: 'amd64 (x86_64)'},
        {value: 'arm64', label: 'arm64 (aarch64)'},
        {value: 'mips64', label: 'mips64'},
        {value: 'mips64le', label: 'mips64le'},
        {value: 'riscv64', label: 'riscv64'},
    ],
};

/**
 * Infer OS and architecture from the browser user agent / platform APIs.
 * macOS is mapped to `linux` (no native mac builds in the asset matrix).
 * @returns {{ os: string, arch: string }}
 */
export function detectPlatform() {
    const ua = navigator.userAgent || '';
    const platform = navigator.platform || '';
    let os = 'linux';
    if (/Win/i.test(platform) || /Windows/i.test(ua)) os = 'windows';
    else if (/FreeBSD/i.test(ua)) os = 'freebsd';
    else if (/NetBSD/i.test(ua)) os = 'netbsd';
    else if (/OpenBSD/i.test(ua)) os = 'openbsd';
    else if (/Mac|iPhone|iPad/i.test(platform) || /Mac OS/i.test(ua)) {
        os = 'linux';
    }

    let arch = 'amd64';
    const uaArch = navigator.userAgentData?.architecture || '';
    if (/arm/i.test(uaArch) || /aarch64/i.test(ua)) arch = 'arm64';
    else if (/x86_64|x64|amd64/i.test(uaArch) || /x86_64|Win64|WOW64/i.test(ua)) arch = 'amd64';

    return {os, arch};
}

/**
 * Fetch non-draft GitHub releases for the product repository.
 * @returns {Promise<Array<{
 *   id: number,
 *   tag: string,
 *   name: string,
 *   body: string,
 *   publishedAt: string,
 *   prerelease: boolean,
 *   assets: Array<{ name: string, size: number, url: string }>
 * }>>}
 * @throws {Error} When the GitHub API responds with a non-OK status.
 */
export async function fetchStableReleases() {
    const res = await fetch(
        `https://api.github.com/repos/${GITHUB_REPO}/releases?per_page=100`,
        {headers: UA_HEADERS},
    );
    if (!res.ok) throw new Error(`GitHub releases HTTP ${res.status}`);
    const data = await res.json();
    return (data || [])
        .filter((r) => !r.draft)
        .map((r) => ({
            id: r.id,
            tag: r.tag_name,
            name: r.name || r.tag_name,
            body: r.body || '',
            publishedAt: r.published_at || r.created_at,
            prerelease: !!r.prerelease,
            assets: (r.assets || []).map((a) => ({
                name: a.name,
                size: a.size,
                url: a.browser_download_url,
            })),
        }));
}

/**
 * List repository contributors (public GitHub API).
 * Paginates until exhausted or `maxPages` is hit. Bot accounts are skipped.
 * @param {{ perPage?: number, maxPages?: number }} [options]
 * @param {number} [options.perPage=100] - Results per page.
 * @param {number} [options.maxPages=10] - Maximum pages to request.
 * @returns {Promise<Array<{
 *   id: number,
 *   login: string,
 *   avatarUrl: string,
 *   htmlUrl: string,
 *   contributions: number
 * }>>}
 * @throws {Error} When the GitHub API responds with a non-OK status.
 */
export async function fetchContributors({perPage = 100, maxPages = 10} = {}) {
    const all = [];
    for (let page = 1; page <= maxPages; page += 1) {
        const res = await fetch(
            `https://api.github.com/repos/${GITHUB_REPO}/contributors?per_page=${perPage}&page=${page}&anon=0`,
            {headers: UA_HEADERS},
        );
        if (!res.ok) throw new Error(`GitHub contributors HTTP ${res.status}`);
        const batch = await res.json();
        if (!Array.isArray(batch) || !batch.length) break;
        for (const c of batch) {
            if (!c || c.type === 'Bot') continue;
            all.push({
                id: c.id,
                login: c.login,
                avatarUrl: c.avatar_url,
                htmlUrl: c.html_url,
                contributions: c.contributions || 0,
            });
        }
        if (batch.length < perPage) break;
    }
    return all;
}

/**
 * First line of a commit subject (trimmed).
 * @param {string} [message]
 * @returns {string}
 */
export function commitSubject(message) {
    return (message || '').trim().split('\n')[0].trim();
}

/**
 * Website-only commits (`[web] ...`) do not produce product nightlies.
 * @param {string} subject - Commit subject (first line).
 * @returns {boolean}
 */
export function isWebOnlyCommit(subject) {
    return commitSubject(subject).toLowerCase().startsWith('[web]');
}

/**
 * Build nightly/preview release metadata from the latest product commit on `main`.
 * Skips `[web]` commits (official site only; no binary build) when choosing the nightly.
 * Body is that single commit's subject — not a dump of every recent message.
 * @returns {Promise<{
 *   tag: string,
 *   name: string,
 *   body: string,
 *   publishedAt: string,
 *   commitSha: string,
 *   downloadUrl: string
 * }>}
 * @throws {Error} When commits cannot be loaded or no product commit is found.
 */
export async function fetchPreviewInfo() {
    const res = await fetch(
        `https://api.github.com/repos/${GITHUB_REPO}/commits?sha=main&per_page=40`,
        {headers: UA_HEADERS},
    );
    if (!res.ok) throw new Error(`GitHub commits HTTP ${res.status}`);
    const commits = await res.json();
    if (!commits?.length) throw new Error('No commits on main');

    let latest = null;
    for (const c of commits) {
        const subject = commitSubject(c.commit?.message);
        if (isWebOnlyCommit(subject)) continue;
        latest = c;
        break;
    }
    if (!latest) throw new Error('No product commits on main (all [web]?)');

    const sha = latest.sha;
    const shortSha = sha.slice(0, 7);
    const publishedAt =
        latest.commit?.committer?.date ||
        latest.commit?.author?.date ||
        '';
    const body = commitSubject(latest.commit?.message);

    return {
        tag: `nightly-${shortSha}`,
        name: `nightly-${shortSha}`,
        body,
        publishedAt,
        commitSha: sha,
        downloadUrl: NIGHTLY_ZIP_URL,
    };
}

/**
 * Find a release asset whose name matches `{os}-{arch}` (strict boundary, then loose fallback).
 * @param {Array<{ name?: string }>|null|undefined} assets - Release assets.
 * @param {string} os - Target OS id (e.g. `windows`, `linux`).
 * @param {string} arch - Target arch id (e.g. `amd64`, `arm64`).
 * @returns {{ name?: string }|null} Matching asset or null.
 */
export function findAssetForPlatform(assets, os, arch) {
    if (!assets?.length || !os || !arch) return null;
    const marker = `${os}-${arch}`.toLowerCase();
    for (const asset of assets) {
        const name = (asset.name || '').toLowerCase();
        const idx = name.indexOf(marker);
        if (idx < 0) continue;
        const end = idx + marker.length;
        if (end >= name.length) return asset;
        const next = name[end];
        if (next === '.' || next === '-' || next === '_' || next === '/') return asset;
    }
    for (const asset of assets) {
        const name = (asset.name || '').toLowerCase();
        if (name.includes(os.toLowerCase()) && name.includes(arch.toLowerCase())) {
            return asset;
        }
    }
    return null;
}

/**
 * Format an ISO date string for display.
 * @param {string} iso - ISO 8601 timestamp.
 * @param {string|undefined} [locale] - BCP 47 locale for `toLocaleString`.
 * @returns {string} Localized date-time, the raw value on failure, or empty string if missing.
 */
export function formatDate(iso, locale = undefined) {
    if (!iso) return '';
    try {
        return new Date(iso).toLocaleString(locale, {
            year: 'numeric',
            month: 'short',
            day: 'numeric',
            hour: '2-digit',
            minute: '2-digit',
        });
    } catch {
        return iso;
    }
}

/**
 * Trigger a browser download for a remote URL or an in-memory Blob.
 * String URLs open in a new tab; Blobs use an object URL that is revoked after 30s.
 * @param {string|Blob} blobOrUrl - Download URL or Blob payload.
 * @param {string} [filename] - Suggested filename for Blob downloads.
 * @returns {void}
 */
export function triggerBrowserDownload(blobOrUrl, filename) {
    if (typeof blobOrUrl === 'string') {
        const a = document.createElement('a');
        a.href = blobOrUrl;
        a.rel = 'noopener';
        a.target = '_blank';
        document.body.appendChild(a);
        a.click();
        a.remove();
        return;
    }
    const url = URL.createObjectURL(blobOrUrl);
    const a = document.createElement('a');
    a.href = url;
    a.download = filename || 'download.zip';
    document.body.appendChild(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(url), 30_000);
}
