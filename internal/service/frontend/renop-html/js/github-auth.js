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

let githubProfileLoadSequence = 0;

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
 * Refresh the GitHub connection card on the current account edit page.
 * @returns {Promise<void>}
 */
export async function refreshGitHubConnection() {
    const sequence = ++githubProfileLoadSequence;
    const section = document.getElementById('profile-github-section');
    const statusText = document.getElementById('profile-github-status');
    const connectButton = document.getElementById('btn-profile-github-connect');
    const disconnectButton = document.getElementById('btn-profile-github-disconnect');
    if (!section || !statusText || !connectButton || !disconnectButton) return;
    section.hidden = false;
    statusText.textContent = t('profile.githubLoading');
    connectButton.hidden = true;
    disconnectButton.hidden = true;
    try {
        const response = await apiRequest('/api/auth/profile/github');
        if (!response.ok) throw new Error('GitHub identity request failed');
        const status = await response.json();
        if (sequence !== githubProfileLoadSequence) return;
        if (!status.configured && !status.linked) {
            section.hidden = true;
            return;
        }
        if (status.linked) {
            const identityText = t('profile.githubConnectedAs', {
                login: status.github_login || '',
                count: Number(status.principal_count) || 1,
            });
            statusText.textContent = status.can_disconnect
                ? identityText
                : identityText + ' ' + t('profile.githubOnlyLogin');
            connectButton.textContent = t('profile.githubRefresh');
            connectButton.hidden = !status.configured;
            disconnectButton.hidden = !status.can_disconnect;
        } else {
            statusText.textContent = t('profile.githubNotConnected');
            connectButton.textContent = t('profile.githubConnect');
            connectButton.hidden = !status.configured;
            disconnectButton.hidden = true;
        }
    } catch (error) {
        if (sequence !== githubProfileLoadSequence) return;
        console.error('Failed to load GitHub connection', error);
        statusText.textContent = t('profile.githubLoadFailed');
    }
}

document.getElementById('btn-profile-github-connect')?.addEventListener('click', startGitHubOAuth);
document.getElementById('btn-profile-github-disconnect')?.addEventListener('click', async event => {
    if (!(await window.showConfirm(t('profile.githubDisconnectConfirm')))) return;
    const button = event.currentTarget;
    button.disabled = true;
    try {
        const response = await apiRequest('/api/auth/profile/github', {method: 'DELETE'});
        if (!response.ok) {
            if (response.headers.get('X-Renop-Error-Code') === 'GITHUB_LAST_LOGIN_METHOD') {
                showAlert(t('profile.githubOnlyLogin'), 'error');
                return;
            }
            throw new Error('GitHub disconnect failed');
        }
        showAlert(t('profile.githubDisconnected'), 'success');
        await refreshGitHubConnection();
        await refreshAccountSecurity();
    } catch (error) {
        console.error('Failed to disconnect GitHub account', error);
        showAlert(t('profile.githubDisconnectFailed'), 'error');
    } finally {
        button.disabled = false;
    }
});
