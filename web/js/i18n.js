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
import zhCN from './i18n/zh-CN.js';
import jaJP from './i18n/ja-JP.js';
import ruRU from './i18n/ru-RU.js';
import frFR from './i18n/fr-FR.js';
import {createLangCard} from '@renop/ui/lang-card';
import {bindModalChrome, configureModalInert} from '@renop/ui/modal';
import {
    animateLangButtonLabel,
    detectLanguage as detectLanguageShared,
    matchLocaleKey,
    syncLangCardsActive,
    translateKey,
} from '@renop/ui/i18n-util';

const STORAGE_KEY = 'renop_web_language';
const DEFAULT_LANG = 'en-US';

/** Primary languages shown in the language modal (exactly five). */
const AVAILABLE_LANGS = ['zh-CN', 'en-US', 'ja-JP', 'ru-RU', 'fr-FR'];

const languages = {
    'en-US': enUS,
    en: enUS,
    'zh-CN': zhCN,
    'zh-cn': zhCN,
    'zh-Hans': zhCN,
    zh: zhCN,
    'zh-HK': zhCN,
    'zh-TW': zhCN,
    'zh-YUE': zhCN,
    'ja-JP': jaJP,
    ja: jaJP,
    'ru-RU': ruRU,
    ru: ruRU,
    'fr-FR': frFR,
    fr: frFR,
};

const languageDetails = {
    'zh-CN': {name: '简体中文', sub: 'Simplified Chinese', code: 'ZH'},
    'en-US': {name: 'English', sub: 'English (US)', code: 'EN'},
    'ja-JP': {name: '日本語', sub: 'Japanese', code: 'JA'},
    'ru-RU': {name: 'Русский', sub: 'Russian', code: 'RU'},
    'fr-FR': {name: 'Français', sub: 'French', code: 'FR'},
};

let currentLang = DEFAULT_LANG;

/**
 * @returns {string[]} Copy of primary language codes shown in the language modal.
 */
export function getAvailableLanguages() {
    return AVAILABLE_LANGS.slice();
}

/**
 * @returns {Record<string, { name: string, sub: string, code: string }>} Display metadata keyed by language code.
 */
export function getLanguageDetails() {
    return languageDetails;
}

/**
 * @returns {string} Active UI language code (e.g. `en-US`).
 */
export function getCurrentLang() {
    return currentLang;
}

/**
 * Translate a message key with optional `{param}` interpolation.
 * Falls back to the default language dictionary, then the key itself.
 * @param {string} key - Message key.
 * @param {Record<string, string|number>} [params={}] - Values for `{name}` placeholders.
 * @returns {string}
 */
export function t(key, params = {}) {
    return translateKey(key, params, {
        current: languages[currentLang] || {},
        fallback: languages[DEFAULT_LANG] || {},
    });
}

/**
 * Sync the language button label and modal active card with `currentLang`.
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
 * Docs content locale folder under `content/docs/{locale}/`.
 * Maps UI language codes onto available documentation trees.
 * @returns {string} Locale directory name (e.g. `zh-CN`, `en-US`).
 */
export function getDocsLocale() {
    if (
        currentLang === 'zh-CN' ||
        currentLang === 'zh-HK' ||
        currentLang === 'zh-TW' ||
        currentLang === 'zh-YUE'
    ) {
        return 'zh-CN';
    }
    if (
        currentLang === 'ja-JP' ||
        currentLang === 'ru-RU' ||
        currentLang === 'fr-FR' ||
        currentLang === 'en-US'
    ) {
        return currentLang;
    }
    return 'en-US';
}

/**
 * Apply translations to all elements with `data-i18n*` attributes and refresh language UI.
 * @returns {void}
 */
export function updatePageTranslations() {
    document.querySelectorAll('[data-i18n]').forEach((el) => {
        const key = el.getAttribute('data-i18n');
        const translation = t(key);
        if (translation !== key) el.textContent = translation;
    });

    document.querySelectorAll('[data-i18n-placeholder]').forEach((el) => {
        const key = el.getAttribute('data-i18n-placeholder');
        const translation = t(key);
        if (translation !== key) el.placeholder = translation;
    });

    document.querySelectorAll('[data-i18n-title]').forEach((el) => {
        const key = el.getAttribute('data-i18n-title');
        const translation = t(key);
        if (translation !== key) el.title = translation;
    });

    document.querySelectorAll('[data-i18n-aria-label]').forEach((el) => {
        const key = el.getAttribute('data-i18n-aria-label');
        const translation = t(key);
        if (translation !== key) el.setAttribute('aria-label', translation);
    });

    updateLanguageUI();
}

/**
 * Resolve browser / stored codes to one of the five primary languages.
 * @param {string|null|undefined} lang - Raw language tag (e.g. `zh`, `en-US`).
 * @returns {string|null} Primary code, or null if unrecognized.
 */
function resolveLanguage(lang) {
    const matched = matchLocaleKey(lang, languages);
    if (!matched) return null;
    if (matched === 'en' || matched === 'en-US') return 'en-US';
    if (matched.startsWith('zh') || matched.startsWith('yue')) return 'zh-CN';
    if (matched === 'ja' || matched === 'ja-JP') return 'ja-JP';
    if (matched === 'ru' || matched === 'ru-RU') return 'ru-RU';
    if (matched === 'fr' || matched === 'fr-FR') return 'fr-FR';
    return languageDetails[matched] ? matched : null;
}

/**
 * Pick the initial language from localStorage, then browser preferences, then default.
 * @returns {string} Primary language code.
 */
function detectLanguage() {
    const {lang} = detectLanguageShared({
        storageKey: STORAGE_KEY,
        defaultLang: DEFAULT_LANG,
        resolve: (raw) => {
            const resolved = resolveLanguage(raw);
            return resolved && languageDetails[resolved] ? resolved : null;
        },
    });
    return lang;
}

/**
 * Switch UI language, persist it, re-translate the page, and dispatch `languageChanged`.
 * @param {string} lang - Desired language code (aliases are resolved).
 * @returns {string} Resolved primary language that was applied.
 */
export function setLanguage(lang) {
    const resolved = resolveLanguage(lang) || DEFAULT_LANG;
    const primary = languageDetails[resolved] ? resolved : DEFAULT_LANG;
    currentLang = primary;
    localStorage.setItem(STORAGE_KEY, primary);
    updatePageTranslations();
    window.dispatchEvent(new CustomEvent('languageChanged', {detail: {lang: primary}}));
    return primary;
}

/** Closes the language modal (wired in `initI18n`). */
let closeLanguageModal = () => {};

/**
 * Rebuild language-selection cards inside `#language-grid`.
 * @returns {void}
 */
function populateLanguageGrid() {
    const grid = document.getElementById('language-grid');
    if (!grid) return;
    grid.innerHTML = '';
    for (const code of getAvailableLanguages()) {
        const d = languageDetails[code];
        grid.appendChild(
            createLangCard({
                code,
                name: d.name,
                sub: d.sub,
                active: code === currentLang,
                onClick: () => {
                    setLanguage(code);
                    closeLanguageModal();
                },
            }),
        );
    }
}

/**
 * Detect language, apply translations, wire modal controls, and expose `window.setLanguage` / `getLanguage`.
 * @returns {void}
 */
export function initI18n() {
    currentLang = detectLanguage();
    if (!languageDetails[currentLang]) {
        currentLang = DEFAULT_LANG;
    }

    configureModalInert({
        modalIds: ['language-modal'],
        rootSelectors: ['#app', '.top-nav', 'main'],
        installGlobal: true,
    });

    updatePageTranslations();
    populateLanguageGrid();

    const chrome = bindModalChrome({
        modal: document.getElementById('language-modal'),
        openTriggers: [document.getElementById('lang-btn')],
        closeTriggers: [
            document.getElementById('btn-close-language-modal'),
            document.getElementById('language-backdrop'),
        ],
        onOpen: () => populateLanguageGrid(),
    });
    if (chrome) closeLanguageModal = chrome.close;

    window.setLanguage = setLanguage;
    window.getLanguage = () => ({current: currentLang, available: getAvailableLanguages()});
}
