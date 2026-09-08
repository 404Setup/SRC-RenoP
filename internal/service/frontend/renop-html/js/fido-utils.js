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

/**
 * Convert base64 / base64url string to ArrayBuffer.
 * @param {string} base64url
 * @returns {ArrayBuffer}
 */
export function base64urlToBuffer(base64url) {
    if (!base64url) return new Uint8Array(0).buffer;
    let base64 = String(base64url).replace(/-/g, '+').replace(/_/g, '/');
    while (base64.length % 4 !== 0) {
        base64 += '=';
    }
    const binary = atob(base64);
    const bytes = new Uint8Array(binary.length);
    for (let i = 0; i < binary.length; i++) {
        bytes[i] = binary.charCodeAt(i);
    }
    return bytes.buffer;
}

/**
 * Convert ArrayBuffer / Uint8Array to base64url string.
 * @param {ArrayBuffer|Uint8Array} buffer
 * @returns {string}
 */
export function bufferToBase64url(buffer) {
    if (!buffer) return '';
    const bytes = new Uint8Array(buffer);
    let binary = '';
    for (let i = 0; i < bytes.byteLength; i++) {
        binary += String.fromCharCode(bytes[i]);
    }
    const base64 = btoa(binary);
    return base64.replace(/\+/g, '-').replace(/\//g, '_').replace(/=/g, '');
}

/** Map browser failures to safe, localized messages. */
export function passkeyErrorMessage(error) {
    const keys = {
        TimeoutError: 'fido.timeout', NotAllowedError: 'fido.cancelled', AbortError: 'fido.cancelled',
        InvalidStateError: 'fido.alreadyRegistered', NotSupportedError: 'login.fidoUnsupported',
        SecurityError: 'fido.insecure', ConstraintError: 'fido.verificationUnavailable',
    };
    return t(keys[error?.name] || 'login.fidoFailed');
}

/** Bound the native prompt even when a browser ignores its timeout or abort signal. */
async function requestCredential(method, publicKey, {signal, button} = {}) {
    if (!window.PublicKeyCredential || !navigator.credentials?.[method]) throw new DOMException('', 'NotSupportedError');
    if (window.isSecureContext === false) throw new DOMException('', 'SecurityError');
    const timeout = Math.min(Number(publicKey.timeout) > 0 ? Number(publicKey.timeout) : 60000, 60000);
    const expires = Date.now() + timeout;
    const controller = new AbortController();
    const cancel = () => controller.abort(new DOMException('', 'AbortError'));
    const original = button ? [...button.childNodes] : [];
    const busy = button?.getAttribute('aria-busy');
    const update = () => button?.replaceChildren(t('fido.waiting', {seconds: Math.max(0, Math.ceil((expires - Date.now()) / 1000))}));
    const timer = setTimeout(() => controller.abort(new DOMException('', 'TimeoutError')), timeout);
    const countdown = button ? setInterval(update, 1000) : undefined;
    let rejectAbort;
    try {
        button?.setAttribute('aria-busy', 'true');
        update();
        signal?.addEventListener('abort', cancel, {once: true});
        window.addEventListener('pagehide', cancel);
        window.addEventListener('popstate', cancel);
        if (signal?.aborted) cancel();
        controller.signal.throwIfAborted();
        const aborted = new Promise((_, reject) => {
            rejectAbort = () => reject(controller.signal.reason);
            controller.signal.addEventListener('abort', rejectAbort, {once: true});
        });
        const credential = await Promise.race([
            navigator.credentials[method]({publicKey: {...publicKey, timeout}, signal: controller.signal}), aborted,
        ]);
        controller.signal.throwIfAborted();
        if (Date.now() >= expires) throw new DOMException('', 'TimeoutError');
        if (!credential) throw new DOMException('', 'NotAllowedError');
        const fields = method === 'get' ? ['authenticatorData', 'clientDataJSON', 'signature'] : ['attestationObject', 'clientDataJSON'];
        if (credential.type !== 'public-key' || !credential.id || !credential.rawId?.byteLength ||
            fields.some(field => !credential.response?.[field]?.byteLength)) throw new DOMException('', 'DataError');
        return credential;
    } finally {
        clearTimeout(timer);
        clearInterval(countdown);
        signal?.removeEventListener('abort', cancel);
        window.removeEventListener('pagehide', cancel);
        window.removeEventListener('popstate', cancel);
        if (rejectAbort) controller.signal.removeEventListener('abort', rejectAbort);
        controller.abort();
        if (button) {
            button.replaceChildren(...original);
            if (busy === null) button.removeAttribute('aria-busy');
            else button.setAttribute('aria-busy', busy);
        }
    }
}

/** Request a Passkey assertion using the server's verification policy. */
export async function requestPasskeyAssertion(options, controls) {
    if (!options?.publicKey?.challenge) throw new DOMException('', 'DataError');
    const publicKey = {...options.publicKey, challenge: base64urlToBuffer(options.publicKey.challenge)};
    if (publicKey.allowCredentials?.length) {
        publicKey.allowCredentials = publicKey.allowCredentials.map(credential => ({...credential, id: base64urlToBuffer(credential.id)}));
    } else {
        delete publicKey.allowCredentials;
    }
    publicKey.userVerification ||= 'preferred';
    const assertion = await requestCredential('get', publicKey, controls);
    return {
        id: assertion.id, rawId: bufferToBase64url(assertion.rawId), type: assertion.type,
        response: {
            authenticatorData: bufferToBase64url(assertion.response.authenticatorData),
            clientDataJSON: bufferToBase64url(assertion.response.clientDataJSON),
            signature: bufferToBase64url(assertion.response.signature),
            userHandle: assertion.response.userHandle ? bufferToBase64url(assertion.response.userHandle) : null,
        },
    };
}

/** Register a Passkey with the same bounded prompt as authentication. */
export async function requestPasskeyRegistration(options, controls) {
    if (!options?.publicKey?.challenge || !options.publicKey.user?.id) throw new DOMException('', 'DataError');
    const publicKey = {...options.publicKey, challenge: base64urlToBuffer(options.publicKey.challenge),
        user: {...options.publicKey.user, id: base64urlToBuffer(options.publicKey.user.id)},
        authenticatorSelection: {...options.publicKey.authenticatorSelection},
    };
    if (publicKey.excludeCredentials?.length) {
        publicKey.excludeCredentials = publicKey.excludeCredentials.map(credential => ({...credential, id: base64urlToBuffer(credential.id)}));
    } else delete publicKey.excludeCredentials;
    delete publicKey.authenticatorSelection.authenticatorAttachment;
    publicKey.authenticatorSelection.userVerification ||= 'preferred';
    publicKey.authenticatorSelection.residentKey ||= 'preferred';
    const credential = await requestCredential('create', publicKey, controls);
    return {
        id: credential.id, rawId: bufferToBase64url(credential.rawId), type: credential.type,
        response: {attestationObject: bufferToBase64url(credential.response.attestationObject),
            clientDataJSON: bufferToBase64url(credential.response.clientDataJSON)},
    };
}
