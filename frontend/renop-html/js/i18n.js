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
import {createLangCard} from '@renop/ui/lang-card';
import {bindModalChrome} from '@renop/ui/modal';
import {
    animateLangButtonLabel,
    detectLanguage as detectLanguageShared,
    matchLocaleKey,
    syncLangCardsActive,
    translateKey,
} from '@renop/ui/i18n-util';

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
    const details = languageDetails[currentLang] || languageDetails[DEFAULT_LANG];
    const newName = details ? details.name : currentLang;
    animateLangButtonLabel({
        nameEl: document.getElementById('current-lang-name'),
        btn: document.getElementById('lang-btn'),
        newName,
    });
    syncLangCardsActive(document.getElementById('language-grid'), currentLang);
}

/**
 * Normalize and match a language code to a supported entry in the registry.
 * @param {string} lang - Raw language tag (e.g. 'zh', 'en-US', 'yue-HK').
 * @returns {string|null} Matched registry key, or null if unsupported.
 */
function resolveLanguage(lang) {
    return matchLocaleKey(lang, languages);
}

/**
 * Detect preferred language from localStorage, then browser preferences, then default.
 * @returns {{lang: string, source: 'localStorage'|'browser'|'fallback'}} Detected language and source.
 */
function detectLanguage() {
    return detectLanguageShared({
        storageKey: STORAGE_KEY,
        defaultLang: DEFAULT_LANG,
        resolve: resolveLanguage,
    });
}

/**
 * Translate a key into the active language, with optional `{param}` interpolation.
 * @param {string} key - Message key (falls back to key text if missing).
 * @param {Object.<string, *>} [params={}] - Values substituted for `{name}` placeholders.
 * @returns {string} Translated (and interpolated) string.
 */
export function t(key, params = {}) {
    return translateKey(key, params, {
        current: languages[currentLang] || {},
        fallback: languages[DEFAULT_LANG] || {},
    });
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
    'Target executable not found in update package': 'error.targetExeNotFound',
    'WebAuthn initialization failed': 'error.webauthnInitFailed',
    'Failed to begin registration': 'error.fidoBeginRegFailed',
    'Invalid or expired session': 'error.invalidOrExpiredSession',
    'Invalid credential payload': 'error.invalidCredentialPayload',
    'Failed to parse creation response': 'error.fidoParseCreationFailed',
    'Registration failed': 'error.fidoRegFailed',
    'Failed to begin FIDO login': 'error.fidoBeginLoginFailed',
    'Invalid or expired login session': 'error.invalidOrExpiredLoginSession',
    'Failed to parse assertion response': 'error.fidoParseAssertionFailed',
    'FIDO credential not found': 'error.fidoCredNotFound',
    'FIDO authentication failed': 'error.fidoAuthFailed',
    'An error occurred during FIDO login': 'error.fidoLoginError',
    'Invalid FIDO credential': 'error.invalidFidoCred',
    'Failed to delete FIDO device': 'error.fidoDeleteFailed',
    'Error loading FIDO devices': 'error.fidoLoadFailed',
    'Bad path': 'error.badPath',
    'Insufficient storage': 'error.insufficientStorage',
    'Session not found': 'error.sessionNotFound',
    'This is an invalid domain': 'error.invalidDomain',
    'This is an invalid domain.': 'error.invalidDomain',
    'Invalid domain': 'error.invalidDomain',
    'Invalid domain.': 'error.invalidDomain',
    'Invalid database driver': 'error.invalidDbDriver',
    'Database DSN must not be empty': 'error.dbDsnEmpty',
    'Port must be between 1 and 65535': 'error.invalidPort',
    'Max active requests must be positive': 'error.maxActiveRequestsPositive',
    'Storage path must not be empty': 'error.storagePathEmpty',
    'Invalid channel': 'error.invalidChannel',
    'Invalid mode': 'error.invalidMode',
    'Invalid visibility. Expected PUBLIC, HIDDEN, or PRIVATE': 'error.invalidVisibility',
    'Invalid URL': 'error.invalidUrl',
    'URL must be http or https': 'error.urlMustBeHttpOrHttps',
    'URL must not contain credentials': 'error.urlNoCredentials',
    'Invalid URL host': 'error.invalidUrlHost',
    'Could not resolve host': 'error.couldNotResolveHost',
    'URL points to an internal or private IP': 'error.urlInternalIp',
    'Failed to access background URL or returned non-success status': 'error.failedAccessBackgroundUrl'
};

/**
 * Match and translate backend error messages or system messages.
 * Handles JSON error payloads, known phrase maps, trailing punctuation, and `prefix: rest` splitting.
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

    const tryTranslateSingle = (str) => {
        if (!str) return str;
        const clean = str.trim();
        if (ERROR_KEY_MAP[clean]) return t(ERROR_KEY_MAP[clean]);

        const stripped = clean.replace(/[\.\:]+$/, '').trim();
        if (ERROR_KEY_MAP[stripped]) return t(ERROR_KEY_MAP[stripped]);

        const direct = t(clean);
        if (direct !== clean) return direct;

        const strippedDirect = t(stripped);
        if (strippedDirect !== stripped) return strippedDirect;

        return clean;
    };

    const directRes = tryTranslateSingle(trimmed);
    const trimmedStripped = trimmed.replace(/[\.\:]+$/, '').trim();
    if (directRes !== trimmed && directRes !== trimmedStripped) {
        return directRes;
    }

    const prefixSplitter = ': ';
    if (trimmed.includes(prefixSplitter)) {
        const parts = trimmed.split(prefixSplitter);
        const prefix = parts[0].trim();
        const rest = parts.slice(1).join(prefixSplitter).trim();

        const translatedPrefix = tryTranslateSingle(prefix);
        const translatedRest = tryTranslateSingle(rest);

        if (translatedPrefix !== prefix || translatedRest !== rest) {
            return `${translatedPrefix}: ${translatedRest}`;
        }
    }

    return directRes;
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
        const langGrid = document.getElementById('language-grid');
        let closeModal = () => {
        };

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

        const chrome = bindModalChrome({
            modal: document.getElementById('language-modal'),
            openTriggers: [document.getElementById('lang-btn')],
            closeTriggers: [
                document.getElementById('btn-close-language-modal'),
                document.getElementById('language-backdrop'),
            ],
            onOpen: () => updateLanguageUI(),
        });
        if (chrome) closeModal = chrome.close;

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

