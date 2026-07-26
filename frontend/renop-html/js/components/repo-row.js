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
import {createRoleChip} from './role-chip.js';

/**
 * Per-repository permission row with view/update role chips.
 */
export class RenopRepoRow extends HTMLElement {
    /**
     * @returns {string[]}
     */
    static get observedAttributes() {
        return ['repo-name'];
    }

    /**
     * Render when inserted into the DOM.
     * @returns {void}
     */
    connectedCallback() {
        this.render();
    }

    /**
     * Re-render when observed attributes change.
     * @returns {void}
     */
    attributeChangedCallback() {
        this.render();
    }

    /**
     * Rebuild repo row content from attributes.
     * @returns {void}
     */
    render() {
        const repoName = this.getAttribute('repo-name') || '';
        this.className = 'roles-repo-row';
        this.innerHTML = '';

        const info = el('div', {class: 'roles-repo-info'},
            el('span', {class: 'roles-repo-name'}, repoName),
            el('span', {class: 'roles-repo-hint'}, t('users.perRepoAccess'))
        );

        const actions = el('div', {class: 'roles-repo-actions'});

        const viewChip = createRoleChip(`canview:${repoName}`, {
            title: t('users.roleView'),
            code: `canview:${repoName}`,
            tone: 'view',
            compact: true
        });

        const updateChip = createRoleChip(`canupdate:${repoName}`, {
            title: t('users.roleDeploy'),
            code: `canupdate:${repoName}`,
            tone: 'update',
            compact: true
        });

        actions.appendChild(viewChip);
        actions.appendChild(updateChip);

        this.appendChild(info);
        this.appendChild(actions);
    }
}

if (!customElements.get('renop-repo-row')) {
    customElements.define('renop-repo-row', RenopRepoRow);
}

/**
 * Create a repository permission row for the given repo name.
 * @param {string} repoName - Repository name.
 * @returns {HTMLElement}
 */
export function createRepoRow(repoName) {
    const row = document.createElement('renop-repo-row');
    row.setAttribute('repo-name', repoName);
    return row;
}
