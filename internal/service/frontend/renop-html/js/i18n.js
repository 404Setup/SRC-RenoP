/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import defaultLocale, {loadLocale} from './i18n/catalog.generated.js';
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
    [DEFAULT_LANG]: defaultLocale,
};

const languageAliases = {
    'en-US': 'en-US',
    'en': 'en-US',
    'zh-CN': 'zh-CN',
    'zh-cn': 'zh-CN',
    'zh-Hans': 'zh-CN',
    'zh-hans': 'zh-CN',
    'zh': 'zh-CN',
    'zh-HK': 'zh-HK',
    'zh-hk': 'zh-HK',
    'zh-Hant-HK': 'zh-HK',
    'zh-hant-hk': 'zh-HK',
    'zh-TW': 'zh-TW',
    'zh-tw': 'zh-TW',
    'zh-Hant': 'zh-TW',
    'zh-hant': 'zh-TW',
    'zh-Hant-TW': 'zh-TW',
    'zh-hant-tw': 'zh-TW',
    'zh-YUE': 'zh-YUE',
    'zh-yue': 'zh-YUE',
    'yue': 'zh-YUE',
    'yue-HK': 'zh-YUE',
    'yue-hk': 'zh-YUE',
    'zh-Hant-HK-yue': 'zh-YUE',
    'ko-KR': 'ko-KR',
    'ko-kr': 'ko-KR',
    'ko': 'ko-KR',
    'ja-JP': 'ja-JP',
    'ja': 'ja-JP',
    'de-DE': 'de-DE',
    'de': 'de-DE',
    'fr-FR': 'fr-FR',
    'fr': 'fr-FR',
    'ru-RU': 'ru-RU',
    'ru-ru': 'ru-RU',
    'ru': 'ru-RU',
    'es-ES': 'es-ES',
    'es-es': 'es-ES',
    'es': 'es-ES',
    'pt-PT': 'pt-PT',
    'pt-pt': 'pt-PT',
    'pt-BR': 'pt-PT',
    'pt-br': 'pt-PT',
    'pt': 'pt-PT'
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
let translationObserver = null;
let translationFlushScheduled = false;
let languageRequestID = 0;
const pendingTranslationRoots = new Set();
const translationBindings = Object.freeze([
    {attribute: 'data-i18n', target: 'text'},
    {attribute: 'data-i18n-placeholder', target: 'placeholder'},
    {attribute: 'data-i18n-title', target: 'title'},
    {attribute: 'data-i18n-aria-label', target: 'aria-label'},
]);
const translationSelector = translationBindings.map(binding => `[${binding.attribute}]`).join(',');

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
 * Expose language-catalog loading state in the picker and prevent duplicate selections.
 * @param {boolean} loading - Whether a locale request is active.
 * @returns {void}
 */
function setLanguageLoading(loading) {
    const modal = document.getElementById('language-modal');
    const progress = document.getElementById('language-load-progress');
    if (modal) modal.toggleAttribute('aria-busy', loading);
    if (progress) progress.hidden = !loading;
    document.querySelectorAll('#language-grid .lang-card').forEach(card => {
        card.disabled = loading;
    });
}

/**
 * Normalize and match a language code to a supported entry in the registry.
 * @param {string} lang - Raw language tag (e.g. 'zh', 'en-US', 'yue-HK').
 * @returns {string|null} Matched registry key, or null if unsupported.
 */
function resolveLanguage(lang) {
    const matched = matchLocaleKey(lang, languageAliases);
    return matched ? languageAliases[matched] : null;
}

/**
 * Ensure one canonical language dictionary is available.
 * @param {string} lang - Canonical language identifier.
 * @returns {Promise<void>}
 */
async function ensureLanguage(lang) {
    if (languages[lang]) return;
    languages[lang] = await loadLocale(lang);
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

/**
 * Apply every declarative translation binding present on one element.
 * @param {Element} element - Element carrying one or more `data-i18n*` attributes.
 * @returns {void}
 */
function translateBoundElement(element) {
    for (const binding of translationBindings) {
        const key = element.getAttribute(binding.attribute);
        if (!key) continue;
        const translation = t(key);
        if (translation === key) continue;
        if (binding.target === 'text') {
            if (element.textContent !== translation) element.textContent = translation;
        } else if (binding.target === 'placeholder') {
            if (element.getAttribute('placeholder') !== translation) element.setAttribute('placeholder', translation);
        } else if (binding.target === 'title') {
            if (element.getAttribute('title') !== translation) element.setAttribute('title', translation);
        } else if (element.getAttribute('aria-label') !== translation) {
            element.setAttribute('aria-label', translation);
        }
    }
}

/**
 * Translate one newly rendered subtree without rescanning unrelated page content.
 * @param {Document|DocumentFragment|Element} root - Root whose bindings should be refreshed.
 * @returns {void}
 */
function translateSubtree(root) {
    if (!root || typeof root.querySelectorAll !== 'function') return;
    if (root instanceof Element && root.matches(translationSelector)) {
        translateBoundElement(root);
    }
    root.querySelectorAll(translationSelector).forEach(translateBoundElement);
}

/**
 * Queue a minimal translation root and coalesce nested asynchronous DOM insertions.
 * @param {Node} node - Node added to the observed application DOM.
 * @returns {void}
 */
function queueTranslationRoot(node) {
    if (!(node instanceof Element)) return;
    for (const root of pendingTranslationRoots) {
        if (root.contains(node)) return;
        if (node.contains(root)) pendingTranslationRoots.delete(root);
    }
    pendingTranslationRoots.add(node);
    if (translationFlushScheduled) return;
    translationFlushScheduled = true;
    queueMicrotask(() => {
        translationFlushScheduled = false;
        const roots = Array.from(pendingTranslationRoots);
        pendingTranslationRoots.clear();
        roots.forEach(translateSubtree);
    });
}

/**
 * Observe asynchronous UI rendering so declarative translations never wait for a page refresh.
 * @returns {void}
 */
function startTranslationObserver() {
    if (translationObserver || !document.body || typeof MutationObserver === 'undefined') return;
    translationObserver = new MutationObserver(records => {
        for (const record of records) {
            if (record.type === 'attributes') {
                queueTranslationRoot(record.target);
                continue;
            }
            record.addedNodes.forEach(queueTranslationRoot);
        }
    });
    translationObserver.observe(document.body, {
        childList: true,
        subtree: true,
        attributes: true,
        attributeFilter: translationBindings.map(binding => binding.attribute),
    });
}

const ERROR_KEY_MAP = {
    'A user may register at most 10 GPG keys': 'profile.gpgKeyLimit',
    'An error occurred during FIDO login': 'error.fidoLoginError',
    'An error occurred during login': 'login.loginError',
    'An unexpected error occurred while attempting to encode a Protobuf message': 'error.internalServerError',
    'Authentication failed': 'login.loginError',
    'Artifact or directory was deleted before publication': 'profile.gpgFailure.deletedBeforePublication',
    'Artifact redeployment is not allowed': 'profile.gpgFailure.redeploymentDenied',
    'Detached GPG signature was not uploaded before the publication deadline': 'profile.gpgFailure.signatureDeadline',
    'Failed to register GPG key': 'profile.gpgAddFailed',
    'GPG key could not be resolved by configured key servers': 'profile.gpgResolveFailed',
    'GPG key ID is ambiguous; use the full fingerprint': 'profile.gpgAmbiguousKey',
    'GPG key server URLs must contain only an HTTPS origin': 'settings.gpgOriginOnly',
    'GPG key server URLs must use HTTPS': 'settings.gpgHttpsOnly',
    'GPG validation timed out': 'profile.gpgFailure.validationTimedOut',
    'Invalid GPG key ID or fingerprint': 'profile.gpgInvalidKey',
    'Invalid profile links': 'profile.linksInvalid',
    'Maven artifact was not uploaded before the publication deadline': 'profile.gpgFailure.artifactDeadline',
    'Publication failed during GPG validation or storage commit': 'profile.gpgFailure.generic',
    'Failed to update profile links': 'profile.linksUpdateFailed',
    'Repository was deleted before publication': 'profile.gpgFailure.repositoryDeleted',
    'The detached GPG signature is invalid': 'profile.gpgFailure.invalidSignature',
    'The quarantined artifact or signature is no longer available': 'profile.gpgFailure.quarantineMissing',
    'The signing key is not registered for the uploader': 'profile.gpgFailure.keyUnregistered',
    'The target repository no longer exists': 'profile.gpgFailure.repositoryMissing',
    'Uploader account was deleted': 'profile.gpgFailure.uploaderDeleted',
    'at least one GPG key server is required': 'settings.gpgAtLeastOne',
    'at most 8 GPG key servers are allowed': 'settings.gpgAtMostEight',
    'at most 16 global proxies are allowed': 'settings.proxyAtMostSixteen',
    'global proxy configuration is too long': 'settings.proxyInvalidConfig',
    'global proxy credentials must use the username and password fields': 'settings.proxyInvalidUrl',
    'global proxy name is invalid': 'settings.proxyInvalidConfig',
    'global proxy name is required': 'settings.proxyNameRequired',
    'global proxy names must be unique': 'settings.proxyNamesUnique',
    'global proxy URL is invalid': 'settings.proxyInvalidUrl',
    'global proxy URL must not contain a path, query, or fragment': 'settings.proxyInvalidUrl',
    'global proxy URL must use http, https, or socks5': 'settings.proxyInvalidUrl',
    'invalid GPG key server URL': 'settings.gpgInvalidServer',
    'selected global proxy does not exist': 'settings.proxySelectedMissing',
    'SOCKS5 global proxy URL must include a port': 'settings.proxyInvalidUrl',
    'storage path cannot be changed while GPG publications are pending': 'settings.gpgPendingStorageChange',
    'Background image exceeds 5 MiB': 'error.bgExceedsSize',
    'Background URL must be a valid WebP image': 'error.bgMustBeWebp',
    'Bad path': 'error.badPath',
    'Bad request': 'error.badRequest',
    'Bad Request': 'error.badRequest',
    'Cancel': 'confirm.cancelBtn',
    'Cannot delete current account': 'error.cannotDeleteCurrentAccount',
    'Click to copy': 'prompt.clickToCopy',
    'Close mirrors': 'details.closeMirrors',
    'Close modal': 'modal.close',
    'Confirm': 'confirm.confirmBtn',
    'Confirm Action': 'confirm.title',
    'Conflict': 'error.conflict',
    'Copied to clipboard!': 'prompt.copied',
    'Could not resolve host': 'error.couldNotResolveHost',
    'Database DSN must not be empty': 'error.dbDsnEmpty',
    'Debug mode is not active (enable server.debug_mode and restart)': 'error.debugModeInactive',
    'Error': 'common.error',
    'Error generating token': 'profile.genTokenError',
    'Error loading FIDO devices': 'error.fidoLoadFailed',
    'Error updating password': 'profile.updatePasswordError',
    'Executable binary does not match current system or architecture': 'error.incompatibleBinary',
    'Failed to access background URL or returned non-success status': 'error.failedAccessBackgroundUrl',
    'Failed to begin FIDO login': 'error.fidoBeginLoginFailed',
    'Failed to begin registration': 'error.fidoBeginRegFailed',
    'Failed to create session': 'error.failedCreateSession',
    'Failed to delete FIDO device': 'error.fidoDeleteFailed',
    'Failed to delete file.': 'browser.failedDelete',
    'Failed to delete repository': 'repos.failedDelete',
    'Failed to delete token': 'users.failedDeleteToken',
    'Failed to generate token': 'profile.genTokenFailed',
    'Failed to load policy.': 'privacy.failedLoad',
    'Failed to parse assertion response': 'error.fidoParseAssertionFailed',
    'Failed to parse creation response': 'error.fidoParseCreationFailed',
    'Failed to read chunk': 'error.readChunk',
    'Failed to revoke session': 'sessions.revokeFailed',
    'Failed to revoke sessions': 'sessions.revokeOthersFailed',
    'Failed to save FIDO device': 'error.failedSaveFidoDevice',
    'Failed to save token': 'error.failedSaveToken',
    'Failed to save user': 'users.failedSaveUser',
    'Failed to update FIDO device': 'error.failedUpdateFidoDevice',
    'Failed to update password': 'profile.updatePasswordFailed',
    'Failed to update token': 'error.failedUpdateToken',
    'FIDO authentication failed': 'error.fidoAuthFailed',
    'FIDO credential not found': 'error.fidoCredNotFound',
    'File not found on S3': 'error.notFound',
    'Forbidden': 'error.forbidden',
    'Input Required': 'prompt.title',
    'Installation already in progress': 'error.installInProgress',
    'Insufficient disk space': 'error.insufficientStorage',
    'Insufficient disk space to download update package': 'updater.insufficientDiskSpace',
    'Insufficient disk space to generate POM': 'error.insufficientStorage',
    'Insufficient disk space to upload file': 'error.insufficientStorage',
    'Insufficient disk space to upload update package': 'updater.insufficientDiskSpace',
    'Insufficient storage': 'error.insufficientStorage',
    'Internal Server Error': 'error.internalServerError',
    'Invalid channel': 'error.invalidChannel',
    'Invalid credential payload': 'error.invalidCredentialPayload',
    'Invalid credentials': 'login.invalidCreds',
    'Invalid database driver': 'error.invalidDbDriver',
    'Invalid domain': 'error.invalidDomain',
    'Invalid domain.': 'error.invalidDomain',
    'Invalid FIDO credential': 'error.invalidFidoCred',
    'Invalid mode': 'error.invalidMode',
    'Invalid mode. Expected \'full\' or \'diff\'': 'error.invalidRebuildMode',
    'Invalid or expired login session': 'error.invalidOrExpiredLoginSession',
    'Invalid or expired session': 'error.invalidOrExpiredSession',
    'Invalid repository name': 'error.invalidRepoName',
    'Invalid S3 key prefix': 'error.invalidS3KeyPrefix',
    'Invalid URL': 'error.invalidUrl',
    'Invalid URL host': 'error.invalidUrlHost',
    'Invalid visibility. Expected PUBLIC, HIDDEN, or PRIVATE': 'error.invalidVisibility',
    'Is a dir': 'error.isDirectory',
    'Javadocs preview is not enabled on this RenoP instance.': 'error.javadocsPreviewDisabled',
    'Max active requests must be positive': 'error.maxActiveRequestsPositive',
    'Max Javadocs size limit must be positive': 'error.maxJavadocSizePositive',
    'Method not allowed': 'error.methodNotAllowed',
    'No update package file uploaded': 'error.noUpdatePackageUploaded',
    'No update ready to install': 'error.noUpdateReady',
    'Not found': 'error.notFound',
    'Not Found': 'error.notFound',
    'OK': 'common.ok',
    'Password must be between 6 and 72 bytes': 'error.passwordLength',
    'Port must be between 1 and 65535': 'error.invalidPort',
    'profile not found': 'error.notFound',
    'Registration failed': 'error.fidoRegFailed',
    'Repository not found': 'error.notFound',
    'Request Entity Too Large': 'error.requestEntityTooLarge',
    'Service Unavailable': 'error.serviceUnavailable',
    'Session not found': 'error.sessionNotFound',
    'Storage path must not be empty': 'error.storagePathEmpty',
    'Target executable not found in update package': 'error.targetExeNotFound',
    'Task submission failed': 'error.taskFailed',
    'This is an invalid domain': 'error.invalidDomain',
    'This is an invalid domain.': 'error.invalidDomain',
    'Token name already exists': 'error.tokenExists',
    'Too Many Requests': 'error.tooManyRequests',
    'Unauthorized': 'error.unauthorized',
    'Uploaded file must be a .zip package': 'error.mustBeZipPackage',
    'Uploaded file must be a .br or .zip package': 'error.mustBeZipPackage',
    'URL must be http or https': 'error.urlMustBeHttpOrHttps',
    'URL must not contain credentials': 'error.urlNoCredentials',
    'URL points to an internal or private IP': 'error.urlInternalIp',
    'Users cannot clear their own activity logs': 'error.cannotClearOwnAuditLogs',
    'WebAuthn initialization failed': 'error.webauthnInitFailed',
};

/**
 * Translate an exact backend error only when it has a registered locale entry.
 * Unknown response text is intentionally rejected so callers can use a safe,
 * localized fallback instead of exposing server internals.
 * @param {string} errorText - Bounded backend error payload.
 * @returns {string} Localized error, or an empty string when the payload is unknown.
 */
export function translateKnownError(errorText) {
    if (!errorText || typeof errorText !== 'string') return '';
    let normalized = errorText.trim();
    if (normalized.startsWith('{') && normalized.endsWith('}')) {
        try {
            const parsed = JSON.parse(normalized);
            if (parsed.error && typeof parsed.error === 'string') normalized = parsed.error.trim();
            else if (parsed.message && typeof parsed.message === 'string') normalized = parsed.message.trim();
        } catch {
            return '';
        }
    }

    const stripped = normalized.replace(/[\.:]+$/, '').trim();
    for (const candidate of normalized === stripped ? [normalized] : [normalized, stripped]) {
        const mappedKey = ERROR_KEY_MAP[candidate];
        if (mappedKey) return t(mappedKey);
        const direct = t(candidate);
        if (direct !== candidate) return direct;
    }
    return '';
}

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
    translateSubtree(document);
    updateLanguageUI();
    document.documentElement.lang = currentLang;
    document.documentElement.dataset.i18nReady = 'true';
}

/**
 * Set the active language, persist it, update the page, and emit `languageChanged`.
 * Falls back to the default language when the code is unsupported.
 * @param {string} lang - Language code or alias to activate.
 * @returns {Promise<string>} Resolved language code that is now active.
 */
export async function setLanguage(lang) {
    let resolved = resolveLanguage(lang);
    let source = 'user-set';
    const available = getAvailableLanguages();
    const requestID = ++languageRequestID;
    setLanguageLoading(true);

    try {
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

        try {
            await ensureLanguage(resolved);
        } catch (error) {
            console.error(`[i18n] Failed to load language '${resolved}'.`, error);
            resolved = DEFAULT_LANG;
            source = 'fallback';
        }
        if (requestID !== languageRequestID) return currentLang;

        currentLang = resolved;
        currentSource = source;
        localStorage.setItem(STORAGE_KEY, resolved);
        const langSelect = document.getElementById('lang-select');
        if (langSelect && langSelect.value !== resolved) {
            langSelect.value = resolved;
        }
        updatePageTranslations();
        window.dispatchEvent(new CustomEvent('languageChanged', {detail: {lang: resolved}}));
        console.log(`[i18n] Language changed to '${resolved}'.`);
        return resolved;
    } finally {
        if (requestID === languageRequestID) setLanguageLoading(false);
    }
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
 * @returns {Promise<string>} Active language code after initialization.
 */
export async function initI18n() {
    const detected = detectLanguage();
    currentLang = detected.lang;
    currentSource = detected.source;
    try {
        await ensureLanguage(currentLang);
    } catch (error) {
        console.error(`[i18n] Failed to load detected language '${currentLang}'.`, error);
        currentLang = DEFAULT_LANG;
        currentSource = 'fallback';
    }

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
                    onClick: async () => {
                        await setLanguage(code);
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
                void setLanguage(e.target.value);
            });
        }
        startTranslationObserver();
        updatePageTranslations();
    };

    if (!document.body) {
        document.addEventListener('DOMContentLoaded', setupLanguageModal);
    } else {
        setupLanguageModal();
    }

    console.log(`[i18n] Initialized language '${currentLang}' (source: ${currentSource}). Available: ${getAvailableLanguages().join(', ')}.`);
    return currentLang;
}
