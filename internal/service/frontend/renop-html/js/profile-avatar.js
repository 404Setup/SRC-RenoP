/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {el} from '@renop/ui/dom';
import {apiRequest} from './api.js';
import {showAlert} from './alert.js';
import {createIcon, runButtonAction} from './components.js';
import {t} from './i18n.js';
import {LocalizedResponseError} from './response-errors.js';
import {formatBytes} from './browser/utils.js';
import {renderProfileAvatar, syncUserProfile} from './user-profiles.js';

const avatarErrorKeys = Object.freeze({
    avatar_too_large: 'profile.avatarTooLarge',
    avatar_invalid_type: 'profile.avatarInvalidType',
    avatar_dimensions: 'profile.avatarDimensions',
    avatar_unsafe: 'profile.avatarUnsafe',
    publication_file_quota: 'publicationQuota.fileExceeded',
    publication_byte_quota: 'publicationQuota.byteExceeded',
    publication_quota_unavailable: 'profile.avatarSaveFailed',
    github_not_linked: 'profile.avatarGitHubNotLinked',
    avatar_download_failed: 'profile.avatarSyncFailed',
    avatar_storage_failed: 'profile.avatarSaveFailed',
});

/**
 * Convert one avatar API failure into registered localized copy.
 * @param {Response} response - Failed avatar response.
 * @param {string} fallbackKey - Fallback translation key.
 * @returns {LocalizedResponseError} Safe localized failure.
 */
function avatarResponseError(response, fallbackKey) {
    const code = String(response.headers.get('X-Renop-Error-Code') || '').toLowerCase();
    return new LocalizedResponseError(t(avatarErrorKeys[code] || fallbackKey), response.status);
}

/**
 * Build the own-profile avatar upload and GitHub synchronization controls.
 * @param {object} initialProfile - Own profile response.
 * @param {{onUpdated?: (profile: object) => void}} [options] - Update observer.
 * @returns {HTMLElement} Avatar editor.
 */
export function createProfileAvatarEditor(initialProfile, {onUpdated} = {}) {
    let profile = initialProfile;
    const maxBytes = Math.max(1, Number(profile.avatar_max_size_bytes) || 1048576);
    const preview = el('div', {class: 'profile-avatar-editor-preview', 'aria-hidden': 'true'});
    const input = el('input', {
        type: 'file', accept: 'image/png,image/jpeg,image/webp', hidden: true,
    });
    const uploadButton = el('button', {type: 'button', class: 'pill-btn pill-btn--primary'},
        createIcon('upload'), el('span', {}, t('profile.avatarUpload')));
    const syncButton = el('button', {
        type: 'button', class: 'pill-btn pill-btn--soft', hidden: profile.github?.linked !== true,
    }, createIcon('refresh'), el('span', {}, t('profile.avatarSyncGitHub')));
    const removeButton = el('button', {
        type: 'button', class: 'pill-btn pill-btn--ghost-danger', hidden: !profile.avatar_url,
    }, createIcon('delete'), el('span', {}, t('profile.avatarRemove')));

    /** Apply one authoritative profile response everywhere. */
    const applyUpdated = updated => {
        profile = {...profile, ...updated};
        renderProfileAvatar(preview, profile, {length: 1});
        removeButton.hidden = !profile.avatar_url;
        syncButton.hidden = profile.github?.linked !== true;
        syncUserProfile(profile);
        if (typeof onUpdated === 'function') onUpdated(profile);
    };
    renderProfileAvatar(preview, profile, {length: 1});

    uploadButton.addEventListener('click', () => input.click());
    input.addEventListener('change', async () => {
        const file = input.files?.[0];
        input.value = '';
        if (!file) return;
        if (file.size > maxBytes) {
            showAlert(t('profile.avatarTooLarge'), 'error');
            return;
        }
        uploadButton.disabled = true;
        try {
            const response = await apiRequest('/api/auth/profile/avatar', {
                method: 'PUT', headers: {'Content-Type': file.type}, body: file,
            });
            if (!response.ok) throw avatarResponseError(response, 'profile.avatarSaveFailed');
            applyUpdated(await response.json());
            showAlert(t('profile.avatarUpdated'), 'success');
        } catch (error) {
            showAlert(error instanceof LocalizedResponseError ? error.message : t('profile.avatarSaveFailed'), 'error');
        } finally {
            uploadButton.disabled = false;
        }
    });

    syncButton.addEventListener('click', event => void runButtonAction(event.currentTarget, async () => {
        const response = await apiRequest('/api/auth/profile/avatar/github', {method: 'POST'});
        if (!response.ok) throw avatarResponseError(response, 'profile.avatarSyncFailed');
        applyUpdated(await response.json());
        showAlert(t('profile.avatarUpdated'), 'success');
    }).catch(error => {
        showAlert(error instanceof LocalizedResponseError ? error.message : t('profile.avatarSyncFailed'), 'error');
    }));

    removeButton.addEventListener('click', event => void runButtonAction(event.currentTarget, async () => {
        const response = await apiRequest('/api/auth/profile/avatar', {method: 'DELETE'});
        if (!response.ok) throw avatarResponseError(response, 'profile.avatarRemoveFailed');
        applyUpdated(await response.json());
        showAlert(t('profile.avatarRemoved'), 'success');
    }).catch(error => {
        showAlert(error instanceof LocalizedResponseError ? error.message : t('profile.avatarRemoveFailed'), 'error');
    }));

    return el('div', {class: 'profile-avatar-editor'},
        preview,
        el('div', {class: 'profile-avatar-editor-copy'},
            el('strong', {}, t('profile.avatarTitle')),
            el('p', {}, t('profile.avatarRequirements', {size: formatBytes(maxBytes)})),
            el('div', {class: 'profile-avatar-editor-actions'}, uploadButton, syncButton, removeButton),
            input
        )
    );
}
