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
import {responseErrorMessage} from './response-errors.js';
import {navigateToLogin} from './login-route.js';
import {verifyProfileEmail} from './profile-email-verification.js';
import {renderAccountEmailAliases} from './account-emails.js';
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
	const passkeyMFA = document.getElementById('profile-mfa-passkey');
	if (passkeyMFA) {
		passkeyMFA.checked = security.passkey_second_factor === true;
		passkeyMFA.disabled = !passkeyMFA.checked && (!(Number(security.fido_device_count) > 0) ||
			!(security.github_linked || Number(security.oauth_identity_count) > 0 || (security.password_configured && security.password_login_enabled)));
	}
	$('#profile-mfa-totp-status').text(t(security.totp_enabled ? 'mfa.enabled' : 'mfa.disabled'));
	$('#profile-mfa-totp').text(t(security.totp_enabled ? 'mfa.remove' : 'mfa.setup'));
    const section = $('#profile-account-security-section').get(0);
    const emailInput = $('#profile-private-email').get(0);
    const toggle = $('#profile-password-login-toggle').get(0);
    const passwordHint = $('#profile-password-login-hint').get(0);
    const recoveryStatus = $('#profile-recovery-code-status').get(0);
    const recoveryButton = $('#btn-profile-recovery-codes').get(0);
    if (!section || !emailInput || !toggle || !passwordHint || !recoveryStatus || !recoveryButton) return;
    $(section).prop('hidden', false);
    $(emailInput).val(security.email || '');
    renderAccountEmailAliases(security, renderAccountSecurity, showMFASettingsError);
    $('#profile-private-email-hint').text(t(security.email_verification_required
        ? 'profile.privateEmailVerificationHint' : 'profile.privateEmailHint'));
    $(toggle).prop('checked', security.password_login_enabled === true);
    $(toggle).prop('disabled', toggle.checked
        ? security.can_disable_password_login !== true
        : security.password_configured !== true);
    if (!security.password_configured) {
        $(passwordHint).text(t('profile.passwordLoginNotConfigured'));
    } else if (toggle.checked && !security.can_disable_password_login) {
        $(passwordHint).text(t(security.passkey_second_factor ? 'mfa.primaryMethodHint' : 'profile.passwordLoginNeedsAlternative'));
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

/** Explain security failures and offer a fresh sign-in when the session is too old. */
async function showMFASettingsError(response) {
    const message = await responseErrorMessage(response, 'mfa.unavailable');
    if (response.headers.get('X-Renop-Error-Code') === 'MFA_REAUTH_REQUIRED') {
        if (await window.showConfirm(message)) navigateToLogin(window.location.pathname, {reauth: true});
    } else showAlert(message, 'error');
}

/** Save a confirmed second-factor preference and refresh dependent login controls. */
async function saveMFASettings(body) {
    const response = await apiRequest('/api/auth/profile/mfa', {
        method: 'PUT', headers: {'Content-Type': 'application/json'}, body: JSON.stringify(body),
    });
    if (!response.ok) { await showMFASettingsError(response); return; }
    renderAccountSecurity(await response.json());
    showAlert(t('mfa.saved'), 'success');
}

$('#profile-mfa-passkey').on('change', async event => {
    const toggle = event.currentTarget;
    toggle.disabled = true;
    try { await saveMFASettings({passkey_second_factor: toggle.checked}); }
    catch { showAlert(t('mfa.unavailable'), 'error'); }
    finally { if (currentSecurity) renderAccountSecurity(currentSecurity); }
});

/** Show the setup key once, with a local QR image and a code confirmation form. */
async function showTOTPSetup(setup) {
    const code = el('input', {id: 'totp-setup-code', type: 'text', inputmode: 'numeric',
        autocomplete: 'one-time-code', pattern: '[0-9]{6}', minlength: '6', maxlength: '6', required: true});
    const secret = el('code', {class: 'mfa-setup-secret'}, setup.secret);
    const image = el('img', {src: setup.qr, alt: t('mfa.qrAlt'), width: '320', height: '320'});
    const errorBox = el('p', {class: 'account-form-error', role: 'alert', hidden: true});
    const form = el('form', {class: 'account-verification'}, image,
        el('p', {}, t('mfa.manualHint')), secret,
        el('div', {class: 'account-field'}, el('label', {for: 'totp-setup-code'}, t('mfa.code')), code), errorBox);
    form.addEventListener('submit', event => {
        event.preventDefault();
        void runButtonAction(document.getElementById('totp-setup-verify'), async () => {
            errorBox.hidden = true;
            try {
                const response = await apiRequest('/api/auth/profile/mfa/totp/confirm', {
                    method: 'POST', headers: {'Content-Type': 'application/json'},
                    body: JSON.stringify({id: setup.id, code: code.value}),
                });
                code.value = '';
                if (!response.ok) { errorBox.textContent = await responseErrorMessage(response, 'mfa.invalid'); errorBox.hidden = false; return; }
                renderAccountSecurity(await response.json());
                document.getElementById('totp-setup-dialog')?.close(true);
                showAlert(t('mfa.saved'), 'success');
            } catch { errorBox.textContent = t('mfa.unavailable'); errorBox.hidden = false; }
        });
    });
    await RenopDialog.show({
        id: 'totp-setup-dialog', title: t('mfa.authenticator'), subtitle: t('mfa.setupHint'),
        icon: 'fileKey', maxWidth: '520px', body: form,
        footer: [
            {text: t('common.cancel'), className: 'action-btn', onClick: (event, dialog) => dialog.close(false)},
            {id: 'totp-setup-verify', text: t('mfa.verify'), className: 'action-btn primary-btn', onClick: () => form.requestSubmit()},
        ],
        onClose: () => { code.value = ''; secret.textContent = ''; image.removeAttribute('src'); setup.secret = setup.uri = setup.qr = ''; },
    });
}

$('#profile-mfa-totp').on('click', event => void runButtonAction(event.currentTarget, async () => {
    try {
        if (currentSecurity?.totp_enabled) {
            if (await window.showConfirm(t('mfa.removeConfirm'))) await saveMFASettings({totp_enabled: false});
            return;
        }
        const response = await apiRequest('/api/auth/profile/mfa/totp/begin', {
            method: 'POST', headers: {'Content-Type': 'application/json'}, body: '{}',
        });
        if (!response.ok) { await showMFASettingsError(response); return; }
        await showTOTPSetup(await response.json());
    } catch { showAlert(t('mfa.unavailable'), 'error'); }
}));

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
    const route = window.location.pathname;
    try {
        const response = await apiRequest('/api/auth/profile/email', {
            method: 'PUT',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({email}),
        });
        if (window.location.pathname !== route) return;
        if (!response.ok) {
            showAlert(await responseErrorMessage(response, 'profile.privateEmailSaveFailed'), 'error');
            return;
        }
        const result = await response.json();
        const security = response.status === 202 ? await verifyProfileEmail(email, result) : result;
        if (!security || window.location.pathname !== route) return;
        renderAccountSecurity(security);
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
