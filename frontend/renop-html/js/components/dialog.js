/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {closeModalWithAnim} from '@renop/ui/modal';
import {t, translateError} from '../i18n.js';
import {el} from '../cfg-ui.js';
import {createIcon, ICONS} from './icon.js';

/**
 * Modal dialog custom element with form, footer, and promise-based open/close.
 */
export class RenopDialog extends HTMLElement {
    /**
     * Initialize internal dialog state.
     */
    constructor() {
        super();
        this._resolve = null;
        this._options = {};
        this._backdrop = null;
        this._modalContent = null;
        this._isClosing = false;
        this._handleEsc = null;
    }

    /**
     * Create or reuse a dialog, open it, and resolve when closed.
     * @param {object} [options={}] - Dialog configuration.
     * @param {string} [options.id] - Element id for reuse.
     * @param {string} [options.backdropId] - Backdrop element id.
     * @param {string} [options.className] - Extra classes for modal content.
     * @param {string} [options.size] - Size token (e.g. sm, lg) → modal-{size}.
     * @param {string} [options.maxWidth] - Max width CSS value.
     * @param {{id?: string, className?: string, onSubmit?: Function}} [options.form] - Wrap body in a form.
     * @param {boolean} [options.closable=true] - Show close button and allow backdrop/Escape close.
     * @param {string} [options.closeBtnId] - Close button id.
     * @param {string|Node} [options.title] - Header title.
     * @param {string|Node} [options.subtitle] - Header subtitle.
     * @param {string|Node} [options.icon] - Icon name, HTML string, or node.
     * @param {string} [options.headerClass] - Extra header classes.
     * @param {object} [options.headerStyle] - Header inline styles.
     * @param {string} [options.titleClass] - Title element class.
     * @param {object} [options.titleStyle] - Title inline styles.
     * @param {string} [options.titleId] - Title span id.
     * @param {object} [options.subtitleStyle] - Subtitle inline styles.
     * @param {boolean} [options.centered] - Center header/footer layout.
     * @param {string|Node|Array|Function} [options.body] - Body content or builder.
     * @param {string} [options.bodyClass] - Body class name.
     * @param {object} [options.bodyStyle] - Body inline styles.
     * @param {Array|Node|Function} [options.footer] - Footer buttons config, node, or builder.
     * @param {string} [options.footerClass] - Footer class name.
     * @param {object} [options.footerStyle] - Footer inline styles.
     * @param {boolean} [options.destroyOnClose=true] - Remove from DOM after close.
     * @param {Function} [options.onClose] - Called with close result.
     * @returns {Promise<*>} Resolves with the value passed to close().
     */
    static show(options = {}) {
        return new Promise((resolve) => {
            let dialog = options.id ? document.getElementById(options.id) : null;
            if (!dialog || !(dialog instanceof RenopDialog)) {
                dialog = document.createElement('renop-dialog');
                if (options.id) dialog.id = options.id;
                document.body.appendChild(dialog);
            }
            dialog._resolve = resolve;
            dialog._options = options;
            dialog.buildUI();
            dialog.open();
        });
    }

    /**
     * Build modal structure from the current options object.
     * @returns {void}
     */
    buildUI() {
        const opts = this._options;
        this.className = 'modal';
        this.style.display = 'none';
        this.innerHTML = '';
        this._isClosing = false;

        this._backdrop = el('div', {class: 'modal-backdrop'});
        if (opts.backdropId) this._backdrop.id = opts.backdropId;
        this.appendChild(this._backdrop);

        const modalClasses = ['modal-content'];
        if (opts.glass || opts.frosted) modalClasses.push('modal-glass');
        if (opts.className) modalClasses.push(...opts.className.split(' '));
        if (opts.size) modalClasses.push(`modal-${opts.size}`);

        const modalStyle = {};
        if (opts.maxWidth) modalStyle.maxWidth = opts.maxWidth;

        this._modalContent = el('div', {class: modalClasses.join(' '), style: modalStyle});

        let formOrWrapper = this._modalContent;
        if (opts.form) {
            const formProps = {
                action: 'javascript:void(0);',
                class: opts.form.className || 'token-form-layout'
            };
            if (opts.form.id) formProps.id = opts.form.id;

            const form = el('form', formProps);
            if (opts.form.onSubmit) {
                form.addEventListener('submit', (e) => opts.form.onSubmit(e, this));
            }
            this._modalContent.appendChild(form);
            formOrWrapper = form;
        }

        if (opts.closable !== false) {
            const closeBtnProps = {
                class: 'close-btn',
                ariaLabel: t('modal.close') || 'Close'
            };
            if (opts.closeBtnId) closeBtnProps.id = opts.closeBtnId;

            const closeBtn = el('button', closeBtnProps);
            closeBtn.appendChild(createIcon('close'));
            closeBtn.onclick = (e) => {
                e.preventDefault();
                e.stopPropagation();
                this.close(false);
            };
            this._modalContent.appendChild(closeBtn);
        }

        if (opts.title || opts.subtitle || opts.icon) {
            const headerClasses = ['modal-header'];
            if (opts.headerClass) headerClasses.push(...opts.headerClass.split(' '));
            if (opts.centered || opts.titleStyle?.justifyContent === 'center') headerClasses.push('modal-header--center');

            const header = el('div', {class: headerClasses.join(' ')});
            if (opts.headerStyle) Object.assign(header.style, opts.headerStyle);
            if (opts.closable === false) {
                header.style.paddingRight = '0';
            }

            const titleEl = el('h3', {class: opts.titleClass || 'modal-title token-form-title'});
            if (opts.titleStyle) Object.assign(titleEl.style, opts.titleStyle);

            if (opts.icon) {
                if (typeof opts.icon === 'string' && ICONS[opts.icon]) {
                    titleEl.appendChild(createIcon(opts.icon, {class: 'token-form-title-icon'}));
                } else if (typeof opts.icon === 'string') {
                    const iconSpan = el('span', {class: 'modal-title-icon'});
                    iconSpan.innerHTML = opts.icon;
                    titleEl.appendChild(iconSpan);
                } else if (opts.icon instanceof Node) {
                    titleEl.appendChild(opts.icon);
                }
            }

            if (typeof opts.title === 'string') {
                const titleSpan = el('span', {}, translateError(opts.title));
                if (opts.titleId) titleSpan.id = opts.titleId;
                titleEl.appendChild(titleSpan);
            } else if (opts.title instanceof Node) {
                titleEl.appendChild(opts.title);
            }

            header.appendChild(titleEl);

            if (opts.subtitle) {
                const subProps = {class: 'modal-subtitle'};
                if (opts.subtitleStyle) subProps.style = opts.subtitleStyle;
                const sub = el('p', subProps,
                    typeof opts.subtitle === 'string' ? translateError(opts.subtitle) : ''
                );
                if (opts.subtitle instanceof Node) sub.appendChild(opts.subtitle);
                header.appendChild(sub);
            }

            formOrWrapper.appendChild(header);
        }

        const bodyEl = el('div', {class: opts.bodyClass || 'modal-body'});
        if (opts.bodyStyle) Object.assign(bodyEl.style, opts.bodyStyle);
        if (typeof opts.body === 'string') {
            bodyEl.innerHTML = opts.body;
        } else if (opts.body instanceof Node) {
            bodyEl.appendChild(opts.body);
        } else if (Array.isArray(opts.body)) {
            opts.body.forEach(item => {
                if (item instanceof Node) bodyEl.appendChild(item);
                else if (typeof item === 'string') bodyEl.appendChild(document.createTextNode(item));
            });
        } else if (typeof opts.body === 'function') {
            opts.body(bodyEl);
        }
        formOrWrapper.appendChild(bodyEl);

        if (opts.footer) {
            const footerClasses = [opts.footerClass || 'modal-footer'];
            if (opts.centered || opts.footerStyle?.justifyContent === 'center') {
                footerClasses.push('modal-footer--center');
            }
            const footerEl = el('div', {class: footerClasses.join(' ')});
            if (opts.footerStyle) Object.assign(footerEl.style, opts.footerStyle);
            if (Array.isArray(opts.footer)) {
                opts.footer.forEach(btnConfig => {
                    const btnProps = {
                        type: btnConfig.type || 'button',
                        class: btnConfig.className || 'action-btn'
                    };
                    if (btnConfig.id) btnProps.id = btnConfig.id;
                    if (btnConfig.style) btnProps.style = btnConfig.style;
                    if (btnConfig.disabled) btnProps.disabled = true;

                    const btn = el('button', btnProps, translateError(btnConfig.text || ''));
                    if (btnConfig.onClick) {
                        btn.addEventListener('click', (e) => btnConfig.onClick(e, this));
                    }
                    footerEl.appendChild(btn);
                });
            } else if (opts.footer instanceof Node) {
                footerEl.appendChild(opts.footer);
            } else if (typeof opts.footer === 'function') {
                opts.footer(footerEl);
            }
            formOrWrapper.appendChild(footerEl);
        }

        this.appendChild(this._modalContent);
    }

    /**
     * Show the dialog and wire Escape / backdrop dismiss handlers.
     * @returns {void}
     */
    open() {
        this.style.display = 'flex';
        this.classList.add('visible');
        if (window.updateModalInertState) window.updateModalInertState();

        this._handleEsc = (e) => {
            if (e.key === 'Escape') {
                e.stopPropagation();
                if (document.activeElement && document.activeElement !== document.body) {
                    document.activeElement.blur();
                }
                this.close(false);
            }
        };
        document.addEventListener('keydown', this._handleEsc);

        if (this._backdrop) {
            this._backdrop.onclick = () => {
                if (this._options.closable !== false) this.close(false);
            };
        }
    }

    /**
     * Animate closed, optionally destroy, and resolve the open promise.
     * @param {*} result - Value passed to the show() promise and onClose.
     * @returns {void}
     */
    close(result) {
        if (this._isClosing || this.dataset.isClosing === 'true') return;
        this._isClosing = true;
        if (this._handleEsc) {
            document.removeEventListener('keydown', this._handleEsc);
        }

        closeModalWithAnim(this, () => {
            this.classList.remove('visible');
            if (this._options.destroyOnClose !== false && this.parentNode) {
                this.parentNode.removeChild(this);
            }
            if (this._options.onClose) this._options.onClose(result);
            if (this._resolve) this._resolve(result);
            this._isClosing = false;
        });
    }
}

if (!customElements.get('renop-dialog')) {
    customElements.define('renop-dialog', RenopDialog);
}
