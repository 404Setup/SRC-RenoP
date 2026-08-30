/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {logout} from './auth.js';
import {protoObjectOptions} from './proto/index.js';

/** MIME type for protobuf request/response bodies (must match backend). */
export const PROTO_CONTENT_TYPE = 'application/x-protobuf';

/**
 * Build auth headers for API requests.
 * Browser sessions use a server-managed HttpOnly cookie; the session secret
 * is kept out of localStorage so scripts cannot access it.
 * @returns {Object} Empty headers object (credentials are sent via cookies).
 */
export function getAuthHeaders() {
    return {};
}

/**
 * Merge default fetch options so session cookies are always included.
 * @param {RequestInit} [options={}] - Additional fetch options to merge.
 * @returns {RequestInit} Options with credentials, explicit API cache bypass, and auth headers.
 */
export function withCredentials(options = {}) {
    return {
        credentials: 'include',
        cache: 'no-store',
        ...options,
        headers: {
            ...getAuthHeaders(),
            ...(options.headers || {}),
        },
    };
}

/**
 * Delegate authentication failures to the idempotent logout boundary.
 * @param {Response} response - Fetch response to inspect.
 * @param {{logoutOnForbidden?: boolean}} [policy={}] - Whether a 403 also proves the session is invalid; defaults to false.
 * @throws {Error} Always throws with message 'Unauthorized' on auth failure.
 */
function handleAuthFailure(response, policy = {}) {
    const {logoutOnForbidden = false} = policy;
    if (response.status === 401 || (response.status === 403 && logoutOnForbidden)) {
        void logout('kicked');
        throw new Error('Unauthorized');
    }
}

/**
 * Perform a fetch with credentials and handle auth failures.
 * @param {string} url - Request URL.
 * @param {RequestInit} [options={}] - Fetch options.
 * @param {{logoutOnForbidden?: boolean}} [authPolicy={}] - Authentication-failure handling policy.
 * @returns {Promise<Response>} The fetch response; throws on 401 and policy-gated 403 responses.
 */
export async function apiRequest(url, options = {}, authPolicy = {}) {
    const response = await fetch(url, withCredentials(options));
    handleAuthFailure(response, authPolicy);
    return response;
}

/**
 * Decode a protobuf response body into a plain object (snake_case fields).
 * @param {Response} response
 * @param {{decode: Function, toObject: Function}} MessageType
 */
export async function decodeProtoResponse(response, MessageType) {
    const buf = new Uint8Array(await response.arrayBuffer());
    const message = MessageType.decode(buf);
    return MessageType.toObject(message, protoObjectOptions);
}

/**
 * GET (or other) a protobuf API endpoint and decode into a plain object.
 * Does not throw on non-OK status; caller checks response.ok.
 *
 * @param {string} url
 * @param {{decode: Function, toObject: Function}} MessageType
 * @param {RequestInit} [options]
 * @returns {Promise<{response: Response, data: object|null}>}
 */
export async function fetchProto(url, MessageType, options = {}) {
    const response = await fetch(url, withCredentials({
        ...options,
        headers: {
            Accept: PROTO_CONTENT_TYPE,
            ...(options.headers || {}),
        },
    }));
    if (!response.ok) {
        return {response, data: null};
    }
    const data = await decodeProtoResponse(response, MessageType);
    return {response, data};
}

/**
 * Send a protobuf request body and optionally decode a protobuf response.
 *
 * @param {string} url
 * @param {string} method
 * @param {{create: Function, encode: Function}|null} [RequestType]
 * @param {object} [requestPayload] plain object matching proto fields
 * @param {{decode: Function, toObject: Function}|null} [ResponseType]
 * @param {RequestInit} [options]
 */
export async function sendProto(url, method, RequestType = null, requestPayload = null, ResponseType = null, options = {}) {
    const headers = {
        Accept: PROTO_CONTENT_TYPE,
        ...(options.headers || {}),
    };
    let body;
    if (RequestType) {
        body = RequestType.encode(RequestType.create(requestPayload || {})).finish();
        headers['Content-Type'] = PROTO_CONTENT_TYPE;
    }
    const response = await fetch(url, withCredentials({
        method,
        ...options,
        headers,
        body,
    }));
    if (!response.ok || !ResponseType) {
        return {response, data: null};
    }
    const data = await decodeProtoResponse(response, ResponseType);
    return {response, data};
}

/**
 * POST a protobuf request body and optionally decode a protobuf response.
 *
 * @param {string} url
 * @param {{create: Function, encode: Function}|null} [RequestType]
 * @param {object} [requestPayload] plain object matching proto fields
 * @param {{decode: Function, toObject: Function}|null} [ResponseType]
 * @param {RequestInit} [options]
 */
export async function postProto(url, RequestType = null, requestPayload = null, ResponseType = null, options = {}) {
    return sendProto(url, 'POST', RequestType, requestPayload, ResponseType, options);
}

/**
 * PUT a protobuf request body and optionally decode a protobuf response.
 *
 * @param {string} url
 * @param {{create: Function, encode: Function}|null} [RequestType]
 * @param {object} [requestPayload] plain object matching proto fields
 * @param {{decode: Function, toObject: Function}|null} [ResponseType]
 * @param {RequestInit} [options]
 */
export async function putProto(url, RequestType = null, requestPayload = null, ResponseType = null, options = {}) {
    return sendProto(url, 'PUT', RequestType, requestPayload, ResponseType, options);
}
