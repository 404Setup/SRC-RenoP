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
import {RenopDialog} from './components.js';
import {writeClipboardText} from './clipboard.js';
import {t} from './i18n.js';
import {formatTimestamp} from './time.js';
import {el} from '@renop/ui/dom';
import {$} from '@renop/ui/jquery';

let accountSecuritySequence = 0;
let currentSecurity = null;

/**
 * Render the current private account-security state.
 * @param {object} security - Private security response.
 * @returns {void}
 */
function renderAccountSecurity(security) {
    currentSecurity = security;
    const section = $('#profile-account-security-section').get(0);
    const emailInput = $('#profile-private-email').get(0);
    const toggle = $('#profile-password-login-toggle').get(0);
    const passwordHint = $('#profile-password-login-hint').get(0);
    const recoveryStatus = $('#profile-recovery-code-status').get(0);
    const recoveryButton = $('#btn-profile-recovery-codes').get(0);
    if (!section || !emailInput || !toggle || !passwordHint || !recoveryStatus || !recoveryButton) return;
    $(section).prop('hidden', false);
    $(emailInput).val(security.email || '');
    $(toggle).prop('checked', security.password_login_enabled === true);
    $(toggle).prop('disabled', toggle.checked
        ? security.can_disable_password_login !== true
        : security.password_configured !== true);
    if (!security.password_configured) {
        $(passwordHint).text(t('profile.passwordLoginNotConfigured'));
    } else if (toggle.checked && !security.can_disable_password_login) {
        $(passwordHint).text(t('profile.passwordLoginNeedsAlternative'));
    } else if (toggle.checked) {
        $(passwordHint).text(t('profile.passwordLoginEnabled'));
    } else {
        $(passwordHint).text(t('profile.passwordLoginDisabled'));
    }
    const remaining = Number(security.recovery_codes_remaining) || 0;
    const total = Number(security.recovery_code_count) || 0;
    if (total > 0) {
        $(recoveryStatus).text(t('profile.recoveryCodesStatus', {
            remaining,
            date: formatTimestamp(security.recovery_generated_at, {fallback: t('common.unknown')}),
        }));
        $(recoveryButton).text(t('profile.regenerateRecoveryCodes'));
    } else {
        $(recoveryStatus).text(t('profile.recoveryCodesNone'));
        $(recoveryButton).text(t('profile.generateRecoveryCodes'));
    }
    window.dispatchEvent(new CustomEvent('accountSecurityUpdated', {detail: {...security}}));
}

/**
 * Display newly generated recovery codes exactly once.
 * @param {string[]} codes - Plaintext recovery codes returned once by the server.
 * @returns {void}
 */
function showRecoveryCodes(codes) {
    const codeList = el('div', {class: 'profile-recovery-code-list'});
    codes.forEach((code, index) => {
        codeList.appendChild(el('code', {class: 'profile-recovery-code'},
            el('span', {class: 'profile-recovery-code-index'}, String(index + 1)),
            el('span', {}, code)
        ));
    });
    const body = el('div', {class: 'profile-recovery-dialog'},
        el('div', {class: 'profile-recovery-warning'}, t('profile.recoveryCodesOneTimeWarning')),
        codeList
    );
    void RenopDialog.show({
        id: 'profile-recovery-codes-dialog',
        maxWidth: '720px',
        icon: 'fileKey',
        title: t('profile.recoveryCodesTitle'),
        subtitle: t('profile.recoveryCodesDialogDesc'),
        body,
        footer: [
            {
                text: t('profile.copyAllRecoveryCodes'),
                className: 'action-btn',
                onClick: async () => {
                    try {
                        await writeClipboardText(codes.join('\n'));
                        showAlert(t('prompt.copied'), 'success');
                    } catch (error) {
                        console.error('Failed to copy recovery codes', error);
                        showAlert(t('profile.recoveryCodesCopyFailed'), 'error');
                    }
                }
            },
            {
                text: t('common.close'),
                className: 'action-btn primary-btn',
                onClick: (event, dialog) => dialog.close(true)
            }
        ]
    });
}

/**
 * Reload private account-security state for the profile edit page.
 * @returns {Promise<void>}
 */
export async function refreshAccountSecurity() {
    const sequence = ++accountSecuritySequence;
    const section = $('#profile-account-security-section').get(0);
    if (!section) return;
    $(section).prop('hidden', false);
    try {
        const response = await apiRequest('/api/auth/profile/security');
        if (!response.ok) throw new Error('Account security request failed');
        const security = await response.json();
        if (sequence !== accountSecuritySequence) return;
        renderAccountSecurity(security);
    } catch (error) {
        if (sequence !== accountSecuritySequence) return;
        console.error('Failed to load account security', error);
    }
}

$(window).on('languageChanged', () => {
    if (currentSecurity) renderAccountSecurity(currentSecurity);
});

$('#profile-private-email-form').on('submit', async event => {
    event.preventDefault();
    const form = event.currentTarget;
    const input = $('#profile-private-email').get(0);
    const button = $(form).find('button[type="submit"]').get(0);
    if (!input || !button) return;
    const email = input.value.trim();
    if (!email || !input.checkValidity()) {
        showAlert(t('profile.privateEmailInvalid'), 'error');
        input.focus();
        input.reportValidity();
        return;
    }
    $(button).prop('disabled', true);
    try {
        const response = await apiRequest('/api/auth/profile/email', {
            method: 'PUT',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({email}),
        });
        if (!response.ok) {
            const code = response.headers.get('X-Renop-Error-Code');
            showAlert(t(code === 'ACCOUNT_EMAIL_CONFLICT'
                ? 'profile.privateEmailConflict'
                : 'profile.privateEmailInvalid'), 'error');
            return;
        }
        renderAccountSecurity(await response.json());
        showAlert(t('profile.privateEmailSaved'), 'success');
    } catch (error) {
        console.error('Failed to save private email', error);
        showAlert(t('profile.privateEmailSaveFailed'), 'error');
    } finally {
        $(button).prop('disabled', false);
    }
});

$('#profile-password-login-toggle').on('change', async event => {
    const toggle = event.currentTarget;
    const previous = !toggle.checked;
    $(toggle).prop('disabled', true);
    try {
        const response = await apiRequest('/api/auth/profile/password-login', {
            method: 'PUT',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({enabled: toggle.checked}),
        });
        if (!response.ok) {
            toggle.checked = previous;
            const code = response.headers.get('X-Renop-Error-Code');
            showAlert(t(code === 'ACCOUNT_PASSWORD_NOT_CONFIGURED'
                ? 'profile.passwordLoginNotConfigured'
                : 'profile.passwordLoginNeedsAlternative'), 'error');
            return;
        }
        renderAccountSecurity(await response.json());
        showAlert(t(toggle.checked
            ? 'profile.passwordLoginEnabledNotice'
            : 'profile.passwordLoginDisabledNotice'), 'success');
    } catch (error) {
        toggle.checked = previous;
        console.error('Failed to update password login', error);
        showAlert(t('profile.passwordLoginUpdateFailed'), 'error');
    } finally {
        if (currentSecurity) renderAccountSecurity(currentSecurity);
    }
});

$('#btn-profile-recovery-codes').on('click', async event => {
    const button = event.currentTarget;
    if ((Number(currentSecurity?.recovery_code_count) || 0) > 0 &&
        !(await window.showConfirm(t('profile.recoveryCodesRegenerateConfirm')))) {
        return;
    }
    $(button).prop('disabled', true);
    try {
        const response = await apiRequest('/api/auth/profile/recovery-codes', {method: 'POST'});
        if (!response.ok) throw new Error('Recovery-code generation failed');
        const result = await response.json();
        if (!Array.isArray(result.codes) || result.codes.length !== 12) {
            throw new Error('Recovery-code response is incomplete');
        }
        renderAccountSecurity(result.security || currentSecurity || {});
        showRecoveryCodes(result.codes);
    } catch (error) {
        console.error('Failed to generate recovery codes', error);
        showAlert(t('profile.recoveryCodesGenerateFailed'), 'error');
    } finally {
        $(button).prop('disabled', false);
    }
});
