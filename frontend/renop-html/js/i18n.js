/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import enUS from './i18n/en-US.js';
import deDE from './i18n/de-DE.js';
import frFR from './i18n/fr-FR.js';
import jaJP from './i18n/ja-JP.js';
import zhCN from './i18n/zh-CN.js';
import zhHK from './i18n/zh-HK.js';
import zhTW from './i18n/zh-TW.js';
import zhYUE from './i18n/zh-YUE.js';
import koKR from './i18n/ko-KR.js';
import ruRU from './i18n/ru-RU.js';
import esES from './i18n/es-ES.js';
import ptPT from './i18n/pt-PT.js';
import {createLangCard} from './components.js';

const STORAGE_KEY = 'renop_language';
const DEFAULT_LANG = 'en-US';

const languages = {
    'en-US': enUS,
    'en': enUS,
    'zh-CN': zhCN,
    'zh-cn': zhCN,
    'zh-Hans': zhCN,
    'zh-hans': zhCN,
    'zh': zhCN,
    'zh-HK': zhHK,
    'zh-hk': zhHK,
    'zh-Hant-HK': zhHK,
    'zh-hant-hk': zhHK,
    'zh-TW': zhTW,
    'zh-tw': zhTW,
    'zh-Hant': zhTW,
    'zh-hant': zhTW,
    'zh-Hant-TW': zhTW,
    'zh-hant-tw': zhTW,
    'zh-YUE': zhYUE,
    'zh-yue': zhYUE,
    'yue': zhYUE,
    'yue-HK': zhYUE,
    'yue-hk': zhYUE,
    'zh-Hant-HK-yue': zhYUE,
    'ko-KR': koKR,
    'ko-kr': koKR,
    'ko': koKR,
    'ja-JP': jaJP,
    'ja': jaJP,
    'de-DE': deDE,
    'de': deDE,
    'fr-FR': frFR,
    'fr': frFR,
    'ru-RU': ruRU,
    'ru-ru': ruRU,
    'ru': ruRU,
    'es-ES': esES,
    'es-es': esES,
    'es': esES,
    'pt-PT': ptPT,
    'pt-pt': ptPT,
    'pt-BR': ptPT,
    'pt-br': ptPT,
    'pt': ptPT
};

const languageDetails = {
    'zh-CN': {name: '简体中文', sub: 'Simplified Chinese', code: 'ZH'},
    'zh-HK': {name: '繁體中文 (香港)', sub: 'Traditional Chinese (HK)', code: 'HK'},
    'zh-TW': {name: '繁體中文 (台灣)', sub: 'Traditional Chinese (TW)', code: 'TW'},
    'zh-YUE': {name: '繁體中文 (粵語)', sub: 'Cantonese (Traditional)', code: 'YUE'},
    'ko-KR': {name: '한국어', sub: 'Korean', code: 'KO'},
    'en-US': {name: 'English', sub: 'English (US)', code: 'EN'},
    'ja-JP': {name: '日本語', sub: 'Japanese', code: 'JA'},
    'de-DE': {name: 'Deutsch', sub: 'German', code: 'DE'},
    'fr-FR': {name: 'Français', sub: 'French', code: 'FR'},
    'ru-RU': {name: 'Русский', sub: 'Russian', code: 'RU'},
    'es-ES': {name: 'Español', sub: 'Spanish', code: 'ES'},
    'pt-PT': {name: 'Português', sub: 'Portuguese', code: 'PT'}
};

let currentLang = DEFAULT_LANG;
let currentSource = 'default';

/**
 * Return the list of primary (canonical) language codes shown in the UI.
 * @returns {string[]} Canonical language codes (e.g. 'en-US', 'zh-CN').
 */
export function getAvailableLanguages() {
    return ['zh-CN', 'zh-HK', 'zh-TW', 'zh-YUE', 'ko-KR', 'en-US', 'ja-JP', 'de-DE', 'fr-FR', 'ru-RU', 'es-ES', 'pt-PT'];
}

/**
 * Return a map of language code → native display name for UI labels.
 * @returns {Object.<string, string>} Code to human-readable name.
 */
export function getLanguageNames() {
    return {
        'zh-CN': '简体中文',
        'zh-HK': '繁體中文 (香港)',
        'zh-TW': '繁體中文 (台灣)',
        'zh-YUE': '繁體中文 (粵語)',
        'ko-KR': '한국어',
        'en-US': 'English',
        'ja-JP': '日本語',
        'de-DE': 'Deutsch',
        'fr-FR': 'Français',
        'ru-RU': 'Русский',
        'es-ES': 'Español',
        'pt-PT': 'Português'
    };
}

/**
 * Return detailed language metadata (native name, English sublabel, short code).
 * @returns {Object.<string, {name: string, sub: string, code: string}>}
 */
export function getLanguageDetails() {
    return languageDetails;
}

/**
 * Synchronize UI indicators for the active language (button label and modal cards).
 * @returns {void}
 */
function updateLanguageUI() {
    const currentLangNameEl = document.getElementById('current-lang-name');
    const langBtn = document.getElementById('lang-btn');
    const details = languageDetails[currentLang] || languageDetails[DEFAULT_LANG];
    const newName = details ? details.name : currentLang;

    if (currentLangNameEl) {
        const oldName = currentLangNameEl.textContent.trim();

        if (oldName && oldName !== newName && langBtn && document.readyState !== 'loading') {
            const startWidth = langBtn.getBoundingClientRect().width;

            langBtn.style.width = `${startWidth}px`;
            langBtn.classList.add('is-animating');
            currentLangNameEl.classList.add('lang-name-changing');

            setTimeout(() => {
                currentLangNameEl.textContent = newName;

                langBtn.style.width = 'auto';
                const targetWidth = langBtn.getBoundingClientRect().width;

                langBtn.style.width = `${startWidth}px`;
                void langBtn.offsetWidth;

                langBtn.style.width = `${targetWidth}px`;
                currentLangNameEl.classList.remove('lang-name-changing');
                currentLangNameEl.classList.add('lang-name-changed');

                setTimeout(() => {
                    langBtn.style.width = '';
                    langBtn.classList.remove('is-animating');
                    currentLangNameEl.classList.remove('lang-name-changed');
                }, 300);
            }, 120);
        } else {
            currentLangNameEl.textContent = newName;
        }
    }

    const grid = document.getElementById('language-grid');
    if (grid) {
        grid.querySelectorAll('.lang-card').forEach(card => {
            const lKey = card.getAttribute('data-lang');
            if (lKey === currentLang) {
                card.classList.add('active');
            } else {
                card.classList.remove('active');
            }
        });
    }
}

/**
 * Normalize and match a language code to a supported entry in the registry.
 * @param {string} lang - Raw language tag (e.g. 'zh', 'en-US', 'yue-HK').
 * @returns {string|null} Matched registry key, or null if unsupported.
 */
function resolveLanguage(lang) {
    if (!lang || typeof lang !== 'string') return null;
    const clean = lang.trim();

    for (const key of Object.keys(languages)) {
        if (key.toLowerCase() === clean.toLowerCase()) {
            return key;
        }
    }

    const cleanLower = clean.toLowerCase();

    if (cleanLower.startsWith('zh') || cleanLower.startsWith('yue')) {
        if (cleanLower.includes('yue') || cleanLower.includes('cantonese')) return 'zh-YUE';
        if (cleanLower.includes('hk')) return 'zh-HK';
        if (cleanLower.includes('tw') || cleanLower.includes('hant')) return 'zh-TW';
        if (cleanLower.includes('cn') || cleanLower.includes('hans') || cleanLower.includes('sg')) return 'zh-CN';
        return 'zh-CN';
    }

    const base = clean.split('-')[0].toLowerCase();
    for (const key of Object.keys(languages)) {
        if (key.toLowerCase().startsWith(base)) {
            return key;
        }
    }

    return null;
}

/**
 * Detect preferred language from localStorage, then browser preferences, then default.
 * @returns {{lang: string, source: 'localStorage'|'browser'|'fallback'}} Detected language and source.
 */
function detectLanguage() {
    const saved = localStorage.getItem(STORAGE_KEY);
    if (saved) {
        const resolved = resolveLanguage(saved);
        if (resolved) {
            return {lang: resolved, source: 'localStorage'};
        }
    }

    const browserLangs = (navigator.languages && navigator.languages.length)
        ? navigator.languages
        : [navigator.language];

    for (const bLang of browserLangs) {
        if (bLang) {
            const resolved = resolveLanguage(bLang);
            if (resolved) {
                return {lang: resolved, source: 'browser'};
            }
        }
    }

    return {lang: DEFAULT_LANG, source: 'fallback'};
}

/**
 * Translate a key into the active language, with optional `{param}` interpolation.
 * @param {string} key - Message key (falls back to key text if missing).
 * @param {Object.<string, *>} [params={}] - Values substituted for `{name}` placeholders.
 * @returns {string} Translated (and interpolated) string.
 */
export function t(key, params = {}) {
    const dict = languages[currentLang] || languages[DEFAULT_LANG] || {};
    let text = dict[key] || languages[DEFAULT_LANG]?.[key] || key;

    if (typeof text === 'string' && params && typeof params === 'object') {
        for (const [pKey, pVal] of Object.entries(params)) {
            text = text.replace(new RegExp(`\\{${pKey}\\}`, 'g'), pVal);
        }
    }

    return text;
}

const ERROR_KEY_MAP = {
    'Confirm Action': 'confirm.title',
    'Confirm': 'confirm.confirmBtn',
    'Cancel': 'confirm.cancelBtn',
    'OK': 'common.ok',
    'Close modal': 'modal.close',
    'Input Required': 'prompt.title',
    'Click to copy': 'prompt.clickToCopy',
    'Copied to clipboard!': 'prompt.copied',
    'Invalid credentials': 'login.invalidCreds',
    'An error occurred during login': 'login.loginError',
    'Failed to delete file.': 'browser.failedDelete',
    'Failed to delete repository': 'repos.failedDelete',
    'Failed to delete token': 'users.failedDeleteToken',
    'Failed to regenerate token': 'users.failedRegenToken',
    'Failed to save user': 'users.failedSaveUser',
    'Failed to update password': 'profile.updatePasswordFailed',
    'Error updating password': 'profile.updatePasswordError',
    'Failed to generate token': 'profile.genTokenFailed',
    'Error generating token': 'profile.genTokenError',
    'Failed to load policy.': 'privacy.failedLoad',
    'Close mirrors': 'details.closeMirrors',
    'Unauthorized': 'error.unauthorized',
    'Forbidden': 'error.forbidden',
    'Bad Request': 'error.badRequest',
    'Bad request': 'error.badRequest',
    'Not found': 'error.notFound',
    'Not Found': 'error.notFound',
    'Internal Server Error': 'error.internalServerError',
    'Password must be between 6 and 72 bytes': 'error.passwordLength',
    'Failed to update token': 'error.failedUpdateToken',
    'Invalid mode. Expected \'full\' or \'diff\'': 'error.invalidRebuildMode',
    'URL must be http or https': 'error.urlScheme',
    'Invalid URL': 'error.invalidUrl',
    'Could not resolve host': 'error.resolveHost',
    'URL points to an internal or private IP': 'error.internalIp',
    'Failed to access background URL or returned non-success status': 'error.accessBgUrl',
    'Failed to read chunk': 'error.readChunk',
    'Background image exceeds 5 MiB': 'error.bgExceedsSize',
    'Background URL must be a valid WebP image': 'error.bgMustBeWebp',
    'Invalid repository name': 'error.invalidRepoName',
    'Max Javadocs size limit must be positive': 'error.maxJavadocSizePositive',
    'Token name already exists': 'error.tokenExists',
    'Failed to save token': 'error.failedSaveToken',
    'Installation already in progress': 'error.installInProgress',
    'Task submission failed': 'error.taskFailed',
    'No update ready to install': 'error.noUpdateReady',
    'No update package file uploaded': 'error.noUpdatePackageUploaded',
    'Uploaded file must be a .zip package': 'error.mustBeZipPackage',
    'Executable binary does not match current system or architecture': 'error.incompatibleBinary',
    'Target executable not found in update package': 'error.targetExeNotFound'
};

/**
 * Match and translate backend error messages or system messages.
 * Handles JSON error payloads, known phrase maps, and `prefix: rest` splitting.
 * @param {string} errorText - Raw error text from API or UI.
 * @returns {string} Localized message, or the original text when unmapped.
 */
export function translateError(errorText) {
    if (!errorText || typeof errorText !== 'string') return errorText;
    let trimmed = errorText.trim();

    if (trimmed.startsWith('{') && trimmed.endsWith('}')) {
        try {
            const parsed = JSON.parse(trimmed);
            if (parsed.error && typeof parsed.error === 'string') {
                trimmed = parsed.error.trim();
            } else if (parsed.message && typeof parsed.message === 'string') {
                trimmed = parsed.message.trim();
            }
        } catch (e) {
        }
    }

    if (ERROR_KEY_MAP[trimmed]) {
        return t(ERROR_KEY_MAP[trimmed]);
    }

    const directTranslation = t(trimmed);
    if (directTranslation !== trimmed) {
        return directTranslation;
    }

    const prefixSplitter = ': ';
    if (trimmed.includes(prefixSplitter)) {
        const parts = trimmed.split(prefixSplitter);
        const prefix = parts[0].trim();
        const rest = parts.slice(1).join(prefixSplitter).trim();

        const translatedPrefix = ERROR_KEY_MAP[prefix] ? t(ERROR_KEY_MAP[prefix]) : t(prefix);
        const translatedRest = ERROR_KEY_MAP[rest] ? t(ERROR_KEY_MAP[rest]) : t(rest);

        if (translatedPrefix !== prefix || translatedRest !== rest) {
            return `${translatedPrefix}: ${translatedRest}`;
        }
    }

    return trimmed;
}

/**
 * Update all DOM elements with data-i18n* attributes and refresh language UI.
 * @returns {void}
 */
export function updatePageTranslations() {
    document.querySelectorAll('[data-i18n]').forEach(el => {
        const key = el.getAttribute('data-i18n');
        const translation = t(key);
        if (translation !== key) {
            el.textContent = translation;
        }
    });

    document.querySelectorAll('[data-i18n-placeholder]').forEach(el => {
        const key = el.getAttribute('data-i18n-placeholder');
        const translation = t(key);
        if (translation !== key) {
            el.placeholder = translation;
        }
    });

    document.querySelectorAll('[data-i18n-title]').forEach(el => {
        const key = el.getAttribute('data-i18n-title');
        const translation = t(key);
        if (translation !== key) {
            el.title = translation;
        }
    });

    document.querySelectorAll('[data-i18n-aria-label]').forEach(el => {
        const key = el.getAttribute('data-i18n-aria-label');
        const translation = t(key);
        if (translation !== key) {
            el.setAttribute('aria-label', translation);
        }
    });

    updateLanguageUI();
}

/**
 * Set the active language, persist it, update the page, and emit `languageChanged`.
 * Falls back to the default language when the code is unsupported.
 * @param {string} lang - Language code or alias to activate.
 * @returns {string} Resolved language code that is now active.
 */
export function setLanguage(lang) {
    const resolved = resolveLanguage(lang);
    const available = getAvailableLanguages();

    if (!resolved) {
        console.warn(`[i18n] Language '${lang}' is not supported. Defaulting to '${DEFAULT_LANG}'. Available languages: ${available.join(', ')}`);
        currentLang = DEFAULT_LANG;
        currentSource = 'fallback';
        localStorage.setItem(STORAGE_KEY, DEFAULT_LANG);
        const langSelect = document.getElementById('lang-select');
        if (langSelect && langSelect.value !== DEFAULT_LANG) {
            langSelect.value = DEFAULT_LANG;
        }
        updatePageTranslations();
        return DEFAULT_LANG;
    }

    currentLang = resolved;
    currentSource = 'user-set';
    localStorage.setItem(STORAGE_KEY, resolved);
    const langSelect = document.getElementById('lang-select');
    if (langSelect && langSelect.value !== resolved) {
        langSelect.value = resolved;
    }
    updatePageTranslations();
    window.dispatchEvent(new CustomEvent('languageChanged', {detail: {lang: resolved}}));
    console.log(`[i18n] Language changed to '${resolved}'.`);
    return resolved;
}

/**
 * Return (and log) the current language status for console / debugging.
 * @returns {{current: string, source: string, available: string[]}} Status snapshot.
 */
export function getLanguage() {
    const status = {
        current: currentLang,
        source: currentSource,
        available: getAvailableLanguages()
    };
    console.log('[i18n] Current language status:', status);
    return status;
}

/**
 * Initialize the i18n subsystem: detect language, expose window helpers, wire the language modal.
 * @returns {string} Active language code after initialization.
 */
export function initI18n() {
    const detected = detectLanguage();
    currentLang = detected.lang;
    currentSource = detected.source;

    window.setLanguage = setLanguage;
    window.getLanguage = getLanguage;
    window.getAvailableLanguages = getAvailableLanguages;
    window.getLanguageNames = getLanguageNames;
    window.getLanguageDetails = getLanguageDetails;
    window.translateError = translateError;
    window.i18n = {
        t,
        translateError,
        setLanguage,
        getLanguage,
        getAvailableLanguages,
        getLanguageNames,
        getLanguageDetails,
        currentLanguage: () => currentLang
    };

    const setupLanguageModal = () => {
        const langBtn = document.getElementById('lang-btn');
        const langModal = document.getElementById('language-modal');
        const langBackdrop = document.getElementById('language-backdrop');
        const closeBtn = document.getElementById('btn-close-language-modal');
        const langGrid = document.getElementById('language-grid');

        if (langGrid && langGrid.children.length === 0) {
            getAvailableLanguages().forEach(code => {
                const info = languageDetails[code] || {name: code, sub: '', code: code.substring(0, 2)};
                const card = createLangCard({
                    code,
                    name: info.name,
                    sub: info.sub,
                    active: code === currentLang,
                    onClick: () => {
                        setLanguage(code);
                        closeModal();
                    }
                });
                langGrid.appendChild(card);
            });
        }

        const openModal = () => {
            if (langModal) {
                if (langModal.dataset.isClosing === 'true') return;
                const backdrop = langModal.querySelector('.modal-backdrop');
                const content = langModal.querySelector('.modal-content');
                if (backdrop) backdrop.style.animation = 'backdropFadeIn 0.25s ease-out forwards';
                if (content) content.style.animation = 'modalFadeIn 0.3s cubic-bezier(0.16, 1, 0.3, 1) forwards';
                langModal.style.display = 'flex';
                if (typeof window.updateModalInertState === 'function') {
                    window.updateModalInertState();
                }
                updateLanguageUI();
            }
        };

        const closeModal = () => {
            if (!langModal || langModal.style.display === 'none' || langModal.dataset.isClosing === 'true') return;
            langModal.dataset.isClosing = 'true';
            const backdrop = langModal.querySelector('.modal-backdrop');
            const content = langModal.querySelector('.modal-content');

            if (backdrop) backdrop.style.animation = 'backdropFadeOut 0.2s ease-out forwards';
            if (content) content.style.animation = 'modalFadeOut 0.2s cubic-bezier(0.16, 1, 0.3, 1) forwards';

            setTimeout(() => {
                langModal.style.display = 'none';
                if (backdrop) backdrop.style.animation = '';
                if (content) content.style.animation = '';
                langModal.dataset.isClosing = 'false';
                if (typeof window.updateModalInertState === 'function') {
                    window.updateModalInertState();
                }
            }, 180);
        };

        if (langBtn) {
            langBtn.addEventListener('click', openModal);
        }
        if (closeBtn) {
            closeBtn.addEventListener('click', closeModal);
        }
        if (langBackdrop) {
            langBackdrop.addEventListener('click', closeModal);
        }

        document.addEventListener('keydown', (e) => {
            if (e.key === 'Escape' && langModal && langModal.style.display === 'flex') {
                closeModal();
            }
        });

        const langSelect = document.getElementById('lang-select');
        if (langSelect) {
            langSelect.value = currentLang;
            langSelect.addEventListener('change', (e) => {
                setLanguage(e.target.value);
            });
        }
    };

    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', setupLanguageModal);
    } else {
        setupLanguageModal();
    }

    updatePageTranslations();
    console.log(`[i18n] Initialized language '${currentLang}' (source: ${currentSource}). Available: ${getAvailableLanguages().join(', ')}.`);
    return currentLang;
}

