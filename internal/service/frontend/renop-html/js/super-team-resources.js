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
import {apiRequest} from './api.js';
import {createIcon} from './components.js';
import {t} from './i18n.js';
import {packageResourceTarget} from './profile-links.js';
import {getRepositoryFormat} from './repository-formats.js';
import {localizedResponseError} from './response-errors.js';

const resourcePageSize = 8;
const resourceFormats = Object.freeze([
    ['maven', 'profile.mavenDomains'],
    ['cargo', 'profile.cargoPackages'],
    ['docker', 'profile.dockerImages'],
    ['npm', 'profile.npmPackages'],
]);

/**
 * Navigate one package link through the application shell.
 * @param {MouseEvent} event - Link click.
 * @returns {void}
 */
function openResource(event) {
    if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
    event.preventDefault();
    window.history.pushState(null, '', event.currentTarget.getAttribute('href'));
    window.dispatchEvent(new PopStateEvent('popstate'));
}

/**
 * Render one resource row.
 * @param {object} resource - Server resource payload.
 * @returns {HTMLAnchorElement} Routed resource row.
 */
function resourceRow(resource) {
    const link = el('a', {class: 'super-team-resource-row', href: packageResourceTarget(resource)},
        el('span', {class: 'super-team-resource-main'},
            el('strong', {}, resource.name || ''),
            el('span', {}, resource.description || resource.repository || t('superTeam.noDescription'))
        ),
        el('span', {class: 'super-team-resource-meta'},
            resource.repository ? el('code', {}, resource.repository) : null,
            resource.archived ? el('span', {class: 'super-team-resource-archived'},
                t(resource.format === 'npm' ? 'npm.archived' : 'cargo.archived')) : null,
            createIcon('chevron')
        )
    );
    link.addEventListener('click', openResource);
    return link;
}

/**
 * Load one server-backed resource page.
 * @param {string} prefix - Global-team prefix.
 * @param {string} format - Package format.
 * @param {HTMLElement} panel - Stable format panel.
 * @param {number} [offset=0] - Page offset.
 * @returns {Promise<void>}
 */
async function loadResourcePage(prefix, format, panel, offset = 0) {
    const list = panel.querySelector('.super-team-resource-list');
    const pager = panel.querySelector('.super-team-resource-pager');
    if (!list || !pager) return;
    const sequence = (Number(panel.dataset.loadSequence) || 0) + 1;
    panel.dataset.loadSequence = String(sequence);
    try {
        const response = await apiRequest(`/api/super-teams/${encodeURIComponent(prefix)}/resources` +
            `?format=${encodeURIComponent(format)}&limit=${resourcePageSize}&offset=${offset}`);
        if (!panel.isConnected || panel.dataset.loadSequence !== String(sequence)) return;
        if (!response.ok) throw await localizedResponseError(response, 'superTeam.resourcesLoadFailed');
        const payload = await response.json();
        const resources = Array.isArray(payload.resources) ? payload.resources : [];
        const total = Math.max(0, Number(payload.total) || 0);
        if (total === 0) {
            const section = panel.closest('.super-team-resources-section');
            panel.remove();
            if (section && !section.querySelector('.super-team-resource-panel')) section.remove();
            return;
        }
        const currentOffset = offset >= total ? Math.max(0,
            Math.floor((total - 1) / resourcePageSize) * resourcePageSize) : offset;
        if (currentOffset !== offset) {
            await loadResourcePage(prefix, format, panel, currentOffset);
            return;
        }
        await morphElementHeight(list, () => list.replaceChildren(...resources.map(resourceRow)), {duration: 260});
        const page = Math.floor(currentOffset / resourcePageSize);
        const pages = Math.max(1, Math.ceil(total / resourcePageSize));
        pager.replaceChildren();
        if (pages > 1) {
            const summary = t('superTeam.pageSummary', {page: page + 1, pages, total});
            pager.append(
                el('button', {
                    type: 'button', class: 'renop-pagination-btn', disabled: page === 0,
                    onclick: () => void loadResourcePage(prefix, format, panel,
                        Math.max(0, currentOffset - resourcePageSize))
                }, t('common.prev')),
                el('span', {class: 'renop-pagination-summary'}, summary),
                el('button', {
                    type: 'button', class: 'renop-pagination-btn', disabled: page >= pages - 1,
                    onclick: () => void loadResourcePage(prefix, format, panel, currentOffset + resourcePageSize)
                }, t('common.next'))
            );
            pager.setAttribute('aria-label', summary);
        }
    } catch (error) {
        if (!panel.isConnected || panel.dataset.loadSequence !== String(sequence)) return;
        console.error(`Failed to load ${format} global team resources`, error);
        pager.replaceChildren();
        await morphElementHeight(list, () => list.replaceChildren(
            el('div', {class: 'super-team-state is-error'}, t('superTeam.resourcesLoadFailed'))
        ), {duration: 240});
    }
}

/**
 * Build the bounded cross-format resource collection for one global team.
 * @param {string} prefix - Global-team prefix.
 * @returns {HTMLElement} Asynchronously populated resource section.
 */
export function createSuperTeamResourcesSection(prefix) {
    const panels = resourceFormats.map(([format, titleKey]) => {
        const descriptor = getRepositoryFormat(format);
        const panel = el('section', {class: `super-team-resource-panel is-${format}`},
            el('header', {},
                el('span', {class: 'super-team-resource-icon', 'aria-hidden': 'true'},
                    createIcon(descriptor.icon || 'repositoryFiles')),
                el('h3', {}, t(titleKey))
            ),
            el('div', {class: 'super-team-resource-list'},
                el('div', {class: 'super-team-state'},
                    el('div', {class: 'sessions-loading-spinner', 'aria-hidden': 'true'}),
                    el('span', {}, t('common.loading'))
                )
            ),
            el('nav', {class: 'renop-pagination super-team-resource-pager'})
        );
        void loadResourcePage(prefix, format, panel);
        return panel;
    });
    return el('section', {class: 'super-team-resources-section'},
        el('header', {class: 'super-team-resources-header'},
            el('h2', {}, t('superTeam.resourcesTitle')),
            el('p', {}, t('superTeam.resourcesSubtitle'))
        ),
        el('div', {class: 'super-team-resources-grid'}, ...panels)
    );
}
