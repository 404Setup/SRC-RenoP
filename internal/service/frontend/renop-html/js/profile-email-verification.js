/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {apiRequest} from './api.js';
import {RenopDialog, runButtonAction} from './components.js';
import {responseErrorMessage} from './response-errors.js';
import {mailStatusLabel} from './mail-status.js';
import {t} from './i18n.js';
import {el} from '@renop/ui/dom';

/** Confirm a queued security-email change while keeping its code and status ticket inside the dialog. */
export async function verifyProfileEmail(email, receipt) {
    const controller = new AbortController(), route = window.location.pathname;
    const deadline = Date.now() + 10 * 60 * 1000;
    let timer;
    const code = el('input', {id: 'profile-email-code', type: 'text', inputmode: 'numeric',
        autocomplete: 'one-time-code', pattern: '[0-9]{8}', minlength: '8', maxlength: '8', required: true});
    const error = el('p', {class: 'account-form-error', role: 'alert', hidden: true});
    const status = el('p', {class: 'profile-security-hint', role: 'status'}, mailStatusLabel(receipt.status));
    const refresh = el('button', {class: 'action-btn', type: 'button', hidden: true}, t('mail.refreshStatus'));
    const form = el('form', {class: 'account-verification'}, el('p', {}, t('profile.emailVerificationSent', {email})),
        el('div', {class: 'account-field'}, el('label', {for: 'profile-email-code'}, t('login.emailCode')), code),
        status, refresh, error);
    const close = () => document.getElementById('profile-email-verification-dialog')?.close(null);
    const active = () => !controller.signal.aborted && window.location.pathname === route;
    const poll = async () => {
        clearTimeout(timer);
        if (!active()) { close(); return; }
        refresh.hidden = true;
        try {
            const response = await apiRequest('/api/auth/mail/' + encodeURIComponent(receipt.id), {
                signal: controller.signal, headers: {'X-Renop-Mail-Ticket': receipt.ticket},
            });
            if (!response.ok) throw new Error('Email status unavailable');
            const job = await response.json();
            if (!active()) return;
            status.textContent = mailStatusLabel(job.status);
            if (['queued', 'paused', 'sending', 'checking', 'queued_provider'].includes(job.status) && Date.now() < deadline) {
                timer = setTimeout(() => void poll(), 3000);
            }
        } catch {
            if (!active()) return;
            status.textContent = t('mail.requestFailed');
            refresh.hidden = false;
        }
    };
    refresh.addEventListener('click', () => void runButtonAction(refresh, poll));
    form.addEventListener('submit', event => {
        event.preventDefault();
        if (!active()) return;
        void runButtonAction(document.getElementById('profile-email-verify'), async () => {
            error.hidden = true;
            try {
                const response = await apiRequest('/api/auth/profile/email/confirm', {
                    method: 'POST', signal: controller.signal, headers: {'Content-Type': 'application/json'},
                    body: JSON.stringify({email, code: code.value}),
                });
                code.value = '';
                if (!active()) return;
                if (!response.ok) {
                    error.textContent = await responseErrorMessage(response, 'profile.privateEmailSaveFailed');
                    error.hidden = false;
                    code.focus();
                    return;
                }
                const security = await response.json();
                if (active()) document.getElementById('profile-email-verification-dialog')?.close(security);
            } catch {
                if (!active()) return;
                error.textContent = t('profile.privateEmailSaveFailed');
                error.hidden = false;
            }
        });
    });
    window.addEventListener('pagehide', close);
    window.addEventListener('popstate', close);
    timer = setTimeout(() => void poll(), 3000);
    return RenopDialog.show({
        id: 'profile-email-verification-dialog', title: t('profile.verifyPrivateEmail'),
        icon: 'send', maxWidth: '480px', body: form,
        footer: [
            {text: t('common.cancel'), className: 'action-btn', onClick: close},
            {id: 'profile-email-verify', text: t('mfa.verify'), className: 'action-btn primary-btn', onClick: () => form.requestSubmit()},
        ],
        onClose: () => {
            controller.abort(); clearTimeout(timer); code.value = ''; receipt.ticket = '';
            window.removeEventListener('pagehide', close);
            window.removeEventListener('popstate', close);
        },
    });
}
