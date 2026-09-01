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
import {apiRequest} from '../api.js';
import {responseErrorMessage} from '../response-errors.js';
import {createIcon, RenopDialog, runButtonAction} from '../components.js';
import {el} from '@renop/ui/dom';
import {formatTimestamp} from '../time.js';

const maxBanReasonLength = 512;

function localDateTimeValue(timestamp) {
    const date = new Date(Number(timestamp));
    if (!Number.isFinite(date.getTime())) return '';
    return new Date(date.getTime() - date.getTimezoneOffset() * 60000).toISOString().slice(0, 16);
}

/**
 * Open the administrator suspension editor for one account.
 * @param {object} account - Account list record.
 * @param {() => void|Promise<void>} refresh - Users-list refresh callback.
 * @returns {void}
 */
export function openUserBanDialog(account, refresh) {
    const currentBan = account?.ban || null;
    const reason = el('input', {
        id: 'user-ban-reason', type: 'text', maxlength: maxBanReasonLength,
        value: currentBan?.reason || '', class: 'user-ban-input', autocomplete: 'off',
        placeholder: t('users.banReasonPlaceholder'),
    });
    const permanent = el('input', {
        id: 'user-ban-permanent', type: 'checkbox', checked: !currentBan?.expires_at,
    });
    const minimumExpiry = Date.now() + 60000;
    const expiresAt = el('input', {
        id: 'user-ban-expires-at', type: 'datetime-local', class: 'user-ban-input',
        min: localDateTimeValue(minimumExpiry),
        value: localDateTimeValue(currentBan?.expires_at || Date.now() + 24 * 60 * 60 * 1000),
    });
    const expiryGroup = el('label', {class: 'user-ban-field', htmlFor: expiresAt.id},
        el('span', {}, t('users.banUntilLabel')), expiresAt);
    const syncExpiry = () => {
        expiryGroup.hidden = permanent.checked;
        expiresAt.disabled = permanent.checked;
    };
    permanent.addEventListener('change', syncExpiry);
    syncExpiry();

    const body = el('div', {class: 'user-ban-body'},
        ...(currentBan ? [el('div', {class: 'user-ban-current'},
            createIcon('warning', {width: '18', height: '18'}),
            el('div', {},
                el('strong', {}, t(currentBan.expires_at
                    ? 'users.banCurrentUntil'
                    : 'users.banCurrentPermanent', {
                    date: formatTimestamp(currentBan.expires_at, {fallback: t('common.unknown')})
                })),
                el('p', {}, t('users.banCurrentReason', {reason: currentBan.reason || t('common.unknown')}))
            ))] : []),
        el('label', {class: 'user-ban-field', htmlFor: reason.id},
            el('span', {}, t('users.banReasonLabel')), reason),
        el('label', {class: 'user-ban-permanent', htmlFor: permanent.id},
            permanent, el('span', {}, t('users.banPermanent'))),
        expiryGroup
    );

    const submit = async (event, dialog) => {
        event.preventDefault();
        const normalizedReason = reason.value.trim();
        if (!normalizedReason) {
            showAlert(t('users.banReasonRequired'), 'error');
            reason.focus();
            return;
        }
        let expiry = null;
        if (!permanent.checked) {
            expiry = Date.parse(expiresAt.value);
            if (!Number.isFinite(expiry)) {
                showAlert(t('users.banExpiryRequired'), 'error');
                expiresAt.focus();
                return;
            }
            if (expiry <= Date.now()) {
                showAlert(t('users.banExpiryFuture'), 'error');
                expiresAt.focus();
                return;
            }
        }
        const button = document.getElementById('user-ban-save');
        await runButtonAction(button, async () => {
            const response = await apiRequest(`/api/tokens/${encodeURIComponent(account.name)}/ban`, {
                method: 'PUT', headers: {'Content-Type': 'application/json'},
                body: JSON.stringify({reason: normalizedReason, expires_at: expiry}),
            });
            if (!response.ok) {
                showAlert(await responseErrorMessage(response, 'users.banFailed'), 'error');
                return;
            }
            dialog.close(true);
            showAlert(t('users.banSaved'), 'success');
            await refresh?.();
        });
    };

    const footer = [
        {text: t('common.cancel'), className: 'action-btn', onClick: (event, dialog) => dialog.close(false)},
    ];
    if (currentBan) {
        footer.push({
            text: t('users.unban'), className: 'action-btn danger-btn',
            onClick: async (event, dialog) => {
                await runButtonAction(event.currentTarget, async () => {
                    const response = await apiRequest(`/api/tokens/${encodeURIComponent(account.name)}/ban`, {
                        method: 'DELETE'
                    });
                    if (!response.ok) {
                        showAlert(await responseErrorMessage(response, 'users.banFailed'), 'error');
                        return;
                    }
                    dialog.close(true);
                    showAlert(t('users.unbanned'), 'success');
                    await refresh?.();
                });
            },
        });
    }
    footer.push({text: t('common.save'), className: 'action-btn primary-btn', type: 'submit', id: 'user-ban-save'});

    void RenopDialog.show({
        id: 'user-ban-modal', maxWidth: '560px', icon: 'warning',
        title: t('users.banDialogTitle', {user: account.name}),
        subtitle: t('users.manageBanDesc'),
        form: {id: 'user-ban-form', className: 'user-ban-form', onSubmit: submit},
        bodyClass: 'modal-body', body, footer,
    });
    requestAnimationFrame(() => reason.focus());
}
