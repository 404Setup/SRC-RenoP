/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {t} from '../i18n.js';
import {el} from '@renop/ui/dom';
import {createBadge} from './badge.js';
import {createUserIdentity} from './user-identity.js';
import {createButton} from './button.js';
import {createIcon} from './icon.js';
import {RenopDialog} from './dialog.js';
import {formatTimestamp} from '../time.js';

/**
 * Create a skeleton placeholder table row for the users list.
 * @returns {HTMLElement}
 */
export function createUsersSkeletonRow() {
    const tr = el('tr', {class: 'skeleton-row-tr'});
    const cell1 = el('td', {class: 'user-cell', style: {display: 'flex', alignItems: 'center', gap: '12px'}},
        el('div', {
            class: 'skeleton-bone',
            style: {width: '36px', height: '36px', borderRadius: '50%', flexShrink: '0'}
        }),
        el('div', {class: 'skeleton-bone', style: {width: '110px', height: '16px'}})
    );
    const cell2 = el('td', {}, el('div', {
        class: 'skeleton-bone',
        style: {width: '90px', height: '22px', borderRadius: '12px'}
    }));
    const cell3 = el('td', {}, el('div', {class: 'skeleton-bone', style: {width: '80px', height: '14px'}}));
    const cell4 = el('td', {style: {textAlign: 'right'}}, el('div', {
        class: 'skeleton-bone',
        style: {width: '130px', height: '28px', borderRadius: '6px', marginLeft: 'auto'}
    }));

    tr.append(cell1, cell2, cell3, cell4);
    return tr;
}

/**
 * Custom element wrapper that renders a users-table row from a token object.
 * @extends HTMLElement
 */
export class RenopUserRow extends HTMLElement {
    /**
     * Attributes that trigger re-render when changed.
     * @returns {string[]}
     */
    static get observedAttributes() {
        return ['user-name', 'created-at'];
    }

    /**
     * Lifecycle hook: render the row when inserted into the DOM.
     */
    connectedCallback() {
        this.render();
    }

    /**
     * Assign token data and action options, then re-render the row.
     * @param {object} token
     * @param {UserRowOptions} [options]
     */
    setToken(token, options = {}) {
        this._token = token;
        this._options = options;
        this.render();
    }

    /**
     * Rebuild the row DOM from the current token and options.
     */
    render() {
        if (!this._token) return;
        const token = this._token;
        const options = this._options || {};
        this.dataset.userName = token.name;
        this.className = 'user-row';
        this.innerHTML = '';

        const tr = createUserRow(token, options);
        this.appendChild(tr);
    }
}

if (!customElements.get('renop-user-row')) {
    customElements.define('renop-user-row', RenopUserRow);
}

/**
 * @typedef {object} UserRowOptions
 * @property {(perm: string) => string} [formatPermissionTag] - Format a permission for display
 * @property {(token: object) => void} [onEdit] - Edit action handler
 * @property {(token: object) => void} [onDelete] - Delete action handler
 * @property {(token: object) => void} [onSessions] - Sessions action handler
 */

/**
 * Open a Dialog showing card operations for a specific user.
 * @param {object} token - User/Token object
 * @param {UserRowOptions} options - Callback options
 */
export function openUserActionsDialog(token, options = {}) {
    const username = token.name || 'User';

    const cardsGrid = el('div', {
        class: 'user-action-cards-grid',
        style: {
            display: 'grid',
            gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))',
            gap: '12px',
            padding: '4px 0'
        }
    });

    const actions = [
        {
            id: 'audit',
            icon: 'fileText',
            iconColor: '#3b82f6',
            iconBg: 'rgba(59, 130, 246, 0.1)',
            title: t('users.auditLogs') || 'Activity Logs',
            desc: t('users.auditLogsDesc') || 'View activity log history for this user',
            handler: options.onAuditLogs
        },
        {
            id: 'fido',
            icon: 'fileKey',
            iconColor: '#10b981',
            iconBg: 'rgba(16, 185, 129, 0.1)',
            title: t('users.fidoDevices') || 'Passkeys',
            desc: t('users.fidoDevicesDesc') || 'Manage Passkeys',
            handler: options.onFido
        },
        {
            id: 'sessions',
            icon: 'ssl',
            iconColor: '#8b5cf6',
            iconBg: 'rgba(139, 92, 246, 0.1)',
            title: t('users.sessions') || 'Sessions',
            desc: t('users.sessionsDesc') || 'View and revoke active login sessions',
            handler: options.onSessions
        },
        {
            id: 'edit',
            icon: 'edit',
            iconColor: '#6366f1',
            iconBg: 'rgba(99, 102, 241, 0.1)',
            title: t('common.edit') || 'Edit',
            desc: t('users.editDesc') || 'Edit user profile and permissions',
            handler: options.onEdit
        },
        {
            id: 'delete',
            icon: 'delete',
            iconColor: '#ef4444',
            iconBg: 'rgba(239, 68, 68, 0.1)',
            title: t('common.delete') || 'Delete',
            desc: t('users.deleteDesc') || 'Delete user account and token',
            handler: options.onDelete
        }
    ];

    let dialogRef = null;

    actions.forEach(act => {
        const card = el('div', {
            class: 'user-action-card',
            style: {
                display: 'flex',
                alignItems: 'center',
                gap: '12px',
                padding: '14px 16px',
                borderRadius: '10px',
                border: '1px solid var(--border-color, #e5e7eb)',
                background: 'var(--card-bg, #ffffff)',
                cursor: 'pointer',
                transition: 'all 0.18s ease'
            },
            onClick: () => {
                const dialogEl = document.getElementById('user-actions-modal');
                if (dialogEl && typeof dialogEl.close === 'function') {
                    dialogEl.close(true);
                }
                if (act.handler) {
                    act.handler(token);
                }
            }
        });

        card.addEventListener('mouseenter', () => {
            card.style.borderColor = act.iconColor;
            card.style.transform = 'translateY(-2px)';
            card.style.boxShadow = `0 4px 12px ${act.iconBg}`;
        });
        card.addEventListener('mouseleave', () => {
            card.style.borderColor = 'var(--border-color, #e5e7eb)';
            card.style.transform = 'none';
            card.style.boxShadow = 'none';
        });

        const iconContainer = el('div', {
            style: {
                width: '38px',
                height: '38px',
                borderRadius: '8px',
                background: act.iconBg,
                color: act.iconColor,
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                flexShrink: '0'
            }
        }, createIcon(act.icon, {width: '18', height: '18'}));

        const textCol = el('div', {style: {flex: '1', minWidth: '0'}},
            el('div', {
                style: {
                    fontWeight: '600',
                    fontSize: '0.9rem',
                    marginBottom: '2px',
                    color: 'var(--text-color, #111827)'
                }
            }, act.title),
            el('div', {
                style: {
                    fontSize: '0.78rem',
                    opacity: '0.65',
                    whiteSpace: 'nowrap',
                    overflow: 'hidden',
                    textOverflow: 'ellipsis'
                }
            }, act.desc)
        );

        const chevron = el('div', {style: {opacity: '0.4'}}, createIcon('chevron', {width: '14', height: '14'}));

        card.append(iconContainer, textCol, chevron);
        cardsGrid.appendChild(card);
    });

    const titleText = t('users.userActionsTitle', {user: username}) || `User Actions ("${username}")`;

    dialogRef = RenopDialog.show({
        id: 'user-actions-modal',
        maxWidth: '560px',
        icon: 'settings',
        title: titleText,
        body: cardsGrid,
        footer: [
            {
                text: t('common.close') || 'Close',
                className: 'pill-btn pill-btn--soft pill-btn--sm',
                onClick: (e, d) => d.close(true)
            }
        ]
    });
}

/**
 * Build a users-table row for one access token / user.
 * @param {object} token - Token with name, permissions, tokens, created_at
 * @param {UserRowOptions} [options]
 * @returns {HTMLTableRowElement}
 */
export function createUserRow(token, options = {}) {
    const row = document.createElement('tr');
    row.dataset.userName = token.name;

    const nameTd = el('td', {class: 'user-cell'},
        createUserIdentity(token.name, {avatar: true})
    );

    const permsTd = el('td');
    const perms = token.permissions || [];
    if (perms.length > 0) {
        const seenTags = new Set();
        perms.forEach(perm => {
            const formatted = options.formatPermissionTag ? options.formatPermissionTag(perm) : perm;
            if (seenTags.has(formatted)) return;
            seenTags.add(formatted);

            let type = 'none';
            if (perm.toLowerCase().includes('admin') || perm.toLowerCase().includes('manager')) {
                type = 'admin';
            } else if (perm.toLowerCase().includes('view')) {
                type = 'view';
            } else if (perm.toLowerCase().includes('update')) {
                type = 'update';
            }
            permsTd.appendChild(createBadge(formatted, type, {title: perm}));
        });
    } else {
        permsTd.appendChild(createBadge(t('common.none'), 'none'));
    }

    const dateTd = el('td', {
        style: {
            fontSize: '0.85rem',
            color: 'var(--text-color)',
            opacity: '0.8'
        }
    }, formatTimestamp(token.created_at, {dateOnly: true, fallback: t('common.unknown')}));

    const actionsBtn = createButton('', {
        class: 'table-action-btn action-btn',
        icon: 'settings',
        title: t('users.thActions') || 'Actions',
        onClick: () => openUserActionsDialog(token, options)
    });
    actionsBtn.appendChild(el('span', {class: 'btn-text'}, t('users.thActions') || 'Actions'));

    const actionsWrap = el('div', {class: 'users-actions'}, actionsBtn);
    const actionsTd = el('td', {class: 'users-actions-cell'}, actionsWrap);

    row.append(nameTd, permsTd, dateTd, actionsTd);
    return row;
}
