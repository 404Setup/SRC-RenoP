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
import {apiRequest} from '../api.js';
import {canUpdateRepo} from '../auth.js';
import {showAlert, showConfirm} from '../alert.js';
import {createIcon, createMetaGrid, createSkeleton, createUserIdentity, RenopDialog, runButtonAction} from '../components.js';
import {dockerResponseError} from '../docker-errors.js';
import {t} from '../i18n.js';
import {setSafeMarkdown} from '../markdown.js';
import {getRepositoryFormat} from '../repository-formats.js';
import {copyWithFeedback} from './copy-feedback.js';
import {decodePathSegment, encodePathSegment, encodeRelativePath, formatBytes} from './utils.js';
import {resolveUserDisplayName} from '../user-profiles.js';
import {
    createRepositoryBackButton,
    createRepositoryMirrorBadge,
    ensureRepositoryView,
    formatRepositoryTimestamp,
    hideRepositoryView,
    replaceRepositoryView,
    setRepositoryViewBusy
} from './repository-view.js';
import {RepositoryUserSuggestions} from './user-suggestions.js';

const dockerRepositoryIcon = getRepositoryFormat('docker').icon;
let dockerViewContainer = null;
let activeRepository = '';
let activeNavigate = null;
let dockerLoadSequence = 0;

let inviteLevel = 1;

/**
 * Search users for the active Docker image invitation form.
 * @param {string} query - Username prefix.
 * @returns {Promise<string[]>} Bounded username results.
 */
async function searchDockerInvitationUsers(query) {
    if (!activeRepository) return [];
    const response = await apiRequest(`/api/docker/repositories/${encodeURIComponent(activeRepository)}/users/search?q=${encodeURIComponent(query)}`);
    if (!response.ok) return [];
    const data = await response.json();
    return Array.isArray(data?.users) ? data.users : [];
}

const dockerUserSuggestions = new RepositoryUserSuggestions({
    id: 'docker-invite-suggestions',
    searchDelay: 140,
    closeDelay: 160,
    fetchUsers: searchDockerInvitationUsers,
    onError: (error) => console.error('Failed to search Docker invitation users', error)
});

/**
 * Return or lazily create the persistent Docker view container.
 * @returns {HTMLElement} View container element.
 */
function ensureDockerContainer() {
    dockerViewContainer = ensureRepositoryView(dockerViewContainer, {
        id: 'docker-repository-view',
        className: 'docker-repository-view',
        create: true,
        mountResolver: () => document.querySelector('.browser-column') ||
            document.querySelector('.file-list-container')?.parentElement || null
    });
    return dockerViewContainer;
}

/**
 * Hide the Docker repository view when switching to another repository format.
 * @returns {void}
 */
export function hideDockerRepositoryView() {
    hideRepositoryView(dockerViewContainer);
    dockerUserSuggestions.detach();
}

/**
 * Trigger copy animation with toast feedback on a copy button or element.
 * @param {HTMLElement} element - Copy button or badge.
 * @param {string} text - Text to copy to clipboard.
 * @returns {Promise<void>}
 */
async function triggerDockerCopy(element, text) {
    if (!element || !text) return;
    try {
        await copyWithFeedback(element, text, {copiedLabel: t('details.copied')});
    } catch (err) {
        console.error('Failed to copy', err);
        showAlert(t('docker.copyFailed'), 'error');
    }
}

/**
 * Format a Docker timestamp into a readable localized date/time string.
 * @param {number|string} value - Timestamp value.
 * @returns {string} Formatted date string.
 */
function dockerDate(value) {
    return formatRepositoryTimestamp(value);
}

/**
 * Open the detailed manifest inspector modal dialog for a container image tag.
 * @param {string} repoName - Repository name.
 * @param {string} imageName - Image name.
 * @param {string} digest - Manifest digest.
 * @param {string} tag - Tag name.
 * @returns {Promise<void>}
 */
async function openManifestDetails(repoName, imageName, digest, tag) {
    const reference = digest || tag;
    try {
        const resp = await apiRequest(`/api/docker/repositories/${encodeURIComponent(repoName)}/manifests?image=${encodeURIComponent(imageName)}&ref=${encodeURIComponent(reference)}`);
        if (!resp.ok) {
            showAlert(t('docker.manifestLoadFailed'), 'error');
            return;
        }
        const manifest = await resp.json();
        const bodyNodes = [];

        const gridItems = [
            {label: t('docker.digest'), value: manifest.digest || '-', isCode: true},
            {label: t('docker.mediaType'), value: manifest.media_type || '-', isCode: true},
            {label: t('docker.configDigest'), value: manifest.config_digest || '-', isCode: true},
            {label: t('docker.size'), value: manifest.size > 0 ? formatBytes(manifest.size) : '-'},
            {label: t('docker.updated'), value: dockerDate(manifest.created_at) || '-'}
        ];
        if (manifest.publisher) {
            gridItems.push({label: t('docker.publisher'), value: createUserIdentity(manifest.publisher)});
        }
        bodyNodes.push(createMetaGrid(gridItems));

        if (manifest.raw_json) {
            const rawHeader = el('div', {style: {display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginTop: '0.75rem', marginBottom: '0.35rem'}},
                el('strong', {style: {fontSize: '0.85rem', color: 'var(--text-color)'}}, t('docker.rawManifest')),
                el('button', {
                    class: 'docker-pull-copy-btn',
                    type: 'button',
                    title: t('details.copy'),
                    onclick: (e) => triggerDockerCopy(e.currentTarget, manifest.raw_json)
                }, createIcon('copy', {class: 'icon-svg'}))
            );
            const pre = el('pre', {class: 'docker-raw-json-block'},
                el('code', {}, manifest.raw_json)
            );
            bodyNodes.push(rawHeader, pre);
        }

        const bodyWrap = el('div', {class: 'docker-manifest-dialog-body'}, ...bodyNodes);

        RenopDialog.show({
            title: t('docker.manifestDetails'),
            subtitle: `${imageName}:${tag || 'latest'}`,
            icon: dockerRepositoryIcon,
            maxWidth: '640px',
            body: bodyWrap,
            footer: [
                {
                    text: t('common.close'),
                    className: 'action-btn',
                    onClick: (e, dlg) => dlg.close()
                }
            ]
        });
    } catch (err) {
        console.error('Failed to load Docker manifest details', err);
        showAlert(t('docker.manifestLoadFailed'), 'error');
    }
}

/**
 * Open the README / Description markdown editor dialog for a container image.
 * @param {string} repoName - Repository name.
 * @param {string} imageName - Image name.
 * @param {string} currentDescription - Current markdown content.
 * @param {Function} onSaved - Callback on successful save.
 * @returns {void}
 */
function openReadmeEditor(repoName, imageName, currentDescription, onSaved) {
    const textarea = el('textarea', {
        placeholder: t('docker.readmePlaceholder')
    }, currentDescription || '');

    const editorWrap = el('div', {class: 'docker-readme-editor'}, textarea);

    RenopDialog.show({
        title: t('docker.editReadme'),
        subtitle: `${repoName}/${imageName}`,
        icon: 'fileText',
        maxWidth: '680px',
        body: editorWrap,
        footer: [
            {
                text: t('common.cancel'),
                className: 'action-btn',
                onClick: (e, dlg) => dlg.close()
            },
            {
                text: t('docker.saveReadme'),
                className: 'action-btn primary-btn',
                onClick: async (e, dlg) => {
                    const newDescription = textarea.value.trim();
                    try {
                        const resp = await apiRequest(`/api/docker/repositories/${encodeURIComponent(repoName)}/images?image=${encodeURIComponent(imageName)}`, {
                            method: 'PUT',
                            headers: {'Content-Type': 'application/json'},
                            body: JSON.stringify({description: newDescription})
                        });
                        if (resp.ok) {
                            dlg.close();
                            showAlert(t('docker.readmeSaved'), 'success');
                            if (typeof onSaved === 'function') onSaved(newDescription);
                        } else {
                            showAlert(dockerResponseError(resp, 'docker.updateReadmeFailed'), 'error');
                        }
                    } catch (err) {
                        console.error('Failed to update Docker image README', err);
                        showAlert(t('docker.updateReadmeFailed'), 'error');
                    }
                }
            }
        ]
    });
}

/**
 * Main render function called by browser router for Docker repositories.
 * @param {string} path - Browser route path.
 * @param {object|null} repoDetails - Repository metadata.
 * @param {Function} navigateToPath - Navigation callback.
 * @returns {Promise<void>}
 */
export async function renderDockerRepository(path, repoDetails, navigateToPath) {
    const seq = ++dockerLoadSequence;
    const container = ensureDockerContainer();
    container.hidden = false;

    activeNavigate = navigateToPath;
    const pathParts = path.split('/').filter(Boolean).map(decodePathSegment);
    const repoName = pathParts[0] || '';
    activeRepository = repoName;
    const imageName = pathParts.slice(1).join('/');

    if (!container.firstElementChild) {
        container.replaceChildren(
            el('div', {class: 'docker-page-hero'},
                createSkeleton({lines: 3})
            )
        );
    }

    if (!imageName) {
        await renderCatalogView(container, repoName, seq);
    } else {
        await renderImageDetailsView(container, repoName, imageName, seq);
    }
}

/**
 * Render the image catalog for a Docker repository.
 * @param {HTMLElement} container - View container.
 * @param {string} repoName - Repository name.
 * @param {number} seq - Sequence number.
 * @returns {Promise<void>}
 */
async function renderCatalogView(container, repoName, seq) {
    dockerUserSuggestions.detach();
    setRepositoryViewBusy(container, true);
    try {
        const response = await apiRequest(`/api/docker/repositories/${encodeURIComponent(repoName)}/images`);
        if (seq !== dockerLoadSequence) return;
        if (!response.ok) {
            await replaceRepositoryView(container,
                el('div', {class: 'docker-page-hero'},
                    el('p', {class: 'error-text'}, t('docker.loadFailed'))),
                {duration: 260, enter: false});
            return;
        }

        const data = await response.json();
        const images = data.images || [];
        const totalTags = images.reduce((acc, img) => acc + (img.tag_count || 0), 0);

        if (images.length === 0) {
            const hero = el('div', {class: 'docker-page-hero'},
                el('span', {class: 'docker-page-kicker'}, t('docker.kickerRegistry')),
                el('div', {class: 'docker-hero-header'},
                    el('h2', {class: 'docker-hero-title'},
                        createIcon(dockerRepositoryIcon, {class: 'icon-svg'}),
                        repoName
                    ),
                    createImageButton(repoName)
                ),
                el('p', {class: 'text-muted', style: {marginBottom: '1rem'}}, t('docker.noImages')),
                el('p', {class: 'docker-create-first-hint'}, t('docker.createFirstHint'))
            );

            await replaceRepositoryView(container, hero, {duration: 280, enterDuration: 440});
            return;
        }

        const hero = el('div', {class: 'docker-page-hero'},
            el('span', {class: 'docker-page-kicker'}, t('docker.kickerRegistry')),
            el('div', {class: 'docker-hero-header'},
                el('h2', {class: 'docker-hero-title'},
                    createIcon(dockerRepositoryIcon, {class: 'icon-svg'}),
                    el('span', {}, repoName)
                ),
                createImageButton(repoName)
            ),
            el('p', {class: 'text-muted'}, t('docker.imagesSubtitle')),
            el('div', {class: 'docker-hero-meta-row'},
                el('div', {class: 'docker-meta-chip'},
                    createIcon(dockerRepositoryIcon, {class: 'icon-svg'}),
                    el('span', {}, t('docker.totalImages', {count: images.length}))
                ),
                el('div', {class: 'docker-meta-chip'},
                    createIcon('fileCode', {class: 'icon-svg'}),
                    el('span', {}, t('docker.totalTags', {count: totalTags}))
                )
            )
        );

        const grid = el('div', {class: 'docker-image-grid'});
        for (const img of images) {
            const publisherMeta = el('span', {class: 'docker-card-publisher'},
                createIcon('user', {class: 'icon-svg'}),
                img.publisher ? createUserIdentity(img.publisher) : el('span', {}, t('docker.unspecifiedPublisher'))
            );

            const pullMeta = el('span', {class: 'docker-card-pulls'},
                createIcon('download', {class: 'icon-svg'}),
                el('span', {}, t('docker.pullCount', {count: img.pull_count || 0}))
            );

            const card = el('div', {
                class: 'docker-image-card',
                onclick: () => {
                    if (activeNavigate) {
                        activeNavigate(`/${encodePathSegment(repoName)}/${encodeRelativePath(img.image_name)}`);
                    }
                }
            },
                el('div', {},
                    el('div', {class: 'docker-image-name'},
                        createIcon(dockerRepositoryIcon, {class: 'icon-svg'}),
                        el('span', {}, img.image_name),
                        img.private ? el('span', {class: 'docker-private-badge'},
                            createIcon('ssl', {class: 'icon-svg'}), t('docker.private')) : null,
                        img.mirrored === true ? createRepositoryMirrorBadge(t('common.fromMirror')) : null
                    ),
                    img.description ? el('p', {class: 'docker-image-desc'}, img.description) : null
                ),
                el('div', {class: 'docker-image-meta'},
                    el('span', {class: 'docker-tag-badge'}, img.latest_tag
                        ? t('docker.latestTag', {tag: img.latest_tag})
                        : t('docker.tagCount', {count: img.tag_count || 0})),
                    publisherMeta,
                    pullMeta
                )
            );
            grid.appendChild(card);
        }

        const imagesSection = el('div', {class: 'docker-page-section'},
            el('h3', {style: {fontSize: '1rem', fontWeight: '650', marginBottom: '0.85rem', color: 'var(--text-color)'}}, t('docker.imagesTitle')),
            grid
        );

        await replaceRepositoryView(container, [hero, imagesSection], {duration: 280, enterDuration: 440});
    } catch (err) {
        if (seq !== dockerLoadSequence) return;
        console.error('Failed to load Docker image catalog', err);
        await replaceRepositoryView(container,
            el('div', {class: 'docker-page-hero'},
                el('p', {class: 'error-text'}, t('docker.loadFailed'))),
            {duration: 260, enter: false});
    }
}

/**
 * Format a Docker image permission with its explicit level.
 * @param {number|string} level - Permission level from L0 through L4.
 * @returns {string} Localized permission label.
 */
function dockerPermissionLabel(level) {
    const normalized = Math.max(0, Math.min(4, Number(level) || 0));
    return t(`docker.permissionL${normalized}`);
}

/**
 * Open the explicit Docker image creation dialog.
 * @param {string} repoName - Docker repository name.
 * @returns {void}
 */
function openCreateImageDialog(repoName) {
    const imageInput = el('input', {
        type: 'text', maxlength: '255', autocomplete: 'off', class: 'docker-create-image-input',
        placeholder: t('docker.imageNamePlaceholder')
    });
    const privateInput = el('input', {type: 'checkbox'});
    const privateOption = el('label', {class: 'docker-create-private-option'},
        privateInput,
        el('span', {class: 'docker-create-private-copy'},
            el('strong', {}, t('docker.privateImage')),
            el('span', {}, t('docker.privateImageHint'))
        )
    );
    RenopDialog.show({
        id: 'docker-image-create-dialog', maxWidth: '560px', icon: dockerRepositoryIcon,
        title: t('docker.createImage'), subtitle: t('docker.createImageSubtitle'),
        body: el('div', {class: 'docker-create-image-form'},
            el('label', {class: 'docker-create-image-field'},
                el('span', {}, t('docker.imageName')),
                imageInput,
                el('small', {}, t('docker.imageNameHint'))
            ),
            privateOption
        ),
        footer: [
            {text: t('common.cancel'), className: 'action-btn', onClick: (event, dialog) => dialog.close(false)},
            {
                text: t('common.create'), className: 'action-btn primary-btn',
                onClick: async (event, dialog) => {
                    const imageName = imageInput.value.trim().toLowerCase();
                    if (!imageName) {
                        imageInput.focus();
                        return;
                    }
                    const button = event.currentTarget;
                    await runButtonAction(button, async () => {
                        try {
                            const response = await apiRequest(`/api/docker/repositories/${encodeURIComponent(repoName)}/images`, {
                                method: 'POST', headers: {'Content-Type': 'application/json'},
                                body: JSON.stringify({image: imageName, private: privateInput.checked})
                            });
                            if (!response.ok) {
                                const key = response.status === 409 ? 'docker.imageAlreadyExists' :
                                    (response.status === 400 ? 'docker.invalidImageName' :
                                        (response.status === 503 ? 'docker.imageNameCheckFailed' : 'docker.createImageFailed'));
                                showAlert(t(key), 'error');
                                return;
                            }
                            const image = await response.json();
                            dialog.close(true);
                            showAlert(t('docker.imageCreated'), 'success');
                            activeNavigate?.(`/${encodePathSegment(repoName)}/${encodeRelativePath(image.image_name)}`);
                        } catch (error) {
                            console.error('Failed to create Docker image', error);
                            showAlert(t('docker.createImageFailed'), 'error');
                        }
                    });
                }
            }
        ]
    });
    requestAnimationFrame(() => imageInput.focus());
}

/**
 * Build the repository-scoped image creation button when permitted.
 * @param {string} repoName - Docker repository name.
 * @returns {HTMLButtonElement|null} Creation action or null.
 */
function createImageButton(repoName) {
    if (!canUpdateRepo(repoName)) return null;
    return el('button', {
        type: 'button', class: 'pill-btn pill-btn--primary', onclick: () => openCreateImageDialog(repoName)
    }, createIcon('plus', {class: 'icon-svg'}), el('span', {}, t('docker.createImage')));
}

/**
 * Persist a Docker team permission change and refresh the animated member list.
 * @param {object} options - Team update context.
 * @param {HTMLElement} options.container - Active Docker view.
 * @param {string} options.repoName - Repository name.
 * @param {string} options.imageName - Container image name.
 * @param {number} options.sequence - Active route sequence.
 * @param {object} options.member - Member being updated.
 * @param {number} options.permissionLevel - Current user's image permission.
 * @param {number} options.newLevel - Requested permission.
 * @param {HTMLElement} options.selector - Permission selector to restore on failure.
 * @returns {Promise<void>}
 */
async function updateDockerTeamMember({
    container, repoName, imageName, sequence, member, permissionLevel, newLevel, selector
}) {
    const previousLevel = Number(member.level ?? 1);
    if (newLevel === previousLevel) return;
    const currentUsername = String(localStorage.getItem('username') || '').trim().toLowerCase();
    const transfersOwnership = newLevel === 4 && permissionLevel === 4 &&
        String(member.username || '').toLowerCase() !== currentUsername;
    if (transfersOwnership) {
        const displayName = await resolveUserDisplayName(member.username);
        const confirmed = await showConfirm(t('team.transferOwnershipConfirm', {name: displayName}), {
            title: t('team.transferOwnership'), confirmText: t('team.transferOwnership')
        });
        if (!confirmed) {
            if (typeof selector.setValue === 'function') selector.setValue(String(previousLevel));
            return;
        }
    }
    try {
        const memberReference = member.user_id || member.username;
        const response = await apiRequest(`/api/docker/repositories/${encodeURIComponent(repoName)}/owners/${encodeURIComponent(memberReference)}?image=${encodeURIComponent(imageName)}`, {
            method: 'PUT',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({level: newLevel})
        });
        if (!response.ok) {
            throw new Error(dockerResponseError(response, 'docker.updateMemberFailed'));
        }
        member.level = newLevel;
        showAlert(t('docker.memberUpdated'), 'success');
        await renderImageDetailsView(container, repoName, imageName, sequence);
    } catch (error) {
        if (typeof selector.setValue === 'function') selector.setValue(String(previousLevel));
        console.error('Failed to update Docker team permission', error);
        showAlert(t('docker.updateMemberFailed'), 'error');
    }
}

/**
 * Remove another Docker member or leave the image team after confirmation.
 * @param {object} options - Team removal context.
 * @param {HTMLElement} options.container - Active Docker view.
 * @param {string} options.repoName - Repository name.
 * @param {string} options.imageName - Container image name.
 * @param {number} options.sequence - Active route sequence.
 * @param {object} options.member - Member being removed.
 * @param {boolean} options.isSelf - Whether the current user is leaving.
 * @returns {Promise<void>}
 */
async function removeDockerTeamMember({container, repoName, imageName, sequence, member, isSelf}) {
    const displayName = isSelf ? '' : await resolveUserDisplayName(member.username);
    const confirmed = await showConfirm(
        isSelf ? t('team.leaveConfirm') : t('docker.removeMemberConfirm', {name: displayName}), {
        title: isSelf ? t('team.leave') : t('docker.removeMember'),
        confirmText: isSelf ? t('team.leave') : t('common.remove'),
        danger: true
    });
    if (!confirmed) return;
    const memberReference = member.user_id || member.username;
    const response = await apiRequest(`/api/docker/repositories/${encodeURIComponent(repoName)}/owners/${encodeURIComponent(memberReference)}?image=${encodeURIComponent(imageName)}`, {
        method: 'DELETE'
    });
    if (!response.ok) {
        showAlert(dockerResponseError(response, 'docker.removeMemberFailed'), 'error');
        return;
    }
    showAlert(isSelf ? t('team.left') : t('docker.memberRemoved'), 'success');
    if (isSelf) {
        if (activeNavigate) activeNavigate(`/${encodePathSegment(repoName)}`);
        return;
    }
    await renderImageDetailsView(container, repoName, imageName, sequence);
}

/**
 * Render details, tags, and README markdown for a specific Docker container image.
 * @param {HTMLElement} container - View container.
 * @param {string} repoName - Repository name.
 * @param {string} imageName - Image name.
 * @param {number} seq - Sequence number.
 * @returns {Promise<void>}
 */
async function renderImageDetailsView(container, repoName, imageName, seq) {
    const animateTeam = container.querySelector('.docker-team-list') !== null;
    dockerUserSuggestions.detach();
    setRepositoryViewBusy(container, true);
    try {
        const response = await apiRequest(`/api/docker/repositories/${encodeURIComponent(repoName)}/images?image=${encodeURIComponent(imageName)}`);
        if (seq !== dockerLoadSequence) return;
        if (!response.ok) {
            await replaceRepositoryView(container,
                el('div', {class: 'docker-page-hero'},
                    createRepositoryBackButton({
                        path: `/${encodePathSegment(repoName)}`,
                        label: t('docker.backToImages'),
                        navigate: activeNavigate,
                        className: 'docker-page-back',
                        iconClass: 'icon-svg'
                    }),
                    el('p', {class: 'error-text'}, t('docker.imageNotFound'))),
                {duration: 260, enter: false});
            return;
        }

        const details = await response.json();
        let image = details.image || {};
        const tags = details.tags || [];
        const members = details.members || [];
        const permissionLevel = Number(details.permission_level || 0);
        const isAdministrator = Boolean(details.administrator);
        const currentUsername = String(localStorage.getItem('username') || '').trim().toLowerCase();

        const canManageL2 = isAdministrator || permissionLevel >= 2;
        const canManageL3 = isAdministrator || permissionLevel >= 3;
        const canTransferOwnership = isAdministrator || permissionLevel === 4;
        const canPush = isAdministrator || permissionLevel >= 1;

        const latestTag = tags[0]?.tag || 'latest';
        const clientCommand = tags.length > 0
            ? `docker pull ${window.location.host}/${repoName}/${imageName}:${latestTag}`
            : (canPush ? `docker push ${window.location.host}/${repoName}/${imageName}:<tag>` : '');

        const backBtn = createRepositoryBackButton({
            path: `/${encodePathSegment(repoName)}`,
            label: t('docker.backToImages'),
            navigate: activeNavigate,
            className: 'docker-page-back',
            iconClass: 'icon-svg'
        });

        const topNav = el('div', {class: 'docker-hero-nav'},
            backBtn,
            el('span', {class: 'docker-page-kicker'}, t('docker.kickerImage'))
        );

        let deleteImgBtn = null;
        if (canManageL3) {
            deleteImgBtn = el('button', {
                class: 'docker-btn-danger',
                type: 'button',
                title: t('docker.deleteImage'),
                onclick: async () => {
                    const confirmed = await showConfirm(t('docker.deleteImageConfirm', {image: imageName}));
                    if (confirmed) {
                        try {
                            const delResp = await apiRequest(`/api/docker/repositories/${encodeURIComponent(repoName)}/images?image=${encodeURIComponent(imageName)}`, {
                                method: 'DELETE'
                            });
                            if (delResp.ok) {
                                showAlert(t('docker.imageDeleted'), 'success');
                                if (activeNavigate) activeNavigate(`/${encodePathSegment(repoName)}`);
                            } else {
                                showAlert(dockerResponseError(delResp, 'docker.deleteImageFailed'), 'error');
                            }
                        } catch (error) {
                            console.error('Failed to delete Docker image', error);
                            showAlert(t('docker.deleteImageFailed'), 'error');
                        }
                    }
                }
            }, createIcon('delete', {class: 'icon-svg'}), el('span', {}, t('docker.deleteImage')));
        }

        const metaRow = el('div', {class: 'docker-hero-meta-row'},
            el('div', {class: 'docker-meta-chip'},
                createIcon('folder', {class: 'icon-svg'}),
                el('span', {}, repoName)
            )
        );

        metaRow.appendChild(
            el('div', {class: 'docker-meta-chip'},
                createIcon('user', {class: 'icon-svg'}),
                el('span', {}, `${t('docker.publisher')}:`),
                image.publisher ? createUserIdentity(image.publisher) : el('span', {}, t('docker.unspecifiedPublisher'))
            )
        );

        if (image.private) {
            metaRow.appendChild(el('div', {class: 'docker-meta-chip is-private'},
                createIcon('ssl', {class: 'icon-svg'}), el('span', {}, t('docker.private'))));
        }
        if (image.mirrored === true) {
            metaRow.appendChild(createRepositoryMirrorBadge(t('common.fromMirror')));
        }

        metaRow.appendChild(
            el('div', {class: 'docker-meta-chip'},
                createIcon('fileCode', {class: 'icon-svg'}),
                el('span', {}, t('docker.latestTag', {tag: latestTag}))
            )
        );

        metaRow.appendChild(
            el('div', {class: 'docker-meta-chip'},
                createIcon(dockerRepositoryIcon, {class: 'icon-svg'}),
                el('span', {}, t('docker.totalTags', {count: tags.length}))
            )
        );

        metaRow.appendChild(
            el('div', {class: 'docker-meta-chip'},
                createIcon('download', {class: 'icon-svg'}),
                el('span', {}, t('docker.pullCount', {count: image.pull_count || 0}))
            )
        );

        if (details.total_size > 0) {
            metaRow.appendChild(
                el('div', {class: 'docker-meta-chip'},
                    createIcon('storage', {class: 'icon-svg'}),
                    el('span', {}, formatBytes(details.total_size))
                )
            );
        }
        if (image.created_at) {
            metaRow.appendChild(
                el('div', {class: 'docker-meta-chip'},
                    createIcon('clock', {class: 'icon-svg'}),
                    el('span', {}, `${t('docker.created')}: ${dockerDate(image.created_at)}`)
                )
            );
        }
        if (image.updated_at && image.updated_at !== image.created_at) {
            metaRow.appendChild(
                el('div', {class: 'docker-meta-chip'},
                    createIcon('clock', {class: 'icon-svg'}),
                    el('span', {}, `${t('docker.updated')}: ${dockerDate(image.updated_at)}`)
                )
            );
        }

        const pullBox = clientCommand
            ? el('div', {class: 'docker-pull-box'},
                el('span', {class: 'docker-pull-text'}, clientCommand),
                el('button', {
                    class: 'docker-pull-copy-btn',
                    type: 'button',
                    title: t(tags.length > 0 ? 'docker.copyPull' : 'docker.copyPush'),
                    onclick: (e) => triggerDockerCopy(e.currentTarget, clientCommand)
                }, createIcon('copy', {class: 'icon-svg'}))
            )
            : el('p', {class: 'docker-create-first-hint'}, t('docker.awaitingFirstPush'));

        const hero = el('div', {class: 'docker-page-hero'},
            topNav,
            el('div', {class: 'docker-hero-header'},
                el('h2', {class: 'docker-hero-title'},
                    createIcon(dockerRepositoryIcon, {class: 'icon-svg'}),
                    el('span', {}, imageName)
                ),
                deleteImgBtn
            ),
            metaRow,
            pullBox
        );

        const tagListEl = el('div', {class: 'docker-tag-list'});
        if (tags.length === 0) {
            tagListEl.appendChild(
                el('div', {class: 'docker-readme-empty'},
                    createIcon('fileCode', {class: 'icon-svg'}),
                    el('span', {}, t('docker.noTags'))
                )
            );
        } else {
            for (const tObj of tags) {
                const tagPullCmd = `docker pull ${window.location.host}/${repoName}/${imageName}:${tObj.tag}`;
                const shortDigest = tObj.digest ? tObj.digest.slice(0, 19) + '…' : '';
                const sizeStr = tObj.size > 0 ? formatBytes(tObj.size) : '';
                const timeStr = tObj.updated_at ? dockerDate(tObj.updated_at) : (tObj.created_at ? dockerDate(tObj.created_at) : '');
                const tagPublisher = tObj.publisher || image.publisher || '';

                const digestPill = shortDigest
                    ? el('button', {
                        type: 'button',
                        class: 'docker-tag-digest',
                        title: `${tObj.digest} (${t('docker.copyDigest')})`,
                        onclick: (e) => triggerDockerCopy(e.currentTarget, tObj.digest)
                    }, createIcon('fileHash', {class: 'icon-svg'}), el('span', {}, shortDigest))
                    : null;

                const publisherChip = el('span', {class: 'docker-tag-publisher'},
                    createIcon('user', {class: 'icon-svg'}),
                    tagPublisher ? createUserIdentity(tagPublisher) : el('span', {}, t('docker.unspecifiedPublisher'))
                );

                const timeChip = timeStr
                    ? el('span', {class: 'docker-tag-time'},
                        createIcon('clock', {class: 'icon-svg'}),
                        el('span', {}, timeStr)
                    )
                    : null;

                const actionsWrap = el('div', {class: 'docker-tag-actions'},
                    el('button', {
                        class: 'docker-action-btn',
                        type: 'button',
                        title: t('docker.copyPull'),
                        onclick: (e) => triggerDockerCopy(e.currentTarget, tagPullCmd)
                    }, createIcon('copy', {class: 'icon-svg'})),
                    el('button', {
                        class: 'docker-action-btn',
                        type: 'button',
                        title: t('docker.inspect'),
                        onclick: () => openManifestDetails(repoName, imageName, tObj.digest, tObj.tag)
                    }, createIcon('eye', {class: 'icon-svg'}))
                );

                if (canManageL2) {
                    actionsWrap.appendChild(
                        el('button', {
                            class: 'docker-action-btn docker-action-btn--delete',
                            type: 'button',
                            title: t('docker.deleteTag'),
                            onclick: async () => {
                                const confirmed = await showConfirm(t('docker.deleteTagConfirm', {tag: tObj.tag, image: imageName}));
                                if (confirmed) {
                                    try {
                                        const delResp = await apiRequest(`/api/docker/repositories/${encodeURIComponent(repoName)}/tags?image=${encodeURIComponent(imageName)}&tag=${encodeURIComponent(tObj.tag)}`, {
                                            method: 'DELETE'
                                        });
                                        if (delResp.ok) {
                                            showAlert(t('docker.tagDeleted'), 'success');
                                            renderImageDetailsView(container, repoName, imageName, seq);
                                        } else {
                                            showAlert(dockerResponseError(delResp, 'docker.deleteTagFailed'), 'error');
                                        }
                                    } catch (error) {
                                        console.error('Failed to delete Docker tag', error);
                                        showAlert(t('docker.deleteTagFailed'), 'error');
                                    }
                                }
                            }
                        }, createIcon('delete', {class: 'icon-svg'}))
                    );
                }

                const row = el('div', {class: 'docker-tag-row'},
                    el('div', {class: 'docker-tag-left'},
                        el('span', {class: 'docker-tag-badge'}, tObj.tag),
                        digestPill,
                        sizeStr ? el('span', {class: 'docker-tag-size'}, sizeStr) : null,
                        publisherChip,
                        timeChip
                    ),
                    actionsWrap
                );
                tagListEl.appendChild(row);
            }
        }

        const tagsSection = el('div', {class: 'docker-page-section'},
            el('div', {style: {display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '0.85rem'}},
                el('h3', {style: {fontSize: '1rem', fontWeight: '650', margin: '0', color: 'var(--text-color)'}}, t('docker.tagsTitle')),
                el('span', {class: 'docker-tag-badge'}, `${tags.length}`)
            ),
            tagListEl
        );

        // README / Markdown Section
        const readmeContent = el('div');
        const updateReadmeView = (descText) => {
            if (descText && descText.trim().length > 0) {
                readmeContent.className = 'docker-readme-body';
                setSafeMarkdown(readmeContent, descText.trim());
            } else {
                readmeContent.className = 'docker-readme-empty';
                readmeContent.replaceChildren(
                    createIcon('fileText', {class: 'icon-svg'}),
                    el('span', {}, t('docker.noReadme'))
                );
            }
        };
        updateReadmeView(image.description);

        let editReadmeBtn = null;
        if (canManageL2) {
            editReadmeBtn = el('button', {
                class: 'docker-btn-secondary',
                type: 'button',
                title: t('docker.editReadme'),
                onclick: () => {
                    openReadmeEditor(repoName, imageName, image.description || '', (newDesc) => {
                        image.description = newDesc;
                        updateReadmeView(newDesc);
                    });
                }
            }, createIcon('edit', {class: 'icon-svg'}), el('span', {}, t('docker.editReadme')));
        }

        const readmeSection = el('div', {class: 'docker-readme-card'},
            el('div', {class: 'docker-readme-header'},
                el('h3', {class: 'docker-readme-title'},
                    createIcon('fileText', {class: 'icon-svg'}),
                    t('docker.readme')
                ),
                editReadmeBtn
            ),
            readmeContent
        );

        // Team / Collaborators Section
        let teamSection = null;
        if (members.length > 0 || canManageL3) {
            const teamListEl = el('div', {class: `docker-team-list${animateTeam ? ' is-updated' : ''}`});

            for (let index = 0; index < members.length; index++) {
                const member = members[index];
                const memberLevel = Number(member.level ?? 1);
                const isSelf = String(member.username || '').toLowerCase() === currentUsername;
                const levelLabel = dockerPermissionLabel(memberLevel);

                const memberControls = el('div', {class: 'docker-team-controls'});

                if (canManageL3 && memberLevel < 4) {
                    const levelSelect = makeCustomSelect(
                        ([
                            {value: '0', label: dockerPermissionLabel(0)},
                            {value: '1', label: dockerPermissionLabel(1)},
                            {value: '2', label: dockerPermissionLabel(2)},
                            {value: '3', label: dockerPermissionLabel(3)}
                        ]).concat(canTransferOwnership
                            ? [{value: '4', label: dockerPermissionLabel(4)}]
                            : []),
                        String(memberLevel),
                        async (newVal) => {
                            const newLevel = parseInt(newVal, 10);
                            await updateDockerTeamMember({
                                container, repoName, imageName, sequence: seq, member,
                                permissionLevel, newLevel, selector: levelSelect
                            });
                        }
                    );
                    levelSelect.classList.add('docker-permission-select');
                    memberControls.appendChild(levelSelect);

                    const removeBtn = el('button', {
                        class: 'docker-action-btn docker-action-btn--delete',
                        type: 'button',
                        title: isSelf ? t('team.leave') : t('docker.removeMember'),
                        onclick: () => removeDockerTeamMember({
                            container, repoName, imageName, sequence: seq, member, isSelf
                        })
                    }, isSelf ? el('span', {}, t('team.leave')) : createIcon('delete', {class: 'icon-svg'}));
                    memberControls.appendChild(removeBtn);
                } else {
                    memberControls.appendChild(
                        el('span', {class: 'docker-permission-badge'}, levelLabel)
                    );
                    if (!canManageL3 && isSelf && memberLevel < 4) {
                        memberControls.appendChild(el('button', {
                            class: 'docker-action-btn docker-action-btn--delete',
                            type: 'button',
                            title: t('team.leave'),
                            onclick: () => removeDockerTeamMember({
                                container, repoName, imageName, sequence: seq, member, isSelf: true
                            })
                        }, el('span', {}, t('team.leave'))));
                    }
                }

                const memberRow = el('div', {class: 'docker-team-row'},
                    el('div', {class: 'docker-team-member'},
                        createUserIdentity(member.username, {avatar: true, userID: member.user_id}),
                        member.added_at ? el('span', {class: 'docker-team-time'}, dockerDate(member.added_at)) : null
                    ),
                    memberControls
                );
                if (animateTeam) memberRow.style.animationDelay = `${Math.min(index, 8) * 35}ms`;
                teamListEl.appendChild(memberRow);
            }

            let inviteForm = null;
            if (canManageL3) {
                const inviteInput = el('input', {
                    type: 'text',
                    class: 'docker-invite-input',
                    placeholder: t('docker.invitePlaceholder'),
                    autocomplete: 'off'
                });
                dockerUserSuggestions.attach(inviteInput);

                inviteLevel = 1;
                const inviteLevelSelect = makeCustomSelect(
                    ([
                        {value: '0', label: dockerPermissionLabel(0)},
                        {value: '1', label: dockerPermissionLabel(1)},
                        {value: '2', label: dockerPermissionLabel(2)},
                        {value: '3', label: dockerPermissionLabel(3)}
                    ]).concat(canTransferOwnership
                        ? [{value: '4', label: dockerPermissionLabel(4)}]
                        : []),
                    '1',
                    (val) => {
                        const parsed = Number.parseInt(val, 10);
                        inviteLevel = Number.isInteger(parsed) ? parsed : 1;
                    }
                );
                inviteLevelSelect.classList.add('docker-permission-select');

                const sendInviteBtn = el('button', {
                    type: 'submit',
                    class: 'docker-btn-secondary primary-btn',
                    title: t('docker.invite')
                }, createIcon('userPlus', {class: 'icon-svg'}), el('span', {}, t('docker.invite')));

                inviteForm = el('form', {
                    class: 'docker-invite-form',
                    onsubmit: async (e) => {
                        e.preventDefault();
                        const userToInvite = inviteInput.value.trim();
                        if (!userToInvite) {
                            showAlert(t('docker.inviteUsernameRequired'), 'warning');
                            return;
                        }
                        const currentUser = localStorage.getItem('username') || '';
                        if (currentUser && userToInvite.toLowerCase() === currentUser.toLowerCase()) {
                            showAlert(t('docker.cannotInviteSelf'), 'warning');
                            return;
                        }
                        sendInviteBtn.disabled = true;
                        try {
                            const inviteResp = await apiRequest(`/api/docker/repositories/${encodeURIComponent(repoName)}/owners?image=${encodeURIComponent(imageName)}`, {
                                method: 'POST',
                                headers: {'Content-Type': 'application/json'},
                                body: JSON.stringify({
                                    users: [userToInvite],
                                    level: inviteLevel
                                })
                            });
                            if (inviteResp.ok) {
                                dockerUserSuggestions.clear();
                                showAlert(t('docker.inviteSent', {name: userToInvite}), 'success');
                                renderImageDetailsView(container, repoName, imageName, seq);
                            } else {
                                showAlert(dockerResponseError(inviteResp, 'docker.sendInviteFailed', {name: userToInvite}), 'error');
                            }
                        } catch (err) {
                            console.error('Failed to invite Docker image member', err);
                            showAlert(t('docker.sendInviteFailed'), 'error');
                        } finally {
                            sendInviteBtn.disabled = false;
                        }
                    }
                }, inviteInput, inviteLevelSelect, sendInviteBtn);
            }

            teamSection = el('div', {class: 'docker-page-section'},
                el('div', {style: {display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '0.85rem'}},
                    el('h3', {style: {fontSize: '1rem', fontWeight: '650', margin: '0', color: 'var(--text-color)'}}, t('docker.teamTitle')),
                    el('span', {class: 'docker-tag-badge'}, `${members.length}`)
                ),
                teamListEl,
                inviteForm
            );
        }

        const sections = [hero, tagsSection, readmeSection];
        if (teamSection) {
            sections.push(teamSection);
        }

        await replaceRepositoryView(container, sections, {duration: 280, enterDuration: 440});
    } catch (err) {
        if (seq !== dockerLoadSequence) return;
        console.error('Failed to load Docker image details', err);
        await replaceRepositoryView(container,
            el('div', {class: 'docker-page-hero'},
                el('p', {class: 'error-text'}, t('docker.imageNotFound'))),
            {duration: 260, enter: false});
    }
}

