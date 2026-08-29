/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

import {t} from '../i18n.js';
import {showAlert} from '../alert.js';
import {putProto} from '../api.js';
import {el} from '@renop/ui/dom';
import {CreateAccessTokenRequest, CreateAccessTokenResponse} from '../proto/index.js';
import {createIcon, RenopDialog, runButtonAction} from '../components.js';
import {attachPasswordStrength, confirmWeakPasswordIfNeeded, getPasswordLengthError} from '../password-strength.js';
import {getUserProfile, invalidateUserProfiles} from '../user-profiles.js';
import {writeClipboardText} from '../clipboard.js';
import {responseErrorMessage} from '../response-errors.js';
import {
    cancelUserPermissionLoad,
    populateUserPermissions,
    selectedUserPermissions,
    updateUserPermissionCount,
} from './permissions.js';

const userEditorIds = Object.freeze({
    modal: 'user-editor-modal',
    form: 'user-editor-form',
    originalName: 'user-original-name',
    username: 'user-name',
    nickname: 'user-nickname',
    password: 'user-password',
    title: 'user-editor-title',
    subtitle: 'user-editor-subtitle',
    close: 'user-editor-close',
    cancel: 'user-editor-cancel',
    submit: 'user-editor-submit',
});

let passwordStrengthController = null;
let usersRefreshHandler = null;

/**
 * Registers the callback used to refresh the users table after a successful save.
 * @param {(() => void|Promise<void>)|null} handler - Refresh callback.
 * @returns {void}
 */
export function setUsersRefreshHandler(handler) {
    usersRefreshHandler = typeof handler === 'function' ? handler : null;
}

/**
 * Invokes the registered users-table refresh callback.
 * @returns {Promise<void>}
 */
async function refreshUsersList() {
    if (typeof usersRefreshHandler === 'function') await usersRefreshHandler();
}

/**
 * Creates one labeled input group with declarative localization attributes.
 * @param {object} options - Field options.
 * @param {string} options.id - Input element id.
 * @param {string} options.labelKey - Label translation key.
 * @param {string} options.hintKey - Hint translation key.
 * @param {string} options.type - Input type.
 * @param {string} options.value - Initial input value.
 * @param {string} options.placeholderKey - Placeholder translation key.
 * @param {string} options.autocomplete - Browser autocomplete token.
 * @param {boolean} [options.required=false] - Whether the field is required.
 * @returns {{group: HTMLElement, input: HTMLInputElement}}
 */
function createUserField({id, labelKey, hintKey, type, value, placeholderKey, autocomplete, required = false}) {
    const hintId = `${id}-hint`;
    const input = el('input', {
        id,
        type,
        value: value || '',
        autocomplete,
        required,
        class: 'user-editor-input',
        placeholder: t(placeholderKey),
        'data-i18n-placeholder': placeholderKey,
        'aria-describedby': hintId,
    });
    const label = el('label', {htmlFor: id},
        el('span', {'data-i18n': labelKey}, t(labelKey)),
        el('span', {id: hintId, class: 'user-editor-field-hint', 'data-i18n': hintKey}, t(hintKey))
    );
    return {group: el('div', {class: 'form-group user-editor-field'}, label, input), input};
}

/**
 * Creates a semantic editor section with a shared title icon.
 * @param {string} icon - Canonical icon name.
 * @param {string} titleKey - Section-title translation key.
 * @param {string} descriptionKey - Section-description translation key.
 * @param {Node[]} children - Section body nodes.
 * @param {string} modifier - Section modifier class.
 * @returns {HTMLElement}
 */
function createUserSection(icon, titleKey, descriptionKey, children, modifier) {
    return el('section', {class: `user-editor-section user-editor-section--${modifier}`},
        el('div', {class: 'user-editor-section-header'},
            createIcon(icon, {class: 'user-editor-section-icon'}),
            el('div', {class: 'user-editor-section-heading'},
                el('h4', {class: 'user-editor-section-title', 'data-i18n': titleKey}, t(titleKey)),
                el('p', {class: 'user-editor-section-desc', 'data-i18n': descriptionKey}, t(descriptionKey))
            )
        ),
        ...children
    );
}

/**
 * Validates and saves the current user editor form.
 * @param {SubmitEvent} event - Form submit event.
 * @param {{close: (result?: unknown) => void}} dialog - Owning dialog.
 * @returns {Promise<void>}
 */
async function handleUserSubmit(event, dialog) {
    event.preventDefault();
    const originalName = document.getElementById(userEditorIds.originalName)?.value.trim() || '';
    const username = document.getElementById(userEditorIds.username)?.value.trim() || '';
    const nickname = document.getElementById(userEditorIds.nickname)?.value.trim() || '';
    const password = document.getElementById(userEditorIds.password)?.value || '';

    if ((!originalName || username.toLowerCase() !== originalName.toLowerCase()) &&
        !/^[A-Za-z0-9_]{4,18}$/.test(username)) {
        showAlert(t('profile.usernameHint'), 'error');
        document.getElementById(userEditorIds.username)?.focus();
        return;
    }
    if (Array.from(nickname).length > 36) {
        showAlert(t('profile.nicknameHint'), 'error');
        document.getElementById(userEditorIds.nickname)?.focus();
        return;
    }
    if (password) {
        const lengthError = getPasswordLengthError(password);
        if (lengthError) {
            showAlert(lengthError, 'error');
            document.getElementById(userEditorIds.password)?.focus();
            return;
        }
        if (!(await confirmWeakPasswordIfNeeded(password))) return;
    }

    const targetName = originalName || username;
    const payload = {permissions: selectedUserPermissions(), nickname};
    if (!originalName) payload.is_create = true;
    if (username !== originalName) payload.new_name = username;
    if (password) payload.secret = password;

    const submitButton = document.getElementById(userEditorIds.submit);
    await runButtonAction(submitButton, async () => {
        submitButton?.setAttribute('aria-busy', 'true');
        try {
            const {response, data} = await putProto(
                `/api/tokens/${encodeURIComponent(targetName)}`,
                CreateAccessTokenRequest,
                payload,
                CreateAccessTokenResponse
            );
            if (!response.ok || !data) {
                const message = response.status === 409
                    ? t('profile.usernameExists')
                    : (response.status === 429
                        ? t('profile.renameRateLimited')
                        : (response.status === 400
                            ? t('profile.identityInvalid')
                            : await responseErrorMessage(response, 'users.failedSaveUser')));
                showAlert(message, 'error');
                return;
            }

            const generatedPassword = data.secret && data.secret !== password ? data.secret : '';
            invalidateUserProfiles(originalName, username);
            dialog.close(true);
            if (generatedPassword) showGeneratedPasswordDialog(generatedPassword);
            else showAlert(t('users.userSavedSuccess'), 'success');
            await refreshUsersList();
        } catch (error) {
            console.error('Failed to save user', error);
            showAlert(t('users.failedSaveUser'), 'error');
        } finally {
            submitButton?.removeAttribute('aria-busy');
        }
    });
}

/**
 * Opens a create-user or edit-user dialog.
 * @param {object|null} [account=null] - Existing account record, or null when creating.
 * @returns {Promise<void>}
 */
export async function openUserModal(account = null) {
    const editing = Boolean(account);
    const profileRequest = editing
        ? getUserProfile(account.name, {refresh: true}).catch(error => {
            console.error('Failed to load user profile for editing', error);
            return null;
        })
        : Promise.resolve(null);

    const username = createUserField({
        id: userEditorIds.username,
        labelKey: 'users.usernameLabel',
        hintKey: 'users.usernameHint',
        type: 'text',
        value: editing ? account.name : '',
        placeholderKey: 'users.usernamePlaceholder',
        autocomplete: 'username',
        required: true,
    });
    username.input.minLength = 4;
    username.input.maxLength = 18;

    const nickname = createUserField({
        id: userEditorIds.nickname,
        labelKey: 'profile.nicknameLabel',
        hintKey: 'profile.nicknameHint',
        type: 'text',
        value: '',
        placeholderKey: 'profile.nicknamePlaceholder',
        autocomplete: 'nickname',
    });
    nickname.input.addEventListener('input', () => {
        nickname.input.dataset.dirty = 'true';
    });

    const passwordHintKey = editing ? 'users.passwordEditHint' : 'users.passwordCreateHint';
    const password = createUserField({
        id: userEditorIds.password,
        labelKey: 'users.passwordLabel',
        hintKey: passwordHintKey,
        type: 'password',
        value: '',
        placeholderKey: 'users.passwordPlaceholder',
        autocomplete: 'new-password',
    });

    const identitySection = createUserSection(
        'identity',
        'users.accountSectionTitle',
        editing ? 'users.accountSectionEditDesc' : 'users.accountSectionCreateDesc',
        [el('div', {class: 'user-editor-fields'}, username.group, nickname.group, password.group)],
        'identity'
    );

    const permissionCount = el('span', {
        class: 'user-permission-count',
        id: 'user-permissions-count',
        'data-count': '0',
    }, t('users.rolesSelected', {count: 0}));
    const permissionsPanel = el('div', {class: 'roles-panel user-permissions-panel', id: 'user-permissions-grid'});
    const permissionsSection = createUserSection(
        'settings',
        'users.rolesLabel',
        'users.rolesSectionDesc',
        [permissionsPanel],
        'permissions'
    );
    permissionsSection.querySelector('.user-editor-section-header')?.appendChild(permissionCount);

    const body = el('div', {class: 'user-editor-body'},
        el('input', {
            type: 'hidden',
            id: userEditorIds.originalName,
            value: editing ? account.name : '',
        }),
        identitySection,
        permissionsSection
    );

    void RenopDialog.show({
        id: userEditorIds.modal,
        className: 'user-editor-card',
        maxWidth: '920px',
        icon: editing ? 'edit' : 'userPlus',
        iconClass: 'user-editor-title-icon',
        title: editing ? t('users.editUserTitle') : t('users.createUserTitle'),
        titleId: userEditorIds.title,
        titleClass: 'modal-title user-editor-title',
        subtitle: t(editing ? 'users.accountSectionEditDesc' : 'users.accountSectionCreateDesc'),
        subtitleId: userEditorIds.subtitle,
        closeBtnId: userEditorIds.close,
        backdropId: 'user-editor-backdrop',
        form: {
            id: userEditorIds.form,
            className: 'user-editor-layout',
            onSubmit: handleUserSubmit,
        },
        bodyClass: 'modal-body user-editor-scroll',
        body,
        footerClass: 'modal-footer user-editor-footer',
        footer: [
            {
                text: t('users.cancelBtn'),
                className: 'action-btn',
                id: userEditorIds.cancel,
                onClick: (event, dialog) => dialog.close(false),
            },
            {
                text: t('users.saveBtn'),
                className: 'action-btn primary-btn',
                type: 'submit',
                id: userEditorIds.submit,
            },
        ],
        onClose: () => {
            cancelUserPermissionLoad();
            passwordStrengthController = null;
        },
    });

    passwordStrengthController = attachPasswordStrength(password.input);
    const initialPermissions = editing ? (account.permissions || []) : ['base'];
    requestAnimationFrame(() => username.input.isConnected && username.input.focus());
    const [, profile] = await Promise.all([
        populateUserPermissions(initialPermissions),
        profileRequest,
    ]);
    if (profile && nickname.input.isConnected && nickname.input.dataset.dirty !== 'true') {
        nickname.input.value = profile.nickname || '';
    } else if (editing && !profile && nickname.input.isConnected) {
        showAlert(t('profile.loadFailed'), 'error');
    }
}

/**
 * Opens the editor for one existing account.
 * @param {object} account - Existing account record.
 * @returns {Promise<void>}
 */
export async function editUser(account) {
    await openUserModal(account);
}

/**
 * Shows an auto-generated password once with a copy action.
 * @param {string} generatedPassword - Plaintext generated password returned by the server.
 * @returns {void}
 */
function showGeneratedPasswordDialog(generatedPassword) {
    const password = el('code', {class: 'user-generated-password', id: 'user-generated-password'}, generatedPassword);
    const copyButton = el('button', {
        type: 'button',
        id: 'user-generated-password-copy',
        class: 'action-btn user-generated-password-copy',
        onClick: async () => {
            try {
                await writeClipboardText(password.textContent || '');
                showAlert(t('prompt.copied'), 'success');
            } catch (error) {
                console.error('Failed to copy generated password', error);
            }
        },
    }, t('prompt.clickToCopy'));
    const body = el('div', {class: 'user-generated-password-dialog'},
        el('p', {class: 'user-generated-password-label', 'data-i18n': 'users.genPasswordLabel'},
            t('users.genPasswordLabel')),
        el('div', {class: 'user-generated-password-row'}, password, copyButton),
        el('p', {class: 'user-generated-password-warning', 'data-i18n': 'users.copySecretWarning'},
            t('users.copySecretWarning'))
    );
    void RenopDialog.show({
        id: 'user-password-result-modal',
        maxWidth: '500px',
        icon: 'success',
        title: t('users.userSavedSuccess'),
        closeBtnId: 'user-password-result-close',
        backdropId: 'user-password-result-backdrop',
        body,
        footer: [{
            text: t('common.ok'),
            className: 'action-btn primary-btn',
            id: 'user-password-result-confirm',
            onClick: (event, dialog) => dialog.close(true),
        }],
    });
}

/**
 * Wires the create-user button to the user editor.
 * @returns {void}
 */
export function initUsersModal() {
    document.getElementById('btn-create-user')?.addEventListener('click', () => void openUserModal());
}

window.addEventListener('languageChanged', () => {
    const modal = document.getElementById(userEditorIds.modal);
    if (!modal || modal.style.display === 'none' || modal.style.display === '' || modal.dataset.isClosing === 'true') return;
    const editing = Boolean(document.getElementById(userEditorIds.originalName)?.value);
    const title = document.getElementById(userEditorIds.title);
    if (title) title.textContent = t(editing ? 'users.editUserTitle' : 'users.createUserTitle');
    const subtitle = document.getElementById(userEditorIds.subtitle);
    if (subtitle) subtitle.textContent = t(editing ? 'users.accountSectionEditDesc' : 'users.accountSectionCreateDesc');
    const cancel = document.getElementById(userEditorIds.cancel);
    const submit = document.getElementById(userEditorIds.submit);
    if (cancel) cancel.textContent = t('users.cancelBtn');
    if (submit) submit.textContent = t('users.saveBtn');
    const selected = selectedUserPermissions();
    void populateUserPermissions(selected).then(updateUserPermissionCount);
});
