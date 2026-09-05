/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {apiRequest} from '../api.js';
import {showAlert, showConfirm} from '../alert.js';
import {RenopDialog, runButtonAction} from '../components.js';
import {t} from '../i18n.js';
import {responseErrorMessage} from '../response-errors.js';
import {formatTimestamp} from '../time.js';
import {el} from '@renop/ui/dom';

/** Render one labelled retention deadline or completion timestamp. */
function retentionFact(label, value) {
    return el('div', {class: 'profile-public-meta-item'}, el('dt', {}, label), el('dd', {}, value));
}

/** Open the administrator's retained-email and activity-log controls for a retired account. */
export async function openAccountRetentionDialog(token, onUpdated) {
    const username = String(token?.name || '').trim();
    if (!username) return;
    let status;
    try {
        const response = await apiRequest(`/api/tokens/${encodeURIComponent(username)}/retention`);
        if (!response.ok) {
            showAlert(await responseErrorMessage(response, 'users.retentionLoadFailed'), 'error');
            return;
        }
        status = await response.json();
    } catch {
        showAlert(t('users.retentionLoadFailed'), 'error');
        return;
    }
    const emailReleased = Number(status.email_released_at) > 0;
    const auditPurged = Number(status.audit_purged_at) > 0;
    const body = el('div', {class: 'users-retention-dialog'},
        el('p', {}, t('users.retentionHint')),
        el('dl', {class: 'profile-public-meta'},
            retentionFact(t('users.retiredAt'), formatTimestamp(status.deleted_at, {fallback: t('common.unknown')})),
            retentionFact(t('users.emailReleaseAt'), emailReleased
                ? t('users.retentionReleasedAt', {
                    date: formatTimestamp(status.email_released_at, {fallback: t('common.unknown')})
                })
                : formatTimestamp(status.email_release_at, {fallback: t('common.unknown')})),
            retentionFact(t('users.auditPurgeAt'), auditPurged
                ? t('users.retentionPurgedAt', {
                    date: formatTimestamp(status.audit_purged_at, {fallback: t('common.unknown')})
                })
                : formatTimestamp(status.audit_purge_at, {fallback: t('common.unknown')}))
        )
    );
    const actions = [];
    if (!emailReleased) actions.push({
        text: t('users.releaseEmailNow'), className: 'action-btn danger-btn',
        onClick: async (event, dialog) => {
            await runButtonAction(event.currentTarget, async () => {
                if (!(await showConfirm(t('users.releaseEmailConfirm', {username}), {danger: true}))) return;
                const result = await apiRequest(
                    `/api/tokens/${encodeURIComponent(username)}/retention/email`, {method: 'DELETE'});
                if (!result.ok) {
                    showAlert(await responseErrorMessage(result, 'users.releaseEmailFailed'), 'error');
                    return;
                }
                dialog.close(true);
                showAlert(t('users.releaseEmailSuccess'), 'success');
                await onUpdated?.();
            }).catch(() => showAlert(t('users.releaseEmailFailed'), 'error'));
        }
    });
    if (!auditPurged) actions.push({
        text: t('users.purgeAuditNow'), className: 'action-btn danger-btn',
        onClick: async (event, dialog) => {
            await runButtonAction(event.currentTarget, async () => {
                if (!(await showConfirm(t('users.purgeAuditConfirm', {username}), {danger: true}))) return;
                const result = await apiRequest(
                    `/api/tokens/${encodeURIComponent(username)}/retention/audit`, {method: 'DELETE'});
                if (!result.ok) {
                    showAlert(await responseErrorMessage(result, 'users.purgeAuditFailed'), 'error');
                    return;
                }
                dialog.close(true);
                showAlert(t('users.purgeAuditSuccess'), 'success');
                await onUpdated?.();
            }).catch(() => showAlert(t('users.purgeAuditFailed'), 'error'));
        }
    });
    actions.push({
        text: t('common.close'), className: 'action-btn primary-btn',
        onClick: (event, dialog) => dialog.close(false)
    });
    void RenopDialog.show({
        id: 'account-retention-dialog', maxWidth: '680px', icon: 'fileLock',
        title: t('users.retentionTitle', {username}), body,
        footer: actions
    });
}
