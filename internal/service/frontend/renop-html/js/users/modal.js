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
import {showAlert} from '../alert.js';
import {fetchProto, putProto} from '../api.js';
import {el} from '@renop/ui/dom';
import {CreateAccessTokenRequest, CreateAccessTokenResponse, MavenRepositoriesResponse} from '../proto/index.js';
import {
    createEmptyState,
    createRepoRow as buildRepoRow,
    createRoleChip as buildRoleChip,
    createRolesGroup,
    RenopDialog
} from '../components.js';
import {attachPasswordStrength, confirmWeakPasswordIfNeeded, getPasswordLengthError} from '../password-strength.js';
import {getUserProfile, invalidateUserProfiles} from '../user-profiles.js';
import {writeClipboardText} from '../clipboard.js';

let secretStrengthCtrl = null;
let currentDialogInstance = null;
let currentResultDialogInstance = null;
/** Avoid circular import with users.js — registered from there after init. */
let tokensRefreshHandler = null;

/**
 * Register the callback used to refresh the tokens list after create/edit.
 * @param {(() => void|Promise<void>)|null} handler
 */
export function setTokensRefreshHandler(handler) {
    tokensRefreshHandler = typeof handler === 'function' ? handler : null;
}

/**
 * Invoke the registered tokens refresh handler, if any.
 * @returns {void|Promise<void>}
 */
function refreshTokensList() {
    if (typeof tokensRefreshHandler === 'function') {
        return tokensRefreshHandler();
    }
    return Promise.resolve();
}

/**
 * Resolve display metadata (title, description, tone) for a base role key.
 * @param {string} role
 * @returns {{ title: string, desc?: string, tone: string }}
 */
function getRoleMeta(role) {
    const metas = {
        admin: {
            title: t('users.roleAdminTitle'),
            desc: t('users.roleAdminDesc'),
            tone: 'admin'
        },
        base: {
            title: t('users.roleBaseTitle'),
            desc: t('users.roleBaseDesc'),
            tone: 'system'
        },
        showing: {
            title: t('users.roleShowingTitle'),
            desc: t('users.roleShowingDesc'),
            tone: 'system'
        },
        allview: {
            title: t('users.roleAllviewTitle'),
            desc: t('users.roleAllviewDesc'),
            tone: 'view'
        },
        'canview:*': {
            title: t('users.roleCanviewAllTitle'),
            desc: t('users.roleCanviewAllDesc'),
            tone: 'view'
        },
        'canupdate:*': {
            title: t('users.roleCanupdateAllTitle'),
            desc: t('users.roleCanupdateAllDesc'),
            tone: 'update'
        }
    };
    return metas[role] || {title: role, tone: 'system'};
}

const BASE_ROLES = ['admin', 'base', 'showing', 'allview', 'canview:*', 'canupdate:*'];

/**
 * Attach (or return) the password strength meter for the secret input.
 * @returns {ReturnType<typeof attachPasswordStrength>|null}
 */
function ensureSecretStrengthMeter() {
    const secretInput = document.getElementById('token-secret');
    if (!secretInput) return null;
    if (!secretStrengthCtrl) {
        secretStrengthCtrl = attachPasswordStrength(secretInput);
    }
    return secretStrengthCtrl;
}

/**
 * Update the selected-roles count label from checked role checkboxes.
 */
function updateRolesSelectedCount() {
    const countEl = document.getElementById('roles-selected-count');
    if (!countEl) return;
    const n = document.querySelectorAll('#token-roles-grid input[type="checkbox"]:checked').length;
    countEl.textContent = t('users.rolesSelected', {count: n});
    countEl.dataset.count = String(n);
}

/**
 * Create a selectable role chip that refreshes the selected-count on change.
 * @param {string} value - Role value stored on the checkbox
 * @param {{ title?: string, desc?: string, tone?: string, code?: string }} [options]
 * @returns {HTMLElement}
 */
function createRoleChip(value, {title, desc, tone = 'system', code} = {}) {
    return buildRoleChip(value, {
        title,
        desc,
        tone,
        code,
        onChange: () => updateRolesSelectedCount()
    });
}


/**
 * Create a repository access row for the roles panel.
 * @param {string} repoName
 * @returns {HTMLElement}
 */
function createRepoRow(repoName) {
    return buildRepoRow(repoName);
}

/**
 * Populate the roles panel with system roles and per-repository access chips.
 * @returns {Promise<void>}
 */
export async function populateRoles() {
    const panel = document.getElementById('token-roles-grid');
    if (!panel) return;
    panel.innerHTML = '';
    if (!panel.dataset.listenerAttached) {
        panel.addEventListener('change', () => updateRolesSelectedCount());
        panel.dataset.listenerAttached = 'true';
    }

    const systemGroup = createRolesGroup(
        t('users.systemRoles'),
        t('users.systemRolesDesc')
    );
    const systemGrid = el('div', {class: 'roles-chip-grid'});

    BASE_ROLES.forEach(role => {
        const meta = getRoleMeta(role);
        systemGrid.appendChild(createRoleChip(role, {
            title: meta.title,
            desc: meta.desc,
            tone: meta.tone,
            code: role
        }));
    });
    systemGroup.appendChild(systemGrid);
    panel.appendChild(systemGroup);

    const repoGroup = createRolesGroup(
        t('users.repoAccess'),
        t('users.repoAccessDesc')
    );
    const repoList = el('div', {class: 'roles-repo-list'});

    try {
        const {response, data} = await fetchProto('/api/settings/maven/repositories', MavenRepositoriesResponse);
        if (response.ok && data) {
            const repositories = data.repositories || {};
            const keys = Object.keys(repositories);
            if (keys.length === 0) {
                repoList.appendChild(createEmptyState({message: t('users.noReposYet')}));
            } else {
                keys.forEach(repo => {
                    repoList.appendChild(createRepoRow(repo));
                });
            }
        } else {
            repoList.appendChild(createEmptyState({message: t('users.couldNotLoadRepos')}));
        }
    } catch (e) {
        console.error('Failed to fetch repositories for roles', e);
        repoList.appendChild(createEmptyState({message: t('users.couldNotLoadRepos')}));
    }

    repoGroup.appendChild(repoList);
    panel.appendChild(repoGroup);
    updateRolesSelectedCount();
}

/**
 * Validate and submit the create/edit user form, then refresh the tokens list.
 * @param {Event} e
 * @param {{ close: (result?: unknown) => void }} dialog
 * @returns {Promise<void>}
 */
async function handleUserSubmit(e, dialog) {
    e.preventDefault();
    const originalNameInput = document.getElementById('token-original-name');
    const nameInput = document.getElementById('token-name');
    const nicknameInput = document.getElementById('token-nickname');
    const secretInput = document.getElementById('token-secret');

    const originalName = originalNameInput ? originalNameInput.value.trim() : '';
    const newName = nameInput ? nameInput.value.trim() : '';
    const secret = secretInput ? secretInput.value.trim() : '';
    const nickname = nicknameInput ? nicknameInput.value.trim() : '';
	if ((!originalName || newName.toLowerCase() !== originalName.toLowerCase()) && !/^[A-Za-z0-9_]{4,18}$/.test(newName)) {
		showAlert(t('profile.usernameHint'), 'error');
		return;
	}
	if (Array.from(nickname).length > 36) {
		showAlert(t('profile.nicknameHint'), 'error');
		return;
	}

    if (secret) {
        const lengthError = getPasswordLengthError(secret);
        if (lengthError) {
            showAlert(lengthError, 'error');
            return;
        }
        if (!(await confirmWeakPasswordIfNeeded(secret))) {
            return;
        }
    }

    const checkboxes = document.querySelectorAll('#token-roles-grid input[type="checkbox"]:checked');
    const roles = Array.from(checkboxes).map(cb => cb.value);

    const targetName = originalName || newName;
    const payload = {permissions: roles, nickname};
    if (!originalName) {
        payload.is_create = true;
    }
    if (newName !== originalName) payload.new_name = newName;
    if (secret) payload.secret = secret;

    const submitButton = e.currentTarget?.querySelector('button[type="submit"]');
    if (submitButton) {
        submitButton.disabled = true;
        submitButton.setAttribute('aria-busy', 'true');
    }
    try {
        const {
            response,
            data
        } = await putProto(`/api/tokens/${targetName}`, CreateAccessTokenRequest, payload, CreateAccessTokenResponse);

        if (response.ok && data) {
            const generatedSecret = (data.secret && data.secret !== secret) ? data.secret : null;

            invalidateUserProfiles(originalName, newName);
            dialog.close(true);
            if (generatedSecret) {
                showUserResultModal(generatedSecret);
            } else {
                showAlert(t('users.userSavedSuccess'), 'success');
            }
            refreshTokensList();
        } else {
            const errText = await response.text();
			const message = response.status === 409
				? t('profile.usernameExists')
				: (response.status === 429
					? t('profile.renameRateLimited')
					: (response.status === 400 ? t('profile.identityInvalid') : errText));
            showAlert(message || t('users.failedSaveUser'), 'error');
        }
    } catch (err) {
        console.error('Failed to save user', err);
        showAlert(t('users.failedSaveUser'), 'error');
    } finally {
        if (submitButton) {
            submitButton.disabled = false;
            submitButton.removeAttribute('aria-busy');
        }
    }
}

/**
 * Open the create-user or edit-user modal, optionally pre-filled from a token.
 * @param {object|null} [token] - Existing token for edit mode; null for create
 * @returns {Promise<void>}
 */
export async function openUserModal(token = null) {
    const isEdit = !!token;
	let currentProfile = null;
	if (isEdit) {
		try {
			currentProfile = await getUserProfile(token.name, {refresh: true});
		} catch {
			showAlert(t('profile.loadFailed'), 'error');
			return;
		}
	}

    const originalNameInput = el('input', {type: 'hidden', id: 'token-original-name', value: isEdit ? token.name : ''});

    const usernameInput = el('input', {
        type: 'text',
        id: 'token-name',
        autocomplete: 'username',
        required: true,
        placeholder: t('users.usernamePlaceholder'),
        value: isEdit ? token.name : ''
    });

    const usernameLabel = el('label', {htmlFor: 'token-name'},
        el('span', {}, t('users.usernameLabel'))
    );
    const usernameHint = t('users.usernameHint');
    if (usernameHint && usernameHint !== 'users.usernameHint') {
        usernameLabel.appendChild(el('span', {class: 'token-form-field-hint'}, usernameHint));
    }
    const usernameGroup = el('div', {class: 'form-group'}, usernameLabel, usernameInput);

	const nicknameInput = el('input', {
		type: 'text', id: 'token-nickname', autocomplete: 'nickname',
		placeholder: t('profile.nicknamePlaceholder'), value: currentProfile?.nickname || ''
	});
	const nicknameLabel = el('label', {htmlFor: 'token-nickname'},
		el('span', {}, t('profile.nicknameLabel')),
		el('span', {class: 'token-form-field-hint'}, t('profile.nicknameHint'))
	);
	const nicknameGroup = el('div', {class: 'form-group'}, nicknameLabel, nicknameInput);

    const secretInput = el('input', {
        type: 'password',
        id: 'token-secret',
        autocomplete: 'new-password',
        placeholder: t('users.passwordPlaceholder'),
        value: ''
    });

    const secretHintKey = isEdit ? 'users.passwordEditHint' : 'users.passwordCreateHint';
    const secretHint = t(secretHintKey);
    const secretLabel = el('label', {htmlFor: 'token-secret'},
        el('span', {}, t('users.passwordLabel'))
    );
    if (secretHint && secretHint !== secretHintKey) {
        secretLabel.appendChild(el('span', {class: 'token-form-field-hint'}, secretHint));
    }
    const secretGroup = el('div', {class: 'form-group'}, secretLabel, secretInput);

    const accountDescKey = isEdit ? 'users.accountSectionEditDesc' : 'users.accountSectionCreateDesc';
    const accountDesc = t(accountDescKey);
    const accountTitleWrap = el('div', {class: 'token-form-section-title-wrap'},
        el('h4', {class: 'token-form-section-title'}, t('users.accountSectionTitle'))
    );
    if (accountDesc && accountDesc !== accountDescKey) {
        accountTitleWrap.appendChild(el('p', {class: 'token-form-section-desc'}, accountDesc));
    }

    const accountSection = el('section', {class: 'token-form-section token-form-section--account'},
        el('div', {class: 'token-form-section-header'}, accountTitleWrap),
        el('div', {class: 'token-form-fields'},
            usernameGroup,
			nicknameGroup,
            secretGroup
        )
    );

    const rolesDesc = t('users.rolesSectionDesc');
    const rolesTitleWrap = el('div', {class: 'token-form-section-title-wrap'},
        el('h4', {class: 'token-form-section-title'}, t('users.rolesLabel'))
    );
    if (rolesDesc && rolesDesc !== 'users.rolesSectionDesc') {
        rolesTitleWrap.appendChild(el('p', {class: 'token-form-section-desc'}, rolesDesc));
    }

    const rolesHeader = el('div', {class: 'roles-label-row token-form-section-header'},
        rolesTitleWrap,
        el('span', {class: 'roles-selected-count', id: 'roles-selected-count'}, '0 selected')
    );

    const rolesPanel = el('div', {class: 'roles-panel', id: 'token-roles-grid'});

    const rolesSection = el('section', {class: 'token-form-section token-form-section--roles'},
        rolesHeader,
        el('div', {class: 'form-group roles-form-group'}, rolesPanel)
    );

    const bodyNode = el('div', {class: 'token-form-body'},
        originalNameInput,
        accountSection,
        rolesSection
    );

    RenopDialog.show({
        id: 'create-token-modal',
        className: 'token-form-card token-form-card--roles',
        maxWidth: '680px',
        icon: 'user',
        title: isEdit ? t('users.editUserTitle') : t('users.createUserTitle'),
        titleId: 'create-token-title',
        closeBtnId: 'btn-close-create-token',
        backdropId: 'modal-backdrop',
        form: {
            id: 'create-token-form',
            className: 'token-form-layout',
            onSubmit: handleUserSubmit
        },
        bodyClass: 'modal-body token-form-scroll',
        body: bodyNode,
        footerClass: 'modal-footer token-form-footer',
        footer: [
            {text: t('users.saveBtn'), className: 'action-btn primary-btn', type: 'submit', id: 'btn-submit-token'},
            {
                text: t('users.cancelBtn'),
                className: 'action-btn',
                id: 'btn-cancel-create-token',
                onClick: (e, dialog) => dialog.close(false)
            }
        ],
        onClose: () => {
            secretStrengthCtrl = null;
            currentDialogInstance = null;
        }
    });

    currentDialogInstance = document.getElementById('create-token-modal');

    ensureSecretStrengthMeter();

    await populateRoles();

    if (isEdit && token.permissions) {
        const tokenRoles = token.permissions || [];
        const checkboxes = document.querySelectorAll('#token-roles-grid input[type="checkbox"]');
        checkboxes.forEach(cb => {
            cb.checked = tokenRoles.includes(cb.value);
            const chip = cb.closest('.role-chip');
            if (chip) chip.classList.toggle('is-checked', cb.checked);
        });
        updateRolesSelectedCount();
    }
}

/**
 * Open the edit-user modal for the given token.
 * @param {object} token
 * @returns {Promise<void>}
 */
export async function editToken(token) {
    await openUserModal(token);
}

/**
 * Close the create/edit user modal, optionally running a callback after close.
 * @param {(() => void)|undefined} [callback]
 */
export function closeTokenModal(callback) {
    const modal = document.getElementById('create-token-modal');
    if (modal && modal instanceof RenopDialog) {
        modal.close(false);
        if (typeof callback === 'function') setTimeout(callback, 210);
    } else if (typeof callback === 'function') {
        callback();
    }
}

/**
 * Show a dialog with a newly generated secret and copy controls.
 * @param {string} generatedSecret
 */
export function showUserResultModal(generatedSecret) {
    const codeEl = el('code', {
        id: 'user-result-secret',
        style: {
            flex: '1',
            background: 'var(--bg-color)',
            padding: '0.5rem 0.75rem',
            fontSize: '1.05rem',
            fontFamily: 'monospace',
            borderRadius: '6px',
            border: '1px solid var(--border-color)',
            wordBreak: 'break-all',
            userSelect: 'all'
        }
    }, generatedSecret || '');

    const copyBtn = el('button', {
        type: 'button',
        id: 'btn-copy-user-result-secret',
        class: 'action-btn',
        style: {whitespace: 'nowrap'},
        onClick: async () => {
            if (!codeEl.textContent) return;
            try {
                await writeClipboardText(codeEl.textContent);
                showAlert(t('prompt.copied'), 'success');
            } catch (err) {
                console.error('Failed to copy generated user secret', err);
            }
        }
    }, t('prompt.clickToCopy'));

    const secretContainer = el('div', {
            id: 'user-result-secret-container',
            style: {
                padding: '1rem',
                backgroundColor: 'var(--item-hover-bg)',
                borderRadius: '8px',
                border: '1px solid var(--border-color)'
            }
        },
        el('p', {style: {margin: '0 0 0.5rem 0', fontSize: '0.9rem', fontWeight: '500'}}, t('users.genPasswordLabel')),
        el('div', {style: {display: 'flex', gap: '8px', alignItems: 'center'}}, codeEl, copyBtn),
        el('p', {
            style: {
                color: '#ef4444',
                margin: '0.75rem 0 0 0',
                fontSize: '0.85rem',
                fontWeight: '500'
            }
        }, t('users.copySecretWarning'))
    );

    RenopDialog.show({
        id: 'user-result-modal',
        maxWidth: '480px',
        icon: 'success',
        title: t('users.userSavedSuccess'),
        titleId: 'user-result-title',
        closeBtnId: 'btn-close-user-result',
        backdropId: 'user-result-backdrop',
        body: secretContainer,
        footer: [
            {
                text: t('common.ok'),
                className: 'action-btn primary-btn',
                id: 'btn-confirm-user-result',
                onClick: (e, d) => d.close(true)
            }
        ],
        onClose: () => {
            currentResultDialogInstance = null;
        }
    });

    currentResultDialogInstance = document.getElementById('user-result-modal');
}

/**
 * Close the post-save secret result modal if it is open.
 */
export function closeUserResultModal() {
    const modal = document.getElementById('user-result-modal');
    if (modal && modal instanceof RenopDialog) {
        modal.close(false);
    }
}

/**
 * Wire the create-user button to open an empty user modal.
 */
export function initUsersModal() {
    const createBtn = document.getElementById('btn-show-create-token');
    if (createBtn) {
        createBtn.addEventListener('click', () => openUserModal(null));
    }
}

window.addEventListener('languageChanged', () => {
    const modal = document.getElementById('create-token-modal');
    if (modal && modal.style.display !== 'none' && modal.style.display !== '') {
        const title = document.getElementById('create-token-title');
        const originalNameInput = document.getElementById('token-original-name');
        if (title) {
            title.textContent = (originalNameInput && originalNameInput.value)
                ? t('users.editUserTitle')
                : t('users.createUserTitle');
        }
        populateRoles();
    }
});
