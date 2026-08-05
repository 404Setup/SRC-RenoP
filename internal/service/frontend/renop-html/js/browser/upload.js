/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {t} from '../i18n.js';
import {showAlert} from '../alert.js';
import {fetchProto, getAuthHeaders} from '../api.js';
import {canUpdateRepo} from '../auth.js';
import {loadDirectory} from '../browser.js';
import {createUploadEntry} from '../components.js';
import {
    shouldUseChunkedUpload,
    uploadFileChunked,
    uploadFileSinglePut,
} from '../chunked-upload.js';
import {collapseElement, expandElement} from '@renop/ui/height-anim';
import {decodePathSegment, encodePathSegment, encodeRelativePath, formatBytes} from './utils.js';
import {InstanceStatus} from '../proto/index.js';

let pendingFiles = [];
let uploading = false;
let controlsAnimToken = 0;
let zoneAnimToken = 0;

const UPLOAD_PANEL_MARGIN = '1.25rem';
const UPLOAD_CONTROLS_MARGIN = '0.85rem';

/**
 * Return the lowercased file extension without the leading dot, or `''`.
 * @param {string} name
 * @returns {string}
 */
function fileExt(name) {
    const idx = name.lastIndexOf('.');
    return idx > 0 ? name.slice(idx + 1).toLowerCase() : '';
}

/**
 * Clear the pending-files list UI (entries, counts, batch progress).
 * @returns {void}
 */
function clearUploadControlsContent() {
    const uploadFileList = document.getElementById('upload-file-list');
    const fileCount = document.getElementById('upload-file-count');
    const totalSize = document.getElementById('upload-total-size');
    const batchProgress = document.getElementById('upload-batch-progress-container');
    if (uploadFileList) {
        uploadFileList.innerHTML = '';
        uploadFileList.classList.remove('is-deleting', 'has-multiple-files', 'has-many-files');
    }
    if (fileCount) fileCount.textContent = '';
    if (totalSize) totalSize.textContent = '';
    if (batchProgress) batchProgress.style.display = 'none';
}

/**
 * Open/close the selected-files panel with a real pixel-height animation
 * (same helpers as repo-stats). CSS grid 0fr→1fr is unreliable here.
 * @param {boolean} open
 * @returns {Promise<void>}
 */
async function setUploadControlsOpen(open) {
    const uploadControls = document.getElementById('upload-controls');
    if (!uploadControls) return;

    const token = ++controlsAnimToken;

    if (open) {
        uploadControls.setAttribute('aria-hidden', 'false');

        if (uploadControls.classList.contains('is-visible')
            && !uploadControls.hidden
            && getComputedStyle(uploadControls).display !== 'none') {
            uploadControls.classList.add('is-open');
            return;
        }

        uploadControls.classList.add('is-open');
        await expandElement(uploadControls, {
            duration: 360,
            marginTop: UPLOAD_CONTROLS_MARGIN,
        });

        if (token !== controlsAnimToken) return;
        if (pendingFiles.length === 0) {
            await setUploadControlsOpen(false);
        }
        return;
    }

    uploadControls.setAttribute('aria-hidden', 'true');
    uploadControls.classList.remove('is-open');

    if (!uploadControls.classList.contains('is-visible')) {
        if (pendingFiles.length === 0) clearUploadControlsContent();
        uploadControls.hidden = true;
        uploadControls.style.display = 'none';
        return;
    }

    await collapseElement(uploadControls, {
        duration: 320,
        marginTop: true,
    });

    if (token !== controlsAnimToken) return;
    if (pendingFiles.length === 0) {
        clearUploadControlsContent();
    }
}

/**
 * Whether an animated panel element is currently fully visible.
 * @param {HTMLElement|null} el
 * @returns {boolean}
 */
function isPanelVisible(el) {
    return !!(el
        && el.classList.contains('is-visible')
        && !el.hidden
        && getComputedStyle(el).display !== 'none');
}

/**
 * Hard-reset upload controls DOM state and clear pending-file UI content.
 * @returns {void}
 */
function resetUploadControlsState() {
    controlsAnimToken += 1;
    const uploadControls = document.getElementById('upload-controls');
    if (uploadControls) {
        uploadControls.classList.remove('is-open', 'is-visible');
        uploadControls.setAttribute('aria-hidden', 'true');
        uploadControls.hidden = true;
        uploadControls.style.display = 'none';
        uploadControls.style.height = '';
        uploadControls.style.overflow = '';
        uploadControls.style.opacity = '';
        uploadControls.style.marginTop = '';
        uploadControls.style.transition = '';
    }
    clearUploadControlsContent();
}

/**
 * Show/hide the whole Overview upload window (dropzone + controls).
 * Uses one height animation for the entire panel so appear/disappear feels complete.
 * @param {boolean} open
 * @returns {Promise<void>}
 */
async function setUploadPanelOpen(open) {
    const panel = document.getElementById('upload-zone-container');
    if (!panel) return;

    const token = ++zoneAnimToken;

    if (open) {
        if (isPanelVisible(panel)) {
            panel.style.display = 'flex';
            return;
        }

        await expandElement(panel, {
            duration: 380,
            marginTop: UPLOAD_PANEL_MARGIN,
        });

        if (token !== zoneAnimToken) return;
        panel.style.display = 'flex';
        return;
    }

    pendingFiles = [];
    controlsAnimToken += 1;

    if (!panel.classList.contains('is-visible')) {
        panel.hidden = true;
        panel.style.display = 'none';
        resetUploadControlsState();
        return;
    }

    await collapseElement(panel, {
        duration: 340,
        marginTop: true,
    });

    if (token !== zoneAnimToken) return;
    resetUploadControlsState();
}

/**
 * Show or hide the upload panel based on path and update permission.
 * @param {string} path
 * @returns {Promise<void>}
 */
export async function updateUploadZone(path) {
    const uploadZoneContainer = document.getElementById('upload-zone-container');
    const uploadDestination = document.getElementById('upload-destination');
    if (!uploadZoneContainer) return;

    const pathParts = path.split('/').filter(p => p.length > 0).map(decodePathSegment);
    const shouldShow = pathParts.length >= 1 && canUpdateRepo(pathParts[0]);

    if (shouldShow) {
        if (uploadDestination) {
            uploadDestination.value = pathParts.join('/');
        }
        await setUploadPanelOpen(true);
    } else {
        await setUploadPanelOpen(false);
    }
}

/**
 * Wire dropzone, file input, clear, and upload-button handlers.
 * @returns {void}
 */
export function initUpload() {
    const uploadZone = document.getElementById('upload-zone');
    const fileInput = document.getElementById('file-upload-input');
    const uploadBtn = document.getElementById('upload-btn');
    const uploadDestination = document.getElementById('upload-destination');
    const checksumCheckbox = document.getElementById('checksum-checkbox');
    const pomCheckbox = document.getElementById('pom-checkbox');
    const pomForm = document.getElementById('pom-form');
    const pomGroupId = document.getElementById('pom-group-id');
    const pomArtifactId = document.getElementById('pom-artifact-id');
    const pomVersion = document.getElementById('pom-version');
    const clearBtn = document.getElementById('upload-clear-btn');

    if (!uploadZone) return;

    uploadZone.addEventListener('click', (e) => {
        if (e.target.closest('button')) return;
        fileInput?.click();
    });

    uploadZone.addEventListener('keydown', (e) => {
        if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault();
            fileInput?.click();
        }
    });

    let dragCounter = 0;

    uploadZone.addEventListener('dragenter', (e) => {
        e.preventDefault();
        dragCounter++;
        uploadZone.classList.add('dragover');
    });

    uploadZone.addEventListener('dragover', (e) => {
        e.preventDefault();
    });

    uploadZone.addEventListener('dragleave', (e) => {
        e.preventDefault();
        dragCounter--;
        if (dragCounter <= 0) {
            dragCounter = 0;
            uploadZone.classList.remove('dragover');
        }
    });

    uploadZone.addEventListener('drop', (e) => {
        e.preventDefault();
        dragCounter = 0;
        uploadZone.classList.remove('dragover');
        if (e.dataTransfer && e.dataTransfer.files.length > 0) {
            addPendingFiles(e.dataTransfer.files);
        }
    });

    if (fileInput) {
        fileInput.addEventListener('change', () => {
            if (fileInput.files.length > 0) {
                addPendingFiles(fileInput.files);
                fileInput.value = '';
            }
        });
    }

    if (pomCheckbox) {
        pomCheckbox.addEventListener('change', () => {
            if (pomForm) {
                pomForm.classList.toggle('is-open', pomCheckbox.checked);
            }
        });
        if (pomForm) {
            pomForm.classList.toggle('is-open', pomCheckbox.checked);
        }
    }

    if (clearBtn) {
        clearBtn.addEventListener('click', () => {
            if (uploading) return;
            const listEl = document.getElementById('upload-file-list');
            const entries = listEl
                ? Array.from(listEl.querySelectorAll('.upload-file-entry:not(.is-leaving)'))
                : [];
            if (entries.length > 0) {
                if (listEl) listEl.classList.add('is-deleting');
                entries.forEach((entry, i) => {
                    setTimeout(() => {
                        if (!entry || !entry.isConnected) return;
                        const currentHeight = entry.offsetHeight;
                        entry.style.height = currentHeight + 'px';
                        entry.style.setProperty('--entry-height', currentHeight + 'px');
                        requestAnimationFrame(() => {
                            requestAnimationFrame(() => {
                                entry.style.transition =
                                    'height 0.3s cubic-bezier(0.4, 0, 0.2, 1),' +
                                    'margin 0.3s cubic-bezier(0.4, 0, 0.2, 1),' +
                                    'padding 0.3s cubic-bezier(0.4, 0, 0.2, 1),' +
                                    'border-width 0.3s cubic-bezier(0.4, 0, 0.2, 1)';
                                entry.style.height = '0';
                                entry.style.marginTop = '0';
                                entry.style.marginBottom = '-0.65rem';
                                entry.style.paddingTop = '0';
                                entry.style.paddingBottom = '0';
                                entry.style.borderWidth = '0';
                                entry.classList.add('is-leaving');
                            });
                        });
                    }, i * 30);
                });
                const leaveMs = 320 + Math.max(0, entries.length - 1) * 30;
                setTimeout(() => {
                    pendingFiles = [];
                    renderPendingFiles();
                }, leaveMs);
            } else {
                pendingFiles = [];
                renderPendingFiles();
            }
        });
    }

    if (uploadBtn) {
        uploadBtn.addEventListener('click', async () => {
            if (pendingFiles.length === 0 || uploading) return;

            if (await updateUploadSpaceCheck()) {
                showAlert(t('browser.insufficientDiskSpace'), 'error');
                return;
            }

            const dest = uploadDestination ? uploadDestination.value : '';
            const headers = getAuthHeaders();

            if (checksumCheckbox && checksumCheckbox.checked) {
                headers['X-Generate-Checksums'] = 'true';
            }

            const currentPath = window.location.pathname;
            const repoSegment = currentPath.split('/').filter(p => p.length > 0)[0];
            if (!repoSegment) return;
            const repoName = decodePathSegment(repoSegment);

            uploading = true;
            setUploadBusy(true);

            const totalFiles = pendingFiles.length;
            const totalBytesAll = pendingFiles.reduce((acc, f) => acc + f.size, 0);
            let completedFiles = 0;
            let bytesUploadedBefore = 0;

            const batchContainer = document.getElementById('upload-batch-progress-container');
            const batchText = document.getElementById('upload-batch-progress-text');
            const batchPercent = document.getElementById('upload-batch-progress-percent');
            const batchFill = document.getElementById('upload-batch-progress-fill');

            if (totalFiles > 1 && batchContainer) {
                batchContainer.style.display = 'block';
                if (batchText) batchText.textContent = `${completedFiles} / ${totalFiles} ${t('browser.file') || 'files'}`;
                if (batchPercent) batchPercent.textContent = '0%';
                if (batchFill) batchFill.style.width = '0%';
            }

            for (let i = 0; i < pendingFiles.length; i++) {
                const file = pendingFiles[i];
                file._status = 'uploading';
                const entryDiv = document.getElementById('upload-entry-' + i);
                if (entryDiv) {
                    entryDiv.setAttribute('status', 'uploading');
                }

                try {
                    let cleanDest = dest;
                    if (cleanDest.startsWith('/')) cleanDest = cleanDest.substring(1);
                    if (!cleanDest.endsWith('/')) cleanDest += '/';
                    const relativeDest = cleanDest + file.name;
                    const targetPath = '/' + encodeRelativePath(relativeDest);

                    if (entryDiv) {
                        entryDiv.classList.add('is-uploading');
                        entryDiv.setAttribute('status', 'uploading');
                        entryDiv.setAttribute('progress', '0%');
                    }

                    const applyProgress = (loaded, total) => {
                        if (!(total > 0)) return;
                        const percentComplete = Math.round((loaded / total) * 100);
                        file._progress = percentComplete + '%';
                        if (entryDiv) {
                            entryDiv.setAttribute('progress', percentComplete + '%');
                        }

                        if (totalFiles > 1 && totalBytesAll > 0) {
                            const currentOverallBytes = bytesUploadedBefore + loaded;
                            const overallPercent = Math.min(100, Math.round((currentOverallBytes / totalBytesAll) * 100));
                            if (batchPercent) batchPercent.textContent = overallPercent + '%';
                            if (batchFill) batchFill.style.width = overallPercent + '%';
                        }
                    };

                    const applyChunkProgress = (chunks) => {
                        if (entryDiv && typeof entryDiv.setChunkStates === 'function') {
                            entryDiv.setChunkStates(chunks);
                        }
                    };

                    const markDone = () => {
                        file._status = 'done';
                        file._progress = '100%';
                        if (entryDiv) {
                            entryDiv.classList.remove('is-uploading');
                            entryDiv.classList.add('is-done');
                            entryDiv.setAttribute('status', 'done');
                            entryDiv.setAttribute('progress', '100%');
                        }
                        showAlert(t('browser.uploadedSuccess', {name: file.name}), 'success');
                    };

                    const markError = () => {
                        file._status = 'error';
                        if (entryDiv) {
                            entryDiv.classList.remove('is-uploading');
                            entryDiv.classList.add('is-error');
                            entryDiv.setAttribute('status', 'error');
                        }
                        showAlert(t('browser.failedUpload', {name: file.name}), 'error');
                    };

                    let ok = false;
                    if (shouldUseChunkedUpload(file)) {
                        const result = await uploadFileChunked(file, {
                            purpose: 'storage',
                            path: relativeDest.replace(/\/+/g, '/').replace(/^\//, ''),
                            generateChecksums: !!(checksumCheckbox && checksumCheckbox.checked),
                            headers,
                            onProgress: applyProgress,
                            onChunkProgress: applyChunkProgress,
                        });
                        ok = result.ok;
                    } else {
                        const result = await uploadFileSinglePut(targetPath, file, headers, applyProgress);
                        ok = result.ok;
                    }

                    if (ok) markDone();
                    else markError();

                    completedFiles++;
                    bytesUploadedBefore += file.size;
                    if (totalFiles > 1 && batchText) {
                        batchText.textContent = `${completedFiles} / ${totalFiles} ${t('browser.file') || 'files'}`;
                    }
                } catch (err) {
                    console.error('Upload failed', err);
                    file._status = 'error';
                    if (entryDiv) {
                        entryDiv.classList.remove('is-uploading');
                        entryDiv.classList.add('is-error');
                        entryDiv.setAttribute('status', 'error');
                    }
                    showAlert(t('browser.failedUpload', {name: file.name}), 'error');
                    completedFiles++;
                    bytesUploadedBefore += file.size;
                    if (totalFiles > 1 && batchText) {
                        batchText.textContent = `${completedFiles} / ${totalFiles} ${t('browser.file') || 'files'}`;
                    }
                }
            }

            if (pomCheckbox && pomCheckbox.checked) {
                const groupId = pomGroupId.value;
                const artifactId = pomArtifactId.value;
                const version = pomVersion.value;

                if (groupId && artifactId && version) {
                    try {
                        const relPath = dest.substring(repoName.length + 1) || '';
                        const encodedPomPath = encodeRelativePath(relPath);
                        const pomResp = await fetch(`/api/maven/generate/pom/${encodePathSegment(repoName)}/${encodedPomPath}`, {
                            method: 'POST',
                            headers: {
                                ...getAuthHeaders(),
                                'Content-Type': 'application/json'
                            },
                            body: JSON.stringify({
                                group_id: groupId,
                                artifact_id: artifactId,
                                version: version
                            })
                        });

                        if (!pomResp.ok) {
                            showAlert(t('browser.failedGenPom'), 'error');
                        }
                    } catch (err) {
                        console.error('POM generation failed', err);
                    }
                } else {
                    showAlert(t('browser.fillPomFields'), 'error');
                }
            }

            pendingFiles = [];
            uploading = false;
            setUploadBusy(false);
            renderPendingFiles();
            loadDirectory(currentPath);
        });
    }
}

/**
 * Enable/disable upload UI while a batch upload is in progress.
 * @param {boolean} busy
 * @returns {void}
 */
function setUploadBusy(busy) {
    const uploadBtn = document.getElementById('upload-btn');
    const clearBtn = document.getElementById('upload-clear-btn');
    const uploadZone = document.getElementById('upload-zone');
    if (uploadBtn) {
        uploadBtn.disabled = busy;
        uploadBtn.classList.toggle('is-busy', busy);
        const label = uploadBtn.querySelector('.upload-btn-label');
        if (label) label.textContent = busy ? t('browser.uploading') : t('browser.uploadFilesBtn');
    }
    if (clearBtn) clearBtn.disabled = busy;
    if (uploadZone) uploadZone.classList.toggle('is-busy', busy);
}

/**
 * Append files to the pending queue and re-render the controls list.
 * @param {FileList|File[]} files
 * @returns {void}
 */
function addPendingFiles(files) {
    for (let i = 0; i < files.length; i++) {
        const file = files[i];
        file._isNew = true;
        if (!file._id) {
            file._id = 'file_' + Math.random().toString(36).substring(2, 10) + '_' + Date.now() + '_' + i;
        }
        pendingFiles.push(file);
    }
    renderPendingFiles();
}

/**
 * Reconcile the pending-files list UI with `pendingFiles` and open controls.
 * @returns {void}
 */
function renderPendingFiles() {
    const uploadFileList = document.getElementById('upload-file-list');
    const fileCount = document.getElementById('upload-file-count');
    const totalSizeEl = document.getElementById('upload-total-size');
    if (!uploadFileList) return;

    if (pendingFiles.length === 0) {
        setUploadControlsOpen(false);
        return;
    }

    if (fileCount) {
        fileCount.textContent = t('browser.filesSelectedCount', {count: pendingFiles.length});
    }

    const totalBytes = pendingFiles.reduce((acc, f) => acc + f.size, 0);
    if (totalSizeEl) {
        totalSizeEl.textContent = formatBytes(totalBytes);
        totalSizeEl.style.display = 'inline-flex';
    }

    if (pendingFiles.length > 1) {
        uploadFileList.classList.add('has-multiple-files');
    } else {
        uploadFileList.classList.remove('has-multiple-files');
    }

    if (pendingFiles.length > 4) {
        uploadFileList.classList.add('has-many-files');
    } else {
        uploadFileList.classList.remove('has-many-files');
    }

    const currentMap = new Map();
    Array.from(uploadFileList.children).forEach(child => {
        if (child.dataset && child.dataset.fileId) {
            currentMap.set(child.dataset.fileId, child);
        }
    });

    const pendingIds = new Set(pendingFiles.map(f => {
        if (!f._id) {
            f._id = 'file_' + Math.random().toString(36).substring(2, 10) + '_' + Date.now();
        }
        return f._id;
    }));

    currentMap.forEach((el, id) => {
        if (!pendingIds.has(id)) {
            el.remove();
        }
    });

    /**
     * Return the n-th child that is not mid leave-animation, or null.
     * @param {number} n
     * @returns {Element|null}
     */
    function getActiveChild(n) {
        let count = 0;
        for (let i = 0; i < uploadFileList.children.length; i++) {
            const child = uploadFileList.children[i];
            if (!child.classList.contains('is-leaving')) {
                if (count === n) return child;
                count++;
            }
        }
        return null;
    }

    pendingFiles.forEach((file, index) => {
        const isNew = file._isNew === true;
        let entry = currentMap.get(file._id);

        const onRemoveHandler = () => {
            if (uploading) return;
            const currIdx = pendingFiles.indexOf(file);
            if (currIdx === -1) return;
            const entryEl = document.getElementById('upload-entry-' + currIdx);
            if (entryEl) {
                const currentHeight = entryEl.offsetHeight;
                entryEl.style.height = currentHeight + 'px';
                entryEl.style.setProperty('--entry-height', currentHeight + 'px');

                requestAnimationFrame(() => {
                    requestAnimationFrame(() => {
                        entryEl.style.transition =
                            'height 0.3s cubic-bezier(0.4, 0, 0.2, 1),' +
                            'margin 0.3s cubic-bezier(0.4, 0, 0.2, 1),' +
                            'padding 0.3s cubic-bezier(0.4, 0, 0.2, 1),' +
                            'border-width 0.3s cubic-bezier(0.4, 0, 0.2, 1)';
                        entryEl.style.height = '0';
                        entryEl.style.marginTop = '0';
                        entryEl.style.marginBottom = '-0.65rem';
                        entryEl.style.paddingTop = '0';
                        entryEl.style.paddingBottom = '0';
                        entryEl.style.borderWidth = '0';
                        entryEl.classList.add('is-leaving');
                    });
                });

                let cleaned = false;
                const cleanup = () => {
                    if (cleaned) return;
                    cleaned = true;
                    if (entryEl.parentNode) entryEl.remove();
                    pendingFiles.splice(currIdx, 1);
                    if (pendingFiles.length <= 1) {
                        uploadFileList.scrollTop = 0;
                    }
                    renderPendingFiles();
                };
                entryEl.addEventListener('transitionend', function onEnd(e) {
                    if (e.propertyName !== 'height') return;
                    entryEl.removeEventListener('transitionend', onEnd);
                    cleanup();
                });
                setTimeout(cleanup, 380);
            } else {
                pendingFiles.splice(currIdx, 1);
                if (pendingFiles.length <= 1) {
                    uploadFileList.scrollTop = 0;
                }
                renderPendingFiles();
            }
        };

        if (entry && !entry.classList.contains('is-leaving')) {
            entry.setAttribute('index', String(index));
            entry.setAttribute('total', String(pendingFiles.length));
            entry.setAttribute('status', file._status || '');
            entry.setAttribute('progress', file._progress || '');
            entry.id = 'upload-entry-' + index;
            entry.style.setProperty('--item-index', String(index));
            entry._onRemove = onRemoveHandler;

            const targetChild = getActiveChild(index);
            if (targetChild !== entry) {
                uploadFileList.insertBefore(entry, targetChild || null);
            }
        } else {
            entry = createUploadEntry({
                filename: file.name,
                filesize: formatBytes(file.size),
                ext: fileExt(file.name),
                index: index,
                total: pendingFiles.length,
                status: file._status || '',
                progress: file._progress || '',
                onRemove: () => {
                    if (entry._onRemove) entry._onRemove();
                    else onRemoveHandler();
                }
            });
            entry._onRemove = onRemoveHandler;
            entry.dataset.fileId = file._id;
            entry.id = 'upload-entry-' + index;
            entry.style.setProperty('--item-index', String(index));

            if (isNew) {
                entry.classList.add('is-entering');
                delete file._isNew;
            }

            const targetChild = getActiveChild(index);
            if (targetChild) {
                uploadFileList.insertBefore(entry, targetChild);
            } else {
                uploadFileList.appendChild(entry);
            }

            if (isNew) {
                setTimeout(() => {
                    if (entry && entry.classList) {
                        entry.classList.remove('is-entering');
                    }
                }, 300 + index * 20);
            }
        }
    });

    const hasLeaving = uploadFileList.querySelector('.upload-file-entry.is-leaving') !== null;
    if (!hasLeaving) {
        uploadFileList.classList.remove('is-deleting');
    }

    setUploadControlsOpen(true);
    updateUploadSpaceCheck();
}

/**
 * Estimate disk space required for pending uploads (includes overhead).
 * @param {Array<{name: string, size: number}>} files
 * @returns {number} bytes
 */
function calculatePendingRequiredSpace(files) {
    let total = 0;
    for (const f of files) {
        const isJavadoc = f.name.toLowerCase().endsWith('-javadoc.jar');
        total += f.size * (isJavadoc ? 4 : 2) + 1024 * 1024;
    }
    return total;
}

/**
 * Check free disk space against pending files; update warning UI and disable upload if insufficient.
 * @returns {Promise<boolean>} true when there is not enough free space
 */
async function updateUploadSpaceCheck() {
    const uploadBtn = document.getElementById('upload-btn');
    const uploadZone = document.getElementById('upload-zone');
    let warningEl = document.getElementById('upload-space-warning');

    if (pendingFiles.length === 0) {
        if (warningEl) warningEl.style.display = 'none';
        return false;
    }

    try {
        const {response: resp, data} = await fetchProto('/api/status/instance', InstanceStatus);
        if (!resp.ok || !data) return false;
        const diskUsed = data.disk_used !== undefined ? data.disk_used : data.diskUsed;
        const diskTotal = data.disk_total !== undefined ? data.disk_total : data.diskTotal;

        if (diskUsed !== undefined && diskTotal !== undefined) {
            const freeSpace = Math.max(0, Number(diskTotal) - Number(diskUsed));
            const requiredSpace = calculatePendingRequiredSpace(pendingFiles);

            if (freeSpace < requiredSpace || freeSpace <= 0) {
                if (!warningEl) {
                    warningEl = document.createElement('div');
                    warningEl.id = 'upload-space-warning';
                    warningEl.className = 'upload-space-warning';
                    warningEl.style.color = '#ef4444';
                    warningEl.style.fontSize = '0.85rem';
                    warningEl.style.marginTop = '8px';
                    warningEl.style.fontWeight = '500';
                    const controlsCard = document.querySelector('.upload-controls-card');
                    if (controlsCard) controlsCard.appendChild(warningEl);
                }
                warningEl.textContent = t('browser.insufficientDiskSpace');
                warningEl.style.display = 'block';

                if (uploadBtn) {
                    uploadBtn.disabled = true;
                    uploadBtn.classList.add('is-disabled');
                }
                if (uploadZone) {
                    uploadZone.classList.add('is-disabled');
                }
                return true;
            } else {
                if (warningEl) warningEl.style.display = 'none';
                if (uploadBtn && !uploading) {
                    uploadBtn.disabled = false;
                    uploadBtn.classList.remove('is-disabled');
                }
                if (uploadZone) {
                    uploadZone.classList.remove('is-disabled');
                }
            }
        }
    } catch (err) {
        console.error('Failed to check disk space', err);
    }
    return false;
}

window.addEventListener('languageChanged', () => {
    if (pendingFiles.length > 0) {
        renderPendingFiles();
    }
});

