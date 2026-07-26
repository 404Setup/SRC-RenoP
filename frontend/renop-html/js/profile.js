/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {apiRequest} from './api.js';
import {showAlert} from './alert.js';
import {t} from './i18n.js';
import {attachPasswordStrength, confirmWeakPasswordIfNeeded, getPasswordLengthError} from './password-strength.js';
import {openSessionsDialog} from './sessions.js';

/**
 * Wire up the profile page: password change, upload token generation, and sessions.
 */
export function setupProfile() {
    const username = localStorage.getItem('username') || '';
    const avatarEl = document.getElementById('profile-avatar-initials');
    const displayNameEl = document.getElementById('profile-display-name');
    const usernameHiddenEl = document.getElementById('profile-username-hidden');
    if (usernameHiddenEl) {
        usernameHiddenEl.value = username;
    }
    if (avatarEl) {
        avatarEl.textContent = username ? username.charAt(0).toUpperCase() : '?';
    }
    if (displayNameEl && username) {
        displayNameEl.textContent = username;
    }

    const passwordInput = document.getElementById('profile-new-password');
    const strengthCtrl = passwordInput ? attachPasswordStrength(passwordInput) : null;

    const btnUpdatePassword = document.getElementById('btn-update-password');
    const btnGenerateToken = document.getElementById('btn-generate-upload-token');

    const passwordForm = document.getElementById('profile-password-form');
    if (passwordForm && !passwordForm.dataset.listenerAttached) {
        passwordForm.dataset.listenerAttached = 'true';
        passwordForm.addEventListener('submit', (e) => {
            e.preventDefault();
            if (btnUpdatePassword) {
                btnUpdatePassword.click();
            }
        });
    }

    if (btnUpdatePassword && !btnUpdatePassword.dataset.listenerAttached) {
        btnUpdatePassword.dataset.listenerAttached = 'true';
        btnUpdatePassword.addEventListener('click', async (e) => {
            if (e) e.preventDefault();
            const input = document.getElementById('profile-new-password');
            const newPassword = input.value;
            const lengthError = getPasswordLengthError(newPassword);
            if (lengthError) {
                showAlert(lengthError, 'error');
                return;
            }

            if (!(await confirmWeakPasswordIfNeeded(newPassword))) {
                return;
            }

            try {
                const response = await apiRequest('/api/auth/profile/password', {
                    method: 'PUT',
                    headers: {
                        'Content-Type': 'application/json'
                    },
                    body: JSON.stringify({new_password: newPassword})
                });

                if (response.ok) {
                    showAlert(t('profile.passwordUpdated'), 'success');
                    input.value = '';
                    if (strengthCtrl) strengthCtrl.reset();
                } else {
                    const msg = await response.text();
                    showAlert(t('profile.updatePasswordFailed') + ': ' + msg, 'error');
                }
            } catch (error) {
                showAlert(t('profile.updatePasswordError'), 'error');
            }
        });
    }

    if (btnGenerateToken && !btnGenerateToken.dataset.listenerAttached) {
        btnGenerateToken.dataset.listenerAttached = 'true';
        btnGenerateToken.addEventListener('click', async () => {
            if (!(await window.showConfirm(t('profile.confirmGenToken')))) {
                return;
            }

            try {
                const response = await apiRequest('/api/auth/profile/token', {
                    method: 'POST'
                });

                if (response.ok) {
                    const data = await response.json();
                    const tokenEl = document.getElementById('profile-upload-token-value');
                    tokenEl.textContent = data.token;
                    tokenEl.style.cursor = 'pointer';
                    tokenEl.title = t('prompt.clickToCopy');
                    tokenEl.onclick = async () => {
                        try {
                            await navigator.clipboard.writeText(data.token);
                            showAlert(t('prompt.copied'), 'success');
                        } catch (err) {
                        }
                    };
                    document.getElementById('profile-token-result').style.display = 'block';
                    showAlert(t('profile.tokenGeneratedSuccess'), 'success');
                } else {
                    const msg = await response.text();
                    showAlert(t('profile.genTokenFailed') + ': ' + msg, 'error');
                }
            } catch (error) {
                showAlert(t('profile.genTokenError'), 'error');
            }
        });
    }

    const btnSessions = document.getElementById('btn-profile-sessions');
    if (btnSessions && !btnSessions.dataset.listenerAttached) {
        btnSessions.dataset.listenerAttached = 'true';
        btnSessions.addEventListener('click', () => {
            openSessionsDialog({mode: 'self'});
        });
    }
}
