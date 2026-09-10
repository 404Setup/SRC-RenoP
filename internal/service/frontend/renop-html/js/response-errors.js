/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {t, translateKnownError} from './i18n.js';

export const RESPONSE_ERROR_CODE_HEADER = 'X-Renop-Error-Code';

const MAX_ERROR_BODY_BYTES = 2048;
const commonErrorCodeKeys = Object.freeze({
    captcha_required: 'captcha.complete',
    captcha_invalid: 'captcha.failed',
    captcha_unavailable: 'captcha.unavailable',
    captcha_changed: 'captcha.changed',
    captcha_cookies_required: 'captcha.cookiesRequired',
    captcha_settings_invalid: 'captcha.settingsInvalid',
    captcha_settings_save_failed: 'captcha.settingsSaveFailed',
    legal_consent_required: 'legal.consentRequired',
    legal_settings_invalid: 'legal.invalid',
    legal_settings_save_failed: 'legal.saveFailed',
    oauth_invalid: 'oauth.failed',
    oauth_unavailable: 'oauth.failed',
    oauth_last_login_method: 'oauth.onlyLogin',
    oauth_settings_invalid: 'oauth.settingsInvalid',
    oauth_settings_save_failed: 'oauth.failed',
    registration_disabled: 'registration.disabled',
    registration_ip_limited: 'registration.ipLimited',
    registration_cooldown: 'registration.cooldown',
    registration_pending: 'registration.pending',
    registration_invalid: 'registration.invalid',
    registration_unavailable: 'registration.unavailable',
    registration_username_conflict: 'registration.usernameConflict',
    registration_identity_linked: 'registration.identityLinked',
    registration_settings_invalid: 'registration.settingsInvalid',
    registration_settings_save_failed: 'registration.unavailable',
    MFA_REQUIRED: 'mfa.loginHint',
    MFA_PRIMARY_REQUIRED: 'mfa.primaryRequired',
    MFA_INVALID: 'mfa.invalid',
    MFA_UNAVAILABLE: 'mfa.unavailable',
    MFA_REAUTH_REQUIRED: 'mfa.reauth',
    cache_settings_invalid: 'cache.invalid',
    cache_settings_save_failed: 'cache.saveFailed',
    cache_connection_failed: 'cache.testFailed',
    mail_settings_invalid: 'mail.invalid',
    mail_settings_save_failed: 'mail.requestFailed',
    mail_request_invalid: 'mail.invalid',
    mail_unavailable: 'mail.requestFailed',
    mail_status_unavailable: 'mail.requestFailed',
    mail_disabled: 'mail.disabled',
    mail_manual_rate_limited: 'mail.rateLimited',
    mail_queue_full: 'mail.queueFull',
    mail_no_account: 'mail.noAccount',
    mail_recipient_blocked: 'mail.recipientBlocked',
    ACCOUNT_BANNED: 'login.accountBanned',
    LOG_FILTER_INVALID: 'audit.invalidFilter',
    ACCOUNT_DELETED: 'login.accountDeleted',
    ACCOUNT_RETIREMENT_BLOCKED: 'profile.retireBlockedHint',
    ACCOUNT_RETIREMENT_CONFIRMATION: 'profile.retireConfirmationMismatch',
    ACCOUNT_BAN_INVALID: 'users.banInvalid',
    ACCOUNT_BAN_SELF: 'users.banSelf',
    ACCOUNT_BAN_PROTECTED: 'users.banProtected',
    ACCOUNT_BAN_IP_UNKNOWN: 'users.banIPUnknown',
    IP_BANNED: 'login.ipBanned',
    ACCOUNT_EMAIL_CONFLICT: 'profile.privateEmailConflict',
    ACCOUNT_LOCALE_INVALID: 'profile.languageSaveFailed',
    ACCOUNT_EMAIL_PROOF_REQUIRED: 'profile.providerEmailUnverified',
    ACCOUNT_EMAIL_LIMIT: 'profile.emailAliasLimit',
    ACCOUNT_EMAIL_PRIMARY: 'profile.primaryEmailRequired',
    ACCOUNT_EMAIL_INVALID: 'profile.privateEmailInvalid',
    ACCOUNT_EMAIL_CODE_INVALID: 'login.emailCodeInvalid',
    ACCOUNT_LAST_LOGIN_METHOD: 'profile.passwordLoginNeedsAlternative',
    ACCOUNT_PASSWORD_NOT_CONFIGURED: 'profile.passwordLoginNotConfigured',
    API_TOKEN_INVALID: 'profile.apiTokenCreateFailed',
    API_TOKEN_LIMIT: 'profile.apiTokenLimitReached',
    API_TOKEN_NAME_CONFLICT: 'profile.apiTokenNameConflict',
    GITHUB_LAST_LOGIN_METHOD: 'profile.githubOnlyLogin',
    MAVEN_USER_NOT_FOUND: 'maven.userNotFound',
    package_deprecated: 'package.deprecationNotice',
    resource_locked: 'resourceLock.blocked',
    review_pending: 'package.reviewPending',
    maven_domain_closed: 'maven.domainClosedError',
    maven_domain_locked: 'maven.domainLockedError',
    maven_domain_review_pending: 'maven.domainReviewPending',
    maven_claim_review_invalid: 'maven.claimReviewInvalid',
    publication_file_quota: 'publicationQuota.fileExceeded',
    publication_byte_quota: 'publicationQuota.byteExceeded',
    publication_count_quota: 'publicationQuota.countExceeded',
    repository_migration_pending_gpg: 'repos.migrationPendingGpg',
    repository_migration_pending_review: 'repos.migrationPendingReview',
    repository_pending_review: 'repos.pendingReviewMutation',
});
const statusErrorKeys = Object.freeze({
    401: 'error.unauthorized',
    403: 'error.forbidden',
    404: 'error.notFound',
    409: 'error.conflict',
    413: 'error.requestEntityTooLarge',
    429: 'error.tooManyRequests',
    503: 'error.serviceUnavailable',
    507: 'error.insufficientStorage',
});

/**
 * Read at most the configured number of bytes from an error response.
 * @param {Response|null|undefined} response - Failed response to inspect.
 * @returns {Promise<string>} Bounded response text.
 */
async function readBoundedErrorText(response) {
    if (!response || response.bodyUsed) return '';
    if (!response.body?.getReader) return '';

    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    let output = '';
    let consumed = 0;
    let complete = false;
    try {
        while (consumed < MAX_ERROR_BODY_BYTES) {
            const {done, value} = await reader.read();
            if (done) {
                complete = true;
                break;
            }
            const remaining = MAX_ERROR_BODY_BYTES - consumed;
            const chunk = value.byteLength > remaining ? value.subarray(0, remaining) : value;
            consumed += chunk.byteLength;
            output += decoder.decode(chunk, {stream: true});
            if (chunk.byteLength !== value.byteLength) break;
        }
        output += decoder.decode();
        if (!complete) {
            try {
                await reader.cancel();
            } catch (error) {
                console.debug('Unable to cancel the bounded error response reader', error);
            }
        }
        return output;
    } finally {
        reader.releaseLock();
    }
}

/**
 * Resolve an HTTP failure without exposing unregistered backend text.
 * @param {Response|null|undefined} response - Failed response metadata and body.
 * @param {string} fallbackKey - Feature-specific translation key.
 * @param {Object.<string, *>} [params={}] - Interpolation parameters.
 * @param {Object.<string, string>} [errorCodeKeys={}] - Feature-specific stable-code mappings.
 * @returns {Promise<string>} Localized user-facing error.
 */
export async function responseErrorMessage(response, fallbackKey, params = {}, errorCodeKeys = {}) {
    const code = response?.headers?.get?.(RESPONSE_ERROR_CODE_HEADER) || '';
    const codeKey = errorCodeKeys[code] || commonErrorCodeKeys[code];
    if (codeKey) return t(codeKey, params);

    const translatedBody = translateKnownError(await readBoundedErrorText(response));
    if (translatedBody) return translatedBody;

    const statusKey = statusErrorKeys[Number(response?.status) || 0];
    return t(statusKey || fallbackKey, params);
}

/** Error whose message has already been resolved through the active locale. */
export class LocalizedResponseError extends Error {
    /**
     * Create a localized request error.
     * @param {string} message - Localized user-facing message.
     * @param {number} [status=0] - HTTP status retained for routing decisions.
     */
    constructor(message, status = 0) {
        super(message);
        this.name = 'LocalizedResponseError';
        this.status = Number(status) || 0;
    }
}

/**
 * Create an error from a failed response without retaining raw response text.
 * @param {Response|null|undefined} response - Failed response metadata and body.
 * @param {string} fallbackKey - Feature-specific translation key.
 * @param {Object.<string, *>} [params={}] - Interpolation parameters.
 * @param {Object.<string, string>} [errorCodeKeys={}] - Feature-specific stable-code mappings.
 * @returns {Promise<LocalizedResponseError>} Localized error instance.
 */
export async function localizedResponseError(response, fallbackKey, params = {}, errorCodeKeys = {}) {
    return new LocalizedResponseError(
        await responseErrorMessage(response, fallbackKey, params, errorCodeKeys),
        response?.status
    );
}

/**
 * Return a localized request error or a safe fallback for runtime failures.
 * @param {unknown} error - Caught value.
 * @param {string} fallbackKey - Translation key for non-response failures.
 * @param {Object.<string, *>} [params={}] - Interpolation parameters.
 * @returns {string} Localized user-facing message.
 */
export function caughtErrorMessage(error, fallbackKey, params = {}) {
    return error instanceof LocalizedResponseError ? error.message : t(fallbackKey, params);
}
