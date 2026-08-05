/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {fetchProto, getAuthHeaders, postProto} from './api.js';
import {showAlert} from './alert.js';
import {t} from './i18n.js';
import {updateTabIndicator} from '@renop/ui/tabs';
import {closeModalWithAnim} from './app-ui.js';
import {loadDirectory} from './browser.js';
import {stopDashboardRefresh} from './dashboard.js';
import {LoginRequest, SessionDetails} from './proto/index.js';
import {base64urlToBuffer, bufferToBase64url} from './fido-utils.js';

const loginBtn = document.getElementById('login-btn');
const logoutBtn = document.getElementById('logout-btn');
const userInfo = document.getElementById('user-info');
const usernameDisplay = document.getElementById('username-display');
const loginModal = document.getElementById('login-modal');
const closeLoginModal = document.getElementById('close-login-modal');
const loginForm = document.getElementById('login-form');
const loginError = document.getElementById('login-error');

export let cachedIsLoggedIn = false;
export let cachedIsManager = false;
/** @type {Array<{identifier?: string, shortcut?: string}>} */
export let cachedPermissions = [];
/** @type {Array<{path?: string, permission?: {identifier?: string, shortcut?: string}}>} */
export let cachedRoutes = [];

const MANAGER_PERMISSION_IDS = new Set([
    'manager',
    'm',
    'access-token:manager',
    'admin',
]);

/** Set by main.js after switchTab is defined — avoids auth↔main import cycle. */
let switchTabHandler = null;

/**
 * Register the tab-switch callback from main.js (avoids auth↔main import cycle).
 * @param {(tabId: string) => *} fn - Handler that switches the active tab.
 * @returns {void}
 */
export function setSwitchTabHandler(fn) {
    switchTabHandler = fn;
}

/**
 * Switch to a tab via the registered handler, or persist the selection if none is set.
 * @param {string} tabId - Tab identifier (e.g. 'overview', 'dashboard').
 * @returns {*} Result of the handler, if any.
 */
function requestSwitchTab(tabId) {
    if (typeof switchTabHandler === 'function') {
        return switchTabHandler(tabId);
    }
    localStorage.setItem('selectedTab', tabId);
}

/**
 * Whether session permissions grant management UI / admin APIs.
 * Mirrors backend isManagerPermissions / User.IsManager.
 * @param {Array<string|{identifier?: string}>} permissions - Session permission entries.
 * @returns {boolean} True when any entry matches a manager/admin permission id.
 */
export function isManagerFromSession(permissions) {
    if (!permissions || !permissions.length) return false;
    return permissions.some((p) => {
        const id = typeof p === 'string' ? p : p && p.identifier;
        return id && MANAGER_PERMISSION_IDS.has(id);
    });
}

/**
 * Decode the first path segment as a repository name.
 * @param {string} path
 * @returns {string}
 */
export function repoNameFromPath(path) {
    if (!path) return '';
    const parts = String(path).split('/').filter((p) => p.length > 0);
    if (parts.length === 0) return '';
    try {
        return decodeURIComponent(parts[0]);
    } catch {
        return parts[0];
    }
}

/**
 * Whether the current session may write (PUT/POST/DELETE) to a repository.
 * Aligned with config.User.CheckUpdatePermission:
 * manager/admin, canupdate:*, canupdate:<repo>, or session routes with route:write.
 * @param {string} repoName
 * @returns {boolean}
 */
export function canUpdateRepo(repoName) {
    if (!cachedIsLoggedIn) return false;
    if (cachedIsManager) return true;
    if (!repoName) return false;

    let name = repoName;
    try {
        name = decodeURIComponent(repoName);
    } catch {
        name = repoName;
    }

    for (const p of cachedPermissions) {
        const id = typeof p === 'string' ? p : p && p.identifier;
        if (!id) continue;
        if (id === 'canupdate:*') return true;
        if (id === `canupdate:${name}`) return true;
    }

    for (const r of cachedRoutes) {
        if (!r) continue;
        const permId = r.permission && r.permission.identifier;
        if (permId !== 'route:write' && permId !== 'w') continue;
        if (r.path === '*' || r.path === name) return true;
    }

    return false;
}

/**
 * Update module-level session caches from login/session payload.
 * @param {boolean} isLoggedIn - Whether the user is authenticated.
 * @param {Array} [permissions=[]] - Permission entries from the session.
 * @param {Array} [routes=[]] - Route permission entries from the session.
 * @returns {void}
 */
function applySessionCaches(isLoggedIn, permissions = [], routes = []) {
    cachedIsLoggedIn = isLoggedIn;
    cachedPermissions = isLoggedIn && Array.isArray(permissions) ? permissions : [];
    cachedRoutes = isLoggedIn && Array.isArray(routes) ? routes : [];
    cachedIsManager = isLoggedIn && isManagerFromSession(cachedPermissions);
}

/**
 * Refresh auth-related UI (login/logout controls, manager tabs, avatar) and session caches.
 * @param {boolean} isLoggedIn - Whether the user is authenticated.
 * @param {string} [name=''] - Display name for the signed-in user.
 * @param {boolean} [isManager=false] - Legacy manager flag when no permission list is provided.
 * @param {Array} [permissions=[]] - Session permissions (preferred source of manager status).
 * @param {Array} [routes=[]] - Session route permissions.
 * @returns {void}
 */
export function updateAuthUI(isLoggedIn, name = '', isManager = false, permissions = [], routes = []) {
    applySessionCaches(isLoggedIn, permissions, routes);
    if (isLoggedIn && (!permissions || permissions.length === 0) && isManager) {
        cachedIsManager = true;
    }
    isManager = cachedIsManager;

    const avatarDot = document.getElementById('user-avatar-dot');

    if (isLoggedIn) {
        if (loginBtn) loginBtn.style.display = 'none';
        if (userInfo) userInfo.style.display = 'flex';
        if (usernameDisplay) usernameDisplay.textContent = name;
        if (avatarDot) {
            avatarDot.textContent = (name || '?').trim().charAt(0).toUpperCase() || '?';
        }
    } else {
        if (loginBtn) loginBtn.style.display = 'inline-flex';
        if (userInfo) userInfo.style.display = 'none';
        if (usernameDisplay) usernameDisplay.textContent = '';
        if (avatarDot) avatarDot.textContent = '';
    }

    document.querySelectorAll('.manager-only').forEach(el => {
        el.style.display = isManager ? 'inline-block' : 'none';
    });

    document.querySelectorAll('.user-only').forEach(el => {
        el.style.display = isLoggedIn ? 'inline-block' : 'none';
    });

    const visibleTabs = Array.from(document.querySelectorAll('#tabs .tab')).filter(el => el.style.display !== 'none');
    const tabsContainer = document.querySelector('#tabs')?.closest('.tabs-container');
    const tabsEl = document.querySelector('#tabs');
    if (tabsEl) {
        let indicator = tabsEl.querySelector('.tab-indicator');
        if (!indicator) {
            indicator = document.createElement('div');
            indicator.className = 'tab-indicator';
            tabsEl.appendChild(indicator);
        }
        if (tabsContainer) {
            if (visibleTabs.length <= 1) {
                tabsContainer.classList.add('single-tab');
                indicator.style.display = 'none';
            } else {
                tabsContainer.classList.remove('single-tab');
                indicator.style.display = 'block';
            }
        }
        updateTabIndicator(tabsEl);
    }

    const currentTabId = localStorage.getItem('selectedTab') || 'overview';
    const tabEl = document.querySelector(`.tabs .tab[data-tab="${currentTabId}"]`);
    if (!isLoggedIn || (tabEl && tabEl.classList.contains('manager-only') && !isManager)) {
        localStorage.setItem('selectedTab', 'overview');
        requestSwitchTab('overview');
    }
}

/**
 * Restore session state from `/api/auth/me` and update the auth UI.
 * Logs out on 401/403 when a prior username was stored; otherwise marks logged out.
 * @returns {Promise<void>}
 */
export async function initializeSession() {
    localStorage.removeItem('session-token');
    const wasLoggedIn = !!localStorage.getItem('username');

    try {
        const {response, data: sessionData} = await fetchProto('/api/auth/me', SessionDetails);
        if (response.ok) {
            const permissions = (sessionData && sessionData.permissions) || [];
            const routes = (sessionData && sessionData.routes) || [];
            const isManager = isManagerFromSession(permissions);

            const serverName = sessionData && sessionData.access_token && sessionData.access_token.name
                ? sessionData.access_token.name
                : (localStorage.getItem('username') || '');

            if (serverName) {
                localStorage.setItem('username', serverName);
            }

            updateAuthUI(true, serverName, isManager, permissions, routes);
        } else if (response.status === 403 || response.status === 401) {
            if (wasLoggedIn) {
                logout('expired');
            } else {
                updateAuthUI(false);
            }
        } else {
            const username = localStorage.getItem('username') || '';
            updateAuthUI(!!username, username, false);
        }
    } catch (error) {
        console.error('Failed to fetch session:', error);
        updateAuthUI(false);
    }
}

/**
 * Authenticate with name/secret, update session UI, and reload the browser directory.
 * Shows an inline error on the login form on failure.
 * @param {string} name - Access token / username.
 * @param {string} secret - Access token secret / password.
 * @returns {Promise<void>}
 */
export async function login(name, secret) {
    loginError.style.display = 'none';

    try {
        const {response, data: sessionData} = await postProto(
            '/api/auth/login',
            LoginRequest,
            {name, secret},
            SessionDetails
        );

        if (response.ok) {
            const permissions = (sessionData && sessionData.permissions) || [];
            const routes = (sessionData && sessionData.routes) || [];
            const isManager = isManagerFromSession(permissions);

            localStorage.removeItem('token-name');
            localStorage.removeItem('token-secret');
            localStorage.setItem('username', name);
            localStorage.removeItem('session-token');

            updateAuthUI(true, name, isManager, permissions, routes);
            closeModalWithAnim(loginModal, () => {
                loginForm.reset();
            });

            showAlert(t('login.welcomeBack', {name}), 'success');
            loadDirectory(window.location.pathname);
        } else {
            loginError.textContent = window.translateError ? window.translateError('Invalid credentials') : 'Invalid credentials';
            loginError.style.display = 'block';
        }
    } catch (error) {
        console.error('Login error:', error);
        loginError.textContent = window.translateError ? window.translateError('An error occurred during login') : 'An error occurred during login';
        loginError.style.display = 'block';
    }
}

/**
 * End the server session, clear local auth state, and reset UI to the overview tab.
 * @param {string} [reason] - Logout reason: 'expired' | 'kicked' | 'silent' | undefined (manual).
 * @returns {Promise<void>}
 */
export async function logout(reason) {
    const wasLoggedIn = !!localStorage.getItem('username');
    try {
        await fetch('/api/auth/logout', {
            method: 'POST',
            credentials: 'include',
            headers: {
                ...getAuthHeaders(),
                'Cache-Control': 'no-store',
            },
        });
    } catch (error) {
        console.error('Failed to logout on server:', error);
    }
    localStorage.removeItem('session-token');
    localStorage.removeItem('username');
    localStorage.removeItem('token-name');
    localStorage.removeItem('token-secret');
    document.cookie = 'renop_session=; path=/; max-age=0; expires=Thu, 01 Jan 1970 00:00:00 GMT';
    document.cookie = 'renop_session=; path=/; max-age=0; expires=Thu, 01 Jan 1970 00:00:00 GMT; secure';
    updateAuthUI(false);

    if (wasLoggedIn && (reason === 'expired' || reason === 'kicked')) {
        showAlert(t('login.sessionExpired'), 'error');
    } else if (wasLoggedIn && reason !== 'silent') {
        showAlert(t('login.signedOut'), 'info');
    }

    stopDashboardRefresh();

    localStorage.setItem('selectedTab', 'overview');
    requestSwitchTab('overview');
    loadDirectory(window.location.pathname);
}

export async function fidoLogin() {
    loginError.style.display = 'none';

    if (!window.PublicKeyCredential) {
        loginError.textContent = t('login.fidoUnsupported') || 'FIDO/WebAuthn is not supported by your browser.';
        loginError.style.display = 'block';
        return;
    }

    const usernameInput = document.getElementById('username');
    const name = usernameInput ? usernameInput.value.trim() : '';

    try {
        const beginRes = await fetch('/api/auth/fido/login/begin', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({username: name})
        });

        if (!beginRes.ok) {
            const errText = await beginRes.text();
            const translatedErr = window.translateError ? window.translateError(errText) : errText;
            loginError.textContent = translatedErr || t('login.fidoFailed') || 'FIDO login failed';
            loginError.style.display = 'block';
            return;
        }

        const {session_id, options} = await beginRes.json();
        if (!options || !options.publicKey) {
            loginError.textContent = t('login.fidoFailed') || 'FIDO login failed';
            loginError.style.display = 'block';
            return;
        }

        const publicKey = options.publicKey;
        publicKey.challenge = base64urlToBuffer(publicKey.challenge);

        if (Array.isArray(publicKey.allowCredentials)) {
            publicKey.allowCredentials = publicKey.allowCredentials.map(c => ({
                ...c,
                id: base64urlToBuffer(c.id)
            }));
            if (publicKey.allowCredentials.length === 0) {
                delete publicKey.allowCredentials;
            }
        }

        if (publicKey.authenticatorSelection) {
            delete publicKey.authenticatorSelection.authenticatorAttachment;
        }
        if (!publicKey.userVerification) {
            publicKey.userVerification = 'preferred';
        }

        const assertion = await navigator.credentials.get({publicKey});
        if (!assertion) {
            loginError.textContent = t('login.fidoFailed') || 'FIDO login failed';
            loginError.style.display = 'block';
            return;
        }

        const credentialJSON = {
            id: assertion.id,
            rawId: bufferToBase64url(assertion.rawId),
            type: assertion.type,
            response: {
                authenticatorData: bufferToBase64url(assertion.response.authenticatorData),
                clientDataJSON: bufferToBase64url(assertion.response.clientDataJSON),
                signature: bufferToBase64url(assertion.response.signature),
                userHandle: assertion.response.userHandle ? bufferToBase64url(assertion.response.userHandle) : null,
            }
        };

        const finishRes = await fetch('/api/auth/fido/login/finish', {
            method: 'POST',
            credentials: 'include',
            headers: {
                'Content-Type': 'application/json',
                'Accept': 'application/json'
            },
            body: JSON.stringify({
                session_id,
                credential: credentialJSON
            })
        });

        if (finishRes.ok) {
            const sessionData = await finishRes.json();
            const permissions = (sessionData && sessionData.permissions) || [];
            const routes = (sessionData && sessionData.routes) || [];
            const isManager = isManagerFromSession(permissions);

            const serverName = sessionData && sessionData.access_token && sessionData.access_token.name
                ? sessionData.access_token.name
                : name;

            if (serverName) {
                localStorage.setItem('username', serverName);
            }

            updateAuthUI(true, serverName, isManager, permissions, routes);
            closeModalWithAnim(loginModal, () => {
                loginForm.reset();
            });

            showAlert(t('login.welcomeBack', {name: serverName}), 'success');
            loadDirectory(window.location.pathname);
        } else {
            const errText = await finishRes.text();
            const translatedErr = window.translateError ? window.translateError(errText) : errText;
            loginError.textContent = translatedErr || (window.translateError ? window.translateError('Invalid FIDO credential') : 'Invalid FIDO credential');
            loginError.style.display = 'block';
        }
    } catch (error) {
        console.error('FIDO Login error:', error);
        const errMsg = error && (error.message || error.name || String(error));
        const translatedMsg = errMsg && window.translateError ? window.translateError(errMsg) : errMsg;
        loginError.textContent = translatedMsg || (window.translateError ? window.translateError('An error occurred during FIDO login') : 'An error occurred during FIDO login');
        loginError.style.display = 'block';
    }
}

if (loginBtn) {
    loginBtn.addEventListener('click', () => {
        loginModal.style.display = 'flex';
        if (window.updateModalInertState) window.updateModalInertState();
    });
}

if (closeLoginModal) {
    closeLoginModal.addEventListener('click', () => {
        closeModalWithAnim(loginModal, () => {
            loginError.style.display = 'none';
            loginForm.reset();
        });
    });
}

if (loginForm) {
    loginForm.addEventListener('submit', (e) => {
        e.preventDefault();
        const name = document.getElementById('username').value;
        const secret = document.getElementById('password').value;
        login(name, secret);
    });
}

const btnFidoLogin = document.getElementById('btn-fido-login');
if (btnFidoLogin) {
    btnFidoLogin.addEventListener('click', (e) => {
        e.preventDefault();
        fidoLogin();
    });
}

if (logoutBtn) {
    logoutBtn.addEventListener('click', logout);
}
