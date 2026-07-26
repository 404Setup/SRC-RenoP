/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

/** Shared DOM helpers with no feature-module dependencies (avoids import cycles). */

const tabResizeObserver = new ResizeObserver((entries) => {
    const containersToUpdate = new Set();
    for (const entry of entries) {
        const target = entry.target;
        if (target.classList.contains('tabs')) {
            containersToUpdate.add(target);
        } else {
            const container = target.closest('.tabs');
            if (container) {
                containersToUpdate.add(container);
            }
        }
    }
    for (const container of containersToUpdate) {
        updateTabIndicator(container);
    }
});

/**
 * Position the sliding tab indicator under the active tab in a tabs container.
 * @param {Element|null} tabsContainer - Element with class `tabs` containing `.tab` children.
 * @returns {void}
 */
export function updateTabIndicator(tabsContainer) {
    if (!tabsContainer) return;
    const activeTab = tabsContainer.querySelector('.tab.active');
    let indicator = tabsContainer.querySelector('.tab-indicator');
    if (activeTab) {
        let isNew = false;
        if (!indicator) {
            indicator = document.createElement('div');
            indicator.className = 'tab-indicator';
            tabsContainer.appendChild(indicator);
            isNew = true;
        }
        if (activeTab.style.display !== 'none' && activeTab.offsetWidth > 0) {
            indicator.style.display = 'block';
            if (isNew) {
                indicator.style.transition = 'none';
                indicator.style.width = activeTab.offsetWidth + 'px';
                indicator.style.transform = `translateX(${activeTab.offsetLeft}px)`;
                void indicator.offsetHeight;
                indicator.style.transition = '';
            } else {
                indicator.style.width = activeTab.offsetWidth + 'px';
                indicator.style.transform = `translateX(${activeTab.offsetLeft}px)`;
            }
        } else {
            indicator.style.display = 'none';
        }
    }
}

/**
 * Observe a tabs container (and its tabs) so the indicator updates on resize.
 * @param {Element|null} container - Tabs container element to observe.
 * @returns {void}
 */
export function registerTabContainer(container) {
    if (!container) return;
    tabResizeObserver.observe(container);
    container.querySelectorAll('.tab').forEach(tab => {
        tabResizeObserver.observe(tab);
    });
}

let scrollToTopTimer = null;

/**
 * Smoothly scroll the window to the top, temporarily locking main min-height to avoid jump.
 * @param {number} [duration=350] - Approximate scroll duration in ms (used for min-height restore).
 * @returns {void}
 */
export function smoothScrollToTop(duration = 350) {
    const startY = window.scrollY || window.pageYOffset || document.documentElement.scrollTop || 0;
    if (startY <= 0) return;

    const mainEl = document.querySelector('main') || document.querySelector('.container') || document.body;
    const originalMinHeight = mainEl.style.minHeight;
    const currentTotalHeight = Math.max(
        document.body.scrollHeight,
        document.documentElement.scrollHeight,
        startY + window.innerHeight
    );

    mainEl.style.minHeight = `${currentTotalHeight}px`;

    try {
        window.scrollTo({top: 0, behavior: 'smooth'});
    } catch (e) {
        window.scrollTo(0, 0);
    }

    if (scrollToTopTimer) clearTimeout(scrollToTopTimer);
    scrollToTopTimer = setTimeout(() => {
        mainEl.style.minHeight = originalMinHeight;
    }, duration + 100);
}

/**
 * Set `inert` on the main app and top nav when any known modal is open.
 * @returns {void}
 */
export function updateModalInertState() {
    const modalIds = [
        'create-token-modal', 'user-result-modal', 'login-modal', 'privacy-policy-modal', 'repo-mirrors-modal',
        'language-modal', 'renop-confirm-container', 'renop-prompt-container'
    ];
    const isAnyModalOpen = modalIds.some(id => {
        const el = document.getElementById(id);
        return el && el.style.display !== 'none' && el.style.display !== '' && el.dataset.isClosing !== 'true';
    });
    const app = document.getElementById('app');
    const nav = document.querySelector('.top-nav');
    if (app && app.inert !== isAnyModalOpen) app.inert = isAnyModalOpen;
    if (nav && nav.inert !== isAnyModalOpen) nav.inert = isAnyModalOpen;
}

window.updateModalInertState = updateModalInertState;

/**
 * Close a modal with fade-out animation, then hide it and run an optional callback.
 * @param {HTMLElement|null} modal - Modal root element.
 * @param {Function} [callback] - Invoked after the close animation finishes.
 * @returns {void}
 */
export function closeModalWithAnim(modal, callback) {
    if (!modal || modal.dataset.isClosing === 'true') return;
    modal.dataset.isClosing = 'true';
    updateModalInertState();

    const content = modal.querySelector('.modal-content') || modal;
    if (content !== modal) {
        content.style.animation = 'modalFadeOut 0.15s ease-out forwards';
        modal.style.animation = 'backdropFadeOut 0.15s ease-out forwards';
    } else {
        modal.style.animation = 'modalFadeOut 0.15s ease-out forwards';
    }

    setTimeout(() => {
        modal.style.display = 'none';
        modal.style.animation = '';
        modal.style.transition = '';
        modal.style.opacity = '';
        if (content !== modal) content.style.animation = '';
        modal.dataset.isClosing = 'false';
        updateModalInertState();
        if (callback) callback();
    }, 140);
}

/**
 * Enable click-and-drag horizontal scrolling on an overflow container.
 * Suppresses click events that follow a drag so nested links are not activated.
 * @param {HTMLElement|null} container - Scrollable element.
 * @returns {void}
 */
export function enableDragToScroll(container) {
    if (!container) return;
    let isDown = false;
    let isDragging = false;
    let startX;
    let scrollLeft;

    const stopDragging = () => {
        isDown = false;
        container.style.cursor = '';
        container.classList.remove('is-dragging');
        setTimeout(() => {
            isDragging = false;
        }, 0);
    };

    container.addEventListener('pointerdown', (e) => {
        if (e.button !== 0) return;
        if (container.scrollWidth <= container.clientWidth) return;
        isDown = true;
        isDragging = false;
        startX = e.pageX - container.offsetLeft;
        scrollLeft = container.scrollLeft;
    });

    container.addEventListener('pointerleave', stopDragging);
    container.addEventListener('pointerup', stopDragging);
    container.addEventListener('pointercancel', stopDragging);

    container.addEventListener('pointermove', (e) => {
        if (!isDown) return;
        const x = e.pageX - container.offsetLeft;
        const walk = (x - startX) * 1.5;
        if (Math.abs(walk) > 5) {
            if (!isDragging) {
                isDragging = true;
                if (window.getSelection) {
                    window.getSelection().removeAllRanges();
                }
            }
            container.style.cursor = 'grabbing';
            container.classList.add('is-dragging');
            e.preventDefault();
        }
        if (isDragging) {
            container.scrollLeft = scrollLeft - walk;
        }
    });

    container.addEventListener('click', (e) => {
        if (isDragging) {
            e.preventDefault();
            e.stopPropagation();
        }
    }, true);

    container.addEventListener('dragstart', (e) => {
        if (isDown) e.preventDefault();
    });
}
