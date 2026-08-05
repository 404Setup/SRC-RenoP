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
import {fetchProto, postProto, putProto} from './api.js';
import {buildInput, createSection} from './cfg-ui.js';
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
    FrontendConfig,
    IndexDomainSettings,
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
    updater: UpdaterConfig,
    index: IndexDomainSettings,
};

let currentDomain = null;
let currentConfig = null;
let initialConfig = null;
let domainsList = [];
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
            domainsList = data.domains || [];
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
 * @param {string} domain - Domain key (frontend, server, storage, updater, index).
 * @returns {string} Localized or title-cased label.
 */
function domainLabel(domain) {
    const labels = {
        frontend: t('settings.domainFrontend'),
        server: t('settings.domainServer'),
        storage: t('settings.domainStorage'),
        updater: t('settings.domainUpdater'),
        index: t('settings.domainIndex'),
    };
    return labels[domain] || domain.charAt(0).toUpperCase() + domain.slice(1);
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
        const MessageType = DOMAIN_MESSAGE_TYPES[domain];
        if (!MessageType) {
            console.error('Unknown settings domain', domain);
            return;
        }
        const {response, data} = await fetchProto(`/api/settings/domain/${domain}`, MessageType);

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
 * @param {string} domain - Domain key (frontend, server, storage, updater, index).
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
        renderServerSettings(container, data);
    } else if (domain === 'storage') {
        renderStorageSettings(container, data);
    } else if (domain === 'updater') {
        renderUpdaterSettings(container, data);
    } else if (domain === 'index') {
        renderIndexSettings(container);
    }
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

    wrap.appendChild(identitySection);
    wrap.appendChild(brandSection);
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
        {value: 'mysql', label: 'MySQL'}
    ];

    const dsnContainer = el('div', {class: 'cfg-dsn-container'});
    let dsnAnimTimer1 = null;
    let dsnAnimTimer2 = null;

    function buildDsnFields(driver) {
        const fragment = document.createDocumentFragment();
        if (driver === 'mysql') {
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
        } else if (val === 'sqlite3' && (!currentConfig.database.dsn || currentConfig.database.dsn.includes('tcp('))) {
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
 * Renders storage domain settings (path, Javadoc preview, size limits).
 * @param {HTMLElement} container - Form container element.
 * @param {object} data - StorageConfig fields.
 * @returns {void}
 */
function renderStorageSettings(container, data) {
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
        badgeText: t('settings.fastUpdate'),
        badgeType: 'success',
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
        badgeText: t('settings.deepScan'),
        badgeType: 'warning',
        iconName: 'delete',
        iconVariant: 'danger',
        title: t('settings.fullRebuild'),
        desc: t('settings.fullRebuildDesc'),
        callout: {
            type: 'danger',
            text: t('settings.fullRebuildWarning'),
            icon: 'warning'
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
            const errText = await response.text();
            showAlert(errText || t('settings.rebuildFailed', {mode}), 'error');
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

    const MessageType = DOMAIN_MESSAGE_TYPES[currentDomain];
    if (!MessageType || currentDomain === 'index') {
        return;
    }

    try {
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
            const errText = await response.text();
            showAlert(errText || t('settings.saveFailed'), 'error');
        }
    } catch (e) {
        console.error('Failed to save settings', e);
        showAlert(t('settings.saveFailed'), 'error');
    }
}

document.getElementById('settings-save-btn')?.addEventListener('click', saveDomainSettings);

document.getElementById('settings-restart-btn')?.addEventListener('click', async () => {
    if (await window.showConfirm(t('settings.confirmRestart'))) {
        await restartApp();
    }
});
