/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {showAlert} from './alert.js';
import {closeModalWithAnim} from './app-ui.js';
import {RenopDialog} from './components.js';
import {t} from './i18n.js';
import {attachPasswordStrength, confirmWeakPasswordIfNeeded, getPasswordLengthError,} from './password-strength.js';
import {el} from '@renop/ui/dom';

/**
 * Open the four-code password recovery workflow.
 * @returns {void}
 */
function openPasswordRecoveryDialog() {
    const identifier = el('input', {
        class: 'profile-input', type: 'text', maxlength: '254', autocomplete: 'username',
        placeholder: t('login.recoveryIdentifierPlaceholder')
    });
    const codeInputs = Array.from({length: 4}, (_, index) => el('input', {
        class: 'profile-input recovery-code-input', type: 'text', maxlength: '64',
        autocomplete: 'one-time-code', spellcheck: 'false',
        placeholder: t('login.recoveryCodePlaceholder', {index: index + 1})
    }));
    const password = el('input', {
        class: 'profile-input', type: 'password', maxlength: '72', autocomplete: 'new-password',
        placeholder: t('profile.newPasswordPlaceholder')
    });
    const confirmation = el('input', {
        class: 'profile-input', type: 'password', maxlength: '72', autocomplete: 'new-password',
        placeholder: t('login.confirmNewPassword')
    });
    const error = el('p', {class: 'password-recovery-error', role: 'alert'});
    const body = el('div', {class: 'password-recovery-form'},
        el('label', {}, el('span', {}, t('login.recoveryIdentifier')), identifier),
        el('div', {class: 'password-recovery-codes'},
            el('span', {class: 'password-recovery-label'}, t('login.recoveryCodesPrompt')),
            ...codeInputs
        ),
        el('label', {}, el('span', {}, t('profile.newPasswordLabel')), password),
        el('label', {}, el('span', {}, t('login.confirmNewPassword')), confirmation),
        error
    );

    void RenopDialog.show({
        id: 'password-recovery-dialog',
        maxWidth: '620px',
        icon: 'fileKey',
        title: t('login.recoveryTitle'),
        subtitle: t('login.recoverySubtitle'),
        body,
        form: {
            id: 'password-recovery-form',
            className: 'password-recovery-layout',
            onSubmit: async (event, dialog) => {
                event.preventDefault();
                error.textContent = '';
                const passwordError = getPasswordLengthError(password.value);
                if (passwordError) {
                    error.textContent = passwordError;
                    password.focus();
                    return;
                }
                if (password.value !== confirmation.value) {
                    error.textContent = t('login.passwordsDoNotMatch');
                    confirmation.focus();
                    return;
                }
                const codes = codeInputs.map(input => input.value.trim());
                if (codes.some(code => !code) || new Set(codes.map(code => code.toUpperCase())).size !== 4) {
                    error.textContent = t('login.fourDistinctCodesRequired');
                    return;
                }
                if (!(await confirmWeakPasswordIfNeeded(password.value))) return;
                const submit = dialog.querySelector('#password-recovery-submit');
                if (submit) submit.disabled = true;
                try {
                    const response = await fetch('/api/auth/recovery/password', {
                        method: 'POST',
                        credentials: 'include',
                        cache: 'no-store',
                        headers: {'Content-Type': 'application/json'},
                        body: JSON.stringify({
                            identifier: identifier.value.trim(),
                            codes,
                            new_password: password.value,
                        }),
                    });
                    if (!response.ok) {
                        error.textContent = response.status === 429 || response.status === 403
                            ? t('login.recoveryRateLimited')
                            : t('login.recoveryInvalid');
                        return;
                    }
                    const result = await response.json();
                    const loginName = String(result.username || identifier.value.trim());
                    const usernameInput = document.getElementById('username');
                    if (usernameInput) usernameInput.value = loginName;
                    dialog.close(true);
                    showAlert(t('login.recoverySuccess'), 'success');
                    const loginModal = document.getElementById('login-modal');
                    if (loginModal) {
                        loginModal.style.display = 'flex';
                        if (window.updateModalInertState) window.updateModalInertState();
                    }
                } catch (requestError) {
                    console.error('Password recovery failed', requestError);
                    error.textContent = t('login.recoveryFailed');
                } finally {
                    if (submit) submit.disabled = false;
                }
            }
        },
        footer: [
            {
                text: t('common.cancel'),
                className: 'action-btn',
                onClick: (event, dialog) => dialog.close(false)
            },
            {
                id: 'password-recovery-submit',
                text: t('login.resetPassword'),
                className: 'action-btn primary-btn',
                type: 'submit'
            }
        ]
    });
    requestAnimationFrame(() => {
        attachPasswordStrength(password);
        identifier.focus();
    });
}

document.getElementById('btn-forgot-password')?.addEventListener('click', event => {
    event.preventDefault();
    const loginModal = document.getElementById('login-modal');
    if (loginModal && loginModal.style.display !== 'none') {
        closeModalWithAnim(loginModal, openPasswordRecoveryDialog);
        return;
    }
    openPasswordRecoveryDialog();
});
