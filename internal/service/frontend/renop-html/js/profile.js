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
import {t, translateKnownError} from './i18n.js';
import {el} from '@renop/ui/dom';
import {createIcon, RenopDialog} from './components.js';
import {attachPasswordStrength, confirmWeakPasswordIfNeeded, getPasswordLengthError} from './password-strength.js';
import {openSessionsDialog} from './sessions.js';
import {base64urlToBuffer, bufferToBase64url} from './fido-utils.js';
import {
	FidoDeviceList,
	GpgKeyDto,
	GpgKeyList,
	GpgKeyReferenceRequest,
	GpgReleaseList,
	StatusOk,
	UpdatePasswordRequest
} from './proto/index.js';
import {closeModalWithAnim} from './app-ui.js';
import {openAuditLogsDialog} from './audit.js';
import {formatTimestamp} from './time.js';
import {getRepositoryFormat} from './repository-formats.js';
import {renderGitHubConnection} from './github-auth.js';
import {refreshAccountSecurity} from './account-security.js';
import {refreshAPITokenSummary} from './api-tokens.js';
import {renderProfileSuperTeamLimits} from './super-teams.js';
import {
	caughtErrorMessage,
	LocalizedResponseError,
	localizedResponseError,
	responseErrorMessage
} from './response-errors.js';
import {collapseElement, expandElement, morphElementHeight} from '@renop/ui/height-anim';
import {
	getUserProfile,
	invalidateUserProfiles,
	navigateToUserProfile,
	profileRouteFromPath,
	profileDisplayName,
} from './user-profiles.js';

let profileFidoLoadSeq = 0;
let profilePageLoadSeq = 0;

/**
 * Format a GPG timestamp for the current locale.
 * @param {number|string} value Unix milliseconds
 * @returns {string}
 */
function formatGPGDate(value) {
	return formatTimestamp(value, {dateOnly: true, fallback: t('profile.gpgNoExpiry')});
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
			throw await localizedResponseError(response, 'profile.gpgLoadFailed');
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
				showAlert(await responseErrorMessage(response, 'profile.gpgAddFailed'), 'error');
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
		children.push(el('div', {class: 'gpg-release-failure'},
			translateKnownError(release.failure_reason) || t('profile.gpgFailure.generic')));
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
			throw await localizedResponseError(response, 'profile.gpgReleasesLoadFailed');
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
                    }, t('error.fidoLoadFailed') || 'Failed to load Passkeys')
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
                }, t('common.none')));
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
                nameEl.textContent = dev.name || t('profile.fidoTitle');
                const dateEl = document.createElement('span');
                dateEl.style.cssText = 'font-size: 0.78rem; opacity: 0.65;';
                dateEl.textContent = formatTimestamp(dev.created_at, {fallback: t('common.unknown')});
                info.appendChild(nameEl);
                info.appendChild(dateEl);

                const delBtn = document.createElement('button');
                delBtn.type = 'button';
                delBtn.className = 'pill-btn pill-btn--danger';
                delBtn.style.cssText = 'padding: 4px 10px; font-size: 0.8rem;';
                delBtn.textContent = t('common.delete');
                delBtn.addEventListener('click', async () => {
                    const confirmMsg = t('profile.confirmDeleteFido', {name: dev.name});
                    if (await window.showConfirm(confirmMsg)) {
                        try {
                            const delRes = await apiRequest(`/api/auth/profile/fido/${dev.id}`, {method: 'DELETE'});
                            if (delRes.ok) {
                                showAlert(t('profile.fidoDeleted'), 'success');
                                loadProfileFidoDevices();
                                void refreshAccountSecurity();
                            } else {
                                showAlert(await responseErrorMessage(delRes, 'error.fidoDeleteFailed'), 'error');
                            }
                        } catch (err) {
                            console.error('Failed to delete FIDO device', err);
                            showAlert(caughtErrorMessage(err, 'error.fidoDeleteFailed'), 'error');
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
                }, t('error.fidoLoadFailed'))
            );
        }, {duration: 300});
    }
}

export async function addFidoDevice() {
    if (!window.PublicKeyCredential) {
        showAlert(t('login.fidoUnsupported'), 'error');
        return;
    }

    const deviceName = await window.showPrompt(
        t('profile.fidoPromptName'),
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
            showAlert(await responseErrorMessage(beginRes, 'error.fidoBeginRegFailed'), 'error');
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
            showAlert(t('login.fidoFailed'), 'error');
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
            showAlert(t('profile.fidoAdded'), 'success');
            loadProfileFidoDevices();
            void refreshAccountSecurity();
        } else {
            showAlert(await responseErrorMessage(finishRes, 'error.fidoRegFailed'), 'error');
        }
    } catch (err) {
        console.error('FIDO registration error:', err);
        showAlert(caughtErrorMessage(err, 'error.fidoRegFailed'), 'error');
    }
}

/**
 * Format the account creation timestamp for the current locale.
 * @param {string} value - RFC 3339 timestamp.
 * @returns {string} Localized date or a fallback label.
 */
function formatProfileCreatedAt(value) {
    return formatTimestamp(value, {dateOnly: true, fallback: t('common.unknown')});
}

/**
 * Return the first visible character for the profile avatar.
 * @param {object} profile - Public profile payload.
 * @returns {string} Uppercase avatar character.
 */
function profileAvatarLetter(profile) {
    const characters = Array.from(profileDisplayName(profile));
    return characters[0]?.toUpperCase() || '?';
}

/**
 * Update the account heading shown above the profile editor.
 * @param {object} profile - Own profile payload.
 * @returns {void}
 */
function updateProfileEditHeading(profile) {
    const editView = document.getElementById('profile-edit-view');
    if (!editView) return;
    const avatar = document.getElementById('profile-avatar-initials');
    const heading = document.getElementById('profile-display-name');
    const subtitle = editView.querySelector('.profile-hero-sub');
    if (avatar) avatar.textContent = profileAvatarLetter(profile);
    if (heading) heading.textContent = profileDisplayName(profile);
    if (subtitle) subtitle.textContent = `@${profile.username} · ${t('profile.editSubtitle')}`;
}

/**
 * Format the username-change allowance displayed in the identity editor.
 * @param {object} profile - Own profile payload.
 * @returns {string} Localized allowance text.
 */
function profileRenameHint(profile) {
    const remaining = Number(profile.username_changes_remaining || 0);
    const resetAt = Number(profile.username_change_window_resets_at || 0);
    return remaining > 0
        ? t('profile.renameRemaining', {count: remaining})
        : t('profile.renameUnavailable', {
            date: formatTimestamp(resetAt, {fallback: t('profile.later')})
        });
}

/**
 * Leave the profile route while preserving normal browser history.
 * @returns {void}
 */
function leaveUserProfileRoute() {
    const route = profileRouteFromPath(window.location.pathname);
    if (window.history.state?.renopProfileCanGoBack) {
        window.history.back();
        return;
    }
    if (route?.section) {
        window.history.replaceState({
            renopProfileReturnPath: '/',
            renopProfileCanGoBack: false
        }, '', `/user/${encodeURIComponent(route.username)}`);
    } else {
        window.history.replaceState(null, '', window.history.state?.renopProfileReturnPath || '/');
    }
    window.dispatchEvent(new PopStateEvent('popstate'));
}

/**
 * Navigate from a profile membership row into the repository browser.
 * @param {MouseEvent} event - Membership link click.
 * @returns {void}
 */
function openProfilePackage(event) {
    if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
    event.preventDefault();
    const path = new URL(event.currentTarget.href).pathname;
    window.history.pushState(null, '', path);
    window.dispatchEvent(new PopStateEvent('popstate'));
}

/**
 * Build the repository-browser path for one public membership entry.
 * @param {object} membership - Membership payload.
 * @returns {string} Encoded application path.
 */
function profileMembershipTarget(membership) {
    const repository = encodeURIComponent(String(membership.repository || ''));
    const name = String(membership.name || '').split('/').filter(Boolean).map(encodeURIComponent).join('/');
    if (membership.format === 'maven') return `/${repository}/domains/${name}`;
    if (membership.format === 'cargo') return `/${repository}/packages/${name}`;
    if (membership.format === 'npm') return `/${repository}/packages/${name}`;
    return `/${repository}/${name}`;
}

/**
 * Format a package-team role without hiding its numeric level.
 * @param {'maven'|'cargo'|'docker'|'npm'} format - Repository format.
 * @param {number|string} level - Team permission level.
 * @returns {string} Localized role label.
 */
function profileMembershipRole(format, level) {
    const numericLevel = Math.max(0, Number(level) || 0);
    const key = format === 'npm' ? `npm.level${numericLevel}` : `${format}.permissionL${numericLevel}`;
    const label = t(key);
    if (!label || label === key) return `L${numericLevel}`;
    const wrapped = new RegExp(`^L${numericLevel}\\s*[（(](.*)[）)]$`).exec(label);
    return `L${numericLevel} · ${wrapped ? wrapped[1] : label}`;
}

/**
 * Render a username-scoped package membership list.
 * @param {object} profile - Public profile payload.
 * @param {'maven'|'cargo'|'docker'|'npm'} format - Requested membership format.
 * @param {number} sequence - Profile load generation.
 * @returns {Promise<void>}
 */
async function renderProfileMemberships(profile, format, sequence) {
    const publicView = document.getElementById('profile-public-view');
    if (!publicView) return;
    const displayName = profileDisplayName(profile);
    const titleKey = `profile.${format}MembershipsTitle`;
    const list = el('div', {class: 'profile-membership-list'},
        el('div', {class: 'profile-route-loading'},
            el('div', {class: 'sessions-loading-spinner', 'aria-hidden': 'true'}),
            el('span', {}, t('common.loading'))
        )
    );
    publicView.replaceChildren(
        el('button', {type: 'button', class: 'profile-route-back', onclick: leaveUserProfileRoute},
            createIcon('chevronLeft'), el('span', {}, t('profile.backToProfile'))),
        el('section', {class: 'profile-memberships-card'},
            el('header', {class: 'profile-memberships-header'},
                el('span', {class: `profile-memberships-icon is-${format}`, 'aria-hidden': 'true'},
                    createIcon(getRepositoryFormat(format).icon || 'repositoryFiles')),
                el('div', {},
                    el('h2', {}, t(titleKey, {name: displayName})),
                    el('p', {}, `@${profile.username}`)
                )
            ),
            list
        )
    );
    try {
        const response = await apiRequest(
            `/api/users/${encodeURIComponent(profile.username)}/memberships?format=${encodeURIComponent(format)}`
        );
        if (sequence !== profilePageLoadSeq) return;
        if (!response.ok) throw await localizedResponseError(response, 'profile.membershipsLoadFailed');
        const payload = await response.json();
        const memberships = Array.isArray(payload.memberships) ? payload.memberships : [];
        await morphElementHeight(list, () => {
            list.replaceChildren();
            if (memberships.length === 0) {
                list.appendChild(el('div', {class: 'profile-memberships-empty'}, t('profile.membershipsEmpty')));
                return;
            }
            memberships.forEach(membership => {
                const link = el('a', {
                    class: 'profile-membership-row',
                    href: profileMembershipTarget(membership)
                },
                el('span', {class: 'profile-membership-main'},
                    el('strong', {}, String(membership.name || '')),
                    el('span', {}, membership.description || membership.repository)
                ),
                el('span', {class: 'profile-membership-meta'},
                    el('span', {class: 'profile-membership-repository'}, membership.repository || ''),
                    el('span', {class: 'profile-membership-role'},
                        profileMembershipRole(format, membership.permission_level)),
                    createIcon('chevron')
                ));
                link.addEventListener('click', openProfilePackage);
                list.appendChild(link);
            });
        }, {duration: 280});
    } catch (error) {
        if (sequence !== profilePageLoadSeq) return;
        console.error('Failed to load profile memberships', error);
        await morphElementHeight(list, () => {
            list.replaceChildren(el('div', {class: 'profile-memberships-empty is-error'},
                t('profile.membershipsLoadFailed')));
        }, {duration: 240});
    }
}

/**
 * Render the default public-facing profile page.
 * @param {object} profile - Public profile payload.
 * @returns {void}
 */
function renderPublicProfile(profile) {
    const publicView = document.getElementById('profile-public-view');
    const editView = document.getElementById('profile-edit-view');
    if (!publicView || !editView) return;
    editView.hidden = true;
    publicView.hidden = false;
    const displayName = profileDisplayName(profile);
    const actions = el('div', {class: 'profile-public-actions'});
    if (profile.own_profile) {
        actions.appendChild(el('button', {
            type: 'button',
            class: 'pill-btn pill-btn--primary',
            onclick: () => navigateToUserProfile(profile.username, 'edit')
        }, createIcon('edit'), el('span', {}, t('common.edit'))));
    }
    publicView.replaceChildren(
        el('button', {
            type: 'button', class: 'profile-route-back', onclick: leaveUserProfileRoute
        }, createIcon('chevronLeft'), el('span', {}, t('profile.back'))),
        el('article', {class: 'profile-public-card'},
            el('div', {class: 'profile-public-banner', 'aria-hidden': 'true'}),
            el('div', {class: 'profile-public-content'},
                el('div', {class: 'profile-public-avatar', 'aria-hidden': 'true'}, profileAvatarLetter(profile)),
                el('div', {class: 'profile-public-heading'},
                    el('div', {class: 'profile-public-name-row'},
                        el('h2', {class: 'profile-public-name', title: displayName}, displayName),
                        actions
                    ),
                    el('p', {class: 'profile-public-username'}, `@${profile.username}`),
                    el('p', {class: 'profile-public-description'}, t('profile.publicDescription'))
                ),
                el('dl', {class: 'profile-public-meta'},
                    el('div', {class: 'profile-public-meta-item'},
                        el('dt', {}, t('profile.usernameLabel')),
                        el('dd', {}, profile.username)
                    ),
                    el('div', {class: 'profile-public-meta-item'},
                        el('dt', {}, t('profile.memberSince')),
                        el('dd', {}, formatProfileCreatedAt(profile.created_at))
                    ),
                    el('div', {class: 'profile-public-meta-item'},
                        el('dt', {}, t('profile.mavenDomains')),
                        el('dd', {}, el('a', {
                            class: 'profile-public-meta-link',
                            href: `/user/${encodeURIComponent(profile.username)}/maven`,
                            onclick: event => {
                                event.preventDefault();
                                navigateToUserProfile(profile.username, 'maven');
                            }
                        }, el('span', {}, String(profile.maven_domain_count || 0)), createIcon('chevron')))
                    ),
                    el('div', {class: 'profile-public-meta-item'},
                        el('dt', {}, t('profile.cargoPackages')),
                        el('dd', {}, el('a', {
                            class: 'profile-public-meta-link',
                            href: `/user/${encodeURIComponent(profile.username)}/cargo`,
                            onclick: event => {
                                event.preventDefault();
                                navigateToUserProfile(profile.username, 'cargo');
                            }
                        }, el('span', {}, String(profile.cargo_package_count || 0)), createIcon('chevron')))
                    ),
                    el('div', {class: 'profile-public-meta-item'},
                        el('dt', {}, t('profile.dockerImages')),
                        el('dd', {}, el('a', {
                            class: 'profile-public-meta-link',
                            href: `/user/${encodeURIComponent(profile.username)}/docker`,
                            onclick: event => {
                                event.preventDefault();
                                navigateToUserProfile(profile.username, 'docker');
                            }
                        }, el('span', {}, String(profile.docker_image_count || 0)), createIcon('chevron')))
                    ),
                    el('div', {class: 'profile-public-meta-item'},
                        el('dt', {}, t('profile.npmPackages')),
                        el('dd', {}, el('a', {
                            class: 'profile-public-meta-link',
                            href: `/user/${encodeURIComponent(profile.username)}/npm`,
                            onclick: event => {
                                event.preventDefault();
                                navigateToUserProfile(profile.username, 'npm');
                            }
                        }, el('span', {}, String(profile.npm_package_count || 0)), createIcon('chevron')))
                    )
                )
            )
        )
    );
}

/**
 * Reset a profile disclosure without animation.
 * @param {HTMLDetailsElement} card - Collapsible settings section.
 * @returns {void}
 */
function resetProfileDisclosure(card) {
    const content = card.querySelector('.profile-collapsible-content');
    const summary = card.querySelector('.profile-collapsible-summary');
    card.open = false;
    card.dataset.disclosureAnimating = 'false';
    if (summary) summary.setAttribute('aria-expanded', 'false');
    if (content) {
        content.hidden = true;
        content.classList.remove('is-visible');
        content.style.display = 'none';
        content.style.height = '';
        content.style.overflow = '';
        content.style.opacity = '';
        content.style.transition = '';
    }
}

/**
 * Add an animated, accessible toggle to one profile settings section.
 * @param {HTMLDetailsElement} card - Collapsible settings section.
 * @returns {void}
 */
function wireProfileDisclosure(card) {
    if (card.dataset.disclosureWired === 'true') return;
    const summary = card.querySelector('.profile-collapsible-summary');
    const content = card.querySelector('.profile-collapsible-content');
    if (!summary || !content) return;
    card.dataset.disclosureWired = 'true';
    summary.setAttribute('aria-expanded', 'false');
    summary.addEventListener('click', async event => {
        event.preventDefault();
        if (card.dataset.disclosureAnimating === 'true') return;
        card.dataset.disclosureAnimating = 'true';
        if (card.open) {
            summary.setAttribute('aria-expanded', 'false');
            await collapseElement(content, {duration: 240, marginTop: false});
            card.open = false;
        } else {
            card.open = true;
            summary.setAttribute('aria-expanded', 'true');
            await expandElement(content, {duration: 280});
        }
        card.dataset.disclosureAnimating = 'false';
    });
}

/**
 * Build and insert the identity editor above the security settings.
 * @param {object} profile - Own profile payload.
 * @returns {HTMLDetailsElement|null} Identity section.
 */
function buildProfileIdentityEditor(profile) {
    const settingsCard = document.querySelector('#profile-edit-view .profile-settings-card');
    if (!settingsCard) return null;
    settingsCard.querySelector('.profile-identity-card')?.remove();
    const nicknameInput = el('input', {
        id: 'profile-nickname', class: 'profile-input', type: 'text',
        autocomplete: 'nickname', value: profile.nickname || '',
        placeholder: t('profile.nicknamePlaceholder')
    });
    const nicknameCounter = el('span', {class: 'profile-character-count'});
    const updateCounter = () => {
        const count = Array.from(nicknameInput.value).length;
        nicknameCounter.textContent = t('profile.nicknameCount', {count});
        nicknameInput.setCustomValidity(count > 36 ? t('profile.nicknameHint') : '');
    };
    nicknameInput.addEventListener('input', updateCounter);
    updateCounter();
    const usernameInput = el('input', {
        id: 'profile-username', class: 'profile-input', type: 'text',
        autocomplete: 'username', value: profile.username,
        'aria-describedby': 'profile-username-hint'
    });
    usernameInput.addEventListener('input', () => usernameInput.setCustomValidity(''));
    const rateHint = el('p', {class: 'profile-rate-hint'}, profileRenameHint(profile));
    const saveButton = el('button', {
        type: 'submit', class: 'pill-btn pill-btn--primary'
    }, t('users.saveBtn'));
    const form = el('form', {class: 'profile-identity-form', action: 'javascript:void(0);'},
        el('div', {class: 'profile-field'},
            el('div', {class: 'profile-field-label-row'},
                el('label', {for: 'profile-nickname'}, t('profile.nicknameLabel')),
                nicknameCounter
            ),
            nicknameInput,
            el('p', {class: 'profile-field-hint'}, t('profile.nicknameHint'))
        ),
        el('div', {class: 'profile-field'},
            el('label', {for: 'profile-username'}, t('profile.usernameLabel')),
            usernameInput,
            el('p', {id: 'profile-username-hint', class: 'profile-field-hint'}, t('profile.usernameHint')),
            rateHint
        ),
        el('div', {class: 'profile-identity-actions'}, saveButton)
    );
    form.addEventListener('submit', async event => {
        event.preventDefault();
        const requestedUsername = usernameInput.value.trim().toLowerCase();
        const requestedNickname = nicknameInput.value.trim();
        if (requestedUsername !== profile.username && !/^[A-Za-z0-9_]{4,18}$/.test(requestedUsername)) {
            usernameInput.setCustomValidity(t('profile.usernameHint'));
        }
        if (!form.reportValidity()) return;
        if (requestedUsername !== profile.username) {
            const confirmed = await window.showConfirm(t('profile.renameConfirm', {name: requestedUsername}), {
                title: t('profile.renameTitle'), confirmText: t('profile.renameAction')
            });
            if (!confirmed) return;
        }
        saveButton.disabled = true;
        try {
            const response = await apiRequest('/api/auth/profile', {
                method: 'PUT',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({username: requestedUsername, nickname: requestedNickname})
            });
            if (!response.ok) {
                const message = response.status === 409
                    ? t('profile.usernameExists')
                    : (response.status === 429
                        ? t('profile.renameRateLimited')
                        : (response.status === 400
                            ? t('profile.identityInvalid')
                            : await responseErrorMessage(response, 'profile.updateFailed')));
                throw new LocalizedResponseError(message, response.status);
            }
            const updated = await response.json();
            const oldUsername = profile.username;
            profile = {...profile, ...updated};
            localStorage.setItem('username', updated.username);
            invalidateUserProfiles(oldUsername, updated.username);
            window.dispatchEvent(new CustomEvent('profileUpdated', {
                detail: {...updated, old_username: oldUsername}
            }));
            showAlert(t('profile.updated'), 'success');
            if (oldUsername !== updated.username) {
                const route = profileRouteFromPath(window.location.pathname);
                const suffix = route?.section === 'edit' ? '/edit' : '';
                const path = `/user/${encodeURIComponent(updated.username)}${suffix}`;
                window.history.replaceState(window.history.state, '', path);
            }
            nicknameInput.value = updated.nickname || '';
            usernameInput.value = updated.username;
            rateHint.textContent = profileRenameHint(updated);
            updateCounter();
            updateProfileEditHeading(updated);
            renderGitHubConnection(updated.github);
        } catch (error) {
            console.error('Failed to update profile identity', error);
            showAlert(caughtErrorMessage(error, 'profile.updateFailed'), 'error');
        } finally {
            saveButton.disabled = false;
        }
    });
    const card = el('details', {class: 'profile-settings-section profile-identity-card profile-collapsible-card'},
        el('summary', {class: 'profile-section-card-header profile-collapsible-summary'},
            el('div', {class: 'profile-section-icon'}, createIcon('user')),
            el('div', {class: 'profile-section-meta'},
                el('h3', {class: 'profile-section-title'}, t('profile.identityTitle')),
                el('p', {class: 'profile-section-desc'}, t('profile.identityDescription'))
            ),
            createIcon('chevronDown', {class: 'profile-collapse-chevron'})
        ),
        el('div', {class: 'profile-collapsible-content', hidden: true},
            el('div', {class: 'profile-section-body'}, form)
        )
    );
    settingsCard.prepend(card);
    return card;
}

/**
 * Switch an own profile from its public view to editing controls.
 * @param {object} profile - Own profile payload.
 * @returns {void}
 */
function showProfileEdit(profile) {
    const publicView = document.getElementById('profile-public-view');
    const editView = document.getElementById('profile-edit-view');
    if (!publicView || !editView) return;
    publicView.hidden = true;
    editView.hidden = false;
    updateProfileEditHeading(profile);
    buildProfileIdentityEditor(profile);
    renderProfileSuperTeamLimits(profile.super_team_limits);
    editView.querySelectorAll('details.profile-collapsible-card').forEach(card => {
        resetProfileDisclosure(card);
        wireProfileDisclosure(card);
    });
    wireProfileEditActions(profile);
    renderGitHubConnection(profile.github);
    void refreshAccountSecurity();
    void refreshAPITokenSummary();
    window.scrollTo({top: 0, behavior: 'smooth'});
}

/**
 * Load and render a username-based profile route.
 * @param {{username: string, section: ''|'edit'|'maven'|'cargo'|'docker'|'npm'}|null} [route=null] - Parsed profile route.
 * @returns {Promise<void>}
 */
export async function setupProfile(route = null) {
    const targetRoute = route || profileRouteFromPath(window.location.pathname);
    const targetUsername = String(targetRoute?.username || '').trim().toLowerCase();
    const publicView = document.getElementById('profile-public-view');
    const editView = document.getElementById('profile-edit-view');
    if (!publicView || !editView) return;
    editView.hidden = true;
    publicView.hidden = false;
    const sequence = ++profilePageLoadSeq;
    publicView.replaceChildren(el('div', {class: 'profile-route-loading'},
        el('div', {class: 'sessions-loading-spinner', 'aria-hidden': 'true'}),
        el('span', {}, t('common.loading'))
    ));
    if (!targetUsername) {
        publicView.replaceChildren(el('div', {class: 'profile-route-error'}, t('profile.notFound')));
        return;
    }
    try {
        const profile = await getUserProfile(targetUsername, {refresh: true});
        if (sequence !== profilePageLoadSeq) return;
        if (targetRoute?.section === 'maven' || targetRoute?.section === 'cargo' ||
            targetRoute?.section === 'docker' || targetRoute?.section === 'npm') {
            await renderProfileMemberships(profile, targetRoute.section, sequence);
        } else if (targetRoute?.section === 'edit' && profile.own_profile) {
            showProfileEdit(profile);
        } else {
            if (targetRoute?.section === 'edit') {
                window.history.replaceState(window.history.state, '', `/user/${encodeURIComponent(profile.username)}`);
            }
            renderPublicProfile(profile);
        }
    } catch (error) {
        if (sequence !== profilePageLoadSeq) return;
        publicView.replaceChildren(
            el('button', {type: 'button', class: 'profile-route-back', onclick: leaveUserProfileRoute},
                createIcon('chevronLeft'), el('span', {}, t('profile.back'))),
            el('div', {class: 'profile-route-error'},
                createIcon('alertCircle'),
                el('h2', {}, error.status === 404 ? t('profile.notFound') : t('profile.loadFailed'))
            )
        );
    }
}

/**
 * Wire up the profile page: password changes, API-token summaries, sessions, and FIDO devices.
 * @param {object} profile - Own profile payload.
 * @returns {void}
 */
function wireProfileEditActions(profile) {
    const username = profile?.username || localStorage.getItem('username') || '';
    const avatarEl = document.getElementById('profile-avatar-initials');
    const displayNameEl = document.getElementById('profile-display-name');
    const usernameHiddenEl = document.getElementById('profile-username-hidden');
    if (usernameHiddenEl) {
        usernameHiddenEl.value = username;
    }
    if (avatarEl) {
        avatarEl.textContent = profileAvatarLetter(profile);
    }
    if (displayNameEl) {
        displayNameEl.textContent = profileDisplayName(profile);
    }

    const passwordInput = document.getElementById('profile-new-password');
    const strengthCtrl = passwordInput ? attachPasswordStrength(passwordInput) : null;

    const btnUpdatePassword = document.getElementById('btn-update-password');

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
                    void refreshAccountSecurity();
                } else {
                    showAlert(await responseErrorMessage(response, 'profile.updatePasswordFailed'), 'error');
                }
            } catch (error) {
                showAlert(t('profile.updatePasswordError'), 'error');
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
