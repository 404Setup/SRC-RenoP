/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

import {el} from '@renop/ui/dom';
import {makeCustomSelect} from '@renop/ui/custom-select';
import {apiRequest} from '../api.js';
import {showAlert, showConfirm} from '../alert.js';
import {canUpdateRepo} from '../auth.js';
import {createButton, createIcon, createSkeleton, RenopDialog, runButtonAction} from '../components.js';
import {t} from '../i18n.js';
import {npmResponseError} from '../npm-errors.js';
import {getRepositoryFormat} from '../repository-formats.js';
import {copyWithFeedback} from './copy-feedback.js';
import {
    createRepositoryBackButton,
    createRepositoryFactsSection,
    createRepositoryMirrorBadge,
    ensureRepositoryView,
    formatRepositoryTimestamp,
    hideRepositoryView,
    replaceRepositoryView,
    setRepositoryViewBusy
} from './repository-view.js';
import {RepositoryUserSuggestions} from './user-suggestions.js';
import {formatBytes} from './utils.js';

const npmIcon = getRepositoryFormat('npm').icon;
const pageSize = 24;
let view = null;
let activeRepository = '';
let activeNavigate = null;
let loadSequence = 0;
let packageDetails = null;
let packageList = [];
let packageTotal = 0;
let packageOffset = 0;
let inviteLevel = 0;

/** Localized failure returned by the stable npm management error boundary. */
class NPMRequestError extends Error {}

/**
 * Return only localized npm request failures to user-facing surfaces.
 * @param {unknown} error - Caught request or runtime failure.
 * @param {string} fallbackKey - Safe fallback translation key.
 * @returns {string} Localized failure text.
 */
function npmErrorMessage(error, fallbackKey) {
    return error instanceof NPMRequestError ? error.message : t(fallbackKey);
}

/**
 * Build an npm management endpoint with an optional package query.
 * @param {string} suffix - Endpoint suffix.
 * @param {string} [packageName=''] - Optional package name.
 * @returns {string} Same-origin API URL.
 */
function npmAPI(suffix, packageName = '') {
    const base = `/api/npm/repositories/${encodeURIComponent(activeRepository)}/${suffix}`;
    return packageName ? `${base}?package=${encodeURIComponent(packageName)}` : base;
}

/**
 * Perform one npm management request and return its JSON payload.
 * @param {string} url - Same-origin URL.
 * @param {RequestInit} [options={}] - Fetch options.
 * @param {string} [fallbackKey='npm.operationFailed'] - Failure translation.
 * @returns {Promise<any>} Parsed response or null for an empty response.
 */
async function npmRequest(url, options = {}, fallbackKey = 'npm.operationFailed') {
    const response = await apiRequest(url, options, {logoutOnForbidden: false});
    if (!response.ok) throw new NPMRequestError(npmResponseError(response, fallbackKey));
    if (response.status === 204) return null;
    return response.json();
}

/** @returns {HTMLElement} Persistent npm repository container. */
function ensureNPMView() {
    view = ensureRepositoryView(view, {
        id: 'npm-repository-view', className: 'npm-repository-view', create: true,
        mountResolver: () => document.querySelector('.browser-column') ||
            document.querySelector('.file-list-container')?.parentElement || null
    });
    return view;
}

/** Hide and clear the npm view. */
export function hideNPMRepositoryView() {
    hideRepositoryView(view);
    npmUserSuggestions.detach();
}

/**
 * Search bounded account suggestions for the active npm package.
 * @param {string} query - Username prefix.
 * @returns {Promise<string[]>} Matching account names.
 */
async function searchNPMUsers(query) {
    if (!activeRepository || !packageDetails?.package?.name) return [];
    const response = await apiRequest(
        `${npmAPI('users/search', packageDetails.package.name)}&q=${encodeURIComponent(query)}`,
        {}, {logoutOnForbidden: false}
    );
    if (!response.ok) return [];
    const payload = await response.json();
    return Array.isArray(payload?.users) ? payload.users : [];
}

const npmUserSuggestions = new RepositoryUserSuggestions({
    id: 'npm-invite-suggestions', searchDelay: 140, closeDelay: 160,
    fetchUsers: searchNPMUsers,
    onError: error => console.error('Failed to search npm invitation users', error)
});

/**
 * Build a compact npm package status badge.
 * @param {string} label - Localized badge label.
 * @param {string} [className=''] - Optional visual state class.
 * @returns {HTMLElement} Badge element.
 */
function statusBadge(label, className = '') {
    return el('span', {class: `npm-status-badge${className ? ` ${className}` : ''}`}, label);
}

/**
 * Copy a client command with shared clipboard feedback.
 * @param {HTMLElement} button - Copy control.
 * @param {string} value - Command text.
 * @returns {Promise<void>}
 */
async function copyCommand(button, value) {
    try {
        await copyWithFeedback(button, value, {copiedLabel: t('details.copied')});
    } catch (error) {
        console.error('Failed to copy npm command', error);
        showAlert(t('npm.copyFailed'), 'error');
    }
}

/**
 * Build one horizontally scrollable client-command block.
 * @param {string} title - Localized command title.
 * @param {string} command - Command text.
 * @returns {HTMLElement} Command block.
 */
function commandBlock(title, command) {
    const button = createButton(t('details.copy'), {
        class: 'pill-btn pill-btn--soft pill-btn--sm', icon: 'copy', title: t('details.copy')
    });
    button.addEventListener('click', () => copyCommand(button, command));
    return el('div', {class: 'npm-command'},
        el('div', {class: 'npm-command-header'}, el('strong', {}, title), button),
        el('pre', {}, el('code', {}, command))
    );
}

/**
 * Build npm configuration and package command examples.
 * @param {string} [packageName=''] - Optional selected package.
 * @returns {HTMLElement} Commands section.
 */
function registryCommands(packageName = '') {
    const registry = `${window.location.origin}/${encodeURIComponent(activeRepository)}/`;
    const authPath = `${window.location.host}/${encodeURIComponent(activeRepository)}/`;
    const section = el('section', {class: 'npm-page-section'}, el('h3', {}, t('npm.commands')));
    section.appendChild(commandBlock(t('npm.configureRegistry'),
        `npm config set registry ${registry}\nnpm config set //${authPath}:_authToken <API_TOKEN>`));
    if (packageName) {
        section.appendChild(commandBlock(t('npm.installPackage'), `npm install ${packageName} --registry ${registry}`));
        section.appendChild(commandBlock(t('npm.publishPackage'), `npm publish --registry ${registry}`));
    }
    return section;
}

/** @returns {HTMLElement} npm repository overview header. */
function catalogHero() {
    const actions = el('div', {class: 'npm-page-actions'});
    if (canUpdateRepo(activeRepository)) {
        const create = createButton(t('npm.createPackage'), {
            class: 'pill-btn pill-btn--primary pill-btn--sm', icon: 'plus'
        });
        create.addEventListener('click', showCreatePackageDialog);
        actions.appendChild(create);
    }
    return el('section', {class: 'npm-page-hero'},
        el('div', {class: 'npm-hero-title'}, createIcon(npmIcon),
            el('div', {}, el('span', {class: 'npm-page-kicker'}, 'npm'),
                el('h2', {}, activeRepository))),
        el('p', {}, t('npm.repositoryDescription')),
        actions
    );
}

/**
 * Build a navigable npm package summary card.
 * @param {object} pkg - Package summary returned by the management API.
 * @returns {HTMLElement} Package card.
 */
function packageCard(pkg) {
    const badges = el('div', {class: 'npm-card-badges'});
    if (pkg.private) badges.appendChild(statusBadge(t('npm.private'), 'is-private'));
    if (pkg.mirrored) badges.appendChild(createRepositoryMirrorBadge(t('common.fromMirror')));
    if (pkg.archived) badges.appendChild(statusBadge(t('npm.archived'), 'is-archived'));
    const card = el('button', {class: 'npm-package-card', type: 'button'},
        el('div', {class: 'npm-package-card-icon'}, createIcon(npmIcon)),
        el('div', {class: 'npm-package-card-copy'},
            el('strong', {}, pkg.name || ''),
            el('p', {}, pkg.description || t('npm.noDescription')),
            el('span', {class: 'npm-package-card-meta'},
                pkg.latest_version ? `v${pkg.latest_version}` : t('npm.noVersions'))
        ),
        badges
    );
    card.addEventListener('click', () => activeNavigate?.(
        `/${encodeURIComponent(activeRepository)}/packages/${encodeURIComponent(pkg.name)}`
    ));
    return card;
}

/** @returns {HTMLElement[]} Current npm catalog sections. */
function renderCatalog() {
    const grid = el('div', {class: 'npm-package-grid'});
    for (const pkg of packageList) grid.appendChild(packageCard(pkg));
    const section = el('section', {class: 'npm-page-section'},
        el('div', {class: 'npm-section-heading'},
            el('div', {}, el('h3', {}, t('npm.packages')), el('p', {}, t('npm.packageCount', {count: packageTotal})))
        )
    );
    if (packageList.length === 0) {
        section.appendChild(el('div', {class: 'npm-empty'}, createIcon(npmIcon), el('p', {}, t('npm.noPackages'))));
    } else {
        section.appendChild(grid);
    }
    if (packageList.length < packageTotal) {
        const more = createButton(t('npm.loadMore'), {class: 'pill-btn pill-btn--soft pill-btn--sm'});
        more.addEventListener('click', () => runButtonAction(more, loadMorePackages));
        section.appendChild(el('div', {class: 'npm-load-more'}, more));
    }
    return [catalogHero(), registryCommands(), section];
}

/**
 * Load one bounded npm package page.
 * @param {number} sequence - Active route-load sequence.
 * @param {boolean} [append=false] - Append instead of replacing current rows.
 * @returns {Promise<void>}
 */
async function loadCatalog(sequence, append = false) {
    if (!append) {
        packageOffset = 0;
        packageList = [];
    }
    const payload = await npmRequest(
        `${npmAPI('packages')}?limit=${pageSize}&offset=${packageOffset}`, {}, 'npm.loadFailed'
    );
    if (sequence !== loadSequence) return;
    const packages = Array.isArray(payload?.packages) ? payload.packages : [];
    packageList = append ? packageList.concat(packages) : packages;
    packageTotal = Number(payload?.total || packageList.length);
    packageOffset = packageList.length;
}

/** @returns {Promise<void>} Completion of the next catalog-page render. */
async function loadMorePackages() {
    const sequence = loadSequence;
    setRepositoryViewBusy(view, true);
    try {
        await loadCatalog(sequence, true);
        if (sequence === loadSequence) await replaceRepositoryView(view, renderCatalog(), {duration: 260});
    } catch (error) {
        showAlert(npmErrorMessage(error, 'npm.loadFailed'), 'error');
    } finally {
        if (sequence === loadSequence) setRepositoryViewBusy(view, false);
    }
}

/** Open the package reservation dialog. */
function showCreatePackageDialog() {
    const name = el('input', {
        type: 'text', maxlength: '214', autocomplete: 'off', placeholder: t('npm.packageNamePlaceholder'), required: true
    });
    const privateInput = el('input', {type: 'checkbox'});
    const body = el('div', {class: 'npm-dialog-fields'},
        el('label', {}, el('span', {}, t('npm.packageName')), name),
        el('label', {class: 'npm-check-row'}, privateInput, el('span', {}, t('npm.privatePackage'))),
        el('p', {class: 'npm-dialog-hint'}, t('npm.privateRequiresScope'))
    );
    RenopDialog.show({
        id: 'npm-create-package-dialog', icon: npmIcon, title: t('npm.createPackage'),
        subtitle: t('npm.createPackageHint'),
        form: {
            id: 'npm-create-package-form',
            onSubmit: async (event, dialog) => {
                event.preventDefault();
                const submit = event.submitter;
                if (submit) submit.disabled = true;
                try {
                    const pkg = await npmRequest(npmAPI('packages'), {
                        method: 'POST', headers: {'Content-Type': 'application/json'},
                        body: JSON.stringify({name: name.value.trim(), private: privateInput.checked})
                    }, 'npm.createFailed');
                    dialog.close(true);
                    showAlert(t('npm.packageCreated'), 'success');
                    activeNavigate?.(`/${encodeURIComponent(activeRepository)}/packages/${encodeURIComponent(pkg.name)}`);
                } catch (error) {
                    showAlert(npmErrorMessage(error, 'npm.createFailed'), 'error');
                } finally {
                    if (submit?.isConnected) submit.disabled = false;
                }
            }
        },
        body,
        footer: [
            {text: t('common.create'), className: 'action-btn primary-btn', type: 'submit'},
            {text: t('common.cancel'), className: 'action-btn', onClick: (event, dialog) => dialog.close(false)}
        ]
    });
    requestAnimationFrame(() => name.focus());
}

/**
 * Build a canonical same-origin npm tarball URL.
 * @param {string} packageName - Canonical package name.
 * @param {string} version - Semantic version.
 * @returns {string} Encoded tarball URL.
 */
function tarballURL(packageName, version) {
    const baseName = packageName.includes('/') ? packageName.slice(packageName.lastIndexOf('/') + 1) : packageName;
    return `/${encodeURIComponent(activeRepository)}/${packageName.split('/').map(encodeURIComponent).join('/')}/-/${encodeURIComponent(`${baseName}-${version}.tgz`)}`;
}

/**
 * Build the selected package header and authorized lifecycle actions.
 * @param {object} pkg - Selected package metadata.
 * @returns {HTMLElement} Package header.
 */
function packageHero(pkg) {
    const badges = el('div', {class: 'npm-card-badges'});
    if (pkg.private) badges.appendChild(statusBadge(t('npm.private'), 'is-private'));
    if (pkg.archived) badges.appendChild(statusBadge(t('npm.archived'), 'is-archived'));
    if (pkg.mirrored) badges.appendChild(createRepositoryMirrorBadge(t('common.fromMirror')));
    const actions = el('div', {class: 'npm-page-actions'});
    const canLifecycle = packageDetails.administrator || Number(pkg.permission_level) >= 2;
    const canOwn = packageDetails.administrator || Number(pkg.permission_level) >= 4;
    if (canLifecycle && !pkg.mirrored) {
        const edit = createButton(t('npm.editDescription'), {class: 'pill-btn pill-btn--soft pill-btn--sm', icon: 'edit'});
        edit.addEventListener('click', showDescriptionDialog);
        actions.appendChild(edit);
    }
    if (canOwn && !pkg.mirrored) {
        if (pkg.name.startsWith('@')) {
            const visibility = createButton(pkg.private ? t('npm.makePublic') : t('npm.makePrivate'), {
                class: 'pill-btn pill-btn--soft pill-btn--sm', icon: 'eye'
            });
            visibility.addEventListener('click', () => mutatePackage({private: !pkg.private}, 'npm.visibilityUpdated'));
            actions.appendChild(visibility);
        }
        const archive = createButton(pkg.archived ? t('npm.restorePackage') : t('npm.archivePackage'), {
            class: 'pill-btn pill-btn--soft pill-btn--sm'
        });
        archive.addEventListener('click', () => mutatePackage({archived: !pkg.archived},
            pkg.archived ? 'npm.packageRestored' : 'npm.packageArchived'));
        actions.appendChild(archive);
        const remove = createButton(t('npm.deletePackage'), {class: 'pill-btn pill-btn--danger pill-btn--sm', icon: 'delete'});
        remove.addEventListener('click', deletePackage);
        actions.appendChild(remove);
    }
    return el('section', {class: 'npm-page-hero'},
        createRepositoryBackButton({
            path: `/${encodeURIComponent(activeRepository)}`, label: t('npm.backToPackages'),
            navigate: activeNavigate, className: 'npm-page-back'
        }),
        el('div', {class: 'npm-hero-title'}, createIcon(npmIcon),
            el('div', {}, el('span', {class: 'npm-page-kicker'}, t('npm.packageKicker')), el('h2', {}, pkg.name))),
        badges,
        el('p', {}, pkg.description || t('npm.noDescription')),
        actions
    );
}

/** @returns {HTMLElement} Immutable npm version list. */
function versionsSection() {
    const section = el('section', {class: 'npm-page-section'}, el('h3', {}, t('npm.versions')));
    const versions = Array.isArray(packageDetails.versions) ? packageDetails.versions : [];
    const list = el('div', {class: 'npm-version-list'});
    const canDelete = packageDetails.administrator || Number(packageDetails.package.permission_level) >= 2;
    for (const version of versions) {
        const row = el('article', {class: `npm-version${version.unpublished ? ' is-unpublished' : ''}`},
            el('div', {}, el('strong', {}, `v${version.version}`),
                el('span', {}, formatRepositoryTimestamp(version.created_at))),
            el('div', {class: 'npm-version-meta'},
                version.publisher ? el('span', {}, t('npm.publishedBy', {name: version.publisher})) : null,
                version.size > 0 ? el('span', {}, formatBytes(Number(version.size))) : null,
                version.deprecated ? statusBadge(t('npm.deprecated'), 'is-deprecated') : null,
                version.unpublished ? statusBadge(t('npm.unpublished'), 'is-archived') : null
            )
        );
        const actions = el('div', {class: 'npm-version-actions'});
        if (!version.unpublished) {
            actions.appendChild(el('a', {
                class: 'pill-btn pill-btn--soft pill-btn--sm', href: tarballURL(packageDetails.package.name, version.version),
                download: `${packageDetails.package.name.replace('/', '-')}-${version.version}.tgz`
            }, createIcon('download'), el('span', {}, t('npm.downloadTarball'))));
            if (canDelete && !version.mirrored) {
                const remove = createButton(t('npm.deleteVersion'), {class: 'pill-btn pill-btn--danger pill-btn--sm', icon: 'delete'});
                remove.addEventListener('click', () => deleteVersion(version.version));
                actions.appendChild(remove);
            }
        }
        row.appendChild(actions);
        list.appendChild(row);
    }
    section.appendChild(versions.length ? list : el('div', {class: 'npm-empty'}, el('p', {}, t('npm.noVersions'))));
    return section;
}

/** @returns {HTMLElement} Sorted distribution-tag summary. */
function distTagsSection() {
    const section = el('section', {class: 'npm-page-section'}, el('h3', {}, t('npm.distTags')));
    const tags = Object.entries(packageDetails.dist_tags || {})
        .sort(([left], [right]) => left.localeCompare(right, undefined, {sensitivity: 'base'}));
    if (tags.length === 0) {
        section.appendChild(el('div', {class: 'npm-empty npm-empty--compact'}, el('p', {}, t('npm.noDistTags'))));
        return section;
    }
    section.appendChild(el('div', {class: 'npm-dist-tags'}, ...tags.map(([tag, version]) =>
        el('div', {class: 'npm-dist-tag'}, el('code', {}, tag), el('span', {}, version))
    )));
    return section;
}

/** @returns {HTMLElement|null} Authorized L0-L4 package-team controls. */
function teamSection() {
    const canManage = packageDetails.administrator || Number(packageDetails.package.permission_level) >= 3;
    if (!canManage || packageDetails.package.mirrored) return null;
    const section = el('section', {class: 'npm-page-section'}, el('h3', {}, t('npm.team')));
    const input = el('input', {
        type: 'text', maxlength: '255', autocomplete: 'off', placeholder: t('npm.invitePlaceholder')
    });
    npmUserSuggestions.attach(input);
    const level = makeCustomSelect([0, 1, 2, 3, 4].map(value => ({
        value: String(value), label: `L${value} — ${t(`npm.level${value}`)}`
    })), String(inviteLevel), value => { inviteLevel = Number(value); });
    const invite = createButton(t('npm.invite'), {class: 'pill-btn pill-btn--primary pill-btn--sm', icon: 'userPlus'});
    invite.addEventListener('click', () => runButtonAction(invite, async () => {
        const username = input.value.trim();
        if (!username) {
            showAlert(t('team.inviteUsernameRequired'), 'warning');
            input.focus();
            return;
        }
        await npmRequest(npmAPI('owners', packageDetails.package.name), {
            method: 'POST', headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({users: [username], level: inviteLevel})
        }, 'npm.inviteFailed');
        showAlert(t('npm.inviteSent', {name: username}), 'success');
        await refreshPackage();
    }));
    section.appendChild(el('div', {class: 'npm-invite'}, input, level, invite));
    const members = el('div', {class: 'npm-member-list'});
    for (const member of packageDetails.members || []) {
        const memberLevel = makeCustomSelect([0, 1, 2, 3, 4].map(value => ({
            value: String(value), label: `L${value}`
        })), String(member.level), async value => {
            try {
                await npmRequest(`${npmAPI(`owners/${encodeURIComponent(member.user_id || member.username)}`, packageDetails.package.name)}`, {
                    method: 'PUT', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({level: Number(value)})
                }, 'npm.teamUpdateFailed');
                await refreshPackage();
            } catch (error) {
                showAlert(npmErrorMessage(error, 'npm.teamUpdateFailed'), 'error');
                await refreshPackage();
            }
        });
        const remove = createButton('', {class: 'icon-btn', icon: 'delete', title: t('npm.removeMember')});
        remove.addEventListener('click', async () => {
            if (!await showConfirm(t('npm.removeMemberConfirm', {name: member.username}), {danger: true})) return;
            try {
                await npmRequest(npmAPI(`owners/${encodeURIComponent(member.user_id || member.username)}`, packageDetails.package.name),
                    {method: 'DELETE'}, 'npm.removeMemberFailed');
                await refreshPackage();
            } catch (error) {
                showAlert(npmErrorMessage(error, 'npm.removeMemberFailed'), 'error');
            }
        });
        members.appendChild(el('div', {class: 'npm-member'},
            el('strong', {}, member.username), el('div', {}, memberLevel, remove)));
    }
    section.appendChild(members);
    return section;
}

/** @returns {HTMLElement[]} Selected npm package sections. */
function renderPackage() {
    const pkg = packageDetails.package;
    const facts = createRepositoryFactsSection(t('npm.packageInformation'), [
        {label: t('npm.latestVersion'), value: pkg.latest_version || t('npm.noVersions'), code: true},
        {label: t('npm.versionCount'), value: pkg.version_count},
        {label: t('npm.publisher'), value: pkg.publisher || '-'},
        {label: t('npm.updated'), value: formatRepositoryTimestamp(pkg.updated_at)},
        {label: t('npm.permission'), value: packageDetails.administrator ? t('npm.administrator') :
            packageDetails.member ? `L${Number(pkg.permission_level || 0)}` : t('npm.publicAccess')}
    ], {className: 'npm-page-section'});
    return [packageHero(pkg), registryCommands(pkg.name), facts, distTagsSection(), versionsSection(), teamSection()]
        .filter(Boolean);
}

/** @returns {Promise<void>} Completion of a selected-package refresh. */
async function refreshPackage() {
    if (!packageDetails?.package?.name) return;
    const payload = await npmRequest(npmAPI('packages', packageDetails.package.name), {}, 'npm.loadFailed');
    packageDetails = payload;
    await replaceRepositoryView(view, renderPackage(), {duration: 280});
}

/**
 * Apply one package metadata or lifecycle change.
 * @param {object} change - Single-setting mutation payload.
 * @param {string} successKey - Success translation key.
 * @returns {Promise<boolean>} Whether the mutation and refresh succeeded.
 */
async function mutatePackage(change, successKey) {
    try {
        await npmRequest(npmAPI('packages', packageDetails.package.name), {
            method: 'PUT', headers: {'Content-Type': 'application/json'}, body: JSON.stringify(change)
        });
        showAlert(t(successKey), 'success');
        await refreshPackage();
        return true;
    } catch (error) {
        showAlert(npmErrorMessage(error, 'npm.operationFailed'), 'error');
        return false;
    }
}

/** Open the package-description editor. */
function showDescriptionDialog() {
    const textarea = el('textarea', {maxlength: '4000', rows: '5'}, packageDetails.package.description || '');
    RenopDialog.show({
        id: 'npm-description-dialog', icon: 'edit', title: t('npm.editDescription'),
        form: {id: 'npm-description-form', onSubmit: async (event, dialog) => {
            event.preventDefault();
            if (await mutatePackage({description: textarea.value}, 'npm.descriptionUpdated')) dialog.close(true);
        }},
        body: el('div', {class: 'npm-dialog-fields'}, el('label', {}, el('span', {}, t('npm.description')), textarea)),
        footer: [
            {text: t('npm.save'), className: 'action-btn primary-btn', type: 'submit'},
            {text: t('common.cancel'), className: 'action-btn', onClick: (event, dialog) => dialog.close(false)}
        ]
    });
}

/**
 * Confirm and unpublish one immutable version.
 * @param {string} version - Semantic version to tombstone.
 * @returns {Promise<void>}
 */
async function deleteVersion(version) {
    if (!await showConfirm(t('npm.deleteVersionConfirm', {version}), {danger: true})) return;
    try {
        await npmRequest(`${npmAPI('versions', packageDetails.package.name)}&version=${encodeURIComponent(version)}`,
            {method: 'DELETE'}, 'npm.deleteVersionFailed');
        showAlert(t('npm.versionDeleted'), 'success');
        await refreshPackage();
    } catch (error) {
        showAlert(npmErrorMessage(error, 'npm.deleteVersionFailed'), 'error');
    }
}

/** @returns {Promise<void>} Completion of a confirmed package deletion. */
async function deletePackage() {
    const name = packageDetails.package.name;
    if (!await showConfirm(t('npm.deletePackageConfirm', {name}), {danger: true})) return;
    try {
        await npmRequest(npmAPI('packages', name), {method: 'DELETE'}, 'npm.deletePackageFailed');
        showAlert(t('npm.packageDeleted'), 'success');
        activeNavigate?.(`/${encodeURIComponent(activeRepository)}`);
    } catch (error) {
        showAlert(npmErrorMessage(error, 'npm.deletePackageFailed'), 'error');
    }
}

/**
 * Load and animate the npm repository overview.
 * @param {number} sequence - Active route-load sequence.
 * @returns {Promise<void>}
 */
async function loadOverview(sequence) {
    packageDetails = null;
    setRepositoryViewBusy(view, true);
    if (!view.firstElementChild) view.replaceChildren(el('section', {class: 'npm-page-section'}, createSkeleton('list', 3)));
    try {
        await loadCatalog(sequence);
        if (sequence === loadSequence) await replaceRepositoryView(view, renderCatalog(), {duration: 300});
    } catch (error) {
        if (sequence === loadSequence) view.replaceChildren(el('section', {class: 'npm-page-section npm-error'},
            createIcon('alertCircle'), el('h2', {}, t('npm.pageUnavailable')),
            el('p', {}, npmErrorMessage(error, 'npm.loadFailed'))));
    } finally {
        if (sequence === loadSequence) setRepositoryViewBusy(view, false);
    }
}

/**
 * Load and animate one npm package page.
 * @param {string} packageName - Canonical package name.
 * @param {number} sequence - Active route-load sequence.
 * @returns {Promise<void>}
 */
async function loadPackage(packageName, sequence) {
    setRepositoryViewBusy(view, true);
    if (!view.firstElementChild) view.replaceChildren(el('section', {class: 'npm-page-section'}, createSkeleton('list', 3)));
    try {
        packageDetails = await npmRequest(npmAPI('packages', packageName), {}, 'npm.loadFailed');
        if (sequence === loadSequence) await replaceRepositoryView(view, renderPackage(), {duration: 300});
    } catch (error) {
        if (sequence === loadSequence) view.replaceChildren(el('section', {class: 'npm-page-section npm-error'},
            createIcon('alertCircle'), el('h2', {}, t('npm.pageUnavailable')),
            el('p', {}, npmErrorMessage(error, 'npm.loadFailed'))));
    } finally {
        if (sequence === loadSequence) setRepositoryViewBusy(view, false);
    }
}

/**
 * Render an npm repository root or package subpage.
 * @param {string} path - Application path.
 * @param {object} repoDetails - Repository metadata.
 * @param {(path: string) => void} navigate - SPA navigation callback.
 * @returns {Promise<void>}
 */
export async function renderNPMRepository(path, repoDetails, navigate) {
    const container = ensureNPMView();
    container.hidden = false;
    activeNavigate = navigate;
    const parts = String(path || '').split('/').filter(Boolean);
    activeRepository = decodeURIComponent(parts[0] || repoDetails?.name || '');
    const sequence = ++loadSequence;
    if (parts[1] === 'packages' && parts.length >= 3) {
        let packageName = parts.slice(2).map(part => {
            try { return decodeURIComponent(part); } catch { return part; }
        }).join('/');
        packageName = packageName.replace(/^%40/i, '@');
        await loadPackage(packageName, sequence);
        return;
    }
    await loadOverview(sequence);
}

window.addEventListener('languageChanged', () => {
    if (!view || view.hidden) return;
    void (packageDetails?.package ? replaceRepositoryView(view, renderPackage(), {duration: 220}) :
        replaceRepositoryView(view, renderCatalog(), {duration: 220}));
});
