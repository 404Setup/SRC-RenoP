/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {t} from './i18n.js';
import {responseErrorMessage} from './response-errors.js';
import {requestPasskeyAssertion} from './fido-utils.js';
import {runButtonAction} from './components/button.js';

const form = document.getElementById('mfa-login-form');
const code = document.getElementById('mfa-login-code');
const errorBox = document.getElementById('mfa-login-error');
const passwordForm = document.getElementById('login-form');
const passkey = document.getElementById('mfa-login-passkey');
let sequence = 0, active = false, deadline = 0, abort;

/** Keep errors visible even if the pending challenge cannot be loaded. */
function showChallengeError(message) {
    const target = active ? errorBox : document.getElementById('login-error');
    target.textContent = message;
    target.hidden = false;
    if (!active) target.style.display = 'block';
}

/** Send a private second-factor request without treating a rejected code as a lost session. */
function mfaRequest(path = '', body, method = 'POST') {
    return fetch('/api/auth/mfa' + path, {
        method, credentials: 'include', cache: 'no-store',
        headers: {'Content-Type': 'application/json'},
        body: body === undefined ? undefined : JSON.stringify(body),
        signal: abort?.signal,
    });
}

/** Show the second-factor step only for a live server-side login challenge. */
export async function showMFALogin({silent = false} = {}) {
    const current = ++sequence;
    try {
        const response = await mfaRequest('', undefined, 'GET');
        if (current !== sequence) return;
        if (!response.ok) {
            if (!silent) showChallengeError(await responseErrorMessage(response, 'mfa.invalid'));
            return;
        }
        const status = await response.json();
        if (current !== sequence) return;
        active = true;
        passwordForm.reset();
        passwordForm.hidden = true;
        form.hidden = false;
        errorBox.hidden = true;
        form.reset();
        document.getElementById('mfa-login-totp').hidden = !status.totp;
        code.disabled = !status.totp;
        document.getElementById('mfa-login-submit').hidden = !status.totp;
        passkey.hidden = !status.passkey;
        passkey.disabled = false;
        (status.totp ? code : passkey).focus({preventScroll: true});
        clearTimeout(deadline);
        deadline = setTimeout(() => {
            if (current !== sequence) return;
            errorBox.textContent = t('mfa.invalid');
            errorBox.hidden = false;
            code.disabled = true;
            passkey.disabled = true;
        }, Math.max(0, Number(status.expires_at) - Date.now()));
    } catch {
        if (!silent && current === sequence) showChallengeError(t('mfa.unavailable'));
    }
}

/** Clear codes and unfinished challenges when leaving or restarting sign-in. */
export function resetMFALogin() {
    const hadChallenge = active;
    ++sequence;
    active = false;
    clearTimeout(deadline);
    abort?.abort();
    abort = undefined;
    form.reset();
    form.hidden = true;
    errorBox.hidden = true;
    code.disabled = false;
    passkey.disabled = false;
    passwordForm.hidden = false;
    if (hadChallenge) void mfaRequest('', undefined, 'DELETE').catch(() => {});
}

/** Restore a pending OAuth or refreshed login challenge when the login route opens. */
export function updateMFALoginPage(visible, entering = false) {
    if (!visible) resetMFALogin();
    else if (entering) void showMFALogin({silent: true});
}

/** Complete one explicitly selected factor and publish the authenticated session to the login UI. */
async function verifyFactor(factor) {
    const current = sequence;
    abort?.abort();
    abort = new AbortController();
    errorBox.hidden = true;
    try {
        let payload = {code: code.value};
        if (factor === 'passkey') {
            const begin = await mfaRequest('/passkey/begin', {});
            if (!begin.ok) { errorBox.textContent = await responseErrorMessage(begin, 'mfa.invalid'); errorBox.hidden = false; return; }
            const {options} = await begin.json();
            payload = {credential: await requestPasskeyAssertion(options, abort.signal)};
        }
        const response = await mfaRequest(factor === 'totp' ? '/totp' : '/passkey/finish', payload);
        if (current !== sequence) return;
        code.value = '';
        if (!response.ok) { errorBox.textContent = await responseErrorMessage(response, 'mfa.invalid'); errorBox.hidden = false; code.focus(); return; }
        const session = await response.json();
        if (current !== sequence) return;
        active = false;
        resetMFALogin();
        window.dispatchEvent(new CustomEvent('mfaLoginCompleted', {detail: session}));
    } catch (error) {
        if (current !== sequence || error.name === 'AbortError') return;
        errorBox.textContent = t(factor === 'passkey' ? 'login.fidoFailed' : 'mfa.unavailable');
        errorBox.hidden = false;
    }
}

form?.addEventListener('submit', event => {
    event.preventDefault();
    if (code.checkValidity()) void runButtonAction(document.getElementById('mfa-login-submit'), () => verifyFactor('totp'));
});
passkey?.addEventListener('click', () => void runButtonAction(passkey, () => verifyFactor('passkey')));
document.getElementById('mfa-login-restart')?.addEventListener('click', () => { resetMFALogin(); document.getElementById('username').focus(); });
window.addEventListener('pagehide', resetMFALogin);
