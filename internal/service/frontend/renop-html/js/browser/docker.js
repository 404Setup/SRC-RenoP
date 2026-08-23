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
import {apiRequest} from '../api.js';
import {showAlert, showConfirm} from '../alert.js';
import {createIcon, createMetaGrid, createSkeleton, RenopDialog} from '../components.js';
import {t} from '../i18n.js';
import {decodePathSegment, encodePathSegment, formatBytes} from './utils.js';

let dockerViewContainer = null;
let activeRepository = '';
let activeNavigate = null;
let dockerLoadSequence = 0;

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
                            showAlert('Failed to update README.');
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
            const publisherMeta = img.publisher
                ? el('span', {class: 'docker-card-publisher'},
                    createIcon('user', {class: 'icon-svg'}),
                    el('span', {}, img.publisher)
                )
                : null;

            const card = el('div', {
                class: 'docker-image-card',
                onclick: () => {
                    if (activeNavigate) {
                        activeNavigate(`/${encodePathSegment(repoName)}/${encodePathSegment(img.image_name)}`);
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
                    el('p', {class: 'error-text'}, String(err.message || err))
                )
            );
            container.classList.remove('is-updating');
        }, {duration: 260});
    }
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

        const deleteImgBtn = el('button', {
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
                    }
                }
            }
        }, createIcon('delete', {class: 'icon-svg'}), el('span', {}, t('docker.deleteImage') || 'Delete Image'));

        const metaRow = el('div', {class: 'docker-hero-meta-row'},
            el('div', {class: 'docker-meta-chip'},
                createIcon('folder', {class: 'icon-svg'}),
                el('span', {}, repoName)
            )
        );

        if (image.publisher) {
            metaRow.appendChild(
                el('div', {class: 'docker-meta-chip'},
                    createIcon('user', {class: 'icon-svg'}),
                    el('span', {}, t('docker.publishedBy', {name: image.publisher}) || image.publisher)
                )
            );
        }

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
                const tagPublisher = tObj.publisher || '';

                const digestPill = shortDigest
                    ? el('span', {
                        class: 'docker-tag-digest',
                        title: `${tObj.digest} (${t('docker.copyDigest') || 'Click to copy digest'})`,
                        onclick: (e) => triggerDockerCopy(e.currentTarget, tObj.digest)
                    }, createIcon('fileHash', {class: 'icon-svg'}), el('span', {}, shortDigest))
                    : null;

                const publisherChip = tagPublisher
                    ? el('span', {class: 'docker-tag-publisher'},
                        createIcon('user', {class: 'icon-svg'}),
                        el('span', {}, tagPublisher)
                    )
                    : null;

                const timeChip = timeStr
                    ? el('span', {class: 'docker-tag-time'},
                        createIcon('clock', {class: 'icon-svg'}),
                        el('span', {}, timeStr)
                    )
                    : null;

                const row = el('div', {class: 'docker-tag-row'},
                    el('div', {class: 'docker-tag-left'},
                        el('span', {class: 'docker-tag-badge'}, tObj.tag),
                        digestPill,
                        sizeStr ? el('span', {class: 'docker-tag-size'}, sizeStr) : null,
                        publisherChip,
                        timeChip
                    ),
                    el('div', {class: 'docker-tag-actions'},
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
                        }, createIcon('eye', {class: 'icon-svg'})),
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
                                    }
                                }
                            }
                        }, createIcon('delete', {class: 'icon-svg'}))
                    )
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

        const editReadmeBtn = el('button', {
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

        await morphElementHeight(container, () => {
            container.replaceChildren(hero, tagsSection, readmeSection);
            container.classList.remove('is-updating');
            container.classList.add('is-entering');
            setTimeout(() => container.classList.remove('is-entering'), 400);
        }, {duration: 280});
    } catch (err) {
        if (seq !== dockerLoadSequence) return;
        await morphElementHeight(container, () => {
            container.replaceChildren(
                el('div', {class: 'docker-page-hero'},
                    el('p', {class: 'error-text'}, String(err.message || err))
                )
            );
            container.classList.remove('is-updating');
        }, {duration: 260});
    }
}

