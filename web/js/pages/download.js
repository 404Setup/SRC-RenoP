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
import {clear, el, registerTabContainer, updateTabIndicator} from '@renop/ui';
import {bindModalChrome} from '@renop/ui/modal';
import {
    detectPlatform,
    fetchPreviewReleases,
    fetchStableReleases,
    findTargetForPlatform,
    formatDate,
    getArchOptionsForOs,
    normalizePlatform,
    packageDownloadUrl,
    PLATFORMS,
    triggerBrowserDownload,
    X64_VERSIONS,
} from '../lib/official.js';
import {makeCustomSelect} from '@renop/ui/custom-select';

const STORAGE_OS = 'renop_web_os';
const STORAGE_ARCH = 'renop_web_arch';
const STABLE_PAGE_SIZE = 5;

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
 * @param {string} labelText
 * @param {Array<{value:string,label:string}>} options
 * @param {string} value
 * @param {(value: string) => void} onChange
 * @param {() => void} onInfoClick
 * @returns {{ field: HTMLElement, select: ReturnType<typeof makeCustomSelect> }}
 */
function createX64Field(labelText, options, value, onChange, onInfoClick) {
    const infoBtn = el('button', {
        type: 'button',
        class: 'x64-info-btn',
        title: t('download.x64InfoTitle'),
        'aria-label': t('download.x64InfoTitle'),
        onClick: (e) => {
            e.stopPropagation();
            onInfoClick();
        },
    }, '?');

    const label = el('div', {class: 'download-field-label'},
        el('span', {}, labelText),
        infoBtn,
    );

    const field = el('div', {class: 'download-field download-field-x64'}, label);
    const select = makeCustomSelect(options, value, onChange);
    field.appendChild(select.wrap);
    return {field, select};
}

/**
 * @returns {{ dialogEl: HTMLElement, open: () => void, close: () => void, destroy: () => void }}
 */
function createX64InfoDialog() {
    const backdrop = el('div', {class: 'modal-backdrop', id: 'x64-info-backdrop'});
    const closeBtn = el('button', {
        type: 'button',
        class: 'close-btn',
        id: 'x64-info-close-btn',
        'aria-label': t('modal.close'),
    }, '×');

    const modalBody = el('div', {class: 'modal-body x64-modal-body'},
        el('div', {class: 'x64-version-list'},
            el('div', {class: 'x64-version-item'},
                el('h4', {class: 'x64-version-name'}, 'v1'),
                el('p', {class: 'x64-version-desc'}, t('download.x64V1Desc')),
            ),
            el('div', {class: 'x64-version-item'},
                el('h4', {class: 'x64-version-name'}, 'v2'),
                el('p', {class: 'x64-version-desc'}, t('download.x64V2Desc')),
            ),
            el('div', {class: 'x64-version-item'},
                el('h4', {class: 'x64-version-name'}, 'v3'),
                el('p', {class: 'x64-version-desc'}, t('download.x64V3Desc')),
            ),
            el('div', {class: 'x64-version-item'},
                el('h4', {class: 'x64-version-name'}, 'v4'),
                el('p', {class: 'x64-version-desc'}, t('download.x64V4Desc')),
            ),
        ),
        el('div', {class: 'x64-advice-box'},
            el('h4', {class: 'x64-advice-title'}, t('download.x64AdviceTitle')),
            el('p', {class: 'x64-advice-desc'}, t('download.x64AdviceDesc')),
        ),
    );

    const dialogEl = el('div', {
            id: 'x64-info-modal',
            class: 'modal x64-info-modal',
            style: {display: 'none'},
        },
        backdrop,
        el('div', {class: 'modal-content x64-modal-content', style: {maxWidth: '560px'}},
            closeBtn,
            el('div', {class: 'modal-header'},
                el('h3', {class: 'modal-title'}, t('download.x64DialogTitle')),
                el('p', {class: 'modal-subtitle'}, t('download.x64DialogSubtitle')),
            ),
            modalBody,
        ),
    );

    document.body.appendChild(dialogEl);

    const chrome = bindModalChrome({
        modal: dialogEl,
        closeTriggers: [closeBtn, backdrop],
        escape: true,
    });

    return {
        dialogEl,
        open: () => chrome?.open(),
        close: () => chrome?.close(),
        destroy: () => {
            chrome?.close();
            dialogEl.remove();
        },
    };
}

/**
 * @param {object} opts
 * @param {string} opts.title
 * @param {string} [opts.badge]
 * @param {string} [opts.badgeClass]
 * @param {string} [opts.date]
 * @param {string} [opts.body]
 * @param {() => void} [opts.onDownload]
 * @param {string} opts.statusId
 * @param {boolean} [opts.disabled]
 * @param {number} [opts.index=0]
 * @returns {{ card: HTMLElement, setDisabled: (disabled: boolean) => void }}
 */
function releaseCard({
                         title,
                         badge,
                         badgeClass,
                         date,
                         body,
                         onDownload,
                         statusId,
                         disabled = false,
                         index = 0,
                     }) {
    const card = el('article', {
        class: 'card release-card release-card-enter',
        style: {
            animationDelay: `${index * 60}ms`,
        },
    });
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

    const actions = el('div', {class: 'release-actions'});

    const disabledBtn = el('button', {
        type: 'button',
        class: 'pill-btn pill-btn--disabled',
        disabled: true,
        style: disabled ? {} : {display: 'none'},
    }, t('download.buildUnavailable'));

    const primaryBtn = el('button', {
        type: 'button',
        class: 'pill-btn pill-btn--primary',
        style: disabled ? {display: 'none'} : {},
        onClick: onDownload,
    }, t('download.downloadBtn'));

    actions.appendChild(disabledBtn);
    actions.appendChild(primaryBtn);
    actions.appendChild(el('span', {class: 'release-status', id: statusId}, ''));
    card.appendChild(actions);

    return {
        card,
        setDisabled(nextDisabled) {
            disabledBtn.style.display = nextDisabled ? '' : 'none';
            primaryBtn.style.display = nextDisabled ? 'none' : '';
        },
    };
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
    let currentPage = 1;
    let cachedReleases = null;
    let listRequestId = 0;
    const platform = loadSavedPlatform();
    let os = platform.os;
    let mainArch = platform.arch.startsWith('amd64') ? 'amd64' : platform.arch;
    let x64Version = platform.arch.startsWith('amd64') ? platform.arch : 'amd64';

    /**
     * @returns {string}
     */
    function getEffectiveArch() {
        return mainArch === 'amd64' ? x64Version : mainArch;
    }

    const hero = el('header', {class: 'page-hero'},
        el('h1', {}, t('download.title')),
    );

    const tabsContainer = el('div', {class: 'tabs-container'},
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

    const tabsEl = tabsContainer.querySelector('.tabs');
    registerTabContainer(tabsEl);

    /** @type {Array<{ card: HTMLElement, release: object, globalIndex: number, setDisabled: (disabled: boolean) => void }>} */
    let renderedCardControllers = [];

    function updatePlatformState() {
        const isStable = channel === 'stable';
        const effectiveArch = getEffectiveArch();
        renderedCardControllers.forEach((ctrl) => {
            if (isStable) {
                ctrl.setDisabled(false);
            } else {
                const target = findTargetForPlatform(ctrl.release.targets, os, effectiveArch);
                const isAvailable = ctrl.globalIndex === 0 && Boolean(target?.file);
                ctrl.setDisabled(!isAvailable);
            }
        });
    }

    const x64Dialog = createX64InfoDialog();

    const osField = createCustomField(t('download.os'), PLATFORMS.os, os, (v) => {
        os = v;
        mainArch = archField.select.setOptions(getArchOptionsForOs(os), mainArch);
        syncX64Visibility();
        savePlatform(os, getEffectiveArch());
        ensurePlatform();
        updatePlatformState();
    });

    const archField = createCustomField(
        t('download.arch'),
        getArchOptionsForOs(os),
        mainArch,
        (v) => {
            mainArch = v;
            syncX64Visibility();
            savePlatform(os, getEffectiveArch());
            ensurePlatform();
            updatePlatformState();
        },
    );

    const x64Field = createX64Field(
        t('download.x64Version'),
        X64_VERSIONS,
        x64Version,
        (v) => {
            x64Version = v;
            savePlatform(os, getEffectiveArch());
            ensurePlatform();
            updatePlatformState();
        },
        () => x64Dialog.open(),
    );

    function syncX64Visibility() {
        if (mainArch === 'amd64') {
            x64Field.field.classList.add('is-visible');
        } else {
            x64Field.field.classList.remove('is-visible');
        }
    }

    syncX64Visibility();

    const toolbar = el('div', {class: 'download-toolbar'},
        osField.field,
        archField.field,
        x64Field.field,
    );

    const platformWarn = el('p', {
        class: 'platform-required',
        style: {display: 'none'},
    }, t('download.platformRequired'));

    const list = el('div', {class: 'download-list'});
    const paginationWrapper = el('div', {class: 'download-pagination-wrapper'});

    root.appendChild(hero);
    root.appendChild(tabsContainer);
    root.appendChild(toolbar);
    root.appendChild(platformWarn);
    root.appendChild(list);
    root.appendChild(paginationWrapper);

    requestAnimationFrame(() => updateTabIndicator(tabsEl));

    /**
     * @returns {boolean}
     */
    function ensurePlatform() {
        const ok = platformSelected(os, getEffectiveArch());
        platformWarn.style.display = ok ? 'none' : 'block';
        return ok;
    }

    /**
     * @param {'stable'|'preview'|string} nextChannel
     * @returns {void}
     */
    function setActiveTab(nextChannel) {
        if (channel === nextChannel) return;
        channel = nextChannel;
        currentPage = 1;
        cachedReleases = null;

        tabsEl.querySelectorAll('.tab').forEach((b) => {
            b.classList.toggle('active', b.getAttribute('data-channel') === nextChannel);
        });
        updateTabIndicator(tabsEl);

        list.classList.remove('is-switching');
        void list.offsetWidth;
        list.classList.add('is-switching');

        void fetchAndRender();
    }

    tabsEl.querySelectorAll('.tab').forEach((btn) => {
        btn.addEventListener('click', () => {
            const next = btn.getAttribute('data-channel');
            if (next) setActiveTab(next);
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
        const target = findTargetForPlatform(release.targets, os, getEffectiveArch());
        if (!target?.file || (!target.downloadUrl && !version)) {
            setStatus(statusId, t('download.noAsset'), 'error');
            return;
        }
        const url = packageDownloadUrl(channelKey, version, target.file, target.downloadUrl);
        triggerBrowserDownload(url, target.file);
        setStatus(statusId, t('download.ready'), 'ok');
    }

    function animateAndRenderPage() {
        list.classList.remove('is-switching');
        void list.offsetWidth;
        list.classList.add('is-switching');
        renderPageContent();
    }

    function renderPageContent() {
        clear(list);
        clear(paginationWrapper);
        renderedCardControllers = [];

        const releases = cachedReleases || [];
        if (!releases.length) {
            list.appendChild(el('p', {class: 'download-empty animate-fade-in'}, t('download.noReleases')));
            return;
        }

        const isStable = channel === 'stable';
        const pageSize = STABLE_PAGE_SIZE;
        const totalPages = Math.ceil(releases.length / pageSize) || 1;

        if (currentPage > totalPages) currentPage = totalPages;
        if (currentPage < 1) currentPage = 1;

        const startIdx = (currentPage - 1) * pageSize;
        const pageReleases = releases.slice(startIdx, startIdx + pageSize);

        pageReleases.forEach((rel, index) => {
            const globalIndex = startIdx + index;
            const statusId = `status-${channel}-${globalIndex}`;
            let ctrl;
            if (isStable) {
                ctrl = releaseCard({
                    title: rel.tag || rel.name,
                    badge: t('download.stableBadge'),
                    date: formatDate(rel.publishedAt, getCurrentLang()),
                    body: rel.body,
                    statusId,
                    index,
                    onDownload: () => downloadPackage('stable', {
                        version: rel.id || rel.tag,
                        tag: rel.tag,
                        targets: rel.targets,
                    }, statusId),
                });
            } else {
                const target = findTargetForPlatform(rel.targets, os, getEffectiveArch());
                const isAvailable = globalIndex === 0 && Boolean(target?.file);
                ctrl = releaseCard({
                    title: rel.name || t('download.latestPreview'),
                    badge: t('download.previewBadge'),
                    badgeClass: 'release-badge--preview',
                    date: formatDate(rel.publishedAt, getCurrentLang()),
                    body: rel.body,
                    statusId,
                    index,
                    disabled: !isAvailable,
                    onDownload: () => downloadPackage('nightly', {
                        version: rel.version,
                        tag: rel.tag,
                        targets: rel.targets,
                    }, statusId),
                });
            }
            ctrl.release = rel;
            ctrl.globalIndex = globalIndex;
            renderedCardControllers.push(ctrl);
            list.appendChild(ctrl.card);
        });

        if (totalPages > 1) {
            const prevBtn = el('button', {
                type: 'button',
                class: `pill-btn pill-btn--soft pill-btn--sm${currentPage <= 1 ? ' pill-btn--disabled' : ''}`,
                disabled: currentPage <= 1,
                onClick: () => {
                    if (currentPage > 1) {
                        currentPage--;
                        animateAndRenderPage();
                    }
                },
            }, t('download.prev'));

            const pageInfo = el('span', {class: 'download-page-info'},
                t('download.page', {page: currentPage, total: totalPages})
            );

            const nextBtn = el('button', {
                type: 'button',
                class: `pill-btn pill-btn--soft pill-btn--sm${currentPage >= totalPages ? ' pill-btn--disabled' : ''}`,
                disabled: currentPage >= totalPages,
                onClick: () => {
                    if (currentPage < totalPages) {
                        currentPage++;
                        animateAndRenderPage();
                    }
                },
            }, t('download.next'));

            paginationWrapper.appendChild(
                el('div', {class: 'download-pagination'},
                    prevBtn,
                    pageInfo,
                    nextBtn,
                )
            );
        }
    }

    /**
     * @returns {Promise<void>}
     */
    async function fetchAndRender() {
        const requestId = ++listRequestId;
        const requestedChannel = channel;

        clear(list);
        clear(paginationWrapper);
        list.appendChild(el('p', {class: 'download-loading animate-fade-in'}, t('download.loading')));

        try {
            let releases = [];
            if (requestedChannel === 'stable') {
                releases = await fetchStableReleases();
            } else {
                releases = await fetchPreviewReleases();
            }

            if (requestId !== listRequestId || channel !== requestedChannel) return;

            cachedReleases = releases || [];
            renderPageContent();
        } catch (err) {
            if (requestId !== listRequestId) return;
            console.error(err);
            clear(list);
            clear(paginationWrapper);
            list.appendChild(el('p', {class: 'download-error animate-fade-in'}, t('download.loadError')));
        }
    }

    ensurePlatform();
    await fetchAndRender();

    return () => {
        listRequestId += 1;
        osField.select.destroy();
        archField.select.destroy();
        x64Field.select.destroy();
        x64Dialog.destroy();
    };
}

