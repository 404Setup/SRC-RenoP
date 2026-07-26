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
import {el} from '../cfg-ui.js';
import {createBadge} from './badge.js';
import {createUserAvatar} from './user-avatar.js';
import {createButton} from './button.js';
import {createCodeBadge} from './code-badge.js';

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
    const cell3 = el('td', {}, el('div', {
        class: 'skeleton-bone',
        style: {width: '100px', height: '20px', borderRadius: '4px'}
    }));
    const cell4 = el('td', {}, el('div', {class: 'skeleton-bone', style: {width: '80px', height: '14px'}}));
    const cell5 = el('td', {style: {textAlign: 'right'}}, el('div', {
        class: 'skeleton-bone',
        style: {width: '130px', height: '28px', borderRadius: '6px', marginLeft: 'auto'}
    }));

    tr.append(cell1, cell2, cell3, cell4, cell5);
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
 * @property {(token: object) => void} [onReset] - Reset-token action handler
 * @property {(token: object) => void} [onSessions] - Sessions action handler
 */

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
        createUserAvatar(token.name),
        el('div', {class: 'user-info-col'},
            el('span', {class: 'user-name'}, token.name)
        )
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

    const tokensTd = el('td');
    if (token.tokens && token.tokens.length > 0) {
        tokensTd.appendChild(createCodeBadge(token.tokens[0]));
    } else {
        tokensTd.appendChild(el('span', {style: {opacity: '0.5', fontSize: '0.85rem'}}, t('common.none')));
    }

    const dateTd = el('td', {
        style: {
            fontSize: '0.85rem',
            color: 'var(--text-color)',
            opacity: '0.8'
        }
    }, new Date(token.created_at).toLocaleDateString());

    const sessionsBtn = createButton('', {
        class: 'table-action-btn sessions-btn',
        icon: 'ssl',
        title: t('users.sessions'),
        onClick: () => options.onSessions && options.onSessions(token)
    });
    sessionsBtn.appendChild(el('span', {class: 'btn-text'}, t('users.sessions')));

    const editBtn = createButton('', {
        class: 'table-action-btn edit-btn',
        icon: 'edit',
        title: t('common.edit'),
        onClick: () => options.onEdit && options.onEdit(token)
    });
    editBtn.appendChild(el('span', {class: 'btn-text'}, t('common.edit')));

    const deleteBtn = createButton('', {
        class: 'table-action-btn delete-btn',
        icon: 'delete',
        title: t('common.delete'),
        onClick: () => options.onDelete && options.onDelete(token)
    });
    deleteBtn.appendChild(el('span', {class: 'btn-text'}, t('common.delete')));

    const resetBtn = createButton('', {
        class: 'table-action-btn reset-btn',
        icon: 'refresh',
        iconProps: {width: '13', height: '13'},
        title: t('users.reset'),
        onClick: () => options.onReset && options.onReset(token)
    });
    resetBtn.appendChild(el('span', {class: 'btn-text'}, t('users.reset')));

    const actionsWrap = el('div', {class: 'users-actions'}, sessionsBtn, resetBtn, editBtn, deleteBtn);
    const actionsTd = el('td', {class: 'users-actions-cell'}, actionsWrap);

    row.append(nameTd, permsTd, tokensTd, dateTd, actionsTd);
    return row;
}
