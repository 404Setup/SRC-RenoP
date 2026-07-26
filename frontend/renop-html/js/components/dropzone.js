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
import {el} from '../cfg-ui.js';
import {createIcon} from './icon.js';

/**
 * Drag-and-drop file picker custom element.
 */
export class RenopDropzone extends HTMLElement {
    /**
     * @returns {string[]}
     */
    static get observedAttributes() {
        return ['title', 'hint', 'accept'];
    }

    /**
     * Initialize interaction handlers and render.
     * @returns {void}
     */
    connectedCallback() {
        if (!this._initialized) {
            this.setupComponent();
            this._initialized = true;
        }
        this.render();
    }

    /**
     * Re-render when observed attributes change.
     * @param {string} name - Attribute name.
     * @param {string|null} oldValue - Previous value.
     * @param {string|null} newValue - New value.
     * @returns {void}
     */
    attributeChangedCallback(name, oldValue, newValue) {
        if (oldValue !== newValue) {
            this.render();
        }
    }

    /**
     * Wire file input, click/keyboard, and drag-and-drop handlers.
     * @returns {void}
     */
    setupComponent() {
        this._fileInput = document.createElement('input');
        this._fileInput.type = 'file';
        this._fileInput.style.display = 'none';

        this._fileInput.addEventListener('change', () => {
            if (this._fileInput.files && this._fileInput.files.length > 0) {
                const file = this._fileInput.files[0];
                this.dispatchEvent(new CustomEvent('fileselect', {
                    detail: {file, files: this._fileInput.files},
                    bubbles: true
                }));
                this._fileInput.value = '';
            }
        });

        this.addEventListener('click', (e) => {
            if (e.target.closest('button, input, a')) return;
            this._fileInput.click();
        });

        this.addEventListener('keydown', (e) => {
            if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                this._fileInput.click();
            }
        });

        this.addEventListener('dragover', (e) => {
            e.preventDefault();
            e.stopPropagation();
            this.classList.add('dropzone--active');
            if (e.dataTransfer) {
                e.dataTransfer.dropEffect = 'copy';
            }
        });

        this.addEventListener('dragenter', (e) => {
            e.preventDefault();
            e.stopPropagation();
            this.classList.add('dropzone--active');
        });

        this.addEventListener('dragleave', (e) => {
            e.preventDefault();
            e.stopPropagation();
            if (!this.contains(e.relatedTarget)) {
                this.classList.remove('dropzone--active');
            }
        });

        this.addEventListener('drop', (e) => {
            e.preventDefault();
            e.stopPropagation();
            this.classList.remove('dropzone--active');
            if (e.dataTransfer && e.dataTransfer.files && e.dataTransfer.files.length > 0) {
                const file = e.dataTransfer.files[0];
                this.dispatchEvent(new CustomEvent('fileselect', {
                    detail: {file, files: e.dataTransfer.files},
                    bubbles: true
                }));
            }
        });

        if (!this.getAttribute('tabindex')) {
            this.setAttribute('tabindex', '0');
        }
        if (!this.getAttribute('role')) {
            this.setAttribute('role', 'button');
        }
    }

    /**
     * Rebuild dropzone UI from attributes.
     * @returns {void}
     */
    render() {
        if (!this._initialized) {
            this.setupComponent();
            this._initialized = true;
        }

        const titleText = this.getAttribute('title') || t('updater.dropzoneTitle') || 'Drop file here';
        const hintText = this.getAttribute('hint') || t('updater.dropzoneHint') || '';
        const acceptText = this.getAttribute('accept') || '';

        this.className = 'dropzone';

        if (this._fileInput) {
            if (acceptText) {
                this._fileInput.setAttribute('accept', acceptText);
            } else {
                this._fileInput.removeAttribute('accept');
            }
        }

        const iconEl = createIcon('upload', {class: 'dropzone-icon'});
        const titleEl = el('div', {class: 'dropzone-title'}, titleText);
        const hintEl = hintText ? el('div', {class: 'dropzone-hint'}, hintText) : null;

        this.innerHTML = '';
        if (this._fileInput) {
            this.appendChild(this._fileInput);
        }
        this.appendChild(iconEl);
        this.appendChild(titleEl);
        if (hintEl) {
            this.appendChild(hintEl);
        }
    }
}

if (!customElements.get('renop-dropzone')) {
    customElements.define('renop-dropzone', RenopDropzone);
}

/**
 * Create a file dropzone element.
 * @param {object} [options={}] - Dropzone configuration.
 * @param {string} [options.title] - Primary dropzone title.
 * @param {string} [options.hint] - Secondary hint text.
 * @param {string} [options.accept] - File input accept attribute.
 * @param {function(File, FileList): void} [options.onSelect] - Called when a file is selected.
 * @returns {HTMLElement}
 */
export function createDropzone({title, hint, accept, onSelect} = {}) {
    const dz = document.createElement('renop-dropzone');
    if (title) dz.setAttribute('title', title);
    if (hint) dz.setAttribute('hint', hint);
    if (accept) dz.setAttribute('accept', accept);
    if (onSelect) {
        dz.addEventListener('fileselect', e => onSelect(e.detail.file, e.detail.files));
    }
    return dz;
}
