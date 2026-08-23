/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {marked} from 'marked';
import {el} from '@renop/ui/dom';
import {morphElementHeight} from '@renop/ui/height-anim';
import {makeCustomSelect} from '@renop/ui/custom-select';
import {apiRequest} from '../api.js';
import {showAlert, showConfirm} from '../alert.js';
import {createIcon, createMetaGrid, createSkeleton, RenopDialog} from '../components.js';
import {t} from '../i18n.js';
import {decodePathSegment, encodePathSegment, encodeRelativePath, formatBytes} from './utils.js';

let dockerViewContainer = null;
let activeRepository = '';
let activeNavigate = null;
let dockerLoadSequence = 0;

let inviteSuggestionPanel = null;
let inviteInput = null;
let inviteSuggestionTimer = 0;
let inviteCloseTimer = 0;
let inviteSuggestions = [];
let activeInviteSuggestion = -1;
let inviteSuggestionVersion = 0;
let inviteLevel = 1;

const INVITE_SEARCH_DELAY_MS = 140;
const INVITE_CLOSE_DELAY_MS = 160;

marked.setOptions({
    gfm: true,
    breaks: false
});

/**
 * Return or lazily create the persistent Docker view container.
 * @returns {HTMLElement} View container element.
 */
function ensureDockerContainer() {
    if (dockerViewContainer && dockerViewContainer.isConnected) {
        return dockerViewContainer;
    }
    let container = document.getElementById('docker-repository-view');
    if (!container) {
        container = el('section', {
            id: 'docker-repository-view',
            class: 'docker-repository-view',
            hidden: true
        });
        const browserColumn = document.querySelector('.browser-column') || document.querySelector('.file-list-container')?.parentElement;
        if (browserColumn) {
            browserColumn.appendChild(container);
        } else {
            document.body.appendChild(container);
        }
    }
    dockerViewContainer = container;
    return container;
}

/**
 * Hide the Docker repository view when switching to another repository format.
 * @returns {void}
 */
export function hideDockerRepositoryView() {
    if (dockerViewContainer) {
        dockerViewContainer.hidden = true;
        dockerViewContainer.classList.remove('is-updating', 'is-entering');
        dockerViewContainer.replaceChildren();
    }
    closeInviteSuggestions(true);
}

/**
 * Trigger copy animation with toast feedback on a copy button or element.
 * @param {HTMLElement} element - Copy button or badge.
 * @param {string} text - Text to copy to clipboard.
 * @returns {void}
 */
function triggerDockerCopy(element, text) {
    if (!element || !text) return;
    navigator.clipboard.writeText(text).then(() => {
        element.classList.add('copied');
        const toast = el('span', {class: 'copy-toast'}, t('details.copied') || 'Copied!');
        element.appendChild(toast);
        setTimeout(() => {
            if (element.isConnected) {
                element.classList.remove('copied');
                toast.remove();
            }
        }, 1600);
    }).catch((err) => {
        console.error('Failed to copy', err);
    });
}

/**
 * Format a Docker timestamp into a readable localized date/time string.
 * @param {number|string} value - Timestamp value.
 * @returns {string} Formatted date string.
 */
function dockerDate(value) {
    if (!value) return '';
    const timestamp = typeof value === 'number' ? (value > 1e11 ? value : value * 1000) : Date.parse(value);
    return Number.isFinite(timestamp) && timestamp > 0
        ? new Date(timestamp).toLocaleString()
        : '';
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
            showAlert(t('docker.manifestLoadFailed') || 'Failed to load manifest details.');
            return;
        }
        const manifest = await resp.json();
        const bodyNodes = [];

        const gridItems = [
            { label: t('docker.digest') || 'Digest', value: manifest.digest || '-', isCode: true },
            { label: t('docker.mediaType') || 'Media Type', value: manifest.media_type || '-', isCode: true },
            { label: t('docker.configDigest') || 'Config Digest', value: manifest.config_digest || '-', isCode: true },
            { label: t('docker.size') || 'Size', value: manifest.size > 0 ? formatBytes(manifest.size) : '-' },
            { label: t('docker.updated') || 'Updated', value: dockerDate(manifest.created_at) || '-' }
        ];
        if (manifest.publisher) {
            gridItems.push({ label: t('docker.publisher') || 'Publisher', value: manifest.publisher });
        }
        bodyNodes.push(createMetaGrid(gridItems));

        if (manifest.raw_json) {
            const rawHeader = el('div', {style: {display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginTop: '0.75rem', marginBottom: '0.35rem'}},
                el('strong', {style: {fontSize: '0.85rem', color: 'var(--text-color)'}}, t('docker.rawManifest') || 'Raw Manifest JSON'),
                el('button', {
                    class: 'docker-pull-copy-btn',
                    type: 'button',
                    title: t('details.copy') || 'Copy',
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
            title: t('docker.manifestDetails') || 'Manifest Details',
            subtitle: `${imageName}:${tag || 'latest'}`,
            icon: 'box',
            maxWidth: '640px',
            body: bodyWrap,
            footer: [
                {
                    text: t('common.close') || 'Close',
                    className: 'action-btn',
                    onClick: (e, dlg) => dlg.close()
                }
            ]
        });
    } catch (err) {
        showAlert(t('docker.manifestLoadFailed') || String(err.message || err));
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
        placeholder: t('docker.readmePlaceholder') || 'Write markdown description or documentation for this container image...'
    }, currentDescription || '');

    const editorWrap = el('div', {class: 'docker-readme-editor'}, textarea);

    RenopDialog.show({
        title: t('docker.editReadme') || 'Edit README',
        subtitle: `${repoName}/${imageName}`,
        icon: 'fileText',
        maxWidth: '680px',
        body: editorWrap,
        footer: [
            {
                text: t('common.cancel') || 'Cancel',
                className: 'action-btn',
                onClick: (e, dlg) => dlg.close()
            },
            {
                text: t('docker.saveReadme') || 'Save README',
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
                            showAlert(t('docker.readmeSaved') || 'README updated successfully.');
                            if (typeof onSaved === 'function') onSaved(newDescription);
                        } else {
                            showAlert(t('docker.updateReadmeFailed') || 'Failed to update README.');
                        }
                    } catch (err) {
                        showAlert(String(err.message || err));
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
    container.classList.add('is-updating');
    try {
        const response = await apiRequest(`/api/docker/repositories/${encodeURIComponent(repoName)}/images`);
        if (seq !== dockerLoadSequence) return;
        if (!response.ok) {
            await morphElementHeight(container, () => {
                container.replaceChildren(
                    el('div', {class: 'docker-page-hero'},
                        el('p', {class: 'error-text'}, t('docker.noImages') || 'Failed to load container images.')
                    )
                );
                container.classList.remove('is-updating');
            }, {duration: 260});
            return;
        }

        const data = await response.json();
        const images = data.images || [];
        const totalTags = images.reduce((acc, img) => acc + (img.tag_count || 0), 0);

        const host = window.location.host;
        const pushHint2 = `docker push ${host}/${repoName}/<image>:${t('docker.tag') || 'tag'}`;

        if (images.length === 0) {
            const hero = el('div', {class: 'docker-page-hero'},
                el('span', {class: 'docker-page-kicker'}, t('docker.kickerRegistry') || 'Docker Registry'),
                el('div', {class: 'docker-hero-header'},
                    el('h2', {class: 'docker-hero-title'},
                        createIcon('box', {class: 'icon-svg'}),
                        repoName
                    )
                ),
                el('p', {class: 'text-muted', style: {marginBottom: '1rem'}}, t('docker.noImages') || 'No container images found in this repository.'),
                el('p', {style: {fontSize: '0.82rem', fontWeight: '650', color: 'var(--text-color)', marginBottom: '0.35rem'}}, t('docker.pushGuidance') || 'Push container images using the Docker CLI:'),
                el('div', {class: 'docker-pull-box'},
                    el('span', {class: 'docker-pull-text'}, pushHint2),
                    el('button', {
                        class: 'docker-pull-copy-btn',
                        type: 'button',
                        title: t('details.copy') || 'Copy',
                        onclick: (e) => triggerDockerCopy(e.currentTarget, pushHint2)
                    }, createIcon('copy', {class: 'icon-svg'}))
                )
            );

            await morphElementHeight(container, () => {
                container.replaceChildren(hero);
                container.classList.remove('is-updating');
                container.classList.add('is-entering');
                setTimeout(() => container.classList.remove('is-entering'), 400);
            }, {duration: 280});
            return;
        }

        const hero = el('div', {class: 'docker-page-hero'},
            el('span', {class: 'docker-page-kicker'}, t('docker.kickerRegistry') || 'Docker Registry'),
            el('div', {class: 'docker-hero-header'},
                el('h2', {class: 'docker-hero-title'},
                    createIcon('box', {class: 'icon-svg'}),
                    el('span', {}, repoName)
                )
            ),
            el('p', {class: 'text-muted'}, t('docker.imagesSubtitle') || 'Browse and manage Docker / OCI container images and tags in this repository.'),
            el('div', {class: 'docker-hero-meta-row'},
                el('div', {class: 'docker-meta-chip'},
                    createIcon('box', {class: 'icon-svg'}),
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
            const pubName = img.publisher || t('docker.unspecifiedPublisher') || 'Unspecified';
            const publisherMeta = el('span', {class: 'docker-card-publisher'},
                createIcon('user', {class: 'icon-svg'}),
                el('span', {}, pubName)
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
                        createIcon('box', {class: 'icon-svg'}),
                        el('span', {}, img.image_name)
                    ),
                    img.description ? el('p', {class: 'docker-image-desc'}, img.description) : null
                ),
                el('div', {class: 'docker-image-meta'},
                    el('span', {class: 'docker-tag-badge'}, img.latest_tag ? `tag: ${img.latest_tag}` : `${img.tag_count || 0} tags`),
                    publisherMeta,
                    pullMeta,
                    el('span', {}, t('docker.tagCount', {count: img.tag_count || 0}))
                )
            );
            grid.appendChild(card);
        }

        const imagesSection = el('div', {class: 'docker-page-section'},
            el('h3', {style: {fontSize: '1rem', fontWeight: '650', marginBottom: '0.85rem', color: 'var(--text-color)'}}, t('docker.imagesTitle') || 'Container Images'),
            grid
        );

        await morphElementHeight(container, () => {
            container.replaceChildren(hero, imagesSection);
            container.classList.remove('is-updating');
            container.classList.add('is-entering');
            setTimeout(() => container.classList.remove('is-entering'), 400);
        }, {duration: 280});
    } catch (err) {
        if (seq !== dockerLoadSequence) return;
        await morphElementHeight(container, () => {
            container.replaceChildren(
                el('div', {class: 'docker-page-hero'},
                    el('p', {class: 'error-text'}, t('docker.loadFailed') || String(err.message || err))
                )
            );
            container.classList.remove('is-updating');
        }, {duration: 260});
    }
}

/**
 * Return the body-level username suggestion panel for Docker invitation.
 * @returns {HTMLElement} Suggestion panel.
 */
function ensureInviteSuggestionPanel() {
    if (inviteSuggestionPanel?.isConnected) return inviteSuggestionPanel;
    inviteSuggestionPanel = el('div', {
        id: 'docker-invite-suggestions',
        class: 'docker-user-suggestions',
        role: 'listbox',
        hidden: true
    });
    inviteSuggestionPanel.addEventListener('click', handleSuggestionClick);
    document.body.appendChild(inviteSuggestionPanel);
    return inviteSuggestionPanel;
}

/**
 * Position the suggestion panel anchored beneath the invite input.
 * @returns {void}
 */
function positionInviteSuggestions() {
    const panel = ensureInviteSuggestionPanel();
    if (!(inviteInput instanceof HTMLInputElement) || panel.hidden) return;
    const rect = inviteInput.getBoundingClientRect();
    panel.style.left = `${Math.max(10, rect.left)}px`;
    panel.style.width = `${Math.min(rect.width, window.innerWidth - 20)}px`;
    panel.style.top = `${rect.bottom + 6}px`;
}

/**
 * Hide suggestion panel with animation.
 * @param {boolean} [immediate=false] - Skip exit transition.
 * @returns {void}
 */
function closeInviteSuggestions(immediate = false) {
    const panel = ensureInviteSuggestionPanel();
    if (inviteCloseTimer) clearTimeout(inviteCloseTimer);
    panel.classList.remove('is-visible');
    inviteInput?.setAttribute('aria-expanded', 'false');
    if (immediate || panel.hidden) {
        panel.hidden = true;
        panel.classList.remove('is-leaving');
        inviteCloseTimer = 0;
        return;
    }
    panel.classList.add('is-leaving');
    inviteCloseTimer = setTimeout(() => {
        panel.hidden = true;
        panel.classList.remove('is-leaving');
        inviteCloseTimer = 0;
    }, INVITE_CLOSE_DELAY_MS);
}

/**
 * Handle username autocomplete typing.
 * @param {InputEvent} event - Input event.
 * @returns {void}
 */
function handleInviteInput(event) {
    if (inviteSuggestionTimer) clearTimeout(inviteSuggestionTimer);
    const query = String(event.currentTarget.value || '').trim();
    if (!query) {
        renderInviteSuggestions([]);
        return;
    }
    const version = ++inviteSuggestionVersion;
    inviteSuggestionTimer = setTimeout(() => fetchInviteSuggestions(query, version), INVITE_SEARCH_DELAY_MS);
}

/**
 * Fetch username suggestions from API.
 * @param {string} query - Query string.
 * @param {number} version - Request version.
 * @returns {Promise<void>}
 */
async function fetchInviteSuggestions(query, version) {
    if (!activeRepository) return;
    try {
        const res = await apiRequest(`/api/docker/repositories/${encodeURIComponent(activeRepository)}/users/search?q=${encodeURIComponent(query)}`);
        if (version !== inviteSuggestionVersion) return;
        if (res.ok) {
            const data = await res.json();
            renderInviteSuggestions(Array.isArray(data?.users) ? data.users : []);
        } else {
            renderInviteSuggestions([]);
        }
    } catch {
        renderInviteSuggestions([]);
    }
}

/**
 * Render username suggestions in panel.
 * @param {string[]} users - User list.
 * @returns {void}
 */
function renderInviteSuggestions(users) {
    const panel = ensureInviteSuggestionPanel();
    inviteSuggestions = users.slice(0, 8);
    activeInviteSuggestion = -1;
    panel.replaceChildren();
    for (const username of inviteSuggestions) {
        panel.appendChild(el('button', {
            type: 'button',
            role: 'option',
            class: 'docker-user-suggestion',
            'data-suggestion': username
        }, username));
    }
    if (inviteSuggestions.length === 0) {
        closeInviteSuggestions();
        return;
    }
    if (inviteCloseTimer) {
        clearTimeout(inviteCloseTimer);
        inviteCloseTimer = 0;
    }
    panel.hidden = false;
    panel.classList.remove('is-leaving');
    inviteInput?.setAttribute('aria-expanded', 'true');
    positionInviteSuggestions();
    requestAnimationFrame(() => panel.classList.add('is-visible'));
}

/**
 * Apply selected suggestion.
 * @param {MouseEvent} event - Click event.
 * @returns {void}
 */
function handleSuggestionClick(event) {
    const opt = event.target.closest('[data-suggestion]');
    if (!opt) return;
    const username = opt.dataset.suggestion;
    if (inviteInput instanceof HTMLInputElement) {
        inviteInput.value = username;
        inviteInput.focus();
    }
    closeInviteSuggestions();
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
    container.classList.add('is-updating');
    try {
        const response = await apiRequest(`/api/docker/repositories/${encodeURIComponent(repoName)}/images?image=${encodeURIComponent(imageName)}`);
        if (seq !== dockerLoadSequence) return;
        if (!response.ok) {
            await morphElementHeight(container, () => {
                container.replaceChildren(
                    el('div', {class: 'docker-page-hero'},
                        el('button', {
                            class: 'docker-page-back',
                            type: 'button',
                            onclick: () => {
                                if (activeNavigate) activeNavigate(`/${encodePathSegment(repoName)}`);
                            }
                        }, createIcon('chevronLeft', {class: 'icon-svg'}), el('span', {}, t('docker.backToImages') || 'Back')),
                        el('p', {class: 'error-text'}, 'Container image not found.')
                    )
                );
                container.classList.remove('is-updating');
            }, {duration: 260});
            return;
        }

        const details = await response.json();
        let image = details.image || {};
        const tags = details.tags || [];
        const members = details.members || [];
        const permissionLevel = Number(details.permission_level || 0);
        const isAdministrator = Boolean(details.administrator);

        const canManageL2 = isAdministrator || permissionLevel >= 2;
        const canManageL3 = isAdministrator || permissionLevel >= 3;

        const latestTag = tags[0]?.tag || 'latest';
        const pullCmd = `docker pull ${window.location.host}/${repoName}/${imageName}:${latestTag}`;

        const backBtn = el('button', {
            class: 'docker-page-back',
            type: 'button',
            onclick: () => {
                if (activeNavigate) activeNavigate(`/${encodePathSegment(repoName)}`);
            }
        }, createIcon('chevronLeft', {class: 'icon-svg'}), el('span', {}, t('docker.backToImages') || 'Back'));

        const topNav = el('div', {class: 'docker-hero-nav'},
            backBtn,
            el('span', {class: 'docker-page-kicker'}, t('docker.kickerImage') || 'Container Image')
        );

        let deleteImgBtn = null;
        if (canManageL3) {
            deleteImgBtn = el('button', {
                class: 'docker-btn-danger',
                type: 'button',
                title: t('docker.deleteImage') || 'Delete Image',
                onclick: async () => {
                    const confirmed = await showConfirm(
                        t('docker.deleteImageConfirm', {image: imageName}) || `Delete image "${imageName}"?`
                    );
                    if (confirmed) {
                        const delResp = await apiRequest(`/api/docker/repositories/${encodeURIComponent(repoName)}/images?image=${encodeURIComponent(imageName)}`, {
                            method: 'DELETE'
                        });
                        if (delResp.ok) {
                            showAlert(t('docker.imageDeleted') || 'Image deleted.');
                            if (activeNavigate) activeNavigate(`/${encodePathSegment(repoName)}`);
                        } else {
                            showAlert(t('docker.deleteImageFailed') || 'Failed to delete image.');
                        }
                    }
                }
            }, createIcon('delete', {class: 'icon-svg'}), el('span', {}, t('docker.deleteImage') || 'Delete Image'));
        }

        const metaRow = el('div', {class: 'docker-hero-meta-row'},
            el('div', {class: 'docker-meta-chip'},
                createIcon('folder', {class: 'icon-svg'}),
                el('span', {}, repoName)
            )
        );

        const heroPubName = image.publisher || t('docker.unspecifiedPublisher') || 'Unspecified';
        metaRow.appendChild(
            el('div', {class: 'docker-meta-chip'},
                createIcon('user', {class: 'icon-svg'}),
                el('span', {}, t('docker.publishedBy', {name: heroPubName}) || `Published by ${heroPubName}`)
            )
        );

        metaRow.appendChild(
            el('div', {class: 'docker-meta-chip'},
                createIcon('fileCode', {class: 'icon-svg'}),
                el('span', {}, t('docker.latestTag', {tag: latestTag}))
            )
        );

        metaRow.appendChild(
            el('div', {class: 'docker-meta-chip'},
                createIcon('box', {class: 'icon-svg'}),
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
                    el('span', {}, `${t('docker.created') || 'Created'}: ${dockerDate(image.created_at)}`)
                )
            );
        }
        if (image.updated_at && image.updated_at !== image.created_at) {
            metaRow.appendChild(
                el('div', {class: 'docker-meta-chip'},
                    createIcon('clock', {class: 'icon-svg'}),
                    el('span', {}, `${t('docker.updated') || 'Updated'}: ${dockerDate(image.updated_at)}`)
                )
            );
        }

        const pullBox = el('div', {class: 'docker-pull-box'},
            el('span', {class: 'docker-pull-text'}, pullCmd),
            el('button', {
                class: 'docker-pull-copy-btn',
                type: 'button',
                title: t('docker.copyPull') || 'Copy pull command',
                onclick: (e) => triggerDockerCopy(e.currentTarget, pullCmd)
            }, createIcon('copy', {class: 'icon-svg'}))
        );

        const hero = el('div', {class: 'docker-page-hero'},
            topNav,
            el('div', {class: 'docker-hero-header'},
                el('h2', {class: 'docker-hero-title'},
                    createIcon('box', {class: 'icon-svg'}),
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
                    el('span', {}, t('docker.noTags') || 'No tags or manifests found for this container image.')
                )
            );
        } else {
            for (const tObj of tags) {
                const tagPullCmd = `docker pull ${window.location.host}/${repoName}/${imageName}:${tObj.tag}`;
                const shortDigest = tObj.digest ? tObj.digest.slice(0, 19) + '…' : '';
                const sizeStr = tObj.size > 0 ? formatBytes(tObj.size) : '';
                const timeStr = tObj.updated_at ? dockerDate(tObj.updated_at) : (tObj.created_at ? dockerDate(tObj.created_at) : '');
                const tagPublisher = tObj.publisher || image.publisher || t('docker.unspecifiedPublisher') || 'Unspecified';

                const digestPill = shortDigest
                    ? el('span', {
                        class: 'docker-tag-digest',
                        title: `${tObj.digest} (${t('docker.copyDigest') || 'Click to copy digest'})`,
                        onclick: (e) => triggerDockerCopy(e.currentTarget, tObj.digest)
                    }, createIcon('fileHash', {class: 'icon-svg'}), el('span', {}, shortDigest))
                    : null;

                const publisherChip = el('span', {class: 'docker-tag-publisher'},
                    createIcon('user', {class: 'icon-svg'}),
                    el('span', {}, tagPublisher)
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
                        title: t('docker.copyPull') || 'Copy pull command',
                        onclick: (e) => triggerDockerCopy(e.currentTarget, tagPullCmd)
                    }, createIcon('copy', {class: 'icon-svg'})),
                    el('button', {
                        class: 'docker-action-btn',
                        type: 'button',
                        title: t('docker.inspect') || 'Inspect Manifest',
                        onclick: () => openManifestDetails(repoName, imageName, tObj.digest, tObj.tag)
                    }, createIcon('eye', {class: 'icon-svg'}))
                );

                if (canManageL2) {
                    actionsWrap.appendChild(
                        el('button', {
                            class: 'docker-action-btn docker-action-btn--delete',
                            type: 'button',
                            title: t('docker.deleteTag') || 'Delete tag',
                            onclick: async () => {
                                const confirmed = await showConfirm(
                                    t('docker.deleteTagConfirm', {tag: tObj.tag, image: imageName}) || `Delete tag "${tObj.tag}"?`
                                );
                                if (confirmed) {
                                    const delResp = await apiRequest(`/api/docker/repositories/${encodeURIComponent(repoName)}/tags?image=${encodeURIComponent(imageName)}&tag=${encodeURIComponent(tObj.tag)}`, {
                                        method: 'DELETE'
                                    });
                                    if (delResp.ok) {
                                        showAlert(t('docker.tagDeleted') || 'Tag deleted.');
                                        renderImageDetailsView(container, repoName, imageName, seq);
                                    } else {
                                        showAlert(t('docker.deleteTagFailed') || 'Failed to delete tag.');
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
                el('h3', {style: {fontSize: '1rem', fontWeight: '650', margin: '0', color: 'var(--text-color)'}}, t('docker.tagsTitle') || 'Tags & Manifests'),
                el('span', {class: 'docker-tag-badge'}, `${tags.length}`)
            ),
            tagListEl
        );

        // README / Markdown Section
        const readmeContent = el('div');
        const updateReadmeView = (descText) => {
            if (descText && descText.trim().length > 0) {
                readmeContent.className = 'docker-readme-body';
                readmeContent.innerHTML = marked.parse(descText.trim());
            } else {
                readmeContent.className = 'docker-readme-empty';
                readmeContent.replaceChildren(
                    createIcon('fileText', {class: 'icon-svg'}),
                    el('span', {}, t('docker.noReadme') || 'No README or description provided for this container image.')
                );
            }
        };
        updateReadmeView(image.description);

        let editReadmeBtn = null;
        if (canManageL2) {
            editReadmeBtn = el('button', {
                class: 'docker-btn-secondary',
                type: 'button',
                title: t('docker.editReadme') || 'Edit README',
                onclick: () => {
                    openReadmeEditor(repoName, imageName, image.description || '', (newDesc) => {
                        image.description = newDesc;
                        updateReadmeView(newDesc);
                    });
                }
            }, createIcon('edit', {class: 'icon-svg'}), el('span', {}, t('docker.editReadme') || 'Edit README'));
        }

        const readmeSection = el('div', {class: 'docker-readme-card'},
            el('div', {class: 'docker-readme-header'},
                el('h3', {class: 'docker-readme-title'},
                    createIcon('fileText', {class: 'icon-svg'}),
                    t('docker.readme') || 'README'
                ),
                editReadmeBtn
            ),
            readmeContent
        );

        // Team / Collaborators Section
        let teamSection = null;
        if (members.length > 0 || canManageL3) {
            const teamListEl = el('div', {class: 'docker-team-list'});

            for (const member of members) {
                const memberLevel = Number(member.level || 1);
                const levelLabel = memberLevel === 4
                    ? (t('docker.permissionL4') || 'L4 (Owner)')
                    : (memberLevel === 3
                        ? (t('docker.permissionL3') || 'L3 (Team)')
                        : (memberLevel === 2 ? (t('docker.permissionL2') || 'L2 (Manage)') : (t('docker.permissionL1') || 'L1 (Push)')));

                const memberControls = el('div', {class: 'docker-team-controls'});

                if (canManageL3) {
                    const levelSelect = makeCustomSelect(
                        [
                            { value: '1', label: t('docker.permissionL1') || 'L1 (Push)' },
                            { value: '2', label: t('docker.permissionL2') || 'L2 (Manage)' },
                            { value: '3', label: t('docker.permissionL3') || 'L3 (Team)' },
                            { value: '4', label: t('docker.permissionL4') || 'L4 (Owner)' }
                        ],
                        String(memberLevel),
                        async (newVal) => {
                            const newLevel = parseInt(newVal, 10);
                            try {
                                const updateResp = await apiRequest(`/api/docker/repositories/${encodeURIComponent(repoName)}/owners/${encodeURIComponent(member.username)}?image=${encodeURIComponent(imageName)}`, {
                                    method: 'PUT',
                                    headers: {'Content-Type': 'application/json'},
                                    body: JSON.stringify({level: newLevel})
                                });
                                if (updateResp.ok) {
                                    member.level = newLevel;
                                    showAlert(t('docker.memberUpdated') || 'Permission level updated.', 'success');
                                    renderImageDetailsView(container, repoName, imageName, seq);
                                } else {
                                    const errMsg = await updateResp.text();
                                    showAlert(t('docker.updateMemberFailed') || errMsg || 'Failed to update member permission.');
                                    if (typeof levelSelect.setValue === 'function') levelSelect.setValue(String(memberLevel));
                                }
                            } catch (err) {
                                showAlert(String(err.message || err));
                                if (typeof levelSelect.setValue === 'function') levelSelect.setValue(String(memberLevel));
                            }
                        }
                    );
                    levelSelect.classList.add('docker-permission-select');
                    memberControls.appendChild(levelSelect);

                    const removeBtn = el('button', {
                        class: 'docker-action-btn docker-action-btn--delete',
                        type: 'button',
                        title: t('docker.removeMember') || 'Remove Member',
                        onclick: async () => {
                            const confirmed = await showConfirm(
                                t('docker.removeMemberConfirm', {name: member.username}) || `Remove member "${member.username}"?`
                            );
                            if (confirmed) {
                                const remResp = await apiRequest(`/api/docker/repositories/${encodeURIComponent(repoName)}/owners/${encodeURIComponent(member.username)}?image=${encodeURIComponent(imageName)}`, {
                                    method: 'DELETE'
                                });
                                if (remResp.ok) {
                                    showAlert(t('docker.memberRemoved') || 'Member removed.', 'success');
                                    renderImageDetailsView(container, repoName, imageName, seq);
                                } else {
                                    const errMsg = await remResp.text();
                                    showAlert(t('docker.removeMemberFailed') || errMsg || 'Failed to remove member.');
                                }
                            }
                        }
                    }, createIcon('delete', {class: 'icon-svg'}));
                    memberControls.appendChild(removeBtn);
                } else {
                    memberControls.appendChild(
                        el('span', {class: 'docker-permission-badge'}, levelLabel)
                    );
                }

                const memberRow = el('div', {class: 'docker-team-row'},
                    el('div', {class: 'docker-team-member'},
                        el('strong', {class: 'docker-team-username'}, member.username),
                        member.added_at ? el('span', {class: 'docker-team-time'}, dockerDate(member.added_at)) : null
                    ),
                    memberControls
                );
                teamListEl.appendChild(memberRow);
            }

            let inviteForm = null;
            if (canManageL3) {
                inviteInput = el('input', {
                    type: 'text',
                    class: 'docker-invite-input',
                    placeholder: t('docker.invitePlaceholder') || 'Enter username to invite...',
                    autocomplete: 'off',
                    oninput: handleInviteInput,
                    onkeydown: (e) => {
                        if (e.key === 'Escape') closeInviteSuggestions();
                    }
                });

                inviteLevel = 1;
                const inviteLevelSelect = makeCustomSelect(
                    [
                        { value: '1', label: t('docker.permissionL1') || 'L1 (Push)' },
                        { value: '2', label: t('docker.permissionL2') || 'L2 (Manage)' },
                        { value: '3', label: t('docker.permissionL3') || 'L3 (Team)' },
                        { value: '4', label: t('docker.permissionL4') || 'L4 (Owner)' }
                    ],
                    '1',
                    (val) => {
                        inviteLevel = parseInt(val, 10) || 1;
                    }
                );
                inviteLevelSelect.classList.add('docker-permission-select');

                const sendInviteBtn = el('button', {
                    type: 'submit',
                    class: 'docker-btn-secondary primary-btn',
                    title: t('docker.invite') || 'Invite'
                }, createIcon('userPlus', {class: 'icon-svg'}), el('span', {}, t('docker.invite') || 'Invite'));

                inviteForm = el('form', {
                    class: 'docker-invite-form',
                    onsubmit: async (e) => {
                        e.preventDefault();
                        const userToInvite = inviteInput.value.trim();
                        if (!userToInvite) {
                            showAlert(t('docker.inviteUsernameRequired') || 'Please enter a username to invite.', 'warning');
                            return;
                        }
                        const currentUser = localStorage.getItem('username') || '';
                        if (currentUser && userToInvite.toLowerCase() === currentUser.toLowerCase()) {
                            showAlert(t('docker.cannotInviteSelf') || 'You cannot invite yourself.', 'warning');
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
                                inviteInput.value = '';
                                closeInviteSuggestions();
                                const respData = await inviteResp.json().catch(() => null);
                                showAlert(respData?.message || t('docker.inviteSent', {name: userToInvite}) || `Invitation sent to ${userToInvite}.`, 'success');
                                renderImageDetailsView(container, repoName, imageName, seq);
                            } else {
                                const errStatus = inviteResp.status;
                                const errText = await inviteResp.text();
                                let msg = t('docker.sendInviteFailed') || 'Failed to send invitation.';
                                if (errText.includes('already a member')) {
                                    msg = t('docker.memberAlreadyExists') || 'User is already a member of this image team.';
                                } else if (errText.includes('pending')) {
                                    msg = t('docker.invitationAlreadyPending') || 'An active invitation is already pending for this user.';
                                } else if (errText.includes('does not exist')) {
                                    msg = t('docker.userNotFound', {name: userToInvite}) || `User "${userToInvite}" does not exist.`;
                                } else if (errText.includes('yourself')) {
                                    msg = t('docker.cannotInviteSelf') || 'You cannot invite yourself.';
                                } else if (errStatus === 403) {
                                    msg = t('docker.permissionDenied') || 'Permission denied.';
                                }
                                showAlert(msg);
                            }
                        } catch (err) {
                            showAlert(String(err.message || err));
                        } finally {
                            sendInviteBtn.disabled = false;
                        }
                    }
                }, inviteInput, inviteLevelSelect, sendInviteBtn);
            }

            teamSection = el('div', {class: 'docker-page-section'},
                el('div', {style: {display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: '0.85rem'}},
                    el('h3', {style: {fontSize: '1rem', fontWeight: '650', margin: '0', color: 'var(--text-color)'}}, t('docker.teamTitle') || 'Team & Collaborators'),
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

        await morphElementHeight(container, () => {
            container.replaceChildren(...sections);
            container.classList.remove('is-updating');
            container.classList.add('is-entering');
            setTimeout(() => container.classList.remove('is-entering'), 400);
        }, {duration: 280});
    } catch (err) {
        if (seq !== dockerLoadSequence) return;
        await morphElementHeight(container, () => {
            container.replaceChildren(
                el('div', {class: 'docker-page-hero'},
                    el('p', {class: 'error-text'}, t('docker.imageNotFound') || String(err.message || err))
                )
            );
            container.classList.remove('is-updating');
        }, {duration: 260});
    }
}

