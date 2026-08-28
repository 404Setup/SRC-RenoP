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
import {showAlert} from './alert.js';
import {el} from '@renop/ui/dom';
import {makeCustomSelect} from '@renop/ui/custom-select';
import {smoothScrollToTop} from '@renop/ui/scroll';
import {registerTabContainer, updateTabIndicator} from '@renop/ui/tabs';
import {apiRequest, fetchProto, postProto, putProto} from './api.js';
import {buildInput, createSection, makeTagListInput} from './cfg-ui.js';
import {
    createCallout,
    createFieldRow,
    createIcon,
    createIndexCard,
    createSkeleton,
    createTab,
    createToggleRow
} from './components.js';
import {logout} from './auth.js';
import {restartApp} from './dashboard.js';
import {
    caughtErrorMessage,
    LocalizedResponseError,
    responseErrorMessage
} from './response-errors.js';
import {
    FrontendConfig,
    IndexDomainSettings,
    ProxyConfig,
    RebuildIndexRequest,
    ServerConfig,
    SettingsDomainsResponse,
    StorageConfig,
    UpdaterConfig,
} from './proto/index.js';

const DOMAIN_MESSAGE_TYPES = {
    frontend: FrontendConfig,
    server: ServerConfig,
    storage: StorageConfig,
    proxy: ProxyConfig,
    updater: UpdaterConfig,
    index: IndexDomainSettings,
};

const SERVICE_DOMAINS = Object.freeze(['server', 'github_oauth', 'proxy', 'storage']);

let currentDomain = null;
let currentConfig = null;
let initialConfig = null;
let domainsList = [];
let availableDomains = [];
let skeletonTimer = null;
let activeFetchId = 0;

/**
 * Renders a form skeleton in the settings form container, with optional enter animation.
 * @param {'next'|'prev'|'none'} [direction='next'] - Slide direction for the skeleton animation.
 * @returns {void}
 */
function renderSettingsSkeleton(direction = 'next') {
    const container = document.getElementById('settings-form-container');
    if (!container) return;
    container.classList.remove('is-content-entering', 'settings-form--enter-next', 'settings-form--enter-prev', 'settings-form--exiting-next', 'settings-form--exiting-prev');
    container.innerHTML = '';

    const skeleton = createSkeleton('form', 2);
    container.appendChild(skeleton);

    if (direction !== 'none') {
        const animClass = direction === 'prev' ? 'settings-form--enter-prev' : 'settings-form--enter-next';
        requestAnimationFrame(() => {
            container.classList.add(animClass);
        });
    }
}

/**
 * Initializes the settings page: loads available domains, renders tabs, and loads the active domain.
 * @returns {Promise<void>}
 */
export async function initSettings() {
    const container = document.getElementById('settings-form-container');
    if (container && !container.innerHTML.trim()) {
        renderSettingsSkeleton('none');
    }
    try {
        const {response, data} = await fetchProto('/api/settings/domains', SettingsDomainsResponse);
        if (response.ok && data) {
            availableDomains = Array.isArray(data.domains) ? data.domains : [];
            domainsList = availableDomains.filter(domain =>
                domain !== 'proxy' && domain !== 'storage' && domain !== 'github_oauth'
            );
            const targetDomain = (currentDomain && domainsList.includes(currentDomain)) ? currentDomain : (domainsList[0] || null);
            renderDomainTabs(domainsList, targetDomain);
            if (targetDomain) {
                await loadDomainSettings(targetDomain, 'none');
            }
        } else if (response.status === 401 || response.status === 403) {
            logout('kicked');
        }
    } catch (e) {
        console.error('Failed to load settings', e);
    }
}

/**
 * Renders settings domain tabs and wires click handlers to load each domain with slide direction.
 * @param {string[]} domains - Domain keys returned by the settings API.
 * @param {string|null} activeDomain - Currently selected domain key.
 * @returns {void}
 */
function renderDomainTabs(domains, activeDomain) {
    domainsList = domains;
    const tabsContainer = document.getElementById('settings-tabs');
    if (!tabsContainer) return;
    tabsContainer.innerHTML = '';

    domains.forEach((domain, idx) => {
        const a = createTab(domainLabel(domain), {
            active: domain === activeDomain,
            onClick: (e) => {
                e.preventDefault();
                if (a.classList.contains('active')) return;

                const oldIndex = domainsList.indexOf(currentDomain);
                const direction = (oldIndex !== -1 && idx < oldIndex) ? 'prev' : 'next';

                Array.from(tabsContainer.querySelectorAll('.tab')).forEach(child => child.classList.remove('active'));
                a.classList.add('active');

                updateTabIndicator(tabsContainer);

                if (window.scrollY > 0) {
                    smoothScrollToTop();
                }

                loadDomainSettings(domain, direction);
            }
        });
        tabsContainer.appendChild(a);
    });

    const activeTab = tabsContainer.querySelector('.tab.active') || tabsContainer.querySelector('.tab');
    if (activeTab) {
        activeTab.classList.add('active');
        requestAnimationFrame(() => {
            updateTabIndicator(tabsContainer);
        });
    }

    registerTabContainer(tabsContainer);
}

/**
 * Returns a localized display label for a settings domain key.
 * @param {string} domain - Visible domain key (frontend, server, updater, index).
 * @returns {string} Localized or title-cased label.
 */
function domainLabel(domain) {
    const labels = {
        frontend: t('settings.domainFrontend'),
        server: t('settings.domainServer'),
        proxy: t('settings.domainProxy'),
        storage: t('settings.domainStorage'),
        updater: t('settings.domainUpdater'),
        index: t('settings.domainIndex'),
    };
    return labels[domain] || domain.charAt(0).toUpperCase() + domain.slice(1);
}

/**
 * Load the write-only GitHub OAuth settings JSON view.
 * @returns {Promise<{response: Response, data: object|null}>}
 */
async function fetchGitHubOAuthSettings() {
    const response = await apiRequest('/api/settings/github-oauth');
    return {
        response,
        data: response.ok ? await response.json() : null,
    };
}

/**
 * Loads configuration for a settings domain and renders its form with transition animation.
 * Uses a fetch id so stale responses are ignored when the user switches tabs quickly.
 * @param {string} domain - Domain key to load.
 * @param {'next'|'prev'|'none'} [direction='next'] - Form transition direction.
 * @returns {Promise<void>}
 */
async function loadDomainSettings(domain, direction = 'next') {
    const fetchId = ++activeFetchId;
    currentDomain = domain;
    const tabsContainer = document.getElementById('settings-tabs');
    if (tabsContainer) {
        tabsContainer.querySelectorAll('.tab').forEach(tab => tab.classList.remove('is-loading'));
        const activeTab = tabsContainer.querySelector('.tab.active');
        if (activeTab) activeTab.classList.add('is-loading');
    }

    const container = document.getElementById('settings-form-container');

    if (container && container.firstElementChild && direction !== 'none') {
        container.classList.remove('settings-form--enter-next', 'settings-form--enter-prev', 'is-content-entering');
        container.classList.add(direction === 'prev' ? 'settings-form--exiting-prev' : 'settings-form--exiting-next');
    }

    if (skeletonTimer) clearTimeout(skeletonTimer);
    skeletonTimer = setTimeout(() => {
        if (activeFetchId === fetchId) {
            renderSettingsSkeleton(direction);
        }
    }, 90);

    try {
        let response;
        let data;
        if (domain === 'server') {
            const serviceDomains = SERVICE_DOMAINS.filter(name => availableDomains.includes(name));
            const results = await Promise.all(serviceDomains.map(async name => ({
                name,
                result: name === 'github_oauth'
                    ? await fetchGitHubOAuthSettings()
                    : await fetchProto(`/api/settings/domain/${name}`, DOMAIN_MESSAGE_TYPES[name])
            })));
            const denied = results.find(({result}) => result.response.status === 401 || result.response.status === 403);
            if (denied) {
                logout('kicked');
                return;
            }
            const failed = results.find(({result}) => !result.response.ok || !result.data);
            if (failed) {
                throw new Error(`Failed to load ${failed.name} settings`);
            }
            response = {ok: true};
            data = Object.fromEntries(results.map(({name, result}) => [name, result.data]));
        } else {
            const MessageType = DOMAIN_MESSAGE_TYPES[domain];
            if (!MessageType) {
                console.error('Unknown settings domain', domain);
                return;
            }
            ({response, data} = await fetchProto(`/api/settings/domain/${domain}`, MessageType));
        }

        if (activeFetchId !== fetchId) return;

        if (skeletonTimer) clearTimeout(skeletonTimer);

        if (response.ok && data) {
            currentConfig = data;
            initialConfig = JSON.parse(JSON.stringify(data));

            if (activeFetchId !== fetchId) return;

            renderSettingsForm(domain, data);

            if (container) {
                requestAnimationFrame(() => {
                    if (activeFetchId !== fetchId) return;
                    container.classList.remove('settings-form--exiting-next', 'settings-form--exiting-prev', 'settings-form--enter-next', 'settings-form--enter-prev', 'is-content-entering');
                    // Force browser reflow to restart CSS keyframe animation
                    void container.offsetWidth;

                    if (direction === 'none') {
                        container.classList.add('is-content-entering');
                    } else {
                        const animClass = direction === 'prev' ? 'settings-form--enter-prev' : 'settings-form--enter-next';
                        container.classList.add(animClass);
                    }
                });
            }
            const saveBtn = document.getElementById('settings-save-btn');
            if (saveBtn) saveBtn.disabled = true;
        } else if (response.status === 401 || response.status === 403) {
            logout('kicked');
        }
    } catch (e) {
        console.error('Failed to load domain settings', e);
    } finally {
        if (activeFetchId === fetchId && tabsContainer) {
            tabsContainer.querySelectorAll('.tab').forEach(tab => tab.classList.remove('is-loading'));
        }
    }
}

/**
 * Enables the settings save button after a configuration field changes.
 * @returns {void}
 */
function enableSave() {
    const btn = document.getElementById('settings-save-btn');
    if (btn) btn.disabled = false;
}

/**
 * Dispatches rendering of the settings form for the given domain.
 * @param {string} domain - Visible domain key (frontend, server, updater, index).
 * @param {object} data - Domain configuration object from the API.
 * @returns {void}
 */
function renderSettingsForm(domain, data) {
    const container = document.getElementById('settings-form-container');
    if (!container) return;
    container.innerHTML = '';

    if (domain === 'frontend') {
        renderFrontendSettings(container, data);
    } else if (domain === 'server') {
        renderServiceSettings(container, data);
    } else if (domain === 'proxy') {
        renderProxySettings(container, data);
    } else if (domain === 'storage') {
        renderStorageSettings(container, data);
    } else if (domain === 'updater') {
        renderUpdaterSettings(container, data);
    } else if (domain === 'index') {
        renderIndexSettings(container);
    }
}

const MAX_GLOBAL_PROXIES = 16;

/**
 * Builds dropdown options for direct routing and all named global proxies.
 * @param {Array<{name?: string}>} proxies - Editable proxy records.
 * @returns {Array<{value: string, label: string}>} Select options.
 */
function globalProxyOptions(proxies) {
    const options = [{value: '', label: t('settings.proxyDirect')}];
    for (const proxy of proxies) {
        const name = String(proxy?.name || '').trim();
        if (name) options.push({value: name, label: name});
    }
    return options;
}

/**
 * Finds the next unused stable name for a newly added proxy.
 * @param {Array<{name?: string}>} proxies - Existing proxy records.
 * @returns {string} A unique `proxy-N` name.
 */
function nextGlobalProxyName(proxies) {
    const names = new Set(proxies.map(proxy => String(proxy?.name || '').trim().toLowerCase()));
    for (let number = 1; number <= MAX_GLOBAL_PROXIES + 1; number++) {
        const candidate = `proxy-${number}`;
        if (!names.has(candidate)) return candidate;
    }
    return `proxy-${Date.now()}`;
}

/**
 * Creates a compact labeled control for one proxy property.
 * @param {string} label - Localized field label.
 * @param {HTMLElement} control - Input control.
 * @param {boolean} [wide=false] - Whether the field spans the editor width.
 * @returns {HTMLLabelElement} Labeled proxy field.
 */
function createGlobalProxyField(label, control, wide = false) {
    return el('label', {class: `global-proxy-field${wide ? ' global-proxy-field--wide' : ''}`},
        el('span', {class: 'global-proxy-field-label'}, label),
        control
    );
}

/**
 * Renders global proxy selection and the editable named proxy list.
 * @param {HTMLElement} container - Settings form container.
 * @param {{selected?: string, proxies?: Array<object>}} data - ProxyConfig fields.
 * @returns {void}
 */
function renderProxySettings(container, data) {
    const currentConfig = data;
    const proxies = Array.isArray(data.proxies) ? data.proxies : [];
    currentConfig.proxies = proxies;
    currentConfig.selected = data.selected || '';
    let selectedProxy = proxies.find(proxy => proxy.name === currentConfig.selected) || null;

    const wrap = el('div', {class: 'cfg-layout'});
    const routingSection = createSection(
        createIcon('network'),
        t('settings.proxyRoutingTitle'),
        t('settings.proxyRoutingSubtitle'),
        {defaultCollapsed: true}
    );
    const routingFields = routingSection.querySelector('.cfg-fields');
    const activeSelect = makeCustomSelect(globalProxyOptions(proxies), currentConfig.selected, value => {
        currentConfig.selected = value;
        selectedProxy = proxies.find(proxy => proxy.name === value) || null;
        enableSave();
    });
    routingFields.appendChild(createFieldRow(
        t('settings.proxyActive'),
        t('settings.proxyActiveHint'),
        activeSelect
    ));

    const proxiesSection = createSection(
        createIcon('network'),
        t('settings.proxyListTitle'),
        t('settings.proxyListSubtitle'),
        {defaultCollapsed: true}
    );
    const list = el('div', {class: 'global-proxy-list'});
    const addButton = el('button', {
        type: 'button',
        class: 'pill-btn pill-btn--soft pill-btn--sm',
        title: t('settings.proxyAdd'),
        ariaLabel: t('settings.proxyAdd')
    }, createIcon('plus'), el('span', {}, t('settings.proxyAdd')));
    const sectionHeader = proxiesSection.querySelector('.cfg-section-header');
    const sectionChevron = proxiesSection.querySelector('.cfg-section-chevron');
    sectionHeader?.insertBefore(addButton, sectionChevron || null);

    /**
     * Refreshes the active proxy dropdown after list or name changes.
     * @returns {void}
     */
    function refreshActiveSelect() {
        currentConfig.selected = selectedProxy ? String(selectedProxy.name || '').trim() : '';
        activeSelect.setOptions(globalProxyOptions(proxies), currentConfig.selected);
        activeSelect.setValue(currentConfig.selected);
    }

    /**
     * Removes one proxy after confirmation and animates the list update.
     * @param {number} index - Proxy index in the editable array.
     * @param {HTMLElement} entry - Rendered proxy entry.
     * @returns {Promise<void>}
     */
    async function removeProxy(index, entry) {
        const proxy = proxies[index];
        if (!proxy) return;
        const confirmed = await window.showConfirm(t('settings.proxyConfirmDelete', {
            name: proxy.name || t('settings.proxyUnnamed')
        }));
        if (!confirmed) return;
        entry.classList.add('global-proxy-entry--leaving');
        setTimeout(() => {
            if (selectedProxy === proxy) selectedProxy = null;
            proxies.splice(index, 1);
            refreshActiveSelect();
            renderProxyList();
            enableSave();
        }, 180);
    }

    /**
     * Re-renders proxy editors from the current mutable configuration.
     * @returns {void}
     */
    function renderProxyList() {
        list.innerHTML = '';
        addButton.disabled = proxies.length >= MAX_GLOBAL_PROXIES;
        if (proxies.length === 0) {
            list.appendChild(el('div', {class: 'global-proxy-empty'},
                createIcon('network'),
                el('span', {}, t('settings.proxyEmpty'))
            ));
            return;
        }

        proxies.forEach((proxy, index) => {
            const title = el('span', {class: 'global-proxy-entry-name'}, proxy.name || t('settings.proxyUnnamed'));
            const endpoint = el('span', {class: 'global-proxy-entry-url'}, proxy.url || t('settings.proxyUrlMissing'));
            const deleteButton = el('button', {
                type: 'button',
                class: 'global-proxy-delete-btn',
                title: t('settings.proxyDelete'),
                ariaLabel: t('settings.proxyDelete')
            }, createIcon('delete'));
            const entry = el('div', {class: 'global-proxy-entry global-proxy-entry--entering'});
            const header = el('div', {class: 'global-proxy-entry-header'},
                el('div', {class: 'global-proxy-entry-meta'}, title, endpoint),
                deleteButton
            );
            const fields = el('div', {class: 'global-proxy-editor-grid'});

            const nameInput = buildInput('text', proxy.name, 'proxy-1', event => {
                proxy.name = event.target.value;
                title.textContent = proxy.name || t('settings.proxyUnnamed');
                refreshActiveSelect();
                enableSave();
            });
            const urlInput = buildInput('text', proxy.url, 'http://127.0.0.1:8080', event => {
                proxy.url = event.target.value;
                endpoint.textContent = proxy.url || t('settings.proxyUrlMissing');
                enableSave();
            });
            const usernameInput = buildInput('text', proxy.username, t('settings.proxyUsernamePlaceholder'), event => {
                proxy.username = event.target.value;
                enableSave();
            });
            const passwordInput = buildInput('password', proxy.password, t('settings.proxyPasswordPlaceholder'), event => {
                proxy.password = event.target.value;
                enableSave();
            });

            fields.appendChild(createGlobalProxyField(t('settings.proxyName'), nameInput));
            fields.appendChild(createGlobalProxyField(t('settings.proxyUrl'), urlInput, true));
            fields.appendChild(createGlobalProxyField(t('settings.proxyUsername'), usernameInput));
            fields.appendChild(createGlobalProxyField(t('settings.proxyPassword'), passwordInput));
            deleteButton.addEventListener('click', () => removeProxy(index, entry));
            entry.appendChild(header);
            entry.appendChild(fields);
            list.appendChild(entry);
        });
    }

    addButton.addEventListener('click', () => {
        if (proxies.length >= MAX_GLOBAL_PROXIES) return;
        proxies.push({
            name: nextGlobalProxyName(proxies),
            url: '',
            username: '',
            password: ''
        });
        renderProxyList();
        refreshActiveSelect();
        enableSave();
        list.lastElementChild?.querySelector('input')?.focus();
    });

    renderProxyList();
    proxiesSection.querySelector('.cfg-fields')?.appendChild(list);
    wrap.appendChild(routingSection);
    wrap.appendChild(proxiesSection);
    container.appendChild(wrap);
}

/**
 * Render server, outbound proxy, and storage configuration as one service domain.
 * @param {HTMLElement} container - Settings form container.
 * @param {{server?: object, proxy?: object, storage?: object}} data - Service configuration groups.
 * @returns {void}
 */
function renderServiceSettings(container, data) {
    const stack = el('div', {class: 'cfg-service-stack'});
    if (data.server) renderServerSettings(stack, data.server);
    if (data.github_oauth) renderGitHubOAuthSettings(stack, data.github_oauth);
    if (data.proxy) renderProxySettings(stack, data.proxy);
    if (data.storage) renderStorageSettings(stack, data.storage);
    container.appendChild(stack);
}

/**
 * Render administrator-managed GitHub OAuth credentials without reading the stored secret.
 * @param {HTMLElement} container - Settings form container.
 * @param {object} data - Write-only GitHub OAuth settings view.
 * @returns {void}
 */
function renderGitHubOAuthSettings(container, data) {
    const wrap = el('div', {class: 'cfg-layout'});
    const section = createSection(
        createIcon('user'),
        t('settings.githubOAuthTitle'),
        t('settings.githubOAuthSubtitle'),
        {defaultCollapsed: true}
    );
    const fields = section.querySelector('.cfg-fields');
    fields.appendChild(createToggleRow(
        t('settings.githubOAuthEnabled'),
        t('settings.githubOAuthEnabledHint'),
        data.enabled === true,
        checked => {
            data.enabled = checked;
            enableSave();
        }
    ));
    const clientID = buildInput('text', data.client_id || '', 'Iv1.…', event => {
        data.client_id = event.target.value;
        enableSave();
    });
    clientID.autocomplete = 'off';
    fields.appendChild(createFieldRow(
        t('settings.githubOAuthClientId'),
        t('settings.githubOAuthClientIdHint'),
        clientID
    ));
    const clientSecret = buildInput('password', '', t('settings.githubOAuthSecretPlaceholder'), event => {
        data.client_secret = event.target.value;
        enableSave();
    });
    clientSecret.id = 'settings-github-oauth-secret';
    clientSecret.autocomplete = 'new-password';
    fields.appendChild(createFieldRow(
        t('settings.githubOAuthClientSecret'),
        data.client_secret_configured
            ? t('settings.githubOAuthSecretKeepHint')
            : t('settings.githubOAuthSecretRequiredHint'),
        clientSecret
    ));
    const callback = buildInput('url', data.callback_url || '', 'https://repo.example/api/auth/github/callback', event => {
        data.callback_url = event.target.value;
        enableSave();
    });
    fields.appendChild(createFieldRow(
        t('settings.githubOAuthCallback'),
        t('settings.githubOAuthCallbackHint'),
        callback
    ));
    section.appendChild(createCallout(
        data.client_secret_configured ? 'success' : 'warning',
        data.client_secret_configured
            ? t('settings.githubOAuthConfigured')
            : t('settings.githubOAuthNotConfigured'),
        data.client_secret_configured ? 'success' : 'warning'
    ));
    wrap.appendChild(section);
    container.appendChild(wrap);
}

/**
 * Renders updater domain settings (channel and mode selects).
 * @param {HTMLElement} container - Form container element.
 * @param {object} data - UpdaterConfig fields (channel, mode, etc.).
 * @returns {void}
 */
function renderUpdaterSettings(container, data) {
    const wrap = el('div', {class: 'cfg-layout'});

    const updaterSection = createSection(
        createIcon('updater'),
        t('settings.updaterTitle'),
        t('settings.updaterSubtitle')
    );
    const fields = updaterSection.querySelector('.cfg-fields');

    const channelOptions = [
        {value: 'release', label: t('settings.channelRelease')},
        {value: 'nightly', label: t('settings.channelNightly')}
    ];
    const channelSelect = makeCustomSelect(channelOptions, data.channel || 'release', val => {
        currentConfig.channel = val;
        enableSave();
    });
    fields.appendChild(createFieldRow(t('settings.updateChannel'), t('settings.updateChannelHint'), channelSelect));

    const modeOptions = [
        {value: 'manual', label: t('settings.modeManual')},
        {value: 'auto_check', label: t('settings.modeAutoCheck')},
        {value: 'auto_install', label: t('settings.modeAutoInstall')},
        {value: 'safe_install', label: t('settings.modeSafeInstall')}
    ];
    const modeSelect = makeCustomSelect(modeOptions, data.mode || 'manual', val => {
        currentConfig.mode = val;
        enableSave();
    });
    fields.appendChild(createFieldRow(t('settings.updateMode'), t('settings.updateModeHint'), modeSelect));

    wrap.appendChild(updaterSection);
    container.appendChild(wrap);
}

/**
 * Renders frontend domain settings (identity, branding, compliance).
 * @param {HTMLElement} container - Form container element.
 * @param {object} data - FrontendConfig fields.
 * @returns {void}
 */
function renderFrontendSettings(container, data) {
    const wrap = el('div', {class: 'cfg-layout'});

    const identitySection = createSection(
        createIcon('identity'),
        t('settings.identity'),
        t('settings.identityDesc')
    );
    const identityFields = identitySection.querySelector('.cfg-fields');

    const idInput = buildInput('text', data.id, 'e.g. my-renop', e => {
        currentConfig.id = e.target.value;
        enableSave();
    });
    identityFields.appendChild(createFieldRow(t('settings.instanceId'), t('settings.instanceIdHint'), idInput));

    const titleInput = buildInput('text', data.title, 'e.g. My Maven Repository', e => {
        currentConfig.title = e.target.value;
        enableSave();
    });
    identityFields.appendChild(createFieldRow(t('settings.titleLabel'), t('settings.titleHint'), titleInput));

    const descInput = buildInput('text', data.description, t('settings.descHint'), e => {
        currentConfig.description = e.target.value;
        enableSave();
    });
    identityFields.appendChild(createFieldRow(t('settings.descLabel'), t('settings.descHint'), descInput));

    const brandSection = createSection(
        createIcon('branding'),
        t('settings.branding'),
        t('settings.brandingDesc')
    );
    const brandFields = brandSection.querySelector('.cfg-fields');

    const orgWebInput = buildInput('text', data.organization_website, 'https://example.com', e => {
        currentConfig.organization_website = e.target.value;
        enableSave();
    });
    brandFields.appendChild(createFieldRow(t('settings.orgWeb'), t('settings.orgWebHint'), orgWebInput));

    const orgLogoInput = buildInput('text', data.organization_logo, 'https://example.com/logo.png', e => {
        currentConfig.organization_logo = e.target.value;
        enableSave();
    });
    brandFields.appendChild(createFieldRow(t('settings.orgLogo'), t('settings.orgLogoHint'), orgLogoInput));

    const bgInput = buildInput('text', data.background_url, 'https://example.com/bg.jpg', e => {
        currentConfig.background_url = e.target.value;
        enableSave();
    });
    brandFields.appendChild(createFieldRow(t('settings.bgUrl'), t('settings.bgUrlHint'), bgInput));

    const typographySection = createSection(
        createIcon('fileFont'),
        t('settings.typography'),
        t('settings.typographyDesc')
    );
    const typographyFields = typographySection.querySelector('.cfg-fields');
    const fontOptions = [
        {value: 'system', label: t('settings.fontSystem')},
        {value: 'inter', label: t('settings.fontInter')},
        {value: 'noto_sans', label: t('settings.fontNotoSans')},
        {value: 'open_sans', label: t('settings.fontOpenSans')},
        {value: 'source_sans', label: t('settings.fontSourceSans')},
        {value: 'custom', label: t('settings.fontCustom')}
    ];
    currentConfig.font_preset = data.font_preset || 'system';
    currentConfig.font_url = data.font_url || '';
    const fontUrlInput = buildInput('url', currentConfig.font_url, 'https://example.com/font.woff2', e => {
        currentConfig.font_url = e.target.value;
        enableSave();
    });
    const fontUrlRow = createFieldRow(t('settings.fontUrl'), t('settings.fontUrlHint'), fontUrlInput);
    /**
     * Shows the resource URL only for the custom webfont mode.
     * @returns {void}
     */
    function updateFontUrlVisibility() {
        const custom = currentConfig.font_preset === 'custom';
        fontUrlRow.hidden = !custom;
        fontUrlInput.required = custom;
    }
    const fontSelect = makeCustomSelect(fontOptions, currentConfig.font_preset, value => {
        currentConfig.font_preset = value;
        updateFontUrlVisibility();
        enableSave();
    });
    typographyFields.appendChild(createFieldRow(
        t('settings.fontPreset'),
        t('settings.fontPresetHint'),
        fontSelect
    ));
    typographyFields.appendChild(fontUrlRow);
    typographySection.appendChild(createCallout('neutral', t('settings.fontApplyHint'), 'info'));
    updateFontUrlVisibility();

    const complianceSection = createSection(
        createIcon('compliance'),
        t('settings.compliance'),
        t('settings.complianceDesc')
    );
    const complianceFields = complianceSection.querySelector('.cfg-fields');

    const icpInput = buildInput('text', data.icp_license, 'e.g. 京ICP备XXXXXXXX号', e => {
        currentConfig.icp_license = e.target.value;
        enableSave();
    });
    complianceFields.appendChild(createFieldRow(t('settings.icpLicense'), t('settings.icpLicenseHint'), icpInput));

    const publicSecurityFilingInput = buildInput('text', data.public_security_filing, 'e.g. 京公网安备XXXXXXXXXXXXXX号', e => {
        currentConfig.public_security_filing = e.target.value;
        enableSave();
    });
    complianceFields.appendChild(createFieldRow(
        t('settings.publicSecurityFiling'),
        t('settings.publicSecurityFilingHint'),
        publicSecurityFilingInput
    ));

    const legalNoticeInput = buildInput('url', data.legal_notice_url, 'https://example.com/legal', e => {
        currentConfig.legal_notice_url = e.target.value;
        enableSave();
    });
    complianceFields.appendChild(createFieldRow(t('settings.legalNotice'), t('settings.legalNoticeHint'), legalNoticeInput));

    wrap.appendChild(identitySection);
    wrap.appendChild(brandSection);
    wrap.appendChild(typographySection);
    wrap.appendChild(complianceSection);
    container.appendChild(wrap);
}

/**
 * Renders server domain settings (TLS/SSL, performance, network).
 * @param {HTMLElement} container - Form container element.
 * @param {object} data - ServerConfig fields.
 * @returns {void}
 */
function renderServerSettings(container, data) {
    const currentConfig = data;
    const wrap = el('div', {class: 'cfg-layout'});

    const sslSection = createSection(
        createIcon('ssl'),
        t('settings.tlsSsl'),
        t('settings.tlsSslDesc')
    );
    const sslFields = sslSection.querySelector('.cfg-fields');

    sslFields.appendChild(createToggleRow(
        t('settings.enableSsl'),
        t('settings.enableSslDesc'),
        data.ssl_enabled === true,
        checked => {
            currentConfig.ssl_enabled = checked;
            enableSave();
        }
    ));

    const certInput = buildInput('text', data.ssl_cert_path, '/path/to/cert.pem', e => {
        currentConfig.ssl_cert_path = e.target.value;
        enableSave();
    });
    sslFields.appendChild(createFieldRow(t('settings.certPath'), t('settings.certPathHint'), certInput));

    const keyInput = buildInput('text', data.ssl_key_path, '/path/to/key.pem', e => {
        currentConfig.ssl_key_path = e.target.value;
        enableSave();
    });
    sslFields.appendChild(createFieldRow(t('settings.keyPath'), t('settings.keyPathHint'), keyInput));

    const perfSection = createSection(
        createIcon('performance'),
        t('settings.performance'),
        t('settings.performanceDesc')
    );
    const perfFields = perfSection.querySelector('.cfg-fields');

    perfFields.appendChild(createToggleRow(
        t('settings.enableCompression'),
        t('settings.enableCompressionDesc'),
        data.enable_compression === true,
        checked => {
            currentConfig.enable_compression = checked;
            enableSave();
        }
    ));

    const cacheInput = buildInput('number', data.file_cache_size_mb, '128', e => {
        const n = Number(e.target.value);
        if (!Number.isFinite(n) || n < 0) return;
        currentConfig.file_cache_size_mb = Math.trunc(n);
        enableSave();
    });
    perfFields.appendChild(createFieldRow(t('settings.fileCacheSize'), t('settings.fileCacheSizeHint'), cacheInput));

    const maxReqInput = buildInput('number', data.max_active_requests, '100', e => {
        const n = Number(e.target.value);
        if (!Number.isFinite(n) || n < 1) return;
        currentConfig.max_active_requests = Math.trunc(n);
        enableSave();
    });
    perfFields.appendChild(createFieldRow(t('settings.maxActiveReq'), t('settings.maxActiveReqHint'), maxReqInput));

    const debugSection = createSection(
        createIcon('performance'),
        t('settings.debugTitle'),
        t('settings.debugSubtitle')
    );
    const debugFields = debugSection.querySelector('.cfg-fields');
    debugFields.appendChild(createToggleRow(
        t('settings.debugMode'),
        t('settings.debugModeDesc'),
        data.debug_mode === true,
        checked => {
            currentConfig.debug_mode = checked;
            enableSave();
        }
    ));
    debugSection.appendChild(createCallout('warning', t('settings.debugModeRestart'), 'warning'));

    const netSection = createSection(
        createIcon('network'),
        t('settings.network'),
        t('settings.networkDesc')
    );
    const netFields = netSection.querySelector('.cfg-fields');

    const hostInput = buildInput('text', data.host || '0.0.0.0', '0.0.0.0', e => {
        currentConfig.host = e.target.value;
        enableSave();
    });
    netFields.appendChild(createFieldRow(t('settings.serverHost'), t('settings.serverHostHint'), hostInput));

    const portInput = buildInput('number', data.port || 3000, '3000', e => {
        const n = Number(e.target.value);
        if (!Number.isFinite(n) || n < 1 || n > 65535) return;
        currentConfig.port = Math.trunc(n);
        enableSave();
    });
    netFields.appendChild(createFieldRow(t('settings.serverPort'), t('settings.serverPortHint'), portInput));

    const domainsValue = Array.isArray(data.domains) ? data.domains.join(', ') : '';
    const domainsInput = buildInput('text', domainsValue, 'e.g. mvnc.pkg.one, repo.example.com', e => {
        const raw = e.target.value.trim();
        currentConfig.domains = raw
            ? raw.split(',').map(s => s.trim()).filter(Boolean)
            : [];
        enableSave();
    });
    netFields.appendChild(createFieldRow(t('settings.domains'), t('settings.domainsHint'), domainsInput));

    const corsValue = Array.isArray(data.cors_origins) ? data.cors_origins.join(', ') : '';
    const corsInput = buildInput('text', corsValue, 'e.g. *.pkg.one, https://app.example.com, *', e => {
        const raw = e.target.value.trim();
        currentConfig.cors_origins = raw
            ? raw.split(',').map(s => s.trim()).filter(Boolean)
            : [];
        enableSave();
    });
    netFields.appendChild(createFieldRow(t('settings.corsOrigins'), t('settings.corsOriginsHint'), corsInput));

    const cdnInput = buildInput('text', data.cdn_ip_header, 'e.g. CF-Connecting-IP', e => {
        currentConfig.cdn_ip_header = e.target.value;
        enableSave();
    });
    netFields.appendChild(createFieldRow(t('settings.cdnIpHeader'), t('settings.cdnIpHeaderHint'), cdnInput));

    const proxiesValue = Array.isArray(data.trusted_proxies) ? data.trusted_proxies.join(', ') : '';
    const proxiesInput = buildInput('text', proxiesValue, 'e.g. 10.0.0.0/8, 172.16.0.1', e => {
        const raw = e.target.value.trim();
        currentConfig.trusted_proxies = raw
            ? raw.split(',').map(s => s.trim()).filter(Boolean)
            : [];
        enableSave();
    });
    netFields.appendChild(createFieldRow(t('settings.trustedProxies'), t('settings.trustedProxiesHint'), proxiesInput));

    wrap.appendChild(sslSection);
    wrap.appendChild(perfSection);
    wrap.appendChild(debugSection);
    wrap.appendChild(netSection);

    const gpgConfig = data.gpg || currentConfig.gpg || {key_servers: []};
    currentConfig.gpg = gpgConfig;
    const gpgSection = createSection(
        createIcon('fileKey'),
        t('settings.gpgTitle'),
        t('settings.gpgSubtitle'),
        {defaultCollapsed: true}
    );
    const gpgFields = gpgSection.querySelector('.cfg-fields');
    const keyServers = makeTagListInput({
        items: Array.isArray(gpgConfig.key_servers) ? gpgConfig.key_servers : [],
        type: 'allow',
        placeholder: 'https://keyserver.example',
        emptyText: t('settings.gpgKeyServersEmpty'),
        onChange: items => {
            currentConfig.gpg.key_servers = [...items];
            enableSave();
        }
    });
    gpgFields.appendChild(createFieldRow(
        t('settings.gpgKeyServers'),
        t('settings.gpgKeyServersHint'),
        keyServers
    ));
    wrap.appendChild(gpgSection);

    if (!currentConfig.audit_log) {
        currentConfig.audit_log = data.audit_log || {
            retention_days: 14,
            max_rows: 10000,
        };
    }

    const auditSection = createSection(
        createIcon('fileText'),
        t('settings.auditLogTitle') || 'Activity Log Settings',
        t('settings.auditLogSubtitle') || 'Configure log retention duration and maximum row limits.'
    );
    const auditFields = auditSection.querySelector('.cfg-fields');

    const retentionInput = buildInput('number', currentConfig.audit_log.retention_days || 14, '14', e => {
        const n = Number(e.target.value);
        if (!Number.isFinite(n) || n < 1) return;
        currentConfig.audit_log.retention_days = Math.trunc(n);
        enableSave();
    });
    auditFields.appendChild(createFieldRow(t('settings.retentionDays') || 'Retention Days', t('settings.retentionDaysHint') || 'Number of days to keep activity logs (default: 14)', retentionInput));

    const maxRowsInput = buildInput('number', currentConfig.audit_log.max_rows || 10000, '10000', e => {
        const n = Number(e.target.value);
        if (!Number.isFinite(n) || n < 100) return;
        currentConfig.audit_log.max_rows = Math.trunc(n);
        enableSave();
    });
    auditFields.appendChild(createFieldRow(t('settings.maxRows') || 'Max Log Entries', t('settings.maxRowsHint') || 'Maximum number of activity log records to retain (default: 10000)', maxRowsInput));

    wrap.appendChild(auditSection);

    if (!currentConfig.database) {
        currentConfig.database = data.database || {
            driver: 'sqlite3',
            dsn: 'renop.db',
            max_open_conns: 25,
            max_idle_conns: 25,
            conn_max_lifetime_sec: 300,
        };
    }

    const dbSection = createSection(
        createIcon('storage'),
        t('settings.databaseTitle'),
        t('settings.databaseSubtitle')
    );
    const dbFields = dbSection.querySelector('.cfg-fields');


    const driverOptions = [
        {value: 'sqlite3', label: 'SQLite (sqlite3)'},
        {value: 'mysql', label: 'MySQL'},
        {value: 'postgres', label: 'PostgreSQL'},
        {value: 'clickhouse', label: 'ClickHouse'}
    ];

    const dsnContainer = el('div', {class: 'cfg-dsn-container'});
    let dsnAnimTimer1 = null;
    let dsnAnimTimer2 = null;

    function buildDsnFields(driver) {
        const fragment = document.createDocumentFragment();
        if (driver === 'clickhouse' || driver === 'ch') {
            const clickHouseParts = parseClickHouseDsn(currentConfig.database.dsn);
            const updateClickHouse = () => {
                currentConfig.database.dsn = formatClickHouseDsn(clickHouseParts);
                enableSave();
            };
            const hostInput = buildInput('text', clickHouseParts.host, '127.0.0.1', event => {
                clickHouseParts.host = event.target.value;
                updateClickHouse();
            });
            fragment.appendChild(createFieldRow(t('settings.dbHost'), t('settings.dbHostHint'), hostInput));
            const portInput = buildInput('number', clickHouseParts.port, '9000', event => {
                clickHouseParts.port = event.target.value;
                updateClickHouse();
            });
            fragment.appendChild(createFieldRow(t('settings.dbPort'), t('settings.dbPortHint'), portInput));
            const userInput = buildInput('text', clickHouseParts.user, 'default', event => {
                clickHouseParts.user = event.target.value;
                updateClickHouse();
            });
            fragment.appendChild(createFieldRow(t('settings.dbUser'), t('settings.dbUserHint'), userInput));
            const passInput = buildInput('password', clickHouseParts.password, '••••••••', event => {
                clickHouseParts.password = event.target.value;
                updateClickHouse();
            });
            fragment.appendChild(createFieldRow(t('settings.dbPassword'), t('settings.dbPasswordHint'), passInput));
            const dbNameInput = buildInput('text', clickHouseParts.database, 'default', event => {
                clickHouseParts.database = event.target.value;
                updateClickHouse();
            });
            fragment.appendChild(createFieldRow(t('settings.dbName'), t('settings.dbNameHint'), dbNameInput));
            const paramsInput = buildInput('text', clickHouseParts.params, 'secure=false&compress=lz4', event => {
                clickHouseParts.params = event.target.value;
                updateClickHouse();
            });
            fragment.appendChild(createFieldRow(t('settings.dbParams'), t('settings.dbParamsHint'), paramsInput));
        } else if (driver === 'postgres' || driver === 'postgresql' || driver === 'pgx' || driver === 'pg') {
            const pgParts = parsePostgresDsn(currentConfig.database.dsn);

            const updatePostgres = () => {
                currentConfig.database.dsn = formatPostgresDsn(pgParts);
                enableSave();
            };

            const hostInput = buildInput('text', pgParts.host, '127.0.0.1', e => {
                pgParts.host = e.target.value;
                updatePostgres();
            });
            fragment.appendChild(createFieldRow(t('settings.dbHost'), t('settings.dbHostHint'), hostInput));

            const portInput = buildInput('number', pgParts.port, '5432', e => {
                pgParts.port = e.target.value;
                updatePostgres();
            });
            fragment.appendChild(createFieldRow(t('settings.dbPort'), t('settings.dbPortHint'), portInput));

            const userInput = buildInput('text', pgParts.user, 'postgres', e => {
                pgParts.user = e.target.value;
                updatePostgres();
            });
            fragment.appendChild(createFieldRow(t('settings.dbUser'), t('settings.dbUserHint'), userInput));

            const passInput = buildInput('password', pgParts.password, '••••••••', e => {
                pgParts.password = e.target.value;
                updatePostgres();
            });
            fragment.appendChild(createFieldRow(t('settings.dbPassword'), t('settings.dbPasswordHint'), passInput));

            const dbNameInput = buildInput('text', pgParts.database, 'renop', e => {
                pgParts.database = e.target.value;
                updatePostgres();
            });
            fragment.appendChild(createFieldRow(t('settings.dbName'), t('settings.dbNameHint'), dbNameInput));

            const paramsInput = buildInput('text', pgParts.params, 'sslmode=disable', e => {
                pgParts.params = e.target.value;
                updatePostgres();
            });
            fragment.appendChild(createFieldRow(t('settings.dbParams'), t('settings.dbParamsHint'), paramsInput));
        } else if (driver === 'mysql') {
            const mysqlParts = parseMysqlDsn(currentConfig.database.dsn);

            const updateMysql = () => {
                currentConfig.database.dsn = formatMysqlDsn(mysqlParts);
                enableSave();
            };

            const hostInput = buildInput('text', mysqlParts.host, '127.0.0.1', e => {
                mysqlParts.host = e.target.value;
                updateMysql();
            });
            fragment.appendChild(createFieldRow(t('settings.dbHost'), t('settings.dbHostHint'), hostInput));

            const portInput = buildInput('number', mysqlParts.port, '3306', e => {
                mysqlParts.port = e.target.value;
                updateMysql();
            });
            fragment.appendChild(createFieldRow(t('settings.dbPort'), t('settings.dbPortHint'), portInput));

            const userInput = buildInput('text', mysqlParts.user, 'root', e => {
                mysqlParts.user = e.target.value;
                updateMysql();
            });
            fragment.appendChild(createFieldRow(t('settings.dbUser'), t('settings.dbUserHint'), userInput));

            const passInput = buildInput('password', mysqlParts.password, '••••••••', e => {
                mysqlParts.password = e.target.value;
                updateMysql();
            });
            fragment.appendChild(createFieldRow(t('settings.dbPassword'), t('settings.dbPasswordHint'), passInput));

            const dbNameInput = buildInput('text', mysqlParts.database, 'renop', e => {
                mysqlParts.database = e.target.value;
                updateMysql();
            });
            fragment.appendChild(createFieldRow(t('settings.dbName'), t('settings.dbNameHint'), dbNameInput));

            const paramsInput = buildInput('text', mysqlParts.params, 'charset=utf8mb4&parseTime=True&loc=Local', e => {
                mysqlParts.params = e.target.value;
                updateMysql();
            });
            fragment.appendChild(createFieldRow(t('settings.dbParams'), t('settings.dbParamsHint'), paramsInput));
        } else {
            // sqlite3
            const dsnInput = buildInput('text', currentConfig.database.dsn || 'renop.db', 'renop.db', e => {
                currentConfig.database.dsn = e.target.value;
                enableSave();
            });
            fragment.appendChild(createFieldRow(t('settings.dbDsn'), t('settings.dbDsnHint'), dsnInput));
        }
        return fragment;
    }

    function updateDsnUI(driver, animate = false) {
        if (dsnAnimTimer1) clearTimeout(dsnAnimTimer1);
        if (dsnAnimTimer2) clearTimeout(dsnAnimTimer2);
        dsnAnimTimer1 = null;
        dsnAnimTimer2 = null;

        if (!animate || window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
            dsnContainer.innerHTML = '';
            dsnContainer.style.height = '';
            dsnContainer.style.transition = '';
            dsnContainer.style.overflow = '';
            dsnContainer.appendChild(buildDsnFields(driver));
            return;
        }

        const oldRows = Array.from(dsnContainer.children);
        const startHeight = dsnContainer.getBoundingClientRect().height;

        oldRows.forEach(row => {
            row.classList.remove('cfg-field-row--entering');
            row.classList.add('cfg-field-row--leaving');
        });

        if (startHeight > 0) {
            dsnContainer.style.height = `${startHeight}px`;
            dsnContainer.style.overflow = 'hidden';
        }

        dsnAnimTimer1 = setTimeout(() => {
            dsnContainer.innerHTML = '';
            const fragment = buildDsnFields(driver);
            const newRows = Array.from(fragment.children);

            newRows.forEach((row, idx) => {
                row.style.setProperty('--field-index', idx);
                row.classList.add('cfg-field-row--entering');
            });

            dsnContainer.appendChild(fragment);

            dsnContainer.style.height = 'auto';
            const targetHeight = dsnContainer.getBoundingClientRect().height;
            dsnContainer.style.height = `${startHeight}px`;

            void dsnContainer.offsetHeight; // force reflow

            dsnContainer.style.transition = 'height 0.35s cubic-bezier(0.16, 1, 0.3, 1)';
            dsnContainer.style.height = `${targetHeight}px`;

            dsnAnimTimer2 = setTimeout(() => {
                dsnContainer.style.height = '';
                dsnContainer.style.transition = '';
                dsnContainer.style.overflow = '';
                newRows.forEach(row => row.classList.remove('cfg-field-row--entering'));
                dsnAnimTimer1 = null;
                dsnAnimTimer2 = null;
            }, 360);
        }, 120);
    }

    const driverSelect = makeCustomSelect(driverOptions, currentConfig.database.driver || 'sqlite3', val => {
        currentConfig.database.driver = val;
        if (val === 'mysql' && (!currentConfig.database.dsn || !currentConfig.database.dsn.includes('tcp('))) {
            currentConfig.database.dsn = formatMysqlDsn(parseMysqlDsn(''));
        } else if (val === 'postgres' && (!currentConfig.database.dsn || (!currentConfig.database.dsn.startsWith('postgres://') && !currentConfig.database.dsn.startsWith('postgresql://')))) {
            currentConfig.database.dsn = formatPostgresDsn(parsePostgresDsn(''));
        } else if (val === 'clickhouse' && (!currentConfig.database.dsn || !currentConfig.database.dsn.startsWith('clickhouse://'))) {
            currentConfig.database.dsn = formatClickHouseDsn(parseClickHouseDsn(''));
        } else if (val === 'sqlite3' && (!currentConfig.database.dsn || currentConfig.database.dsn.includes('tcp(') || currentConfig.database.dsn.startsWith('postgres://') || currentConfig.database.dsn.startsWith('postgresql://') || currentConfig.database.dsn.startsWith('clickhouse://'))) {
            currentConfig.database.dsn = 'renop.db';
        }
        updateDsnUI(val, true);
        enableSave();
    });
    dbFields.appendChild(createFieldRow(t('settings.dbDriver'), t('settings.dbDriverHint'), driverSelect));
    dbFields.appendChild(dsnContainer);

    updateDsnUI(currentConfig.database.driver || 'sqlite3');

    const maxOpenInput = buildInput('number', currentConfig.database.max_open_conns || 25, '25', e => {
        const n = Number(e.target.value);
        if (!Number.isFinite(n) || n < 1) return;
        currentConfig.database.max_open_conns = Math.trunc(n);
        enableSave();
    });
    dbFields.appendChild(createFieldRow(t('settings.dbMaxOpenConns'), t('settings.dbMaxOpenConnsHint'), maxOpenInput));

    const maxIdleInput = buildInput('number', currentConfig.database.max_idle_conns || 25, '25', e => {
        const n = Number(e.target.value);
        if (!Number.isFinite(n) || n < 1) return;
        currentConfig.database.max_idle_conns = Math.trunc(n);
        enableSave();
    });
    dbFields.appendChild(createFieldRow(t('settings.dbMaxIdleConns'), t('settings.dbMaxIdleConnsHint'), maxIdleInput));

    const lifetimeInput = buildInput('number', currentConfig.database.conn_max_lifetime_sec || 300, '300', e => {
        const n = Number(e.target.value);
        if (!Number.isFinite(n) || n < 1) return;
        currentConfig.database.conn_max_lifetime_sec = Math.trunc(n);
        enableSave();
    });
    dbFields.appendChild(createFieldRow(t('settings.dbConnMaxLifetime'), t('settings.dbConnMaxLifetimeHint'), lifetimeInput));

    wrap.appendChild(dbSection);
    container.appendChild(wrap);
}

/**
 * Parses a MySQL DSN string into components (user, password, host, port, database, params).
 * @param {string} dsnStr - MySQL DSN string.
 * @returns {object} Parsed components.
 */
function parseMysqlDsn(dsnStr) {
    const defaults = {
        user: 'root',
        password: '',
        host: '127.0.0.1',
        port: '3306',
        database: 'renop',
        params: 'charset=utf8mb4&parseTime=True&loc=Local',
    };
    if (!dsnStr || typeof dsnStr !== 'string') return defaults;
    const str = dsnStr.trim();
    if (!str) return defaults;

    let user = '';
    let password = '';
    let host = '127.0.0.1';
    let port = '3306';
    let database = '';
    let params = '';

    let rest = str;

    const atIdx = rest.indexOf('@');
    if (atIdx !== -1) {
        const userPass = rest.substring(0, atIdx);
        rest = rest.substring(atIdx + 1);
        const colonIdx = userPass.indexOf(':');
        if (colonIdx !== -1) {
            user = userPass.substring(0, colonIdx);
            password = userPass.substring(colonIdx + 1);
        } else {
            user = userPass;
        }
    }

    const qIdx = rest.indexOf('?');
    if (qIdx !== -1) {
        params = rest.substring(qIdx + 1);
        rest = rest.substring(0, qIdx);
    }

    const slashIdx = rest.indexOf('/');
    if (slashIdx !== -1) {
        database = rest.substring(slashIdx + 1);
        const protoAddr = rest.substring(0, slashIdx);
        if (protoAddr) {
            const openParen = protoAddr.indexOf('(');
            const closeParen = protoAddr.lastIndexOf(')');
            if (openParen !== -1 && closeParen > openParen) {
                const addr = protoAddr.substring(openParen + 1, closeParen);
                const colonIdx = addr.lastIndexOf(':');
                if (colonIdx !== -1) {
                    host = addr.substring(0, colonIdx);
                    port = addr.substring(colonIdx + 1);
                } else {
                    host = addr;
                }
            } else {
                const colonIdx = protoAddr.lastIndexOf(':');
                if (colonIdx !== -1) {
                    host = protoAddr.substring(0, colonIdx);
                    port = protoAddr.substring(colonIdx + 1);
                } else {
                    host = protoAddr;
                }
            }
        }
    } else {
        database = rest;
    }

    return {
        user: user || 'root',
        password: password || '',
        host: host || '127.0.0.1',
        port: port || '3306',
        database: database || 'renop',
        params: params || '',
    };
}

/**
 * Formats MySQL components into a standard MySQL DSN string.
 * @param {object} parts - MySQL components (user, password, host, port, database, params).
 * @returns {string} DSN string.
 */
function formatMysqlDsn(parts) {
    const u = (parts.user || '').trim();
    const p = parts.password || '';
    const h = (parts.host || '127.0.0.1').trim();
    const port = (parts.port || '3306').trim();
    const db = (parts.database || '').trim();
    const params = (parts.params || '').trim();

    let userPass = u;
    if (p) {
        userPass += ':' + p;
    }

    let addr = h;
    if (port) {
        addr += ':' + port;
    }

    let dsn = '';
    if (userPass) {
        dsn += userPass + '@';
    }
    dsn += 'tcp(' + addr + ')/' + db;
    if (params) {
        dsn += '?' + params;
    }
    return dsn;
}

/**
 * Parse a native ClickHouse DSN into editable connection fields.
 * @param {string} dsnStr - ClickHouse connection URL.
 * @returns {object} Parsed connection fields.
 */
function parseClickHouseDsn(dsnStr) {
    const defaults = {
        user: 'default', password: '', host: '127.0.0.1', port: '9000', database: 'default', params: ''
    };
    if (!dsnStr || typeof dsnStr !== 'string' || !dsnStr.trim().startsWith('clickhouse://')) return defaults;
    try {
        const parsedUrl = new URL(dsnStr.trim());
        return {
            user: decodeURIComponent(parsedUrl.username || '') || defaults.user,
            password: decodeURIComponent(parsedUrl.password || ''),
            host: parsedUrl.hostname || defaults.host,
            port: parsedUrl.port || defaults.port,
            database: decodeURIComponent((parsedUrl.pathname || '').replace(/^\//, '')) || defaults.database,
            params: parsedUrl.search ? parsedUrl.search.substring(1) : ''
        };
    } catch {
        return defaults;
    }
}

/**
 * Format editable fields as a native ClickHouse DSN.
 * @param {object} parts - ClickHouse connection fields.
 * @returns {string} Native ClickHouse connection URL.
 */
function formatClickHouseDsn(parts) {
    const user = encodeURIComponent(String(parts.user || 'default').trim());
    const password = String(parts.password || '');
    const credentials = password ? `${user}:${encodeURIComponent(password)}` : user;
    const host = String(parts.host || '127.0.0.1').trim();
    const port = String(parts.port || '9000').trim();
    const database = encodeURIComponent(String(parts.database || 'default').trim());
    const params = String(parts.params || '').trim().replace(/^\?/, '');
    return `clickhouse://${credentials}@${host}${port ? `:${port}` : ''}/${database}${params ? `?${params}` : ''}`;
}

/**
 * Parses a PostgreSQL DSN string into components (user, password, host, port, database, params).
 * @param {string} dsnStr - PostgreSQL DSN or connection URL string.
 * @returns {object} Parsed components.
 */
function parsePostgresDsn(dsnStr) {
    const defaults = {
        user: 'postgres',
        password: '',
        host: '127.0.0.1',
        port: '5432',
        database: 'renop',
        params: 'sslmode=disable',
    };
    if (!dsnStr || typeof dsnStr !== 'string') return defaults;
    const str = dsnStr.trim();
    if (!str) return defaults;

    if (str.startsWith('postgres://') || str.startsWith('postgresql://')) {
        try {
            const parsedUrl = new URL(str);
            const user = decodeURIComponent(parsedUrl.username || '');
            const password = decodeURIComponent(parsedUrl.password || '');
            const host = parsedUrl.hostname || '127.0.0.1';
            const port = parsedUrl.port || '5432';
            const database = (parsedUrl.pathname || '').replace(/^\//, '') || 'renop';
            const params = parsedUrl.search ? parsedUrl.search.substring(1) : '';
            return {
                user: user || 'postgres',
                password: password || '',
                host: host || '127.0.0.1',
                port: port || '5432',
                database: database || 'renop',
                params: params || 'sslmode=disable',
            };
        } catch {
            // Fallback to manual parsing if URL constructor throws
        }
    }

    if (str.includes('=')) {
        const result = {...defaults, params: ''};
        const extraParams = [];
        const regex = /([a-zA-Z_]+)=('(?:\\'|[^'])*'|\S+)/g;
        let match;
        while ((match = regex.exec(str)) !== null) {
            const key = match[1].toLowerCase();
            let val = match[2];
            if (val.startsWith("'") && val.endsWith("'")) {
                val = val.slice(1, -1).replace(/\\'/g, "'");
            }
            if (key === 'user') result.user = val;
            else if (key === 'password') result.password = val;
            else if (key === 'host') result.host = val;
            else if (key === 'port') result.port = val;
            else if (key === 'dbname' || key === 'database') result.database = val;
            else extraParams.push(`${key}=${val}`);
        }
        if (extraParams.length > 0) {
            result.params = extraParams.join('&');
        }
        return result;
    }

    return defaults;
}

/**
 * Formats PostgreSQL components into a standard PostgreSQL URL DSN string.
 * @param {object} parts - PostgreSQL components (user, password, host, port, database, params).
 * @returns {string} DSN string.
 */
function formatPostgresDsn(parts) {
    const u = encodeURIComponent((parts.user || 'postgres').trim());
    const p = parts.password || '';
    const h = (parts.host || '127.0.0.1').trim();
    const port = (parts.port || '5432').trim();
    const db = (parts.database || 'renop').trim();
    const params = (parts.params || '').trim();

    let userPass = u;
    if (p) {
        userPass += ':' + encodeURIComponent(p);
    }

    let addr = h;
    if (port) {
        addr += ':' + port;
    }

    let dsn = 'postgres://';
    if (userPass) {
        dsn += userPass + '@';
    }
    dsn += addr + '/' + db;
    if (params) {
        dsn += '?' + (params.startsWith('?') ? params.slice(1) : params);
    }
    return dsn;
}

/**
 * Renders storage domain settings (path, Javadoc preview, size limits).
 * @param {HTMLElement} container - Form container element.
 * @param {object} data - StorageConfig fields.
 * @returns {void}
 */
function renderStorageSettings(container, data) {
    const currentConfig = data;
    const wrap = el('div', {class: 'cfg-layout'});

    const storageSection = createSection(
        createIcon('storage'),
        t('settings.fileStorage'),
        t('settings.fileStorageDesc')
    );
    const fields = storageSection.querySelector('.cfg-fields');

    const pathInput = buildInput('text', data.storage_path, '/var/renop/storage', e => {
        currentConfig.storage_path = e.target.value;
        enableSave();
    });
    fields.appendChild(createFieldRow(t('settings.storagePath'), t('settings.storagePathHint'), pathInput));

    const callout = createCallout('info', t('settings.storagePathWarning'), 'info');
    storageSection.appendChild(callout);

    fields.appendChild(createToggleRow(
        t('settings.enableJavadocPreview'),
        t('settings.enableJavadocPreviewDesc'),
        data.enable_javadoc_preview === true,
        checked => {
            currentConfig.enable_javadoc_preview = checked;
            enableSave();
        }
    ));

    const javadocPathInput = buildInput('text', data.javadoc_extract_path || '', t('settings.javadocExtractPathHint'), e => {
        currentConfig.javadoc_extract_path = e.target.value;
        enableSave();
    });
    fields.appendChild(createFieldRow(t('settings.javadocExtractPath'), t('settings.javadocExtractPathHint'), javadocPathInput));

    const maxJavadocSizeInput = buildInput('number', data.max_javadoc_size_mb, '256', e => {
        const n = Number(e.target.value);
        if (!Number.isFinite(n) || n < 1) return;
        currentConfig.max_javadoc_size_mb = Math.trunc(n);
        enableSave();
    });
    fields.appendChild(createFieldRow(t('settings.maxJavadocSize'), t('settings.maxJavadocSizeHint'), maxJavadocSizeInput));

    fields.appendChild(createToggleRow(
        t('settings.enableCargodocPreview'),
        t('settings.enableCargodocPreviewDesc'),
        data.enable_cargodoc_preview === true,
        checked => {
            currentConfig.enable_cargodoc_preview = checked;
            enableSave();
        }
    ));

    const cargodocPathInput = buildInput('text', data.cargodoc_extract_path || '', t('settings.cargodocExtractPathHint'), e => {
        currentConfig.cargodoc_extract_path = e.target.value;
        enableSave();
    });
    fields.appendChild(createFieldRow(t('settings.cargodocExtractPath'), t('settings.cargodocExtractPathHint'), cargodocPathInput));

    const maxCargodocSizeInput = buildInput('number', data.max_cargodoc_size_mb, '256', e => {
        const n = Number(e.target.value);
        if (!Number.isFinite(n) || n < 1) return;
        currentConfig.max_cargodoc_size_mb = Math.trunc(n);
        enableSave();
    });
    fields.appendChild(createFieldRow(t('settings.maxCargodocSize'), t('settings.maxCargodocSizeHint'), maxCargodocSizeInput));

    wrap.appendChild(storageSection);
    container.appendChild(wrap);
}

/**
 * Renders index management UI with incremental and full rebuild actions.
 * @param {HTMLElement} container - Form container element.
 * @returns {void}
 */
function renderIndexSettings(container) {
    const wrap = el('div', {class: 'cfg-layout'});

    const headerCallout = createCallout('neutral', t('settings.indexCallout'), 'info');
    wrap.appendChild(headerCallout);

    const grid = el('div', {class: 'cfg-index-grid'});

    const incrCard = createIndexCard({
        iconName: 'refresh',
        iconVariant: 'success',
        title: t('settings.incrScan'),
        desc: t('settings.incrScanDesc'),
        buttonId: 'diff-rebuild-btn',
        buttonText: t('settings.runIncrScan'),
        buttonVariant: 'primary',
        buttonTitle: t('settings.fastRebuildTooltip'),
        onButtonClick: async () => {
            if (await window.showConfirm(t('settings.confirmIncrRebuild'))) {
                triggerIndexRebuild('diff');
            }
        }
    });

    const fullCard = createIndexCard({
        iconName: 'delete',
        iconVariant: 'danger',
        title: t('settings.fullRebuild'),
        desc: t('settings.fullRebuildDesc'),
        note: {
            text: t('settings.fullRebuildWarning'),
            icon: 'clock'
        },
        buttonId: 'full-rebuild-btn',
        buttonText: t('settings.runFullRebuild'),
        buttonVariant: 'danger',
        buttonTitle: t('settings.slowRebuildTooltip'),
        onButtonClick: async () => {
            if (await window.showConfirm(t('settings.confirmFullRebuild'))) {
                triggerIndexRebuild('full');
            }
        }
    });

    grid.appendChild(incrCard);
    grid.appendChild(fullCard);
    wrap.appendChild(grid);
    container.appendChild(wrap);
}

/**
 * Triggers an index rebuild of the given mode and shows success/error alerts.
 * @param {'diff'|'full'|string} mode - Rebuild mode (`diff` incremental or `full`).
 * @returns {Promise<void>}
 */
async function triggerIndexRebuild(mode) {
    try {
        const {response} = await postProto('/api/settings/index/rebuild', RebuildIndexRequest, {mode});

        if (response.ok) {
            showAlert(t('settings.rebuildSuccess', {mode}), 'success');
        } else {
            showAlert(await responseErrorMessage(response, 'settings.rebuildFailed', {mode}), 'error');
        }
    } catch (e) {
        console.error('Failed to trigger index rebuild', e);
        showAlert(t('settings.rebuildFailed', {mode}), 'error');
    }
}

/**
 * Persists the current domain configuration via PUT, or no-ops if unchanged / index domain.
 * @returns {Promise<void>}
 */
export async function saveDomainSettings() {
    if (!currentDomain || !currentConfig) return;

    if (initialConfig && JSON.stringify(currentConfig) === JSON.stringify(initialConfig)) {
        showAlert(t('settings.savedSuccess'), 'success');
        const saveBtn = document.getElementById('settings-save-btn');
        if (saveBtn) saveBtn.disabled = true;
        return;
    }

    if (currentDomain === 'index') {
        return;
    }

    try {
        if (currentDomain === 'server') {
            const serviceDomains = SERVICE_DOMAINS.filter(domain => currentConfig[domain] && initialConfig?.[domain]);
            for (const domain of serviceDomains) {
                if (JSON.stringify(currentConfig[domain]) === JSON.stringify(initialConfig[domain])) continue;
                let response;
                let savedData = null;
                if (domain === 'github_oauth') {
                    response = await apiRequest('/api/settings/github-oauth', {
                        method: 'PUT',
                        headers: {'Content-Type': 'application/json'},
                        body: JSON.stringify(currentConfig[domain]),
                    });
                    if (response.ok) savedData = await response.json();
                } else {
                    ({response} = await putProto(
                        `/api/settings/domain/${domain}`,
                        DOMAIN_MESSAGE_TYPES[domain],
                        currentConfig[domain]
                    ));
                }
                if (!response.ok) {
                    const fallbackKey = domain === 'github_oauth'
                        ? 'settings.githubOAuthSaveFailed'
                        : 'settings.saveFailed';
                    throw new LocalizedResponseError(
                        await responseErrorMessage(response, fallbackKey),
                        response.status
                    );
                }
                if (domain === 'github_oauth' && savedData) {
                    currentConfig[domain] = {
                        ...savedData,
                        client_secret: '',
                        clear_client_secret: false,
                    };
                    const secretInput = document.getElementById('settings-github-oauth-secret');
                    if (secretInput) secretInput.value = '';
                }
                initialConfig[domain] = JSON.parse(JSON.stringify(currentConfig[domain]));
            }
            showAlert(t('settings.savedSuccess'), 'success');
            const saveBtn = document.getElementById('settings-save-btn');
            if (saveBtn) saveBtn.disabled = true;
            return;
        }

        const MessageType = DOMAIN_MESSAGE_TYPES[currentDomain];
        if (!MessageType) return;
        const {response} = await putProto(
            `/api/settings/domain/${currentDomain}`,
            MessageType,
            currentConfig
        );

        if (response.ok) {
            initialConfig = JSON.parse(JSON.stringify(currentConfig));
            showAlert(t('settings.savedSuccess'), 'success');
            const saveBtn = document.getElementById('settings-save-btn');
            if (saveBtn) saveBtn.disabled = true;
        } else {
            showAlert(await responseErrorMessage(response, 'settings.saveFailed'), 'error');
        }
    } catch (e) {
        console.error('Failed to save settings', e);
        showAlert(caughtErrorMessage(e, 'settings.saveFailed'), 'error');
    }
}

document.getElementById('settings-save-btn')?.addEventListener('click', saveDomainSettings);

document.getElementById('settings-restart-btn')?.addEventListener('click', async () => {
    if (await window.showConfirm(t('settings.confirmRestart'))) {
        await restartApp();
    }
});
