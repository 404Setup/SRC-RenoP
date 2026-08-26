/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {t} from './i18n.js';
import {showAlert} from './alert.js';
import {fetchProto, getAuthHeaders, putProto, sendProto} from './api.js';
import {logout} from './auth.js';
import {MavenRepositoriesResponse, ProxyConfig, Repository, StatusOk} from './proto/index.js';
import {el} from '@renop/ui/dom';
import {makeCustomSelect} from '@renop/ui/custom-select';
import {
    createButton,
    createCallout,
    createEmptyState,
    createFieldRow as makeFieldRow,
    createIcon,
    createSkeleton,
    createSubHeader,
    createToggleRow as makeToggleRow,
    RenopDialog,
    runButtonAction
} from './components.js';
import {
    animateFieldsToggle,
    makeCfgInput,
    makeInlineNumber,
    makeInlineToggle,
    makeTagListInput
} from './cfg-ui.js';
import {
    createRepositoryDraft,
    getRepositoryFormat,
    isValidRepositorySlug,
    listRepositoryFormats
} from './repository-formats.js';
import {paginateRepositoryNames, sortedRepositoryNames} from './repository-list.js';

let currentConfig = null;
let initialReposMap = {};
let globalProxyConfig = {selected: '', proxies: []};
const saveSeqByRepo = new Map();
const selectedRepositoryEngines = new Set();
const repositoryPageSize = 10;
let repositoryPage = 0;

/**
 * Build the color-only repository visibility indicator with an accessible label.
 * @param {string} visibility - Repository visibility.
 * @returns {HTMLSpanElement} Visibility indicator.
 */
function repositoryVisibilityIndicator(visibility) {
    const normalized = ['PUBLIC', 'HIDDEN', 'PRIVATE'].includes(visibility) ? visibility : 'PUBLIC';
    const labelKey = {
        PUBLIC: 'repos.visibilityPublic',
        HIDDEN: 'repos.visibilityHidden',
        PRIVATE: 'repos.visibilityPrivate'
    }[normalized];
    const label = t(labelKey);
    return el('span', {
        class: `cfg-repository-visibility is-${normalized.toLowerCase()}`,
        role: 'img', title: label, 'aria-label': label
    });
}

/**
 * Renders a repository-list skeleton placeholder in the repositories container.
 * @returns {void}
 */
function renderRepositoriesSkeleton() {
    const container = document.getElementById('repositories-container');
    if (!container) return;
    container.classList.remove('is-content-entering');
    container.innerHTML = '';
    container.appendChild(createSkeleton('repo', 2));
}

/**
 * Loads package repositories from the API and renders the repositories settings UI.
 * @returns {Promise<void>}
 */
export async function initRepositories() {
    renderRepositoriesSkeleton();
    try {
        const [repositoriesResult, proxyResult] = await Promise.all([
            fetchProto('/api/settings/repositories', MavenRepositoriesResponse),
            fetchProto('/api/settings/domain/proxy', ProxyConfig)
        ]);
        const {response, data} = repositoriesResult;
        if (proxyResult.response.ok && proxyResult.data) {
            globalProxyConfig = proxyResult.data;
        }
        if (response.ok && data) {
            const repos = data.repositories || {};
            currentConfig = {repositories: repos};
            initialReposMap = JSON.parse(JSON.stringify(repos));
            renderRepositories(document.getElementById('repositories-container'), currentConfig);
        } else if (response.status === 401 || response.status === 403) {
            logout('kicked');
        }
    } catch (e) {
        console.error('Failed to load repositories', e);
    }
}

/**
 * Renders all package repository sections (or empty state) into the container.
 * @param {HTMLElement} container - Repositories container element.
 * @param {{repositories?: Object.<string, object>}} data - Config holding a repositories map.
 * @returns {void}
 */
function renderRepositories(container, data) {
    if (!data.repositories) data.repositories = {};

    const originalHeight = container.offsetHeight;
    if (originalHeight > 0) {
        container.style.minHeight = `${originalHeight}px`;
    }

    try {
        const layout = el('div', {class: 'cfg-layout'});
        const allKeys = sortedRepositoryNames(data.repositories);
        const filteredKeys = sortedRepositoryNames(data.repositories, selectedRepositoryEngines);
        const page = paginateRepositoryNames(filteredKeys, repositoryPage, repositoryPageSize);
        repositoryPage = page.page;

        if (allKeys.length === 0) {
            layout.appendChild(createEmptyState(t('repos.noRepos')));
        } else if (filteredKeys.length === 0) {
            layout.appendChild(createEmptyState(t('repos.noFilteredRepos')));
        }

        page.names.forEach(repoKey => {
            const repo = data.repositories[repoKey];
            layout.appendChild(buildRepoSection(container, data, repoKey, repo));
        });

        container.innerHTML = '';
        container.classList.remove('is-content-entering');
        void container.offsetWidth;
        container.classList.add('is-content-entering');
        if (allKeys.length > 0) container.appendChild(buildRepositoryListToolbar(container, data));
        container.appendChild(layout);
        const pagination = buildRepositoryPagination(container, data, page);
        if (pagination) container.appendChild(pagination);
    } finally {
        container.style.minHeight = '';
    }
}

/**
 * Build an engine filter that supports selecting multiple repository protocols.
 * @param {HTMLElement} container - Repositories container element.
 * @param {{repositories?: Object.<string, object>}} data - Current repositories config.
 * @returns {HTMLElement} Filter toolbar.
 */
function buildRepositoryListToolbar(container, data) {
    const filters = el('div', {class: 'repository-engine-filters', role: 'group', 'aria-label': t('repos.filterByEngine')});
    const allSelected = selectedRepositoryEngines.size === 0;
    filters.appendChild(el('button', {
        type: 'button', class: `repository-engine-filter${allSelected ? ' is-active' : ''}`,
        'aria-pressed': String(allSelected),
        onclick: () => {
            selectedRepositoryEngines.clear();
            repositoryPage = 0;
            renderRepositories(container, data);
        }
    }, t('repos.filterAll')));
    for (const format of listRepositoryFormats()) {
        const selected = selectedRepositoryEngines.has(format.protocol);
        filters.appendChild(el('button', {
            type: 'button', class: `repository-engine-filter${selected ? ' is-active' : ''}`,
            'aria-pressed': String(selected),
            onclick: () => {
                if (selected) selectedRepositoryEngines.delete(format.protocol);
                else selectedRepositoryEngines.add(format.protocol);
                repositoryPage = 0;
                renderRepositories(container, data);
            }
        }, createIcon(format.icon || 'repositoryFiles'), el('span', {}, t(format.labelKey))));
    }
    return el('div', {class: 'repository-list-toolbar'},
        el('span', {class: 'repository-list-toolbar-label'}, t('repos.filterByEngine')),
        filters
    );
}

/**
 * Build bounded repository-list pagination controls.
 * @param {HTMLElement} container - Repositories container element.
 * @param {{repositories?: Object.<string, object>}} data - Current repositories config.
 * @param {{page: number, pages: number, total: number, start: number, end: number}} page - Current page metadata.
 * @returns {HTMLElement|null} Pagination control when more than one page exists.
 */
function buildRepositoryPagination(container, data, page) {
    if (page.pages <= 1) return null;
    const previous = el('button', {
        type: 'button', class: 'repository-pagination-btn', disabled: page.page === 0,
        'aria-label': t('common.prev'),
        onclick: () => {
            repositoryPage = Math.max(0, page.page - 1);
            renderRepositories(container, data);
        }
    }, createIcon('chevronLeft'), el('span', {}, t('common.prev')));
    const next = el('button', {
        type: 'button', class: 'repository-pagination-btn', disabled: page.page + 1 >= page.pages,
        'aria-label': t('common.next'),
        onclick: () => {
            repositoryPage = Math.min(page.pages - 1, page.page + 1);
            renderRepositories(container, data);
        }
    }, el('span', {}, t('common.next')), createIcon('chevronRight'));
    return el('nav', {class: 'repository-list-pagination', 'aria-label': t('repos.paginationLabel')},
        el('span', {class: 'repository-pagination-info', 'aria-live': 'polite'},
            t('repos.paginationShowing', {start: page.start, end: page.end, total: page.total})),
        el('div', {class: 'repository-pagination-controls'}, previous,
            el('span', {}, `${page.page + 1} / ${page.pages}`), next)
    );
}

/**
 * Resolve a safe localized migration failure without exposing backend response text.
 * @param {Response} response - Failed migration response.
 * @returns {string} Localized user-facing error.
 */
function repositoryMigrationErrorMessage(response) {
    if (response.headers.get('X-Renop-Error-Code') === 'repository_migration_pending_gpg') {
        return t('repos.migrationPendingGpg');
    }
    return t('repos.migrationFailed');
}

/**
 * Build the Maven/files-only engine migration control.
 * @param {string} repository - Repository name.
 * @param {object} format - Canonical current format descriptor.
 * @returns {HTMLElement|null} Migration control or null for immutable engines.
 */
function buildRepositoryMigrationControl(repository, format) {
    if (format.protocol !== 'maven' && format.protocol !== 'files') return null;
    const target = format.protocol === 'maven' ? 'files' : 'maven';
    const labelKey = target === 'files' ? 'repos.migrationToFiles' : 'repos.migrationToMaven';
    const hintKey = target === 'files' ? 'repos.migrationToFilesHint' : 'repos.migrationToMavenHint';
    const confirmKey = target === 'files' ? 'repos.migrationToFilesConfirm' : 'repos.migrationToMavenConfirm';
    const button = createButton(t(labelKey), {
        class: 'pill-btn pill-btn--soft repository-migration-button',
        icon: 'refresh'
    });
    button.addEventListener('click', async event => {
        event.stopPropagation();
        const confirmed = await window.showConfirm(t(confirmKey, {name: repository}), {danger: false});
        if (!confirmed) return;
        await runButtonAction(button, async () => {
            try {
                const {response} = await sendProto(
                    `/api/settings/repositories/${encodeURIComponent(repository)}/migrate/${target}`,
                    'POST', null, null, StatusOk
                );
                if (!response.ok) {
                    showAlert(repositoryMigrationErrorMessage(response), 'error');
                    return;
                }
                const targetFormat = getRepositoryFormat(target);
                showAlert(t('repos.migrationSuccess', {format: t(targetFormat.labelKey)}), 'success');
                window.dispatchEvent(new CustomEvent('repositorySettingsChanged', {
                    detail: {repository, format: targetFormat.id}
                }));
                await initRepositories();
            } catch (error) {
                console.error('Failed to migrate repository engine', error);
                showAlert(t('repos.migrationFailed'), 'error');
            }
        });
    });
    return el('div', {class: 'repository-migration-control'},
        el('div', {class: 'repository-migration-copy'},
            el('strong', {}, t('repos.engineMigration')),
            el('span', {}, t(hintKey))
        ),
        button
    );
}

/**
 * Builds a collapsible UI section for a package repository (format, visibility, S3, mirrors).
 * @param {HTMLElement} container - Parent repositories container (used for remove animation).
 * @param {{repositories: Object.<string, object>}} data - Full repositories config.
 * @param {string} repoKey - Repository name/key.
 * @param {object} repo - Repository settings object (mutated by controls).
 * @returns {HTMLElement} The section element.
 */
function buildRepoSection(container, data, repoKey, repo) {
    const format = getRepositoryFormat(repo.format);
    const section = el('div', {class: 'cfg-section is-collapsed'});
    section.dataset.repository = repoKey;

    const header = el('div', {class: 'cfg-section-header'});

    const formatLabel = t(format.labelKey);
    let visibilityIndicator = repositoryVisibilityIndicator(repo.visibility || 'PUBLIC');
    const iconBox = el('div', {
        class: `cfg-section-icon cfg-repository-type-icon is-${format.protocol}`
    },
    el('span', {
        class: 'cfg-repository-format-icon', role: 'img', title: formatLabel, 'aria-label': formatLabel
    }, createIcon(format.icon || 'repositoryFiles')),
    visibilityIndicator);

    const meta = el('div', {class: 'cfg-section-meta'});
    const titleRow = el('div', {class: 'cfg-section-title-row'});
    titleRow.appendChild(el('p', {class: 'cfg-section-title'}, repoKey));
    meta.appendChild(titleRow);

    const mirrorCount = (repo.mirrors || []).length;
    meta.appendChild(el('p', {class: 'cfg-section-subtitle'},
        t('repos.mirrorCount', {count: mirrorCount})
    ));

    const deleteBtn = el('button', {
        class: 'pill-btn pill-btn--danger pill-btn--sm cfg-repository-delete-btn',
        type: 'button',
        title: t('repos.deleteRepoTitle', {name: repoKey}),
        'aria-label': t('repos.deleteRepoTitle', {name: repoKey})
    }, createIcon('delete'));
    deleteBtn.addEventListener('click', async (e) => {
        e.stopPropagation();
        if (await window.showConfirm(t('repos.confirmDelete', {name: repoKey}))) {
            try {
                const headers = getAuthHeaders();
                const response = await fetch(`/api/settings/repositories/${encodeURIComponent(repoKey)}`, {
                    method: 'DELETE', headers
                });
                if (response.status === 401 || response.status === 403) {
                    logout('kicked');
                    return;
                }
                if (response.ok) {
                    delete data.repositories[repoKey];
                    delete initialReposMap[repoKey];
                    saveSeqByRepo.delete(repoKey);
                    showAlert(t('repos.deletedSuccess', {name: repoKey}), 'success');
                    animateRemoveRepoSection(section, container, data);
                } else {
                    const errText = await response.text();
                    showAlert(errText || t('repos.failedDelete'), 'error');
                }
            } catch {
                showAlert(t('repos.failedDelete'), 'error');
            }
        }
    });

    const chevronBox = el('div', {class: 'cfg-section-chevron'}, createIcon('chevronDown'));

    header.appendChild(iconBox);
    header.appendChild(meta);
    header.appendChild(deleteBtn);
    header.appendChild(chevronBox);
    section.appendChild(header);

    header.addEventListener('click', () => {
        section.classList.toggle('is-collapsed');
    });

    const bodyInner = el('div', {class: 'cfg-section-body-inner'});

    const fields = el('div', {class: 'cfg-fields'});

    const formatValue = el('span', {class: 'cfg-readonly-value'}, t(format.labelKey));
    fields.appendChild(makeFieldRow(t('repos.format'), t('repos.formatImmutableDesc'), formatValue));

    const visOptions = [
        {value: 'PUBLIC', label: t('repos.visibilityPublic')},
        {value: 'HIDDEN', label: t('repos.visibilityHidden')},
        {value: 'PRIVATE', label: t('repos.visibilityPrivate')}
    ];

    /**
     * Persist visibility and replace only the visibility indicator on the type icon.
     * @param {string} value - Selected visibility.
     * @returns {void}
     */
    function handleVisibilityChange(value) {
        repo.visibility = value;
        if (visibilityIndicator.isConnected) {
            const newIndicator = repositoryVisibilityIndicator(value);
            iconBox.replaceChild(newIndicator, visibilityIndicator);
            visibilityIndicator = newIndicator;
        }
        saveRepoSettings(repoKey, repo);
    }

    /**
     * Persist the Maven-only redeployment setting.
     * @param {boolean} checked - Whether redeployment is allowed.
     * @returns {void}
     */
    function handleRedeploymentChange(checked) {
        repo.allow_redeployment = checked;
        saveRepoSettings(repoKey, repo);
    }

    /**
     * Persist the Maven-only GPG signature requirement.
     * @param {boolean} checked - Whether uploads require a valid signature.
     * @returns {void}
     */
    function handleGpgRequirementChange(checked) {
        repo.require_gpg_signature = checked;
        saveRepoSettings(repoKey, repo);
    }

    /**
     * Persist the Maven browser layout without changing its repository protocol.
     * @param {string} value - `modern` or `classic`.
     * @returns {void}
     */
    function handleMavenLayoutChange(value) {
        repo.format = value === 'classic' ? 'maven-classic' : 'maven';
        saveRepoSettings(repoKey, repo);
    }

    const visSelect = makeCustomSelect(
        visOptions,
        repo.visibility || 'PUBLIC',
        handleVisibilityChange
    );
    fields.appendChild(makeFieldRow(t('repos.visibility'), t('repos.visibilityDesc'), visSelect));

    if (format.protocol === 'maven') {
        const layoutSelect = makeCustomSelect([
            {value: 'modern', label: t('repos.mavenLayoutModern')},
            {value: 'classic', label: t('repos.mavenLayoutClassic')}
        ], format.layout || 'modern', handleMavenLayoutChange);
        fields.appendChild(makeFieldRow(t('repos.mavenLayout'), t('repos.mavenLayoutDesc'), layoutSelect));
    }

    if (format.supportsRedeployment) {
        fields.appendChild(makeToggleRow(
            t('repos.allowRedeploy'),
            t('repos.allowRedeployDesc'),
            repo.allow_redeployment === true,
            handleRedeploymentChange
        ));
    }
    if (format.supportsGpg) {
        fields.appendChild(makeToggleRow(
            t('repos.requireGpgSignature'),
            t('repos.requireGpgSignatureDesc'),
            repo.require_gpg_signature === true,
            handleGpgRequirementChange
        ));
    }
    const migrationControl = buildRepositoryMigrationControl(repoKey, format);
    if (migrationControl) fields.appendChild(migrationControl);

    bodyInner.appendChild(fields);
    bodyInner.appendChild(buildS3Section(repoKey, repo));
    bodyInner.appendChild(buildMirrorsSection(container, data, repoKey, repo, meta));

    const bodyContainer = el('div', {class: 'cfg-section-body'}, bodyInner);
    section.appendChild(bodyContainer);

    return section;
}

/**
 * Builds the S3 storage subsection for a repository (enable toggle and credential fields).
 * @param {string} repoKey - Repository name/key.
 * @param {object} repo - Repository settings object (ensures repo.s3 exists).
 * @returns {HTMLElement} Wrapper element containing S3 controls.
 */
function buildS3Section(repoKey, repo) {
    const s3 = repo.s3 || {
        enabled: false, endpoint: '', bucket: '', region: 'auto',
        access_key_id: '', secret_access_key: '', key_prefix: '',
        force_path_style: true, redirect_downloads: false
    };
    if (!repo.s3) repo.s3 = s3;

    const wrapper = el('div', {});
    wrapper.appendChild(createSubHeader('storage', t('repos.s3Title')));

    const s3Fields = el('div', {class: 'cfg-fields'});

    const fieldsContainer = el('form', {
        class: 'cfg-fields',
        action: 'javascript:void(0);',
        onsubmit: e => e.preventDefault(),
        style: {display: s3.enabled ? '' : 'none'}
    });

    s3Fields.appendChild(makeToggleRow(
        t('repos.enableS3'),
        t('repos.enableS3Desc'),
        s3.enabled === true,
        checked => {
            repo.s3.enabled = checked;
            animateFieldsToggle(fieldsContainer, checked);
            saveRepoSettings(repoKey, repo);
        }
    ));

    const textFields = [
        {
            id: 'endpoint',
            label: t('repos.s3Endpoint'),
            hint: t('repos.s3EndpointHint'),
            placeholder: 'https://...'
        },
        {id: 'bucket', label: t('repos.s3Bucket'), hint: t('repos.s3BucketHint'), placeholder: 'my-repo-bucket'},
        {
            id: 'key_prefix',
            label: t('repos.s3KeyPrefix'),
            hint: t('repos.s3KeyPrefixHint'),
            placeholder: 'renop/production'
        },
        {id: 'region', label: t('repos.s3Region'), hint: t('repos.s3RegionHint'), placeholder: 'auto'},
        {id: 'access_key_id', label: t('repos.s3AccessKey'), hint: t('repos.s3AccessKeyHint'), placeholder: 'AKIA...'},
        {
            id: 'secret_access_key',
            label: t('repos.s3SecretKey'),
            hint: t('repos.s3SecretKeyHint'),
            placeholder: '••••••',
            type: 'password'
        },
    ];

    textFields.forEach(f => {
        const input = makeCfgInput(s3[f.id] || '', f.placeholder, f.type || 'text', v => {
            repo.s3[f.id] = v;
            saveRepoSettings(repoKey, repo);
        });
        fieldsContainer.appendChild(makeFieldRow(f.label, f.hint, input));
    });

    fieldsContainer.appendChild(makeToggleRow(
        t('repos.s3ForcePathStyle'),
        t('repos.s3ForcePathStyleHint'),
        s3.force_path_style === true,
        checked => {
            repo.s3.force_path_style = checked;
            saveRepoSettings(repoKey, repo);
        }
    ));

    fieldsContainer.appendChild(makeToggleRow(
        t('repos.s3RedirectDownloads'),
        t('repos.s3RedirectDownloadsHint'),
        s3.redirect_downloads === true,
        checked => {
            repo.s3.redirect_downloads = checked;
            saveRepoSettings(repoKey, repo);
        }
    ));

    s3Fields.appendChild(fieldsContainer);
    wrapper.appendChild(s3Fields);
    return wrapper;
}

/**
 * Builds the mirrors list subsection with add-mirror control and mirror blocks.
 * @param {HTMLElement} container - Parent repositories container.
 * @param {{repositories: Object.<string, object>}} data - Full repositories config.
 * @param {string} repoKey - Repository name/key.
 * @param {object} repo - Repository settings (mirrors array is mutated).
 * @param {HTMLElement|null} metaNode - Section meta node used to update mirror count subtitle.
 * @returns {HTMLElement} Mirrors section element.
 */
function buildMirrorsSection(container, data, repoKey, repo, metaNode) {
    const section = el('div', {});
    const addBtn = createButton(t('repos.addMirror'), {
        class: 'pill-btn pill-btn--soft pill-btn--sm',
        icon: 'plus',
        iconProps: {width: '14', height: '14'},
        title: t('repos.addMirror')
    });

    const subHeader = createSubHeader('network', t('details.mirrors'), addBtn);
    const list = el('div', {class: 'mirrors-list'});
    addBtn.addEventListener('click', () => {
        if (!repo.mirrors) repo.mirrors = [];
        const newMirror = {
            name: t('details.unnamedMirror') + ' ' + (repo.mirrors.length + 1),
            url: '',
            persist: true,
            negative_cache: true,
            cache_ttl_secs: 3600,
            timeout_secs: 30,
            enabled_date: new Date().toISOString().split('T')[0]
        };
        repo.mirrors.push(newMirror);
        saveRepoSettings(repoKey, repo);

        if (metaNode) {
            const subtitle = metaNode.querySelector('.cfg-section-subtitle');
            if (subtitle) {
                subtitle.textContent = t('repos.mirrorCount', {count: repo.mirrors.length});
            }
        }

        const emptyNotice = list.querySelector('.no-mirrors-msg');
        if (emptyNotice) {
            emptyNotice.remove();
        }

        const newIdx = repo.mirrors.length - 1;
        const newBlock = buildMirrorBlock(container, data, repoKey, repo, newMirror, newIdx, metaNode);

        newBlock.style.overflow = 'hidden';
        newBlock.style.maxHeight = '0';
        newBlock.style.opacity = '0';
        newBlock.style.transition = 'max-height 0.35s cubic-bezier(0.16, 1, 0.3, 1), opacity 0.3s ease';

        list.appendChild(newBlock);

        const targetHeight = Math.max(newBlock.scrollHeight, 400);

        void newBlock.offsetHeight;

        newBlock.style.maxHeight = targetHeight + 'px';
        newBlock.style.opacity = '1';

        setTimeout(() => {
            newBlock.style.maxHeight = '';
            newBlock.style.overflow = '';
        }, 350);
    });

    section.appendChild(subHeader);

    if (!repo.mirrors || repo.mirrors.length === 0) {
        list.appendChild(el('div', {
            class: 'no-mirrors-msg',
            style: {
                padding: '1.25rem 1.5rem',
                fontSize: '0.85rem',
                opacity: '0.5',
                fontStyle: 'italic'
            }
        }, t('repos.noMirrors')));
    } else {
        repo.mirrors.forEach((mirror, idx) => {
            list.appendChild(buildMirrorBlock(container, data, repoKey, repo, mirror, idx, metaNode));
        });
    }

    section.appendChild(list);
    return section;
}

/**
 * Builds mirror routing options from the global proxy settings.
 * @param {string} selected - Current mirror selector, if any.
 * @returns {Array<{value: string, label: string}>} Dropdown options.
 */
function mirrorProxyOptions(selected) {
    const current = typeof selected === 'string' ? selected.trim() : '';
    const options = [
        {value: '', label: t('repos.proxyGlobal')},
        {value: 'direct', label: t('repos.proxyDirect')}
    ];
    const proxies = Array.isArray(globalProxyConfig.proxies) ? globalProxyConfig.proxies : [];
    const names = new Set();
    for (const proxy of proxies) {
        const name = String(proxy?.name || '').trim();
        if (!name || names.has(name.toLowerCase())) continue;
        names.add(name.toLowerCase());
        options.push({value: name, label: name});
    }
    if (current && current.toLowerCase() !== 'direct' && !names.has(current.toLowerCase()) &&
        !['global', 'inherit'].includes(current.toLowerCase())) {
        options.push({value: current, label: current});
    }
    return options;
}

/**
 * Builds a single mirror configuration block (name, URL, allow/deny, auth, cache options).
 * @param {HTMLElement} container - Parent repositories container.
 * @param {{repositories: Object.<string, object>}} data - Full repositories config.
 * @param {string} repoKey - Repository name/key.
 * @param {object} repo - Repository settings (mirrors array is mutated on remove).
 * @param {object} mirror - Mirror config object (mutated by controls).
 * @param {number} idx - Initial mirror index (fallback for label when not found in list).
 * @param {HTMLElement|null} metaNode - Section meta node for mirror count updates.
 * @returns {HTMLElement} Mirror block element.
 */
function buildMirrorBlock(container, data, repoKey, repo, mirror, idx, metaNode) {
	const format = getRepositoryFormat(repo.format);
	const isCargo = format.id === 'cargo';
	const isDocker = format.id === 'docker';
    const block = el('div', {
        class: 'mirror-block',
        style: {
            borderBottom: '1px solid color-mix(in srgb, var(--border-color) 60%, transparent)',
        }
    });

    const mirrorHeader = el('div', {
        style: {
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            padding: '0.7rem 1.5rem',
            background: 'color-mix(in srgb, var(--item-hover-bg) 15%, transparent)',
            borderBottom: '1px solid color-mix(in srgb, var(--border-color) 40%, transparent)',
        }
    });
    const mirrorLabel = el('span', {
        style: {fontSize: '0.82rem', fontWeight: '600', opacity: '0.7'}
    });

    /**
     * Refreshes the mirror header label from the current index and name.
     * @returns {void}
     */
    function updateMirrorLabel() {
        const curIdx = (repo.mirrors || []).indexOf(mirror);
        const displayIdx = curIdx !== -1 ? curIdx + 1 : idx + 1;
        mirrorLabel.textContent = t('repos.mirrorLabel', {
            index: displayIdx,
            name: mirror.name || t('details.unnamedMirror')
        });
    }

    updateMirrorLabel();
    block._updateMirrorLabel = updateMirrorLabel;

    const removeBtn = createButton(t('common.remove'), {
        class: 'pill-btn pill-btn--ghost-danger pill-btn--sm',
        icon: 'close',
        iconProps: {width: '14', height: '14'},
        title: t('repos.removeMirrorTitle')
    });
    removeBtn.addEventListener('click', () => {
        const curIdx = (repo.mirrors || []).indexOf(mirror);
        if (curIdx !== -1) {
            repo.mirrors.splice(curIdx, 1);
        }
        saveRepoSettings(repoKey, repo);

        if (metaNode) {
            const subtitle = metaNode.querySelector('.cfg-section-subtitle');
            if (subtitle) {
                subtitle.textContent = t('repos.mirrorCount', {count: (repo.mirrors || []).length});
            }
        }

        block.style.overflow = 'hidden';
        block.style.maxHeight = block.offsetHeight + 'px';
        block.style.opacity = '1';
        block.style.transition = 'max-height 0.3s cubic-bezier(0.4, 0, 0.2, 1), opacity 0.25s ease';
        void block.offsetHeight;
        block.style.maxHeight = '0';
        block.style.opacity = '0';

        setTimeout(() => {
            const parentList = block.parentElement;
            block.remove();
            if (parentList) {
                const remainingBlocks = parentList.querySelectorAll('.mirror-block');
                remainingBlocks.forEach((b) => {
                    if (b._updateMirrorLabel) b._updateMirrorLabel();
                });

                if (!repo.mirrors || repo.mirrors.length === 0) {
                    const emptyEl = el('div', {
                        class: 'no-mirrors-msg',
                        style: {
                            padding: '1.25rem 1.5rem',
                            fontSize: '0.85rem',
                            opacity: '0.5',
                            fontStyle: 'italic'
                        }
                    }, t('repos.noMirrors'));
                    parentList.appendChild(emptyEl);
                }
            }
        }, 300);
    });
    mirrorHeader.appendChild(mirrorLabel);
    mirrorHeader.appendChild(removeBtn);
    block.appendChild(mirrorHeader);

    const fields = el('div', {class: 'cfg-fields'});

    /**
     * Persist the mirror display name and refresh its section heading.
     * @param {string} value - Mirror display name.
     * @returns {void}
     */
    function handleMirrorNameChange(value) {
        mirror.name = value;
        updateMirrorLabel();
        saveRepoSettings(repoKey, repo);
    }

    /**
     * Persist the mirror index or repository base URL.
     * @param {string} value - Mirror base URL.
     * @returns {void}
     */
    function handleMirrorUrlChange(value) {
        mirror.url = value;
        saveRepoSettings(repoKey, repo);
    }

    let mirrorPlaceholder = 'Maven Central';
    let mirrorUrlLabel = 'repos.mavenMirrorUrl';
    let mirrorUrlHint = 'repos.mavenMirrorUrlHint';
    let mirrorUrlPlaceholder = 'https://repo1.maven.org/maven2/';
    let addRulePlaceholder = 'repos.addRulePlaceholder';
    let emptyAllowList = 'repos.emptyAllowList';
    let emptyDenyList = 'repos.emptyDenyList';
    let allowHint = 'repos.mirrorAllowListHint';
    let denyHint = 'repos.mirrorDenyListHint';

    if (isCargo) {
        mirrorPlaceholder = 'crates.io';
        mirrorUrlLabel = 'repos.cargoMirrorUrl';
        mirrorUrlHint = 'repos.cargoMirrorUrlHint';
        mirrorUrlPlaceholder = 'https://index.crates.io/';
        addRulePlaceholder = 'repos.cargoAddRulePlaceholder';
        emptyAllowList = 'repos.cargoEmptyAllowList';
        emptyDenyList = 'repos.cargoEmptyDenyList';
        allowHint = 'repos.cargoMirrorAllowListHint';
        denyHint = 'repos.cargoMirrorDenyListHint';
    } else if (isDocker) {
        mirrorPlaceholder = 'Docker Hub';
        mirrorUrlLabel = 'repos.dockerMirrorUrl';
        mirrorUrlHint = 'repos.dockerMirrorUrlHint';
        mirrorUrlPlaceholder = 'https://registry-1.docker.io';
        addRulePlaceholder = 'repos.dockerAddRulePlaceholder';
        emptyAllowList = 'repos.dockerEmptyAllowList';
        emptyDenyList = 'repos.dockerEmptyDenyList';
        allowHint = 'repos.dockerMirrorAllowListHint';
        denyHint = 'repos.dockerMirrorDenyListHint';
    }

    fields.appendChild(makeFieldRow(t('repos.mirrorName'), t('repos.mirrorNameHint'),
        makeCfgInput(mirror.name || '', mirrorPlaceholder, 'text', handleMirrorNameChange)
    ));

    fields.appendChild(makeFieldRow(
        t(mirrorUrlLabel),
        t(mirrorUrlHint),
        makeCfgInput(mirror.url || '', mirrorUrlPlaceholder, 'text', handleMirrorUrlChange)
    ));

    if (format.supportsArtifactTemplate) {
        /**
         * Stores the optional Cargo artifact URL template.
         * @param {string} value - Artifact URL template.
         * @returns {void}
         */
        const updateArtifactUrl = value => {
            if (value) mirror.artifact_url = value;
            else delete mirror.artifact_url;
            saveRepoSettings(repoKey, repo);
        };
        fields.appendChild(makeFieldRow(t('repos.artifactUrl'), t('repos.artifactUrlHint'),
            makeCfgInput(mirror.artifact_url || '', 'https://static.crates.io/crates/{crate}/{crate}-{version}.crate', 'text', updateArtifactUrl)
        ));
    }

    const proxySelection = typeof mirror.proxy === 'string' ? mirror.proxy.trim() : '';
    const proxySelect = makeCustomSelect(
        mirrorProxyOptions(proxySelection),
        proxySelection,
        value => {
            if (value) {
                mirror.proxy = value;
            } else {
                delete mirror.proxy;
            }
            saveRepoSettings(repoKey, repo);
        }
    );
    fields.appendChild(makeFieldRow(
        t('repos.proxyMode'),
        t('repos.proxyModeHint'),
        proxySelect
    ));

    const conflictWarningEl = createCallout('warning', t('repos.ruleConflictWarning'), 'warning');
    conflictWarningEl.className = 'cfg-warning-banner';
    conflictWarningEl.style.display = (Array.isArray(mirror.allow_artifacts) && mirror.allow_artifacts.length > 0 &&
        Array.isArray(mirror.deny_artifacts) && mirror.deny_artifacts.length > 0) ? 'flex' : 'none';

    /**
     * Shows or hides the allow/deny rule conflict warning banner.
     * @returns {void}
     */
    function updateConflictWarning() {
        const hasAllow = Array.isArray(mirror.allow_artifacts) && mirror.allow_artifacts.length > 0;
        const hasDeny = Array.isArray(mirror.deny_artifacts) && mirror.deny_artifacts.length > 0;
        animateFieldsToggle(conflictWarningEl, hasAllow && hasDeny);
    }

    const allowInput = makeTagListInput({
        items: mirror.allow_artifacts || [],
        type: 'allow',
        placeholder: t(addRulePlaceholder),
        emptyText: t(emptyAllowList),
        onChange: (newList) => {
            if (newList.length > 0) mirror.allow_artifacts = newList;
            else delete mirror.allow_artifacts;
            updateConflictWarning();
            saveRepoSettings(repoKey, repo);
        }
    });

    const denyInput = makeTagListInput({
        items: mirror.deny_artifacts || [],
        type: 'deny',
        placeholder: t(addRulePlaceholder),
        emptyText: t(emptyDenyList),
        onChange: (newList) => {
            if (newList.length > 0) mirror.deny_artifacts = newList;
            else delete mirror.deny_artifacts;
            updateConflictWarning();
            saveRepoSettings(repoKey, repo);
        }
    });

    fields.appendChild(makeFieldRow(
		t('repos.mirrorAllowList'),
		t(allowHint),
		allowInput,
		'cfg-field-row--top-align'
	));
    fields.appendChild(makeFieldRow(
		t('repos.mirrorDenyList'),
		t(denyHint),
		denyInput,
		'cfg-field-row--top-align'
	));
    fields.appendChild(conflictWarningEl);

    let currentMethod = (mirror.authorization && mirror.authorization.method)
        ? String(mirror.authorization.method).toLowerCase()
        : 'none';
    if (currentMethod === 'username/password') currentMethod = 'basic';
    if (currentMethod === 'bearer') currentMethod = 'token';
    if (currentMethod === 'custom_header' || currentMethod === 'request-header' || currentMethod === 'header') {
        currentMethod = 'custom-header';
    }
    if (currentMethod !== 'none' && currentMethod !== 'basic' && currentMethod !== 'token' && currentMethod !== 'custom-header') {
        currentMethod = 'none';
    }

    const credsRow = el('form', {
        action: 'javascript:void(0);',
        onsubmit: e => e.preventDefault(),
        style: {
            display: currentMethod === 'none' ? 'none' : 'flex',
            flexDirection: 'column',
            gap: '0'
        }
    });
    credsRow._fieldsVisible = currentMethod !== 'none';

    const userInput = makeCfgInput(
        mirror.authorization ? mirror.authorization.login || '' : '',
        currentMethod === 'custom-header' ? t('repos.headerName') : t('repos.username'), 'text',
        v => {
            if (mirror.authorization) {
                mirror.authorization.login = v;
                saveRepoSettings(repoKey, repo);
            }
        },
        {autocomplete: 'username'}
    );
    const passInput = makeCfgInput(
        mirror.authorization ? mirror.authorization.password || '' : '',
        (currentMethod === 'token' || currentMethod === 'custom-header') ? t('repos.tokenSecret') : t('repos.password'),
        'password',
        v => {
            if (mirror.authorization) {
                mirror.authorization.password = v;
                saveRepoSettings(repoKey, repo);
            }
        },
        {autocomplete: 'new-password'}
    );

    const authOptions = [
        {value: 'none', label: t('repos.authNone')},
        {value: 'basic', label: t('repos.authBasic')},
        {value: 'token', label: t('repos.authToken')},
        {value: 'custom-header', label: t('repos.authCustomHeader')}
    ];

    const userFieldRow = makeFieldRow(
        currentMethod === 'custom-header' ? t('repos.headerName') : t('repos.username'),
        null,
        userInput
    );
    let userFieldVisible = currentMethod !== 'token';
    userFieldRow.style.display = userFieldVisible ? '' : 'none';
    userFieldRow._fieldsVisible = userFieldVisible;
    const passFieldRow = makeFieldRow(
        (currentMethod === 'token' || currentMethod === 'custom-header') ? t('repos.tokenSecret') : t('repos.password'),
        null,
        passInput
    );

    const authSelect = makeCustomSelect(authOptions, currentMethod, val => {
        const credentialsVisible = credsRow._fieldsVisible === true &&
            credsRow.style.display !== 'none' && !credsRow._animTimer1;
        if (val === 'none') {
            delete mirror.authorization;
            animateFieldsToggle(credsRow, false);
        } else {
            mirror.authorization = {
                method: val === 'token' ? 'bearer' : val,
                login: userInput.value,
                password: passInput.value
            };
            const showUser = val !== 'token';
            if (val === 'token') {
                passInput.placeholder = t('repos.tokenSecret');
                userFieldRow.querySelector('.cfg-label-text').textContent = t('repos.username');
                passFieldRow.querySelector('.cfg-label-text').textContent = t('repos.tokenSecret');
            } else if (val === 'custom-header') {
                userInput.placeholder = t('repos.headerName');
                passInput.placeholder = t('repos.tokenSecret');
                userFieldRow.querySelector('.cfg-label-text').textContent = t('repos.headerName');
                passFieldRow.querySelector('.cfg-label-text').textContent = t('repos.tokenSecret');
            } else {
                userInput.placeholder = t('repos.username');
                passInput.placeholder = t('repos.password');
                userFieldRow.querySelector('.cfg-label-text').textContent = t('repos.username');
                passFieldRow.querySelector('.cfg-label-text').textContent = t('repos.password');
            }

            if (!credentialsVisible) {
                // The parent is hidden/animating: establish the final child
                // display before measuring the parent's opening height.
                userFieldVisible = showUser;
                userFieldRow.style.display = showUser ? '' : 'none';
                userFieldRow.style.height = '';
                userFieldRow.style.transition = '';
                userFieldRow.style.overflow = '';
                animateFieldsToggle(credsRow, true);
            } else if (showUser !== userFieldVisible) {
                userFieldVisible = showUser;
                animateFieldsToggle(userFieldRow, showUser);
            }
        }
        saveRepoSettings(repoKey, repo);
    });

    fields.appendChild(makeFieldRow(t('repos.authMethod'), t('repos.authMethodHint'), authSelect));

    credsRow.appendChild(userFieldRow);
    credsRow.appendChild(passFieldRow);
    fields.appendChild(credsRow);

    const optionsRow = el('div', {
        class: 'cfg-field-row',
        style: {flexWrap: 'wrap', gap: '1.5rem'}
    });

    const optLabel = el('div', {class: 'cfg-field-label'});
    optLabel.appendChild(el('span', {class: 'cfg-label-text'}, t('repos.cacheOptions')));
    optLabel.appendChild(el('span', {class: 'cfg-label-hint'}, t('repos.cacheOptionsHint')));
    optionsRow.appendChild(optLabel);

    const optControl = el('div', {
        class: 'cfg-field-control',
        style: {display: 'flex', flexWrap: 'wrap', gap: '1.25rem', alignItems: 'center', justifyContent: 'flex-end'}
    });

    optControl.appendChild(makeInlineToggle(t('repos.optPersist'), mirror.persist !== false, checked => {
        mirror.persist = checked;
        saveRepoSettings(repoKey, repo);
    }));
    optControl.appendChild(makeInlineToggle(t('repos.optNegCache'), mirror.negative_cache !== false, checked => {
        mirror.negative_cache = checked;
        saveRepoSettings(repoKey, repo);
    }));
    optControl.appendChild(makeInlineNumber(t('repos.optTtl'), mirror.cache_ttl_secs ?? 3600, v => {
        const n = Number(v);
        if (!Number.isFinite(n) || n < 0) return;
        mirror.cache_ttl_secs = Math.trunc(n);
        saveRepoSettings(repoKey, repo);
    }));
    optControl.appendChild(makeInlineNumber(t('repos.optTimeout'), mirror.timeout_secs ?? 30, v => {
        const n = Number(v);
        if (!Number.isFinite(n) || n < 0) return;
        mirror.timeout_secs = Math.trunc(n);
        saveRepoSettings(repoKey, repo);
    }));

    optionsRow.appendChild(optControl);
    fields.appendChild(optionsRow);

    block.appendChild(fields);
    return block;
}

/**
 * Animate removal, then rebuild filters and pagination against the updated repository map.
 * @param {HTMLElement|null} section - Section to animate out; falls back to full re-render if null.
 * @param {HTMLElement} container - Repositories container element.
 * @param {{repositories?: Object.<string, object>}} data - Current repositories config.
 * @returns {void}
 */
function animateRemoveRepoSection(section, container, data) {
    if (!section) {
        renderRepositories(container, data);
        return;
    }
    section.classList.add('cfg-section--leaving');
    section.style.pointerEvents = 'none';
    let settled = false;
    const finish = () => {
        if (settled) return;
        settled = true;
        renderRepositories(container, data);
    };
    const onEnd = (e) => {
        if (e.target !== section) return;
        section.removeEventListener('animationend', onEnd);
        finish();
    };
    section.addEventListener('animationend', onEnd);
    setTimeout(finish, 380);
}

/**
 * Builds and animates in a new repository section after creation.
 * @param {HTMLElement} container - Repositories container element.
 * @param {{repositories: Object.<string, object>}} data - Current repositories config.
 * @param {string} repoKey - New repository name/key.
 * @returns {void}
 */
function animateAddRepoSection(container, data, repoKey) {
    selectedRepositoryEngines.clear();
    const keys = sortedRepositoryNames(data.repositories);
    repositoryPage = Math.max(0, Math.floor(keys.indexOf(repoKey) / repositoryPageSize));
    renderRepositories(container, data);
    const section = Array.from(container.querySelectorAll('.cfg-section'))
        .find(candidate => candidate.dataset.repository === repoKey);
    if (!section) return;
    section.classList.add('cfg-section--entering');
    section.addEventListener('animationend', () => {
        section.classList.remove('cfg-section--entering');
    }, {once: true});
    setTimeout(() => section.classList.remove('cfg-section--entering'), 450);
}

/**
 * Build localized options for the creation-only repository format selector.
 * @returns {Array<{value: string, label: string}>} Repository format options.
 */
function repositoryFormatOptions() {
    const options = [];
    for (const format of listRepositoryFormats()) {
        options.push({value: format.id, label: t(format.labelKey)});
    }
    return options;
}

/**
 * Move initial focus into the repository-name field.
 * @param {HTMLInputElement} input - Repository name input.
 * @returns {void}
 */
function focusRepositoryName(input) {
    input.focus();
}

/**
 * Open the repository creation dialog. Format is chosen before the first API
 * request and is immutable after creation.
 * @returns {void}
 */
function openRepositoryCreateDialog() {
    const nameInput = el('input', {
        id: 'repository-create-name',
        type: 'text',
        autocomplete: 'off',
        maxlength: '64',
        placeholder: t('repos.namePlaceholder'),
        required: true
    });
    let selectedFormat = 'maven';

    /**
     * Store the creation-only repository format selection.
     * @param {string} value - Selected format identifier.
     * @returns {void}
     */
    function handleCreateFormatChange(value) {
        selectedFormat = getRepositoryFormat(value).id;
    }

    const formatSelect = makeCustomSelect(
        repositoryFormatOptions(),
        selectedFormat,
        handleCreateFormatChange
    );
    const body = el('div', {class: 'cfg-fields repository-create-fields'},
        makeFieldRow(t('repos.name'), t('repos.nameHint'), nameInput),
        makeFieldRow(t('repos.format'), t('repos.formatCreateDesc'), formatSelect)
    );

    /**
     * Validate and persist a new repository from the creation dialog.
     * @param {SubmitEvent} event - Form submission event.
     * @param {{close: (result?: unknown) => void}} dialog - Active dialog controller.
     * @returns {Promise<void>}
     */
    async function submitRepositoryCreation(event, dialog) {
        event.preventDefault();
        const repoName = nameInput.value.trim();
        if (!isValidRepositorySlug(repoName)) {
            showAlert(t('repos.invalidRepoName'), 'error');
            return;
        }
        if (!currentConfig) currentConfig = {repositories: {}};
        if (!currentConfig.repositories) currentConfig.repositories = {};
        let repositoryExists = false;
        for (const configuredName of Object.keys(currentConfig.repositories)) {
            if (configuredName.toLowerCase() === repoName.toLowerCase()) {
                repositoryExists = true;
                break;
            }
        }
        if (repositoryExists) {
            showAlert(t('repos.repoExists'), 'error');
            return;
        }
        const repo = createRepositoryDraft(repoName, selectedFormat);
        currentConfig.repositories[repoName] = repo;
        const ok = await saveRepoSettings(repoName, repo, {silent: true, isCreate: true});
        if (!ok) {
            delete currentConfig.repositories[repoName];
            return;
        }
        dialog.close(true);
        showAlert(t('repos.createdSuccess', {name: repoName}), 'success');
        const container = document.getElementById('repositories-container');
        animateAddRepoSection(container, currentConfig, repoName);
    }

    /**
     * Close the repository creation dialog without persisting a draft.
     * @param {MouseEvent} event - Button click event.
     * @param {{close: (result?: unknown) => void}} dialog - Active dialog controller.
     * @returns {void}
     */
    function cancelRepositoryCreation(event, dialog) {
        event.preventDefault();
        dialog.close(false);
    }

    RenopDialog.show({
        id: 'repository-create-dialog',
        maxWidth: '560px',
        icon: 'box',
        title: t('repos.createTitle'),
        subtitle: t('repos.createSubtitle'),
        form: {id: 'repository-create-form', onSubmit: submitRepositoryCreation},
        body,
        footer: [
            {text: t('common.create'), className: 'action-btn primary-btn', type: 'submit'},
            {text: t('common.cancel'), className: 'action-btn', onClick: cancelRepositoryCreation}
        ]
    });
    requestAnimationFrame(focusRepositoryName.bind(null, nameInput));
}

document.getElementById('btn-add-repository')?.addEventListener('click', openRepositoryCreateDialog);

/**
 * Persists a single repository via PUT. Skips the request when unchanged (unless creating).
 * Uses a per-repo sequence so stale overlapping saves do not clobber newer UI feedback.
 * @param {string} repoKey - Repository name/key.
 * @param {object} repo - Repository settings payload.
 * @param {{ silent?: boolean, isCreate?: boolean }} [options] - `silent` suppresses success alerts; `isCreate` skips the unchanged check.
 * @returns {Promise<boolean>} True on success or superseded save; false on failure.
 */
async function saveRepoSettings(repoKey, repo, options = {}) {
    const {silent = false, isCreate = false} = options;
    const seq = (saveSeqByRepo.get(repoKey) || 0) + 1;
    saveSeqByRepo.set(repoKey, seq);
    try {
        const orig = initialReposMap[repoKey];
        if (orig && !isCreate && JSON.stringify(orig) === JSON.stringify(repo)) {
            if (!silent && saveSeqByRepo.get(repoKey) === seq) {
                showAlert(t('repos.savedSuccess'), 'success');
            }
            return true;
        }

        const payload = JSON.parse(JSON.stringify(repo));
        const {response} = await putProto(
            `/api/settings/repositories/${encodeURIComponent(repoKey)}`,
            Repository,
            payload
        );
        if (saveSeqByRepo.get(repoKey) !== seq) {
            return true;
        }
        if (response.ok) {
            initialReposMap[repoKey] = payload;
            window.dispatchEvent(new CustomEvent('repositorySettingsChanged', {
                detail: {repository: repoKey, format: payload.format}
            }));
            if (!silent) {
                showAlert(t('repos.savedSuccess'), 'success');
            }
            return true;
        }
        const errText = await response.text();
        showAlert(errText || t('repos.saveFailed'), 'error');
        return false;
    } catch {
        if (saveSeqByRepo.get(repoKey) === seq) {
            showAlert(t('repos.saveFailed'), 'error');
        }
        return false;
    }
}

window.addEventListener('languageChanged', () => {
    if (currentConfig && document.getElementById('repositories-container')) {
        renderRepositories(document.getElementById('repositories-container'), currentConfig);
    }
});

