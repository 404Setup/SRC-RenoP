/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {apiRequest, fetchProto, putProto} from './api.js';
import {showAlert} from './alert.js';
import {t} from './i18n.js';
import {el} from './cfg-ui.js';
import {RenopDialog} from './components.js';
import {attachPasswordStrength, confirmWeakPasswordIfNeeded, getPasswordLengthError} from './password-strength.js';
import {openSessionsDialog} from './sessions.js';
import {base64urlToBuffer, bufferToBase64url} from './fido-utils.js';
import {FidoDeviceList, GenerateTokenResponse, StatusOk, UpdatePasswordRequest} from './proto/index.js';
import {closeModalWithAnim} from './app-ui.js';

export async function loadProfileFidoDevices() {
    const listEl = document.getElementById('profile-fido-list');
    if (!listEl) return;

    try {
        const {response, data} = await fetchProto('/api/auth/profile/fido', FidoDeviceList);
        if (!response.ok || !data) return;
        const devices = data.devices || [];

        listEl.innerHTML = '';
        if (!Array.isArray(devices) || devices.length === 0) {
            listEl.innerHTML = `<div style="opacity: 0.6; font-size: 0.85rem; padding: 0.5rem 0;">${t('common.none') || 'No FIDO devices registered'}</div>`;
            return;
        }

        devices.forEach(dev => {
            const item = document.createElement('div');
            item.className = 'fido-device-item';
            item.style.cssText = 'display: flex; align-items: center; justify-content: space-between; padding: 0.6rem 0.8rem; border: 1px solid var(--border-color, #e5e7eb); border-radius: 8px; margin-bottom: 0.5rem; background: var(--card-bg, #ffffff);';

            const info = document.createElement('div');
            info.style.cssText = 'display: flex; flex-direction: column; gap: 2px;';
            const nameEl = document.createElement('span');
            nameEl.style.cssText = 'font-weight: 600; font-size: 0.9rem;';
            nameEl.textContent = dev.name || 'FIDO Device';
            const dateEl = document.createElement('span');
            dateEl.style.cssText = 'font-size: 0.78rem; opacity: 0.65;';
            dateEl.textContent = new Date(dev.created_at).toLocaleString();
            info.appendChild(nameEl);
            info.appendChild(dateEl);

            const delBtn = document.createElement('button');
            delBtn.type = 'button';
            delBtn.className = 'pill-btn pill-btn--danger';
            delBtn.style.cssText = 'padding: 4px 10px; font-size: 0.8rem;';
            delBtn.textContent = t('common.delete') || 'Delete';
            delBtn.addEventListener('click', async () => {
                const confirmMsg = t('profile.confirmDeleteFido', {name: dev.name}) || `Are you sure you want to delete FIDO device "${dev.name}"?`;
                if (await window.showConfirm(confirmMsg)) {
                    try {
                        const delRes = await apiRequest(`/api/auth/profile/fido/${dev.id}`, {method: 'DELETE'});
                        if (delRes.ok) {
                            showAlert(t('profile.fidoDeleted') || 'FIDO device deleted', 'success');
                            loadProfileFidoDevices();
                        } else {
                            showAlert(t('common.error') || 'Failed to delete FIDO device', 'error');
                        }
                    } catch (err) {
                        showAlert(t('common.error') || 'Failed to delete FIDO device', 'error');
                    }
                }
            });

            item.appendChild(info);
            item.appendChild(delBtn);
            listEl.appendChild(item);
        });
    } catch (err) {
        console.error('Failed to load FIDO devices:', err);
    }
}

export async function addFidoDevice() {
    if (!window.PublicKeyCredential) {
        showAlert(t('login.fidoUnsupported') || 'FIDO/WebAuthn is not supported by your browser', 'error');
        return;
    }

    const deviceName = await window.showPrompt(
        t('profile.fidoPromptName') || 'Enter a name for your FIDO device (e.g. MacBook TouchID, YubiKey 5):',
        'YubiKey 5'
    );

    if (!deviceName || !deviceName.trim()) {
        return;
    }

    try {
        const beginRes = await apiRequest('/api/auth/profile/fido/register/begin', {
            method: 'POST'
        });
        if (!beginRes.ok) {
            const msg = await beginRes.text();
            const translatedMsg = window.translateError ? window.translateError(msg) : msg;
            showAlert(translatedMsg || t('error.fidoBeginRegFailed') || 'Failed to begin FIDO registration', 'error');
            return;
        }

        const {session_id, options} = await beginRes.json();
        const publicKey = options.publicKey;
        publicKey.challenge = base64urlToBuffer(publicKey.challenge);
        publicKey.user.id = base64urlToBuffer(publicKey.user.id);
        if (Array.isArray(publicKey.excludeCredentials)) {
            publicKey.excludeCredentials = publicKey.excludeCredentials.map(c => ({
                ...c,
                id: base64urlToBuffer(c.id)
            }));
        }

        const credential = await navigator.credentials.create({publicKey});
        if (!credential) {
            showAlert(t('login.fidoFailed') || 'FIDO registration cancelled', 'error');
            return;
        }

        const credentialJSON = {
            id: credential.id,
            rawId: bufferToBase64url(credential.rawId),
            type: credential.type,
            response: {
                attestationObject: bufferToBase64url(credential.response.attestationObject),
                clientDataJSON: bufferToBase64url(credential.response.clientDataJSON),
            }
        };

        const finishRes = await apiRequest('/api/auth/profile/fido/register/finish', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({
                session_id,
                name: deviceName.trim(),
                credential: credentialJSON
            })
        });

        if (finishRes.ok) {
            showAlert(t('profile.fidoAdded') || 'FIDO device added successfully!', 'success');
            loadProfileFidoDevices();
        } else {
            const msg = await finishRes.text();
            const translatedMsg = window.translateError ? window.translateError(msg) : msg;
            showAlert(translatedMsg || t('error.fidoRegFailed') || 'Failed to finish FIDO registration', 'error');
        }
    } catch (err) {
        console.error('FIDO registration error:', err);
        const errMsg = err && (err.message || err.name || String(err));
        const translatedMsg = errMsg && window.translateError ? window.translateError(errMsg) : errMsg;
        showAlert(translatedMsg || t('error.fidoRegFailed') || 'FIDO registration failed', 'error');
    }
}

/**
 * Wire up the profile page: password change, upload token generation, sessions, and FIDO devices.
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
                const {response} = await putProto('/api/auth/profile/password', UpdatePasswordRequest, {new_password: newPassword}, StatusOk);

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
                const {
                    response,
                    data
                } = await fetchProto('/api/auth/profile/token', GenerateTokenResponse, {method: 'POST'});

                if (response.ok && data) {
                    const tokenNode = el('div', {class: 'profile-token-reveal', style: {marginTop: '0'}},
                        el('div', {class: 'profile-token-label'}, t('profile.newTokenLabel')),
                        el('div', {class: 'profile-token-value-wrapper'},
                            el('code', {
                                class: 'profile-token-code',
                                style: {cursor: 'pointer'},
                                title: t('prompt.clickToCopy'),
                                onClick: async () => {
                                    try {
                                        await navigator.clipboard.writeText(data.token);
                                        showAlert(t('prompt.copied'), 'success');
                                    } catch (err) {
                                    }
                                }
                            }, data.token)
                        ),
                        el('p', {class: 'profile-token-warning'}, t('profile.tokenWarning'))
                    );

                    RenopDialog.show({
                        id: 'profile-token-modal',
                        maxWidth: '500px',
                        icon: 'fileKey',
                        title: t('profile.uploadTokenTitle'),
                        body: tokenNode,
                        footer: [
                            {
                                text: t('common.ok'),
                                className: 'action-btn primary-btn',
                                onClick: (e, d) => d.close(true)
                            }
                        ]
                    });

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

    const btnProfileFido = document.getElementById('btn-profile-fido');
    if (btnProfileFido && !btnProfileFido.dataset.listenerAttached) {
        btnProfileFido.dataset.listenerAttached = 'true';
        btnProfileFido.addEventListener('click', () => {
            openProfileFidoDialog();
        });
    }

    const btnAddFido = document.getElementById('btn-add-fido-device');
    if (btnAddFido && !btnAddFido.dataset.listenerAttached) {
        btnAddFido.dataset.listenerAttached = 'true';
        btnAddFido.addEventListener('click', () => {
            addFidoDevice();
        });
    }
}

export function openProfileFidoDialog() {
    const modal = document.getElementById('profile-fido-modal');
    const closeBtn = document.getElementById('close-profile-fido-modal');
    const backdrop = document.getElementById('profile-fido-backdrop');
    if (!modal) return;

    if (closeBtn && !closeBtn.dataset.listenerAttached) {
        closeBtn.dataset.listenerAttached = 'true';
        closeBtn.addEventListener('click', () => closeModalWithAnim(modal));
    }

    if (backdrop && !backdrop.dataset.listenerAttached) {
        backdrop.dataset.listenerAttached = 'true';
        backdrop.addEventListener('click', () => closeModalWithAnim(modal));
    }

    modal.style.display = 'flex';
    if (window.updateModalInertState) window.updateModalInertState();
    loadProfileFidoDevices();
}
