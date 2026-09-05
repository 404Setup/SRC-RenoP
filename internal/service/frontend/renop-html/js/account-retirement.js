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
import {RenopDialog, runButtonAction} from './components.js';
import {logout} from './auth.js';
import {t} from './i18n.js';
import {responseErrorMessage} from './response-errors.js';
import {el} from '@renop/ui/dom';

/** Convert the server's ownership checks into localized closure requirements. */
function retirementBlockers(plan) {
    const blockers = [];
    if (plan?.protected_role) blockers.push(t('profile.retireBlockedRole'));
    for (const [field, key] of [
        ['super_team_owner_count', 'profile.retireBlockedSuperTeams'],
        ['maven_domain_owner_count', 'profile.retireBlockedDomains'],
        ['package_owner_count', 'profile.retireBlockedPackages'],
        ['pending_review_count', 'profile.retireBlockedReviews'],
    ]) {
        const count = Number(plan?.[field]) || 0;
        if (count > 0) blockers.push(t(key, {count}));
    }
    return blockers;
}

/** Show all requirements that must be resolved before closing the account. */
function showBlockedRetirement(plan) {
    const blockers = retirementBlockers(plan);
    void RenopDialog.show({
        id: 'account-retirement-blocked-dialog', maxWidth: '620px', icon: 'warning',
        title: t('profile.retireBlockedTitle'), subtitle: t('profile.retireBlockedHint'),
        body: el('ul', {class: 'profile-retirement-blockers'},
            ...blockers.map(message => el('li', {}, message))),
        footer: [{
            text: t('common.close'), className: 'action-btn primary-btn',
            onClick: (event, dialog) => dialog.close(false)
        }]
    });
}

/** Require the exact username before requesting irreversible account closure. */
function showRetirementConfirmation(plan) {
    const username = String(plan?.username || localStorage.getItem('username') || '').trim();
    const input = el('input', {
        class: 'profile-input', type: 'text', autocomplete: 'off', maxlength: '255',
        placeholder: username, 'aria-describedby': 'account-retirement-confirmation-hint'
    });
    const error = el('p', {class: 'profile-inline-error', role: 'alert'});
    const body = el('div', {class: 'profile-retirement-dialog'},
        el('p', {}, t('profile.retireAccountWarning')),
        el('p', {}, t('profile.retireRetentionNotice')),
        el('label', {}, el('span', {}, t('profile.retireTypeUsername', {username})), input),
        el('p', {class: 'profile-security-hint', id: 'account-retirement-confirmation-hint'},
            t('profile.retireIrreversible')),
        error
    );
    void RenopDialog.show({
        id: 'account-retirement-confirm-dialog', maxWidth: '640px', icon: 'warning',
        title: t('profile.retireAccountTitle'), subtitle: t('profile.retireAccountSubtitle'),
        form: {
            id: 'account-retirement-form',
            onSubmit: async (event, dialog) => {
                event.preventDefault();
                if (input.value.trim() !== username) {
                    error.textContent = t('profile.retireConfirmationMismatch');
                    input.focus();
                    return;
                }
                const submit = event.submitter || document.querySelector(
                    '#account-retirement-confirm-dialog button[type="submit"]');
                if (!submit) return;
                await runButtonAction(submit, async () => {
                    const response = await apiRequest('/api/auth/profile/retirement', {
                        method: 'DELETE', headers: {'Content-Type': 'application/json'},
                        body: JSON.stringify({confirmation: input.value.trim()})
                    });
                    if (!response.ok) {
                        if (response.headers.get('X-Renop-Error-Code') === 'ACCOUNT_RETIREMENT_BLOCKED') {
                            dialog.close(false);
                            showBlockedRetirement(await response.json());
                            return;
                        }
                        showAlert(await responseErrorMessage(response, 'profile.retireFailed'), 'error');
                        return;
                    }
                    dialog.close(true);
                    showAlert(t('profile.retireSuccess'), 'success');
                    await logout('account_deleted');
                }).catch(() => showAlert(t('profile.retireFailed'), 'error'));
            }
        },
        body,
        footer: [
            {text: t('common.cancel'), className: 'action-btn', onClick: (event, dialog) => dialog.close(false)},
            {text: t('profile.retireAccount'), className: 'action-btn danger-btn', type: 'submit'}
        ]
    });
    requestAnimationFrame(() => input.focus());
}

document.getElementById('btn-profile-retirement')?.addEventListener('click', async event => {
    await runButtonAction(event.currentTarget, async () => {
        const response = await apiRequest('/api/auth/profile/retirement');
        if (!response.ok) {
            showAlert(await responseErrorMessage(response, 'profile.retirePlanFailed'), 'error');
            return;
        }
        const plan = await response.json();
        if (!plan?.eligible) showBlockedRetirement(plan);
        else showRetirementConfirmation(plan);
    }).catch(() => showAlert(t('profile.retirePlanFailed'), 'error'));
});
