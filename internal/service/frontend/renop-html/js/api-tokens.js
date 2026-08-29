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
import {makeCustomSelect} from '@renop/ui/custom-select';
import {collapseElement, expandElement, morphElementHeight} from '@renop/ui/height-anim';
import {$} from '@renop/ui/jquery';

const scopeKeys = Object.freeze({
    'repository:read': 'repositoryRead',
    'repository:publish': 'repositoryPublish',
    'repository:delete': 'repositoryDelete',
    'package:create': 'packageCreate',
    'package:metadata': 'packageMetadata',
    'package:lifecycle': 'packageLifecycle',
    'team:manage': 'teamManage',
    'domain:read': 'domainRead',
    'domain:create': 'domainCreate',
    'domain:verify': 'domainVerify',
    'domain:delete': 'domainDelete',
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

const scopeGroups = Object.freeze([
    {key: 'repository', scopes: ['repository:read', 'repository:publish', 'repository:delete']},
    {key: 'package', scopes: ['package:create', 'package:metadata', 'package:lifecycle', 'team:manage']},
    {key: 'domain', scopes: ['domain:read', 'domain:create', 'domain:verify', 'domain:delete']},
    {key: 'account', scopes: ['messages:read', 'account:read', 'account:write', 'statistics:read']},
    {key: 'administration', scopes: [
        'admin:users', 'admin:repositories', 'admin:settings', 'admin:audit',
        'admin:notifications', 'admin:updates', 'admin:statistics'
    ]},
]);

let apiTokenLoadSequence = 0;
let cachedTokenCount = null;
let cachedTokenLimit = 50;
let activeAPITokenManager = null;
let activeAPITokenCreate = null;

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
 * @param {string} key - Translation key, or an empty string to clear the error.
 * @param {Record<string, string|number>} [params={}] - Translation parameters.
 * @returns {void}
 */
function setInlineError(target, key, params = {}) {
    if (!key) {
        delete target.dataset.apiTokenErrorKey;
        delete target.dataset.apiTokenErrorParams;
        target.textContent = '';
        return;
    }
    target.dataset.apiTokenErrorKey = key;
    target.dataset.apiTokenErrorParams = JSON.stringify(params);
    target.textContent = t(key, params);
}

/**
 * Morph a token form around a localized error update.
 * @param {HTMLElement} container - Form content whose natural height may change.
 * @param {HTMLElement} target - Error container.
 * @param {string} key - Translation key, or an empty string to clear the error.
 * @param {Record<string, string|number>} [params={}] - Translation parameters.
 * @returns {void}
 */
function morphInlineError(container, target, key, params = {}) {
    void morphElementHeight(container, () => setInlineError(target, key, params), {duration: 200});
}

/**
 * Build localized expiration choices for the shared custom-select control.
 * @returns {Array<{value: string, label: string}>}
 */
function expirationOptions() {
    return [
        ['7', 'sevenDays'], ['30', 'thirtyDays'], ['90', 'ninetyDays'],
        ['365', 'oneYear'], ['0', 'never']
    ].map(([value, key]) => ({value, label: t(`profile.apiTokenExpiry.${key}`)}));
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
 * @returns {Promise<{scopes: string[], targetKinds: Record<string, string>, targetLimit: number}>}
 */
async function fetchAllowedScopes() {
    const response = await apiRequest('/api/auth/profile/api-tokens/scopes', {cache: 'no-store'});
    if (!response.ok) throw new Error('API token scope request failed');
    const result = await response.json();
    return {
        scopes: Array.isArray(result.scopes) ? result.scopes : [],
        targetKinds: result.target_kinds && typeof result.target_kinds === 'object' ? result.target_kinds : {},
        targetLimit: Number(result.target_limit) || 128,
    };
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
 * @param {string} targetKind - Optional canonical target kind.
 * @returns {HTMLDivElement}
 */
function createScopeOption(scope, targetKind = '') {
    const input = el('input', {type: 'checkbox', value: scope, name: 'api-token-scope'});
    if (scope === 'repository:read' || scope === 'repository:publish') input.checked = true;
    const option = el('label', {class: 'profile-api-token-scope-option'}, input,
        el('span', {class: 'profile-api-token-scope-copy'},
            el('strong', {'data-api-token-scope': scope}, scopeLabel(scope)), el('code', {}, scope))
    );
    const entry = el('div', {class: 'profile-api-token-scope-entry'}, option);
    if (!targetKind) return entry;
    const targets = el('textarea', {
        class: 'profile-api-token-targets', rows: '2', maxlength: '4096',
        'data-api-token-target-for': scope,
        placeholder: t(`profile.apiTokenTargetPlaceholder.${targetKind}`),
        'data-i18n-placeholder': `profile.apiTokenTargetPlaceholder.${targetKind}`,
    });
    const targetEditor = el('label', {class: 'profile-api-token-target-editor'},
        el('span', {'data-i18n': 'profile.apiTokenTargetLimit'}, t('profile.apiTokenTargetLimit')),
        targets,
        el('small', {
            class: 'profile-security-hint', 'data-i18n': 'profile.apiTokenTargetsHint'
        }, t('profile.apiTokenTargetsHint'))
    );
    targetEditor.hidden = !input.checked;
    $(targetEditor).toggleClass('is-visible', input.checked);
    $(input).on('change', () => {
        if (input.checked) {
            void expandElement(targetEditor, {duration: 240, marginTop: ''});
        } else {
            void collapseElement(targetEditor, {duration: 210, marginTop: false});
        }
    });
    entry.appendChild(targetEditor);
    return entry;
}

/**
 * Group server-approved scopes by target capability without reordering within each group.
 * @param {string[]} allowedScopes - Scopes filtered by current account permissions.
 * @param {Record<string, string>} targetKinds - Scope-to-target-kind mapping from the server.
 * @returns {HTMLDivElement}
 */
function createScopeGroups(allowedScopes, targetKinds) {
    const allowed = new Set(allowedScopes);
    const groups = scopeGroups.map(group => {
        const scopes = group.scopes.filter(scope => allowed.has(scope));
        if (scopes.length === 0) return null;
        return el('section', {class: 'profile-api-token-scope-group'},
            el('h4', {
                class: 'profile-api-token-scope-group-title',
                'data-i18n': `profile.apiTokenScopeGroup.${group.key}`
            }, t(`profile.apiTokenScopeGroup.${group.key}`)),
            el('div', {class: 'profile-api-token-scope-grid'},
                ...scopes.map(scope => createScopeOption(scope, targetKinds[scope])))
        );
    }).filter(Boolean);
    return el('div', {class: 'profile-api-token-scope-groups'}, ...groups);
}

/**
 * Open the token creation form and return through onCreated after persistence.
 * @param {{scopes: string[], targetKinds: Record<string, string>, targetLimit: number}} catalog - Server-approved scope catalog.
 * @param {() => Promise<void>} onCreated - Manager-list refresh callback.
 * @returns {void}
 */
function openCreateAPITokenDialog(catalog, onCreated) {
    if (document.getElementById('profile-api-token-create-dialog')) return;
    const nameInput = el('input', {
        class: 'profile-input', type: 'text', maxlength: '80', autocomplete: 'off',
        placeholder: t('profile.apiTokenNamePlaceholder'),
        'data-i18n-placeholder': 'profile.apiTokenNamePlaceholder'
    });
    let expirationValue = '30';
    const expiration = makeCustomSelect(expirationOptions(), expirationValue, value => {
        expirationValue = value;
    });
    expiration.classList.add('profile-api-token-expiration');
    const scopeGrid = createScopeGroups(catalog.scopes, catalog.targetKinds);
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
        className: 'profile-api-token-create-modal',
        maxWidth: '720px',
        icon: 'fileKey',
        title: el('span', {'data-i18n': 'profile.createApiToken'}, t('profile.createApiToken')),
        subtitle: el('span', {'data-i18n': 'profile.createApiTokenDesc'}, t('profile.createApiTokenDesc')),
        body,
        form: {
            id: 'profile-api-token-create-form',
            onSubmit: async (event, dialog) => {
                event.preventDefault();
                morphInlineError(body, error, '');
                const scopes = Array.from(scopeGrid.querySelectorAll('input:checked'), input => input.value);
                if (!nameInput.value.trim()) {
                    morphInlineError(body, error, 'profile.apiTokenNameRequired');
                    nameInput.focus();
                    return;
                }
                if (scopes.length === 0) {
                    morphInlineError(body, error, 'profile.apiTokenScopeRequired');
                    return;
                }
                const targets = {};
                let targetCount = 0;
                scopes.forEach(scope => {
                    const editor = scopeGrid.querySelector(`[data-api-token-target-for="${CSS.escape(scope)}"]`);
                    if (!editor) return;
                    const values = [...new Set(editor.value.split(/[\n,]+/u).map(value => value.trim()).filter(Boolean))];
                    if (values.length > 0) {
                        targets[scope] = values;
                        targetCount += values.length;
                    }
                });
                if (targetCount > catalog.targetLimit) {
                    morphInlineError(body, error, 'profile.apiTokenTargetLimitReached', {
                        limit: catalog.targetLimit,
                    });
                    return;
                }
                const submit = dialog.querySelector('#profile-api-token-create-submit');
                await runButtonAction(submit, async () => {
                    const days = Number(expirationValue) || 0;
                    const response = await apiRequest('/api/auth/profile/api-tokens', {
                        method: 'POST',
                        headers: {'Content-Type': 'application/json'},
                        body: JSON.stringify({
                            name: nameInput.value.trim(), scopes, targets,
                            expires_at: days > 0 ? Date.now() + days * 86400000 : null,
                        }),
                    });
                    if (!response.ok) {
                        const code = response.headers.get('X-Renop-Error-Code');
                        morphInlineError(body, error, code === 'API_TOKEN_NAME_CONFLICT'
                            ? 'profile.apiTokenNameConflict'
                            : (code === 'API_TOKEN_LIMIT'
                                ? 'profile.apiTokenLimitReached'
                                : 'profile.apiTokenCreateFailed'));
                        return;
                    }
                    const result = await response.json();
                    if (!result.secret || !result.token) {
                        morphInlineError(body, error, 'profile.apiTokenCreateFailed');
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
    }).finally(() => {
        if (activeAPITokenCreate?.expiration === expiration) activeAPITokenCreate = null;
    });
    activeAPITokenCreate = {
        expiration,
        getValue: () => expirationValue,
    };
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
        const targetPolicy = token.targets && typeof token.targets === 'object'
            ? Object.entries(token.targets).filter(([, targets]) => Array.isArray(targets) && targets.length > 0)
            : [];
        const targetList = targetPolicy.length > 0
            ? el('div', {class: 'profile-api-token-target-policy'}, ...targetPolicy.map(([scope, targets]) =>
                el('div', {class: 'profile-api-token-target-policy-row'},
                    el('strong', {'data-api-token-scope': scope}, scopeLabel(scope)),
                    el('code', {}, targets.join(', ')))))
            : null;
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
            ...(targetList ? [targetList] : []),
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
    let scopeCatalog = {scopes: [], targetKinds: {}, targetLimit: 128};
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
            managerState.tokens.length >= managerState.limit || scopeCatalog.scopes.length === 0;
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
    create.addEventListener('click', () => openCreateAPITokenDialog(scopeCatalog, reload));
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
    void fetchAllowedScopes().then(catalog => {
        scopeCatalog = catalog;
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
    document.querySelectorAll('[data-api-token-error-key]').forEach(node => {
        let params = {};
        try {
            params = JSON.parse(node.dataset.apiTokenErrorParams || '{}');
        } catch {
            params = {};
        }
        node.textContent = t(node.dataset.apiTokenErrorKey, params);
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
    if (activeAPITokenCreate?.expiration?.isConnected) {
        const value = activeAPITokenCreate.getValue();
        activeAPITokenCreate.expiration.setOptions(expirationOptions(), value);
    }
});
