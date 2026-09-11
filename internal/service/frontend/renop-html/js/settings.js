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
import {showAlert, showConfirm} from './alert.js';
import {el} from '@renop/ui/dom';
import {morphElementHeight} from '@renop/ui/height-anim';
import {makeCustomSelect} from '@renop/ui/custom-select';
import {apiRequest, fetchProto, postProto, putProto} from './api.js';
import {buildInput, createSection, makeTagListInput} from './cfg-ui.js';
import {
    createCallout,
    createFieldRow,
    createIcon,
    createIndexCard,
    createSkeleton,
    createToggleRow
} from './components.js';
import {exitProtectedRouteOnDenial} from './protected-route.js';
import {logout} from './auth.js';
import {restartApp} from './dashboard.js';
import {renderCaptchaSettings} from './settings/captcha.js';
import {renderLegalSettings} from './settings/legal.js';
import {renderCacheSettings} from './settings/cache.js';
import {renderRegistrationSettings} from './settings/registration.js';
import {renderMavenDomainSettings} from './settings/maven-domains.js';
import {renderOAuthSettings} from './settings/oauth.js';
import {renderMailSettings} from './settings/mail.js';
import {
    formatClickHouseDsn,
    formatMysqlDsn,
    formatPostgresDsn,
    parseClickHouseDsn,
    parseMysqlDsn,
    parsePostgresDsn
} from './settings/database-dsn.js';
import {caughtErrorMessage, LocalizedResponseError, responseErrorMessage} from './response-errors.js';
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

const SETTINGS_PAGES = Object.freeze({
    captcha: {label: 'captcha.title', render: renderCaptchaSettings},
    legal: {label: 'legal.title', render: renderLegalSettings},
    frontend: {label: 'settings.domainFrontend', render: renderFrontendSettings},
    server: {label: 'settings.domainServer', render: renderServerSettings},
    proxy: {label: 'settings.domainProxy', render: renderProxySettings},
    storage: {label: 'settings.domainStorage', render: renderStorageSettings},
    oauth_providers: {label: 'oauth.settingsTitle', render: renderOAuthSettings},
    super_teams: {label: 'superTeam.settingsTitle', render: renderSuperTeamSettings},
    publication_quota: {label: 'publicationQuota.settingsTitle', render: renderPublicationQuotaSettings},
    maven_domains: {label: 'maven.healthSettings', render: renderMavenDomainSettings},
    cache: {label: 'cache.title', render: renderCacheSettings},
    mail: {label: 'mail.title', render: renderMailSettings},
    registration: {label: 'registration.settingsTitle', render: renderRegistrationSettings},
    updater: {label: 'settings.domainUpdater', render: renderUpdaterSettings},
    index: {label: 'settings.domainIndex', render: renderIndexSettings},
});

const drafts = new Map();
let currentDomain = null;
let currentConfig = null;
let domainsList = [];
let activeFetchId = 0;
let discoveryId = 0;
let accountGeneration = 0;
let settingsOwner = '';
let saving = false;

/** @param {string} domain - Domain key. @returns {string} Localized page name. */
function domainLabel(domain) {
    return t(SETTINGS_PAGES[domain].label);
}

/** @param {object|undefined} draft - Page draft. @returns {boolean} Whether it differs from saved state. */
function dirtyDraft(draft) {
    return Boolean(draft && JSON.stringify(draft.config) !== JSON.stringify(draft.initial));
}

/** @returns {void} Refresh save state, current-page metadata, and draft markers. */
function enableSave() {
    const dirty = dirtyDraft(drafts.get(currentDomain));
    const save = document.getElementById('settings-save-btn');
    if (save) {
        save.disabled = saving || !currentConfig || !dirty || currentDomain === 'index';
        save.classList.toggle('is-saving', saving);
    }
    const reset = document.getElementById('settings-reset-btn');
    if (reset) reset.disabled = saving || !currentConfig || !dirty;
    const status = document.getElementById('settings-draft-status');
    if (status) status.textContent = t(saving ? 'settings.saving' : dirty ? 'settings.unsaved' : 'settings.upToDate');
    for (const button of document.querySelectorAll('#settings-nav [data-settings-domain]')) {
        const hasDraft = dirtyDraft(drafts.get(button.dataset.settingsDomain));
        button.classList.toggle('has-draft', hasDraft);
        button.setAttribute('aria-label', domainLabel(button.dataset.settingsDomain) + (hasDraft ? ` · ${t('settings.unsaved')}` : ''));
        button.disabled = saving;
    }
    const form = document.getElementById('settings-form-container');
    if (form) form.inert = saving;
    const picker = document.querySelector('#settings-page-picker button');
    if (picker) picker.disabled = saving;
    for (const button of document.querySelectorAll('#settings-pagination button')) {
        const index = domainsList.indexOf(currentDomain) + Number(button.dataset.step);
        button.disabled = saving || index < 0 || index >= domainsList.length;
    }
}

/** @returns {void} Render independent settings pages and bounded previous/next navigation. */
function renderDomainNavigation() {
    const nav = document.getElementById('settings-nav');
    if (!nav) return;
    nav.replaceChildren(...domainsList.map(domain => el('button', {
        type: 'button', class: 'settings-nav-item', 'data-settings-domain': domain,
        'aria-current': domain === currentDomain ? 'page' : null,
        onclick: () => {
            if (domain !== currentDomain) void loadDomainSettings(domain, true);
        }
    }, el('span', {}, domainLabel(domain)), el('span', {class: 'settings-draft-dot', 'aria-hidden': 'true'}))));
    const picker = document.getElementById('settings-page-picker');
    if (picker) {
        const select = makeCustomSelect(domainsList.map(domain => ({value: domain, label: domainLabel(domain)})),
            currentDomain || '', domain => {
                if (domain !== currentDomain) void loadDomainSettings(domain, true);
            });
        select.querySelector('button')?.setAttribute('aria-label', t('settings.sections'));
        picker.replaceChildren(select);
    }
    const heading = document.getElementById('settings-page-title');
    if (heading) heading.textContent = currentDomain ? domainLabel(currentDomain) : t('settings.title');
    const pager = document.getElementById('settings-pagination');
    if (pager) pager.replaceChildren(...(currentDomain ? [
        el('button', {
            type: 'button', class: 'renop-pagination-btn', 'data-step': '-1',
            onclick: () => void loadDomainSettings(domainsList[domainsList.indexOf(currentDomain) - 1], true)
        }, t('common.prev')),
        el('span', {class: 'renop-pagination-summary'}, t('settings.sectionPage', {
            page: domainsList.indexOf(currentDomain) + 1, pages: domainsList.length
        })),
        el('button', {
            type: 'button', class: 'renop-pagination-btn', 'data-step': '1',
            onclick: () => void loadDomainSettings(domainsList[domainsList.indexOf(currentDomain) + 1], true)
        }, t('common.next'))
    ] : []));
    enableSave();
}

/** @param {string} message - Localized error or empty state. @param {Function} retry - Retry action. @returns {void} */
function settingsLoadState(message, retry) {
    document.getElementById('settings-form-container')?.replaceChildren(el('div', {
            class: 'settings-load-state',
            role: 'status'
        },
        createIcon('warning'), el('p', {}, message),
        el('button', {type: 'button', class: 'pill-btn pill-btn--soft', onclick: retry}, t('offline.retryBtn'))));
}

/** Discover permitted pages; each page loads only its own configuration. */
export async function initSettings() {
    if (saving) return;
    const requestId = ++discoveryId;
    try {
        const {response, data} = await fetchProto('/api/settings/domains', SettingsDomainsResponse);
        if (requestId !== discoveryId) return;
        if (exitProtectedRouteOnDenial(response)) {
            if (response.status === 401) void logout('kicked');
            return;
        }
        if (!response.ok || !data) throw new LocalizedResponseError(await responseErrorMessage(response, 'settings.loadFailed'), response.status);
        domainsList = [...new Set(data.domains || [])].filter(domain => Object.hasOwn(SETTINGS_PAGES, domain));
        for (const domain of drafts.keys()) if (!domainsList.includes(domain)) drafts.delete(domain);
        currentDomain = domainsList.includes(currentDomain) ? currentDomain : domainsList[0] || null;
        renderDomainNavigation();
        if (currentDomain) await loadDomainSettings(currentDomain);
        else settingsLoadState(t('settings.noSections'), () => void initSettings());
    } catch (error) {
        if (requestId === discoveryId) settingsLoadState(caughtErrorMessage(error, 'settings.loadFailed'), () => void initSettings());
    }
}

/** @param {string} domain - Known page key. @returns {Promise<{response: Response, data: object|null}>} Configuration response. */
async function fetchDomainSettings(domain) {
    const MessageType = DOMAIN_MESSAGE_TYPES[domain];
    if (MessageType) return fetchProto(`/api/settings/domain/${domain}`, MessageType);
    const response = await apiRequest(`/api/settings/${domain.replaceAll('_', '-')}`, {}, {logoutOnForbidden: false});
    return {response, data: response.ok ? await response.json() : null};
}

/** @param {string} domain - Page to open. @param {boolean} [focus=false] - Focus the page heading after navigation. @returns {Promise<void>} */
async function loadDomainSettings(domain, focus = false) {
    if (saving || !domainsList.includes(domain)) return;
    const fetchId = ++activeFetchId;
    currentDomain = domain;
    currentConfig = null;
    renderDomainNavigation();
    const container = document.getElementById('settings-form-container');
    if (!container) return;
    container.setAttribute('aria-busy', 'true');
    container.inert = true;
    if (!container.firstElementChild) container.replaceChildren(createSkeleton('form', 2));
    try {
        let draft = drafts.get(domain);
        const fresh = !dirtyDraft(draft);
        if (fresh) {
            const {response, data} = await fetchDomainSettings(domain);
            if (fetchId !== activeFetchId) return;
            if (exitProtectedRouteOnDenial(response)) {
                if (response.status === 401) void logout('kicked');
                return;
            }
            if (!response.ok || !data) throw new LocalizedResponseError(await responseErrorMessage(response, 'settings.loadFailed'), response.status);
            draft = {config: data, initial: structuredClone(data)};
            drafts.set(domain, draft);
        }
        if (fetchId !== activeFetchId) return;
        currentConfig = draft.config;
        renderSettingsForm(domain, currentConfig);
        if (fresh) draft.initial = structuredClone(draft.config);
        enableSave();
    } catch (error) {
        if (fetchId === activeFetchId) settingsLoadState(caughtErrorMessage(error, 'settings.loadFailed'), () => void loadDomainSettings(domain));
    } finally {
        if (fetchId === activeFetchId) {
            container.setAttribute('aria-busy', 'false');
            container.inert = false;
            if (focus) document.getElementById('settings-page-title')?.focus();
        }
    }
}

/** @param {string} domain - Known page key. @param {object} data - Mutable page configuration. @returns {void} */
function renderSettingsForm(domain, data) {
    const container = document.getElementById('settings-form-container');
    if (!container) return;
    void morphElementHeight(container, () => {
        container.replaceChildren();
        SETTINGS_PAGES[domain].render(container, data, enableSave);
    }, {duration: 280});
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
 * Render global per-account team creation and membership limits.
 * @param {HTMLElement} container - Service settings stack.
 * @param {{create_limit?: number, join_limit?: number}} data - Mutable global team limits.
 * @returns {void}
 */
function renderSuperTeamSettings(container, data) {
    const wrap = el('div', {class: 'cfg-layout'});
    const section = createSection(
        createIcon('identity'),
        t('superTeam.settingsTitle'),
        t('superTeam.settingsSubtitle'),
        {defaultCollapsed: true}
    );
    const fields = section.querySelector('.cfg-fields');
    const createLimit = buildInput('number', Number(data.create_limit) || 5, '5', event => {
        const value = Number(event.target.value);
        if (!Number.isInteger(value) || value < 1 || value > 1000) return;
        data.create_limit = value;
        enableSave();
    });
    createLimit.min = '1';
    createLimit.max = '1000';
    const joinLimit = buildInput('number', Number(data.join_limit) || 20, '20', event => {
        const value = Number(event.target.value);
        if (!Number.isInteger(value) || value < 1 || value > 1000) return;
        data.join_limit = value;
        enableSave();
    });
    joinLimit.min = '1';
    joinLimit.max = '1000';
    fields.append(
        createFieldRow(t('superTeam.createLimit'), t('superTeam.createLimitHint'), createLimit),
        createFieldRow(t('superTeam.joinLimit'), t('superTeam.joinLimitHint'), joinLimit)
    );
    wrap.appendChild(section);
    container.appendChild(wrap);
}

/**
 * Render global publication quota defaults inherited by accounts and global teams.
 * @param {HTMLElement} container - Service settings stack.
 * @param {{file_limit?: number, byte_limit?: number, publication_limit?: number, period?: string}} data - Mutable quota defaults.
 * @returns {void}
 */
function renderPublicationQuotaSettings(container, data) {
    const wrap = el('div', {class: 'cfg-layout'});
    const section = createSection(
        createIcon('database'),
        t('publicationQuota.settingsTitle'),
        t('publicationQuota.settingsSubtitle'),
        {defaultCollapsed: true}
    );
    const fields = section.querySelector('.cfg-fields');
    const fileLimit = buildInput('number', Number.isFinite(Number(data.file_limit)) ? Number(data.file_limit) : 600, '600', event => {
        const value = Number(event.target.value);
        if (!Number.isSafeInteger(value) || value < 1 || value > 10000000) return;
        data.file_limit = value;
        enableSave();
    });
    fileLimit.min = '1';
    fileLimit.max = '10000000';
    const byteLimit = buildInput('number', Math.round(Number(data.byte_limit || 0) / (1024 * 1024)), '40', event => {
        const value = Number(event.target.value);
        if (!Number.isSafeInteger(value) || value < 1 || value > 1073741824) return;
        data.byte_limit = value * 1024 * 1024;
        enableSave();
    });
    byteLimit.min = '1';
    byteLimit.max = '1073741824';
    const publicationLimit = buildInput('number', Number.isFinite(Number(data.publication_limit)) ? Number(data.publication_limit) : 20, '20', event => {
        const value = Number(event.target.value);
        if (!Number.isSafeInteger(value) || value < 1 || value > 1000000) return;
        data.publication_limit = value;
        enableSave();
    });
    publicationLimit.min = '1';
    publicationLimit.max = '1000000';
    const period = makeCustomSelect([
        {value: 'day', label: t('publicationQuota.period.day')},
        {value: 'week', label: t('publicationQuota.period.week')},
        {value: 'month', label: t('publicationQuota.period.month')},
        {value: 'lifetime', label: t('publicationQuota.period.lifetime')},
    ], data.period || 'month', value => {
        data.period = value;
        enableSave();
    });
    fields.append(
        createFieldRow(t('publicationQuota.filesLimit'), t('publicationQuota.filesLimitHint'), fileLimit),
        createFieldRow(t('publicationQuota.storageLimitMiB'), t('publicationQuota.storageLimitHint'), byteLimit),
        createFieldRow(t('publicationQuota.publicationLimit'), t('publicationQuota.publicationLimitHint'), publicationLimit),
        createFieldRow(t('publicationQuota.period'), t('publicationQuota.periodHint'), period)
    );
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
    const currentConfig = data;
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
    const currentConfig = data;
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

    const avatarLimitMiB = Math.round((Number(data.avatar_max_size_bytes) || 1048576) / 65536) / 16;
    const avatarLimitInput = buildInput('number', avatarLimitMiB, '1', e => {
        const n = Number(e.target.value);
        if (!Number.isFinite(n) || n < 0.0625 || n > 16) return;
        currentConfig.avatar_max_size_bytes = Math.round(n * 1048576);
        enableSave();
    });
    avatarLimitInput.min = '0.0625';
    avatarLimitInput.max = '16';
    avatarLimitInput.step = '0.0625';
    perfFields.appendChild(createFieldRow(
        t('settings.avatarMaxSize'), t('settings.avatarMaxSizeHint'), avatarLimitInput
    ));

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

    /** Replace driver fields immediately so rapid changes cannot restore stale controls. */
    function updateDsnUI(driver, animate = false) {
        const replace = () => dsnContainer.replaceChildren(buildDsnFields(driver));
        if (animate) void morphElementHeight(dsnContainer, replace, {duration: 280});
        else replace();
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

/** Persist only the active page; lock editing until its response has settled. */
export async function saveDomainSettings() {
    const domain = currentDomain, draft = drafts.get(domain), generation = accountGeneration;
    if (saving || !currentConfig || !dirtyDraft(draft) || domain === 'index') return;
    const invalid = document.querySelector('#settings-form-container input:invalid:not([data-mail-test]), #settings-form-container textarea:invalid');
    if (invalid) {
        for (const section of document.querySelectorAll('#settings-form-container .cfg-section.is-collapsed')) {
            if (section.contains(invalid)) section.querySelector('.cfg-section-header')?.click();
        }
        invalid.reportValidity();
        return;
    }
    const submitted = structuredClone(draft.config);
    saving = true;
    enableSave();
    try {
        let response, savedData;
        if (DOMAIN_MESSAGE_TYPES[domain]) {
            ({response} = await putProto(`/api/settings/domain/${domain}`, DOMAIN_MESSAGE_TYPES[domain], submitted));
        } else {
            response = await apiRequest(`/api/settings/${domain.replaceAll('_', '-')}`, {
                method: 'PUT', headers: {'Content-Type': 'application/json'},
                body: JSON.stringify(domain === 'oauth_providers' ? {providers: submitted.providers} : submitted),
            }, {logoutOnForbidden: false});
            if (response.ok) savedData = await response.json();
        }
        if (generation !== accountGeneration) return;
        if (!response.ok) throw new LocalizedResponseError(await responseErrorMessage(response, 'settings.saveFailed'), response.status);
        draft.config = savedData || submitted;
        if (domain === 'cache') Object.assign(draft.config, {password: '', clear_password: false});
        if (domain === 'mail') draft.config.clear_secrets = {};
        currentConfig = draft.config;
        renderSettingsForm(domain, currentConfig);
        draft.initial = structuredClone(draft.config);
        if (domain === 'oauth_providers') window.dispatchEvent(new Event('oauthProvidersChanged'));
        if (domain === 'legal') window.dispatchEvent(new Event('legalSettingsChanged'));
        if (domain === 'captcha') window.dispatchEvent(new Event('captchaSettingsChanged'));
        showAlert(t('settings.savedSuccess'), 'success');
    } catch (error) {
        if (generation === accountGeneration) showAlert(caughtErrorMessage(error, 'settings.saveFailed'), 'error');
    } finally {
        if (generation === accountGeneration) {
            saving = false;
            enableSave();
        }
    }
}

document.getElementById('settings-save-btn')?.addEventListener('click', saveDomainSettings);
document.getElementById('settings-reset-btn')?.addEventListener('click', async () => {
    const domain = currentDomain;
    if (saving || !dirtyDraft(drafts.get(domain)) || !await showConfirm(t('settings.discardConfirm'))) return;
    if (domain !== currentDomain || saving) return;
    drafts.delete(domain);
    await loadDomainSettings(domain);
});
document.getElementById('settings-restart-btn')?.addEventListener('click', async () => {
    if (!saving && await showConfirm(t('settings.confirmRestart'))) await restartApp();
});
window.addEventListener('beforeunload', event => {
    if ([...drafts.values()].some(dirtyDraft)) {
        event.preventDefault();
        event.returnValue = '';
    }
});
window.addEventListener('authChanged', event => {
    const user = event.detail;
    if (user?.isLoggedIn && user.isManager && user.username === settingsOwner) return;
    settingsOwner = user?.isLoggedIn && user.isManager ? user.username : '';
    accountGeneration++;
    activeFetchId++;
    discoveryId++;
    drafts.clear();
    currentDomain = null;
    currentConfig = null;
    saving = false;
    domainsList = [];
    document.getElementById('settings-form-container')?.replaceChildren();
    renderDomainNavigation();
});
