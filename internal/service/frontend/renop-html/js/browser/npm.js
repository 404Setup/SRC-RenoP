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
import {
    createButton,
    createIcon,
    createSkeleton,
    createUserIdentity,
    RenopDialog,
    runButtonAction
} from '../components.js';
import {t} from '../i18n.js';
import {safeMarkdownURL, setSafeMarkdown} from '../markdown.js';
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
 * Format an npm package permission with its numeric level.
 * @param {number|string} level - L0-L4 permission level.
 * @returns {string} Localized permission label.
 */
function permissionLabel(level) {
    const numeric = Math.max(0, Math.min(4, Number(level) || 0));
    return `L${numeric} — ${t(`npm.level${numeric}`)}`;
}

/**
 * Return the scope portion of one canonical npm package name.
 * @param {string} packageName - Canonical package name.
 * @returns {string} Scope including `@`, or an empty string.
 */
function packageScope(packageName) {
    return String(packageName || '').startsWith('@') ? String(packageName).split('/', 1)[0] : '';
}

/**
 * Build a copyable integrity or digest row.
 * @param {string} label - Localized metadata label.
 * @param {string} value - Digest or integrity value.
 * @returns {HTMLElement|null} Metadata row when a value exists.
 */
function copyableVersionMetadata(label, value) {
    if (!value) return null;
    const button = createButton('', {
        class: 'npm-metadata-copy', icon: 'copy', title: t('details.copy')
    });
    button.setAttribute('aria-label', t('npm.copyMetadata', {label}));
    button.addEventListener('click', () => copyCommand(button, value));
    return el('div', {class: 'npm-version-digest'},
        el('span', {}, label), el('code', {title: value}, value), button
    );
}

/**
 * Set or clear the invitation field validation state.
 * @param {HTMLInputElement} input - Username input.
 * @param {HTMLElement} errorNode - Inline validation message.
 * @param {string} [message=''] - Localized error text.
 * @returns {void}
 */
function setInviteValidation(input, errorNode, message = '') {
    input.classList.toggle('is-invalid', Boolean(message));
    input.setAttribute('aria-invalid', message ? 'true' : 'false');
    errorNode.textContent = message;
    errorNode.hidden = !message;
}

/**
 * Build one safe external npm project link.
 * @param {string} label - Localized link label.
 * @param {unknown} value - Candidate absolute URL.
 * @param {string} icon - Canonical icon name.
 * @returns {HTMLAnchorElement|null} Safe link or null.
 */
function npmProjectLink(label, value, icon) {
    const href = safeMarkdownURL(value);
    if (!href) return null;
    return el('a', {
        class: 'npm-project-link', href, target: '_blank', rel: 'noopener noreferrer nofollow'
    }, createIcon(icon), el('span', {}, label));
}

/**
 * Render one published author or contributor identity.
 * @param {object|null|undefined} person - Bounded project person metadata.
 * @returns {HTMLElement|null} Person summary.
 */
function npmProjectPerson(person) {
    if (!person || (!person.name && !person.email && !person.url)) return null;
    const link = npmProjectLink(person.name || person.url, person.url, 'user');
    return el('span', {class: 'npm-project-person'},
        link || el('strong', {}, person.name || t('common.unknown')),
        person.email ? el('span', {}, person.email) : null
    );
}

/**
 * Build a compact list of published people.
 * @param {object[]} people - Bounded author-like records.
 * @returns {HTMLElement|null} People list.
 */
function npmProjectPeople(people) {
    const rendered = (Array.isArray(people) ? people : []).map(npmProjectPerson).filter(Boolean);
    return rendered.length ? el('div', {class: 'npm-project-people'}, ...rendered) : null;
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
    const versions = Array.isArray(packageDetails.versions) ? packageDetails.versions : [];
    const section = el('section', {class: 'npm-page-section'},
        el('div', {class: 'npm-section-heading'},
            el('div', {}, el('h3', {}, t('npm.versions')),
                el('p', {}, t('npm.versionCountLabel', {count: versions.length})))
        )
    );
    const list = el('div', {class: 'npm-version-list'});
    const canDelete = packageDetails.administrator || Number(packageDetails.package.permission_level) >= 2;
    const tagsByVersion = new Map();
    for (const [tag, target] of Object.entries(packageDetails.dist_tags || {})) {
        if (!tagsByVersion.has(target)) tagsByVersion.set(target, []);
        tagsByVersion.get(target).push(tag);
    }
    for (const version of versions) {
        const tagNames = (tagsByVersion.get(version.version) || [])
            .sort((left, right) => left.localeCompare(right, undefined, {sensitivity: 'base'}));
        const states = el('div', {class: 'npm-version-states'});
        for (const tag of tagNames) states.appendChild(statusBadge(tag, 'is-tag'));
        if (version.mirrored) states.appendChild(createRepositoryMirrorBadge(t('common.fromMirror')));
        if (version.deprecated) states.appendChild(statusBadge(t('npm.deprecated'), 'is-deprecated'));
        if (version.unpublished) states.appendChild(statusBadge(t('npm.unpublished'), 'is-archived'));

        const header = el('div', {class: 'npm-version-header'},
            el('div', {class: 'npm-version-title'}, el('strong', {}, `v${version.version}`), states),
            el('div', {class: 'npm-version-summary'},
                version.publisher ? createUserIdentity(version.publisher) : el('span', {}, t('common.unknown')),
                version.size > 0 ? el('span', {}, formatBytes(Number(version.size))) : null,
                el('time', {}, formatRepositoryTimestamp(version.created_at))
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
        const digestRows = [
            copyableVersionMetadata(t('npm.integrity'), version.integrity),
            copyableVersionMetadata(t('npm.shasum'), version.shasum)
        ].filter(Boolean);
        const details = el('details', {class: 'npm-version-details'},
            el('summary', {}, createIcon('info'), el('span', {}, t('npm.versionDetails'))),
            el('div', {class: 'npm-version-details-body'},
                digestRows.length ? el('div', {class: 'npm-version-digests'}, ...digestRows) :
                    el('p', {class: 'npm-version-no-integrity'}, t('npm.noIntegrity')),
                actions
            )
        );
        const row = el('article', {class: `npm-version${version.unpublished ? ' is-unpublished' : ''}`},
            header,
            version.deprecated ? el('p', {class: 'npm-deprecation-message'}, version.deprecated) : null,
            details
        );
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

/** @returns {HTMLElement} Package-team summary and authorized L0-L4 controls. */
function teamSection() {
    const canManage = packageDetails.administrator || Number(packageDetails.package.permission_level) >= 3;
    const canOwn = packageDetails.administrator || Number(packageDetails.package.permission_level) >= 4;
    const members = Array.isArray(packageDetails.members) ? packageDetails.members : [];
    const section = el('section', {class: 'npm-page-section npm-team-section'},
        el('div', {class: 'npm-section-heading'},
            el('div', {}, el('h3', {}, t('npm.team')), el('p', {}, t('npm.teamHint'))),
            statusBadge(t('npm.memberCount', {count: Number(packageDetails.member_count || members.length)}), 'is-count')
        )
    );
    if (packageDetails.package.mirrored) {
        section.appendChild(el('p', {class: 'npm-team-notice'}, t('npm.mirrorNoTeam')));
        return section;
    }
    if (!canManage) {
        section.appendChild(el('p', {class: 'npm-team-notice'}, t('npm.teamRestricted')));
        return section;
    }

    const memberList = el('div', {class: 'npm-member-list'});
    for (const member of members) {
        const numericLevel = Math.max(0, Math.min(4, Number(member.level) || 0));
        const controls = el('div', {class: 'npm-member-controls'});
        if (numericLevel === 4 && !canOwn) {
            controls.appendChild(statusBadge(permissionLabel(numericLevel), 'is-permission'));
        } else {
            const allowedLevels = [0, 1, 2, 3];
            if (canOwn) allowedLevels.push(4);
            const memberLevel = makeCustomSelect(allowedLevels.map(value => ({
                value: String(value), label: permissionLabel(value)
            })), String(numericLevel), async value => {
                try {
                    await npmRequest(npmAPI(`owners/${encodeURIComponent(member.user_id || member.username)}`,
                        packageDetails.package.name), {
                        method: 'PUT', headers: {'Content-Type': 'application/json'},
                        body: JSON.stringify({level: Number(value)})
                    }, 'npm.teamUpdateFailed');
                    showAlert(t('npm.memberUpdated', {name: member.username}), 'success');
                    await refreshPackage();
                } catch (error) {
                    showAlert(npmErrorMessage(error, 'npm.teamUpdateFailed'), 'error');
                    await refreshPackage();
                }
            });
            memberLevel.classList.add('npm-permission-select');
            controls.appendChild(memberLevel);
        }
        if (numericLevel < 4) {
            const remove = createButton(t('common.remove'), {
                class: 'npm-member-remove', icon: 'delete', title: t('npm.removeMember'),
                'aria-label': t('npm.removeMember')
            });
            remove.addEventListener('click', () => runButtonAction(remove, async () => {
                if (!await showConfirm(t('npm.removeMemberConfirm', {name: member.username}), {danger: true})) return;
                try {
                    await npmRequest(npmAPI(`owners/${encodeURIComponent(member.user_id || member.username)}`,
                        packageDetails.package.name), {method: 'DELETE'}, 'npm.removeMemberFailed');
                    showAlert(t('npm.memberRemoved', {name: member.username}), 'success');
                    await refreshPackage();
                } catch (error) {
                    showAlert(npmErrorMessage(error, 'npm.removeMemberFailed'), 'error');
                }
            }));
            controls.appendChild(remove);
        } else {
            controls.appendChild(el('span', {class: 'npm-owner-lock', title: t('npm.ownerProtected')},
                createIcon('fileLock'), el('span', {}, t('npm.owner'))));
        }
        memberList.appendChild(el('div', {class: 'npm-member'},
            el('div', {class: 'npm-member-identity'},
                createUserIdentity(member.username, {avatar: true}),
                member.added_at ? el('span', {}, t('npm.addedAt', {
                    date: formatRepositoryTimestamp(member.added_at)
                })) : null
            ),
            controls
        ));
    }
    section.appendChild(memberList);

    const input = el('input', {
        id: 'npm-invite-username', class: 'npm-invite-input', type: 'text', maxlength: '255',
        autocomplete: 'off', placeholder: t('npm.invitePlaceholder'),
        'aria-describedby': 'npm-invite-error', 'aria-invalid': 'false'
    });
    npmUserSuggestions.attach(input);
    const inputError = el('span', {
        id: 'npm-invite-error', class: 'npm-invite-error', role: 'alert', hidden: true
    });
    input.addEventListener('input', () => setInviteValidation(input, inputError));
    const level = makeCustomSelect([0, 1, 2, 3, 4].map(value => ({
        value: String(value), label: permissionLabel(value)
    })), String(inviteLevel), value => { inviteLevel = Number(value); });
    level.classList.add('npm-invite-permission-select');
    const invite = createButton(t('npm.invite'), {
        class: 'pill-btn pill-btn--primary npm-invite-submit', icon: 'userPlus', type: 'submit'
    });
    const form = el('form', {class: 'npm-invite-form', action: 'javascript:void(0);'},
        el('div', {class: 'npm-invite-heading'},
            el('strong', {}, t('npm.invite')),
            el('span', {}, t('npm.inviteMemberHint'))
        ),
        el('div', {class: 'npm-invite-controls'},
            el('div', {class: 'npm-invite-input-wrap'}, input, inputError), level, invite
        )
    );
    form.addEventListener('submit', async event => {
        event.preventDefault();
        const username = input.value.trim();
        if (!username) {
            const message = t('team.inviteUsernameRequired');
            setInviteValidation(input, inputError, message);
            showAlert(message, 'warning');
            input.focus();
            return;
        }
        if (!/^[^\s\0\r\n]{1,255}$/.test(username)) {
            const message = t('npm.invalidUsername');
            setInviteValidation(input, inputError, message);
            showAlert(message, 'warning');
            input.focus();
            return;
        }
        const currentUsername = String(localStorage.getItem('username') || '').toLowerCase();
        if (currentUsername && username.toLowerCase() === currentUsername) {
            const message = t('npm.cannotInviteSelf');
            setInviteValidation(input, inputError, message);
            showAlert(message, 'warning');
            return;
        }
        invite.disabled = true;
        try {
            await npmRequest(npmAPI('owners', packageDetails.package.name), {
                method: 'POST', headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({users: [username], level: inviteLevel})
            }, 'npm.inviteFailed');
            npmUserSuggestions.clear();
            setInviteValidation(input, inputError);
            showAlert(t('npm.inviteSent', {name: username}), 'success');
            await refreshPackage();
        } catch (error) {
            const message = npmErrorMessage(error, 'npm.inviteFailed');
            setInviteValidation(input, inputError, message);
            showAlert(message, 'error');
        } finally {
            invite.disabled = false;
        }
    });
    section.appendChild(form);
    return section;
}

/**
 * Build published author, links, runtime, license, funding, and keyword metadata.
 * @returns {HTMLElement|null} Project metadata section when any fields exist.
 */
function projectMetadataSection() {
    const project = packageDetails.project || {};
    const author = npmProjectPerson(project.author);
    const contributors = npmProjectPeople(project.contributors);
    const maintainers = npmProjectPeople(project.maintainers);
    const keywords = Array.isArray(project.keywords) && project.keywords.length
        ? el('div', {class: 'npm-keyword-list'}, ...project.keywords.map(keyword =>
            el('span', {class: 'npm-keyword'}, keyword)))
        : null;
    const links = [
        npmProjectLink(t('npm.homepage'), project.homepage, 'fileWeb'),
        npmProjectLink(t('npm.sourceRepository'), project.repository, 'fileCode'),
        npmProjectLink(t('npm.bugTracker'), project.bugs, 'alertCircle'),
        ...(Array.isArray(project.funding) ? project.funding.map((url, index) =>
            npmProjectLink(t('npm.fundingLink', {index: index + 1}), url, 'network')) : [])
    ].filter(Boolean);
    const facts = [
        {label: t('npm.author'), value: author, wide: true},
        {label: t('npm.contributors'), value: contributors, wide: true},
        {label: t('npm.maintainers'), value: maintainers, wide: true},
        {label: t('npm.license'), value: project.license},
        {label: t('npm.nodeEngine'), value: project.node_engine, code: true},
        {label: t('npm.packageManager'), value: project.package_manager, code: true},
        {label: t('npm.keywords'), value: keywords, wide: true}
    ];
    if (!facts.some(fact => fact.value) && links.length === 0) return null;
    const section = createRepositoryFactsSection(t('npm.projectInformation'), facts, {
        className: 'npm-page-section npm-project-section'
    });
    if (links.length) section.appendChild(el('div', {class: 'npm-project-links'}, ...links));
    return section;
}

/** @returns {HTMLElement} Safely rendered published package README section. */
function readmeSection() {
    const project = packageDetails.project || {};
    const title = el('div', {class: 'npm-section-heading'},
        el('div', {}, el('h3', {}, t('npm.readme')),
            project.readme_filename ? el('p', {}, project.readme_filename) : null)
    );
    const section = el('section', {class: 'npm-page-section npm-readme-section'}, title);
    if (!project.readme) {
        section.appendChild(el('div', {class: 'npm-empty npm-empty--compact'},
            createIcon('fileMarkdown'), el('p', {}, t('npm.noReadme'))));
        return section;
    }
    const content = el('article', {class: 'npm-readme-body repository-markdown'});
    setSafeMarkdown(content, project.readme);
    section.appendChild(content);
    return section;
}

/** @returns {HTMLElement[]} Selected npm package sections. */
function renderPackage() {
    const pkg = packageDetails.package;
    const scope = packageScope(pkg.name);
    const source = pkg.mirrored ? t('npm.mirrorOrigin') : t('npm.localOrigin');
    const facts = createRepositoryFactsSection(t('npm.packageInformation'), [
        {label: t('npm.packageName'), value: pkg.name, code: true, wide: true},
        {label: t('npm.repository'), value: pkg.repository, code: true},
        {label: t('npm.scope'), value: scope || t('npm.unscoped'), code: Boolean(scope)},
        {label: t('npm.latestVersion'), value: pkg.latest_version || t('npm.noVersions'), code: true},
        {label: t('npm.versionCount'), value: pkg.version_count},
        {label: t('npm.distTagCount'), value: Object.keys(packageDetails.dist_tags || {}).length},
        {label: t('npm.memberCountLabel'), value: Number(packageDetails.member_count || 0)},
        {label: t('npm.publisher'), value: pkg.publisher ? createUserIdentity(pkg.publisher) : t('common.unknown')},
        {label: t('npm.visibility'), value: pkg.private ? t('npm.private') : t('npm.public')},
        {label: t('npm.origin'), value: source},
        {label: t('npm.publishStatus'), value: pkg.publish_enabled ? t('npm.publishEnabled') : t('npm.publishDisabled')},
        {label: t('npm.created'), value: formatRepositoryTimestamp(pkg.created_at)},
        {label: t('npm.updated'), value: formatRepositoryTimestamp(pkg.updated_at)},
        {label: t('npm.permission'), value: packageDetails.administrator ? t('npm.administrator') :
            packageDetails.member ? permissionLabel(pkg.permission_level) : t('npm.publicAccess'), wide: true}
    ], {className: 'npm-page-section'});
    const information = el('div', {class: 'npm-information-grid'}, facts, distTagsSection());
    return [
        packageHero(pkg), registryCommands(pkg.name), information,
        projectMetadataSection(), readmeSection(), versionsSection(), teamSection()
    ].filter(Boolean);
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
