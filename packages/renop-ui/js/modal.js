/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

const EASE_OUT_EXPO = 'cubic-bezier(0.16, 1, 0.3, 1)';

/** Animation CSS values and default close duration (ms). */
export const MODAL_ANIM = {
    openContent: `modalFadeIn 0.3s ${EASE_OUT_EXPO} forwards`,
    openBackdrop: 'backdropFadeIn 0.25s ease-out forwards',
    closeContent: `modalFadeOut 0.2s ${EASE_OUT_EXPO} forwards`,
    closeBackdrop: 'backdropFadeOut 0.2s ease-out forwards',
    /** Slightly shorter shell close used by product UI chrome. */
    closeContentFast: 'modalFadeOut 0.15s ease-out forwards',
    closeBackdropFast: 'backdropFadeOut 0.15s ease-out forwards',
    closeDurationMs: 180,
    closeDurationFastMs: 140,
};

/** @type {{ modalIds: string[], rootSelectors: string[] }} */
const inertConfig = {
    modalIds: [],
    rootSelectors: ['#app', '.top-nav'],
};

/**
 * Configure which modal ids and page roots participate in inert locking.
 * Optionally installs `window.updateModalInertState` for legacy call sites.
 *
 * @param {object} [options]
 * @param {string[]} [options.modalIds] - Element ids that count as open modals.
 * @param {string[]} [options.rootSelectors] - Selectors to toggle `inert` on.
 * @param {boolean} [options.installGlobal=true] - Assign `window.updateModalInertState`.
 * @returns {void}
 */
export function configureModalInert({
    modalIds,
    rootSelectors,
    installGlobal = true,
} = {}) {
    if (Array.isArray(modalIds)) inertConfig.modalIds = modalIds.slice();
    if (Array.isArray(rootSelectors)) inertConfig.rootSelectors = rootSelectors.slice();
    if (installGlobal && typeof window !== 'undefined') {
        window.updateModalInertState = updateModalInertState;
    }
}

/**
 * Set `inert` on configured page roots when any known modal is open.
 * @returns {void}
 */
export function updateModalInertState() {
    const isAnyModalOpen = inertConfig.modalIds.some((id) => {
        const el = document.getElementById(id);
        return isModalOpen(el);
    });
    for (const sel of inertConfig.rootSelectors) {
        const el = document.querySelector(sel);
        if (el && el.inert !== isAnyModalOpen) el.inert = isAnyModalOpen;
    }
}

/**
 * Notify host apps that modal open/close may change page inert state.
 * @returns {void}
 */
function notifyInert() {
    if (typeof window !== 'undefined' && typeof window.updateModalInertState === 'function') {
        window.updateModalInertState();
    } else {
        updateModalInertState();
    }
}

/**
 * Whether a modal root is visible and not mid-close.
 * @param {HTMLElement|null|undefined} modal
 * @returns {boolean}
 */
export function isModalOpen(modal) {
    if (!modal) return false;
    if (modal.dataset.isClosing === 'true') return false;
    const display = modal.style.display;
    return display !== 'none' && display !== '';
}

/**
 * Resolve backdrop / content nodes for a modal root.
 * @param {HTMLElement} modal
 * @returns {{ backdrop: HTMLElement|null, content: HTMLElement|null }}
 */
function modalParts(modal) {
    const backdrop = modal.querySelector('.modal-backdrop');
    const content = modal.querySelector('.modal-content');
    return {backdrop, content};
}

/**
 * Open a modal with enter animations on backdrop + content when present.
 * @param {HTMLElement|null|undefined} modal
 * @param {{ display?: string, onOpen?: () => void }} [options]
 * @returns {boolean} True if the modal was opened.
 */
export function openModalWithAnim(modal, {display = 'flex', onOpen} = {}) {
    if (!modal || modal.dataset.isClosing === 'true') return false;

    const {backdrop, content} = modalParts(modal);
    if (backdrop) backdrop.style.animation = MODAL_ANIM.openBackdrop;
    if (content) content.style.animation = MODAL_ANIM.openContent;

    modal.style.display = display;
    notifyInert();
    if (typeof onOpen === 'function') onOpen();
    return true;
}

/**
 * Close a modal with exit animations, then hide and optional callback.
 *
 * Prefer animating `.modal-backdrop` + `.modal-content` when both exist.
 * Falls back to animating content + modal root (legacy product UI pattern).
 *
 * @param {HTMLElement|null|undefined} modal
 * @param {(() => void)|null|undefined} [callback]
 * @param {{ durationMs?: number, fast?: boolean }} [options]
 * @returns {void}
 */
export function closeModalWithAnim(modal, callback, {durationMs, fast = false} = {}) {
    if (!modal || modal.dataset.isClosing === 'true') return;

    modal.dataset.isClosing = 'true';
    notifyInert();

    const {backdrop, content} = modalParts(modal);
    const contentAnim = fast ? MODAL_ANIM.closeContentFast : MODAL_ANIM.closeContent;
    const backdropAnim = fast ? MODAL_ANIM.closeBackdropFast : MODAL_ANIM.closeBackdrop;
    const wait = durationMs ?? (fast ? MODAL_ANIM.closeDurationFastMs : MODAL_ANIM.closeDurationMs);

    if (backdrop && content) {
        content.style.animation = contentAnim;
        backdrop.style.animation = backdropAnim;
    } else if (content && content !== modal) {
        content.style.animation = contentAnim;
        modal.style.animation = backdropAnim;
    } else {
        modal.style.animation = contentAnim;
    }

    setTimeout(() => {
        modal.style.display = 'none';
        modal.style.animation = '';
        modal.style.transition = '';
        modal.style.opacity = '';
        if (backdrop) backdrop.style.animation = '';
        if (content && content !== modal) content.style.animation = '';
        modal.dataset.isClosing = 'false';
        notifyInert();
        if (typeof callback === 'function') callback();
    }, wait);
}

/**
 * Wire open/close controls for a simple modal (language picker, privacy, …).
 *
 * @param {object} opts
 * @param {HTMLElement|null|undefined} opts.modal - Modal root (`#language-modal`, …).
 * @param {Array<HTMLElement|null|undefined>} [opts.openTriggers] - Buttons that open.
 * @param {Array<HTMLElement|null|undefined>} [opts.closeTriggers] - Buttons / backdrop that close.
 * @param {boolean} [opts.escape=true] - Close on Escape when open.
 * @param {() => void} [opts.onOpen] - After open animation starts / display set.
 * @param {() => void} [opts.onClose] - After close animation finishes.
 * @param {boolean} [opts.fastClose=false] - Use shorter close timings.
 * @returns {{ open: () => void, close: () => void }|null}
 */
export function bindModalChrome({
    modal,
    openTriggers = [],
    closeTriggers = [],
    escape = true,
    onOpen,
    onClose,
    fastClose = false,
} = {}) {
    if (!modal) return null;

    const open = () => {
        openModalWithAnim(modal, {onOpen});
    };

    const close = () => {
        if (!isModalOpen(modal) && modal.style.display === 'none') return;
        closeModalWithAnim(modal, onClose, {fast: fastClose});
    };

    for (const el of openTriggers) {
        if (el) el.addEventListener('click', open);
    }
    for (const el of closeTriggers) {
        if (el) el.addEventListener('click', close);
    }

    if (escape) {
        document.addEventListener('keydown', (e) => {
            if (e.key === 'Escape' && isModalOpen(modal) && modal.style.display === 'flex') {
                close();
            }
        });
    }

    return {open, close};
}
