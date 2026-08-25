/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {initTheme} from '@renop/ui/theme';
import {cachedIsLoggedIn, cachedIsManager, initializeSession, isManagerTab, setSwitchTabHandler} from './auth.js';
import {initI18n, t} from './i18n.js';
import {RenopDialog} from './components.js';
import {el} from '@renop/ui/dom';
import {updateModalInertState} from '@renop/ui/modal';
import {enableDragToScroll, smoothScrollToTop} from '@renop/ui/scroll';
import {registerTabContainer, scrollTabIntoView, updateTabIndicator} from '@renop/ui/tabs';
import {closeModalWithAnim} from './app-ui.js';
import {fetchInstanceStatus, startDashboardRefresh, stopDashboardRefresh} from './dashboard.js';
import {initSettings} from './settings.js';
import {initRepositories} from './repositories.js';
import {fetchTokens} from './users.js';
import {populateRoles} from './users/modal.js';
import {setupProfile} from './profile.js';
import {loadDirectory} from './browser.js';
import {openMavenDomainCenter} from './browser/maven.js';
import {initMessageCenter} from './messages.js';
import './cargo-messages.js';
import './docker-messages.js';
import './maven-messages.js';
import './team-messages.js';
import {navigateToUserProfile, profileRouteFromPath} from './user-profiles.js';

initI18n();

window.addEventListener('languageChanged', async () => {
    updateCopyrightFooter();

    const currentTab = profileRouteFromPath(window.location.pathname)
        ? 'profile'
        : (localStorage.getItem('selectedTab') || 'overview');
    await switchTab(currentTab);

    if (currentTab === 'dashboard') {
        fetchInstanceStatus();
    }

    const createTokenModal = document.getElementById('create-token-modal');
    if (createTokenModal && createTokenModal.style.display !== 'none' && createTokenModal.style.display !== '' && createTokenModal.dataset.isClosing !== 'true') {
        populateRoles();
    }
});

(function () {
    const originalFetch = window.fetch;
    window.fetch = async function (...args) {
        try {
            return await originalFetch.apply(this, args);
        } catch (error) {
            if (error instanceof TypeError) {
                const offlineEl = document.getElementById('backend-offline');
                if (offlineEl) offlineEl.style.display = 'flex';
            }
            throw error;
        }
    };
})();

(function () {
    const bgUrlMeta = document.querySelector('meta[name="renop-background-url"]');
    const bgUrl = bgUrlMeta ? bgUrlMeta.getAttribute("content") : "";
    if (bgUrl && bgUrl !== "{{RENOP.BACKGROUND_URL}}") {
        const img = new Image();
        img.crossOrigin = "anonymous";
        img.onload = function () {
            document.body.style.backgroundImage = 'url("' + bgUrl.replace(/"/g, '\\"') + '")';
            document.body.style.backgroundSize = 'cover';
            document.body.style.backgroundPosition = 'center';
            document.body.style.backgroundAttachment = 'fixed';

            try {
                const canvas = document.createElement('canvas');
                canvas.width = 32;
                canvas.height = 32;
                const ctx = canvas.getContext('2d');
                ctx.drawImage(img, 0, 0, 32, 32);
                const imageData = ctx.getImageData(0, 0, 32, 32);
                const data = imageData.data;
                let totalLum = 0;
                for (let i = 0; i < data.length; i += 4) {
                    const r = data[i];
                    const g = data[i + 1];
                    const b = data[i + 2];
                    totalLum += 0.299 * r + 0.587 * g + 0.114 * b;
                }
                const avgLum = totalLum / (data.length / 4);
                if (avgLum > 130) {
                    document.documentElement.setAttribute('data-custom-bg', 'light');
                    document.body.classList.add('light-custom-bg');
                } else {
                    document.documentElement.setAttribute('data-custom-bg', 'dark');
                    document.body.classList.remove('light-custom-bg');
                }
            } catch (e) {
                document.documentElement.setAttribute('data-custom-bg', 'custom');
                document.body.classList.add('custom-bg-active');
            }
        };
        img.onerror = function () {
            console.error('Failed to load background image, falling back to default.');
        };
        img.src = bgUrl;
    }

    const storedHash = localStorage.getItem('renop_assets_hash');
    fetch('/api/status/hash')
        .then(res => res.json())
        .then(data => {
            const currentHash = data;
            if (currentHash && storedHash && storedHash !== currentHash) {
                const updateMsg = el('p', {
                    style: {
                        margin: '0',
                        lineHeight: '1.6',
                        fontSize: '0.95rem',
                        color: 'var(--text-color)',
                        textAlign: 'center'
                    }
                }, t('main.updateAvailableDesc'));

                RenopDialog.show({
                    id: 'asset-update-modal',
                    glass: true,
                    maxWidth: '420px',
                    closable: false,
                    centered: true,
                    title: t('main.updateAvailable'),
                    headerStyle: {padding: '1.1rem 1.25rem 0.6rem', textAlign: 'center'},
                    titleStyle: {justifyContent: 'center', textAlign: 'center', width: '100%', fontSize: '1.15rem'},
                    body: updateMsg,
                    bodyStyle: {
                        display: 'flex',
                        flexDirection: 'column',
                        alignItems: 'center',
                        justifyContent: 'center',
                        textAlign: 'center',
                        padding: '0.85rem 1.25rem 1.1rem'
                    },
                    footerStyle: {justifyContent: 'center', padding: '0.75rem 1.25rem'},
                    footer: [
                        {
                            text: t('main.forceUpdate'),
                            className: 'pill-btn pill-btn--primary',
                            style: {padding: '0.65rem 2.25rem', fontSize: '0.95rem', minWidth: '150px'},
                            onClick: async () => {
                                if ('caches' in window) {
                                    try {
                                        const cacheNames = await caches.keys();
                                        await Promise.all(cacheNames.map(name => caches.delete(name)));
                                    } catch (e) {
                                    }
                                }
                                if ('serviceWorker' in navigator) {
                                    try {
                                        const registrations = await navigator.serviceWorker.getRegistrations();
                                        for (let r of registrations) {
                                            await r.unregister();
                                        }
                                    } catch (e) {
                                    }
                                }
                                sessionStorage.clear();
                                localStorage.setItem('renop_assets_hash', currentHash);

                                try {
                                    await fetch(window.location.href, {cache: 'reload'});
                                } catch (e) {
                                }

                                const url = new URL(window.location.href);
                                url.searchParams.set('_v', currentHash);
                                url.searchParams.set('_t', Date.now().toString());
                                window.location.href = url.toString();
                            }
                        }
                    ]
                });
            } else if (!storedHash && currentHash) {
                localStorage.setItem('renop_assets_hash', currentHash);
            }
        })
        .catch(e => console.error('Failed to check assets hash', e));
})();

const tabs = document.querySelectorAll('#tabs .tab');
const instanceUrlSpan = document.getElementById('instance-url');
const tabContents = document.querySelectorAll('.tab-content');
const profileMenu = document.getElementById('profile-menu');
const profileMenuWrap = document.getElementById('profile-menu-wrap');
const profileTrigger = document.getElementById('profile-trigger');

/**
 * Open or close the signed-in account navigation menu.
 * @param {boolean} open - Whether the menu should be visible.
 * @param {boolean} [focusFirst=false] - Focus the first menu item after opening.
 * @returns {void}
 */
function setProfileMenuOpen(open, focusFirst = false) {
    if (!profileMenu || !profileTrigger) return;
    profileMenu.hidden = !open;
    profileTrigger.setAttribute('aria-expanded', String(open));
    if (open && focusFirst) {
        requestAnimationFrame(() => profileMenu.querySelector('.nav-profile-menu-item:not([style*="display: none"])')?.focus());
    }
}

/**
 * Mark the menu item representing the active application or profile section.
 * @param {string} tabId - Active application section.
 * @returns {void}
 */
function updateProfileMenuSelection(tabId) {
    if (!profileMenu) return;
    const route = profileRouteFromPath(window.location.pathname);
    profileMenu.querySelectorAll('.nav-profile-menu-item').forEach(item => {
        const action = item.dataset.profileAction;
        const menuTab = item.dataset.profileTab;
        const active = (menuTab === tabId && (menuTab !== 'overview' || window.location.pathname === '/'))
            || (tabId === 'profile' && action === 'view' && route?.section !== 'edit')
            || (tabId === 'profile' && action === 'edit' && route?.section === 'edit');
        item.classList.toggle('is-active', active);
        if (active) item.setAttribute('aria-current', 'page');
        else item.removeAttribute('aria-current');
    });
}

/**
 * Activate a main app tab: update tab UI, show matching content, and run tab-specific init.
 * @param {string} tabId - Tab id (e.g. 'overview', 'dashboard', 'settings').
 * @returns {Promise<void>}
 */
export async function switchTab(tabId) {
    if (isManagerTab(tabId) && !cachedIsManager) tabId = 'overview';
    if (tabId === 'overview' && profileRouteFromPath(window.location.pathname)) {
        tabId = 'profile';
    }
    let activeTabElement = null;
    tabs.forEach(tab => {
        if (tab.dataset.tab === tabId) {
            tab.classList.add('active');
            activeTabElement = tab;
        } else {
            tab.classList.remove('active');
        }
    });

    scrollTabIntoView(activeTabElement);

    updateTabIndicator(document.querySelector('#tabs'));

    if (window.scrollY > 0) {
        smoothScrollToTop();
    }

    tabContents.forEach(content => {
        if (content.id === `tab-content-${tabId}`) {
            content.style.display = 'block';
            content.classList.add('active');
        } else {
            content.style.display = 'none';
            content.classList.remove('active');
        }
    });

    if (tabId !== 'profile') localStorage.setItem('selectedTab', tabId);

    if (tabId === 'dashboard') {
        startDashboardRefresh();
    } else {
        stopDashboardRefresh();
    }

    if (tabId === 'settings') {
        initSettings();
    } else if (typeof window.jsonEditorInstance !== 'undefined' && window.jsonEditorInstance) {
        window.jsonEditorInstance.destroy();
        window.jsonEditorInstance = null;
    }

    if (tabId === 'repositories') {
        initRepositories();
    }
    if (tabId === 'users') {
        fetchTokens();
    }
    if (tabId === 'profile') {
        await setupProfile(profileRouteFromPath(window.location.pathname));
    }
    if (tabId === 'overview') {
        loadDirectory(window.location.pathname);
    }
    updateProfileMenuSelection(tabId);
}

setSwitchTabHandler(switchTab);

tabs.forEach(tab => {
    tab.addEventListener('click', (e) => {
        e.preventDefault();
        if (tab.classList.contains('active')) return;
        if (profileRouteFromPath(window.location.pathname)) {
            window.history.pushState(null, '', '/');
        }
        switchTab(tab.dataset.tab);
    });
});

window.addEventListener('popstate', () => {
    if (profileRouteFromPath(window.location.pathname)) {
        void switchTab('profile');
        return;
    }
    void switchTab('overview');
});

document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') {
        const modals = document.querySelectorAll('.modal');
        modals.forEach(m => {
            if (m.style.display !== 'none' && m.style.display !== '') {
                const closeBtn = m.querySelector('.close-btn') || m.querySelector('#btn-cancel-create-token') || m.querySelector('#btn-close-privacy-policy');
                if (closeBtn) {
                    closeBtn.click();
                } else {
                    closeModalWithAnim(m);
                }
            }
        });
        if (document.activeElement && document.activeElement !== document.body) {
            document.activeElement.blur();
        }
    } else if (e.key === 'Tab') {
        const openModal = Array.from(document.querySelectorAll('.modal')).find(m => m.style.display !== 'none' && m.style.display !== '');
        if (openModal) {
            const focusableElements = openModal.querySelectorAll('button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])');
            const focusable = Array.from(focusableElements).filter(el => !el.disabled && el.style.display !== 'none' && el.offsetWidth > 0);

            if (focusable.length === 0) {
                e.preventDefault();
                return;
            }
            const first = focusable[0];
            const last = focusable[focusable.length - 1];

            if (e.shiftKey && document.activeElement === first) {
                e.preventDefault();
                last.focus();
            } else if (!e.shiftKey && document.activeElement === last) {
                e.preventDefault();
                first.focus();
            } else if (!openModal.contains(document.activeElement)) {
                e.preventDefault();
                first.focus();
            }
        }
    }
});

/**
 * Refresh the footer copyright line with the current year range and translated notices.
 * @returns {void}
 */
export function updateCopyrightFooter() {
    const copyrightDiv = document.getElementById('footer-copyright');
    if (copyrightDiv) {
        const currentYear = new Date().getFullYear();
        const yearDisplay = currentYear > 2026 ? `2026 - ${currentYear}` : '2026';
        copyrightDiv.innerHTML = '';
        copyrightDiv.append(
            document.createTextNode(`${yearDisplay} `),
            el('a', {href: 'https://github.com/404Setup/SRC-RenoP', target: '_blank'}, 'RenoP'),
            document.createTextNode(`. ${t('footer.allRights')} ${t('footer.licenseNotice')}`)
        );
    }
}

/**
 * Initialize the application shell after the document is ready.
 * @returns {Promise<void>}
 */
async function initializeApplication() {
    initTheme();
    initMessageCenter();

    document.querySelectorAll('.snippet-tabs').forEach(enableDragToScroll);
    document.querySelectorAll('.snippet-content').forEach(enableDragToScroll);
    document.querySelectorAll('#tab-content-users .border-container').forEach(enableDragToScroll);
    document.querySelectorAll('#breadcrumb-links, .breadcrumb-trail').forEach(enableDragToScroll);

    if (instanceUrlSpan) {
        instanceUrlSpan.textContent = window.location.origin + '/';
    }

    const modalIds = [
        'create-token-modal', 'user-result-modal', 'login-modal', 'privacy-policy-modal', 'repo-mirrors-modal',
        'language-modal', 'message-center-modal', 'renop-confirm-container', 'renop-prompt-container'
    ];
    const _checkModals = () => {
        updateModalInertState();
    };
    const modalObserver = new MutationObserver(_checkModals);
    modalObserver.observe(document.body, {attributes: true, subtree: true, attributeFilter: ['style']});
    modalObserver.observe(document.body, {childList: true, subtree: false});

    updateModalInertState();

    try {
        updateCopyrightFooter();

        await initializeSession();

        const mainTabs = document.querySelector('#tabs');
        if (mainTabs) {
            registerTabContainer(mainTabs);
        }

        const profileRoute = profileRouteFromPath(window.location.pathname);
        let savedTab = profileRoute ? 'profile' : (localStorage.getItem('selectedTab') || 'overview');
        if (!profileRoute && savedTab === 'profile') {
            localStorage.setItem('selectedTab', 'overview');
            savedTab = 'overview';
        }
        if (!profileRoute && (!cachedIsLoggedIn || (isManagerTab(savedTab) && !cachedIsManager))) {
            savedTab = 'overview';
        }
        await switchTab(savedTab);

        if (profileTrigger && profileMenu && profileMenuWrap) {
            profileTrigger.addEventListener('click', event => {
                event.stopPropagation();
                setProfileMenuOpen(profileMenu.hidden);
            });
            profileTrigger.addEventListener('keydown', event => {
                if (event.key !== 'ArrowDown') return;
                event.preventDefault();
                setProfileMenuOpen(true, true);
            });
            profileMenu.addEventListener('click', async event => {
                const item = event.target.closest('.nav-profile-menu-item');
                if (!item || !profileMenu.contains(item)) return;
                setProfileMenuOpen(false);
                const username = localStorage.getItem('username') || '';
                if (item.dataset.accountAction === 'maven-domains') {
                    openMavenDomainCenter();
                    return;
                }
                if (item.dataset.profileAction) {
                    navigateToUserProfile(username, item.dataset.profileAction === 'edit' ? 'edit' : '');
                    return;
                }
                const targetTab = item.dataset.profileTab;
                if (!targetTab) return;
                if (targetTab === 'overview') {
                    if (window.location.pathname !== '/') {
                        window.history.pushState(null, '', '/');
                    }
                    await switchTab('overview');
                    return;
                }
                if (!cachedIsManager) return;
                if (profileRouteFromPath(window.location.pathname)) {
                    window.history.pushState(null, '', '/');
                }
                await switchTab(targetTab);
            });
            profileMenu.addEventListener('keydown', event => {
                const items = Array.from(profileMenu.querySelectorAll('.nav-profile-menu-item'))
                    .filter(item => item.style.display !== 'none');
                const currentIndex = items.indexOf(document.activeElement);
                let nextIndex = -1;
                if (event.key === 'ArrowDown') nextIndex = (currentIndex + 1) % items.length;
                else if (event.key === 'ArrowUp') nextIndex = (currentIndex - 1 + items.length) % items.length;
                else if (event.key === 'Home') nextIndex = 0;
                else if (event.key === 'End') nextIndex = items.length - 1;
                else if (event.key === 'Escape') {
                    event.preventDefault();
                    event.stopPropagation();
                    setProfileMenuOpen(false);
                    profileTrigger.focus();
                    return;
                }
                if (nextIndex >= 0 && items.length > 0) {
                    event.preventDefault();
                    items[nextIndex].focus();
                }
            });
            document.addEventListener('click', event => {
                if (!profileMenuWrap.contains(event.target)) setProfileMenuOpen(false);
            });
            window.addEventListener('authChanged', event => {
                if (!event.detail?.isLoggedIn) setProfileMenuOpen(false);
            });
        }

        const reloadBtn = document.getElementById('reload-btn');
        if (reloadBtn) {
            reloadBtn.addEventListener('click', () => window.location.reload());
        }

        const headerLogo = document.getElementById('header-logo');
        if (headerLogo) {
            headerLogo.addEventListener('error', function () {
                this.style.display = 'none';
            });
        }

        const privacyLink = document.getElementById('privacy-policy-link');
        const privacySeparator = document.getElementById('privacy-policy-separator');
        const privacyModal = document.getElementById('privacy-policy-modal');
        const privacyContent = document.getElementById('privacy-policy-content');
        const privacyBackdrop = document.getElementById('privacy-policy-backdrop');
        const btnClosePrivacy = document.getElementById('btn-close-privacy-policy');

        if (privacyLink && privacySeparator) {
            fetch('/api/privacy-policy', {method: 'HEAD'})
                .then(res => {
                    if (res.ok) {
                        privacyLink.style.display = 'inline';
                        privacySeparator.style.display = 'inline';
                    }
                })
                .catch(err => console.error('Failed to check privacy policy', err));

            let policyCached = false;
            privacyLink.addEventListener('click', (e) => {
                e.preventDefault();
                privacyModal.style.display = 'flex';
                updateModalInertState();
                if (!policyCached) {
                    privacyContent.textContent = t('privacy.loading');
                    fetch('/api/privacy-policy')
                        .then(res => res.text())
                        .then(text => {
                            privacyContent.textContent = text;
                            policyCached = true;
                        })
                        .catch(err => privacyContent.textContent = t('privacy.failedLoad'));
                }
            });

            const closeModal = () => {
                closeModalWithAnim(privacyModal);
            };
            if (btnClosePrivacy) btnClosePrivacy.addEventListener('click', closeModal);
            if (privacyBackdrop) privacyBackdrop.addEventListener('click', closeModal);
        }

        const icpText = document.getElementById('icp-text');
        const icpContainer = document.getElementById('icp-container');
        if (icpText && icpContainer) {
            const text = icpText.textContent.trim();
            if (text && text !== '{{RENOP.ICP_LICENSE}}') {
                icpContainer.style.display = 'inline';
            }
        }

        const publicSecurityFilingText = document.getElementById('public-security-filing-text');
        const publicSecurityFilingContainer = document.getElementById('public-security-filing-container');
        if (publicSecurityFilingText && publicSecurityFilingContainer) {
            const text = publicSecurityFilingText.textContent.trim();
            if (text && text !== '{{RENOP.PUBLIC_SECURITY_FILING}}') {
                publicSecurityFilingContainer.style.display = 'inline';
            }
        }

        const legalNoticeLink = document.getElementById('legal-notice-link');
        const legalNoticeContainer = document.getElementById('legal-notice-container');
        if (legalNoticeLink && legalNoticeContainer) {
            const rawUrl = legalNoticeLink.dataset.url?.trim();
            if (rawUrl && rawUrl !== '{{RENOP.LEGAL_NOTICE_URL}}') {
                try {
                    const url = new URL(rawUrl);
                    if ((url.protocol === 'http:' || url.protocol === 'https:') && !url.username && !url.password) {
                        legalNoticeLink.href = url.href;
                        legalNoticeContainer.style.display = 'inline';
                    }
                } catch {
                    // Invalid direct YAML values remain hidden.
                }
            }
        }
    } catch (e) {
    }
}

document.addEventListener('DOMContentLoaded', initializeApplication);
