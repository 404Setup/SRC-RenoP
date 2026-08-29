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
import {t} from './i18n.js';
import {refreshAccountSecurity} from './account-security.js';
import {runButtonAction} from './components.js';

let currentGitHubProfileStatus = null;

/**
 * Start a GitHub OAuth flow that returns to the current routed page.
 * @returns {void}
 */
function startGitHubOAuth() {
    const returnTo = window.location.pathname || '/';
    window.location.assign('/api/auth/github/start?return_to=' + encodeURIComponent(returnTo));
}

/**
 * Consume the short result marker added by the backend OAuth callback.
 * @returns {void}
 */
function showGitHubOAuthResult() {
    const current = new URL(window.location.href);
    const result = current.searchParams.get('github_oauth');
    if (!result) return;
    current.searchParams.delete('github_oauth');
    window.history.replaceState(
        window.history.state,
        '',
        current.pathname + current.search + current.hash
    );
    const messages = {
        success: ['login.githubSuccess', 'success'],
        linked: ['profile.githubLinked', 'success'],
        provider_denied: ['login.githubDenied', 'info'],
        state_invalid: ['login.githubExpired', 'error'],
        session_changed: ['login.githubExpired', 'error'],
        scope_missing: ['login.githubScopeMissing', 'error'],
        identity_linked: ['profile.githubAlreadyLinked', 'error'],
    };
    const pair = messages[result] || ['login.githubFailed', 'error'];
    showAlert(t(pair[0]), pair[1]);
}

/**
 * Load public provider availability and expose the login control only when fully configured.
 * @returns {Promise<void>}
 */
export async function initializeGitHubAuth() {
    showGitHubOAuthResult();
    const wrapper = document.getElementById('github-login-wrapper');
    const button = document.getElementById('btn-github-login');
    if (button && !button.dataset.bound) {
        button.dataset.bound = 'true';
        button.addEventListener('click', startGitHubOAuth);
    }
    try {
        const response = await fetch('/api/auth/github/status', {
            credentials: 'include',
            headers: {'Cache-Control': 'no-store'},
        });
        const status = response.ok ? await response.json() : null;
        if (wrapper) wrapper.hidden = !status?.enabled;
    } catch {
        if (wrapper) wrapper.hidden = true;
    }
}

/**
 * Render the GitHub connection carried by the current account profile response.
 * @param {object|null|undefined} status - Private connection state for the signed-in account.
 * @returns {void}
 */
export function renderGitHubConnection(status) {
    const section = document.getElementById('profile-github-section');
    const statusText = document.getElementById('profile-github-status');
    const connectButton = document.getElementById('btn-profile-github-connect');
    const disconnectButton = document.getElementById('btn-profile-github-disconnect');
    if (!section || !statusText || !connectButton || !disconnectButton) return;
    currentGitHubProfileStatus = status && typeof status === 'object' ? {...status} : null;
    connectButton.hidden = true;
    disconnectButton.hidden = true;
    if (!currentGitHubProfileStatus ||
        (!currentGitHubProfileStatus.configured && !currentGitHubProfileStatus.linked)) {
        section.hidden = true;
        return;
    }
    section.hidden = false;
    if (currentGitHubProfileStatus.linked) {
        const identityText = t('profile.githubConnectedAs', {
            login: currentGitHubProfileStatus.github_login || '',
            count: Number(currentGitHubProfileStatus.principal_count) || 1,
        });
        statusText.textContent = currentGitHubProfileStatus.can_disconnect
            ? identityText
            : identityText + ' ' + t('profile.githubOnlyLogin');
        connectButton.textContent = t('profile.githubRefresh');
        connectButton.hidden = !currentGitHubProfileStatus.configured;
        disconnectButton.hidden = !currentGitHubProfileStatus.can_disconnect;
        return;
    }
    statusText.textContent = t('profile.githubNotConnected');
    connectButton.textContent = t('profile.githubConnect');
    connectButton.hidden = !currentGitHubProfileStatus.configured;
}

document.getElementById('btn-profile-github-connect')?.addEventListener('click', startGitHubOAuth);
document.getElementById('btn-profile-github-disconnect')?.addEventListener('click', async event => {
    if (!(await window.showConfirm(t('profile.githubDisconnectConfirm')))) return;
    const button = event.currentTarget;
    await runButtonAction(button, async () => {
        try {
            const response = await apiRequest('/api/auth/profile/github', {method: 'DELETE'});
            if (!response.ok) {
                if (response.headers.get('X-Renop-Error-Code') === 'GITHUB_LAST_LOGIN_METHOD') {
                    showAlert(t('profile.githubOnlyLogin'), 'error');
                    return;
                }
                throw new Error('GitHub disconnect failed');
            }
            const nextStatus = {
                configured: Boolean(currentGitHubProfileStatus?.configured),
                linked: false,
                can_disconnect: false,
            };
            renderGitHubConnection(nextStatus);
            window.dispatchEvent(new CustomEvent('githubConnectionChanged', {detail: nextStatus}));
            showAlert(t('profile.githubDisconnected'), 'success');
            await refreshAccountSecurity();
        } catch (error) {
            console.error('Failed to disconnect GitHub account', error);
            showAlert(t('profile.githubDisconnectFailed'), 'error');
        }
    });
});

window.addEventListener('languageChanged', () => {
    if (currentGitHubProfileStatus) renderGitHubConnection(currentGitHubProfileStatus);
});

window.addEventListener('accountSecurityUpdated', event => {
    if (!currentGitHubProfileStatus?.linked || !event.detail) return;
    const security = event.detail;
    const canDisconnect = Number(security.fido_device_count) > 0 ||
        (security.password_configured === true && security.password_login_enabled === true);
    if (canDisconnect === Boolean(currentGitHubProfileStatus.can_disconnect)) return;
    renderGitHubConnection({...currentGitHubProfileStatus, can_disconnect: canDisconnect});
});
