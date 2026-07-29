/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

const tabResizeObserver = new ResizeObserver((entries) => {
    const containersToUpdate = new Set();
    for (const entry of entries) {
        const target = entry.target;
        if (target.classList.contains('tabs')) {
            containersToUpdate.add(target);
        } else {
            const container = target.closest('.tabs');
            if (container) containersToUpdate.add(container);
        }
    }
    for (const container of containersToUpdate) {
        updateTabIndicator(container);
    }
});

/**
 * Position the sliding tab indicator under the active tab in a tabs container.
 * @param {Element|null|undefined} tabsContainer - Element with class `tabs` containing `.tab` children.
 * @returns {void}
 */
export function updateTabIndicator(tabsContainer) {
    if (!tabsContainer) return;
    const activeTab = tabsContainer.querySelector('.tab.active');
    let indicator = tabsContainer.querySelector('.tab-indicator');
    if (!activeTab) return;

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
            indicator.style.width = `${activeTab.offsetWidth}px`;
            indicator.style.transform = `translateX(${activeTab.offsetLeft}px)`;
            void indicator.offsetHeight;
            indicator.style.transition = '';
        } else {
            indicator.style.width = `${activeTab.offsetWidth}px`;
            indicator.style.transform = `translateX(${activeTab.offsetLeft}px)`;
        }
    } else {
        indicator.style.display = 'none';
    }
}

/**
 * Observe a tabs container (and its tabs) so the indicator updates on resize.
 * @param {Element|null|undefined} container - Tabs container element to observe.
 * @returns {void}
 */
export function registerTabContainer(container) {
    if (!container) return;
    tabResizeObserver.observe(container);
    container.querySelectorAll('.tab').forEach((tab) => {
        tabResizeObserver.observe(tab);
    });
}
