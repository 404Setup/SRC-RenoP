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
import {fetchProto} from '../api.js';
import {createIcon} from '../components.js';
import {t} from '../i18n.js';
import {RepositorySearchResponse} from '../proto/index.js';

const SEARCH_DELAY_MS = 180;
const SEARCH_CLOSE_MS = 150;
let initialized = false;
let activeRepository = '';
let activeFormat = 'maven';
let activeNavigate = null;
let searchTimer = 0;
let searchVersion = 0;
let closeTimer = 0;
let requestController = null;
let resultsPanel = null;

/**
 * Encode an application path without allowing package names to alter its segments.
 * @param {string} repository - Repository route segment.
 * @param {string} relativePath - Search-result path below the repository.
 * @returns {string} Safe same-origin path.
 */
function resultPath(repository, relativePath) {
    const segments = String(relativePath || '').split('/').filter(Boolean).map(encodeURIComponent);
    return `/${encodeURIComponent(repository)}/${segments.join('/')}`;
}

/**
 * Return the shared body-level result panel, creating it on first use.
 * @returns {HTMLElement} Search result panel.
 */
function ensureResultsPanel() {
    if (resultsPanel?.isConnected) return resultsPanel;
    resultsPanel = el('div', {
        id: 'repository-search-results',
        class: 'repository-search-results',
        role: 'listbox',
        hidden: true
    });
    resultsPanel.addEventListener('click', handleResultClick);
    document.body.appendChild(resultsPanel);
    return resultsPanel;
}

/**
 * Position the body-level search results below or above the search field.
 * @returns {void}
 */
function positionResultsPanel() {
    const form = document.getElementById('repository-search');
    const panel = ensureResultsPanel();
    if (!(form instanceof HTMLElement) || panel.hidden) return;
    const rect = form.getBoundingClientRect();
    const viewportPadding = 12;
    const width = Math.min(Math.max(rect.width, 320), window.innerWidth - viewportPadding * 2);
    const left = Math.min(Math.max(rect.left, viewportPadding), window.innerWidth - width - viewportPadding);
    panel.style.left = `${left}px`;
    panel.style.width = `${width}px`;
    panel.style.top = `${rect.bottom + 7}px`;

    const panelHeight = panel.getBoundingClientRect().height;
    if (rect.bottom + panelHeight + 12 > window.innerHeight && rect.top > panelHeight + 12) {
        panel.style.top = `${rect.top - panelHeight - 7}px`;
        panel.classList.add('opens-upward');
    } else {
        panel.classList.remove('opens-upward');
    }
}

/**
 * Reveal the result panel with a non-layout-affecting transition.
 * @returns {void}
 */
function openResultsPanel() {
    const panel = ensureResultsPanel();
    if (closeTimer) {
        clearTimeout(closeTimer);
        closeTimer = 0;
    }
    panel.hidden = false;
    panel.classList.remove('is-leaving');
    positionResultsPanel();
    requestAnimationFrame(() => panel.classList.add('is-visible'));
}

/**
 * Hide the result panel after its exit transition.
 * @param {boolean} [immediate=false] - Skip animation during route teardown.
 * @returns {void}
 */
function closeResultsPanel(immediate = false) {
    const panel = ensureResultsPanel();
    if (closeTimer) clearTimeout(closeTimer);
    panel.classList.remove('is-visible');
    if (immediate || panel.hidden) {
        panel.classList.remove('is-leaving');
        panel.hidden = true;
        closeTimer = 0;
        return;
    }
    panel.classList.add('is-leaving');
    closeTimer = setTimeout(() => {
        panel.hidden = true;
        panel.classList.remove('is-leaving');
        closeTimer = 0;
    }, SEARCH_CLOSE_MS);
}

/**
 * Render one status message inside the search result panel.
 * @param {string} message - Localized status copy.
 * @param {boolean} [isError=false] - Whether to use error styling.
 * @returns {void}
 */
function renderSearchStatus(message, isError = false) {
    const panel = ensureResultsPanel();
    panel.replaceChildren(el('p', {
        class: `repository-search-status${isError ? ' is-error' : ''}`,
        role: 'status'
    }, message));
    openResultsPanel();
}

/**
 * Return a localized type label for a repository search result.
 * @param {string} type - Backend result type.
 * @returns {string} Localized type label.
 */
function searchResultTypeLabel(type) {
    const normalized = String(type || '').toUpperCase();
    if (normalized === 'DIRECTORY') return t('search.directory');
    if (normalized === 'PACKAGE') return t('search.package');
    if (normalized === 'IMAGE') return t('search.image');
    return t('search.file');
}

/**
 * Build one repository search result row.
 * @param {object} result - Search result record.
 * @returns {HTMLButtonElement} Navigable result button.
 */
function buildSearchResult(result) {
    const type = String(result?.type || '').toUpperCase();
    const row = el('button', {
        class: 'repository-search-result',
        type: 'button',
        role: 'option',
        'data-search-path': String(result?.path || ''),
        'data-search-type': type
    });
    const iconName = type === 'DIRECTORY' ? 'folder' : (type === 'PACKAGE' || type === 'IMAGE') ? 'box' : 'file';
    const icon = createIcon(iconName);
    icon.classList.add('repository-search-result-icon');
    const copy = el('span', {class: 'repository-search-result-copy'},
        el('strong', {}, String(result?.name || '')),
        el('span', {}, String(result?.description || result?.path || ''))
    );
    const meta = el('span', {class: 'repository-search-result-meta'}, searchResultTypeLabel(type));
    if (result?.latest_version) {
        meta.appendChild(el('span', {class: 'repository-search-version'}, String(result.latest_version)));
    }
    row.append(icon, copy, meta);
    return row;
}

/**
 * Render a completed repository search response.
 * @param {object} payload - Search response payload.
 * @returns {void}
 */
function renderSearchResults(payload) {
    const panel = ensureResultsPanel();
    const results = Array.isArray(payload?.results) ? payload.results : [];
    panel.replaceChildren();
    if (results.length === 0) {
        panel.appendChild(el('p', {class: 'repository-search-status', role: 'status'}, t('search.noResults')));
    } else {
        const summary = t('search.resultCount', {count: Number(payload?.total || results.length)});
        panel.appendChild(el('p', {class: 'repository-search-summary'}, summary));
        const list = el('div', {class: 'repository-search-result-list'});
        for (const result of results) list.appendChild(buildSearchResult(result));
        panel.appendChild(list);
        if (payload?.has_more === true) {
            panel.appendChild(el('p', {class: 'repository-search-more'}, t('search.moreResults')));
        }
    }
    openResultsPanel();
}

/**
 * Fetch bounded search results for the current repository.
 * @param {string} query - User-entered search text.
 * @param {number} version - Input generation owning this request.
 * @returns {Promise<void>}
 */
async function fetchRepositorySearch(query, version) {
    if (!activeRepository) return;
    if (requestController) requestController.abort();
    requestController = new AbortController();
    try {
        const {response, data: payload} = await fetchProto(
            `/api/repositories/search/${encodeURIComponent(activeRepository)}?q=${encodeURIComponent(query)}&limit=20`,
            RepositorySearchResponse,
            {signal: requestController.signal}
        );
        if (version !== searchVersion) return;
        if (!response.ok || !payload) throw new Error(`HTTP ${response.status}`);
        renderSearchResults(payload);
    } catch (error) {
        if (error?.name === 'AbortError' || version !== searchVersion) return;
        console.error('Repository search failed', error);
        renderSearchStatus(t('search.failed'), true);
    }
}


/**
 * Debounce repository search as the user types.
 * @param {InputEvent} event - Search input event.
 * @returns {void}
 */
function handleSearchInput(event) {
    if (searchTimer) clearTimeout(searchTimer);
    const input = event.currentTarget;
    const query = String(input?.value || '').trim();
    document.getElementById('repository-search-clear')?.toggleAttribute('hidden', query.length === 0);
    const version = ++searchVersion;
    if (!query) {
        if (requestController) requestController.abort();
        closeResultsPanel();
        return;
    }
    renderSearchStatus(t('search.searching'));
    searchTimer = setTimeout(fetchRepositorySearch.bind(null, query, version), SEARCH_DELAY_MS);
}

/**
 * Clear the current query and close its result panel.
 * @param {boolean} [focus=false] - Focus the input after clearing it.
 * @returns {void}
 */
function clearRepositorySearch(focus = false) {
    const input = document.getElementById('repository-search-input');
    if (input instanceof HTMLInputElement) {
        input.value = '';
        if (focus) input.focus();
    }
    document.getElementById('repository-search-clear')?.setAttribute('hidden', '');
    searchVersion++;
    if (requestController) requestController.abort();
    closeResultsPanel();
}

/**
 * Clear the repository search from its visible clear button.
 * @returns {void}
 */
function handleSearchClearClick() {
    clearRepositorySearch(true);
}

/**
 * Keep Enter inside the live-search form from navigating or reloading the page.
 * @param {SubmitEvent} event - Search form submission.
 * @returns {void}
 */
function handleSearchSubmit(event) {
    event.preventDefault();
}

/**
 * Navigate to or download a selected search result.
 * @param {MouseEvent} event - Delegated result click.
 * @returns {void}
 */
function handleResultClick(event) {
    const row = event.target.closest('[data-search-path]');
    if (!(row instanceof HTMLElement)) return;
    const path = resultPath(activeRepository, row.dataset.searchPath || '');
    closeResultsPanel();
    if (row.dataset.searchType === 'FILE') {
        window.location.assign(path);
        return;
    }
    if (typeof activeNavigate === 'function') activeNavigate(path);
}

/**
 * Close search results when focus moves elsewhere or Escape is pressed.
 * @param {KeyboardEvent} event - Search input key event.
 * @returns {void}
 */
function handleSearchKeydown(event) {
    if (event.key === 'Escape') closeResultsPanel();
}

/**
 * Close search results after an outside click.
 * @param {MouseEvent} event - Document click event.
 * @returns {void}
 */
function handleDocumentClick(event) {
    const form = document.getElementById('repository-search');
    if (form?.contains(event.target) || resultsPanel?.contains(event.target)) return;
    closeResultsPanel();
}

/**
 * Keep an open body-level panel anchored during scrolling.
 * @returns {void}
 */
function handleViewportChange() {
    if (resultsPanel && !resultsPanel.hidden) positionResultsPanel();
}

/**
 * Attach stable repository search listeners once.
 * @returns {void}
 */
function initializeRepositorySearch() {
    if (initialized) return;
    initialized = true;
    document.getElementById('repository-search')?.addEventListener('submit', handleSearchSubmit);
    document.getElementById('repository-search-input')?.addEventListener('input', handleSearchInput);
    document.getElementById('repository-search-input')?.addEventListener('keydown', handleSearchKeydown);
    document.getElementById('repository-search-clear')?.addEventListener('click', handleSearchClearClick);
    document.addEventListener('click', handleDocumentClick);
    window.addEventListener('scroll', handleViewportChange, {passive: true});
    window.addEventListener('resize', handleViewportChange, {passive: true});
}

/**
 * Return the localized search placeholder for a repository format.
 * @param {string} format - Repository format identifier.
 * @returns {string} Localized placeholder copy.
 */
function getSearchPlaceholder(format) {
    if (format === 'cargo') return t('search.cargoPlaceholder');
    if (format === 'docker') return t('search.dockerPlaceholder') || t('docker.searchPlaceholder');
    if (format === 'files') return t('search.filesPlaceholder');
    return t('search.mavenPlaceholder');
}

/**
 * Configure search for the active repository or hide it outside repositories.
 * @param {string} repository - Active repository name, or an empty string.
 * @param {string} format - Normalized repository format.
 * @param {(path: string) => void} navigate - In-app navigation callback.
 * @returns {void}
 */
export function updateRepositorySearch(repository, format, navigate) {
    initializeRepositorySearch();
    const form = document.getElementById('repository-search');
    const input = document.getElementById('repository-search-input');
    if (!form || !(input instanceof HTMLInputElement)) return;
    const changed = repository !== activeRepository || format !== activeFormat;
    activeRepository = repository;
    activeFormat = format || 'maven';
    activeNavigate = navigate;
    form.hidden = !activeRepository;
    input.placeholder = getSearchPlaceholder(activeFormat);
    input.setAttribute('aria-label', input.placeholder);
    if (changed || !activeRepository) clearRepositorySearch();
}

/**
 * Refresh localized search copy without discarding an active query.
 * @returns {void}
 */
export function localizeRepositorySearch() {
    const input = document.getElementById('repository-search-input');
    if (!(input instanceof HTMLInputElement) || !activeRepository) return;
    input.placeholder = getSearchPlaceholder(activeFormat);
    input.setAttribute('aria-label', input.placeholder);
}
