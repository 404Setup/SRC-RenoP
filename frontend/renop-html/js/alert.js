/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {t, translateError} from './i18n.js';
import {el} from './cfg-ui.js';
import {createAlert, createFileCard, createDropzone, createIcon, createMetaGrid, RenopDialog} from './components.js';
import {fetchProto} from './api.js';
import {
    shouldUseChunkedUpload,
    uploadFileChunked,
    uploadUpdaterSingle,
    renderChunkProgressStrip,
} from './chunked-upload.js';
import {InstanceStatus} from './proto/index.js';

/**
 * Show a transient toast alert that auto-dismisses after 5 seconds.
 * @param {string} message - Message text (passed through translateError).
 * @param {'info'|'success'|'error'|'warning'|string} [type='info'] - Visual alert type.
 * @returns {void}
 */
export function showAlert(message, type = 'info') {
    let alertContainer = document.getElementById('renop-alert-container');
    if (!alertContainer) {
        alertContainer = document.createElement('div');
        alertContainer.id = 'renop-alert-container';
        document.body.appendChild(alertContainer);
    }

    const translatedMessage = translateError(message);
    const alertEl = createAlert(translatedMessage, type);

    alertContainer.appendChild(alertEl);

    setTimeout(() => {
        if (alertEl && typeof alertEl.dismiss === 'function') {
            alertEl.dismiss();
        }
    }, 5000);
}

window.showAlert = showAlert;

/**
 * Show a confirm dialog and resolve with the user's choice.
 * @param {string} message - Body message (passed through translateError).
 * @param {{ title?: string, confirmText?: string, cancelText?: string, danger?: boolean }} [options]
 * @returns {Promise<boolean>} True if confirmed, false if cancelled.
 */
export function showConfirm(message, options = {}) {
    const rawTitle = options.title || t('confirm.title');
    const rawConfirm = options.confirmText || t('confirm.confirmBtn');
    const rawCancel = options.cancelText || t('confirm.cancelBtn');
    const danger = options.danger !== false;

    const msgSpan = el('p', {class: 'modal-message'}, translateError(message));

    return RenopDialog.show({
        id: 'renop-confirm-container',
        size: 'sm',
        title: rawTitle,
        body: msgSpan,
        footer: [
            {text: rawCancel, className: 'action-btn', onClick: (e, d) => d.close(false)},
            {
                text: rawConfirm,
                className: danger ? 'action-btn primary-btn btn-danger' : 'action-btn primary-btn',
                onClick: (e, d) => d.close(true)
            }
        ]
    });
}

window.showConfirm = showConfirm;

/**
 * Show a prompt dialog with a text input; optionally read-only with click-to-copy.
 * @param {string} message - Body message (passed through translateError).
 * @param {string} [defaultValue=''] - Initial input value.
 * @param {boolean} [readOnly=false] - When true, input is read-only and clickable to copy.
 * @param {{ title?: string, cancelText?: string, okText?: string }} [options={}] - Dialog labels.
 * @returns {Promise<string|null>} Input value on OK, or null if cancelled.
 */
export function showPrompt(message, defaultValue = '', readOnly = false, options = {}) {
    return new Promise((resolve) => {
        const msgSpan = el('p', {class: 'modal-message modal-message--spaced'}, translateError(message));
        const input = el('input', {
            type: 'text',
            autocomplete: 'off',
            value: defaultValue,
            class: readOnly ? 'settings-input modal-input is-readonly' : 'settings-input modal-input'
        });
        if (readOnly) {
            input.readOnly = true;
            input.title = t('prompt.clickToCopy');
            input.onclick = async () => {
                input.select();
                try {
                    await navigator.clipboard.writeText(input.value);
                    showAlert(t('prompt.copied'), 'success');
                } catch (e) {
                }
            };
        }

        const rawTitle = options.title || t('prompt.title');
        const rawCancel = options.cancelText || t('common.cancel');
        const rawOk = options.okText || t('common.ok');

        RenopDialog.show({
            id: 'renop-prompt-container',
            size: 'sm',
            title: rawTitle,
            body: [msgSpan, input],
            footer: [
                {text: rawCancel, className: 'action-btn', onClick: (e, d) => d.close(null)},
                {text: rawOk, className: 'action-btn primary-btn', onClick: (e, d) => d.close(input.value)}
            ],
            onClose: (res) => resolve(res)
        });

        setTimeout(() => {
            input.focus();
            input.addEventListener('keydown', (e) => {
                if (e.key === 'Enter') {
                    const dialog = document.getElementById('renop-prompt-container');
                    if (dialog && dialog.close) dialog.close(input.value);
                }
            });
        }, 10);
    });
}

window.showPrompt = showPrompt;

/**
 * Format a byte count as a human-readable size string (B, KB, MB, GB, TB).
 * @param {number} bytes - Size in bytes.
 * @param {number} [decimals=2] - Fractional digits (clamped to >= 0).
 * @returns {string} Formatted size, e.g. "1.5 MB".
 */
export function formatBytes(bytes, decimals = 2) {
    if (bytes === 0 || !bytes) return '0 B';
    const k = 1024;
    const dm = decimals < 0 ? 0 : decimals;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(dm)) + ' ' + sizes[i];
}

/**
 * Escape HTML special characters in a string for safe text insertion.
 * @param {*} str - Value to escape (coerced to string).
 * @returns {string} Escaped string, or empty string if falsy.
 */
function escapeHtml(str) {
    if (!str) return '';
    return String(str)
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#039;');
}

/**
 * Displays a detailed update confirmation modal showing version, file size, release date, channel/commit, and release notes.
 * @param {{
 *   latest_version: string,
 *   current_version?: string,
 *   size?: number,
 *   release_date?: string,
 *   release_notes?: string,
 *   channel?: string,
 *   commit_sha?: string,
 *   is_release?: boolean
 * }} updateData
 * @returns {Promise<boolean>}
 */
export function showUpdateModal(updateData = {}) {
    const formattedSize = updateData.size && updateData.size > 0 ? formatBytes(updateData.size) : t('common.unknownSize');
    const estDisk = updateData.estimated_disk_space || (updateData.size && updateData.size > 0 ? updateData.size * 3 : 0);
    const formattedEstDisk = estDisk > 0 ? formatBytes(estDisk) : t('common.unknownSize');
    let formattedDate = t('common.unknown');
    if (updateData.release_date) {
        try {
            formattedDate = new Date(updateData.release_date).toLocaleString();
        } catch (e) {
            formattedDate = updateData.release_date;
        }
    }

    const channelLabel = updateData.channel === 'nightly' ? t('settings.channelNightly') : t('settings.channelRelease');
    const commitText = updateData.commit_sha ? (updateData.commit_sha.length > 7 ? updateData.commit_sha.substring(0, 7) : updateData.commit_sha) : '';

    const metaGridItems = [
        {label: t('dashboard.version'), value: updateData.latest_version || '-'},
        {label: t('updater.releaseDate'), value: formattedDate, colon: false},
        {label: t('updater.fileSize'), value: formattedSize, colon: false},
        {label: t('updater.estimatedDiskSpace'), value: formattedEstDisk, colon: false},
        {label: t('settings.updateChannel'), value: channelLabel}
    ];
    if (commitText) {
        metaGridItems.push({label: 'Commit', value: commitText, isCode: true});
    }
    const metaGrid = createMetaGrid(metaGridItems);

    const notesContainer = el('div', {class: 'modal-notes-container'},
        el('label', {class: 'modal-notes-label'}, t('updater.releaseNotes')),
        el('div', {class: 'modal-notes-box'}, updateData.release_notes || t('updater.noReleaseNotes'))
    );

    return RenopDialog.show({
        id: 'renop-update-modal-container',
        size: 'md',
        centered: true,
        title: t('updater.foundNewVersion', {version: updateData.latest_version || ''}),
        bodyClass: 'modal-body modal-body-flex',
        body: [metaGrid, notesContainer],
        footer: [
            {text: t('common.cancel'), className: 'action-btn', onClick: (e, d) => d.close(false)},
            {
                text: t('updater.downloadAndInstall'),
                className: 'action-btn primary-btn',
                onClick: (e, d) => d.close(true)
            }
        ]
    });
}

window.showUpdateModal = showUpdateModal;

/**
 * Displays an offline update modal allowing the user to select/drag a .zip package and upload & install it.
 * @returns {Promise<boolean>}
 */
export function showOfflineUpdateModal() {
    return new Promise((resolve) => {
        let selectedFile = null;

        const getBtn = () => {
            const dlg = document.getElementById('renop-offline-modal-container');
            return dlg ? dlg.querySelector('.primary-btn') : null;
        };

        const disableBtn = () => {
            const btn = getBtn();
            if (btn) { btn.disabled = true; }
        };

        const enableBtn = () => {
            const btn = getBtn();
            if (btn) { btn.disabled = false; }
        };

        let spaceCheckPassed = false;

        const dropzone = createDropzone({
            title: t('updater.dropzoneTitle'),
            hint: t('updater.dropzoneHint'),
            accept: '.zip',
            onSelect: (file) => updateFileDisplay(file)
        });

        const fileCard = el('div', {class: 'file-info-card', style: {display: 'none'}});
        const spaceWarning = el('div', {
            class: 'upload-space-warning',
            style: {display: 'none', color: '#ef4444', fontSize: '0.85rem', marginTop: '8px', fontWeight: '500'}
        });
        const dropzoneContainer = el('div', {class: 'dropzone-container'}, dropzone, fileCard, spaceWarning);

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

        /**
         * Checks disk space for the given file. Returns true if enough space, false otherwise.
         * Also updates spaceWarning element.
         */
        const checkDiskSpace = async (file) => {
            const requiredSpace = file.size * 3;
            try {
                const {response: resp, data} = await fetchProto('/api/status/instance', InstanceStatus);
                if (!resp.ok || !data) {
                    spaceWarning.textContent = t('updater.insufficientDiskSpace');
                    spaceWarning.style.display = 'block';
                    return false;
                }
                const diskUsed = data.disk_used !== undefined ? data.disk_used : data.diskUsed;
                const diskTotal = data.disk_total !== undefined ? data.disk_total : data.diskTotal;

                if (diskUsed === undefined || diskTotal === undefined || Number(diskTotal) <= 0) {
                    spaceWarning.textContent = t('updater.insufficientDiskSpace');
                    spaceWarning.style.display = 'block';
                    return false;
                }

                const freeSpace = Math.max(0, Number(diskTotal) - Number(diskUsed));
                if (freeSpace < requiredSpace || freeSpace <= 0) {
                    showAlert(t('updater.insufficientDiskSpace'), 'error');
                    spaceWarning.textContent = `${t('updater.insufficientDiskSpace')} (${t('updater.estimatedDiskSpace')} ${formatBytes(requiredSpace)}, Free: ${formatBytes(freeSpace)})`;
                    spaceWarning.style.display = 'block';
                    return false;
                }

                spaceWarning.style.display = 'none';
                return true;
            } catch (err) {
                console.error('Failed to check disk space', err);
                spaceWarning.textContent = t('updater.insufficientDiskSpace');
                spaceWarning.style.display = 'block';
                return false;
            }
        };

        const updateFileDisplay = async (file) => {
            spaceCheckPassed = false;
            disableBtn();

            if (!file) {
                selectedFile = null;
                fileCard.style.display = 'none';
                spaceWarning.style.display = 'none';
                dropzone.style.display = 'flex';
                return;
            }

            if (!file.name.toLowerCase().endsWith('.zip')) {
                showAlert(t('updater.invalidZip'), 'error');
                return;
            }

            selectedFile = file;
            const requiredSpace = file.size * 3;
            dropzone.style.display = 'none';
            fileCard.style.display = 'flex';
            fileCard.innerHTML = '';

            const card = createFileCard(file.name, formatBytes(file.size), {
                icon: 'box',
                onRemove: () => updateFileDisplay(null)
            });
            const predInfo = el('div', {
                class: 'file-space-prediction',
                style: {marginTop: '4px', fontSize: '0.8rem', opacity: '0.8'}
            }, `${t('updater.estimatedDiskSpace')} ${formatBytes(requiredSpace)}`);
            card.appendChild(predInfo);
            fileCard.appendChild(card);

            spaceCheckPassed = await checkDiskSpace(file);
            if (spaceCheckPassed) {
                enableBtn();
            } else {
                disableBtn();
            }
        };

        const btnOkConfig = {
            text: t('updater.confirmUploadAndInstall'),
            className: 'action-btn primary-btn',
            disabled: true,
            onClick: async (e, dialog) => {
                if (!selectedFile) {
                    showAlert(t('updater.selectZipFirst'), 'error');
                    return;
                }

                if (!selectedFile.name.toLowerCase().endsWith('.zip')) {
                    showAlert(t('updater.invalidZip'), 'error');
                    return;
                }

                const ok = await checkDiskSpace(selectedFile);
                if (!ok) {
                    showAlert(t('updater.insufficientDiskSpace'), 'error');
                    disableBtn();
                    spaceCheckPassed = false;
                    return;
                }

                const btnOk = dialog.querySelector('.primary-btn');
                const btnCancel = dialog.querySelector('.action-btn:not(.primary-btn)');
                const closeBtn = dialog.querySelector('.close-btn');

                if (btnOk) btnOk.disabled = true;
                if (btnCancel) btnCancel.disabled = true;
                if (closeBtn) closeBtn.disabled = true;

                progressContainer.style.display = 'flex';
                progressMsg.textContent = t('updater.uploadingProgress', {progress: 0});
                progressFill.style.width = '0%';

                const resetProgressUI = () => {
                    progressContainer.style.display = 'none';
                    progressFill.style.width = '0%';
                    progressMsg.textContent = '';
                    if (btnOk) btnOk.disabled = false;
                    if (btnCancel) btnCancel.disabled = false;
                    if (closeBtn) closeBtn.disabled = false;
                };

                try {
                    const applyProgress = (loaded, total) => {
                        if (!(total > 0)) return;
                        const pct = Math.round((loaded / total) * 100);
                        progressFill.style.width = `${pct}%`;
                        progressMsg.textContent = pct < 100
                            ? t('updater.uploadingProgress', {progress: pct})
                            : t('updater.extracting');
                    };

                    const applyChunkProgress = (chunks) => {
                        const strip = renderChunkProgressStrip(progressContainer, chunks);
                        if (strip) strip.classList.add('upload-chunk-strip--modal');
                    };

                    let result;
                    if (shouldUseChunkedUpload(selectedFile)) {
                        result = await uploadFileChunked(selectedFile, {
                            purpose: 'updater',
                            onProgress: applyProgress,
                            onChunkProgress: applyChunkProgress,
                        });
                    } else {
                        result = await uploadUpdaterSingle(selectedFile, applyProgress);
                    }

                    if (result.ok) {
                        progressFill.style.width = '100%';
                        showAlert(t('updater.uploadSuccess'), 'success');
                        dialog.close(true);
                        if (window.fetchUpdaterStatusQuietly) {
                            await window.fetchUpdaterStatusQuietly();
                        }
                    } else {
                        let errText = result.status ? `HTTP ${result.status}` : t('common.unknown');
                        if (result.body && result.body.error) {
                            errText = result.body.error;
                        } else if (result.responseText) {
                            try {
                                const errJson = JSON.parse(result.responseText);
                                if (errJson.error) errText = errJson.error;
                            } catch (ex) { /* ignore */ }
                        }
                        showAlert(errText, 'error');
                        resetProgressUI();
                    }
                } catch (err) {
                    showAlert(err.message || t('common.unknown'), 'error');
                    resetProgressUI();
                }
            }
        };

        RenopDialog.show({
            id: 'renop-offline-modal-container',
            size: 'md',
            centered: true,
            title: t('updater.offlineTitle'),
            subtitle: t('updater.offlineSubtitle'),
            bodyClass: 'modal-body modal-body-flex',
            body: [dropzoneContainer, progressContainer],
            footer: [
                {text: t('common.cancel'), className: 'action-btn', onClick: (e, d) => d.close(false)},
                btnOkConfig
            ],
            onClose: (res) => resolve(res)
        });
    });
}

window.showOfflineUpdateModal = showOfflineUpdateModal;
