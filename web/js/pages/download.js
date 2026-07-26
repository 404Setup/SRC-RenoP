/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {getCurrentLang, t} from '../i18n.js';
import {clear, el} from '../lib/dom.js';
import {
    detectPlatform,
    fetchPreviewInfo,
    fetchStableReleases,
    findAssetForPlatform,
    formatDate,
    getArchOptionsForOs,
    NIGHTLY_ZIP_URL,
    normalizePlatform,
    PLATFORMS,
    triggerBrowserDownload,
} from '../lib/github.js';
import {downloadAndExtractNightly} from '../lib/zip-extract.js';
import {makeCustomSelect} from '../components/custom-select.js';

const PAGE_SIZE = 10;
const STORAGE_OS = 'renop_web_os';
const STORAGE_ARCH = 'renop_web_arch';

/**
 * Load saved OS/arch from localStorage, falling back to auto-detection.
 * Always returns a supported (os, arch) pair for the build matrix.
 * @returns {{ os: string, arch: string }}
 */
function loadSavedPlatform() {
    const detected = detectPlatform();
    const os = localStorage.getItem(STORAGE_OS) || detected.os;
    const arch = localStorage.getItem(STORAGE_ARCH) || detected.arch;
    return normalizePlatform(os, arch);
}

/**
 * Persist OS and/or arch selection to localStorage.
 * @param {string} [os]
 * @param {string} [arch]
 * @returns {void}
 */
function savePlatform(os, arch) {
    if (os) localStorage.setItem(STORAGE_OS, os);
    if (arch) localStorage.setItem(STORAGE_ARCH, arch);
}

/**
 * @param {string} os
 * @param {string} arch
 * @returns {boolean} True when both OS and arch are set.
 */
function platformSelected(os, arch) {
    return Boolean(os && arch);
}

/**
 * Label + custom select field for the download toolbar.
 * @param {string} label - Field label text.
 * @param {Array<{value:string,label:string}|string>} options - Select options.
 * @param {string} value - Current value.
 * @param {(value: string) => void} onChange - Selection callback.
 * @returns {{ field: HTMLElement, select: ReturnType<typeof makeCustomSelect> }}
 */
function createCustomField(label, options, value, onChange) {
    const field = el('div', {class: 'download-field'},
        el('label', {}, label),
    );
    const select = makeCustomSelect(options, value, onChange);
    field.appendChild(select.wrap);
    return {field, select};
}

/**
 * Build a release/download card with changelog and primary download action.
 * @param {object} opts
 * @param {string} opts.title - Release title/tag.
 * @param {string} [opts.badge] - Badge label (stable/preview).
 * @param {string} [opts.badgeClass] - Extra class on the badge.
 * @param {string} [opts.date] - Formatted publish date.
 * @param {string} [opts.body] - Changelog text (plain).
 * @param {() => void} opts.onDownload - Download button handler.
 * @param {string} opts.statusId - Element id for the status message span.
 * @returns {HTMLElement}
 */
function releaseCard({
                         title,
                         badge,
                         badgeClass,
                         date,
                         body,
                         onDownload,
                         statusId,
                     }) {
    const card = el('article', {class: 'card release-card'});
    const header = el('div', {class: 'release-header'},
        el('div', {},
            el('h3', {class: 'release-title'},
                title,
                badge ? el('span', {class: `release-badge${badgeClass ? ` ${badgeClass}` : ''}`}, badge) : null,
            ),
            el('p', {class: 'release-meta'},
                `${t('download.releasedAt')}: ${date || '—'}`,
            ),
        ),
    );
    card.appendChild(header);

    const changelog = el('div', {
        class: `release-changelog${body ? '' : ' is-empty'}`,
        'data-empty': t('download.changelogEmpty'),
    }, body || '');
    card.appendChild(changelog);

    const actions = el('div', {class: 'release-actions'},
        el('button', {
            type: 'button',
            class: 'pill-btn pill-btn--primary',
            onClick: onDownload,
        }, t('download.downloadBtn')),
        el('span', {class: 'release-status', id: statusId}, ''),
    );
    card.appendChild(actions);
    return card;
}

/**
 * Update a release card status line by element id.
 * @param {string} id - Status span id.
 * @param {string} [message] - Status text (empty clears).
 * @param {'error'|'ok'|null|undefined} [kind] - Visual tone.
 * @returns {void}
 */
function setStatus(id, message, kind) {
    const elStatus = document.getElementById(id);
    if (!elStatus) return;
    elStatus.textContent = message || '';
    elStatus.classList.toggle('is-error', kind === 'error');
    elStatus.classList.toggle('is-ok', kind === 'ok');
}

/**
 * Render the download page: stable/preview channels, platform selectors, release list.
 * @param {{ root: HTMLElement }} ctx - Route context.
 * @returns {Promise<() => void>} Cleanup that invalidates in-flight list fetches and destroys selects.
 */
export async function renderDownload({root}) {
    root.innerHTML = '';
    document.title = `RenoP — ${t('download.title')}`;

    let channel = 'stable';
    let page = 1;
    let stableReleases = [];
    /** Monotonic id so an older in-flight fetch cannot overwrite a newer tab switch. */
    let listRequestId = 0;
    const platform = loadSavedPlatform();
    let os = platform.os;
    let arch = platform.arch;

    root.appendChild(
        el('header', {class: 'page-hero'},
            el('h1', {}, t('download.title')),
            el('p', {}, t('download.lead')),
        ),
    );

    const tabs = el('div', {class: 'tabs-container'},
        el('div', {class: 'tabs'},
            el('button', {
                type: 'button',
                class: 'tab active',
                'data-channel': 'stable',
            }, t('download.stable')),
            el('button', {
                type: 'button',
                class: 'tab',
                'data-channel': 'preview',
            }, t('download.preview')),
        ),
    );
    root.appendChild(tabs);

    const osField = createCustomField(t('download.os'), PLATFORMS.os, os, (v) => {
        os = v;
        arch = archField.select.setOptions(getArchOptionsForOs(os), arch);
        savePlatform(os, arch);
        ensurePlatform();
    });
    const archField = createCustomField(
        t('download.arch'),
        getArchOptionsForOs(os),
        arch,
        (v) => {
            arch = v;
            savePlatform(os, arch);
            ensurePlatform();
        },
    );

    const toolbar = el('div', {class: 'download-toolbar'},
        osField.field,
        archField.field,
    );
    root.appendChild(toolbar);

    const platformWarn = el('p', {
        class: 'platform-required',
        style: {display: 'none'},
    }, t('download.platformRequired'));
    root.appendChild(platformWarn);

    const list = el('div', {class: 'download-list'},
        el('p', {class: 'download-loading'}, t('download.loading')),
    );
    root.appendChild(list);

    const pagination = el('div', {class: 'download-pagination', style: {display: 'none'}});
    root.appendChild(pagination);

    /**
     * Show or hide the “platform required” warning based on current OS/arch.
     * @returns {boolean} True when a full platform is selected.
     */
    function ensurePlatform() {
        const ok = platformSelected(os, arch);
        platformWarn.style.display = ok ? 'none' : 'block';
        return ok;
    }

    /**
     * Mark the stable or preview channel tab as active.
     * @param {'stable'|'preview'|string} nextChannel
     * @returns {void}
     */
    function setActiveTab(nextChannel) {
        channel = nextChannel;
        tabs.querySelectorAll('.tab').forEach((b) => {
            b.classList.toggle('active', b.getAttribute('data-channel') === nextChannel);
        });
    }

    tabs.querySelectorAll('.tab').forEach((btn) => {
        btn.addEventListener('click', () => {
            const next = btn.getAttribute('data-channel');
            if (!next || next === channel) return;
            setActiveTab(next);
            page = 1;
            void refreshList();
        });
    });

    /**
     * Start a stable release asset download for the selected platform.
     * @param {{ id: number, assets: Array<{ name: string, url: string }> }} release
     * @returns {Promise<void>}
     */
    async function downloadStable(release) {
        if (!ensurePlatform()) return;
        const statusId = `status-stable-${release.id}`;
        const asset = findAssetForPlatform(release.assets, os, arch);
        if (!asset) {
            setStatus(statusId, t('download.noAsset'), 'error');
            return;
        }
        triggerBrowserDownload(asset.url, asset.name);
        setStatus(statusId, t('download.ready'), 'ok');
    }

    /**
     * Download the nightly multi-arch zip, extract the platform package, or fall back to full zip URL.
     * @param {{ downloadUrl?: string }} preview - Preview metadata from `fetchPreviewInfo`.
     * @returns {Promise<void>}
     */
    async function downloadPreview(preview) {
        if (!ensurePlatform()) return;
        const statusId = 'status-preview';
        setStatus(statusId, t('download.extracting'), null);
        try {
            const {blob, filename} = await downloadAndExtractNightly(
                preview.downloadUrl || NIGHTLY_ZIP_URL,
                os,
                arch,
            );
            triggerBrowserDownload(blob, filename);
            setStatus(statusId, t('download.ready'), 'ok');
        } catch (err) {
            console.warn('[download] nightly extract failed', err);
            setStatus(statusId, t('download.extractFail'), 'error');
            triggerBrowserDownload(NIGHTLY_ZIP_URL);
        }
    }

    /**
     * Render the current page of stable releases and pagination controls.
     * @returns {void}
     */
    function renderStablePage() {
        clear(list);
        if (!stableReleases.length) {
            list.appendChild(el('p', {class: 'download-loading'}, t('download.noReleases')));
            pagination.style.display = 'none';
            return;
        }

        const totalPages = Math.max(1, Math.ceil(stableReleases.length / PAGE_SIZE));
        if (page > totalPages) page = totalPages;
        const start = (page - 1) * PAGE_SIZE;
        const slice = stableReleases.slice(start, start + PAGE_SIZE);

        for (const rel of slice) {
            list.appendChild(
                releaseCard({
                    title: rel.tag || rel.name,
                    badge: t('download.stableBadge'),
                    date: formatDate(rel.publishedAt, getCurrentLang()),
                    body: rel.body,
                    statusId: `status-stable-${rel.id}`,
                    onDownload: () => downloadStable(rel),
                }),
            );
        }

        if (totalPages > 1) {
            pagination.style.display = 'flex';
            clear(pagination);
            const prev = el('button', {
                type: 'button',
                class: 'pill-btn pill-btn--soft pill-btn--sm',
                disabled: page <= 1,
                onClick: () => {
                    page -= 1;
                    renderStablePage();
                    list.scrollIntoView({behavior: 'smooth', block: 'start'});
                },
            }, t('download.prev'));
            const next = el('button', {
                type: 'button',
                class: 'pill-btn pill-btn--soft pill-btn--sm',
                disabled: page >= totalPages,
                onClick: () => {
                    page += 1;
                    renderStablePage();
                    list.scrollIntoView({behavior: 'smooth', block: 'start'});
                },
            }, t('download.next'));
            pagination.append(
                prev,
                el('span', {class: 'download-page-info'}, t('download.page', {page, total: totalPages})),
                next,
            );
        } else {
            pagination.style.display = 'none';
        }
    }

    /**
     * Fetch and render the active channel list (stable releases or single preview card).
     * Ignores responses that are superseded by a newer request id.
     * @returns {Promise<void>}
     */
    async function refreshList() {
        const requestId = ++listRequestId;
        const requestedChannel = channel;

        clear(list);
        list.appendChild(el('p', {class: 'download-loading'}, t('download.loading')));
        pagination.style.display = 'none';

        try {
            if (requestedChannel === 'stable') {
                if (!stableReleases.length) {
                    const releases = await fetchStableReleases();
                    if (requestId !== listRequestId) return;
                    stableReleases = releases;
                } else if (requestId !== listRequestId) {
                    return;
                }
                if (channel !== 'stable' || requestId !== listRequestId) return;
                renderStablePage();
            } else {
                const preview = await fetchPreviewInfo();
                if (requestId !== listRequestId || channel !== 'preview') return;
                clear(list);
                list.appendChild(
                    releaseCard({
                        title: preview.name || t('download.latestPreview'),
                        badge: t('download.previewBadge'),
                        badgeClass: 'release-badge--preview',
                        date: formatDate(preview.publishedAt, getCurrentLang()),
                        body: preview.body,
                        statusId: 'status-preview',
                        onDownload: () => downloadPreview(preview),
                    }),
                );
            }
        } catch (err) {
            if (requestId !== listRequestId) return;
            console.error(err);
            clear(list);
            list.appendChild(el('p', {class: 'download-error'}, t('download.loadError')));
        }
    }

    ensurePlatform();
    await refreshList();

    return () => {
        listRequestId += 1;
        osField.select.destroy();
        archField.select.destroy();
    };
}
