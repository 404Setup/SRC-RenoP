/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {apiRequest} from './api.js';
import {showAlert} from './alert.js';
import {t} from './i18n.js';
import {registerMessageActionHandler, registerMessageRenderer} from './messages.js';
import {localizedResponseError} from './response-errors.js';

/**
 * Return a trusted, bounded Cargo invitation payload from a server message.
 * @param {object} message - Message-center record.
 * @returns {{repository: string, package: string, inviter: string, level: number}|null} Valid invitation payload.
 */
function cargoInvitationPayload(message) {
    let payload = message?.payload;
    if (typeof payload === 'string' && payload) {
        try {
            payload = JSON.parse(payload);
        } catch (_) {
            return null;
        }
    }
    if (!payload || typeof payload !== 'object') return null;
    const repository = String(payload.repository || '');
    const packageName = String(payload.package || '');
    const inviter = String(payload.inviter || '');
    const level = Number(payload.level);
    if (!/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(repository) || repository.length > 64) return null;
    if (!packageName || packageName.length > 64 || !inviter || inviter.length > 255) return null;
    if (!Number.isInteger(level) || level < 1 || level > 3) return null;
    return {repository, package: packageName, inviter, level};
}


/**
 * Render localized Cargo invitation text while retaining server fallback text
 * for malformed or legacy messages.
 * @param {object} message - Message-center record.
 * @returns {{title?: string, body?: string}} Localized message presentation.
 */
function renderCargoInvitation(message) {
    const payload = cargoInvitationPayload(message);
    if (!payload) return {};
    return {
        title: t('cargo.inviteTitle'),
        body: t('cargo.inviteBody', {
            inviter: payload.inviter,
            package: payload.package,
            level: payload.level
        })
    };
}

/**
 * Accept or reject a Cargo package invitation through its repository-owned API.
 * @param {object} message - Actionable invitation message.
 * @param {string} decision - `accept` or `reject`.
 * @returns {Promise<boolean>} Whether the message center should commit the local action state.
 */
async function handleCargoInvitation(message, decision) {
    const payload = cargoInvitationPayload(message);
    if (!payload || (decision !== 'accept' && decision !== 'reject')) {
        throw new Error('Invalid Cargo package invitation');
    }
    const endpoint = `/${encodeURIComponent(payload.repository)}/api/v1/invitations/${encodeURIComponent(message.id)}/${decision}`;
    const response = await apiRequest(endpoint, {method: 'POST'});
    if (!response.ok) throw await localizedResponseError(response, 'messages.actionFailed');
    showAlert(t(decision === 'accept' ? 'cargo.inviteAccepted' : 'cargo.inviteRejected'), 'success');
    window.dispatchEvent(new CustomEvent('cargoMembershipChanged', {
        detail: {repository: payload.repository, package: payload.package}
    }));
    return true;
}

registerMessageRenderer('cargo_package_invite', renderCargoInvitation);
registerMessageActionHandler('cargo_package_invite', handleCargoInvitation);
