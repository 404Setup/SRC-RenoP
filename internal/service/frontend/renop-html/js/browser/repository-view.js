/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {el} from '@renop/ui/dom';
import {morphElementHeight} from '@renop/ui/height-anim';
import {createIcon} from '../components/icon.js';
import {formatTimestamp} from '../time.js';

const entranceTimers = new WeakMap();

/**
 * Cancel the pending entrance-class cleanup for a persistent repository view.
 * @param {HTMLElement|null|undefined} container - Repository view container.
 * @returns {void}
 */
function clearEntranceTimer(container) {
    if (!container) return;
    const timer = entranceTimers.get(container);
    if (timer) clearTimeout(timer);
    entranceTimers.delete(container);
}

/**
 * Locate a persistent repository view and optionally create it at a standard mount point.
 * @param {HTMLElement|null|undefined} current - Previously resolved container.
 * @param {object} options - View configuration.
 * @param {string} options.id - Stable element id.
 * @param {string} [options.className=''] - Class for a newly created view.
 * @param {boolean} [options.create=false] - Whether to create a missing view.
 * @param {string[]} [options.mountSelectors=[]] - Ordered parent selectors for a new view.
 * @param {() => HTMLElement|null} [options.mountResolver] - Optional custom parent resolver.
 * @returns {HTMLElement|null} Resolved view container.
 */
export function ensureRepositoryView(current, {
    id,
    className = '',
    create = false,
    mountSelectors = [],
    mountResolver = null
}) {
    if (current?.isConnected) return current;
    let container = document.getElementById(id);
    if (container || !create) return container;
    container = el('section', {id, class: className, hidden: true});
    const mount = (typeof mountResolver === 'function' ? mountResolver() : null) ||
        mountSelectors.map((selector) => document.querySelector(selector)).find(Boolean) || document.body;
    mount.appendChild(container);
    return container;
}

/**
 * Hide and clear a persistent repository view without leaving animation state behind.
 * @param {HTMLElement|null|undefined} container - Repository view container.
 * @param {string[]} [extraClasses=[]] - Additional state classes to remove.
 * @returns {void}
 */
export function hideRepositoryView(container, extraClasses = []) {
    if (!container) return;
    clearEntranceTimer(container);
    container.hidden = true;
    container.classList.remove('is-visible', 'is-updating', 'is-entering', ...extraClasses);
    container.removeAttribute('aria-busy');
    container.replaceChildren();
}

/**
 * Synchronize repository loading state and accessibility metadata.
 * @param {HTMLElement|null|undefined} container - Repository view container.
 * @param {boolean} busy - Whether a route request is in flight.
 * @returns {void}
 */
export function setRepositoryViewBusy(container, busy) {
    if (!container) return;
    container.classList.toggle('is-updating', busy);
    if (busy) container.setAttribute('aria-busy', 'true');
    else container.removeAttribute('aria-busy');
}

/**
 * Replace repository content with shared height and entrance animation handling.
 * @param {HTMLElement} container - Repository view container.
 * @param {Node[]|Node|Function} content - Replacement nodes or DOM mutation callback.
 * @param {object} [options={}] - Animation options.
 * @param {number} [options.duration=280] - Height morph duration in milliseconds.
 * @param {boolean} [options.enter=true] - Whether to apply the entrance state.
 * @param {number} [options.enterDuration=380] - Entrance state duration in milliseconds.
 * @returns {Promise<void>}
 */
export async function replaceRepositoryView(container, content, {
    duration = 280,
    enter = true,
    enterDuration = 380
} = {}) {
    if (!container) return;
    const nodes = Array.isArray(content) ? content.filter(Boolean) : (content ? [content] : []);
    const replaceContent = typeof content === 'function'
        ? content
        : () => container.replaceChildren(...nodes);
    clearEntranceTimer(container);
    container.classList.remove('is-entering');
    const mutate = () => {
        if (enter) {
            container.classList.add('is-entering');
            const timer = setTimeout(() => {
                entranceTimers.delete(container);
                if (container.isConnected) container.classList.remove('is-entering');
            }, enterDuration);
            entranceTimers.set(container, timer);
        }
        replaceContent();
    };
    await morphElementHeight(container, mutate, {duration});
    setRepositoryViewBusy(container, false);
}

/**
 * Build a consistent repository back-navigation button.
 * @param {object} options - Navigation configuration.
 * @param {string} options.path - Destination route.
 * @param {string} options.label - Localized button label.
 * @param {(path: string) => void} options.navigate - Application navigation callback.
 * @param {string} options.className - View-specific button class.
 * @param {string} [options.iconClass=''] - Optional icon class.
 * @returns {HTMLButtonElement} Back button.
 */
export function createRepositoryBackButton({path, label, navigate, className, iconClass = ''}) {
    return el('button', {
        type: 'button', class: className, onclick: () => navigate?.(path)
    }, createIcon('chevronLeft', iconClass ? {class: iconClass} : {}), el('span', {}, label));
}

/**
 * Format seconds, milliseconds, or date strings for repository metadata.
 * @param {number|string|null|undefined} value - Timestamp value.
 * @param {object} [options={}] - Formatting options.
 * @param {boolean} [options.dateOnly=false] - Return only the localized date.
 * @param {string} [options.fallback=''] - Value returned for invalid timestamps.
 * @returns {string} Localized timestamp or fallback.
 */
export function formatRepositoryTimestamp(value, {dateOnly = false, fallback = ''} = {}) {
    return formatTimestamp(value, {dateOnly, fallback});
}

/**
 * Build a shared metadata facts section for repository package and namespace pages.
 * @param {string} title - Localized section heading.
 * @param {{label: string, value: Node|string|number, code?: boolean, wide?: boolean}[]} facts - Display facts.
 * @param {object} [options={}] - Optional view-specific classes.
 * @param {string} [options.className=''] - Additional section class.
 * @returns {HTMLElement} Metadata section.
 */
export function createRepositoryFactsSection(title, facts, {className = ''} = {}) {
    const grid = el('div', {class: 'repository-facts-grid'});
    for (const fact of facts) {
        if (!fact || fact.value === null || fact.value === undefined || fact.value === '') continue;
        let value = fact.value;
        if (!value?.nodeType) {
            value = fact.code
                ? el('code', {title: String(value)}, String(value))
                : el('span', {}, String(value));
        }
        grid.appendChild(el('div', {
                class: `repository-fact${fact.wide ? ' is-wide' : ''}`
            },
            el('span', {class: 'repository-fact-label'}, fact.label),
            el('div', {class: 'repository-fact-value'}, value)
        ));
    }
    return el('section', {
        class: `repository-facts-section${className ? ` ${className}` : ''}`
    }, el('h3', {class: 'repository-facts-title'}, title), grid);
}

/**
 * Build the shared provenance badge used for packages obtained from mirrors.
 * @param {string} label - Localized mirror-source label.
 * @returns {HTMLSpanElement} Mirror provenance badge.
 */
export function createRepositoryMirrorBadge(label) {
    return el('span', {class: 'repository-mirror-badge'},
        createIcon('network'), el('span', {}, label));
}
