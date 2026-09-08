/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {showAlert} from './alert.js';
import {logout} from './auth.js';
import {loginReturnTo, navigateToLogin} from './login-route.js';
import {runButtonAction} from './components/button.js';
import {t} from './i18n.js';
import {attachPasswordStrength, confirmWeakPasswordIfNeeded, getPasswordLengthError} from './password-strength.js';

const form = document.getElementById('account-recovery-form');
const identifier = document.getElementById('recovery-identifier');
const codeInputs = Array.from(form.querySelectorAll('[name="recovery-code"]'));
const password = document.getElementById('recovery-password');
const confirmation = document.getElementById('recovery-password-confirmation');
const error = document.getElementById('recovery-error');
let strength;

/** Refresh labels and focus on entry; clear secrets whenever the page is left. */
export function updateAccountRecoveryPage(active, entering = false) {
    if (!active) {
        form.reset();
        strength?.reset();
        error.textContent = '';
        return;
    }
    strength = attachPasswordStrength(password);
    codeInputs.forEach((input, index) => {
        input.labels[0].textContent = t('login.recoveryCodePlaceholder', {index: index + 1});
    });
    if (entering) identifier.focus({preventScroll: true});
}

window.addEventListener('pagehide', () => updateAccountRecoveryPage(false));
form.addEventListener('submit', event => {
    event.preventDefault();
    void runButtonAction(form.querySelector('[type="submit"]'), async () => {
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
        const codes = codeInputs.map(input => input.value.trim());
        if (codes.some(code => !code) || new Set(codes.map(code => code.toUpperCase())).size !== 4) {
            error.textContent = t('login.fourDistinctCodesRequired');
            codeInputs[0].focus();
            return;
        }
        const request = {identifier: identifier.value.trim(), codes, new_password: password.value};
        const returnTo = loginReturnTo();
        if (!(await confirmWeakPasswordIfNeeded(request.new_password))) return;
        try {
            const response = await fetch('/api/auth/recovery/password', {
                method: 'POST', credentials: 'include', cache: 'no-store',
                headers: {'Content-Type': 'application/json'},
                body: JSON.stringify(request),
            });
            if (!response.ok) {
                error.textContent = response.status === 429 || response.status === 403
                    ? t('login.recoveryRateLimited')
                    : t(response.status === 400 || response.status === 401 ? 'login.recoveryInvalid' : 'login.recoveryFailed');
                return;
            }
            const result = await response.json();
            await logout('silent');
            updateAccountRecoveryPage(false);
            navigateToLogin(returnTo, {replace: true});
            document.getElementById('username').value = String(result.username || request.identifier);
            document.getElementById('password').focus();
            showAlert(t('login.recoverySuccess'), 'success');
        } catch (requestError) {
            console.error('Account recovery failed', requestError);
            error.textContent = t('login.recoveryFailed');
        }
    });
});
