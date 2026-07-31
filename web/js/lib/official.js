/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

/** Official package host. */
export const OFFICIAL_UPDATE_BASE = 'https://mvnc.pkg.one/update/renop';

export const GITHUB_REPO = '404Setup/SRC-RenoP';

const GH_HEADERS = {
    Accept: 'application/vnd.github+json',
};

export const PLATFORMS = {
    os: [
        {value: 'windows', label: 'Windows'},
        {value: 'darwin', label: 'macOS'},
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
 * Supported GOOS → GOARCH list. Keep in sync with `build.ps1` `$allTargets`.
 * @type {Record<string, string[]>}
 */
export const PLATFORM_MATRIX = {
    windows: ['amd64', 'arm64'],
    darwin: ['amd64', 'arm64'],
    linux: ['amd64', 'arm64', 'mips64', 'mips64le', 'riscv64'],
    freebsd: ['amd64', 'arm64'],
    netbsd: ['amd64', 'arm64'],
    openbsd: ['amd64', 'arm64'],
};

/**
 * @param {string} os
 * @returns {Array<{value: string, label: string}>}
 */
export function getArchOptionsForOs(os) {
    const allowed = PLATFORM_MATRIX[os];
    if (!allowed?.length) return [];
    const allow = new Set(allowed);
    return PLATFORMS.arch.filter((a) => allow.has(a.value));
}

/**
 * @param {string} os
 * @param {string} arch
 * @returns {boolean}
 */
export function isPlatformSupported(os, arch) {
    return Boolean(os && arch && PLATFORM_MATRIX[os]?.includes(arch));
}

/**
 * @param {string} os
 * @param {string} arch
 * @returns {{ os: string, arch: string }}
 */
export function normalizePlatform(os, arch) {
    const validOs = PLATFORMS.os.some((o) => o.value === os) ? os : 'linux';
    const arches = PLATFORM_MATRIX[validOs] || PLATFORM_MATRIX.linux;
    const validArch = arches.includes(arch) ? arch : (arches[0] || 'amd64');
    return {os: validOs, arch: validArch};
}

/**
 * Infer OS/arch from the browser.
 * @returns {{ os: string, arch: string }}
 */
export function detectPlatform() {
    const ua = navigator.userAgent || '';
    const platform = navigator.platform || '';
    let os = 'linux';
    if (/Win/i.test(platform) || /Windows/i.test(ua)) os = 'windows';
    else if (/Mac|iPhone|iPad/i.test(platform) || /Mac OS/i.test(ua)) os = 'darwin';
    else if (/FreeBSD/i.test(ua)) os = 'freebsd';
    else if (/NetBSD/i.test(ua)) os = 'netbsd';
    else if (/OpenBSD/i.test(ua)) os = 'openbsd';

    let arch = 'amd64';
    const uaArch = navigator.userAgentData?.architecture || '';
    if (/arm/i.test(uaArch) || /aarch64/i.test(ua)) arch = 'arm64';
    else if (/x86_64|x64|amd64/i.test(uaArch) || /x86_64|Win64|WOW64/i.test(ua)) arch = 'amd64';

    return normalizePlatform(os, arch);
}

/**
 * @param {'stable'|'nightly'|string} channel
 * @returns {string}
 */
export function channelInfoUrl(channel) {
    const seg = channel === 'nightly' || channel === 'preview' ? 'nightly' : 'stable';
    return `${OFFICIAL_UPDATE_BASE}/${seg}/info.json`;
}

/**
 * Absolute package URL for a target listed in channel info.json.
 * @param {'stable'|'nightly'|string} channel
 * @param {string} version
 * @param {string} file
 * @param {string} [targetUrl]
 * @returns {string}
 */
export function packageDownloadUrl(channel, version, file, targetUrl) {
    if (targetUrl) return targetUrl;
    const seg = channel === 'nightly' || channel === 'preview' ? 'nightly' : 'stable';
    return `${OFFICIAL_UPDATE_BASE}/${seg}/${encodeURIComponent(version)}/${encodeURIComponent(file)}`;
}

function parseTarget(t) {
    return {
        os: String(t.os || ''),
        arch: String(t.arch || ''),
        file: String(t.file || ''),
        sha256: String(t.sha256 || ''),
        size: Number(t.size) || 0,
        downloadUrl: String(t.download_url || t.downloadUrl || ''),
    };
}

function parseReleaseItem(r, channelDefault) {
    return {
        version: String(r.version || ''),
        commit: String(r.commit || ''),
        channel: String(r.channel || channelDefault),
        development: Boolean(r.development),
        publishedAt: String(r.published_at || r.publishedAt || ''),
        changelog: String(r.changelog || ''),
        targets: Array.isArray(r.targets) ? r.targets.map(parseTarget) : [],
    };
}

/**
 * Fetch hosted channel metadata (version, commit, per-platform packages, releases list).
 * @param {'stable'|'nightly'|string} channel
 * @returns {Promise<{
 *   version: string,
 *   commit: string,
 *   channel: string,
 *   development: boolean,
 *   publishedAt: string,
 *   changelog: string,
 *   targets: Array<{ os: string, arch: string, file: string, sha256: string, size: number, downloadUrl: string }>,
 *   releases: Array<{ version: string, commit: string, channel: string, development: boolean, publishedAt: string, changelog: string, targets: Array }>
 * }>}
 */
export async function fetchChannelInfo(channel) {
    const res = await fetch(channelInfoUrl(channel), {mode: 'cors', credentials: 'omit'});
    if (!res.ok) throw new Error(`Official update source HTTP ${res.status}`);
    const data = await res.json();
    const releases = Array.isArray(data.releases) && data.releases.length > 0
        ? data.releases.map((r) => parseReleaseItem(r, channel))
        : [parseReleaseItem(data, channel)];
    const first = releases[0] || {};
    const version = String(data.version || first.version || '');
    if (!version) throw new Error('Invalid channel info.json');

    const targets = Array.isArray(data.targets) && data.targets.length > 0
        ? data.targets.map(parseTarget)
        : (first.targets || []);

    return {
        version,
        commit: String(data.commit || first.commit || ''),
        channel: String(data.channel || first.channel || channel),
        development: Boolean(data.development ?? first.development),
        publishedAt: String(data.published_at || data.publishedAt || first.publishedAt || ''),
        changelog: String(data.changelog || first.changelog || ''),
        targets,
        releases,
    };
}

/**
 * @param {Array<{ os?: string, arch?: string, file?: string, downloadUrl?: string }>|null|undefined} targets
 * @param {string} os
 * @param {string} arch
 * @returns {{ os?: string, arch?: string, file?: string, sha256?: string, size?: number, downloadUrl?: string }|null}
 */
export function findTargetForPlatform(targets, os, arch) {
    if (!targets?.length || !os || !arch) return null;
    const o = os.toLowerCase();
    const a = arch.toLowerCase();
    for (const t of targets) {
        if (String(t.os || '').toLowerCase() === o && String(t.arch || '').toLowerCase() === a) {
            return t;
        }
    }
    const marker = `${o}-${a}`;
    for (const t of targets) {
        if (String(t.file || '').toLowerCase().includes(marker)) return t;
    }
    return null;
}

/**
 * First line of a commit subject.
 * @param {string} [message]
 * @returns {string}
 */
export function commitSubject(message) {
    return (message || '').trim().split('\n')[0].trim();
}

/**
 * @param {string} subject
 * @returns {boolean}
 */
export function isWebOnlyCommit(subject) {
    return commitSubject(subject).toLowerCase().startsWith('[web]');
}

/**
 * Fetch all stable releases from info.json.
 * @returns {Promise<Array<{
 *   id: string,
 *   tag: string,
 *   name: string,
 *   body: string,
 *   publishedAt: string,
 *   commitSha: string,
 *   targets: Array<{ os: string, arch: string, file: string, sha256: string, size: number, downloadUrl: string }>
 * }>>}
 */
export async function fetchStableReleases() {
    const info = await fetchChannelInfo('stable');
    return info.releases.map((rel, index) => {
        const ver = rel.version;
        const tag = ver.startsWith('v') ? ver : `v${ver}`;
        const targets = (rel.targets || []).map((t) => {
            let downloadUrl = t.downloadUrl;
            if (!downloadUrl) {
                if (index <= 1) {
                    downloadUrl = packageDownloadUrl('stable', ver, t.file);
                } else {
                    downloadUrl = `https://github.com/${GITHUB_REPO}/releases/download/${encodeURIComponent(tag)}/${encodeURIComponent(t.file)}`;
                }
            }
            return {...t, downloadUrl};
        });
        return {
            id: ver,
            tag,
            name: ver,
            body: rel.changelog || '',
            publishedAt: rel.publishedAt,
            commitSha: rel.commit,
            targets,
        };
    });
}

/**
 * Stable channel latest card.
 */
export async function fetchStableRelease() {
    const list = await fetchStableReleases();
    return list[0];
}

/**
 * Fetch up to 10 preview releases from info.json.
 * @returns {Promise<Array<{
 *   id: string,
 *   tag: string,
 *   name: string,
 *   body: string,
 *   publishedAt: string,
 *   commitSha: string,
 *   version: string,
 *   targets: Array<{ os: string, arch: string, file: string, sha256: string, size: number, downloadUrl: string }>,
 *   isLatest: boolean
 * }>>}
 */
export async function fetchPreviewReleases() {
    const info = await fetchChannelInfo('nightly');
    const items = info.releases.slice(0, 10);
    return items.map((rel, index) => {
        const short = (rel.commit || rel.version || '').slice(0, 7) || rel.version;
        const name = rel.version.startsWith('nightly-') ? rel.version : `nightly-${short}`;
        return {
            id: rel.version,
            tag: name,
            name,
            body: rel.changelog || '',
            publishedAt: rel.publishedAt,
            commitSha: rel.commit,
            version: rel.version,
            targets: rel.targets,
            isLatest: index === 0,
        };
    });
}

/**
 * Preview channel latest card.
 */
export async function fetchPreviewInfo() {
    const list = await fetchPreviewReleases();
    return list[0];
}

/**
 * List repository contributors (GitHub API).
 * @param {{ perPage?: number, maxPages?: number }} [options]
 * @returns {Promise<Array<{
 *   id: number,
 *   login: string,
 *   avatarUrl: string,
 *   htmlUrl: string,
 *   contributions: number
 * }>>}
 */
export async function fetchContributors({perPage = 100, maxPages = 10} = {}) {
    const all = [];
    for (let page = 1; page <= maxPages; page += 1) {
        const res = await fetch(
            `https://api.github.com/repos/${GITHUB_REPO}/contributors?per_page=${perPage}&page=${page}&anon=0`,
            {headers: GH_HEADERS},
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
 * @param {string} iso
 * @param {string|undefined} [locale]
 * @returns {string}
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
 * @param {string|Blob} blobOrUrl
 * @param {string} [filename]
 * @returns {void}
 */
export function triggerBrowserDownload(blobOrUrl, filename) {
    if (typeof blobOrUrl === 'string') {
        const a = document.createElement('a');
        a.href = blobOrUrl;
        a.rel = 'noopener';
        a.target = '_blank';
        a.download = filename || '';
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
