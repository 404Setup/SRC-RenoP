/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {t} from './i18n.js';
import {showAlert} from './alert.js';
import {apiRequest, fetchProto, getAuthHeaders} from './api.js';
import {createUserRow, createUsersSkeletonRow} from './components.js';
import {editToken as openEditModal, initUsersModal, setTokensRefreshHandler} from './users/modal.js';
import {el} from '@renop/ui/dom';
import {makeCustomSelect} from '@renop/ui/custom-select';
import {logout} from './auth.js';
import {AccessTokenList, FidoDeviceList, GenerateTokenResponse} from './proto/index.js';
import {openSessionsDialog} from './sessions.js';
import {closeModalWithAnim} from './app-ui.js';
import {openAuditLogsDialog} from './audit.js';
import {morphElementHeight} from '@renop/ui/height-anim';
import {formatTimestamp, timestampMilliseconds} from './time.js';

let previousStats = {total: -1, admin: -1, key: -1};
let allTokens = [];
let currentPage = 1;
let pageSize = 5;
let searchQuery = '';
/** @type {'name' | 'created_at' | 'permissions'} */
let sortKey = 'name';
/** @type {'asc' | 'desc'} */
let sortDir = 'asc';
let sortHeadersReady = false;
let userFidoLoadSeq = 0;

/**
 * Locale-aware string compare by first character / full string for table sorting.
 * @param {string} a
 * @param {string} b
 * @returns {number}
 */
function compareByFirstChar(a, b) {
    return String(a || '').localeCompare(String(b || ''), undefined, {
        sensitivity: 'base',
        numeric: true,
        caseFirst: 'upper'
    });
}

/**
 * Build a sortable string key from a token's formatted permission tags.
 * @param {{ permissions?: string[] }} token
 * @returns {string}
 */
function permissionsSortKey(token) {
    const perms = token.permissions || [];
    if (perms.length === 0) return '';
    return perms
        .map(p => formatPermissionTag(p))
        .join(', ')
        .toLowerCase();
}

/**
 * Return a new array of tokens sorted by the current sort key and direction.
 * @param {object[]} tokens
 * @returns {object[]}
 */
function sortTokens(tokens) {
    return [...tokens].sort((a, b) => {
        let cmp;
        if (sortKey === 'created_at') {
            const ta = timestampMilliseconds(a.created_at || 0);
            const tb = timestampMilliseconds(b.created_at || 0);
            cmp = (Number.isFinite(ta) ? ta : 0) - (Number.isFinite(tb) ? tb : 0);
        } else if (sortKey === 'permissions') {
            cmp = compareByFirstChar(permissionsSortKey(a), permissionsSortKey(b));
        } else {
            cmp = compareByFirstChar(a.name, b.name);
        }
        if (cmp === 0 && sortKey !== 'name') {
            cmp = compareByFirstChar(a.name, b.name);
        }
        return sortDir === 'asc' ? cmp : -cmp;
    });
}

/**
 * Sync sortable column header classes and aria-sort with the current sort state.
 */
function updateSortHeaderUI() {
    const headers = document.querySelectorAll('#tokens-table thead th.sortable-th');
    headers.forEach(th => {
        const key = th.dataset.sortKey;
        const isActive = key === sortKey;
        th.classList.toggle('is-sorted', isActive);
        th.classList.toggle('is-sorted-asc', isActive && sortDir === 'asc');
        th.classList.toggle('is-sorted-desc', isActive && sortDir === 'desc');
        th.setAttribute('aria-sort', isActive ? (sortDir === 'asc' ? 'ascending' : 'descending') : 'none');
    });
}

/**
 * Make users-table column headers sortable (idempotent after first setup).
 */
function setupUsersSortHeaders() {
    if (sortHeadersReady) {
        updateSortHeaderUI();
        return;
    }
    const table = document.getElementById('tokens-table');
    if (!table) return;

    const headerRow = table.querySelector('thead tr');
    if (!headerRow) return;

    const sortConfig = [
        {index: 0, key: 'name', i18n: 'users.thUser'},
        {index: 1, key: 'permissions', i18n: 'users.thPermissions'},
        {index: 2, key: 'created_at', i18n: 'users.thCreatedAt'},
    ];

    sortConfig.forEach(({index, key, i18n}) => {
        const th = headerRow.children[index];
        if (!th) return;
        const i18nKey = th.getAttribute('data-i18n') || i18n;
        th.removeAttribute('data-i18n');
        th.classList.add('sortable-th');
        th.dataset.sortKey = key;
        th.dataset.i18nKey = i18nKey;
        th.setAttribute('role', 'columnheader');
        th.setAttribute('tabindex', '0');
        th.title = t(i18nKey);

        th.innerHTML = '';
        const label = document.createElement('span');
        label.className = 'th-sort-label';
        label.dataset.i18nKey = i18nKey;
        label.appendChild(document.createTextNode(t(i18nKey)));
        const indicator = document.createElement('span');
        indicator.className = 'th-sort-indicator';
        indicator.setAttribute('aria-hidden', 'true');
        label.appendChild(indicator);
        th.appendChild(label);

        const activate = () => {
            if (sortKey === key) {
                sortDir = sortDir === 'asc' ? 'desc' : 'asc';
            } else {
                sortKey = key;
                sortDir = key === 'created_at' ? 'desc' : 'asc';
            }
            currentPage = 1;
            updateSortHeaderUI();
            renderUsersPage();
        };

        th.addEventListener('click', activate);
        th.addEventListener('keydown', (e) => {
            if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                activate();
            }
        });
    });

    sortHeadersReady = true;
    updateSortHeaderUI();
}

/**
 * Update a stats counter element and pulse when its value changes.
 * @param {string} elId - Element id of the stat value node
 * @param {'total'|'admin'|'key'} key - Key into previousStats
 * @param {number} newValue
 */
function updateStatWithPulse(elId, key, newValue) {
    const el = document.getElementById(elId);
    if (!el) return;
    const oldVal = previousStats[key];
    el.textContent = newValue;
    if (oldVal !== -1 && oldVal !== newValue) {
        el.classList.remove('stat-value-update');
        void el.offsetWidth;
        el.classList.add('stat-value-update');
    }
    previousStats[key] = newValue;
}

/**
 * Attach the users search input handler and ensure sort headers are set up.
 */
function setupUsersSearch() {
    const searchInput = document.getElementById('users-search-input');
    if (!searchInput) return;

    searchInput.oninput = (e) => {
        searchQuery = e.target.value;
        currentPage = 1;
        renderUsersPage();
    };
    setupUsersSortHeaders();
}

/**
 * Map a raw permission string to a localized display tag.
 * @param {string} perm
 * @returns {string}
 */
export function formatPermissionTag(perm) {
    if (!perm || typeof perm !== 'string') return perm;
    const p = perm.trim();
    const upper = p.toUpperCase();

    if (upper === 'ADMIN' || upper === 'MANAGER' || upper === 'ACCESS-TOKEN:MANAGER') return t('users.tagAdmin');
    if (upper === 'BASE') return t('users.tagBase');
    if (upper === 'SHOWING') return t('users.tagShowing');
    if (upper === 'ALLVIEW') return t('users.tagAllview');

    if (upper === 'CANVIEW:*' || upper === 'ROUTE:READ' || upper === 'READ') return t('users.tagCanviewAll');
    if (upper === 'CANUPDATE:*' || upper === 'ROUTE:WRITE' || upper === 'WRITE') return t('users.tagCanupdateAll');

    if (upper.startsWith('CANVIEW:')) {
        const target = p.substring(8).trim();
        const repoName = target.includes(':') ? target.split(':').slice(1).join(':') : target;
        return t('users.tagCanviewRepo', {repo: repoName});
    }

    if (upper.startsWith('CANUPDATE:')) {
        const target = p.substring(10).trim();
        const repoName = target.includes(':') ? target.split(':').slice(1).join(':') : target;
        return t('users.tagCanupdateRepo', {repo: repoName});
    }

    return p;
}

export async function openUserFidoDialog(username) {
    const modal = document.getElementById('user-fido-modal');
    const title = document.getElementById('user-fido-modal-title');
    const list = document.getElementById('user-fido-list');
    const closeBtn = document.getElementById('close-user-fido-modal');
    if (!modal || !list) return;

    if (title) {
        title.textContent = t('users.fidoModalTitle', {user: username}) || `FIDO Devices for "${username}"`;
    }

    const loadDevices = async () => {
        const seq = ++userFidoLoadSeq;
        void morphElementHeight(list, () => {
            list.replaceChildren(
                el('div', {class: 'sessions-loading'},
                    el('div', {class: 'sessions-loading-spinner', 'aria-hidden': 'true'}),
                    el('span', {}, t('fido.loading') || t('common.loading') || 'Loading...')
                )
            );
        }, {duration: 300});

        try {
            const {
                response,
                data
            } = await fetchProto(`/api/auth/users/${encodeURIComponent(username)}/fido`, FidoDeviceList);
            if (seq !== userFidoLoadSeq) return;

            if (!response.ok || !data) {
                await morphElementHeight(list, () => {
                    list.replaceChildren(
                        el('div', {
                            style: {
                                padding: '1rem',
                                textAlign: 'center',
                                color: '#ef4444',
                            }
                        }, t('error.fidoLoadFailed') || 'Failed to load FIDO devices')
                    );
                }, {duration: 300});
                return;
            }
            const devices = data.devices || [];

            await morphElementHeight(list, () => {
                list.replaceChildren();
                if (!Array.isArray(devices) || devices.length === 0) {
                    list.appendChild(el('div', {
                        style: {
                            padding: '1.5rem',
                            textAlign: 'center',
                            opacity: '0.6',
                            fontSize: '0.9rem',
                        }
                    }, t('common.none') || 'No FIDO devices registered for this user'));
                    return;
                }

                devices.forEach(dev => {
                    const item = document.createElement('div');
                    item.className = 'fido-device-item';
                    item.style.cssText = 'display: flex; align-items: center; justify-content: space-between; padding: 0.6rem 0.8rem; border: 1px solid var(--border-color, #e5e7eb); border-radius: 8px; margin-bottom: 0.5rem; background: var(--card-bg, #ffffff);';

                    const info = document.createElement('div');
                    info.style.cssText = 'display: flex; flex-direction: column; gap: 2px;';
                    const nameEl = document.createElement('span');
                    nameEl.style.cssText = 'font-weight: 600; font-size: 0.9rem;';
                    nameEl.textContent = dev.name || 'FIDO Device';
                    const dateEl = document.createElement('span');
                    dateEl.style.cssText = 'font-size: 0.78rem; opacity: 0.65;';
                    dateEl.textContent = formatTimestamp(dev.created_at, {fallback: t('common.unknown')});
                    info.appendChild(nameEl);
                    info.appendChild(dateEl);

                    const delBtn = document.createElement('button');
                    delBtn.type = 'button';
                    delBtn.className = 'pill-btn pill-btn--danger';
                    delBtn.style.cssText = 'padding: 4px 10px; font-size: 0.8rem;';
                    delBtn.textContent = t('common.delete') || 'Delete';
                    delBtn.addEventListener('click', async () => {
                        const confirmMsg = t('users.confirmDeleteUserFido', {
                            user: username,
                            name: dev.name
                        }) || `Are you sure you want to delete FIDO device "${dev.name}" for user "${username}"?`;
                        if (await window.showConfirm(confirmMsg)) {
                            try {
                                const delRes = await apiRequest(`/api/auth/users/${encodeURIComponent(username)}/fido/${dev.id}`, {method: 'DELETE'});
                                if (delRes.ok) {
                                    showAlert(t('profile.fidoDeleted') || 'FIDO device deleted', 'success');
                                    loadDevices();
                                } else {
                                    showAlert(t('common.error') || 'Failed to delete FIDO device', 'error');
                                }
                            } catch (err) {
                                showAlert(t('common.error') || 'Failed to delete FIDO device', 'error');
                            }
                        }
                    });

                    item.appendChild(info);
                    item.appendChild(delBtn);
                    list.appendChild(item);
                });
            }, {duration: 300});
        } catch (err) {
            if (seq !== userFidoLoadSeq) return;
            console.error('Failed to load user FIDO devices:', err);
            await morphElementHeight(list, () => {
                list.replaceChildren(
                    el('div', {
                        style: {
                            padding: '1rem',
                            textAlign: 'center',
                            color: '#ef4444',
                        }
                    }, t('error.fidoLoadFailed') || 'Error loading FIDO devices')
                );
            }, {duration: 300});
        }
    };

    if (closeBtn && !closeBtn.dataset.listenerAttached) {
        closeBtn.dataset.listenerAttached = 'true';
        closeBtn.addEventListener('click', () => {
            userFidoLoadSeq += 1;
            closeModalWithAnim(modal);
        });
    }

    const backdrop = document.getElementById('user-fido-backdrop');
    if (backdrop && !backdrop.dataset.listenerAttached) {
        backdrop.dataset.listenerAttached = 'true';
        backdrop.addEventListener('click', () => {
            userFidoLoadSeq += 1;
            closeModalWithAnim(modal);
        });
    }

    modal.style.display = 'flex';
    if (window.updateModalInertState) window.updateModalInertState();
    loadDevices();
}

/**
 * Create a users-table row wired to edit/delete/reset/sessions/fido handlers.
 * @param {object} token
 * @returns {HTMLTableRowElement}
 */
function createUserRowElement(token) {
    return createUserRow(token, {
        formatPermissionTag,
        onEdit: (t) => openEditModal(t),
        onDelete: (t) => deleteToken(t.name),
        onReset: (t) => regenerateUserToken(t.name),
        onSessions: (tok) => openSessionsDialog({mode: 'admin', username: tok.name}),
        onFido: (tok) => openUserFidoDialog(tok.name),
        onAuditLogs: (tok) => openAuditLogsDialog({mode: 'user', username: tok.name}),
    });
}

/**
 * Render skeleton placeholder rows while tokens are loading (if empty).
 */
function renderUsersSkeleton() {
    const tbody = document.getElementById('tokens-table-body');
    if (!tbody || tbody.querySelectorAll('tr[data-user-name]').length > 0) return;
    tbody.classList.remove('is-content-entering');
    tbody.replaceChildren(createUsersSkeletonRow(), createUsersSkeletonRow());
}

/**
 * Render pagination info, page-size select, and page navigation controls.
 * @param {number} startIdx - Zero-based start index of the current page slice
 * @param {number} endIdx - Exclusive end index of the current page slice
 * @param {number} totalItems
 * @param {number} totalPages
 */
function renderPaginationControls(startIdx, endIdx, totalItems, totalPages) {
    const container = document.getElementById('users-pagination');
    if (!container) return;

    container.innerHTML = '';

    const startNum = totalItems === 0 ? 0 : startIdx + 1;
    const endNum = endIdx;

    const infoSpan = document.createElement('span');
    infoSpan.className = 'pagination-info-text';
    infoSpan.textContent = t('users.paginationShowing', {
        start: startNum,
        end: endNum,
        total: totalItems
    }) || `Showing ${startNum}-${endNum} of ${totalItems}`;

    document.querySelectorAll('.pagination-custom-select-dropdown').forEach(d => d.remove());

    const selectOptions = [5, 10, 20, 30].map(size => ({
        value: size,
        label: t('users.paginationPerPage', {size}) || `${size} / page`
    }));

    const sizeSelectWrapper = makeCustomSelect(selectOptions, pageSize, (newSize) => {
        const val = parseInt(newSize, 10);
        if (val !== pageSize) {
            pageSize = val;
            currentPage = 1;
            renderUsersPage('next');
        }
    });
    sizeSelectWrapper.classList.add('pagination-size-custom-select');

    const lastDropdown = document.body.lastElementChild;
    if (lastDropdown && lastDropdown.classList.contains('custom-select-dropdown')) {
        lastDropdown.classList.add('pagination-custom-select-dropdown');
    }

    const infoWrapper = document.createElement('div');
    infoWrapper.className = 'pagination-info';
    infoWrapper.appendChild(infoSpan);
    infoWrapper.appendChild(sizeSelectWrapper);

    const controlsWrapper = document.createElement('div');
    controlsWrapper.className = 'pagination-controls';

    const prevBtn = document.createElement('button');
    prevBtn.type = 'button';
    prevBtn.className = 'pagination-btn';
    prevBtn.disabled = currentPage <= 1;
    prevBtn.title = t('users.paginationPrev') || 'Previous';
    prevBtn.innerHTML = `&#8249;`;
    prevBtn.addEventListener('click', () => {
        if (currentPage > 1) {
            currentPage--;
            renderUsersPage('prev');
        }
    });
    controlsWrapper.appendChild(prevBtn);

    /**
     * Build a compact list of page numbers and ellipsis markers for pagination.
     * @param {number} current - Current page (1-based)
     * @param {number} total - Total pages
     * @returns {(number|string)[]}
     */
    const getPageNumbers = (current, total) => {
        if (total <= 7) {
            return Array.from({length: total}, (_, i) => i + 1);
        }
        const pages = [];
        pages.push(1);
        if (current > 3) pages.push('...');
        const start = Math.max(2, current - 1);
        const end = Math.min(total - 1, current + 1);
        for (let i = start; i <= end; i++) pages.push(i);
        if (current < total - 2) pages.push('...');
        pages.push(total);
        return pages;
    };

    const pageNumbers = getPageNumbers(currentPage, totalPages);
    pageNumbers.forEach(p => {
        if (p === '...') {
            const ellipsis = document.createElement('span');
            ellipsis.className = 'pagination-ellipsis';
            ellipsis.textContent = '...';
            ellipsis.style.padding = '0 0.25rem';
            ellipsis.style.opacity = '0.5';
            ellipsis.style.fontSize = '0.82rem';
            controlsWrapper.appendChild(ellipsis);
        } else {
            const btn = document.createElement('button');
            btn.type = 'button';
            btn.className = `pagination-btn${p === currentPage ? ' is-active' : ''}`;
            btn.textContent = String(p);
            btn.addEventListener('click', () => {
                if (p !== currentPage) {
                    const dir = p > currentPage ? 'next' : 'prev';
                    currentPage = p;
                    renderUsersPage(dir);
                }
            });
            controlsWrapper.appendChild(btn);
        }
    });

    const nextBtn = document.createElement('button');
    nextBtn.type = 'button';
    nextBtn.className = 'pagination-btn';
    nextBtn.disabled = currentPage >= totalPages;
    nextBtn.title = t('users.paginationNext') || 'Next';
    nextBtn.innerHTML = `&#8250;`;
    nextBtn.addEventListener('click', () => {
        if (currentPage < totalPages) {
            currentPage++;
            renderUsersPage('next');
        }
    });
    controlsWrapper.appendChild(nextBtn);

    container.appendChild(infoWrapper);
    container.appendChild(controlsWrapper);
}

/**
 * Filter, sort, and render the current page of users in the table.
 * @param {'next'|'prev'|null} [direction] - Optional page-transition animation direction
 */
function renderUsersPage(direction = null) {
    const tbody = document.getElementById('tokens-table-body');
    if (!tbody) return;

    const query = searchQuery.toLowerCase().trim();
    const filteredTokens = allTokens.filter(token => {
        if (!query) return true;
        const name = (token.name || '').toLowerCase();
        const perms = (token.permissions || []).map(p => formatPermissionTag(p).toLowerCase()).join(' ');
        return name.includes(query) || perms.includes(query);
    });
    const sortedTokens = sortTokens(filteredTokens);

    const totalItems = sortedTokens.length;
    const totalPages = Math.max(1, Math.ceil(totalItems / pageSize));
    if (currentPage > totalPages) {
        currentPage = totalPages;
    }
    if (currentPage < 1) {
        currentPage = 1;
    }

    const startIdx = (currentPage - 1) * pageSize;
    const endIdx = Math.min(startIdx + pageSize, totalItems);
    const pageTokens = sortedTokens.slice(startIdx, endIdx);

    tbody.innerHTML = '';
    if (pageTokens.length === 0) {
        const emptyTr = document.createElement('tr');
        emptyTr.innerHTML = `<td colspan="4" style="text-align: center; padding: 2rem; opacity: 0.6;">${t('common.none') || 'None'}</td>`;
        tbody.appendChild(emptyTr);
    } else {
        pageTokens.forEach(token => {
            tbody.appendChild(createUserRowElement(token));
        });
    }

    tbody.classList.remove('is-page-enter-next', 'is-page-enter-prev', 'is-content-entering');
    void tbody.offsetWidth;
    if (direction === 'next') {
        tbody.classList.add('is-page-enter-next');
    } else if (direction === 'prev') {
        tbody.classList.add('is-page-enter-prev');
    } else {
        tbody.classList.add('is-content-entering');
    }

    renderPaginationControls(startIdx, endIdx, totalItems, totalPages);
}

/**
 * Load access tokens from the API, update stats, and refresh the users table.
 * @returns {Promise<void>}
 */
export async function fetchTokens() {
    renderUsersSkeleton();
    try {
        const {response, data} = await fetchProto('/api/tokens', AccessTokenList);
        if (response.ok && data) {
            allTokens = data.tokens || [];

            const totalUsers = allTokens.length;
            const adminUsers = allTokens.filter(t => t.permissions && (t.permissions.includes('admin') || t.permissions.includes('manager'))).length;
            const keyUsers = allTokens.filter(t => t.tokens && t.tokens.length > 0).length;

            updateStatWithPulse('stat-total-users', 'total', totalUsers);
            updateStatWithPulse('stat-admin-users', 'admin', adminUsers);
            updateStatWithPulse('stat-key-users', 'key', keyUsers);

            renderUsersPage();
            setupUsersSearch();
        } else if (response.status === 401 || response.status === 403) {
            logout('kicked');
        }
    } catch (e) {
        console.error('Failed to fetch tokens', e);
    }
}

/**
 * Open the edit-user modal for the given token.
 * @param {object} token
 * @returns {Promise<void>}
 */
export async function editToken(token) {
    openEditModal(token);
}

/**
 * Confirm and delete a user/token by name, then refresh the list.
 * @param {string} name
 * @returns {Promise<void>}
 */
export async function deleteToken(name) {
    if (!(await window.showConfirm(t('users.confirmDeleteToken', {name})))) return;

    const row = Array.from(document.querySelectorAll('#tokens-table-body tr')).find(r => r.dataset.userName === name);
    if (row) {
        row.classList.add('user-row--deleting');
    }

    try {
        const headers = getAuthHeaders();
        const response = await fetch(`/api/tokens/${name}`, {method: 'DELETE', headers});
        if (response.ok) {
            setTimeout(() => {
                fetchTokens();
            }, 200);
        } else {
            if (row) row.classList.remove('user-row--deleting');
            const errText = await response.text();
            showAlert(errText || t('users.failedDeleteToken'), 'error');
        }
    } catch (e) {
        if (row) row.classList.remove('user-row--deleting');
        console.error('Failed to delete token', e);
    }
}

/**
 * Confirm and regenerate a user's API token, then show the new value.
 * @param {string} name
 * @returns {Promise<void>}
 */
export async function regenerateUserToken(name) {
    if (!(await window.showConfirm(t('users.confirmRegenToken', {name})))) return;

    try {
        const {response, data} = await fetchProto(`/api/tokens/${name}/token`, GenerateTokenResponse, {method: 'POST'});
        if (response.ok && data) {
            const row = Array.from(document.querySelectorAll('#tokens-table-body tr')).find(r => r.dataset.userName === name);
            if (row) {
                row.classList.add('user-row--reset-flash');
                setTimeout(() => row.classList.remove('user-row--reset-flash'), 1000);
            }
            window.showPrompt(t('users.newTokenPrompt', {name}), data.token, true);
            fetchTokens();
        } else {
            const errText = await response.text();
            showAlert(errText || t('users.failedRegenToken'), 'error');
        }
    } catch (e) {
        console.error('Failed to regenerate token', e);
    }
}

/**
 * Refresh users-table header labels and re-render rows after a language change.
 */
export function updateUsersTableLanguage() {
    setupUsersSortHeaders();
    document.querySelectorAll('#tokens-table thead th.sortable-th').forEach(th => {
        const key = th.dataset.i18nKey;
        if (!key) return;
        th.title = t(key);
        const label = th.querySelector('.th-sort-label');
        if (!label) return;
        const indicator = label.querySelector('.th-sort-indicator');
        label.replaceChildren(document.createTextNode(t(key)));
        if (indicator) label.appendChild(indicator);
    });
    updateSortHeaderUI();
    renderUsersPage();
}

window.addEventListener('languageChanged', () => {
    updateUsersTableLanguage();
});

setTokensRefreshHandler(fetchTokens);

/**
 * Initialize users page search, sort headers, and create-user modal wiring.
 */
function initUsersPage() {
    setupUsersSearch();
    initUsersModal();
}

if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', initUsersPage);
} else {
    queueMicrotask(initUsersPage);
}
