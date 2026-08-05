/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {collapseElement, expandElement, morphElementHeight} from '@renop/ui/height-anim';
import {formatBytes} from './utils.js';
import {fetchProto} from '../api.js';
import {t} from '../i18n.js';
import {createButton, createMirrorCard} from '../components.js';
import {closeModalWithAnim} from '../app-ui.js';
import {RepoDetailsResponse} from '../proto/index.js';

const REPO_STATS_MARGIN = '1.5rem';
let statsLoadSeq = 0;

/**
 * Create (once) and return the repository mirrors modal element.
 * @returns {HTMLElement}
 */
function ensureMirrorsModal() {
    let modal = document.getElementById('repo-mirrors-modal');
    if (modal) return modal;

    modal = document.createElement('div');
    modal.id = 'repo-mirrors-modal';
    modal.className = 'modal';
    modal.style.display = 'none';
    modal.setAttribute('role', 'dialog');
    modal.setAttribute('aria-modal', 'true');
    modal.setAttribute('aria-labelledby', 'repo-mirrors-modal-title');

    const backdrop = document.createElement('div');
    backdrop.className = 'modal-backdrop';

    const content = document.createElement('div');
    content.className = 'modal-content mirrors-modal-content';

    const closeButton = document.createElement('button');
    closeButton.className = 'close-btn';
    closeButton.type = 'button';
    closeButton.setAttribute('aria-label', t('details.closeMirrors'));
    closeButton.textContent = '\u00d7';

    const header = document.createElement('div');
    header.className = 'modal-header';
    const title = document.createElement('h3');
    title.id = 'repo-mirrors-modal-title';
    title.className = 'modal-title';
    title.textContent = t('details.mirrors');
    const subtitle = document.createElement('p');
    subtitle.className = 'mirrors-modal-subtitle';
    subtitle.id = 'repo-mirrors-modal-subtitle';
    subtitle.textContent = t('details.mirrorsSubtitle');
    header.append(title, subtitle);

    const body = document.createElement('div');
    body.id = 'repo-mirrors-modal-content';
    body.className = 'modal-body mirrors-modal-body';

    content.append(closeButton, header, body);
    modal.append(backdrop, content);
    document.body.appendChild(modal);

    const close = () => {
        closeModalWithAnim(modal);
    };
    closeButton.addEventListener('click', close);
    backdrop.addEventListener('click', close);

    return modal;
}

/**
 * Populate and show the mirrors modal for the given mirror list.
 * @param {Array<object>} mirrors
 * @returns {void}
 */
function openMirrorsModal(mirrors) {
    const modal = ensureMirrorsModal();
    const body = modal.querySelector('#repo-mirrors-modal-content');
    const subtitle = modal.querySelector('#repo-mirrors-modal-subtitle');
    if (subtitle) {
        subtitle.textContent = t('details.mirrorsConfiguredCount', {count: mirrors.length});
    }
    body.replaceChildren(...mirrors.map(mirror => createMirrorCard(mirror, true)));
    modal.style.display = 'flex';
    if (window.updateModalInertState) window.updateModalInertState();
    modal.querySelector('.close-btn').focus();
}

/**
 * Build a single label/value row for the repo stats card.
 * @param {string} label
 * @param {string} value
 * @param {{nested?: boolean, strong?: boolean}} [options]
 * @returns {HTMLElement}
 */
function buildStatRow(label, value, options = {}) {
    const row = document.createElement('div');
    row.className = 'repo-stat-row' + (options.nested ? ' repo-stat-row--nested' : '') + (options.strong ? ' repo-stat-row--strong' : '');

    const labelEl = document.createElement('span');
    labelEl.className = 'repo-stat-label';
    labelEl.textContent = label;

    const valueEl = document.createElement('span');
    valueEl.className = 'repo-stat-value';
    valueEl.textContent = value;

    row.append(labelEl, valueEl);
    return row;
}

/**
 * Render size/file counts and optional mirrors section into the stats container.
 * @param {HTMLElement} repoStatsContent
 * @param {object} data repo details payload
 * @returns {void}
 */
function renderRepoStatsContent(repoStatsContent, data) {
    const mirrors = Array.isArray(data.mirrors) ? data.mirrors : [];
    repoStatsContent.replaceChildren();

    const statsBlock = document.createElement('div');
    statsBlock.className = 'repo-stats-block' + (mirrors.length > 0 ? ' repo-stats-block--with-mirrors' : '');

    statsBlock.append(
        buildStatRow(t('stats.repoSize'), formatBytes(data.total_size), {strong: true}),
        buildStatRow(t('stats.artifactsSize'), formatBytes(data.artifact_size), {nested: true}),
        buildStatRow(t('stats.metadataSize'), formatBytes(data.metadata_size), {nested: true}),
        buildStatRow(
            t('stats.totalFiles'),
            t('stats.totalFilesFormat', {
                total: data.total_files,
                artifacts: data.artifact_count,
                metadata: data.metadata_count
            }),
            {strong: true}
        )
    );
    repoStatsContent.appendChild(statsBlock);

    if (mirrors.length > 0) {
        const section = document.createElement('div');
        section.className = 'repo-mirrors-section';

        const heading = document.createElement('div');
        heading.className = 'repo-mirrors-heading';
        const headingTitle = document.createElement('h4');
        headingTitle.className = 'repo-mirrors-title';
        headingTitle.textContent = t('details.mirrors');
        const count = document.createElement('span');
        count.className = 'repo-mirrors-count';
        count.textContent = String(mirrors.length);
        heading.append(headingTitle, count);
        section.appendChild(heading);

        const mirrorList = document.createElement('div');
        mirrorList.className = 'repo-mirrors-list';
        mirrorList.appendChild(createMirrorCard(mirrors[0]));

        if (mirrors.length > 1) {
            const expandButton = createButton(t('details.viewAllMirrors', {count: mirrors.length}), {
                class: 'pill-btn pill-btn--soft pill-btn--sm mirrors-expand-btn',
                icon: 'network',
                iconProps: {width: '14', height: '14'},
                onClick: () => openMirrorsModal(mirrors)
            });
            mirrorList.appendChild(expandButton);
        }

        section.appendChild(mirrorList);
        repoStatsContent.appendChild(section);
    }
}

/**
 * Whether the repo stats card is currently shown.
 * @param {HTMLElement} card
 * @returns {boolean}
 */
function isRepoStatsVisible(card) {
    return card.classList.contains('is-visible')
        && card.style.display !== 'none'
        && getComputedStyle(card).display !== 'none';
}

/**
 * Collapse and hide the repository stats card.
 * @returns {Promise<void>}
 */
export function hideRepoStats() {
    const repoStatsCard = document.getElementById('repo-stats-card');
    if (!repoStatsCard) return Promise.resolve();
    statsLoadSeq += 1;
    return collapseElement(repoStatsCard, {
        duration: 300,
        marginTop: true,
    });
}

/**
 * Fetch and display repository size/mirror stats for `repoName`.
 * @param {string} repoName
 * @returns {Promise<void>}
 */
export async function updateRepoStats(repoName) {
    const repoStatsCard = document.getElementById('repo-stats-card');
    const repoStatsContent = document.getElementById('repo-stats-content');
    if (!repoStatsCard || !repoStatsContent) return;

    const seq = ++statsLoadSeq;

    try {
        const {response, data} = await fetchProto(`/api/maven/repo-details/${repoName}`, RepoDetailsResponse);
        if (seq !== statsLoadSeq) return;

        if (!response.ok || !data) {
            await hideRepoStats();
            return;
        }

        if (seq !== statsLoadSeq) return;

        const wasVisible = isRepoStatsVisible(repoStatsCard);

        if (wasVisible) {
            repoStatsCard.classList.add('is-updating');
            await morphElementHeight(repoStatsCard, () => {
                if (seq !== statsLoadSeq) return;
                renderRepoStatsContent(repoStatsContent, data);
            }, {duration: 340});
            if (seq === statsLoadSeq) {
                repoStatsCard.classList.remove('is-updating');
            }
        } else {
            renderRepoStatsContent(repoStatsContent, data);
            if (seq !== statsLoadSeq) return;
            await expandElement(repoStatsCard, {
                duration: 360,
                marginTop: REPO_STATS_MARGIN,
            });
        }
    } catch (e) {
        if (seq !== statsLoadSeq) return;
        console.error('Failed to load repo stats', e);
        await hideRepoStats();
    }
}
