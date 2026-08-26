/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

import {t, translateKnownError} from './i18n.js';

export const RESPONSE_ERROR_CODE_HEADER = 'X-Renop-Error-Code';

const MAX_ERROR_BODY_BYTES = 2048;
const commonErrorCodeKeys = Object.freeze({
    ACCOUNT_EMAIL_CONFLICT: 'profile.privateEmailConflict',
    ACCOUNT_EMAIL_INVALID: 'profile.privateEmailInvalid',
    ACCOUNT_LAST_LOGIN_METHOD: 'profile.passwordLoginNeedsAlternative',
    ACCOUNT_PASSWORD_NOT_CONFIGURED: 'profile.passwordLoginNotConfigured',
    API_TOKEN_INVALID: 'profile.apiTokenCreateFailed',
    API_TOKEN_LIMIT: 'profile.apiTokenLimitReached',
    API_TOKEN_NAME_CONFLICT: 'profile.apiTokenNameConflict',
    GITHUB_LAST_LOGIN_METHOD: 'profile.githubOnlyLogin',
    MAVEN_USER_NOT_FOUND: 'maven.userNotFound',
    repository_migration_pending_gpg: 'repos.migrationPendingGpg',
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
