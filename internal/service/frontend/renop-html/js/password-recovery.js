/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {captchaFetch} from './captcha.js';
import {showAlert} from './alert.js';
import {logout} from './auth.js';
import {loginReturnTo, navigateToLogin} from './login-route.js';
import {runButtonAction} from './components/button.js';
import {t} from './i18n.js';
import {attachPasswordStrength, confirmWeakPasswordIfNeeded, getPasswordLengthError} from './password-strength.js';
import {LocalizedResponseError, responseErrorMessage} from './response-errors.js';
import {mailStatusLabel} from './mail-status.js';

const form = document.getElementById('password-reset-form');
const fields = document.getElementById('password-reset-fields');
const email = document.getElementById('password-reset-email');
const code = document.getElementById('password-reset-code');
const password = document.getElementById('password-reset-password');
const confirmation = document.getElementById('password-reset-confirmation');
const error = document.getElementById('password-reset-error');
const availability = document.getElementById('password-reset-availability');
const delivery = document.getElementById('password-reset-delivery');
const refresh = document.getElementById('password-reset-refresh');
let active = false, epoch = 0, availabilityEpoch = 0, receipt, timer, strength;

/** Read bounded auth responses without interpreting a rejected code as an expired session. */
async function requestJSON(path, options = {}) {
    const response = await captchaFetch('/api/auth/' + path, {
        credentials: 'include', cache: 'no-store', signal: AbortSignal.timeout(15000), ...options,
    });
    if (!response.ok) throw new LocalizedResponseError(await responseErrorMessage(response, 'login.recoveryFailed'), response.status);
    return response.json();
}

/** Apply the live mail switch to the public entry links and reset form. */
export async function refreshPasswordRecoveryAvailability() {
    const revision = ++availabilityEpoch;
    try {
        const result = await requestJSON('password-reset/status');
        if (revision !== availabilityEpoch) return;
        const wasDisabled = fields.disabled;
        fields.disabled = result.enabled !== true;
        if (active && wasDisabled && !fields.disabled) email.focus({preventScroll: true});
        availability.textContent = fields.disabled ? t('mail.disabled') : '';
        document.querySelectorAll('[data-password-reset-link]').forEach(link => {
            link.hidden = fields.disabled;
        });
    } catch {
        if (revision !== availabilityEpoch) return;
        fields.disabled = true;
        availability.textContent = t('login.recoveryFailed');
        document.querySelectorAll('[data-password-reset-link]').forEach(link => {
            link.hidden = true;
        });
    }
}

/** Discard the private status capability when its email or page changes. */
function clearDelivery() {
    clearTimeout(timer);
    receipt = null;
    delivery.textContent = '';
    refresh.hidden = true;
}

/** Focus on entry and clear passwords and verification state on departure. */
export function updatePasswordRecoveryPage(isActive, entering = false) {
    active = isActive;
    if (!active) {
        epoch++;
        clearDelivery();
        form.reset();
        strength?.reset();
        error.textContent = '';
        return;
    }
    strength = attachPasswordStrength(password);
    if (entering) email.focus({preventScroll: true});
}

/** Poll only this page's current receipt, for at most the code's ten-minute lifetime. */
async function updateDelivery() {
    clearTimeout(timer);
    const current = receipt;
    if (!active || !current) return;
    refresh.hidden = true;
    try {
        const job = await requestJSON('mail/' + encodeURIComponent(current.id), {headers: {'X-Renop-Mail-Ticket': current.ticket}});
        if (!active || receipt !== current) return;
        delivery.textContent = mailStatusLabel(job.status);
        if (['queued', 'paused', 'sending', 'checking', 'queued_provider'].includes(job.status) && Date.now() < current.deadline) {
            timer = setTimeout(() => {
                void updateDelivery();
            }, 3000);
        }
    } catch {
        if (!active || receipt !== current) return;
        delivery.textContent = t('mail.requestFailed');
        refresh.hidden = false;
    }
}

email.addEventListener('input', clearDelivery);
window.addEventListener('pagehide', () => updatePasswordRecoveryPage(false));
refresh.addEventListener('click', () => {
    void runButtonAction(refresh, updateDelivery);
});
const send = document.getElementById('password-reset-send');
send.addEventListener('click', () => {
    void runButtonAction(send, async () => {
        if (!active || fields.disabled || !email.reportValidity()) return;
        error.textContent = '';
        const revision = epoch, address = email.value.trim();
        try {
            const result = await requestJSON('password-reset/request', {
                method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({email: address}),
            });
            if (!active || revision !== epoch || email.value.trim() !== address) return;
            clearDelivery();
            receipt = {...result, deadline: Date.now() + 10 * 60 * 1000};
            delivery.textContent = mailStatusLabel(result.status);
            code.focus();
            timer = setTimeout(() => {
                void updateDelivery();
            }, 3000);
        } catch (failure) {
            if (active && revision === epoch) error.textContent = failure instanceof LocalizedResponseError ? failure.message : t('login.recoveryFailed');
        }
    });
});

form.addEventListener('submit', event => {
    event.preventDefault();
    void runButtonAction(form.querySelector('[type="submit"]'), async () => {
        if (!active || fields.disabled) return;
        error.textContent = '';
        const passwordError = getPasswordLengthError(password.value);
        if (passwordError) {
            error.textContent = passwordError;
            password.focus();
            return;
        }
        if (password.value !== confirmation.value) {
            error.textContent = t('login.passwordsDoNotMatch');
            confirmation.focus();
            return;
        }
        const revision = epoch, returnTo = loginReturnTo();
        const request = {email: email.value.trim(), code: code.value.trim(), new_password: password.value};
        if (!(await confirmWeakPasswordIfNeeded(request.new_password)) || !active || revision !== epoch) return;
        try {
            const result = await requestJSON('password-reset/confirm', {
                method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify(request),
            });
            const returnToSignIn = active && revision === epoch;
            await logout('silent');
            if (!returnToSignIn) return;
            updatePasswordRecoveryPage(false);
            navigateToLogin(returnTo, {replace: true});
            document.getElementById('username').value = String(result.username || '');
            document.getElementById('password').focus();
            showAlert(t('login.recoverySuccess'), 'success');
        } catch (failure) {
            if (active && revision === epoch) error.textContent = failure instanceof LocalizedResponseError ? failure.message : t('login.recoveryFailed');
        }
    });
});
