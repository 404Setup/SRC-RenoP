/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

/** Event emitted when the active routed page can no longer be accessed. */
export const protectedRouteDeniedEvent = 'renop:protected-route-denied';

/**
 * Ask the application shell to replace the active protected route with home.
 * @param {number} [status=0] - HTTP status that caused the route to be abandoned.
 * @returns {void}
 */
export function requestProtectedRouteExit(status = 0) {
    window.dispatchEvent(new CustomEvent(protectedRouteDeniedEvent, {
        detail: {status: Number(status) || 0}
    }));
}

/**
 * Leave a protected route when an API response proves that it cannot be viewed.
 * Authentication failures are normally intercepted by the shared API client;
 * this helper also accepts them for callers that use a non-throwing request.
 * @param {Response|null|undefined} response - Protected page response.
 * @returns {boolean} Whether a route exit was requested.
 */
export function exitProtectedRouteOnDenial(response) {
    const status = Number(response?.status) || 0;
    if (status !== 401 && status !== 403) return false;
    requestProtectedRouteExit(status);
    return true;
}
