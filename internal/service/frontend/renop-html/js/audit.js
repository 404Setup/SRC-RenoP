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
import {apiRequest, fetchProto} from './api.js';
import {showAlert} from './alert.js';
import {el} from '@renop/ui/dom';
import {createUserIdentity, RenopDialog} from './components.js';
import {enableDragToScroll} from '@renop/ui/scroll';
import {AuditLogList} from './proto/index.js';
import {formatTimestamp} from './time.js';
import {responseErrorMessage} from './response-errors.js';

let currentAuditModal = null;

/** Render a filterable account identity while preserving masked operators. */
function renderLogIdentity(username) {
    if (username === 'Administrator') return t('audit.administrator');
    if (username === 'system') return t('audit.system');
    if (!username) return '-';
    return el('span', {style: {display: 'inline-grid', gap: '2px'}}, createUserIdentity(username),
        el('small', {style: {opacity: '.65'}}, '@' + username));
}

/**
 * Smoothly animate element height changes when content is updated.
 * @param {HTMLElement} element
 * @param {Function} updateFn
 */
function animateModalHeight(element, updateFn) {
    if (!element || !element.isConnected) {
        updateFn();
        return;
    }
    const startHeight = element.getBoundingClientRect().height;

    updateFn();

    element.style.height = 'auto';
    const targetHeight = element.getBoundingClientRect().height;

    if (Math.abs(startHeight - targetHeight) < 2) {
        element.style.height = '';
        return;
    }

    element.style.height = `${startHeight}px`;
    element.style.overflow = 'hidden';
    element.style.transition = 'height 0.28s cubic-bezier(0.16, 1, 0.3, 1)';
    element.offsetHeight;

    requestAnimationFrame(() => {
        element.style.height = `${targetHeight}px`;
    });

    const cleanup = () => {
        element.style.height = '';
        element.style.overflow = '';
        element.style.transition = '';
        element.removeEventListener('transitionend', onEnd);
    };

    const onEnd = (e) => {
        if (e.target === element && e.propertyName === 'height') {
            cleanup();
        }
    };

    element.addEventListener('transitionend', onEnd);
    setTimeout(cleanup, 320);
}

/**
 * Open Audit Logs modal.
 * @param {{ mode: 'self'|'user'|'global', username?: string }} options
 */
export async function openAuditLogsDialog(options = {}) {
    const isSelf = options.mode === 'self';
    const isGlobal = options.mode === 'global';
    const targetUsername = options.username || localStorage.getItem('username') || '';

    let page = 1;
    const pageSize = 15;
    let isFetching = false;
    let hasLoadedOnce = false;

    const titleText = isGlobal ? t('audit.globalTitle') : isSelf
        ? (t('profile.auditLogsTitle') || 'Activity Logs')
        : (t('users.auditLogsTitle', {user: targetUsername}) || `Activity Logs for "${targetUsername}"`);

    const container = el('div', {
        class: 'audit-logs-container',
        style: {minHeight: 'auto', display: 'flex', flexDirection: 'column', gap: '12px'}
    });

    const contentArea = el('div', {
        class: 'audit-logs-list',
        style: {flex: '1', overflowX: 'auto', overflowY: 'auto', maxHeight: '420px'}
    });
    enableDragToScroll(contentArea);

    const paginationArea = el('div', {
        class: 'audit-logs-pagination',
        style: {
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            paddingTop: '8px',
            borderTop: '1px solid var(--border-color, #e5e7eb)',
            fontSize: '0.85rem'
        }
    });

    const fields = {};
    const fieldset = el('fieldset', {style: {border: '0', padding: '0', margin: '0', minWidth: '0', display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(min(100%, 150px), 1fr))', gap: '10px'}});
    const addFilter = (name, label, type = 'text', choices = null) => {
        const input = choices
            ? el('select', {class: 'cfg-input', name}, ...choices.map(([value, text]) => el('option', {value}, text)))
            : el('input', {class: 'cfg-input', name, type, maxLength: name === 'operator' || name === 'username' || name === 'initiator' ? 255 : 64});
        fields[name] = input;
        input.style.minWidth = '0';
        fieldset.append(el('label', {class: type === 'datetime-local' ? 'audit-filter-date' : '', style: {display: 'grid', gap: '4px', minWidth: '0', fontSize: '.85rem'}}, el('span', {}, t(label)), input));
    };
    const all = ['', t('audit.all')];
    const triggerLabels = {web: t('audit.trigger.web'), api: t('audit.trigger.api'), http: t('audit.trigger.http'), system: t('audit.trigger.system'), unknown: t('audit.trigger.unknown')};
    const severityLabels = {info: t('audit.severity.info'), warning: t('audit.severity.warning'), error: t('audit.severity.error')};
    if (isGlobal) addFilter('kind', 'audit.kind', 'text', [all, ['audit', t('profile.auditLogsTitle')], ['system', t('audit.system')]]);
    addFilter('action', 'audit.action');
    fields.action.placeholder = 'LOGIN';
    addFilter('operator', 'audit.operator', 'text', isSelf ? [all, [targetUsername, targetUsername], ['@administrator', t('audit.administrator')]] : null);
    addFilter('initiator', 'audit.initiator', 'text', isSelf ? [all, [targetUsername, targetUsername], ['@administrator', t('audit.administrator')]] : null);
    if (isGlobal) addFilter('username', 'audit.account');
    addFilter('trigger', 'audit.trigger', 'text', [all, ...Object.entries(triggerLabels)]);
    if (isGlobal) addFilter('severity', 'audit.severity', 'text', [all, ...Object.entries(severityLabels)]);
    addFilter('from', 'audit.from', 'datetime-local');
    addFilter('until', 'audit.until', 'datetime-local');
    let filters = new URLSearchParams();
    const filterForm = el('form', {class: 'audit-filters'}, fieldset);
    const apply = el('button', {type: 'submit', class: 'pill-btn pill-btn--primary'}, t('audit.apply'));
    const reset = el('button', {type: 'button', class: 'pill-btn', onClick: () => {
        if (isFetching) return;
        filterForm.reset();
        fields.until.setCustomValidity('');
        filters = new URLSearchParams();
        page = 1;
        void loadLogs();
    }}, t('audit.reset'));
    fieldset.append(el('div', {style: {display: 'flex', gap: '6px', alignItems: 'end'}}, apply, reset));
    filterForm.addEventListener('submit', event => {
        event.preventDefault();
        if (isFetching) return;
        const next = new URLSearchParams();
        for (const [name, input] of Object.entries(fields)) {
            if (input.value) next.set(name, name === 'from' || name === 'until' ? String(new Date(input.value).getTime()) : input.value.trim());
        }
        if (next.has('from') && next.has('until') && Number(next.get('from')) > Number(next.get('until'))) {
            fields.until.setCustomValidity(t('audit.invalidFilter'));
            fields.until.reportValidity();
            return;
        }
        filters = next;
        page = 1;
        void loadLogs();
    });
    fields.from.addEventListener('input', () => fields.until.setCustomValidity(''));
    fields.until.addEventListener('input', () => fields.until.setCustomValidity(''));
    container.appendChild(filterForm);
    container.appendChild(contentArea);
    container.appendChild(paginationArea);

    const loadLogs = async (direction = null) => {
        if (isFetching) return;
        isFetching = true;
        fieldset.disabled = true;

        const modalContent = container.closest('.modal-content');

        if (!hasLoadedOnce) {
            contentArea.replaceChildren(
                el('div', {class: 'sessions-loading'},
                    el('div', {class: 'sessions-loading-spinner', 'aria-hidden': 'true'}),
                    el('span', {}, t('audit.loading'))
                )
            );
            paginationArea.innerHTML = '';
        } else {
            contentArea.classList.add('is-busy');
            const btns = paginationArea.querySelectorAll('button');
            btns.forEach(b => b.disabled = true);
        }

        try {
            const query = new URLSearchParams(filters);
            query.set('page', String(page));
            query.set('page_size', String(pageSize));
            const base = isGlobal ? '/api/auth/logs' : isSelf
                ? '/api/auth/profile/audit-logs'
                : `/api/auth/users/${encodeURIComponent(targetUsername)}/audit-logs`;
            const endpoint = `${base}?${query}`;

            const {response: res, data} = await fetchProto(endpoint, AuditLogList);
            if (!res.ok || !data) {
                showAlert(await responseErrorMessage(res, 'common.error'), 'error');
                contentArea.classList.remove('is-busy');
                if (!hasLoadedOnce) {
                    contentArea.innerHTML = `<div style="padding: 2rem; text-align: center; color: #ef4444; font-size: 0.9rem;">${t('common.error')}</div>`;
                } else {
                    showAlert(t('common.error'), 'error');
                }
                return;
            }

            const logs = data.logs || [];
            const total = data.total || 0;

            const renderNewContent = () => {
                contentArea.classList.remove('is-busy');
                contentArea.innerHTML = '';

                container.style.minHeight = 'auto';
                if (logs.length === 0) {
                    contentArea.innerHTML = `<div style="padding: 2.5rem; text-align: center; opacity: 0.6; font-size: 0.9rem;">${t('common.none')}</div>`;
                } else {
                    const table = el('table', {
                            class: 'audit-table',
                            style: {width: '100%', minWidth: '650px', borderCollapse: 'collapse', fontSize: '0.85rem'}
                        },
                        el('thead', {},
                            el('tr', {
                                    style: {
                                        borderBottom: '1px solid var(--border-color, #e5e7eb)',
                                        textAlign: 'left',
                                        opacity: '0.7'
                                    }
                                },
                                el('th', {
                                    style: {
                                        padding: '8px 12px',
                                        whiteSpace: 'nowrap'
                                    }
                                }, t('audit.time') || 'Time'),
                                el('th', {
                                    style: {
                                        padding: '8px 12px',
                                        whiteSpace: 'nowrap'
                                    }
                                }, t('audit.action') || 'Action'),
                                el('th', {
                                    style: {
                                        padding: '8px 12px',
                                        whiteSpace: 'nowrap'
                                    }
                                }, t('audit.operator') || 'Operator'),
                                el('th', {style: {padding: '8px 12px', whiteSpace: 'nowrap'}}, t('audit.initiator')),
                                el('th', {style: {padding: '8px 12px', whiteSpace: 'nowrap'}}, t('audit.trigger')),
                                ...(isGlobal ? [el('th', {style: {padding: '8px 12px'}}, t('audit.account'))] : []),
                                el('th', {style: {padding: '8px 12px'}}, t('audit.details') || 'Details'),
                                el('th', {
                                    style: {
                                        padding: '8px 12px',
                                        whiteSpace: 'nowrap'
                                    }
                                }, t('audit.authMethod') || 'Auth Method'),
                                el('th', {style: {padding: '8px 12px', whiteSpace: 'nowrap'}}, t('audit.ip') || 'IP')
                            )
                        )
                    );

                    const tbody = el('tbody');
                    logs.forEach(log => {
                        const tr = el('tr', {
                            style: {
                                borderBottom: '1px solid var(--border-color, #e5e7eb)',
                                opacity: '0.9'
                            }
                        });

                        const timeTd = el('td', {style: {padding: '8px 12px', whiteSpace: 'nowrap', opacity: '0.75'}},
                            formatTimestamp(log.created_at, {fallback: t('common.unknown')})
                        );

                        const actionBadge = renderActionBadge(log.action);
                        if (isGlobal && log.kind === 'system') actionBadge.title = severityLabels[log.severity] || log.severity;
                        const actionTd = el('td', {style: {padding: '8px 12px', whiteSpace: 'nowrap'}}, actionBadge);
                        if (isGlobal) actionTd.append(el('div', {style: {fontSize: '.72rem', opacity: '.75', marginTop: '4px'}},
                            `${log.kind === 'system' ? t('audit.system') : t('profile.auditLogsTitle')} · ${severityLabels[log.severity] || log.severity}`));

                        const displayOp = renderLogIdentity(log.operator);
                        const opTd = el('td', {
                            style: {
                                padding: '8px 12px',
                                fontWeight: '500',
                                whiteSpace: 'nowrap'
                            }
                        }, displayOp);

                        const detailsTd = el('td', {
                            style: {
                                padding: '8px 12px',
                                maxWidth: '280px',
                                minWidth: '160px',
                                wordBreak: 'break-word'
                            }
                        }, log.details || '-');

                        const authTd = el('td', {
                            style: {
                                padding: '8px 12px',
                                whiteSpace: 'nowrap',
                                opacity: '0.8'
                            }
                        }, log.auth_method || '-');

                        const ipTd = el('td', {
                            style: {
                                padding: '8px 12px',
                                whiteSpace: 'nowrap',
                                fontFamily: 'monospace',
                                opacity: '0.75'
                            }
                        }, log.ip || '-');

                        const triggerLabel = triggerLabels[log.trigger] || log.trigger || '-';
                        const triggerTd = el('td', {style: {padding: '8px 12px', whiteSpace: 'nowrap'}}, triggerLabel);
                        const initiator = renderLogIdentity(log.initiator);
                        tr.append(timeTd, actionTd, opTd, el('td', {style: {padding: '8px 12px'}}, initiator), triggerTd);
                        if (isGlobal) tr.append(el('td', {style: {padding: '8px 12px'}}, renderLogIdentity(log.username)));
                        tr.append(detailsTd, authTd, ipTd);
                        tbody.appendChild(tr);
                    });

                    table.appendChild(tbody);

                    if (direction === 'next') {
                        tbody.classList.add('is-page-enter-next');
                    } else if (direction === 'prev') {
                        tbody.classList.add('is-page-enter-prev');
                    } else if (hasLoadedOnce) {
                        tbody.classList.add('is-content-entering');
                    }

                    contentArea.appendChild(table);
                }

                paginationArea.innerHTML = '';
                const totalPages = Math.ceil(total / pageSize) || 1;
                const recordsLabel = total === 1 ? (t('common.record') || 'record') : (t('common.records') || 'records');
                const pageInfo = el('span', {style: {opacity: '0.7'}}, `${t('common.page') || 'Page'} ${page} / ${totalPages} (${total} ${recordsLabel})`);

                const prevBtn = el('button', {
                    type: 'button',
                    class: 'pill-btn',
                    style: {padding: '4px 10px', fontSize: '0.8rem', marginRight: '6px'},
                    disabled: page <= 1,
                    onClick: () => {
                        if (page > 1 && !isFetching) {
                            page--;
                            loadLogs('prev');
                        }
                    }
                }, t('common.prev') || 'Prev');

                const nextBtn = el('button', {
                    type: 'button',
                    class: 'pill-btn',
                    style: {padding: '4px 10px', fontSize: '0.8rem'},
                    disabled: page >= totalPages,
                    onClick: () => {
                        if (page < totalPages && !isFetching) {
                            page++;
                            loadLogs('next');
                        }
                    }
                }, t('common.next') || 'Next');

                paginationArea.append(pageInfo, el('div', {}, prevBtn, nextBtn));
            };

            if (modalContent) {
                animateModalHeight(modalContent, renderNewContent);
            } else {
                renderNewContent();
            }

            hasLoadedOnce = true;

        } catch (err) {
            console.error('Failed to load audit logs:', err);
            contentArea.classList.remove('is-busy');
            if (!hasLoadedOnce) {
                contentArea.innerHTML = `<div style="padding: 2rem; text-align: center; color: #ef4444; font-size: 0.9rem;">${t('common.error')}</div>`;
            } else {
                showAlert(t('common.error'), 'error');
            }
        } finally {
            isFetching = false;
            fieldset.disabled = false;
        }
    };


    const footerButtons = [];

    if (!isSelf && !isGlobal && targetUsername) {
        footerButtons.push({
            text: t('users.clearAuditLogs') || 'Clear User Logs',
            className: 'pill-btn pill-btn--danger pill-btn--sm',
            onClick: async (e, dialog) => {
                const confirmMsg = t('users.confirmClearAuditLogs', {user: targetUsername});
                if (await window.showConfirm(confirmMsg)) {
                    try {
                        const delRes = await apiRequest(`/api/auth/users/${encodeURIComponent(targetUsername)}/audit-logs`, {method: 'DELETE'});
                        if (delRes.ok) {
                            showAlert(t('users.auditLogsCleared'), 'success');
                            loadLogs();
                        } else {
                            showAlert(t('common.error'), 'error');
                        }
                    } catch (err) {
                        showAlert(t('common.error'), 'error');
                    }
                }
            }
        });
    }

    footerButtons.push({
        text: t('common.close') || 'Close',
        className: 'pill-btn pill-btn--soft pill-btn--sm',
        onClick: (e, dialog) => dialog.close(true)
    });

    currentAuditModal = RenopDialog.show({
        id: 'audit-logs-modal',
        maxWidth: '850px',
        icon: 'fileText',
        title: titleText,
        body: container,
        footer: footerButtons
    });

    loadLogs();
}

/**
 * Build a localized, color-coded audit action badge.
 * @param {string} action - Stable server-owned audit action identifier.
 * @returns {HTMLSpanElement} Audit action badge.
 */
function renderActionBadge(action) {
    let color = '#6b7280';
    let bg = 'rgba(107, 114, 128, 0.1)';

    switch (action) {
        case 'LOGIN':
            color = '#10b981';
            bg = 'rgba(16, 185, 129, 0.1)';
            break;
        case 'LOGOUT':
            color = '#f59e0b';
            bg = 'rgba(245, 158, 11, 0.1)';
            break;
        case 'UPLOAD':
            color = '#3b82f6';
            bg = 'rgba(59, 130, 246, 0.1)';
            break;
        case 'DELETE':
            color = '#ef4444';
            bg = 'rgba(239, 68, 68, 0.1)';
            break;
        case 'PASSWORD_UPDATE':
            color = '#8b5cf6';
            bg = 'rgba(139, 92, 246, 0.1)';
            break;
        case 'FIDO_UPDATE':
            color = '#10b981';
            bg = 'rgba(16, 185, 129, 0.1)';
            break;
        case 'SETTINGS_UPDATE':
            color = '#6366f1';
            bg = 'rgba(99, 102, 241, 0.1)';
            break;
        case 'SESSION_REVOKE':
            color = '#ec4899';
            bg = 'rgba(236, 72, 153, 0.1)';
            break;
        case 'TOKEN_GENERATE':
            color = '#06b6d4';
            bg = 'rgba(6, 182, 212, 0.1)';
            break;
        case 'USER_PERMISSION_UPDATE':
            color = '#f97316';
            bg = 'rgba(249, 115, 22, 0.1)';
            break;
        case 'LOG_CLEAR':
            color = '#ef4444';
            bg = 'rgba(239, 68, 68, 0.1)';
            break;
    }
    if (String(action).startsWith('CARGO_')) {
        color = '#b45309';
        bg = 'rgba(245, 158, 11, 0.12)';
    }

    const badge = document.createElement('span');
    badge.className = 'audit-action-badge';
    badge.style.cssText = `display: inline-block; white-space: nowrap; flex-shrink: 0; padding: 2px 8px; border-radius: 6px; font-size: 0.75rem; font-weight: 600; color: ${color}; background: ${bg};`;
    const translationKey = 'audit.action.' + action;
    const translation = t(translationKey);
    badge.textContent = translation === translationKey ? action : translation;
    return badge;
}
