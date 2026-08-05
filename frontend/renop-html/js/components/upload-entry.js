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
import {el} from '@renop/ui/dom';
import {createIcon} from './icon.js';
import {getCategoryIconName, getFileTypeCategory, getFileTypeInfo} from './file-item.js';

/**
 * Upload queue entry custom element with progress and chunk strip support.
 */
export class RenopUploadEntry extends HTMLElement {
    /**
     * @returns {string[]}
     */
    static get observedAttributes() {
        return ['filename', 'filesize', 'ext', 'progress', 'index', 'total', 'status'];
    }

    /**
     * Render when inserted into the DOM.
     * @returns {void}
     */
    connectedCallback() {
        this.render();
    }

    /**
     * Incrementally update progress/status or fully re-render as needed.
     * @param {string} name - Attribute name.
     * @param {string|null} oldValue - Previous value.
     * @param {string|null} newValue - New value.
     * @returns {void}
     */
    attributeChangedCallback(name, oldValue, newValue) {
        if (oldValue === newValue) return;
        if (name === 'index' && oldValue !== null && oldValue !== newValue) {
            this._indexUpdated = true;
        }
        if (name === 'progress' && this._rootBuilt) {
            this._syncProgressOnly();
            return;
        }
        if (name === 'status' && this._rootBuilt && oldValue) {
            this._syncStatusClasses();
            this._syncMetaStatus();
            this._syncProgressOnly();
            return;
        }
        this.render();
    }

    /**
     * Sync host status CSS classes without a full re-render.
     * @returns {void}
     */
    _syncStatusClasses() {
        const status = this.getAttribute('status') || '';
        this.classList.remove('is-uploading', 'is-done', 'is-error');
        if (status) this.classList.add('is-' + status);
    }

    /**
     * Refresh status/progress text in the meta row without rebuilding the card.
     * @returns {void}
     */
    _syncMetaStatus() {
        const metaDiv = this.querySelector('.upload-file-meta');
        if (!metaDiv) return;
        const status = this.getAttribute('status') || '';
        const progress = this.getAttribute('progress') || '';
        const total = parseInt(this.getAttribute('total') || '0', 10);

        metaDiv.querySelectorAll(
            '.upload-file-status, .upload-file-progress-text',
        ).forEach((n) => n.remove());

        if (status === 'done') {
            metaDiv.appendChild(el('span', {class: 'upload-file-status upload-file-status--done'},
                createIcon('check', {width: '12', height: '12'}),
                el('span', {}, t('common.done') || 'Done')
            ));
        } else if (status === 'error') {
            metaDiv.appendChild(el('span', {class: 'upload-file-status upload-file-status--error'},
                createIcon('alertCircle', {width: '12', height: '12'}),
                el('span', {}, t('common.error') || 'Error')
            ));
        } else if (status === 'uploading') {
            metaDiv.appendChild(el('span', {class: 'upload-file-progress-text'}, progress || '0%'));
        } else if (total > 1 && !status) {
            metaDiv.appendChild(el('span', {class: 'upload-file-status upload-file-status--pending'},
                createIcon('clock', {width: '11', height: '11'}),
                el('span', {}, t('browser.queued') || 'Queued')
            ));
        }
    }

    /**
     * Update only the progress bar and percentage text.
     * @returns {void}
     */
    _syncProgressOnly() {
        const progress = this.getAttribute('progress') || '';
        const status = this.getAttribute('status') || '';
        const pct = parseInt(String(progress).replace('%', ''), 10);
        const hasPct = !Number.isNaN(pct);

        let progressText = this.querySelector('.upload-file-progress-text');
        if (status === 'uploading') {
            if (!progressText) {
                const metaDiv = this.querySelector('.upload-file-meta');
                if (metaDiv) {
                    progressText = el('span', {class: 'upload-file-progress-text'}, progress || '0%');
                    metaDiv.appendChild(progressText);
                }
            } else {
                progressText.textContent = progress || '0%';
            }
        }

        let bar = this.querySelector('.upload-progress-bar');
        if (status === 'uploading' || status === 'done') {
            if (!bar) {
                bar = document.createElement('div');
                bar.className = 'upload-progress-bar';
                const fill = document.createElement('div');
                fill.className = 'upload-progress-fill';
                bar.appendChild(fill);
                this.appendChild(bar);
            }
            const fill = bar.querySelector('.upload-progress-fill');
            if (fill && hasPct) fill.style.width = Math.min(100, Math.max(0, pct)) + '%';
            if (status === 'done' && fill) fill.style.width = '100%';
        } else if (bar && status === 'error') {
        } else if (bar && !status) {
            bar.remove();
        }
    }

    /**
     * Update multi-chunk progress strip without full re-render.
     * @param {Array<{index:number, loaded:number, total:number, status:string, attempt?:number}>} chunks - Per-chunk progress states.
     * @returns {void}
     */
    setChunkStates(chunks) {
        this._chunkStates = chunks || [];
        let strip = this.querySelector('.upload-chunk-strip');
        if (!chunks || chunks.length === 0) {
            if (strip) strip.remove();
            return;
        }
        if (!strip) {
            strip = document.createElement('div');
            strip.className = 'upload-chunk-strip';
            strip.setAttribute('aria-label', 'Chunk upload progress');
            this.appendChild(strip);
        }
        if (strip.childElementCount !== chunks.length) {
            strip.innerHTML = '';
            for (let i = 0; i < chunks.length; i++) {
                const cell = document.createElement('div');
                cell.className = 'upload-chunk-cell is-pending';
                const fill = document.createElement('div');
                fill.className = 'upload-chunk-fill';
                cell.appendChild(fill);
                strip.appendChild(cell);
            }
        }
        chunks.forEach((c, i) => {
            const cell = strip.children[i];
            if (!cell) return;
            const pct = c.total > 0 ? Math.min(100, Math.round((c.loaded / c.total) * 100)) : 0;
            const fill = cell.querySelector('.upload-chunk-fill');
            if (fill) fill.style.width = pct + '%';
            cell.className = `upload-chunk-cell is-${c.status || 'pending'}`;
            const label = c.status === 'retrying'
                ? `#${i + 1} retry ${c.attempt || 1} · ${pct}%`
                : `#${i + 1} ${pct}%`;
            cell.title = label;
        });
    }

    /**
     * Rebuild the upload entry card from attributes.
     * @returns {void}
     */
    render() {
        const filename = this.getAttribute('filename') || '';
        const filesize = this.getAttribute('filesize') || '';
        const ext = this.getAttribute('ext') || '';
        const progress = this.getAttribute('progress') || '';
        const index = this.getAttribute('index');
        const total = parseInt(this.getAttribute('total') || '0', 10);
        const status = this.getAttribute('status') || '';

        const prevChunks = this._chunkStates;

        const category = getFileTypeCategory(ext, filename);
        const iconName = getCategoryIconName(category);

        const stateClasses = [];
        if (this.classList.contains('is-entering')) stateClasses.push('is-entering');
        if (this.classList.contains('is-leaving')) stateClasses.push('is-leaving');
        if (status) stateClasses.push('is-' + status);

        this.className = `upload-file-entry upload-file-entry--${category} ${stateClasses.join(' ')}`;
        this.innerHTML = '';

        const leftDiv = el('div', {class: 'upload-file-left'});
        const shouldShow = total > 1 && index !== null && index !== undefined;
        const isUpdated = this._indexUpdated === true;
        this._indexUpdated = false;

        const indexSpan = el('span', {
            class: `upload-file-index ${shouldShow ? 'is-visible' : ''} ${isUpdated ? 'is-updated' : ''}`
        }, `#${Number(index) + 1}`);

        if (isUpdated) {
            setTimeout(() => {
                if (indexSpan && indexSpan.classList) {
                    indexSpan.classList.remove('is-updated');
                }
            }, 300);
        }

        leftDiv.appendChild(indexSpan);

        const iconWrap = el('div', {
            class: `upload-file-icon upload-file-icon--${category}`,
            'aria-hidden': 'true'
        }, createIcon(iconName));
        leftDiv.appendChild(iconWrap);

        const info = el('div', {class: 'upload-file-info'});
        const nameDiv = el('div', {class: 'upload-file-name', title: filename}, filename);

        const metaDiv = el('div', {class: 'upload-file-meta'});
        metaDiv.appendChild(el('span', {class: 'upload-file-size'}, filesize));
        if (ext || filename) {
            const typeInfo = getFileTypeInfo(filename, ext);
            const typeLabel = t(typeInfo.key) || typeInfo.defaultLabel || ext.toUpperCase();
            if (ext || (typeInfo.key !== 'browser.file')) {
                const labelText = ext ? typeLabel : (typeInfo.key !== 'browser.file' ? typeLabel : '');
                if (labelText) {
                    metaDiv.appendChild(el('span', {class: `upload-file-ext upload-file-ext--${category}`}, labelText));
                }
            }
        }

        if (status === 'done') {
            metaDiv.appendChild(el('span', {class: 'upload-file-status upload-file-status--done'},
                createIcon('check', {width: '12', height: '12'}),
                el('span', {}, t('common.done') || 'Done')
            ));
        } else if (status === 'error') {
            metaDiv.appendChild(el('span', {class: 'upload-file-status upload-file-status--error'},
                createIcon('alertCircle', {width: '12', height: '12'}),
                el('span', {}, t('common.error') || 'Error')
            ));
        } else if (status === 'uploading') {
            metaDiv.appendChild(el('span', {class: 'upload-file-progress-text'}, progress || '0%'));
        } else if (total > 1 && !status) {
            metaDiv.appendChild(el('span', {class: 'upload-file-status upload-file-status--pending'},
                createIcon('clock', {width: '11', height: '11'}),
                el('span', {}, t('browser.queued') || 'Queued')
            ));
        }

        info.appendChild(nameDiv);
        info.appendChild(metaDiv);

        const removeBtn = el('button', {
            type: 'button',
            class: 'upload-file-remove',
            title: t('browser.removeFile') || 'Remove file',
            ariaLabel: `${t('browser.removeFile') || 'Remove'} ${filename}`
        }, createIcon('close', {width: '14', height: '14'}));

        removeBtn.onclick = (e) => {
            e.stopPropagation();
            this.dispatchEvent(new CustomEvent('remove', {bubbles: true}));
        };

        this.appendChild(leftDiv);
        this.appendChild(info);
        this.appendChild(removeBtn);

        if (status === 'uploading' || status === 'done') {
            const bar = document.createElement('div');
            bar.className = 'upload-progress-bar';
            const fill = document.createElement('div');
            fill.className = 'upload-progress-fill';
            const pct = parseInt(String(progress).replace('%', ''), 10);
            fill.style.width = (status === 'done' ? 100 : (Number.isNaN(pct) ? 0 : pct)) + '%';
            bar.appendChild(fill);
            this.appendChild(bar);
        }

        this._rootBuilt = true;
        if (prevChunks && prevChunks.length) {
            this.setChunkStates(prevChunks);
        }
    }
}

if (!customElements.get('renop-upload-entry')) {
    customElements.define('renop-upload-entry', RenopUploadEntry);
}

/**
 * Create an upload queue entry element.
 * @param {object} [options={}] - Entry configuration.
 * @param {string} options.filename - File name.
 * @param {string} options.filesize - Formatted size string.
 * @param {string} [options.ext] - File extension.
 * @param {string} [options.progress=''] - Progress text (e.g. "42%").
 * @param {number} [options.index] - Zero-based queue index.
 * @param {number} [options.total] - Total files in the queue.
 * @param {string} [options.status] - Status (uploading, done, error, or empty).
 * @param {Function} [options.onRemove] - Handler for the remove event.
 * @returns {HTMLElement}
 */
export function createUploadEntry({filename, filesize, ext, progress = '', index, total, status, onRemove} = {}) {
    const entry = document.createElement('renop-upload-entry');
    entry.setAttribute('filename', filename);
    entry.setAttribute('filesize', filesize);
    if (ext) entry.setAttribute('ext', ext);
    if (progress) entry.setAttribute('progress', progress);
    if (index !== undefined && index !== null) entry.setAttribute('index', String(index));
    if (total !== undefined && total !== null) entry.setAttribute('total', String(total));
    if (status) entry.setAttribute('status', status);
    if (onRemove) entry.addEventListener('remove', onRemove);
    return entry;
}
