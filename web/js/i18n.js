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
import {createLangCard} from './components/lang-card.js';

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
    const dict = languages[currentLang] || languages[DEFAULT_LANG] || {};
    let text = dict[key] || languages[DEFAULT_LANG]?.[key] || key;

    if (typeof text === 'string' && params && typeof params === 'object') {
        for (const [pKey, pVal] of Object.entries(params)) {
            text = text.replace(new RegExp(`\\{${pKey}\\}`, 'g'), String(pVal));
        }
    }
    return text;
}

/**
 * Sync the language button label and modal active card with `currentLang`.
 * Animates the button width when the display name changes after first paint.
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
        grid.querySelectorAll('.lang-card').forEach((card) => {
            const lKey = card.getAttribute('data-lang');
            card.classList.toggle('active', lKey === currentLang);
        });
    }
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
    if (!lang || typeof lang !== 'string') return null;
    const clean = lang.trim();
    const cleanLower = clean.toLowerCase();

    for (const key of Object.keys(languages)) {
        if (key.toLowerCase() === cleanLower) {
            if (key === 'en' || key === 'en-US') return 'en-US';
            if (key.startsWith('zh') || key.startsWith('yue')) return 'zh-CN';
            if (key === 'ja' || key === 'ja-JP') return 'ja-JP';
            if (key === 'ru' || key === 'ru-RU') return 'ru-RU';
            if (key === 'fr' || key === 'fr-FR') return 'fr-FR';
            if (languageDetails[key]) return key;
        }
    }

    if (cleanLower.startsWith('zh') || cleanLower.startsWith('yue')) return 'zh-CN';
    if (cleanLower.startsWith('ja')) return 'ja-JP';
    if (cleanLower.startsWith('ru')) return 'ru-RU';
    if (cleanLower.startsWith('fr')) return 'fr-FR';
    if (cleanLower.startsWith('en')) return 'en-US';

    return null;
}

/**
 * Pick the initial language from localStorage, then browser preferences, then default.
 * @returns {string} Primary language code.
 */
function detectLanguage() {
    const saved = localStorage.getItem(STORAGE_KEY);
    if (saved) {
        const resolved = resolveLanguage(saved);
        if (resolved && languageDetails[resolved]) return resolved;
    }

    const browserLangs = navigator.languages?.length
        ? navigator.languages
        : [navigator.language];

    for (const bLang of browserLangs) {
        if (!bLang) continue;
        const resolved = resolveLanguage(bLang);
        if (resolved && languageDetails[resolved]) return resolved;
    }
    return DEFAULT_LANG;
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
 * Show the language picker modal.
 * @returns {void}
 */
function openLanguageModal() {
    const modal = document.getElementById('language-modal');
    if (!modal) return;
    populateLanguageGrid();
    modal.style.display = 'flex';
}

/**
 * Hide the language picker modal.
 * @returns {void}
 */
function closeLanguageModal() {
    const modal = document.getElementById('language-modal');
    if (modal) modal.style.display = 'none';
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

    updatePageTranslations();
    populateLanguageGrid();

    document.getElementById('lang-btn')?.addEventListener('click', openLanguageModal);
    document.getElementById('btn-close-language-modal')?.addEventListener('click', closeLanguageModal);
    document.getElementById('language-backdrop')?.addEventListener('click', closeLanguageModal);

    window.setLanguage = setLanguage;
    window.getLanguage = () => ({current: currentLang, available: getAvailableLanguages()});
}
