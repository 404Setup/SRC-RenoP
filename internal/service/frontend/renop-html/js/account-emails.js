/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {apiRequest} from './api.js';
import {showAlert} from './alert.js';
import {runButtonAction} from './components.js';
import {responseErrorMessage} from './response-errors.js';
import {verifyProfileEmail} from './profile-email-verification.js';
import {t} from './i18n.js';
import {el} from '@renop/ui/dom';

/** Render private email aliases and reuse the existing proof dialog for additions. */
export function renderAccountEmailAliases(security, onUpdated, onFailure) {
    const list = document.getElementById('profile-email-aliases');
    const form = document.getElementById('profile-email-alias-form');
    const input = document.getElementById('profile-email-alias');
    if (!list || !form || !input) return;
    const emails = security.email_aliases || [];
    list.replaceChildren(...emails.map(email => {
        const remove = el('button', {type: 'button', class: 'pill-btn pill-btn--soft',
            'aria-label': t('profile.removeEmailAlias', {email})}, t('common.remove'));
        remove.addEventListener('click', () => void runButtonAction(remove, async () => {
            const route = window.location.pathname;
            try {
                const response = await apiRequest('/api/auth/profile/email/alias', {
                    method: 'DELETE', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({email}),
                });
                if (window.location.pathname !== route) return;
                if (!response.ok) { await onFailure(response); return; }
                const next = await response.json();
                if (window.location.pathname === route) onUpdated(next);
            } catch { showAlert(t('profile.privateEmailSaveFailed'), 'error'); }
        }));
        return el('li', {}, el('span', {}, email), remove);
    }));
    if (emails.length === 0) list.replaceChildren(el('li', {class: 'profile-security-hint'}, t('common.none')));
    const unavailable = !security.email_verification_required;
    input.disabled = unavailable;
    form.querySelector('button[type="submit"]').disabled = unavailable;
    document.getElementById('profile-email-alias-mail-disabled').hidden = !unavailable;
    if (form.dataset.bound) return;
    form.dataset.bound = 'true';
    form.addEventListener('submit', event => {
        event.preventDefault();
        if (!input.reportValidity()) return;
        void runButtonAction(form.querySelector('button[type="submit"]'), async () => {
            const email = input.value.trim(), route = window.location.pathname;
            try {
                const response = await apiRequest('/api/auth/profile/email', {
                    method: 'PUT', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({email, alias: true}),
                });
                if (window.location.pathname !== route) return;
                if (!response.ok) {
                    showAlert(await responseErrorMessage(response, 'profile.privateEmailSaveFailed'), 'error');
                    return;
                }
                const next = await verifyProfileEmail(email, await response.json());
                if (next && window.location.pathname === route) {
                    input.value = '';
                    onUpdated(next);
                    showAlert(t('profile.privateEmailSaved'), 'success');
                }
            } catch { showAlert(t('profile.privateEmailSaveFailed'), 'error'); }
        });
    });
}
