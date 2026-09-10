/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {ensureLegalConsent} from './legal-consent.js';
import {t} from './i18n.js';
import {showAlert} from './alert.js';
import {runButtonAction} from './components/button.js';
import {loginReturnTo, navigateToLogin} from './login-route.js';
import {attachPasswordStrength, confirmWeakPasswordIfNeeded, getPasswordLengthError} from './password-strength.js';
import {LocalizedResponseError, responseErrorMessage} from './response-errors.js';
import {mailStatusLabel} from './mail-status.js';

const form = document.getElementById('registration-form');
const fields = document.getElementById('registration-fields');
const username = document.getElementById('registration-username');
const nickname = document.getElementById('registration-nickname');
const email = document.getElementById('registration-email');
const code = document.getElementById('registration-code');
const password = document.getElementById('registration-password');
const confirmation = document.getElementById('registration-confirmation');
const error = document.getElementById('registration-error');
const availability = document.getElementById('registration-availability');
const delivery = document.getElementById('registration-delivery');
const send = document.getElementById('registration-send');
const importProfile = document.getElementById('registration-import');
const github = document.getElementById('registration-github');
let active = false, epoch = 0, availabilityEpoch = 0, pending, receipt, timer, expiryTimer, strength;

/** Read public auth results without treating a rejected confirmation as an expired login. */
async function requestJSON(path, body) {
    const response = await fetch('/api/auth/' + path, {
        credentials: 'include', cache: 'no-store',
        signal: AbortSignal.timeout(15000), ...(body ? {
            method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify(body),
        } : {})
    });
    if (!response.ok) throw new LocalizedResponseError(await responseErrorMessage(response, 'registration.unavailable'), response.status);
    return response.json();
}

/** Keep registration links and the active form aligned with the live administrator switch. */
export async function refreshRegistrationAvailability() {
    const revision = ++availabilityEpoch;
    try {
        const status = await requestJSON('registration/status');
        if (revision !== availabilityEpoch) return;
        document.querySelectorAll('[data-registration-link]').forEach(link => {
            link.hidden = status.enabled !== true;
        });
        if (!active) return;
        fields.disabled = status.enabled !== true;
        availability.textContent = fields.disabled ? t('registration.disabled') : '';
        if (fields.disabled) return;
        email.required = status.email_required === true || Boolean(pending?.provider);
        email.readOnly = Boolean(pending?.provider) && pending.email_required !== true;
        code.required = pending?.provider ? pending.email_required === true : status.email_required === true;
        code.closest('.account-field').hidden = !code.required;
        send.hidden = !code.required;
        if (code.required && status.email_required !== true) {
            fields.disabled = true;
            availability.textContent = t('oauth.mailRequired');
        }
        if (pending?.provider && pending.expires_at <= Date.now()) {
            fields.disabled = true;
            availability.textContent = t('registration.expired');
        }
    } catch {
        if (revision !== availabilityEpoch) return;
        document.querySelectorAll('[data-registration-link]').forEach(link => {
            link.hidden = true;
        });
        if (active) {
            fields.disabled = true;
            availability.textContent = t('registration.unavailable');
        }
    }
}

/** Clear the private delivery receipt when an address or page changes. */
function clearDelivery() {
    clearTimeout(timer);
    receipt = null;
    delivery.textContent = '';
}

/** Restore a browser-bound confirmation without storing passwords, codes, or status tickets. */
async function loadRegistration() {
    const revision = epoch;
    availabilityEpoch++;
    fields.disabled = true;
    try {
        const result = await requestJSON('registration/pending');
        if (!active || revision !== epoch) return;
        pending = result;
        email.value = result.email || '';
        if (result.provider) {
            document.getElementById('registration-provider-info').hidden = false;
            document.getElementById('registration-provider-hint').textContent = result.provider === 'github' ? t('registration.providerHint')
                : t(result.email_required ? 'oauth.providerEmailHint' : 'oauth.providerHint', {provider: result.provider_name || result.provider});
            if (!result.username_available) error.textContent = t('registration.chooseUsername');
            expiryTimer = setTimeout(() => {
                if (!active || pending !== result) return;
                fields.disabled = true;
                availability.textContent = t('registration.expired');
            }, Math.max(0, result.expires_at - Date.now()));
        }
    } catch (failure) {
        if (!active || revision !== epoch) return;
        if (new URLSearchParams(window.location.search).has('provider')) {
            fields.disabled = true;
            availability.textContent = t('registration.expired');
            return;
        }
        if (!(failure instanceof LocalizedResponseError) || failure.status !== 400) {
            error.textContent = t('registration.unavailable');
        }
    }
    if (!active || revision !== epoch) return;
    await refreshRegistrationAvailability();
    if (active && revision === epoch && !fields.disabled) username.focus({preventScroll: true});
}

/** Start the registration page and discard transient credentials on departure. */
export function updateRegistrationPage(isActive, entering = false) {
    active = isActive;
    if (!active) {
        epoch++;
        clearDelivery();
        clearTimeout(expiryTimer);
        pending = null;
        form.reset();
        strength?.reset();
        error.textContent = '';
        fields.disabled = true;
        document.getElementById('registration-provider-info').hidden = true;
        return;
    }
    strength = attachPasswordStrength(password);
    if (entering) {
        epoch++;
        void loadRegistration();
    }
}

/** Poll this page's current delivery receipt only during its ten-minute lifetime. */
async function pollDelivery() {
    clearTimeout(timer);
    const current = receipt;
    if (!active || !current) return;
    try {
        const response = await fetch('/api/auth/mail/' + encodeURIComponent(current.id), {
            credentials: 'include', cache: 'no-store',
            headers: {'X-Renop-Mail-Ticket': current.ticket}, signal: AbortSignal.timeout(15000)
        });
        if (!response.ok) throw new Error('mail status unavailable');
        const result = await response.json();
        if (!active || receipt !== current) return;
        delivery.textContent = mailStatusLabel(result.status);
        if (['queued', 'paused', 'sending', 'checking', 'queued_provider'].includes(result.status) && Date.now() < current.deadline) {
            timer = setTimeout(() => {
                void pollDelivery();
            }, 3000);
        }
    } catch {
        if (active && receipt === current) delivery.textContent = t('mail.requestFailed');
    }
}

email.addEventListener('input', clearDelivery);
window.addEventListener('pagehide', () => updateRegistrationPage(false));
importProfile.addEventListener('change', () => {
    if (!pending?.provider) return;
    username.value = importProfile.checked && pending.username_available ? pending.username : '';
    nickname.value = importProfile.checked ? pending.nickname : '';
});
github.addEventListener('click', async () => {
    if (!active || fields.disabled) return;
    if (!(await ensureLegalConsent('registration'))) return;
    window.location.assign('/api/auth/github/start?intent=register&return_to=' + encodeURIComponent(loginReturnTo()));
});
send.addEventListener('click', () => runButtonAction(send, async () => {
    if (!active || fields.disabled || !email.reportValidity() || !(await ensureLegalConsent('registration'))) return;
    const revision = epoch, address = email.value.trim();
    error.textContent = '';
    try {
        const result = await requestJSON('registration/code', {email: address, provider: pending?.provider || ''});
        if (!active || revision !== epoch || address !== email.value.trim()) return;
        clearDelivery();
        receipt = {...result, deadline: Date.now() + 600000};
        delivery.textContent = mailStatusLabel(result.status);
        code.focus();
        timer = setTimeout(() => {
            void pollDelivery();
        }, 3000);
    } catch (failure) {
        if (active && revision === epoch) error.textContent = failure instanceof LocalizedResponseError ? failure.message : t('registration.unavailable');
    }
}));
form.addEventListener('submit', event => {
    event.preventDefault();
    void runButtonAction(form.querySelector('[type="submit"]'), async () => {
        if (!active || fields.disabled || !(await ensureLegalConsent('registration'))) return;
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
        const body = {
            username: username.value.trim(), nickname: nickname.value.trim(), email: email.value.trim(),
            password: password.value, code: code.value.trim(), provider: pending?.provider || '',
            import_avatar: importProfile.checked && pending?.avatar_available !== false
        };
        if (!(await confirmWeakPasswordIfNeeded(body.password)) || !active || revision !== epoch) return;
        try {
            const result = await requestJSON('registration', body);
            if (!active || revision !== epoch) return;
            updateRegistrationPage(false);
            navigateToLogin(returnTo, {replace: true});
            document.getElementById('username').value = result.username;
            document.getElementById('password').focus();
            showAlert(t(body.import_avatar && !result.avatar_imported ? 'registration.avatarSkipped' : 'registration.success'), 'success');
        } catch (failure) {
            if (active && revision === epoch) error.textContent = failure instanceof LocalizedResponseError ? failure.message : t('registration.unavailable');
        }
    });
});
