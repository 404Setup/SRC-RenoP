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
    fetchStableRelease,
    findTargetForPlatform,
    formatDate,
    getArchOptionsForOs,
    normalizePlatform,
    packageDownloadUrl,
    PLATFORMS,
    triggerBrowserDownload,
} from '../lib/official.js';
import {makeCustomSelect} from '../components/custom-select.js';

const STORAGE_OS = 'renop_web_os';
const STORAGE_ARCH = 'renop_web_arch';

/**
 * Load saved OS/arch from localStorage, falling back to auto-detection.
 * @returns {{ os: string, arch: string }}
 */
function loadSavedPlatform() {
    const detected = detectPlatform();
    const os = localStorage.getItem(STORAGE_OS) || detected.os;
    const arch = localStorage.getItem(STORAGE_ARCH) || detected.arch;
    return normalizePlatform(os, arch);
}

/**
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
 * @returns {boolean}
 */
function platformSelected(os, arch) {
    return Boolean(os && arch);
}

/**
 * @param {string} label
 * @param {Array<{value:string,label:string}|string>} options
 * @param {string} value
 * @param {(value: string) => void} onChange
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
 * @param {object} opts
 * @param {string} opts.title
 * @param {string} [opts.badge]
 * @param {string} [opts.badgeClass]
 * @param {string} [opts.date]
 * @param {string} [opts.body]
 * @param {() => void} opts.onDownload
 * @param {string} opts.statusId
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
 * @param {string} id
 * @param {string} [message]
 * @param {'error'|'ok'|null|undefined} [kind]
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
 * Render the download page: stable/preview channels from the official update host.
 * @param {{ root: HTMLElement }} ctx
 * @returns {Promise<() => void>}
 */
export async function renderDownload({root}) {
    root.innerHTML = '';
    document.title = `RenoP — ${t('download.title')}`;

    let channel = 'stable';
    /** Monotonic id so an older in-flight fetch cannot overwrite a newer tab switch. */
    let listRequestId = 0;
    const platform = loadSavedPlatform();
    let os = platform.os;
    let arch = platform.arch;

    const hero = el('header', {class: 'page-hero'},
        el('h1', {}, t('download.title')),
    );
    const lead = t('download.lead');
    if (lead) hero.appendChild(el('p', {}, lead));
    root.appendChild(hero);

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

    /**
     * @returns {boolean}
     */
    function ensurePlatform() {
        const ok = platformSelected(os, arch);
        platformWarn.style.display = ok ? 'none' : 'block';
        return ok;
    }

    /**
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
            void refreshList();
        });
    });

    /**
     * @param {'stable'|'nightly'} channelKey
     * @param {{ version?: string, tag?: string, targets: Array }} release
     * @param {string} statusId
     * @returns {void}
     */
    function downloadPackage(channelKey, release, statusId) {
        if (!ensurePlatform()) return;
        const version = release.version || release.tag || '';
        const target = findTargetForPlatform(release.targets, os, arch);
        if (!target?.file || !version) {
            setStatus(statusId, t('download.noAsset'), 'error');
            return;
        }
        const url = packageDownloadUrl(channelKey, version, target.file);
        triggerBrowserDownload(url, target.file);
        setStatus(statusId, t('download.ready'), 'ok');
    }

    /**
     * @returns {Promise<void>}
     */
    async function refreshList() {
        const requestId = ++listRequestId;
        const requestedChannel = channel;

        clear(list);
        list.appendChild(el('p', {class: 'download-loading'}, t('download.loading')));

        try {
            if (requestedChannel === 'stable') {
                const release = await fetchStableRelease();
                if (requestId !== listRequestId || channel !== 'stable') return;
                clear(list);
                list.appendChild(
                    releaseCard({
                        title: release.tag || release.name,
                        badge: t('download.stableBadge'),
                        date: formatDate(release.publishedAt, getCurrentLang()),
                        body: release.body,
                        statusId: 'status-stable',
                        onDownload: () => downloadPackage('stable', {
                            version: release.id || release.tag,
                            tag: release.tag,
                            targets: release.targets,
                        }, 'status-stable'),
                    }),
                );
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
                        onDownload: () => downloadPackage('nightly', {
                            version: preview.version,
                            tag: preview.tag,
                            targets: preview.targets,
                        }, 'status-preview'),
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
