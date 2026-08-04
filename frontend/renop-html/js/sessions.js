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
import {el} from './cfg-ui.js';
import {fetchProto, PROTO_CONTENT_TYPE, withCredentials} from './api.js';
import {SessionList} from './proto/index.js';
import {RenopDialog} from './components/dialog.js';
import {createButton} from './components/button.js';
import {createBadge} from './components/badge.js';
import {createEmptyState} from './components/empty-state.js';
import {logout} from './auth.js';

/**
 * @typedef {object} SessionView
 * @property {string} public_id
 * @property {string} username
 * @property {string} ip
 * @property {string} user_agent
 * @property {number} created_at
 * @property {number} last_active
 * @property {number} expires_at
 * @property {boolean} current
 * @property {string} [login_method]
 */

/**
 * Resolve display text for login method.
 * @param {string} [method]
 * @returns {string}
 */
function formatLoginMethod(method) {
    if (method === 'fido') {
        return t('sessions.methodFido');
    }
    return t('sessions.methodPassword');
}

/**
 * Shorten public id to an 8-character identification string.
 * @param {string} [publicId]
 * @returns {string}
 */
function shortSessionId(publicId) {
    const id = (publicId || '').trim();
    if (!id) return '—';
    if (id.length <= 8) return id;
    return id.slice(0, 8);
}

/**
 * @typedef {object} SessionsDialogOptions
 * @property {'self'|'admin'} mode
 * @property {string} [username]
 */

/**
 * Format a millisecond timestamp for display, or return an em dash if invalid.
 * @param {number|string|null|undefined} ms - Epoch milliseconds (or value coercible to number)
 * @returns {string}
 */
function formatDateTime(ms) {
    if (!ms || !Number.isFinite(Number(ms))) return '—';
    try {
        return new Date(Number(ms)).toLocaleString();
    } catch {
        return '—';
    }
}

/**
 * Shorten a user-agent string for the sessions table device column.
 * @param {string} [userAgent]
 * @returns {string}
 */
function shortDevice(userAgent) {
    const ua = (userAgent || '').trim();
    if (!ua) return t('sessions.unknownDevice');
    if (ua.length <= 72) return ua;
    return ua.slice(0, 69) + '…';
}

/**
 * Build the API URL used to list sessions for the given dialog mode.
 * @param {SessionsDialogOptions} opts
 * @returns {string}
 */
function listUrl(opts) {
    if (opts.mode === 'admin') {
        return `/api/tokens/${encodeURIComponent(opts.username)}/sessions`;
    }
    return '/api/auth/profile/sessions';
}

/**
 * Build the API URL used to revoke a single session.
 * @param {SessionsDialogOptions} opts
 * @param {string} publicId - Session public id
 * @returns {string}
 */
function revokeOneUrl(opts, publicId) {
    if (opts.mode === 'admin') {
        return `/api/tokens/${encodeURIComponent(opts.username)}/sessions/${encodeURIComponent(publicId)}`;
    }
    return `/api/auth/profile/sessions/${encodeURIComponent(publicId)}`;
}

/**
 * Build the API URL used for bulk session revoke (all / others).
 * @param {SessionsDialogOptions} opts
 * @returns {string}
 */
function revokeBulkUrl(opts) {
    if (opts.mode === 'admin') {
        return `/api/tokens/${encodeURIComponent(opts.username)}/sessions/revoke-all`;
    }
    return '/api/auth/profile/sessions/revoke-others';
}

/**
 * Fetch the session list for the current dialog mode.
 * @param {SessionsDialogOptions} opts
 * @returns {Promise<SessionView[]>}
 */
async function loadSessions(opts) {
    const {response, data} = await fetchProto(listUrl(opts), SessionList);
    if (response.status === 401 || response.status === 403) {
        if (opts.mode === 'self') logout('kicked');
        throw new Error(response.status === 403 ? 'Forbidden' : 'Unauthorized');
    }
    if (!response.ok) {
        const msg = await response.text().catch(() => '');
        throw new Error(msg || t('sessions.loadFailed'));
    }
    return data?.sessions || [];
}

/**
 * Perform a credentialed session mutation request (DELETE/POST).
 * @param {string} url
 * @param {string} method - HTTP method
 * @returns {Promise<Response>}
 */
async function mutateSessions(url, method) {
    return fetch(url, withCredentials({
        method,
        headers: {
            Accept: PROTO_CONTENT_TYPE,
            'Content-Type': PROTO_CONTENT_TYPE,
        },
        body: method === 'POST' ? new Uint8Array(0) : undefined,
    }));
}

/**
 * Revoke a single session by public id.
 * @param {SessionsDialogOptions} opts
 * @param {string} publicId
 * @returns {Promise<void>}
 */
async function revokeOne(opts, publicId) {
    const response = await mutateSessions(revokeOneUrl(opts, publicId), 'DELETE');
    if (response.status === 401 || response.status === 403) {
        if (opts.mode === 'self') logout('kicked');
        throw new Error(response.status === 403 ? 'Forbidden' : 'Unauthorized');
    }
    if (!response.ok) {
        const msg = await response.text().catch(() => '');
        throw new Error(msg || t('sessions.revokeFailed'));
    }
}

/**
 * Revoke all (admin) or other (self) sessions in bulk.
 * @param {SessionsDialogOptions} opts
 * @returns {Promise<void>}
 */
async function revokeBulk(opts) {
    const response = await mutateSessions(revokeBulkUrl(opts), 'POST');
    if (response.status === 401 || response.status === 403) {
        if (opts.mode === 'self') logout('kicked');
        throw new Error(response.status === 403 ? 'Forbidden' : 'Unauthorized');
    }
    if (!response.ok) {
        const msg = await response.text().catch(() => '');
        throw new Error(msg || t('sessions.revokeOthersFailed'));
    }
}

/**
 * Sort sessions with the current session first, then by last active descending.
 * @param {SessionView[]} sessions
 * @returns {SessionView[]}
 */
function sortSessions(sessions) {
    return [...sessions].sort((a, b) => {
        if (a.current && !b.current) return -1;
        if (!a.current && b.current) return 1;
        return (b.last_active || 0) - (a.last_active || 0);
    });
}

/**
 * Resolve after the given delay (used for leave animations).
 * @param {number} ms
 * @returns {Promise<void>}
 */
function waitMs(ms) {
    return new Promise((resolve) => setTimeout(resolve, ms));
}

/**
 * Enable click-drag (and trackpad-friendly) horizontal scrolling on a container.
 * @param {HTMLElement} container
 */
function enableDragScroll(container) {
    if (!container || container.dataset.dragScroll === '1') return;
    container.dataset.dragScroll = '1';

    let isDown = false;
    let startX = 0;
    let startScrollLeft = 0;
    let moved = false;

    const onMove = (clientX, e) => {
        if (!isDown) return;
        const dx = clientX - startX;
        if (Math.abs(dx) > 3) moved = true;
        if (moved && e) e.preventDefault();
        container.scrollLeft = startScrollLeft - dx;
    };

    const onMouseMove = (e) => onMove(e.clientX, e);
    const onMouseUp = () => {
        isDown = false;
        container.classList.remove('is-dragging');
        window.removeEventListener('mousemove', onMouseMove);
        window.removeEventListener('mouseup', onMouseUp);
    };

    container.addEventListener('mousedown', (e) => {
        if (e.button !== 0) return;
        if (e.target.closest('button, a, input, select, textarea, label')) return;
        isDown = true;
        moved = false;
        container.classList.add('is-dragging');
        startX = e.clientX;
        startScrollLeft = container.scrollLeft;
        window.addEventListener('mousemove', onMouseMove);
        window.addEventListener('mouseup', onMouseUp);
    });

    container.addEventListener('touchstart', (e) => {
        if (!e.touches[0]) return;
        if (e.target.closest('button, a, input, select, textarea, label')) return;
        isDown = true;
        moved = false;
        container.classList.add('is-dragging');
        startX = e.touches[0].clientX;
        startScrollLeft = container.scrollLeft;
    }, {passive: true});
    container.addEventListener('touchmove', (e) => {
        if (!e.touches[0]) return;
        onMove(e.touches[0].clientX, e);
    }, {passive: false});
    const onTouchEnd = () => {
        isDown = false;
        container.classList.remove('is-dragging');
    };
    container.addEventListener('touchend', onTouchEnd);
    container.addEventListener('touchcancel', onTouchEnd);

    container.addEventListener('wheel', (e) => {
        if (container.scrollWidth <= container.clientWidth + 1) return;
        const mostlyVertical = Math.abs(e.deltaY) > Math.abs(e.deltaX);
        if (mostlyVertical && e.deltaY !== 0) {
            container.scrollLeft += e.deltaY;
            e.preventDefault();
        }
    }, {passive: false});

    container.addEventListener('click', (e) => {
        if (moved) {
            e.preventDefault();
            e.stopPropagation();
            moved = false;
        }
    }, true);
}

/**
 * Open the sessions management dialog for the current user or an admin target.
 * @param {SessionsDialogOptions} [options]
 * @returns {Promise<unknown>} Resolves when the dialog closes
 */
export function openSessionsDialog(options = {mode: 'self'}) {
    const opts = {
        mode: options.mode === 'admin' ? 'admin' : 'self',
        username: options.username || '',
    };
    if (opts.mode === 'admin' && !opts.username) {
        showAlert(t('sessions.loadFailed'), 'error');
        return Promise.resolve(false);
    }

    const title = opts.mode === 'admin'
        ? t('sessions.adminTitle', {name: opts.username})
        : t('sessions.title');
    const subtitle = opts.mode === 'admin'
        ? t('sessions.adminSubtitle', {name: opts.username})
        : t('sessions.subtitle');

    const bodyRoot = el('div', {class: 'sessions-dialog-body'});
    const toolbar = el('div', {class: 'sessions-dialog-toolbar'});
    const listHost = el('div', {class: 'sessions-dialog-list'});
    bodyRoot.append(toolbar, listHost);

    /** @type {HTMLElement & { close?: (result?: unknown) => void } | null} */
    let dialogRef = null;
    /** @type {SessionView[]} */
    let sessions = [];
    let hasLoadedOnce = false;
    let refreshInFlight = false;

    const refreshBtn = createButton(t('sessions.refresh'), {
        class: 'pill-btn pill-btn--soft pill-btn--sm sessions-toolbar-btn',
        icon: 'refresh',
        iconProps: {width: '14', height: '14'},
        onClick: () => {
            refresh({animate: true});
        },
    });

    const bulkLabel = opts.mode === 'admin'
        ? t('sessions.revokeAll')
        : t('sessions.revokeOthers');
    const bulkBtn = createButton(bulkLabel, {
        class: 'pill-btn pill-btn--danger pill-btn--sm sessions-toolbar-btn',
        icon: 'delete',
        iconProps: {width: '14', height: '14'},
        onClick: async () => {
            const confirmMsg = opts.mode === 'admin'
                ? t('sessions.confirmRevokeAll', {name: opts.username})
                : t('sessions.confirmRevokeOthers');
            if (!(await window.showConfirm(confirmMsg))) return;
            try {
                bulkBtn.disabled = true;
                listHost.classList.add('is-busy');
                await revokeBulk(opts);
                showAlert(
                    opts.mode === 'admin' ? t('sessions.revokedAll') : t('sessions.revokedOthers'),
                    'success'
                );
                await refresh({animate: true, removeGone: true});
            } catch (e) {
                showAlert(e?.message || t('sessions.revokeOthersFailed'), 'error');
            } finally {
                listHost.classList.remove('is-busy');
                updateBulkDisabled();
            }
        },
    });

    toolbar.append(refreshBtn, bulkBtn);

    /**
     * Enable or disable the bulk-revoke button based on remaining sessions.
     */
    function updateBulkDisabled() {
        const hasCurrent = sessions.some(s => s.current);
        const others = sessions.filter(s => !s.current).length;
        bulkBtn.disabled = hasCurrent ? others === 0 : sessions.length === 0;
    }

    /**
     * Show the first-load spinner state in the sessions list host.
     */
    function showInitialLoading() {
        listHost.replaceChildren();
        listHost.appendChild(el('div', {class: 'sessions-loading'},
            el('div', {class: 'sessions-loading-spinner', 'aria-hidden': 'true'}),
            el('span', {}, t('sessions.loading')),
        ));
    }

    /**
     * Show the empty-state placeholder when no sessions are present.
     */
    function showEmpty() {
        listHost.replaceChildren();
        listHost.appendChild(createEmptyState({
            message: t('sessions.empty'),
            subtext: t('sessions.emptyHint'),
            icon: 'ssl',
        }));
    }

    /**
     * Ensure the sessions table structure exists and return its wrappers.
     * @returns {{ wrap: HTMLElement, tbody: HTMLElement }}
     */
    function ensureTable() {
        let wrap = listHost.querySelector('.sessions-table-wrap');
        let tbody = listHost.querySelector('.sessions-table tbody');
        if (wrap && tbody) {
            return {wrap, tbody};
        }

        listHost.replaceChildren();
        const table = el('table', {class: 'sessions-table'});
        const thead = el('thead', {},
            el('tr', {},
                el('th', {class: 'sessions-col-id'}, t('sessions.colId')),
                el('th', {}, t('sessions.colDevice')),
                el('th', {class: 'sessions-col-method'}, t('sessions.colMethod')),
                el('th', {}, t('sessions.colIp')),
                el('th', {}, t('sessions.colLastActive')),
                el('th', {}, t('sessions.colExpires')),
                el('th', {class: 'sessions-col-actions'}, t('sessions.colActions')),
            )
        );
        tbody = el('tbody');
        table.append(thead, tbody);
        wrap = el('div', {class: 'sessions-table-wrap'}, table);
        listHost.appendChild(wrap);
        enableDragScroll(wrap);
        return {wrap, tbody};
    }

    /**
     * Build a table row for one session, including its revoke action.
     * @param {SessionView} session
     * @returns {HTMLTableRowElement}
     */
    function buildRow(session) {
        const idSpan = el('span', {title: session.public_id || ''}, shortSessionId(session.public_id));
        const idCell = el('td', {class: 'sessions-mono sessions-col-id'}, idSpan);

        const deviceCell = el('td', {class: 'sessions-device-cell'});
        const deviceMain = el('div', {class: 'sessions-device-main'}, shortDevice(session.user_agent));
        deviceMain.title = session.user_agent || '';
        deviceCell.appendChild(deviceMain);
        const badgeHost = el('div', {class: 'sessions-badge-host'});
        if (session.current) {
            badgeHost.appendChild(createBadge(t('sessions.currentBadge'), 'admin'));
        }
        deviceCell.appendChild(badgeHost);

        const methodCell = el('td', {class: 'sessions-col-method'}, formatLoginMethod(session.login_method));

        const revokeBtn = createButton('', {
            class: 'table-action-btn delete-btn sessions-revoke-btn',
            icon: 'delete',
            title: t('sessions.revoke'),
            onClick: async () => {
                const msg = session.current
                    ? t('sessions.confirmRevokeCurrent')
                    : t('sessions.confirmRevoke');
                if (!(await window.showConfirm(msg))) return;
                try {
                    revokeBtn.disabled = true;
                    await revokeOne(opts, session.public_id);
                    if (session.current) {
                        showAlert(t('sessions.revokedCurrent'), 'success');
                        if (dialogRef && typeof dialogRef.close === 'function') {
                            dialogRef.close(true);
                        }
                        logout('kicked');
                        return;
                    }
                    showAlert(t('sessions.revoked'), 'success');
                    await animateRemoveRows([session.public_id]);
                    sessions = sessions.filter(s => s.public_id !== session.public_id);
                    if (!sessions.length) {
                        showEmpty();
                    }
                    updateBulkDisabled();
                } catch (e) {
                    revokeBtn.disabled = false;
                    showAlert(e?.message || t('sessions.revokeFailed'), 'error');
                }
            },
        });
        revokeBtn.appendChild(el('span', {class: 'btn-text'}, t('sessions.revoke')));

        const row = /** @type {HTMLTableRowElement} */ (el('tr', {
                class: session.current ? 'sessions-row sessions-row--current' : 'sessions-row',
            },
            idCell,
            deviceCell,
            methodCell,
            el('td', {class: 'sessions-mono sessions-col-ip'}, session.ip || '—'),
            el('td', {class: 'sessions-col-last-active'}, formatDateTime(session.last_active)),
            el('td', {class: 'sessions-col-expires'}, formatDateTime(session.expires_at)),
            el('td', {class: 'sessions-col-actions'}, revokeBtn),
        ));
        row.dataset.publicId = session.public_id;
        return row;
    }

    /**
     * Update an existing session row in place with fresh session data.
     * @param {SessionView} session
     * @param {HTMLTableRowElement} row
     */
    function updateRow(session, row) {
        row.classList.toggle('sessions-row--current', !!session.current);
        const idCell = row.querySelector('.sessions-col-id span');
        if (idCell) {
            idCell.textContent = shortSessionId(session.public_id);
            idCell.title = session.public_id || '';
        }
        const deviceMain = row.querySelector('.sessions-device-main');
        if (deviceMain) {
            deviceMain.textContent = shortDevice(session.user_agent);
            deviceMain.title = session.user_agent || '';
        }
        const badgeHost = row.querySelector('.sessions-badge-host');
        if (badgeHost) {
            badgeHost.replaceChildren();
            if (session.current) {
                badgeHost.appendChild(createBadge(t('sessions.currentBadge'), 'admin'));
            }
        }
        const methodCell = row.querySelector('.sessions-col-method');
        if (methodCell) methodCell.textContent = formatLoginMethod(session.login_method);
        const ipCell = row.querySelector('.sessions-col-ip');
        if (ipCell) ipCell.textContent = session.ip || '—';
        const lastCell = row.querySelector('.sessions-col-last-active');
        if (lastCell) lastCell.textContent = formatDateTime(session.last_active);
        const expCell = row.querySelector('.sessions-col-expires');
        if (expCell) expCell.textContent = formatDateTime(session.expires_at);
    }

    /**
     * Animate and remove rows matching the given session public ids.
     * @param {string[]} publicIds
     * @returns {Promise<void>}
     */
    async function animateRemoveRows(publicIds) {
        if (!publicIds.length) return;
        const idSet = new Set(publicIds);
        const rows = [...listHost.querySelectorAll('tr.sessions-row')].filter((row) => {
            const id = row.dataset.publicId;
            return id && idSet.has(id);
        });
        if (!rows.length) return;

        rows.forEach((row) => {
            row.style.height = `${row.getBoundingClientRect().height}px`;
            row.classList.add('sessions-row--leaving');
        });
        void listHost.offsetHeight;
        rows.forEach((row) => {
            row.style.height = '0px';
            row.style.paddingTop = '0';
            row.style.paddingBottom = '0';
            row.style.opacity = '0';
        });
        await waitMs(220);
        rows.forEach((row) => row.remove());
    }

    /**
     * Sync list DOM with `sessions` without wiping the dialog shell.
     * @param {{ removeGone?: boolean, animateIn?: boolean }} [options]
     * @returns {Promise<void>}
     */
    async function renderSessions({removeGone = true, animateIn = true} = {}) {
        if (!sessions.length) {
            showEmpty();
            updateBulkDisabled();
            return;
        }

        const sorted = sortSessions(sessions);
        const {tbody} = ensureTable();
        const existing = new Map(
            [...tbody.querySelectorAll('tr.sessions-row')].map((row) => [row.dataset.publicId, row])
        );
        const nextIds = new Set(sorted.map((s) => s.public_id));

        if (removeGone) {
            const gone = [...existing.keys()].filter((id) => id && !nextIds.has(id));
            if (gone.length) {
                await animateRemoveRows(gone);
                gone.forEach((id) => existing.delete(id));
            }
        }

        const frag = document.createDocumentFragment();
        const newRows = [];

        sorted.forEach((session) => {
            let row = existing.get(session.public_id);
            if (row) {
                updateRow(session, row);
                frag.appendChild(row);
            } else {
                row = buildRow(session);
                if (animateIn) {
                    row.classList.add('sessions-row--enter');
                    newRows.push(row);
                }
                frag.appendChild(row);
            }
        });

        tbody.replaceChildren(frag);

        if (newRows.length) {
            requestAnimationFrame(() => {
                newRows.forEach((row) => row.classList.add('sessions-row--enter-active'));
                setTimeout(() => {
                    newRows.forEach((row) => {
                        row.classList.remove('sessions-row--enter', 'sessions-row--enter-active');
                    });
                }, 240);
            });
        }

        updateBulkDisabled();
    }

    /**
     * Reload sessions from the API and re-render the list.
     * @param {{ animate?: boolean, removeGone?: boolean }} [options]
     * @returns {Promise<void>}
     */
    async function refresh({animate = false, removeGone = true} = {}) {
        if (refreshInFlight) return;
        refreshInFlight = true;

        const showSkeleton = !hasLoadedOnce;
        if (showSkeleton) {
            showInitialLoading();
        } else if (animate) {
            listHost.classList.add('is-refreshing');
            refreshBtn.classList.add('is-spinning');
            refreshBtn.disabled = true;
        }

        try {
            sessions = await loadSessions(opts);
            hasLoadedOnce = true;
            await renderSessions({removeGone, animateIn: hasLoadedOnce});
        } catch (e) {
            if (!hasLoadedOnce) {
                sessions = [];
                showEmpty();
            }
            showAlert(e?.message || t('sessions.loadFailed'), 'error');
            updateBulkDisabled();
        } finally {
            listHost.classList.remove('is-refreshing');
            refreshBtn.classList.remove('is-spinning');
            refreshBtn.disabled = false;
            refreshInFlight = false;
        }
    }

    showInitialLoading();

    const closePromise = RenopDialog.show({
        id: 'renop-sessions-dialog',
        className: 'sessions-dialog',
        maxWidth: '760px',
        title,
        subtitle,
        icon: 'ssl',
        body: bodyRoot,
        footer: [
            {
                text: t('sessions.close'),
                className: 'pill-btn pill-btn--soft pill-btn--sm',
                onClick: (e, d) => d.close(false),
            },
        ],
    });

    dialogRef = document.getElementById('renop-sessions-dialog');
    refresh();
    return closePromise;
}
