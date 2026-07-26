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
import {fetchProto, getAuthHeaders, putProto} from './api.js';
import {logout} from './auth.js';
import {MavenRepositoriesResponse, Repository} from './proto/index.js';
import {
    createButton,
    createCallout,
    createEmptyState,
    createIcon,
    createSkeleton,
    createSubHeader
} from './components.js';
import {
    el,
    makeCfgInput,
    makeCustomSelect,
    makeFieldRow,
    makeInlineNumber,
    makeInlineToggle,
    makeTagListInput,
    makeToggleRow,
    makeVisibilityBadge
} from './cfg-ui.js';

let currentConfig = null;
let initialReposMap = {};
const saveSeqByRepo = new Map();
const RESERVED_REPO_NAMES = new Set(['css', 'js', 'svg', 'api', 'javadocs', 'assets']);

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
 * Loads Maven repositories from the API and renders the repositories settings UI.
 * @returns {Promise<void>}
 */
export async function initRepositories() {
    renderRepositoriesSkeleton();
    try {
        const {response, data} = await fetchProto('/api/settings/maven/repositories', MavenRepositoriesResponse);
        if (response.ok && data) {
            const repos = data.repositories || {};
            currentConfig = {repositories: repos};
            initialReposMap = JSON.parse(JSON.stringify(repos));
            renderMavenSettings(document.getElementById('repositories-container'), currentConfig);
        } else if (response.status === 401 || response.status === 403) {
            logout('kicked');
        }
    } catch (e) {
        console.error('Failed to load repositories', e);
    }
}

/**
 * Renders all Maven repository sections (or empty state) into the container.
 * @param {HTMLElement} container - Repositories container element.
 * @param {{repositories?: Object.<string, object>}} data - Config holding a repositories map.
 * @returns {void}
 */
function renderMavenSettings(container, data) {
    if (!data.repositories) data.repositories = {};

    const originalHeight = container.offsetHeight;
    if (originalHeight > 0) {
        container.style.minHeight = `${originalHeight}px`;
    }

    try {
        const layout = el('div', {class: 'cfg-layout'});

        const keys = Object.keys(data.repositories);

        if (keys.length === 0) {
            layout.appendChild(createEmptyState(t('repos.noRepos')));
        }

        keys.forEach(repoKey => {
            const repo = data.repositories[repoKey];
            layout.appendChild(buildRepoSection(container, data, repoKey, repo));
        });

        container.innerHTML = '';
        container.classList.remove('is-content-entering');
        void container.offsetWidth;
        container.classList.add('is-content-entering');
        container.appendChild(layout);
    } finally {
        container.style.minHeight = '';
    }
}

/**
 * Builds a collapsible UI section for a single Maven repository (visibility, S3, mirrors).
 * @param {HTMLElement} container - Parent repositories container (used for remove animation).
 * @param {{repositories: Object.<string, object>}} data - Full repositories config.
 * @param {string} repoKey - Repository name/key.
 * @param {object} repo - Repository settings object (mutated by controls).
 * @returns {HTMLElement} The section element.
 */
function buildRepoSection(container, data, repoKey, repo) {
    const section = el('div', {class: 'cfg-section is-collapsed'});

    const header = el('div', {class: 'cfg-section-header'});

    const iconBox = el('div', {class: 'cfg-section-icon'}, createIcon('box'));

    const meta = el('div', {class: 'cfg-section-meta'});
    const titleRow = el('div', {class: 'cfg-section-title-row', style: {display: 'flex', alignItems: 'center', gap: '0.6rem', minWidth: '0', flexWrap: 'nowrap'}});
    titleRow.appendChild(el('p', {class: 'cfg-section-title'}, repoKey));
    titleRow.appendChild(makeVisibilityBadge(repo.visibility || 'PUBLIC'));
    meta.appendChild(titleRow);

    const mirrorCount = (repo.mirrors || []).length;
    meta.appendChild(el('p', {class: 'cfg-section-subtitle'},
        t('repos.mirrorCount', {count: mirrorCount})
    ));

    const deleteBtn = el('button', {
        class: 'pill-btn pill-btn--danger pill-btn--sm',
        type: 'button',
        title: t('repos.deleteRepoTitle', {name: repoKey})
    }, createIcon('delete'), el('span', {}, t('common.delete')));
    deleteBtn.addEventListener('click', async (e) => {
        e.stopPropagation();
        if (await window.showConfirm(t('repos.confirmDelete', {name: repoKey}))) {
            try {
                const headers = getAuthHeaders();
                const response = await fetch(`/api/settings/maven/repositories/${encodeURIComponent(repoKey)}`, {
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

    const visOptions = [
        {value: 'PUBLIC', label: t('repos.visibilityPublic')},
        {value: 'HIDDEN', label: t('repos.visibilityHidden')},
        {value: 'PRIVATE', label: t('repos.visibilityPrivate')}
    ];
    const visSelect = makeCustomSelect(
        visOptions,
        repo.visibility || 'PUBLIC',
        v => {
            repo.visibility = v;
            const badge = titleRow.querySelector('span');
            if (badge) {
                const newBadge = makeVisibilityBadge(v);
                titleRow.replaceChild(newBadge, badge);
            }
            saveRepoSettings(repoKey, repo);
        }
    );
    fields.appendChild(makeFieldRow(t('repos.visibility'), t('repos.visibilityDesc'), visSelect));

    fields.appendChild(makeToggleRow(
        t('repos.allowRedeploy'),
        t('repos.allowRedeployDesc'),
        repo.allow_redeployment === true,
        checked => {
            repo.allow_redeployment = checked;
            saveRepoSettings(repoKey, repo);
        }
    ));

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
        access_key_id: '', secret_access_key: '', force_path_style: true, redirect_downloads: false
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
            fieldsContainer.style.display = checked ? '' : 'none';
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

    fields.appendChild(makeFieldRow(t('repos.mirrorName'), t('repos.mirrorNameHint'),
        makeCfgInput(mirror.name || '', 'e.g. Maven Central', 'text', v => {
            mirror.name = v;
            updateMirrorLabel();
            saveRepoSettings(repoKey, repo);
        })
    ));

    fields.appendChild(makeFieldRow(t('repos.mirrorUrl'), t('repos.mirrorUrlHint'),
        makeCfgInput(mirror.url || '', 'https://repo1.maven.org/maven2/', 'text', v => {
            mirror.url = v;
            saveRepoSettings(repoKey, repo);
        })
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
        conflictWarningEl.style.display = (hasAllow && hasDeny) ? 'flex' : 'none';
    }

    const allowInput = makeTagListInput({
        items: mirror.allow_artifacts || [],
        type: 'allow',
        placeholder: t('repos.addRulePlaceholder'),
        emptyText: t('repos.emptyAllowList'),
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
        placeholder: t('repos.addRulePlaceholder'),
        emptyText: t('repos.emptyDenyList'),
        onChange: (newList) => {
            if (newList.length > 0) mirror.deny_artifacts = newList;
            else delete mirror.deny_artifacts;
            updateConflictWarning();
            saveRepoSettings(repoKey, repo);
        }
    });

    fields.appendChild(makeFieldRow(t('repos.mirrorAllowList'), t('repos.mirrorAllowListHint'), allowInput, 'cfg-field-row--top-align'));
    fields.appendChild(makeFieldRow(t('repos.mirrorDenyList'), t('repos.mirrorDenyListHint'), denyInput, 'cfg-field-row--top-align'));
    fields.appendChild(conflictWarningEl);

    let currentMethod = (mirror.authorization && mirror.authorization.method)
        ? String(mirror.authorization.method).toLowerCase()
        : 'none';
    if (currentMethod === 'username/password') currentMethod = 'basic';
    if (currentMethod === 'bearer') currentMethod = 'token';
    if (currentMethod !== 'none' && currentMethod !== 'basic' && currentMethod !== 'token') {
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

    const userInput = makeCfgInput(
        mirror.authorization ? mirror.authorization.login || '' : '',
        t('repos.username'), 'text',
        v => {
            if (mirror.authorization) {
                mirror.authorization.login = v;
                saveRepoSettings(repoKey, repo);
            }
        },
        {autocomplete: 'username'}
    );
    userInput.style.display = currentMethod === 'token' ? 'none' : '';

    const passInput = makeCfgInput(
        mirror.authorization ? mirror.authorization.password || '' : '',
        currentMethod === 'token' ? t('repos.tokenSecret') : t('repos.password'),
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
        {value: 'token', label: t('repos.authToken')}
    ];

    const authSelect = makeCustomSelect(authOptions, currentMethod, val => {
        if (val === 'none') {
            delete mirror.authorization;
            credsRow.style.display = 'none';
        } else {
            mirror.authorization = {
                method: val === 'token' ? 'bearer' : val,
                login: userInput.value,
                password: passInput.value
            };
            credsRow.style.display = 'flex';
            if (val === 'token') {
                userInput.style.display = 'none';
                passInput.placeholder = t('repos.tokenSecret');
            } else {
                userInput.style.display = '';
                userInput.placeholder = t('repos.username');
                passInput.placeholder = t('repos.password');
            }
        }
        saveRepoSettings(repoKey, repo);
    });

    fields.appendChild(makeFieldRow(t('repos.authMethod'), t('repos.authMethodHint'), authSelect));

    credsRow.appendChild(makeFieldRow(t('repos.username'), null, userInput));
    credsRow.appendChild(makeFieldRow(`${t('repos.password')} / ${t('repos.tokenSecret')}`, null, passInput));
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
 * Returns the `.cfg-layout` element inside the repositories container, creating it if needed.
 * @param {HTMLElement|null} container - Repositories container element.
 * @returns {HTMLElement|null} Layout element, or null if container is missing.
 */
function getReposLayout(container) {
    if (!container) return null;
    let layout = container.querySelector('.cfg-layout');
    if (!layout) {
        layout = el('div', {class: 'cfg-layout'});
        container.innerHTML = '';
        container.appendChild(layout);
    }
    return layout;
}

/**
 * Animates removal of a repository section; shows empty state when no repos remain.
 * @param {HTMLElement|null} section - Section to animate out; falls back to full re-render if null.
 * @param {HTMLElement} container - Repositories container element.
 * @param {{repositories?: Object.<string, object>}} data - Current repositories config.
 * @returns {void}
 */
function animateRemoveRepoSection(section, container, data) {
    if (!section) {
        renderMavenSettings(container, data);
        return;
    }
    section.classList.add('cfg-section--leaving');
    section.style.pointerEvents = 'none';
    let settled = false;
    const finish = () => {
        if (settled) return;
        settled = true;
        if (section.parentNode) section.remove();
        if (!data.repositories || Object.keys(data.repositories).length === 0) {
            const layout = getReposLayout(container);
            if (layout && !layout.querySelector('.cfg-section')) {
                layout.innerHTML = '';
                layout.appendChild(createEmptyState(t('repos.noRepos')));
            }
        }
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
 * @param {object} repo - New repository settings object.
 * @returns {void}
 */
function animateAddRepoSection(container, data, repoKey, repo) {
    const layout = getReposLayout(container);
    if (!layout) {
        renderMavenSettings(container, data);
        return;
    }
    const empty = layout.querySelector('renop-empty-state, .renop-empty-state');
    if (empty) empty.remove();

    const section = buildRepoSection(container, data, repoKey, repo);
    section.classList.add('cfg-section--entering');
    layout.appendChild(section);
    section.addEventListener('animationend', () => {
        section.classList.remove('cfg-section--entering');
    }, {once: true});
    setTimeout(() => section.classList.remove('cfg-section--entering'), 450);
}

document.getElementById('btn-add-repository')?.addEventListener('click', async () => {
    let repoName = await window.showPrompt(t('repos.enterNewRepoName'));
    if (repoName) {
        repoName = repoName.trim();
        if (!repoName || repoName.includes('/')) {
            showAlert(t('repos.invalidRepoName'), 'error');
            return;
        }
        if (RESERVED_REPO_NAMES.has(repoName.toLowerCase())) {
            showAlert(t('repos.invalidRepoName'), 'error');
            return;
        }
        if (!currentConfig) currentConfig = {repositories: {}};
        if (!currentConfig.repositories) currentConfig.repositories = {};
        if (currentConfig.repositories[repoName]) {
            showAlert(t('repos.repoExists'), 'error');
            return;
        }
        const repo = {name: repoName, visibility: 'PUBLIC', allow_redeployment: false, mirrors: []};
        currentConfig.repositories[repoName] = repo;
        const ok = await saveRepoSettings(repoName, repo, {silent: true, isCreate: true});
        if (ok) {
            showAlert(t('repos.createdSuccess', {name: repoName}), 'success');
            const container = document.getElementById('repositories-container');
            animateAddRepoSection(container, currentConfig, repoName, repo);
        } else {
            delete currentConfig.repositories[repoName];
        }
    }
});

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
            `/api/settings/maven/repositories/${encodeURIComponent(repoKey)}`,
            Repository,
            payload
        );
        if (saveSeqByRepo.get(repoKey) !== seq) {
            return true;
        }
        if (response.ok) {
            initialReposMap[repoKey] = payload;
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
        renderMavenSettings(document.getElementById('repositories-container'), currentConfig);
    }
});

