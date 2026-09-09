/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {el} from '@renop/ui/dom';
import {t} from './i18n.js';
import {apiRequest} from './api.js';
import {showAlert} from './alert.js';
import {createIcon, runButtonAction} from './components.js';
import {loginReturnTo} from './login-route.js';
import {refreshAccountSecurity} from './account-security.js';
import {LocalizedResponseError, responseErrorMessage} from './response-errors.js';

let publicProviders = [], privateProviders = [], profileUsername = '', profileRevision = 0, publicRevision = 0;

/** Start a server-authorized provider action with a local return route. */
function startOAuth(provider, intent) {
    const returnTo = ['login', 'register'].includes(intent) ? loginReturnTo() : window.location.pathname;
    window.location.assign('/api/auth/oauth/' + encodeURIComponent(provider) + '/start?' + new URLSearchParams({
        intent,
        return_to: returnTo
    }));
}

/** Build a provider action without interpreting provider-supplied text as markup. */
function providerButton(label, provider, intent, className = 'pill-btn pill-btn--soft profile-action-btn') {
    return el('button', {type: 'button', class: className, onclick: () => startOAuth(provider, intent)}, label);
}

/** Render the configured public providers on both account entry pages. */
function renderPublicProviders() {
    for (const intent of ['login', 'register']) {
        const container = document.getElementById('oauth-' + intent + '-providers');
        if (!container) continue;
        container.replaceChildren(...publicProviders.map(provider =>
            providerButton(t('oauth.continue', {provider: provider.name}), provider.id, intent, 'account-provider')));
        container.hidden = publicProviders.length === 0;
    }
}

/** Consume the callback marker, then read public provider availability without a login side effect. */
export async function initializeOAuth() {
    const revision = ++publicRevision;
    const current = new URL(window.location.href);
    const result = current.searchParams.get('oauth');
    if (result) {
        current.searchParams.delete('oauth');
        current.searchParams.delete('provider');
        window.history.replaceState(window.history.state, '', current.pathname + current.search + current.hash);
        const messages = {
            success: ['oauth.success', 'success'],
            linked: ['oauth.linked', 'success'],
            provider_denied: ['oauth.denied', 'info'],
            state_invalid: ['oauth.expired', 'error'],
            session_changed: ['oauth.expired', 'error'],
            configuration_changed: ['oauth.expired', 'error'],
            identity_linked: ['oauth.alreadyLinked', 'error'],
            account_banned: ['login.accountBanned', 'error'],
            account_deleted: ['login.accountDeleted', 'error'],
            email_updated: ['profile.privateEmailSaved', 'success'],
            email_conflict: ['profile.privateEmailConflict', 'error'],
            email_unverified: ['profile.providerEmailUnverified', 'error'],
            email_limit: ['profile.emailAliasLimit', 'error'],
            email_blocked: ['mail.recipientBlocked', 'error'],
            email_missing: ['oauth.emailMissing', 'error'],
            avatar_updated: ['profile.avatarUpdated', 'success'],
            avatar_failed: ['oauth.avatarFailed', 'error'],
            registration_disabled: ['registration.disabled', 'error'],
            registration_ip_limited: ['registration.ipLimited', 'error'],
            registration_cooldown: ['registration.cooldown', 'error'],
            registration_pending: ['registration.pending', 'error'],
        };
        const [key, kind] = messages[result] || ['oauth.failed', 'error'];
        showAlert(t(key), kind);
    }
    let providers = [];
    try {
        const response = await fetch('/api/auth/oauth/providers', {
            credentials: 'include',
            cache: 'no-store',
            signal: AbortSignal.timeout(15000)
        });
        providers = response.ok ? (await response.json()).providers : [];
    } catch {
    }
    if (revision !== publicRevision) return;
    publicProviders = providers;
    renderPublicProviders();
}

/** Render private connection controls; the API rechecks every mutation and the last login method. */
function renderPrivateProviders() {
    const container = document.getElementById('profile-oauth-providers');
    if (!container) return;
    container.replaceChildren();
    container.hidden = privateProviders.length === 0;
    for (const provider of privateProviders) {
        const actions = el('div', {class: 'profile-github-actions'});
        if (provider.configured) actions.appendChild(providerButton(t(provider.linked ? 'oauth.refresh' : 'oauth.connect'), provider.id, 'link'));
        if (provider.can_verify_email) actions.appendChild(providerButton(t('oauth.useEmail'), provider.id, 'email'));
        if (provider.linked && provider.can_import_avatar) actions.appendChild(providerButton(t('oauth.useAvatar'), provider.id, 'avatar'));
        if (provider.linked && provider.can_disconnect) {
            const disconnect = el('button', {
                type: 'button',
                class: 'pill-btn pill-btn--soft profile-action-btn'
            }, t('oauth.disconnect'));
            disconnect.addEventListener('click', () => runButtonAction(disconnect, async () => {
                const username = profileUsername;
                if (!(await window.showConfirm(t('oauth.disconnectConfirm', {provider: provider.name}))) || username !== profileUsername) return;
                try {
                    const response = await apiRequest('/api/auth/profile/oauth/' + encodeURIComponent(provider.id), {method: 'DELETE'});
                    if (!response.ok) throw new LocalizedResponseError(await responseErrorMessage(response, 'oauth.failed'), response.status);
                    if (username !== profileUsername) return;
                    showAlert(t('oauth.disconnected'), 'success');
                    await refreshOAuthProfile(username);
                    await refreshAccountSecurity();
                } catch (error) {
                    showAlert(error instanceof LocalizedResponseError ? error.message : t('oauth.failed'), 'error');
                }
            }));
            actions.appendChild(disconnect);
        }
        const status = provider.linked ? t('oauth.connectedAs', {login: provider.login || provider.name}) : t('oauth.notConnected');
        container.appendChild(el('div', {class: 'profile-settings-section'},
            el('div', {class: 'profile-section-card-header'},
                el('div', {class: 'profile-section-icon'}, createIcon('user')),
                el('div', {class: 'profile-section-meta'}, el('h3', {class: 'profile-section-title'}, provider.name),
                    el('p', {class: 'profile-section-desc'}, status))),
            el('div', {class: 'profile-section-body profile-github-body'},
                ...(provider.linked && !provider.can_disconnect ? [el('p', {class: 'profile-github-status'}, t('oauth.onlyLogin'))] : []), actions)));
    }
}

/** Fetch provider bindings only when editing the current account, discarding stale account responses. */
export async function refreshOAuthProfile(username) {
    profileUsername = username;
    const revision = ++profileRevision;
    const container = document.getElementById('profile-oauth-providers');
    if (!container) return;
    privateProviders = [];
    container.hidden = false;
    container.replaceChildren(el('p', {role: 'status'}, t('oauth.loading')));
    try {
        const response = await apiRequest('/api/auth/profile/oauth');
        if (!response.ok) throw new Error('provider status unavailable');
        const data = await response.json();
        if (revision !== profileRevision) return;
        privateProviders = data.providers;
        renderPrivateProviders();
    } catch {
        if (revision === profileRevision) container.replaceChildren(el('p', {role: 'status'}, t('oauth.failed')));
    }
}

window.addEventListener('languageChanged', () => {
    renderPublicProviders();
    if (privateProviders.length) renderPrivateProviders();
});
window.addEventListener('oauthProvidersChanged', initializeOAuth);
window.addEventListener('authChanged', event => {
    if (event.detail?.isLoggedIn && event.detail.username === profileUsername) return;
    profileRevision++;
    profileUsername = '';
    privateProviders = [];
    renderPrivateProviders();
});
window.addEventListener('accountSecurityUpdated', event => {
    if (!privateProviders.length || !event.detail) return;
    const security = event.detail;
    const canDisconnect = (security.password_configured && security.password_login_enabled) || security.github_linked ||
        Number(security.oauth_identity_count) > 1 || (Number(security.fido_device_count) > 0 && !security.passkey_second_factor);
    if (privateProviders.every(provider => !provider.linked || provider.can_disconnect === Boolean(canDisconnect))) return;
    privateProviders = privateProviders.map(provider => ({
        ...provider,
        can_disconnect: provider.linked && Boolean(canDisconnect)
    }));
    renderPrivateProviders();
});
