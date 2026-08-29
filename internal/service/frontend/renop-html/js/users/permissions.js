/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

import {t} from '../i18n.js';
import {fetchProto} from '../api.js';
import {el} from '@renop/ui/dom';
import {MavenRepositoriesResponse} from '../proto/index.js';
import {
    createEmptyState,
    createRepoRow,
    createRoleChip,
    createRolesGroup,
} from '../components.js';

const userPermissionPanelId = 'user-permissions-grid';
const userPermissionCountId = 'user-permissions-count';
const basePermissions = ['admin', 'base', 'showing', 'allview', 'canview:*', 'canupdate:*'];
let permissionLoadSequence = 0;

/**
 * Resolves localized display metadata for a base permission.
 * @param {string} permission - Stable permission value.
 * @returns {{title: string, desc?: string, tone: string}}
 */
function permissionMeta(permission) {
    const metadata = {
        admin: {title: t('users.roleAdminTitle'), desc: t('users.roleAdminDesc'), tone: 'admin'},
        base: {title: t('users.roleBaseTitle'), desc: t('users.roleBaseDesc'), tone: 'system'},
        showing: {title: t('users.roleShowingTitle'), desc: t('users.roleShowingDesc'), tone: 'system'},
        allview: {title: t('users.roleAllviewTitle'), desc: t('users.roleAllviewDesc'), tone: 'view'},
        'canview:*': {title: t('users.roleCanviewAllTitle'), desc: t('users.roleCanviewAllDesc'), tone: 'view'},
        'canupdate:*': {title: t('users.roleCanupdateAllTitle'), desc: t('users.roleCanupdateAllDesc'), tone: 'update'},
    };
    return metadata[permission] || {title: permission, tone: 'system'};
}

/**
 * Returns the currently selected permission values.
 * @returns {string[]}
 */
export function selectedUserPermissions() {
    return Array.from(document.querySelectorAll(`#${userPermissionPanelId} input[type="checkbox"]:checked`),
        checkbox => checkbox.value);
}

/**
 * Updates the selected-permission counter.
 * @returns {void}
 */
export function updateUserPermissionCount() {
    const count = document.getElementById(userPermissionCountId);
    if (!count) return;
    const selected = selectedUserPermissions().length;
    count.textContent = t('users.rolesSelected', {count: selected});
    count.dataset.count = String(selected);
}

/**
 * Synchronizes rendered chips with a permission set.
 * @param {Iterable<string>} permissions - Selected stable permission values.
 * @returns {void}
 */
function applyUserPermissions(permissions) {
    const selected = new Set(permissions || []);
    document.querySelectorAll(`#${userPermissionPanelId} input[type="checkbox"]`).forEach(checkbox => {
        checkbox.checked = selected.has(checkbox.value);
        const chip = checkbox.closest('renop-role-chip, .role-chip');
        if (chip) {
            chip.classList.toggle('is-checked', checkbox.checked);
            if (checkbox.checked) chip.setAttribute('checked', '');
            else chip.removeAttribute('checked');
        }
    });
    updateUserPermissionCount();
}

/**
 * Populates the independent user-permission editor and preserves current selections.
 * @param {Iterable<string>} [permissions=[]] - Permission values to select after rendering.
 * @returns {Promise<void>}
 */
export async function populateUserPermissions(permissions = []) {
    const panel = document.getElementById(userPermissionPanelId);
    if (!panel) return;
    const sequence = ++permissionLoadSequence;
    const selected = new Set(permissions || []);
    panel.replaceChildren();
    panel.onchange = updateUserPermissionCount;

    const systemGroup = createRolesGroup(t('users.systemRoles'), t('users.systemRolesDesc'));
    const systemGrid = el('div', {class: 'roles-chip-grid'});
    for (const permission of basePermissions) {
        const metadata = permissionMeta(permission);
        systemGrid.appendChild(createRoleChip(permission, {
            title: metadata.title,
            desc: metadata.desc,
            tone: metadata.tone,
            code: permission,
            checked: selected.has(permission),
            onChange: updateUserPermissionCount,
        }));
    }
    systemGroup.appendChild(systemGrid);
    panel.appendChild(systemGroup);

    const repositoryGroup = createRolesGroup(t('users.repoAccess'), t('users.repoAccessDesc'));
    const repositoryList = el('div', {class: 'roles-repo-list'},
        el('div', {class: 'user-permissions-loading'}, t('common.loading'))
    );
    repositoryGroup.appendChild(repositoryList);
    panel.appendChild(repositoryGroup);
    updateUserPermissionCount();

    try {
        const {response, data} = await fetchProto('/api/settings/maven/repositories', MavenRepositoriesResponse);
        if (sequence !== permissionLoadSequence || !panel.isConnected) return;
        repositoryList.replaceChildren();
        if (!response.ok || !data) {
            repositoryList.appendChild(createEmptyState({message: t('users.couldNotLoadRepos')}));
        } else {
            const repositories = Object.keys(data.repositories || {}).sort((left, right) =>
                left.localeCompare(right, undefined, {numeric: true, sensitivity: 'base'}));
            if (repositories.length === 0) {
                repositoryList.appendChild(createEmptyState({message: t('users.noReposYet')}));
            } else {
                for (const repository of repositories) repositoryList.appendChild(createRepoRow(repository));
            }
        }
    } catch (error) {
        if (sequence !== permissionLoadSequence || !panel.isConnected) return;
        console.error('Failed to fetch repositories for user permissions', error);
        repositoryList.replaceChildren(createEmptyState({message: t('users.couldNotLoadRepos')}));
    }
    applyUserPermissions(selected);
}

/**
 * Invalidates an in-flight permission load after the user editor closes.
 * @returns {void}
 */
export function cancelUserPermissionLoad() {
    permissionLoadSequence += 1;
}
