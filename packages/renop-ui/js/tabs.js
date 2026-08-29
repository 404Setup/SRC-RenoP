/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {enableDragToScroll} from './scroll.js';
import {$} from './jquery.js';

const tabClickContainers = new WeakSet();

const tabResizeObserver = new ResizeObserver((entries) => {
    const containersToUpdate = new Set();
    for (const entry of entries) {
        const target = entry.target;
        if ($(target).hasClass('tabs')) {
            containersToUpdate.add(target);
        } else {
            const container = $(target).closest('.tabs').get(0);
            if (container) containersToUpdate.add(container);
        }
    }
    for (const container of containersToUpdate) {
        updateTabIndicator(container);
    }
});

/**
 * Smoothly reveal a tab inside its horizontal tabs viewport without moving the page vertically.
 * @param {Element|null|undefined} tab - Tab element to reveal.
 * @param {{behavior?: ScrollBehavior, padding?: number}} [options]
 * @returns {void}
 */
export function scrollTabIntoView(tab, {behavior = 'smooth', padding = 8} = {}) {
    if (!tab) return;
    const scrollContainer = $(tab).closest('.tabs-container').get(0);
    if (!scrollContainer || scrollContainer.scrollWidth <= scrollContainer.clientWidth + 1) return;

    const containerRect = scrollContainer.getBoundingClientRect();
    const tabRect = tab.getBoundingClientRect();
    const leftEdge = containerRect.left + padding;
    const rightEdge = containerRect.right - padding;
    let targetScrollLeft = scrollContainer.scrollLeft;

    if (tabRect.left < leftEdge) {
        targetScrollLeft -= leftEdge - tabRect.left;
    } else if (tabRect.right > rightEdge) {
        targetScrollLeft += tabRect.right - rightEdge;
    } else {
        return;
    }

    const maxScrollLeft = Math.max(0, scrollContainer.scrollWidth - scrollContainer.clientWidth);
    targetScrollLeft = Math.max(0, Math.min(maxScrollLeft, targetScrollLeft));
    scrollContainer.scrollTo({left: targetScrollLeft, behavior});
}

/**
 * Position the sliding tab indicator under the active tab in a tabs container.
 * @param {Element|null|undefined} tabsContainer - Element with class `tabs` containing `.tab` children.
 * @returns {void}
 */
export function updateTabIndicator(tabsContainer) {
    if (!tabsContainer) return;
    const $tabs = $(tabsContainer);
    const activeTab = $tabs.find('.tab.active').get(0);
    let indicator = $tabs.find('.tab-indicator').get(0);
    if (!activeTab) return;

    let isNew = false;
    if (!indicator) {
        indicator = $('<div>', {class: 'tab-indicator'}).appendTo($tabs).get(0);
        isNew = true;
    }

    if (activeTab.style.display !== 'none' && activeTab.offsetWidth > 0) {
        $(indicator).css('display', 'block');
        if (isNew) {
            $(indicator).css({
                transition: 'none',
                width: `${activeTab.offsetWidth}px`,
                transform: `translateX(${activeTab.offsetLeft}px)`,
            });
            void indicator.offsetHeight;
            $(indicator).css('transition', '');
        } else {
            $(indicator).css({
                width: `${activeTab.offsetWidth}px`,
                transform: `translateX(${activeTab.offsetLeft}px)`,
            });
        }
    } else {
        $(indicator).css('display', 'none');
    }
}

/**
 * Observe a tabs container (and its tabs) so the indicator updates on resize.
 * @param {Element|null|undefined} container - Tabs container element to observe.
 * @returns {void}
 */
export function registerTabContainer(container) {
    if (!container) return;
    enableDragToScroll(container);
    if (!tabClickContainers.has(container)) {
        tabClickContainers.add(container);
        $(container).on('click', '.tab', (e) => {
            const tab = e.currentTarget;
            if (container.contains(tab)) scrollTabIntoView(tab);
        });
    }
    tabResizeObserver.observe(container);
    $(container).find('.tab').each((index, tab) => {
        tabResizeObserver.observe(tab);
    });
}
