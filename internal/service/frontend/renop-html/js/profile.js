/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {apiRequest, fetchProto, postProto, putProto} from './api.js';
import {showAlert} from './alert.js';
import {t, translateError} from './i18n.js';
import {el} from '@renop/ui/dom';
import {createIcon, RenopDialog} from './components.js';
import {attachPasswordStrength, confirmWeakPasswordIfNeeded, getPasswordLengthError} from './password-strength.js';
import {openSessionsDialog} from './sessions.js';
import {base64urlToBuffer, bufferToBase64url} from './fido-utils.js';
import {
	FidoDeviceList,
	GenerateTokenResponse,
	GpgKeyDto,
	GpgKeyList,
	GpgKeyReferenceRequest,
	GpgReleaseList,
	StatusOk,
	UpdatePasswordRequest
} from './proto/index.js';
import {closeModalWithAnim} from './app-ui.js';
import {openAuditLogsDialog} from './audit.js';
import {morphElementHeight} from '@renop/ui/height-anim';

let profileFidoLoadSeq = 0;

/**
 * Format a GPG timestamp for the current locale.
 * @param {number|string} value Unix milliseconds
 * @returns {string}
 */
function formatGPGDate(value) {
	const timestamp = Number(value);
	return Number.isFinite(timestamp) && timestamp > 0
		? new Date(timestamp).toLocaleDateString()
		: t('profile.gpgNoExpiry');
}

/**
 * Load and render registered GPG keys inside the profile dialog.
 * @param {HTMLElement} list
 * @param {HTMLElement} count
 * @param {HTMLInputElement} input
 * @param {HTMLButtonElement} addButton
 * @returns {Promise<void>}
 */
async function loadProfileGPGKeys(list, count, input, addButton) {
	await morphElementHeight(list, () => {
		list.replaceChildren(el('div', {class: 'sessions-loading'},
			el('div', {class: 'sessions-loading-spinner', 'aria-hidden': 'true'}),
			el('span', {}, t('profile.gpgLoading'))
		));
	}, {duration: 280});
	try {
		const {response, data} = await fetchProto('/api/auth/profile/gpg', GpgKeyList);
		if (!response.ok || !data) {
			throw new Error(await response.text() || 'Failed to load GPG keys');
		}
		const keys = Array.isArray(data.keys) ? data.keys : [];
		count.textContent = t('profile.gpgKeyCount', {count: keys.length});
		input.disabled = keys.length >= 10;
		addButton.disabled = keys.length >= 10;
		await morphElementHeight(list, () => {
			list.replaceChildren();
			if (keys.length === 0) {
				list.appendChild(el('div', {class: 'gpg-key-empty'}, t('profile.gpgKeysEmpty')));
				return;
			}
			keys.forEach(key => {
				const identity = el('strong', {class: 'gpg-key-identity'}, key.primary_identity || t('profile.gpgUnknownIdentity'));
				const fingerprint = el('code', {class: 'gpg-key-fingerprint'}, key.fingerprint || '');
				const dates = el('span', {class: 'gpg-key-dates'},
					t('profile.gpgKeyDates', {
						added: formatGPGDate(key.added_at),
						expires: formatGPGDate(key.key_expires_at)
					})
				);
				const remove = el('button', {
					type: 'button',
					class: 'file-action-btn file-action-btn--delete',
					title: t('profile.gpgDeleteKey'),
					ariaLabel: t('profile.gpgDeleteKey')
				}, createIcon('delete'));
				remove.addEventListener('click', async () => {
					if (!(await window.showConfirm(t('profile.gpgConfirmDelete')))) return;
					const deleteResponse = await apiRequest(`/api/auth/profile/gpg/${encodeURIComponent(key.fingerprint)}`, {method: 'DELETE'});
					if (!deleteResponse.ok) {
						showAlert(t('profile.gpgDeleteFailed'), 'error');
						return;
					}
					showAlert(t('profile.gpgDeleted'), 'success');
					await loadProfileGPGKeys(list, count, input, addButton);
				});
				list.appendChild(el('div', {class: 'gpg-key-item'},
					el('div', {class: 'gpg-key-info'}, identity, fingerprint, dates),
					remove
				));
			});
		}, {duration: 300});
	} catch (error) {
		console.error('Failed to load GPG keys', error);
		await morphElementHeight(list, () => {
			list.replaceChildren(el('div', {class: 'gpg-key-error'}, t('profile.gpgLoadFailed')));
		}, {duration: 280});
	}
}

/**
 * Open the profile dialog used to register and remove GPG public-key IDs.
 * @returns {void}
 */
function openProfileGPGDialog() {
	const input = el('input', {
		type: 'text',
		class: 'profile-input',
		placeholder: t('profile.gpgKeyPlaceholder'),
		autocomplete: 'off',
		spellcheck: 'false',
		maxlength: '66'
	});
	const addButton = el('button', {
		type: 'submit',
		class: 'pill-btn pill-btn--primary'
	}, createIcon('plus'), el('span', {}, t('profile.gpgAddKey')));
	const count = el('span', {class: 'gpg-key-count'}, t('profile.gpgKeyCount', {count: 0}));
	const list = el('div', {class: 'gpg-key-list'});
	const form = el('form', {class: 'gpg-key-add-form', action: 'javascript:void(0);'}, input, addButton);
	form.addEventListener('submit', async event => {
		event.preventDefault();
		let reference = input.value.trim().replace(/^0x/i, '').replace(/\s+/g, '');
		if (!/^(?:[0-9a-fA-F]{16}|[0-9a-fA-F]{40}|[0-9a-fA-F]{64})$/.test(reference)) {
			showAlert(t('profile.gpgInvalidKey'), 'error');
			return;
		}
		addButton.disabled = true;
		try {
			const {response} = await postProto(
				'/api/auth/profile/gpg',
				GpgKeyReferenceRequest,
				{key_id: reference},
				GpgKeyDto
			);
			if (!response.ok) {
				showAlert(await response.text() || t('profile.gpgAddFailed'), 'error');
				return;
			}
			input.value = '';
			showAlert(t('profile.gpgAdded'), 'success');
			await loadProfileGPGKeys(list, count, input, addButton);
		} catch (error) {
			console.error('Failed to register GPG key', error);
			showAlert(t('profile.gpgAddFailed'), 'error');
		} finally {
			if (!input.disabled) addButton.disabled = false;
		}
	});
	const body = el('div', {class: 'gpg-key-dialog-body'}, form, count, list);
	void RenopDialog.show({
		id: 'profile-gpg-dialog',
		maxWidth: '680px',
		icon: 'fileKey',
		title: t('profile.gpgTitle'),
		subtitle: t('profile.gpgDialogDesc'),
		body,
		footer: [{
			text: t('common.close'),
			className: 'action-btn',
			onClick: (event, dialog) => dialog.close(true)
		}]
	});
	void loadProfileGPGKeys(list, count, input, addButton);
}

/**
 * Translate a durable publication status into a user-facing label.
 * @param {string} status
 * @returns {string}
 */
function profileGPGReleaseStatusLabel(status) {
	const normalized = String(status || '').toLowerCase();
	return t(`profile.gpgReleaseStatus.${normalized}`) || normalized;
}

/**
 * Build one GPG publication-history row without injecting server text as HTML.
 * @param {object} release
 * @returns {HTMLElement}
 */
function createProfileGPGReleaseItem(release) {
	const status = String(release.status || 'queued').toLowerCase();
	const statusBadge = el('span', {
		class: `gpg-release-status gpg-release-status--${status}`
	}, profileGPGReleaseStatusLabel(status));
	const artifact = el('code', {class: 'gpg-release-path'}, release.artifact_path || '');
	const repository = el('span', {class: 'gpg-release-meta-item'},
		t('profile.gpgReleaseRepository', {name: release.repository || ''})
	);
	const signing = el('span', {class: 'gpg-release-meta-item'},
		release.signed ? t('profile.gpgReleaseSigned') : t('profile.gpgReleaseUnsigned')
	);
	const created = el('span', {class: 'gpg-release-meta-item'},
		t('profile.gpgReleaseCreated', {date: formatGPGDate(release.created_at)})
	);
	const children = [
		el('div', {class: 'gpg-release-item-header'}, artifact, statusBadge),
		el('div', {class: 'gpg-release-meta'}, repository, signing, created)
	];
	if (status === 'failed' && release.failure_reason) {
		children.push(el('div', {class: 'gpg-release-failure'}, translateError(release.failure_reason)));
	}
	return el('div', {class: 'gpg-release-item'}, ...children);
}

/**
 * @typedef {object} GPGReleaseView
 * @property {HTMLElement} list
 * @property {HTMLElement} summary
 * @property {HTMLButtonElement} previous
 * @property {HTMLButtonElement} next
 * @property {number} offset
 * @property {number} limit
 * @property {boolean} loading
 * @property {boolean} hasActive
 */

/**
 * Fetch and render one page of the current user's GPG publication history.
 * @param {GPGReleaseView} view
 * @param {boolean} [showLoading=false]
 * @returns {Promise<boolean>} whether queued or validating records remain
 */
async function loadProfileGPGReleases(view, showLoading = false) {
	if (view.loading) return view.hasActive;
	view.loading = true;
	if (showLoading) {
		await morphElementHeight(view.list, () => {
			view.list.replaceChildren(el('div', {class: 'sessions-loading'},
				el('div', {class: 'sessions-loading-spinner', 'aria-hidden': 'true'}),
				el('span', {}, t('profile.gpgReleasesLoading'))
			));
		}, {duration: 240});
	}
	try {
		const query = new URLSearchParams({limit: String(view.limit), offset: String(view.offset)});
		const {response, data} = await fetchProto(`/api/auth/profile/gpg/releases?${query}`, GpgReleaseList);
		if (!response.ok || !data) {
			throw new Error(await response.text() || 'Failed to load GPG releases');
		}
		const releases = Array.isArray(data.releases) ? data.releases : [];
		const total = Number(data.total) || 0;
		view.hasActive = releases.some(release => release.status === 'queued' || release.status === 'validating');
		view.previous.disabled = view.offset <= 0;
		view.next.disabled = view.offset + view.limit >= total;
		if (total === 0) {
			view.summary.textContent = '';
		} else {
			view.summary.textContent = t('profile.gpgReleaseRange', {
				start: view.offset + 1,
				end: Math.min(view.offset + releases.length, total),
				total
			});
		}
		await morphElementHeight(view.list, () => {
			view.list.replaceChildren();
			if (releases.length === 0) {
				view.list.appendChild(el('div', {class: 'gpg-key-empty'}, t('profile.gpgReleasesEmpty')));
				return;
			}
			releases.forEach(release => view.list.appendChild(createProfileGPGReleaseItem(release)));
		}, {duration: 260});
		return view.hasActive;
	} catch (error) {
		console.error('Failed to load GPG releases', error);
		view.hasActive = false;
		await morphElementHeight(view.list, () => {
			view.list.replaceChildren(el('div', {class: 'gpg-key-error'}, t('profile.gpgReleasesLoadFailed')));
		}, {duration: 240});
		return false;
	} finally {
		view.loading = false;
	}
}

/**
 * Open the paginated GPG publication-history dialog and poll active records.
 * @returns {void}
 */
function openProfileGPGReleasesDialog() {
	const list = el('div', {class: 'gpg-release-list'});
	const summary = el('span', {class: 'gpg-release-summary'}, t('profile.gpgReleasesLoading'));
	const previous = el('button', {
		type: 'button',
		class: 'file-action-btn',
		title: t('common.prev'),
		ariaLabel: t('common.prev')
	}, createIcon('chevronLeft'));
	const next = el('button', {
		type: 'button',
		class: 'file-action-btn',
		title: t('common.next'),
		ariaLabel: t('common.next')
	}, createIcon('chevronRight'));
	const refresh = el('button', {
		type: 'button',
		class: 'file-action-btn',
		title: t('profile.gpgReleasesRefresh'),
		ariaLabel: t('profile.gpgReleasesRefresh')
	}, createIcon('refresh'));
	/** @type {GPGReleaseView} */
	const view = {list, summary, previous, next, offset: 0, limit: 20, loading: false, hasActive: true};

	previous.addEventListener('click', () => {
		view.offset = Math.max(0, view.offset - view.limit);
		void loadProfileGPGReleases(view, true);
	});
	next.addEventListener('click', () => {
		view.offset += view.limit;
		void loadProfileGPGReleases(view, true);
	});
	refresh.addEventListener('click', () => void loadProfileGPGReleases(view, true));

	const controls = el('div', {class: 'gpg-release-controls'},
		summary,
		el('div', {class: 'gpg-release-actions'}, refresh, previous, next)
	);
	const body = el('div', {class: 'gpg-release-dialog-body'}, controls, list);
	const closed = RenopDialog.show({
		id: 'profile-gpg-releases-dialog',
		maxWidth: '760px',
		icon: 'clock',
		title: t('profile.gpgReleasesTitle'),
		subtitle: t('profile.gpgReleasesDialogDesc'),
		body,
		footer: [{
			text: t('common.close'),
			className: 'action-btn',
			onClick: (event, dialog) => dialog.close(true)
		}]
	});
	void loadProfileGPGReleases(view, true);
	const pollTimer = window.setInterval(() => {
		if (view.hasActive) void loadProfileGPGReleases(view, false);
	}, 2000);
	void closed.finally(() => window.clearInterval(pollTimer));
}

export async function loadProfileFidoDevices() {
    const listEl = document.getElementById('profile-fido-list');
    if (!listEl) return;

    const seq = ++profileFidoLoadSeq;
    void morphElementHeight(listEl, () => {
        listEl.replaceChildren(
            el('div', {class: 'sessions-loading'},
                el('div', {class: 'sessions-loading-spinner', 'aria-hidden': 'true'}),
                el('span', {}, t('fido.loading') || t('common.loading') || 'Loading...')
            )
        );
    }, {duration: 300});

    try {
        const {response, data} = await fetchProto('/api/auth/profile/fido', FidoDeviceList);
        if (seq !== profileFidoLoadSeq) return;

        if (!response.ok || !data) {
            await morphElementHeight(listEl, () => {
                listEl.replaceChildren(
                    el('div', {
                        style: {
                            padding: '1rem',
                            textAlign: 'center',
                            color: '#ef4444',
                        }
                    }, t('error.fidoLoadFailed') || 'Failed to load FIDO devices')
                );
            }, {duration: 300});
            return;
        }
        const devices = data.devices || [];

        await morphElementHeight(listEl, () => {
            listEl.replaceChildren();
            if (!Array.isArray(devices) || devices.length === 0) {
                listEl.appendChild(el('div', {
                    style: {
                        opacity: '0.6',
                        fontSize: '0.85rem',
                        padding: '0.5rem 0',
                    }
                }, t('common.none') || 'No FIDO devices registered'));
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
        }, {duration: 300});
    } catch (err) {
        if (seq !== profileFidoLoadSeq) return;
        console.error('Failed to load FIDO devices:', err);
        await morphElementHeight(listEl, () => {
            listEl.replaceChildren(
                el('div', {
                    style: {
                        padding: '1rem',
                        textAlign: 'center',
                        color: '#ef4444',
                    }
                }, t('error.fidoLoadFailed') || 'Error loading FIDO devices')
            );
        }, {duration: 300});
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
            if (publicKey.excludeCredentials.length === 0) {
                delete publicKey.excludeCredentials;
            }
        }

        if (!publicKey.authenticatorSelection) {
            publicKey.authenticatorSelection = {};
        }
        delete publicKey.authenticatorSelection.authenticatorAttachment;
        if (!publicKey.authenticatorSelection.userVerification) {
            publicKey.authenticatorSelection.userVerification = 'preferred';
        }
        if (!publicKey.authenticatorSelection.residentKey) {
            publicKey.authenticatorSelection.residentKey = 'preferred';
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

	const btnProfileGPG = document.getElementById('btn-profile-gpg');
	if (btnProfileGPG && !btnProfileGPG.dataset.listenerAttached) {
		btnProfileGPG.dataset.listenerAttached = 'true';
		btnProfileGPG.addEventListener('click', openProfileGPGDialog);
	}

	const btnProfileGPGReleases = document.getElementById('btn-profile-gpg-releases');
	if (btnProfileGPGReleases && !btnProfileGPGReleases.dataset.listenerAttached) {
		btnProfileGPGReleases.dataset.listenerAttached = 'true';
		btnProfileGPGReleases.addEventListener('click', openProfileGPGReleasesDialog);
	}

    const btnAddFido = document.getElementById('btn-add-fido-device');
    if (btnAddFido && !btnAddFido.dataset.listenerAttached) {
        btnAddFido.dataset.listenerAttached = 'true';
        btnAddFido.addEventListener('click', () => {
            addFidoDevice();
        });
    }

    const btnAuditLogs = document.getElementById('btn-profile-audit-logs');
    if (btnAuditLogs && !btnAuditLogs.dataset.listenerAttached) {
        btnAuditLogs.dataset.listenerAttached = 'true';
        btnAuditLogs.addEventListener('click', () => {
            openAuditLogsDialog({mode: 'self'});
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
        closeBtn.addEventListener('click', () => {
            profileFidoLoadSeq += 1;
            closeModalWithAnim(modal);
        });
    }

    if (backdrop && !backdrop.dataset.listenerAttached) {
        backdrop.dataset.listenerAttached = 'true';
        backdrop.addEventListener('click', () => {
            profileFidoLoadSeq += 1;
            closeModalWithAnim(modal);
        });
    }

    modal.style.display = 'flex';
    if (window.updateModalInertState) window.updateModalInertState();
    loadProfileFidoDevices();
}
