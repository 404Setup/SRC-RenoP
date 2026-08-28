/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {el} from '@renop/ui/dom';
import {makeCustomSelect} from '@renop/ui/custom-select';
import {morphElementHeight} from '@renop/ui/height-anim';
import {apiRequest, getAuthHeaders} from '../api.js';
import {cachedIsLoggedIn} from '../auth.js';
import {showAlert, showConfirm} from '../alert.js';
import {
    createDropzone,
    createFileCard,
    createIcon,
    createSkeleton,
    createUserIdentity,
    RenopDialog
} from '../components.js';
import {t} from '../i18n.js';
import {safeMarkdownURL, setSafeMarkdown} from '../markdown.js';
import {getRepositoryFormat} from '../repository-formats.js';
import {caughtErrorMessage, localizedResponseError, responseErrorMessage} from '../response-errors.js';
import {copyWithFeedback} from './copy-feedback.js';
import {decodePathSegment, encodePathSegment, formatBytes} from './utils.js';
import {resolveUserDisplayName} from '../user-profiles.js';
import {
    createRepositoryMirrorBadge,
    formatRepositoryTimestamp,
    hideRepositoryView,
    replaceRepositoryView,
    setRepositoryViewBusy
} from './repository-view.js';
import {RepositoryUserSuggestions} from './user-suggestions.js';

const cargoRepositoryIcon = getRepositoryFormat('cargo').icon;

const CARGO_CATALOG_PAGE_SIZE = 50;
const CARGO_VERSION_PAGE_SIZE = 5;
let cargoLoadSequence = 0;
let activeRepository = '';
let activeAdministrator = false;
let activePackageDetails = null;
let activePackageList = [];
let activePublicPackageNames = new Set();
let activePublicPackageTotal = 0;
let activeCatalogPage = 1;
let activeVersionListPage = 1;
let activeNavigate = null;
let activeView = null;
let activeRouteKind = '';
let activeSelectedVersion = '';
let activePackageTab = 'dependencies';
let activeCommandFormat = 'cargo-add';
let activePackageHeroUpdater = null;
let activeCommandsUpdater = null;
let activeFactsUpdater = null;
let activeInspectionUpdater = null;
let activeVersionsUpdater = null;
let activeVersionsObserver = null;
let listenersInitialized = false;
let inviteLevel = 1;

/**
 * Search users for the active Cargo package invitation form.
 * @param {string} query - Username prefix.
 * @returns {Promise<string[]>} Bounded username results.
 */
async function searchCargoInvitationUsers(query) {
    if (!activePackageDetails?.package) return [];
    const payload = await cargoRequest(
        `${cargoAPIPath('crates', activePackageDetails.package.name, 'users')}?q=${encodeURIComponent(query)}`
    );
    return Array.isArray(payload?.users) ? payload.users : [];
}

const cargoUserSuggestions = new RepositoryUserSuggestions({
    id: 'cargo-invite-suggestions',
    searchDelay: 160,
    closeDelay: 180,
    fetchUsers: searchCargoInvitationUsers,
    onError: (error) => console.error('Failed to search Cargo invitation users', error)
});

/**
 * Perform a same-origin Cargo registry API request and decode optional JSON.
 * @param {string} path - Absolute repository API path.
 * @param {RequestInit} [options] - Fetch options.
 * @param {string} [fallbackKey='cargo.operationFailed'] - Localized request-failure fallback.
 * @returns {Promise<object|null>} Decoded response payload.
 */
async function cargoRequest(path, options = {}, fallbackKey = 'cargo.operationFailed') {
    const response = await apiRequest(path, options, {logoutOnForbidden: false});
    if (!response.ok) throw await localizedResponseError(response, fallbackKey);
    if (response.status === 204) return null;
    const contentType = response.headers.get('content-type') || '';
    return contentType.includes('application/json') ? response.json() : null;
}

/**
 * Build a repository-owned Cargo API path with encoded path segments.
 * @param {...string} segments - API path segments after `/api/v1`.
 * @returns {string} Encoded Cargo API path.
 */
function cargoAPIPath(...segments) {
    const suffix = segments.map(encodeURIComponent).join('/');
    return `/${encodeURIComponent(activeRepository)}/api/v1/${suffix}`;
}

/**
 * Build the browser route for a Cargo package-management subpage.
 * @param {string} [packageName] - Optional Cargo package name.
 * @returns {string} Encoded application route.
 */
function cargoPagePath(packageName = '') {
    const base = `/${encodeURIComponent(activeRepository)}/packages`;
    return packageName ? `${base}/${encodeURIComponent(packageName)}` : base;
}

/**
 * Normalize Cargo package spelling for catalog merges (`-` and `_` are equivalent).
 * @param {unknown} value - Package name candidate.
 * @returns {string} Normalized package key.
 */
function normalizeCargoPackageName(value) {
    return String(value || '').trim().toLowerCase().replaceAll('_', '-');
}

/**
 * Return a localized Cargo permission label.
 * @param {number} level - Permission level 1 through 3.
 * @returns {string} Permission label.
 */
function cargoPermissionLabel(level) {
    return t(`cargo.permissionL${Number(level)}`);
}

/**
 * Format a Cargo metadata timestamp in the current locale.
 * @param {number|string} value - Unix milliseconds.
 * @returns {string} Localized date.
 */
function cargoDate(value) {
    return formatRepositoryTimestamp(value, {dateOnly: true, fallback: t('common.unknown')});
}

/**
 * Navigate within the repository browser without reloading the document.
 * @param {MouseEvent} event - Link click event.
 * @returns {void}
 */
function handleCargoRouteLink(event) {
    if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) {
        return;
    }
    event.preventDefault();
    if (typeof activeNavigate === 'function') activeNavigate(new URL(event.currentTarget.href).pathname);
}

/**
 * Build one package row for the Cargo package-centric repository view.
 * @param {object} packageRecord - Cargo package summary.
 * @returns {HTMLAnchorElement} Package subpage link.
 */
function buildCargoPackageRow(packageRecord) {
    const name = String(packageRecord?.name || '');
    const row = el('a', {href: cargoPagePath(name), class: 'cargo-package-row'});
    row.addEventListener('click', handleCargoRouteLink);
    const meta = el('span', {class: 'cargo-package-row-meta'},
        el('strong', {class: 'cargo-package-row-name'}, name),
        el('span', {class: 'cargo-package-row-description'},
            String(packageRecord?.description || t('cargo.noDescription'))
        )
    );
    const badges = el('span', {class: 'cargo-package-row-badges'});
    if (packageRecord?.archived) {
        badges.appendChild(el('span', {class: 'cargo-state-badge is-archived'}, t('cargo.archived')));
    }
    if (packageRecord?.mirrored) {
        badges.appendChild(createRepositoryMirrorBadge(t('common.fromMirror')));
    }
    if (Number(packageRecord?.permission_level) > 0) {
        badges.appendChild(el('span', {class: 'cargo-permission-badge'}, `L${Number(packageRecord.permission_level)}`));
    }
    if (packageRecord?.max_version) {
        badges.appendChild(el('span', {class: 'cargo-version-badge'}, String(packageRecord.max_version)));
    }
    badges.appendChild(createIcon('chevron'));
    row.append(meta, badges);
    return row;
}

/**
 * Build the Cargo repository overview header.
 * @returns {HTMLElement} Page hero.
 */
function buildCargoOverviewHero() {
    return el('header', {class: 'cargo-page-hero'},
        el('span', {class: 'cargo-page-kicker'}, 'Cargo'),
        el('h2', {class: 'cargo-repository-title'},
            createIcon(cargoRepositoryIcon), el('span', {}, t('cargo.registryTitle'))),
        el('p', {}, t('cargo.registrySubtitle')),
        el('p', {class: 'cargo-page-search-hint'}, t('cargo.searchHint'))
    );
}

/**
 * Return the complete visible catalog count, including managed archived packages.
 * @returns {number} Visible package count.
 */
function cargoCatalogCount() {
    const managedOnlyCount = activePackageList.reduce((count, packageRecord) => {
        const key = normalizeCargoPackageName(packageRecord?.name);
        return count + (key && packageRecord?.archived === true && !activePublicPackageNames.has(key) ? 1 : 0);
    }, 0);
    return activePublicPackageTotal + managedOnlyCount;
}

/**
 * Build the public Cargo package catalog and its bounded pagination control.
 * @returns {HTMLElement} Package-catalog section.
 */
function buildCargoCatalogSection() {
    const section = el('section', {class: 'cargo-page-section'},
        el('div', {class: 'cargo-section-header'},
            el('div', {},
                el('h3', {}, t('cargo.packageCatalog')),
                el('p', {}, t('cargo.packageCatalogSubtitle'))
            ),
            el('span', {class: 'cargo-section-count'}, String(cargoCatalogCount()))
        )
    );
    if (activePackageList.length === 0) {
        section.appendChild(el('p', {class: 'cargo-section-empty'}, t('cargo.noPackages')));
        return section;
    }
    const list = el('div', {class: 'cargo-package-list'});
    for (const packageRecord of activePackageList) list.appendChild(buildCargoPackageRow(packageRecord));
    section.appendChild(list);
    if (activePublicPackageNames.size < activePublicPackageTotal) {
        section.appendChild(el('div', {class: 'cargo-catalog-actions'},
            el('button', {
                type: 'button', class: 'pill-btn pill-btn--soft',
                'data-cargo-action': 'load-more-packages'
            }, t('cargo.loadMorePackages'))
        ));
    }
    return section;
}

/**
 * Render the Cargo repository overview from cached package state.
 * @returns {void}
 */
function renderCargoOverview() {
    if (!activeView) return;
    cargoUserSuggestions.detach();
    activePackageDetails = null;
    activeRouteKind = 'overview';
    activeView.replaceChildren(buildCargoOverviewHero(), buildCargoCatalogSection());
}

/**
 * Return the active selected version record or default to the newest non-yanked version.
 * @returns {object|null} Active version record.
 */
function getSelectedVersion() {
    const versions = Array.isArray(activePackageDetails?.versions) ? activePackageDetails.versions : [];
    if (!versions.length) return null;
    if (activeSelectedVersion) {
        const found = versions.find(v => v.version === activeSelectedVersion);
        if (found) return found;
    }
    const defaultVer = versions.find(v => !v.yanked) || versions[0];
    activeSelectedVersion = defaultVer?.version || '';
    return defaultVer;
}

/**
 * Build the interactive tool commands snippet section with in-place format switching.
 * @param {string} packageName - Package name.
 * @returns {HTMLElement} Commands section.
 */
function buildCargoCommandsSection(packageName) {
    const section = el('section', {class: 'cargo-page-section cargo-commands-section'},
        el('div', {class: 'cargo-section-header'},
            el('div', {},
                el('h3', {}, t('cargo.installAndUse')),
                el('p', {}, t('cargo.toolCommands'))
            )
        )
    );

    const commandTabs = el('div', {class: 'cargo-command-tabs', role: 'tablist'});
    const formats = [
        {id: 'cargo-add', label: t('cargo.commandCargoAdd')},
        {id: 'cargo-toml', label: t('cargo.commandCargoToml')},
        {id: 'cargo-install', label: t('cargo.commandCargoInstall')}
    ];

    const tabButtons = [];
    const codeEl = el('code', {});
    const pre = el('pre', {}, codeEl);

    let currentSnippet = '';

    function computeSnippet() {
        const activeVersion = getSelectedVersion();
        const version = activeVersion?.version || '1.0.0';
        const versions = Array.isArray(activePackageDetails?.versions) ? activePackageDetails.versions : [];
        const isLatest = versions.length > 0 && versions[0]?.version === version;

        switch (activeCommandFormat) {
            case 'cargo-toml':
                return `[dependencies]\n${packageName} = { version = "${version}", registry = "${activeRepository}" }`;
            case 'cargo-install':
                return isLatest
                    ? `cargo install ${packageName} --registry ${activeRepository}`
                    : `cargo install ${packageName} --version ${version} --registry ${activeRepository}`;
            case 'cargo-add':
            default:
                return isLatest
                    ? `cargo add ${packageName} --registry ${activeRepository}`
                    : `cargo add ${packageName}@${version} --registry ${activeRepository}`;
        }
    }

    function updateCommands(animate = true) {
        for (const item of tabButtons) {
            const isActive = activeCommandFormat === item.id;
            item.btn.classList.toggle('is-active', isActive);
            item.btn.setAttribute('aria-selected', isActive ? 'true' : 'false');
        }
        currentSnippet = computeSnippet();
        if (animate) {
            morphElementHeight(codeBox, () => {
                codeEl.textContent = currentSnippet;
                codeEl.style.animation = 'none';
                void codeEl.offsetWidth;
                codeEl.style.animation = 'cargoSnippetFadeIn 0.24s cubic-bezier(0.16, 1, 0.3, 1) both';
            }, {duration: 200});
        } else {
            codeEl.textContent = currentSnippet;
            codeEl.style.animation = '';
        }
    }

    for (const fmt of formats) {
        const tabBtn = el('button', {
            type: 'button',
            class: `cargo-command-tab-btn${activeCommandFormat === fmt.id ? ' is-active' : ''}`,
            role: 'tab',
            'aria-selected': activeCommandFormat === fmt.id ? 'true' : 'false'
        }, fmt.label);
        tabBtn.addEventListener('click', () => {
            if (activeCommandFormat === fmt.id) return;
            activeCommandFormat = fmt.id;
            updateCommands(true);
        });
        tabButtons.push({btn: tabBtn, id: fmt.id});
        commandTabs.appendChild(tabBtn);
    }
    section.appendChild(commandTabs);

    const codeBox = el('div', {class: 'cargo-code-snippet-box'});
    const copyBtn = el('button', {
        type: 'button', class: 'cargo-snippet-copy-btn',
        title: t('cargo.copyCommand'), 'aria-label': t('cargo.copyCommand')
    }, createIcon('copy', {class: 'icon-svg'}));
    copyBtn.addEventListener('click', () => {
        void copyWithFeedback(copyBtn, currentSnippet, {copiedLabel: t('details.copied')}).catch(() => {
        });
    });
    codeBox.append(pre, copyBtn);
    section.appendChild(codeBox);

    updateCommands(false);
    activeCommandsUpdater = updateCommands;

    return section;
}

/**
 * Build the metadata facts grid for the active version with in-place updates.
 * @returns {HTMLElement} Facts section.
 */
function buildCargoVersionFactsSection() {
    const section = el('section', {class: 'cargo-page-section cargo-facts-section'},
        el('h3', {class: 'cargo-section-title'}, t('cargo.packageDetails'))
    );

    const grid = el('div', {class: 'cargo-facts-grid'});
    section.appendChild(grid);

    function updateFacts(animate = false) {
        const activeVersion = getSelectedVersion();
        const packageRecord = activePackageDetails?.package;
        const permissionLevel = Number(packageRecord?.permission_level);
        const accessLabel = activeAdministrator
            ? t('cargo.administratorAccess')
            : permissionLevel > 0 ? cargoPermissionLabel(permissionLevel) : t('cargo.readOnlyAccess');

        const licenseVal = activeVersion?.license || t('cargo.unspecified');
        const msrvVal = activeVersion?.rust_version || t('cargo.unspecified');
        const sizeVal = activeVersion?.size && Number(activeVersion.size) > 0 ? formatBytes(Number(activeVersion.size)) : t('common.unknown');
        const pubDate = activeVersion?.created_at ? cargoDate(activeVersion.created_at) : cargoDate(packageRecord?.updated_at);
        const publisher = activeVersion?.publisher || '';

        const items = [
            el('div', {class: 'cargo-fact-item'},
                el('span', {class: 'cargo-fact-label'}, t('cargo.license')),
                el('span', {class: 'cargo-fact-value'}, licenseVal)
            ),
            el('div', {class: 'cargo-fact-item'},
                el('span', {class: 'cargo-fact-label'}, t('cargo.msrv')),
                el('span', {class: 'cargo-fact-value'}, msrvVal)
            ),
            el('div', {class: 'cargo-fact-item'},
                el('span', {class: 'cargo-fact-label'}, t('cargo.crateSize')),
                el('span', {class: 'cargo-fact-value'}, sizeVal)
            ),
            el('div', {class: 'cargo-fact-item'},
                el('span', {class: 'cargo-fact-label'}, t('cargo.published')),
                el('span', {class: 'cargo-fact-value'}, pubDate)
            ),
            el('div', {class: 'cargo-fact-item'},
                el('span', {class: 'cargo-fact-label'}, t('cargo.publisher')),
                el('span', {class: 'cargo-fact-value'},
                    publisher ? createUserIdentity(publisher) : t('common.unknown')
                )
            ),
            el('div', {class: 'cargo-fact-item'},
                el('span', {class: 'cargo-fact-label'}, t('cargo.readOnlyAccess')),
                el('span', {class: 'cargo-fact-value'}, accessLabel)
            )
        ];

        if (activeVersion?.checksum) {
            const cksum = String(activeVersion.checksum);
            const cksumEl = el('div', {class: 'cargo-fact-item cargo-fact-item--checksum'},
                el('span', {class: 'cargo-fact-label'}, t('cargo.checksum')),
                el('div', {class: 'cargo-checksum-wrap'},
                    el('code', {class: 'cargo-checksum-code'}, cksum),
                    el('button', {
                        type: 'button', class: 'cargo-checksum-copy-btn',
                        title: t('cargo.copyChecksum'), 'aria-label': t('cargo.copyChecksum')
                    }, createIcon('copy', {class: 'icon-svg'}))
                )
            );
            const btn = cksumEl.querySelector('.cargo-checksum-copy-btn');
            btn?.addEventListener('click', () => {
                void copyWithFeedback(btn, cksum, {copiedLabel: t('details.copied')}).catch(() => {
                });
            });
            items.push(cksumEl);
        }

        if (animate) {
            for (let i = 0; i < items.length; i++) {
                items[i].style.animation = `cargoFactFadeIn 0.24s cubic-bezier(0.16, 1, 0.3, 1) ${i * 22}ms both`;
            }
        }

        grid.replaceChildren(...items);
    }

    updateFacts(false);
    activeFactsUpdater = updateFacts;

    return section;
}

/**
 * Build the dependencies and features breakdown tabs section with in-place switching.
 * @returns {HTMLElement} Inspection section.
 */
function buildCargoInspectionSection() {
    const section = el('section', {class: 'cargo-page-section cargo-inspection-section'});

    const tabsHeader = el('div', {class: 'cargo-command-tabs cargo-inspection-tabs', role: 'tablist'});
    const depTabBtn = el('button', {
        type: 'button', class: 'cargo-command-tab-btn', role: 'tab'
    });
    const featTabBtn = el('button', {
        type: 'button', class: 'cargo-command-tab-btn', role: 'tab'
    });
    tabsHeader.append(depTabBtn, featTabBtn);
    section.appendChild(tabsHeader);

    const contentWrap = el('div', {class: 'cargo-inspection-content'});
    section.appendChild(contentWrap);

    function updateInspection(animate = true) {
        const activeVersion = getSelectedVersion();
        const deps = Array.isArray(activeVersion?.deps) ? activeVersion.deps : [];
        const featuresMap = (activeVersion?.features && typeof activeVersion.features === 'object') ? activeVersion.features : {};
        const featureKeys = Object.keys(featuresMap);

        depTabBtn.textContent = `${t('cargo.dependencies')} (${deps.length})`;
        depTabBtn.classList.toggle('is-active', activePackageTab === 'dependencies');
        depTabBtn.setAttribute('aria-selected', activePackageTab === 'dependencies' ? 'true' : 'false');

        featTabBtn.textContent = `${t('cargo.features')} (${featureKeys.length})`;
        featTabBtn.classList.toggle('is-active', activePackageTab === 'features');
        featTabBtn.setAttribute('aria-selected', activePackageTab === 'features' ? 'true' : 'false');

        const mutate = () => {
            if (activePackageTab === 'features') {
                if (featureKeys.length === 0) {
                    contentWrap.replaceChildren(el('p', {class: 'cargo-section-empty'}, t('cargo.noFeatures')));
                } else {
                    const list = el('div', {class: 'cargo-feature-list'});
                    for (const key of featureKeys) {
                        const subFeatures = Array.isArray(featuresMap[key]) ? featuresMap[key] : [];
                        const row = el('div', {class: 'cargo-feature-row'},
                            el('div', {class: 'cargo-feature-name'},
                                el('strong', {}, key),
                                key === 'default' ? el('span', {class: 'cargo-dep-badge is-optional'}, 'default') : null
                            )
                        );
                        if (subFeatures.length > 0) {
                            const subsWrap = el('div', {class: 'cargo-feature-subs'});
                            for (const sub of subFeatures) {
                                subsWrap.appendChild(el('span', {class: 'cargo-feature-sub-badge'}, sub));
                            }
                            row.appendChild(subsWrap);
                        }
                        list.appendChild(row);
                    }
                    contentWrap.replaceChildren(list);
                }
            } else {
                if (deps.length === 0) {
                    contentWrap.replaceChildren(el('p', {class: 'cargo-section-empty'}, t('cargo.noDependencies')));
                } else {
                    const groups = [
                        {kind: 'normal', title: t('cargo.runtimeDependencies')},
                        {kind: 'dev', title: t('cargo.devDependencies')},
                        {kind: 'build', title: t('cargo.buildDependencies')}
                    ];
                    const list = el('div', {class: 'cargo-dependency-list'});
                    for (const group of groups) {
                        const groupDeps = deps.filter(d => (d.kind || 'normal') === group.kind);
                        if (groupDeps.length === 0) continue;

                        const groupHeader = el('div', {class: 'cargo-dep-group-header'},
                            el('span', {class: 'cargo-dep-group-title'}, `${group.title} (${groupDeps.length})`)
                        );
                        list.appendChild(groupHeader);

                        for (const dep of groupDeps) {
                            const depName = String(dep.name || '');
                            const depPackage = dep.package ? String(dep.package) : depName;
                            const depRow = el('div', {class: 'cargo-dep-row'});

                            const nameWrap = el('div', {class: 'cargo-dep-name-wrap'});
                            const nameLink = el('a', {
                                href: cargoPagePath(depPackage),
                                class: 'cargo-dep-link'
                            }, depName);
                            nameLink.addEventListener('click', handleCargoRouteLink);
                            nameWrap.appendChild(nameLink);

                            if (dep.package && dep.package !== depName) {
                                nameWrap.appendChild(el('span', {class: 'cargo-dep-renamed'}, `(crate: ${dep.package})`));
                            }
                            depRow.appendChild(nameWrap);

                            const badgesWrap = el('div', {class: 'cargo-dep-badges-wrap'});
                            const reqStr = dep.req || dep.requirement;
                            if (reqStr) {
                                badgesWrap.appendChild(el('span', {class: 'cargo-dep-req'}, String(reqStr)));
                            }
                            if (dep.optional) {
                                badgesWrap.appendChild(el('span', {class: 'cargo-dep-badge is-optional'}, t('cargo.optional')));
                            }
                            if (dep.target) {
                                badgesWrap.appendChild(el('span', {class: 'cargo-dep-badge is-target'}, `target: ${dep.target}`));
                            }
                            if (dep.default_features === false) {
                                badgesWrap.appendChild(el('span', {class: 'cargo-dep-badge'}, 'default-features: false'));
                            }
                            if (Array.isArray(dep.features) && dep.features.length > 0) {
                                badgesWrap.appendChild(el('span', {class: 'cargo-dep-badge'}, `features: [${dep.features.join(', ')}]`));
                            }
                            depRow.appendChild(badgesWrap);
                            list.appendChild(depRow);
                        }
                    }
                    contentWrap.replaceChildren(list);
                }
            }
            if (animate) {
                contentWrap.classList.remove('cargo-inspection-anim');
                void contentWrap.offsetWidth;
                contentWrap.classList.add('cargo-inspection-anim');
            }
        };

        if (animate) {
            morphElementHeight(contentWrap, mutate, {duration: 220});
        } else {
            mutate();
        }
    }

    depTabBtn.addEventListener('click', () => {
        if (activePackageTab === 'dependencies') return;
        activePackageTab = 'dependencies';
        updateInspection(true);
    });

    featTabBtn.addEventListener('click', () => {
        if (activePackageTab === 'features') return;
        activePackageTab = 'features';
        updateInspection(true);
    });

    updateInspection(false);
    activeInspectionUpdater = updateInspection;

    return section;
}

/**
 * Build the version list with selection, sliding highlight indicator, pagination, and L2/L3 operations.
 * @returns {HTMLElement} Version section.
 */
function buildCargoVersionsSection() {
    const packageRecord = activePackageDetails.package;
    const canManageVersions = activeAdministrator || Number(packageRecord.permission_level) >= 2;
    const section = el('section', {class: 'cargo-page-section'},
        el('div', {class: 'cargo-section-header'},
            el('div', {},
                el('h3', {}, t('cargo.versions')),
                el('p', {}, t('cargo.packageCatalogSubtitle'))
            ),
            el('span', {class: 'cargo-section-count'}, String(activePackageDetails.versions?.length || 0))
        )
    );
    const versions = Array.isArray(activePackageDetails.versions) ? activePackageDetails.versions : [];
    if (versions.length === 0) {
        section.appendChild(el('p', {class: 'cargo-section-empty'}, t('cargo.noVersions')));
        return section;
    }

    const listWrap = el('div', {class: 'cargo-version-list-wrap'});
    const list = el('div', {class: 'cargo-version-list'});
    listWrap.appendChild(list);
    section.appendChild(listWrap);

    const paginationWrap = el('div', {class: 'cargo-version-pagination'});
    section.appendChild(paginationWrap);

    let versionRows = [];
    let versionIndicator = null;

    /**
     * Update active version highlighting and smoothly position the selection indicator.
     * @param {boolean} [animate=true] - Whether to animate the selection indicator transition.
     * @returns {void}
     */
    function updateVersionSelection(animate = true) {
        const curVer = getSelectedVersion();
        const activeItem = versionRows.find(item => curVer && item.version === curVer.version);
        for (const item of versionRows) {
            const isSelected = curVer && item.version === curVer.version;
            item.row.classList.toggle('is-selected', isSelected);
            item.badge.hidden = !isSelected;
            if (isSelected && animate) {
                item.badge.style.animation = 'none';
                void item.badge.offsetWidth;
                item.badge.style.animation = 'cargoBadgePop 0.22s cubic-bezier(0.16, 1, 0.3, 1) both';
            }
        }
        if (activeItem && list.contains(activeItem.row) && activeItem.row.offsetHeight > 0) {
            if (!versionIndicator) {
                versionIndicator = el('div', {class: 'cargo-version-indicator'});
                list.appendChild(versionIndicator);
            }
            list.classList.add('has-indicator');
            const row = activeItem.row;
            const top = row.offsetTop;
            const height = row.offsetHeight;
            const width = row.offsetWidth;
            versionIndicator.style.display = 'block';
            versionIndicator.style.opacity = '1';
            if (!animate) {
                versionIndicator.style.transition = 'none';
                versionIndicator.style.transform = `translateY(${top}px)`;
                versionIndicator.style.height = `${height}px`;
                versionIndicator.style.width = `${width}px`;
                void versionIndicator.offsetHeight;
                versionIndicator.style.transition = '';
            } else {
                versionIndicator.style.transform = `translateY(${top}px)`;
                versionIndicator.style.height = `${height}px`;
                versionIndicator.style.width = `${width}px`;
            }
        } else if (versionIndicator) {
            versionIndicator.style.opacity = '0';
        }
    }

    /**
     * Render the paginated version rows for the current page.
     * @param {boolean} [animate=false] - Whether to animate the container height change.
     * @returns {void}
     */
    function renderVersionPage(animate = false) {
        const totalPages = Math.ceil(versions.length / CARGO_VERSION_PAGE_SIZE) || 1;
        if (activeVersionListPage > totalPages) activeVersionListPage = totalPages;
        if (activeVersionListPage < 1) activeVersionListPage = 1;

        const startIdx = (activeVersionListPage - 1) * CARGO_VERSION_PAGE_SIZE;
        const pageVersions = versions.slice(startIdx, startIdx + CARGO_VERSION_PAGE_SIZE);

        versionRows = [];
        const nextRows = [];

        for (const version of pageVersions) {
            const row = el('div', {class: 'cargo-version-row'});
            const badge = el('span', {class: 'cargo-version-badge is-active-badge'}, t('cargo.activeVersionBadge'));
            badge.hidden = true;

            row.addEventListener('click', (event) => {
                if (event.target.closest('button, a')) return;
                if (activeSelectedVersion === version.version) return;
                activeSelectedVersion = version.version;
                updateVersionSelection(true);
                activePackageHeroUpdater?.(true);
                activeCommandsUpdater?.(true);
                activeFactsUpdater?.(true);
                activeInspectionUpdater?.(true);
            });

            const titleLine = el('div', {class: 'cargo-version-title-line'},
                el('strong', {}, String(version.version || '')),
                badge
            );
            if (version.yanked) {
                titleLine.appendChild(el('span', {class: 'cargo-state-badge is-yanked'}, t('cargo.yanked')));
            }
            if (version.mirrored) {
                titleLine.appendChild(createRepositoryMirrorBadge(t('common.fromMirror')));
            }
            const meta = el('div', {class: 'cargo-version-meta'},
                titleLine,
                el('span', {class: 'cargo-version-publisher'},
                    el('span', {}, `${t('cargo.publisher')}: `),
                    version.publisher ? createUserIdentity(version.publisher) : t('common.unknown')
                )
            );
            row.appendChild(meta);
            const actions = el('div', {class: 'cargo-row-actions'});
            if (version.has_docs === true) {
                const docLink = el('a', {
                    class: 'pill-btn pill-btn--soft pill-btn--sm',
                    href: `/cargodoc/${encodePathSegment(activeRepository)}/${encodePathSegment(packageRecord.name)}/${encodePathSegment(version.version)}/`,
                    target: '_blank', rel: 'noopener noreferrer'
                }, createIcon('docs'), el('span', {}, t('cargo.viewDocs')));
                actions.appendChild(docLink);
            }

            if (canManageVersions) {
                const restoreLocked = version.yanked && (
                    (version.admin_yanked && !activeAdministrator) || packageRecord.archived
                );
                if (!version.has_docs) {
                    actions.appendChild(el('button', {
                        type: 'button', class: 'pill-btn pill-btn--soft pill-btn--sm',
                        'data-cargo-action': 'upload-docs',
                        'data-cargo-version': String(version.version || '')
                    }, createIcon('upload'), el('span', {}, t('cargo.uploadDocs'))));
                }
                if (version.has_docs === true) {
                    actions.appendChild(el('button', {
                        type: 'button', class: 'pill-btn pill-btn--soft pill-btn--sm',
                        'data-cargo-action': 'delete-docs',
                        'data-cargo-version': String(version.version || '')
                    }, t('cargo.deleteDocs')));
                }
                actions.appendChild(el('button', {
                    type: 'button', class: 'pill-btn pill-btn--soft pill-btn--sm',
                    'data-cargo-action': version.yanked ? 'unyank-version' : 'yank-version',
                    'data-cargo-version': String(version.version || ''), disabled: restoreLocked
                }, version.yanked ? t('cargo.restoreVersion') : t('cargo.yankVersion')));
                actions.appendChild(el('button', {
                    type: 'button', class: 'pill-btn pill-btn--danger pill-btn--sm',
                    'data-cargo-action': 'delete-version', 'data-cargo-version': String(version.version || '')
                }, t('common.delete')));
            }
            row.appendChild(actions);
            versionRows.push({row, badge, version: version.version});
            nextRows.push(row);
        }

        const mutateDOM = () => {
            list.replaceChildren(...nextRows);
            versionIndicator = null;
            updateVersionSelection(false);
            if (totalPages > 1) {
                const prevBtn = el('button', {
                    type: 'button',
                    class: 'cargo-pagination-btn',
                    disabled: activeVersionListPage <= 1,
                    title: t('users.paginationPrev') || 'Previous',
                    'aria-label': t('users.paginationPrev') || 'Previous'
                }, createIcon('chevronLeft'));
                prevBtn.addEventListener('click', () => {
                    if (activeVersionListPage > 1) {
                        activeVersionListPage--;
                        renderVersionPage(true);
                    }
                });

                const pageInfo = el('span', {class: 'cargo-pagination-info'}, `${activeVersionListPage} / ${totalPages}`);

                const nextBtn = el('button', {
                    type: 'button',
                    class: 'cargo-pagination-btn',
                    disabled: activeVersionListPage >= totalPages,
                    title: t('users.paginationNext') || 'Next',
                    'aria-label': t('users.paginationNext') || 'Next'
                }, createIcon('chevronRight'));
                nextBtn.addEventListener('click', () => {
                    if (activeVersionListPage < totalPages) {
                        activeVersionListPage++;
                        renderVersionPage(true);
                    }
                });

                paginationWrap.hidden = false;
                paginationWrap.replaceChildren(prevBtn, pageInfo, nextBtn);
            } else {
                paginationWrap.hidden = true;
                paginationWrap.replaceChildren();
            }
        };

        if (animate) {
            morphElementHeight(listWrap, mutateDOM, {duration: 200});
        } else {
            mutateDOM();
        }
    }

    if (activeVersionsObserver) {
        activeVersionsObserver.disconnect();
        activeVersionsObserver = null;
    }
    if (typeof ResizeObserver !== 'undefined') {
        activeVersionsObserver = new ResizeObserver(() => {
            updateVersionSelection(false);
        });
        activeVersionsObserver.observe(list);
    }

    renderVersionPage(false);
    activeVersionsUpdater = (animate = true) => {
        updateVersionSelection(animate);
    };

    return section;
}

/**
 * Build one custom-select permission option.
 * @param {number} level - Cargo permission level.
 * @returns {{value: string, label: string}} Select option.
 */
function buildPermissionOption(level) {
    return {value: String(level), label: `L${level} — ${cargoPermissionLabel(level)}`};
}

/**
 * Persist a team member's L1/L2/L3 permission selection.
 * @param {string} username - Team member username.
 * @param {string} userID - Immutable team member ID.
 * @param {string|number} level - Selected permission level.
 * @returns {void}
 */
function handleMemberLevelChange(username, userID, level) {
    void updateCargoMemberLevel(username, userID, Number(level));
}

/**
 * Build one fixed-width styled permission selector for a team member.
 * @param {object} member - Cargo member record.
 * @returns {HTMLElement} Custom select element.
 */
function buildMemberLevelSelect(member) {
    const username = String(member.login || '');
    const userID = String(member.user_id || '');
    const level = Number(member.level);
    const canTransferOwnership = activeAdministrator ||
        Number(activePackageDetails?.package?.permission_level) === 4;
    const levels = canTransferOwnership ? [1, 2, 3, 4] : [1, 2, 3];
    const select = makeCustomSelect(
        levels.map(buildPermissionOption), String(level),
        handleMemberLevelChange.bind(null, username, userID)
    );
    select.classList.add('cargo-permission-select');
    select.dataset.cargoPermissionUser = username;
    return select;
}

/**
 * Build the package team section and optional L3 invitation form.
 * @param {boolean} [animate=false] - Animate rows after a server-side update.
 * @returns {HTMLElement} Team section.
 */
function buildCargoTeamSection(animate = false) {
    const packageRecord = activePackageDetails.package;
    const canManageTeam = activeAdministrator || Number(packageRecord.permission_level) >= 3;
    const currentUsername = String(localStorage.getItem('username') || '').trim().toLowerCase();
    const section = el('section', {class: 'cargo-page-section'},
        el('h3', {class: 'cargo-section-title'}, t('cargo.team'))
    );
    const members = Array.isArray(activePackageDetails.members) ? activePackageDetails.members : [];
    const list = el('div', {class: `cargo-team-list${animate ? ' is-updated' : ''}`});
    for (let index = 0; index < members.length; index++) {
        const member = members[index];
        const username = String(member.login || '');
        const memberLevel = Number(member.level);
        const isSelf = username.toLowerCase() === currentUsername;
        const row = el('div', {class: 'cargo-team-row'});
        if (animate) row.style.animationDelay = `${Math.min(index, 8) * 35}ms`;
        row.appendChild(el('div', {class: 'cargo-team-member'},
            createUserIdentity(username, {avatar: true, userID: member.user_id}),
            canManageTeam ? el('span', {}, cargoDate(member.added_at)) : el('span', {}, cargoPermissionLabel(member.level))
        ));
        if (canManageTeam) {
            const controls = el('div', {class: 'cargo-team-controls'});
            if (memberLevel === 4) {
                controls.appendChild(el('span', {class: 'cargo-permission-badge'}, cargoPermissionLabel(memberLevel)));
            } else {
                controls.appendChild(buildMemberLevelSelect(member));
                controls.appendChild(el('button', {
                    type: 'button', class: 'pill-btn pill-btn--danger pill-btn--sm',
                    'data-cargo-action': 'remove-member', 'data-cargo-user': username,
                    'data-cargo-user-id': String(member.user_id || '')
                }, isSelf ? t('team.leave') : t('common.remove')));
            }
            row.appendChild(controls);
        } else if (isSelf && memberLevel < 4) {
            row.appendChild(el('div', {class: 'cargo-team-controls'},
                el('button', {
                    type: 'button', class: 'pill-btn pill-btn--danger pill-btn--sm',
                    'data-cargo-action': 'remove-member', 'data-cargo-user': username,
                    'data-cargo-user-id': String(member.user_id || '')
                }, t('team.leave'))
            ));
        }
        list.appendChild(row);
    }
    section.appendChild(list);
    if (canManageTeam) section.appendChild(buildCargoInviteForm());
    return section;
}

/**
 * Store the selected invitation permission level.
 * @param {string|number} level - Selected permission level.
 * @returns {void}
 */
function handleInviteLevelChange(level) {
    inviteLevel = Number(level);
}

/**
 * Build the L3-only invitation form with stable-width permission controls.
 * @returns {HTMLFormElement} Invitation form.
 */
function buildCargoInviteForm() {
    inviteLevel = 1;
    const inviteInput = el('input', {
        id: 'cargo-invite-username', type: 'text', autocomplete: 'off', maxlength: '255',
        placeholder: t('cargo.inviteUsernamePlaceholder')
    });
    cargoUserSuggestions.attach(inviteInput);
    const inputWrap = el('div', {class: 'cargo-invite-input-wrap'}, inviteInput);
    const levelSelect = makeCustomSelect([1, 2, 3, 4].map(buildPermissionOption), '1', handleInviteLevelChange);
    levelSelect.classList.add('cargo-invite-permission-select');
    const form = el('form', {class: 'cargo-invite-form', action: 'javascript:void(0);'},
        el('div', {class: 'cargo-invite-form-heading'},
            el('strong', {}, t('cargo.inviteMember')),
            el('span', {}, t('cargo.inviteMemberHint'))
        ),
        el('div', {class: 'cargo-invite-controls'}, inputWrap, levelSelect,
            el('button', {type: 'submit', class: 'pill-btn pill-btn--primary'}, t('cargo.sendInvite'))
        )
    );
    form.addEventListener('submit', submitCargoInvitation);
    return form;
}

/**
 * Build the package page header, links, and direct download / action buttons.
 * @returns {HTMLElement} Page header element.
 */
function buildCargoPackageHero() {
    const packageRecord = activePackageDetails?.package;
    const packageName = String(packageRecord?.name || '');
    const activeVersion = getSelectedVersion();
    const back = el('a', {href: cargoPagePath(), class: 'cargo-page-back'},
        createIcon('chevronLeft'), el('span', {}, t('cargo.backToPackages'))
    );
    back.addEventListener('click', handleCargoRouteLink);

    const titleRow = el('div', {class: 'cargo-package-title-row'},
        el('h2', {}, packageName)
    );
    const versionBadge = el('span', {class: 'cargo-version-badge'}, activeVersion?.version ? `v${activeVersion.version}` : '');
    if (!activeVersion?.version) versionBadge.hidden = true;
    titleRow.appendChild(versionBadge);

    const archivedBadge = el('span', {class: 'cargo-state-badge is-archived'}, t('cargo.archived'));
    archivedBadge.hidden = !packageRecord?.archived;
    titleRow.appendChild(archivedBadge);

    const yankedBadge = el('span', {class: 'cargo-state-badge is-yanked'}, t('cargo.yanked'));
    yankedBadge.hidden = !activeVersion?.yanked;
    titleRow.appendChild(yankedBadge);

    if (packageRecord?.mirrored) {
        titleRow.appendChild(createRepositoryMirrorBadge(t('common.fromMirror')));
    }

    const hero = el('header', {class: 'cargo-page-hero cargo-package-hero'}, back, titleRow);

    const homepage = safeMarkdownURL(packageRecord?.homepage || activeVersion?.homepage);
    const documentation = safeMarkdownURL(packageRecord?.documentation || activeVersion?.documentation);
    const repository = safeMarkdownURL(packageRecord?.repository_url || activeVersion?.repository_url);
    if (homepage || documentation || repository) {
        const linksList = el('div', {class: 'cargo-package-links'});
        if (homepage) {
            linksList.appendChild(el('a', {
                href: homepage, target: '_blank', rel: 'noopener noreferrer nofollow', class: 'cargo-package-link'
            }, createIcon('fileWeb'), el('span', {}, t('cargo.homepage'))));
        }
        if (documentation) {
            linksList.appendChild(el('a', {
                href: documentation, target: '_blank', rel: 'noopener noreferrer nofollow', class: 'cargo-package-link'
            }, createIcon('docs'), el('span', {}, t('cargo.documentation'))));
        }
        if (repository) {
            linksList.appendChild(el('a', {
                href: repository, target: '_blank', rel: 'noopener noreferrer nofollow', class: 'cargo-package-link'
            }, createIcon('fileCode'), el('span', {}, t('cargo.repository'))));
        }
        hero.appendChild(linksList);
    }

    const descriptionText = String(packageRecord?.description || activeVersion?.description || t('cargo.noDescription'));
    hero.appendChild(el('p', {class: 'cargo-package-description'}, descriptionText));

    const actions = el('div', {class: 'cargo-page-actions'});
    const downloadBtn = el('a', {class: 'pill-btn pill-btn--primary pill-btn--sm'});
    const downloadIcon = createIcon('download');
    const downloadLabel = el('span', {});
    downloadBtn.append(downloadIcon, downloadLabel);

    function updateDownloadBtn(curVer) {
        if (!curVer?.version) {
            downloadBtn.hidden = true;
            return;
        }
        downloadBtn.hidden = false;
        downloadBtn.href = `/${encodePathSegment(activeRepository)}/api/v1/crates/${encodePathSegment(packageName)}/${encodePathSegment(curVer.version)}/download`;
        downloadBtn.download = `${packageName}-${curVer.version}.crate`;
        downloadLabel.textContent = curVer.size && Number(curVer.size) > 0
            ? `${t('cargo.downloadCrate')} (${formatBytes(Number(curVer.size))})`
            : t('cargo.downloadCrate');
    }

    updateDownloadBtn(activeVersion);
    actions.appendChild(downloadBtn);

    const docBtn = el('a', {
        class: 'pill-btn pill-btn--soft pill-btn--sm',
        target: '_blank', rel: 'noopener noreferrer'
    }, createIcon('docs'), el('span', {}, t('cargo.viewDocs')));
    docBtn.hidden = true;
    actions.appendChild(docBtn);

    const canModifyPackage = activeAdministrator || Number(packageRecord?.permission_level) >= 1;
    let uploadDocBtn = null;
    if (canModifyPackage) {
        uploadDocBtn = el('button', {
            type: 'button', class: 'pill-btn pill-btn--soft pill-btn--sm',
            'data-cargo-action': 'upload-docs'
        }, createIcon('upload'), el('span', {}, t('cargo.uploadDocs')));
        actions.appendChild(uploadDocBtn);
    }

    function updateDocsButtons(curVer) {
        if (curVer?.has_docs === true) {
            docBtn.href = `/cargodoc/${encodePathSegment(activeRepository)}/${encodePathSegment(packageName)}/${encodePathSegment(curVer.version)}/`;
            docBtn.hidden = false;
            if (uploadDocBtn) uploadDocBtn.hidden = true;
        } else {
            docBtn.hidden = true;
            if (uploadDocBtn) uploadDocBtn.hidden = !canModifyPackage;
        }
    }

    updateDocsButtons(activeVersion);

    const canManagePackage = activeAdministrator || Number(packageRecord?.permission_level) >= 3;
    if (canManagePackage) {
        const restoreLocked = packageRecord.archived && packageRecord.admin_archived && !activeAdministrator;
        actions.appendChild(el('button', {
            type: 'button', class: 'pill-btn pill-btn--soft pill-btn--sm',
            'data-cargo-action': 'toggle-package-archive', disabled: restoreLocked
        }, packageRecord.archived ? t('cargo.restorePackage') : t('cargo.archivePackage')));
        actions.appendChild(el('button', {
            type: 'button', class: 'pill-btn pill-btn--danger pill-btn--sm',
            'data-cargo-action': 'delete-package'
        }, t('cargo.deletePackage')));
    }
    hero.appendChild(actions);

    activePackageHeroUpdater = (animate = false) => {
        const curVer = getSelectedVersion();
        if (curVer?.version) {
            versionBadge.textContent = `v${curVer.version}`;
            versionBadge.hidden = false;
            if (animate) {
                versionBadge.classList.remove('cargo-badge-anim');
                void versionBadge.offsetWidth;
                versionBadge.classList.add('cargo-badge-anim');
            }
        } else {
            versionBadge.hidden = true;
        }
        archivedBadge.hidden = !activePackageDetails?.package?.archived;
        yankedBadge.hidden = !curVer?.yanked;
        updateDownloadBtn(curVer);
        updateDocsButtons(curVer);
        if (animate) {
            if (downloadBtn && !downloadBtn.hidden) {
                downloadBtn.classList.remove('cargo-badge-anim');
                void downloadBtn.offsetWidth;
                downloadBtn.classList.add('cargo-badge-anim');
            }
            if (docBtn && !docBtn.hidden) {
                docBtn.classList.remove('cargo-badge-anim');
                void docBtn.offsetWidth;
                docBtn.classList.add('cargo-badge-anim');
            }
        }
    };

    return hero;
}

/**
 * Build the README extracted from the latest locally published crate archive.
 * @returns {HTMLElement} Safe Markdown README section.
 */
function buildCargoReadmeSection() {
    const readme = String(activePackageDetails?.package?.readme || '');
    const section = el('section', {class: 'cargo-page-section cargo-readme-section'},
        el('h3', {}, t('cargo.readme'))
    );
    if (!readme) {
        section.appendChild(el('div', {class: 'cargo-readme-empty'},
            createIcon('fileMarkdown'), el('span', {}, t('cargo.noReadme'))));
        return section;
    }
    const content = el('article', {class: 'repository-markdown'});
    setSafeMarkdown(content, readme);
    section.appendChild(content);
    return section;
}

/**
 * Replace the Cargo package subpage from the latest server state.
 * @param {boolean} [animateTeam=false] - Animate refreshed team rows.
 * @returns {void}
 */
function renderCargoPackagePage(animateTeam = false) {
    if (!activeView || !activePackageDetails?.package) return;
    cargoUserSuggestions.detach();
    activeRouteKind = 'package';
    const packageName = String(activePackageDetails.package.name || '');
    const sections = [
        buildCargoPackageHero(),
        buildCargoCommandsSection(packageName),
        buildCargoReadmeSection(),
        buildCargoVersionFactsSection(),
        buildCargoInspectionSection(),
        buildCargoVersionsSection()
    ];
    if (activeAdministrator || Number(activePackageDetails.package.permission_level) > 0) {
        sections.push(buildCargoTeamSection(animateTeam));
    }
    activeView.replaceChildren(...sections);
}

/**
 * Render a loading surface inside the Cargo subpage.
 * @param {string} title - Localized loading title.
 * @returns {void}
 */
function renderCargoLoading(title) {
    if (!activeView) return;
    activeView.replaceChildren(el('section', {class: 'cargo-page-section cargo-page-loading'},
        el('h2', {}, title), createSkeleton('list', 3)
    ));
}

/**
 * Render a route-level Cargo error without exposing raw server responses.
 * @param {string} message - Localized error message.
 * @returns {void}
 */
function renderCargoError(message) {
    if (!activeView) return;
    activeRouteKind = 'error';
    activeView.replaceChildren(el('section', {class: 'cargo-page-section cargo-page-error'},
        createIcon('alertCircle'), el('h2', {}, t('cargo.pageUnavailable')), el('p', {}, message)
    ));
}

/**
 * Mark the Cargo view busy without removing the currently rendered route.
 * @param {boolean} updating - Whether a route request is in flight.
 * @returns {void}
 */
function setCargoUpdating(updating) {
    setRepositoryViewBusy(activeView, updating);
}

/**
 * Show an initial skeleton only when no Cargo content can be preserved.
 * @param {string} title - Localized loading title.
 * @returns {void}
 */
function beginCargoRouteLoad(title) {
    if (!activeView) return;
    if (!activeView.firstElementChild) renderCargoLoading(title);
    setCargoUpdating(true);
}

/**
 * Merge public catalog results with package records carrying management state.
 * @param {Array<object>} publicPackages - Public search results.
 * @param {Array<object>} managedPackages - Packages visible through the management endpoint.
 * @param {boolean} [reset=false] - Whether to replace prior catalog state.
 * @returns {void}
 */
function mergeCargoPackageRecords(publicPackages, managedPackages, reset = false) {
    const records = reset ? [] : activePackageList;
    const packagesByName = new Map();
    for (const packageRecord of records) {
        const key = normalizeCargoPackageName(packageRecord?.name);
        if (key) packagesByName.set(key, packageRecord);
    }
    for (const packageRecord of publicPackages) {
        const key = normalizeCargoPackageName(packageRecord?.name);
        if (!key) continue;
        activePublicPackageNames.add(key);
        packagesByName.set(key, {...(packagesByName.get(key) || {}), ...packageRecord});
    }
    for (const packageRecord of managedPackages) {
        const key = normalizeCargoPackageName(packageRecord?.name);
        if (!key) continue;
        if (packageRecord?.archived !== true) activePublicPackageNames.add(key);
        packagesByName.set(key, {...(packagesByName.get(key) || {}), ...packageRecord});
    }
    activePackageList = Array.from(packagesByName.values()).sort((left, right) =>
        normalizeCargoPackageName(left?.name).localeCompare(normalizeCargoPackageName(right?.name))
    );
}

/**
 * Load the public package catalog plus optional management metadata.
 * @param {number} sequence - Route generation owning this request.
 * @returns {Promise<void>}
 */
async function loadCargoCatalog(sequence) {
    const catalogRequest = cargoRequest(
        `${cargoAPIPath('crates')}?q=&per_page=${CARGO_CATALOG_PAGE_SIZE}&page=1`, {}, 'cargo.loadFailed'
    );
    const managementRequest = cachedIsLoggedIn
        ? cargoRequest(cargoAPIPath('me', 'crates'), {}, 'cargo.loadFailed')
        : Promise.resolve(null);
    const [catalogResult, managementResult] = await Promise.allSettled([catalogRequest, managementRequest]);
    if (catalogResult.status === 'rejected') throw catalogResult.reason;
    if (sequence !== cargoLoadSequence) return;

    let managementPayload = null;
    if (managementResult.status === 'fulfilled') {
        managementPayload = managementResult.value;
    } else {
        console.error('Failed to load Cargo package management state', managementResult.reason);
    }

    const catalogPayload = catalogResult.value;
    activeAdministrator = managementPayload?.administrator === true;
    activePublicPackageNames = new Set();
    activePublicPackageTotal = Math.max(0, Number(catalogPayload?.meta?.total) || 0);
    activeCatalogPage = 1;
    mergeCargoPackageRecords(
        Array.isArray(catalogPayload?.crates) ? catalogPayload.crates : [],
        Array.isArray(managementPayload?.packages) ? managementPayload.packages : [],
        true
    );
}

/**
 * Load and render the Cargo package overview route.
 * @param {number} sequence - Route generation owning this request.
 * @returns {Promise<void>}
 */
async function loadCargoOverview(sequence) {
    activePackageDetails = null;
    activeSelectedVersion = '';
    activeVersionListPage = 1;
    activePackageHeroUpdater = null;
    activeCommandsUpdater = null;
    activeFactsUpdater = null;
    activeInspectionUpdater = null;
    activeVersionsUpdater = null;
    if (activeVersionsObserver) {
        activeVersionsObserver.disconnect();
        activeVersionsObserver = null;
    }
    beginCargoRouteLoad(t('cargo.loadingPackages'));
    try {
        await loadCargoCatalog(sequence);
        if (sequence !== cargoLoadSequence) return;
        await replaceRepositoryView(activeView, renderCargoOverview, {duration: 300, enterDuration: 440});
    } catch (error) {
        if (sequence !== cargoLoadSequence) return;
        console.error('Failed to load Cargo packages', error);
        renderCargoError(t('cargo.loadFailed'));
    } finally {
        if (sequence === cargoLoadSequence) setCargoUpdating(false);
    }
}

/**
 * Load and render one Cargo package-management route.
 * @param {string} packageName - Cargo package name.
 * @param {number} sequence - Route generation owning this request.
 * @returns {Promise<void>}
 */
async function loadCargoPackage(packageName, sequence) {
    activePackageHeroUpdater = null;
    activeCommandsUpdater = null;
    activeFactsUpdater = null;
    activeInspectionUpdater = null;
    activeVersionsUpdater = null;
    if (activeVersionsObserver) {
        activeVersionsObserver.disconnect();
        activeVersionsObserver = null;
    }
    activeVersionListPage = 1;
    beginCargoRouteLoad(t('cargo.loadingPackage'));
    try {
        const details = await cargoRequest(cargoAPIPath('crates', packageName), {}, 'cargo.loadFailed');
        if (sequence !== cargoLoadSequence) return;
        activePackageDetails = details;
        activeAdministrator = details?.administrator === true;
        await replaceRepositoryView(activeView, renderCargoPackagePage, {duration: 300, enterDuration: 440});
    } catch (error) {
        if (sequence !== cargoLoadSequence) return;
        console.error('Failed to load Cargo package', error);
        renderCargoError(caughtErrorMessage(error, 'cargo.loadFailed'));
    } finally {
        if (sequence === cargoLoadSequence) setCargoUpdating(false);
    }
}

/**
 * Append the next bounded public catalog page and preserve any management metadata.
 * @returns {Promise<void>}
 */
async function loadMoreCargoPackages() {
    if (!activeView || activePublicPackageNames.size >= activePublicPackageTotal) return;
    const sequence = cargoLoadSequence;
    const nextPage = activeCatalogPage + 1;
    setCargoUpdating(true);
    try {
        const payload = await cargoRequest(
            `${cargoAPIPath('crates')}?q=&per_page=${CARGO_CATALOG_PAGE_SIZE}&page=${nextPage}`,
            {},
            'cargo.loadFailed'
        );
        if (sequence !== cargoLoadSequence) return;
        activeCatalogPage = nextPage;
        activePublicPackageTotal = Math.max(activePublicPackageTotal, Number(payload?.meta?.total) || 0);
        mergeCargoPackageRecords(Array.isArray(payload?.crates) ? payload.crates : [], []);
        await morphElementHeight(activeView, renderCargoOverview, {duration: 300});
    } catch (error) {
        if (sequence !== cargoLoadSequence) return;
        console.error('Failed to load more Cargo packages', error);
        showAlert(t('cargo.loadFailed'), 'error');
    } finally {
        if (sequence === cargoLoadSequence) setCargoUpdating(false);
    }
}

/**
 * Refresh active package details without tearing down outer container dimensions.
 * @returns {Promise<void>}
 */
async function refreshCargoPackagePage() {
    if (!activePackageDetails?.package || !activeView) return;
    const nextDetails = await cargoRequest(cargoAPIPath('crates', activePackageDetails.package.name));
    activePackageDetails = nextDetails;
    activeAdministrator = nextDetails?.administrator === true;
    await morphElementHeight(activeView, () => renderCargoPackagePage(true), {duration: 280});
}

/**
 * Send a version or package mutation then refresh the package subpage.
 * @param {string} path - Cargo API path.
 * @param {string} method - HTTP method.
 * @param {string} successKey - Translation key for success feedback.
 * @returns {Promise<void>}
 */
async function mutateCargoPackage(path, method, successKey) {
    await cargoRequest(path, {method});
    showAlert(t(successKey), 'success');
    await refreshCargoPackagePage();
}

/**
 * Handle package, version, and team operation buttons on the Cargo subpage.
 * @param {MouseEvent} event - Delegated page click.
 * @returns {Promise<void>}
 */
async function handleCargoPageClick(event) {
    const button = event.target.closest('[data-cargo-action]');
    if (!(button instanceof HTMLButtonElement) || button.disabled) return;
    const action = button.dataset.cargoAction;
    if (action === 'load-more-packages') {
        button.disabled = true;
        try {
            await loadMoreCargoPackages();
        } finally {
            if (button.isConnected) button.disabled = false;
        }
        return;
    }
    if (!activePackageDetails?.package) return;
    const packageName = activePackageDetails.package.name;
    const version = button.dataset.cargoVersion || '';
    const username = button.dataset.cargoUser || '';
    const userID = button.dataset.cargoUserId || '';
    button.disabled = true;
    try {
        switch (action) {
            case 'toggle-package-archive':
                await mutateCargoPackage(
                    cargoAPIPath('crates', packageName, 'archive'),
                    activePackageDetails.package.archived ? 'DELETE' : 'PUT',
                    activePackageDetails.package.archived ? 'cargo.packageRestored' : 'cargo.packageArchived'
                );
                break;
            case 'delete-package':
                await deleteCargoPackage(packageName);
                break;
            case 'yank-version':
                await mutateCargoPackage(cargoAPIPath('crates', packageName, version, 'yank'), 'DELETE', 'cargo.versionYanked');
                break;
            case 'unyank-version':
                await mutateCargoPackage(cargoAPIPath('crates', packageName, version, 'unyank'), 'PUT', 'cargo.versionRestored');
                break;
            case 'delete-version':
                await deleteCargoVersion(packageName, version);
                break;
            case 'remove-member':
                await removeCargoMember(packageName, username, userID);
                break;
            case 'upload-docs': {
                const targetVer = version || getSelectedVersion()?.version || activePackageDetails.versions?.[0]?.version || '';
                await showCargoDocUploadDialog(packageName, targetVer);
                break;
            }
            case 'delete-docs': {
                const targetVer = version || getSelectedVersion()?.version || activePackageDetails.versions?.[0]?.version || '';
                await deleteCargoDocs(packageName, targetVer);
                break;
            }
            default:
                break;
        }
    } catch (error) {
        console.error('Cargo package operation failed', error);
        showAlert(caughtErrorMessage(error, 'cargo.operationFailed'), 'error');
    } finally {
        if (button.isConnected) button.disabled = false;
    }
}

/**
 * Show a modal dialog allowing users to upload a cargo doc archive (.tar.gz, .zip).
 * Reuses the standard Renop modal dropzone and progress styling from offline updates.
 * @param {string} packageName - Cargo crate name.
 * @param {string} version - Target crate version.
 * @returns {Promise<boolean>}
 */
export function showCargoDocUploadDialog(packageName, version) {
    if (!packageName || !version) return Promise.resolve(false);

    let selectedFile = null;

    const getBtn = () => {
        const dlg = document.getElementById('cargo-doc-upload-dialog');
        return dlg ? dlg.querySelector('.primary-btn') : null;
    };

    const disableBtn = () => {
        const btn = getBtn();
        if (btn) btn.disabled = true;
    };

    const enableBtn = () => {
        const btn = getBtn();
        if (btn) btn.disabled = false;
    };

    const dropzone = createDropzone({
        title: t('cargo.uploadDocsHint'),
        hint: '.tar.gz, .tgz, .zip',
        accept: '.tar.gz,.tgz,.zip',
        onSelect: (file) => updateFileDisplay(file)
    });

    const fileCard = el('div', {class: 'file-info-card', style: {display: 'none'}});
    const dropzoneContainer = el('div', {class: 'dropzone-container'}, dropzone, fileCard);

    const progressMsg = el('span', {class: 'modal-notes-label'});
    const progressFill = el('div', {class: 'upload-progress-fill'});
    const progressBar = el('div', {
        class: 'upload-progress-bar',
        style: {position: 'relative', height: '6px', borderRadius: '3px', marginTop: '4px'}
    }, progressFill);

    const progressContainer = el('div', {
        class: 'modal-notes-container',
        style: {display: 'none'}
    }, progressMsg, progressBar);

    const updateFileDisplay = (file) => {
        disableBtn();
        if (!file) {
            selectedFile = null;
            fileCard.style.display = 'none';
            dropzone.style.display = 'flex';
            return;
        }

        const lowerName = file.name.toLowerCase();
        if (!lowerName.endsWith('.tar.gz') && !lowerName.endsWith('.tgz') && !lowerName.endsWith('.zip')) {
            showAlert(t('cargo.uploadDocsHint') + ' (.tar.gz, .tgz, .zip)', 'error');
            return;
        }

        selectedFile = file;
        dropzone.style.display = 'none';
        fileCard.style.display = 'flex';
        fileCard.innerHTML = '';

        const card = createFileCard(file.name, formatBytes(file.size), {
            icon: cargoRepositoryIcon,
            onRemove: () => updateFileDisplay(null)
        });
        fileCard.appendChild(card);
        enableBtn();
    };

    const btnOkConfig = {
        text: t('cargo.uploadDocs'),
        className: 'action-btn primary-btn',
        disabled: true,
        onClick: async (e, dialog) => {
            if (!selectedFile) return;

            const btnOk = dialog.querySelector('.primary-btn');
            const btnCancel = dialog.querySelector('.action-btn:not(.primary-btn)');
            const closeBtn = dialog.querySelector('.close-btn');

            if (btnOk) btnOk.disabled = true;
            if (btnCancel) btnCancel.disabled = true;
            if (closeBtn) closeBtn.disabled = true;

            progressContainer.style.display = 'flex';
            progressMsg.textContent = t('common.loading');
            progressFill.style.width = '30%';

            const resetProgressUI = () => {
                progressContainer.style.display = 'none';
                progressFill.style.width = '0%';
                progressMsg.textContent = '';
                if (btnOk) btnOk.disabled = false;
                if (btnCancel) btnCancel.disabled = false;
                if (closeBtn) closeBtn.disabled = false;
            };

            try {
                const targetUrl = cargoAPIPath('crates', packageName, version, 'docs');
                const headers = getAuthHeaders();
                headers['Content-Type'] = 'application/octet-stream';

                const response = await fetch(targetUrl, {
                    method: 'PUT',
                    headers,
                    body: selectedFile
                });

                if (response.ok) {
                    progressFill.style.width = '100%';
                    showAlert(t('cargo.docsUploaded'), 'success');
                    dialog.close(true);
                    await refreshCargoPackagePage();
                } else {
                    showAlert(await responseErrorMessage(response, 'cargo.operationFailed'), 'error');
                    resetProgressUI();
                }
            } catch (err) {
                console.error('Failed to upload Cargo documentation', err);
                showAlert(caughtErrorMessage(err, 'cargo.operationFailed'), 'error');
                resetProgressUI();
            }
        }
    };

    return RenopDialog.show({
        id: 'cargo-doc-upload-dialog',
        glass: true,
        size: 'md',
        centered: true,
        title: `${t('cargo.uploadDocsTitle')} (${packageName} v${version})`,
        bodyClass: 'modal-body modal-body-flex',
        body: [dropzoneContainer, progressContainer],
        footer: [
            {
                text: t('common.cancel'),
                className: 'action-btn',
                onClick: (e, d) => {
                    d.close(false);
                }
            },
            btnOkConfig
        ]
    });
}

/**
 * Delete cargo documentation for a specific package version after confirmation.
 * @param {string} packageName - Cargo package name.
 * @param {string} version - Version string.
 * @returns {Promise<void>}
 */
async function deleteCargoDocs(packageName, version) {
    const confirmed = await showConfirm(t('cargo.confirmDeleteDocs', {version}), {
        title: t('cargo.deleteDocs'),
        confirmText: t('common.delete'),
        danger: true
    });
    if (!confirmed) return;
    await cargoRequest(cargoAPIPath('crates', packageName, version, 'docs'), {method: 'DELETE'});
    showAlert(t('cargo.docsDeleted'), 'success');
    await refreshCargoPackagePage();
}

/**
 * Delete an entire Cargo package after explicit confirmation.
 * @param {string} packageName - Package name.
 * @returns {Promise<void>}
 */
async function deleteCargoPackage(packageName) {
    const confirmed = await showConfirm(t('cargo.deletePackageConfirm', {package: packageName}), {
        title: t('cargo.deletePackage'), confirmText: t('cargo.deletePackage'), danger: true
    });
    if (!confirmed) return;
    await cargoRequest(cargoAPIPath('crates', packageName), {method: 'DELETE'});
    showAlert(t('cargo.packageDeleted'), 'success');
    if (typeof activeNavigate === 'function') activeNavigate(cargoPagePath());
}

/**
 * Delete one Cargo version after explicit confirmation.
 * @param {string} packageName - Package name.
 * @param {string} version - Version string.
 * @returns {Promise<void>}
 */
async function deleteCargoVersion(packageName, version) {
    const confirmed = await showConfirm(t('cargo.deleteVersionConfirm', {version}), {
        title: t('cargo.deleteVersion'), confirmText: t('common.delete'), danger: true
    });
    if (!confirmed) return;
    await mutateCargoPackage(cargoAPIPath('crates', packageName, version), 'DELETE', 'cargo.versionDeleted');
}

/**
 * Remove a package team member after explicit confirmation.
 * @param {string} packageName - Package name.
 * @param {string} username - Team username.
 * @param {string} [userID=''] - Immutable team member ID.
 * @returns {Promise<void>}
 */
async function removeCargoMember(packageName, username, userID = '') {
    const isSelf = username.toLowerCase() === String(localStorage.getItem('username') || '').trim().toLowerCase();
    const displayName = isSelf ? '' : await resolveUserDisplayName(username);
    const confirmed = await showConfirm(
        isSelf ? t('team.leaveConfirm') : t('cargo.removeMemberConfirm', {name: displayName}), {
            title: isSelf ? t('team.leave') : t('cargo.removeMember'),
            confirmText: isSelf ? t('team.leave') : t('common.remove'), danger: true
        });
    if (!confirmed) return;
    const path = cargoAPIPath('crates', packageName, 'owners', userID || username);
    if (isSelf) {
        await cargoRequest(path, {method: 'DELETE'});
        showAlert(t('team.left'), 'success');
        if (typeof activeNavigate === 'function') activeNavigate(cargoPagePath());
        return;
    }
    await mutateCargoPackage(
        path,
        'DELETE',
        'cargo.memberRemoved'
    );
}

/**
 * Update one package team member's permission without rebuilding sibling controls.
 * @param {string} username - Team username.
 * @param {string} userID - Immutable team member ID.
 * @param {number} level - Permission level 1 through 4.
 * @returns {Promise<void>}
 */
async function updateCargoMemberLevel(username, userID, level) {
    const member = activePackageDetails?.members?.find(candidate => candidate?.login === username);
    const previousLevel = Number(member?.level);
    if (!activePackageDetails?.package || level < 1 || level > 4 || level === previousLevel) return;
    const selector = activeView?.querySelector(`[data-cargo-permission-user="${CSS.escape(username)}"]`);
    const currentUsername = String(localStorage.getItem('username') || '').trim().toLowerCase();
    const transfersOwnership = level === 4 && Number(activePackageDetails.package.permission_level) === 4 &&
        username.toLowerCase() !== currentUsername;
    if (transfersOwnership) {
        const displayName = await resolveUserDisplayName(username);
        const confirmed = await showConfirm(t('team.transferOwnershipConfirm', {name: displayName}), {
            title: t('team.transferOwnership'), confirmText: t('team.transferOwnership')
        });
        if (!confirmed) {
            if (selector && typeof selector.setValue === 'function') selector.setValue(String(previousLevel));
            return;
        }
    }
    try {
        await cargoRequest(cargoAPIPath('crates', activePackageDetails.package.name, 'owners', userID || username), {
            method: 'PUT', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({level})
        });
        if (member) member.level = level;
        showAlert(t('cargo.memberUpdated'), 'success');
        if (level === 4) {
            await refreshCargoPackagePage();
        }
    } catch (error) {
        console.error('Failed to update Cargo member permission', error);
        if (selector && typeof selector.setValue === 'function') selector.setValue(String(previousLevel));
        showAlert(caughtErrorMessage(error, 'cargo.operationFailed'), 'error');
    }
}

/**
 * Send a package-team invitation at the selected permission level.
 * @param {SubmitEvent} event - Invitation form submission.
 * @returns {Promise<void>}
 */
async function submitCargoInvitation(event) {
    event.preventDefault();
    const form = event.currentTarget;
    const username = cargoUserSuggestions.input instanceof HTMLInputElement
        ? cargoUserSuggestions.input.value.trim()
        : '';
    if (!activePackageDetails?.package) return;
    if (!username) {
        showAlert(t('team.inviteUsernameRequired'), 'warning');
        cargoUserSuggestions.close(true);
        return;
    }
    const submit = form.querySelector('button[type="submit"]');
    if (submit) submit.disabled = true;
    try {
        await cargoRequest(cargoAPIPath('crates', activePackageDetails.package.name, 'owners'), {
            method: 'PUT', headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({users: [username], level: inviteLevel})
        });
        cargoUserSuggestions.clear();
        showAlert(t('cargo.inviteSent', {name: username}), 'success');
        if (activeAdministrator) {
            await refreshCargoPackagePage();
        }
    } catch (error) {
        console.error('Failed to invite Cargo package member', error);
        showAlert(caughtErrorMessage(error, 'cargo.operationFailed'), 'error');
    } finally {
        if (submit) submit.disabled = false;
    }
}

/**
 * Reload Cargo state after authentication or package invitation changes.
 * @param {Event} event - State change event.
 * @returns {void}
 */
function handleCargoStateChanged(event) {
    const changedRepository = event instanceof CustomEvent ? event.detail?.repository : '';
    if (!activeRepository || (changedRepository && changedRepository !== activeRepository)) return;
    void renderCargoRepository(window.location.pathname, {format: 'cargo'}, activeNavigate);
}

/**
 * Re-render cached Cargo content when the active language changes.
 * @returns {void}
 */
function handleCargoLanguageChanged() {
    if (!activeView || activeView.hidden) return;
    if (activeRouteKind === 'package' && activePackageDetails) renderCargoPackagePage();
    else if (activeRouteKind === 'overview') renderCargoOverview();
}

/**
 * Attach stable delegated Cargo UI listeners once.
 * @returns {void}
 */
function initializeCargoPageListeners() {
    if (listenersInitialized) return;
    listenersInitialized = true;
    document.getElementById('cargo-repository-view')?.addEventListener('click', handleCargoPageClick);
    window.addEventListener('authChanged', handleCargoStateChanged);
    window.addEventListener('cargoMembershipChanged', handleCargoStateChanged);
    window.addEventListener('languageChanged', handleCargoLanguageChanged);
}

/**
 * Render a Cargo repository overview or package-management subpage.
 * @param {string} path - Current browser route.
 * @param {object|null} repositoryDetails - Repository metadata.
 * @param {(path: string) => void} navigate - In-app navigation callback.
 * @returns {Promise<boolean>} Whether the route belongs to a Cargo repository.
 */
export async function renderCargoRepository(path, repositoryDetails, navigate) {
    initializeCargoPageListeners();
    const format = getRepositoryFormat(repositoryDetails?.format).id;
    if (format !== 'cargo') {
        hideCargoRepositoryView();
        return false;
    }
    const parts = path.split('/').filter(Boolean).map(decodePathSegment);
    if (parts.length === 0) {
        hideCargoRepositoryView();
        return false;
    }
    const sequence = ++cargoLoadSequence;
    activeRepository = parts[0];
    activeNavigate = navigate;
    activeView = document.getElementById('cargo-repository-view');
    if (!activeView) return true;
    activeView.hidden = false;
    activeView.classList.add('is-visible');

    if (parts.length === 1 || (parts.length === 2 && parts[1] === 'packages')) {
        await loadCargoOverview(sequence);
        return true;
    }
    if (parts.length === 3 && parts[1] === 'packages' && parts[2]) {
        await loadCargoPackage(parts[2], sequence);
        return true;
    }
    renderCargoError(t('cargo.routeNotFound'));
    return true;
}

/**
 * Hide and reset the Cargo repository subpage when browsing another format.
 * @returns {void}
 */
export function hideCargoRepositoryView() {
    cargoLoadSequence++;
    cargoUserSuggestions.detach();
    activeRepository = '';
    activeAdministrator = false;
    activePackageDetails = null;
    activePackageList = [];
    activePublicPackageNames = new Set();
    activePublicPackageTotal = 0;
    activeCatalogPage = 1;
    activeVersionListPage = 1;
    activeNavigate = null;
    activeSelectedVersion = '';
    activePackageHeroUpdater = null;
    activeCommandsUpdater = null;
    activeFactsUpdater = null;
    activeInspectionUpdater = null;
    activeVersionsUpdater = null;
    if (activeVersionsObserver) {
        activeVersionsObserver.disconnect();
        activeVersionsObserver = null;
    }
    activeView = document.getElementById('cargo-repository-view');
    hideRepositoryView(activeView);
}
