/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {apiRequest} from './api.js';
import {showAlert} from './alert.js';
import {writeClipboardText} from './clipboard.js';
import {RenopDialog} from './components.js';
import {runButtonAction} from './components/button.js';
import {t} from './i18n.js';
import {formatTimestamp} from './time.js';
import {el} from '@renop/ui/dom';

const scopeKeys = Object.freeze({
    'repository:read': 'repositoryRead',
    'repository:publish': 'repositoryPublish',
    'repository:delete': 'repositoryDelete',
    'package:manage': 'packageManage',
    'domain:manage': 'domainManage',
    'messages:read': 'messagesRead',
    'account:read': 'accountRead',
    'account:write': 'accountWrite',
    'statistics:read': 'statisticsRead',
    'admin:users': 'adminUsers',
    'admin:repositories': 'adminRepositories',
    'admin:settings': 'adminSettings',
    'admin:audit': 'adminAudit',
    'admin:notifications': 'adminNotifications',
    'admin:updates': 'adminUpdates',
    'admin:statistics': 'adminStatistics',
});

let apiTokenLoadSequence = 0;
let cachedTokenCount = null;
let cachedTokenLimit = 50;
let activeAPITokenManager = null;

/**
 * Translate one stable API-token scope.
 * @param {string} scope - Stable backend scope identifier.
 * @returns {string} Localized scope label.
 */
function scopeLabel(scope) {
    const suffix = scopeKeys[scope];
    return suffix ? t(`profile.apiTokenScope.${suffix}`) : scope;
}

/**
 * Render the profile API-token summary from cached non-secret metadata.
 * @param {number} count - Current token count.
 * @param {number} limit - Account token limit.
 * @returns {void}
 */
function renderAPITokenSummary(count, limit) {
    const status = document.getElementById('profile-api-token-status');
    if (!status) return;
    status.removeAttribute('data-i18n');
    status.textContent = count === 0
        ? t('profile.apiTokensNone')
        : t('profile.apiTokensCount', {count, limit});
}

/**
 * Set a localized inline form error and retain its key for live language changes.
 * @param {HTMLElement} target - Error container.
 * @param {string} key - Translation key.
 * @returns {void}
 */
function setInlineError(target, key) {
    target.dataset.i18n = key;
    target.textContent = t(key);
}

/**
 * Load the current token metadata without exposing credential values.
 * @returns {Promise<{tokens: object[], limit: number}>}
 */
async function fetchAPITokens() {
    const response = await apiRequest('/api/auth/profile/api-tokens', {cache: 'no-store'});
    if (!response.ok) throw new Error('API token list request failed');
    const result = await response.json();
    return {
        tokens: Array.isArray(result.tokens) ? result.tokens : [],
        limit: Number(result.limit) || 50,
    };
}

/**
 * Load the scope catalog already reduced to the current account's permissions.
 * @returns {Promise<string[]>}
 */
async function fetchAllowedScopes() {
    const response = await apiRequest('/api/auth/profile/api-tokens/scopes', {cache: 'no-store'});
    if (!response.ok) throw new Error('API token scope request failed');
    const result = await response.json();
    return Array.isArray(result.scopes) ? result.scopes : [];
}

/**
 * Update the compact API-token count on the profile page.
 * @returns {Promise<void>}
 */
export async function refreshAPITokenSummary() {
    const sequence = ++apiTokenLoadSequence;
    const status = document.getElementById('profile-api-token-status');
    if (!status) return;
    try {
        const result = await fetchAPITokens();
        if (sequence !== apiTokenLoadSequence) return;
        cachedTokenCount = result.tokens.length;
        cachedTokenLimit = result.limit;
        renderAPITokenSummary(cachedTokenCount, cachedTokenLimit);
    } catch (error) {
        if (sequence !== apiTokenLoadSequence) return;
        console.error('Failed to load API token summary', error);
        status.textContent = t('profile.apiTokensLoadFailed');
    }
}

/**
 * Show a newly generated secret exactly once with shared clipboard feedback.
 * @param {string} secret - Plaintext token returned only by its create request.
 * @param {object} token - Non-secret token metadata.
 * @returns {void}
 */
function showAPITokenSecret(secret, token) {
    const code = el('code', {class: 'profile-api-token-secret'}, secret);
    const body = el('div', {class: 'profile-api-token-secret-dialog'},
        el('div', {
            class: 'profile-recovery-warning', 'data-i18n': 'profile.apiTokenSecretWarning'
        }, t('profile.apiTokenSecretWarning')),
        el('div', {class: 'profile-api-token-secret-name'}, token.name || ''),
        code
    );
    void RenopDialog.show({
        id: 'profile-api-token-secret-dialog',
        maxWidth: '680px',
        icon: 'fileKey',
        title: el('span', {'data-i18n': 'profile.apiTokenCreatedTitle'}, t('profile.apiTokenCreatedTitle')),
        subtitle: el('span', {'data-i18n': 'profile.apiTokenCreatedDesc'}, t('profile.apiTokenCreatedDesc')),
        body,
        footer: [
            {
                id: 'profile-api-token-copy',
                text: t('profile.apiTokenCopy'),
                className: 'action-btn',
                onClick: async () => {
                    try {
                        await writeClipboardText(secret);
                        showAlert(t('prompt.copied'), 'success');
                    } catch (error) {
                        console.error('Failed to copy API token', error);
                        showAlert(t('profile.apiTokenCopyFailed'), 'error');
                    }
                }
            },
            {
                id: 'profile-api-token-secret-close',
                text: t('common.close'),
                className: 'action-btn primary-btn',
                onClick: (event, dialog) => dialog.close(true)
            }
        ]
    });
}

/**
 * Create a scope checkbox using the server-approved scope catalog.
 * @param {string} scope - Stable scope identifier.
 * @returns {HTMLLabelElement}
 */
function createScopeOption(scope) {
    const input = el('input', {type: 'checkbox', value: scope, name: 'api-token-scope'});
    if (scope === 'repository:read' || scope === 'repository:publish') input.checked = true;
    return el('label', {class: 'profile-api-token-scope-option'},
        input,
        el('span', {class: 'profile-api-token-scope-copy'},
            el('strong', {'data-api-token-scope': scope}, scopeLabel(scope)),
            el('code', {}, scope)
        )
    );
}

/**
 * Open the token creation form and return through onCreated after persistence.
 * @param {string[]} allowedScopes - Scopes filtered by account permissions.
 * @param {() => Promise<void>} onCreated - Manager-list refresh callback.
 * @returns {void}
 */
function openCreateAPITokenDialog(allowedScopes, onCreated) {
    if (document.getElementById('profile-api-token-create-dialog')) return;
    const nameInput = el('input', {
        class: 'profile-input', type: 'text', maxlength: '80', autocomplete: 'off',
        placeholder: t('profile.apiTokenNamePlaceholder'),
        'data-i18n-placeholder': 'profile.apiTokenNamePlaceholder'
    });
    const expiration = el('select', {class: 'profile-input profile-api-token-expiration'});
    for (const [value, key] of [
        ['7', 'sevenDays'], ['30', 'thirtyDays'], ['90', 'ninetyDays'],
        ['365', 'oneYear'], ['0', 'never']
    ]) {
        expiration.appendChild(el('option', {
            value, 'data-i18n': `profile.apiTokenExpiry.${key}`
        }, t(`profile.apiTokenExpiry.${key}`)));
    }
    expiration.value = '30';
    const scopeGrid = el('div', {class: 'profile-api-token-scope-grid'},
        ...allowedScopes.map(createScopeOption)
    );
    const error = el('p', {class: 'password-recovery-error', role: 'alert'});
    const body = el('div', {class: 'profile-api-token-create-form'},
        el('label', {}, el('span', {'data-i18n': 'profile.apiTokenName'}, t('profile.apiTokenName')), nameInput),
        el('label', {}, el('span', {'data-i18n': 'profile.apiTokenExpiration'}, t('profile.apiTokenExpiration')), expiration),
        el('fieldset', {class: 'profile-api-token-scopes'},
            el('legend', {'data-i18n': 'profile.apiTokenScopes'}, t('profile.apiTokenScopes')),
            el('p', {
                class: 'profile-security-hint', 'data-i18n': 'profile.apiTokenScopesHint'
            }, t('profile.apiTokenScopesHint')),
            scopeGrid
        ),
        error
    );
    void RenopDialog.show({
        id: 'profile-api-token-create-dialog',
        maxWidth: '720px',
        icon: 'fileKey',
        title: el('span', {'data-i18n': 'profile.createApiToken'}, t('profile.createApiToken')),
        subtitle: el('span', {'data-i18n': 'profile.createApiTokenDesc'}, t('profile.createApiTokenDesc')),
        body,
        form: {
            id: 'profile-api-token-create-form',
            onSubmit: async (event, dialog) => {
                event.preventDefault();
                error.removeAttribute('data-i18n');
                error.textContent = '';
                const scopes = Array.from(scopeGrid.querySelectorAll('input:checked'), input => input.value);
                if (!nameInput.value.trim()) {
                    setInlineError(error, 'profile.apiTokenNameRequired');
                    nameInput.focus();
                    return;
                }
                if (scopes.length === 0) {
                    setInlineError(error, 'profile.apiTokenScopeRequired');
                    return;
                }
                const submit = dialog.querySelector('#profile-api-token-create-submit');
                await runButtonAction(submit, async () => {
                    const days = Number(expiration.value) || 0;
                    const response = await apiRequest('/api/auth/profile/api-tokens', {
                        method: 'POST',
                        headers: {'Content-Type': 'application/json'},
                        body: JSON.stringify({
                            name: nameInput.value.trim(), scopes,
                            expires_at: days > 0 ? Date.now() + days * 86400000 : null,
                        }),
                    });
                    if (!response.ok) {
                        const code = response.headers.get('X-Renop-Error-Code');
                        setInlineError(error, code === 'API_TOKEN_NAME_CONFLICT'
                            ? 'profile.apiTokenNameConflict'
                            : (code === 'API_TOKEN_LIMIT'
                                ? 'profile.apiTokenLimitReached'
                                : 'profile.apiTokenCreateFailed'));
                        return;
                    }
                    const result = await response.json();
                    if (!result.secret || !result.token) {
                        setInlineError(error, 'profile.apiTokenCreateFailed');
                        return;
                    }
                    dialog.close(true);
                    showAPITokenSecret(result.secret, result.token);
                    showAlert(t('profile.apiTokenCreatedNotice'), 'success');
                    try {
                        await onCreated();
                    } catch (reloadError) {
                        console.error('Failed to refresh API tokens after creation', reloadError);
                    }
                });
            }
        },
        footer: [
            {
                id: 'profile-api-token-create-cancel', text: t('common.cancel'), className: 'action-btn',
                onClick: (event, dialog) => dialog.close(false)
            },
            {
                id: 'profile-api-token-create-submit', text: t('common.create'),
                className: 'action-btn primary-btn', type: 'submit'
            }
        ]
    });
    requestAnimationFrame(() => nameInput.focus());
}

/**
 * Render token metadata cards and their immediate revocation actions.
 * @param {HTMLElement} list - List container.
 * @param {object[]} tokens - Non-secret token metadata.
 * @param {() => Promise<void>} reload - List refresh callback.
 * @returns {void}
 */
function renderAPITokenList(list, tokens, reload) {
    list.replaceChildren();
    if (tokens.length === 0) {
        list.appendChild(el('div', {class: 'profile-api-token-empty'}, t('profile.apiTokensNone')));
        return;
    }
    const now = Date.now();
    tokens.forEach(token => {
        const expired = Number(token.expires_at) > 0 && Number(token.expires_at) <= now;
        const scopeList = el('div', {class: 'profile-api-token-badges'},
            ...(Array.isArray(token.scopes) ? token.scopes : []).map(scope =>
                el('span', {
                    class: 'profile-api-token-scope-badge', title: scope, 'data-api-token-scope': scope
                }, scopeLabel(scope)))
        );
        const revoke = el('button', {
            type: 'button', class: 'pill-btn pill-btn--soft pill-btn--sm',
        }, t('profile.apiTokenRevoke'));
        revoke.addEventListener('click', () => {
            void runButtonAction(revoke, async () => {
                if (!(await window.showConfirm(t('profile.apiTokenRevokeConfirm', {name: token.name})))) return;
                const response = await apiRequest(`/api/auth/profile/api-tokens/${encodeURIComponent(token.id)}`, {
                    method: 'DELETE'
                });
                if (!response.ok) {
                    showAlert(t('profile.apiTokenRevokeFailed'), 'error');
                    return;
                }
                showAlert(t('profile.apiTokenRevoked'), 'success');
                await reload();
            });
        });
        const expiryText = token.expires_at
            ? t(expired ? 'profile.apiTokenExpiredAt' : 'profile.apiTokenExpiresAt', {
                date: formatTimestamp(token.expires_at, {fallback: t('common.unknown')})
            })
            : t('profile.apiTokenNeverExpires');
        list.appendChild(el('article', {class: `profile-api-token-card${expired ? ' is-expired' : ''}`},
            el('div', {class: 'profile-api-token-card-head'},
                el('div', {},
                    el('strong', {}, token.name || t('common.unknown')),
                    el('p', {class: 'profile-security-hint'},
                        t('profile.apiTokenCreatedAt', {
                            date: formatTimestamp(token.created_at, {fallback: t('common.unknown')})
                        }))
                ),
                revoke
            ),
            scopeList,
            el('p', {class: `profile-api-token-expiry${expired ? ' is-expired' : ''}`}, expiryText)
        ));
    });
}

/**
 * Open the API-token manager with bounded server-backed metadata.
 * @returns {void}
 */
function openAPITokenManager() {
    if (document.getElementById('profile-api-token-manager-dialog')) return;
    const count = el('span', {
        class: 'profile-api-token-manager-count', 'data-i18n': 'profile.apiTokensLoading'
    }, t('profile.apiTokensLoading'));
    const create = el('button', {
        type: 'button', class: 'pill-btn pill-btn--primary pill-btn--sm', disabled: true,
        'data-i18n': 'profile.createApiToken'
    }, t('profile.createApiToken'));
    const list = el('div', {class: 'profile-api-token-list'},
        el('div', {class: 'sessions-loading'}, t('profile.apiTokensLoading'))
    );
    const body = el('div', {class: 'profile-api-token-manager'},
        el('div', {class: 'profile-api-token-manager-toolbar'}, count, create),
        list
    );
    let allowedScopes = [];
    let tokenLimit = 50;
    let scopesLoaded = false;
    const managerState = {
        count, create, list, tokens: [], limit: tokenLimit, reload: null, loaded: false, loadFailed: false
    };
    /**
     * Synchronize create-button availability after either bounded request completes.
     * @returns {void}
     */
    const updateCreateAvailability = () => {
        create.disabled = managerState.loadFailed || !managerState.loaded || !scopesLoaded ||
            managerState.tokens.length >= managerState.limit || allowedScopes.length === 0;
    };
    /**
     * Reload non-secret token metadata and update the open manager.
     * @returns {Promise<void>}
     */
    const reload = async () => {
        const result = await fetchAPITokens();
        cachedTokenCount = result.tokens.length;
        cachedTokenLimit = result.limit;
        tokenLimit = result.limit;
        managerState.tokens = result.tokens;
        managerState.limit = result.limit;
        managerState.loaded = true;
        managerState.loadFailed = false;
        count.removeAttribute('data-i18n');
        count.textContent = t('profile.apiTokensCount', {count: result.tokens.length, limit: result.limit});
        updateCreateAvailability();
        renderAPITokenList(list, result.tokens, reload);
        renderAPITokenSummary(cachedTokenCount, cachedTokenLimit);
    };
    managerState.reload = reload;
    activeAPITokenManager = managerState;
    create.addEventListener('click', () => openCreateAPITokenDialog(allowedScopes, reload));
    void RenopDialog.show({
        id: 'profile-api-token-manager-dialog',
        maxWidth: '780px',
        icon: 'fileKey',
        title: el('span', {'data-i18n': 'profile.apiTokensTitle'}, t('profile.apiTokensTitle')),
        subtitle: el('span', {'data-i18n': 'profile.apiTokensManagerDesc'}, t('profile.apiTokensManagerDesc')),
        body,
        footer: [{
            id: 'profile-api-token-manager-close', text: t('common.close'), className: 'action-btn primary-btn',
            onClick: (event, dialog) => dialog.close(true)
        }]
    }).finally(() => {
        if (activeAPITokenManager === managerState) activeAPITokenManager = null;
    });
    void fetchAllowedScopes().then(scopes => {
        allowedScopes = scopes;
        scopesLoaded = true;
        updateCreateAvailability();
    }).catch(error => {
        console.error('Failed to load API token scopes', error);
        scopesLoaded = false;
        updateCreateAvailability();
    });
    void reload().catch(error => {
        console.error('Failed to load API tokens', error);
        managerState.loadFailed = true;
        list.replaceChildren(el('div', {class: 'profile-api-token-empty'}, t('profile.apiTokensLoadFailed')));
        updateCreateAvailability();
    });
}

document.getElementById('btn-manage-api-tokens')?.addEventListener('click', openAPITokenManager);

window.addEventListener('languageChanged', () => {
    if (cachedTokenCount !== null) renderAPITokenSummary(cachedTokenCount, cachedTokenLimit);
    document.querySelectorAll('[data-api-token-scope]').forEach(node => {
        node.textContent = scopeLabel(node.dataset.apiTokenScope);
    });
    const labels = {
        'profile-api-token-copy': 'profile.apiTokenCopy',
        'profile-api-token-secret-close': 'common.close',
        'profile-api-token-create-cancel': 'common.cancel',
        'profile-api-token-create-submit': 'common.create',
        'profile-api-token-manager-close': 'common.close',
    };
    Object.entries(labels).forEach(([id, key]) => {
        const node = document.getElementById(id);
        if (node) node.textContent = t(key);
    });
    if (activeAPITokenManager?.list?.isConnected && activeAPITokenManager.loaded) {
        const manager = activeAPITokenManager;
        manager.count.textContent = t('profile.apiTokensCount', {
            count: manager.tokens.length, limit: manager.limit
        });
        renderAPITokenList(manager.list, manager.tokens, manager.reload);
    }
});
