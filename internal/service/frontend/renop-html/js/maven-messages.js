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
 * Return a trusted, bounded Maven invitation payload from a server message.
 * @param {object} message - Message-center record.
 * @returns {{domain: string, inviter: string, level: number}|null} Valid invitation payload.
 */
function mavenInvitationPayload(message) {
    let payload = message?.payload;
    if (typeof payload === 'string' && payload) {
        try {
            payload = JSON.parse(payload);
        } catch (_) {
            return null;
        }
    }
    if (!payload || typeof payload !== 'object') return null;
    const domain = String(payload.domain || '');
    const inviter = String(payload.inviter || '');
    const level = Number(payload.level);
    if (!domain || domain.length > 255 || !inviter || inviter.length > 255) return null;
    if (!Number.isInteger(level) || level < 0 || level > 4) return null;
    return {domain, inviter, level};
}

/**
 * Render localized Maven domain invitation text.
 * @param {object} message - Message-center record.
 * @returns {{title?: string, body?: string}} Localized message presentation.
 */
function renderMavenInvitation(message) {
    const payload = mavenInvitationPayload(message);
    if (!payload) return {};
    return {
        title: t('maven.inviteTitle'),
        body: t('maven.inviteBody', {
            inviter: payload.inviter,
            domain: payload.domain,
            level: payload.level
        })
    };
}

/**
 * Accept or reject a Maven domain invitation.
 * @param {object} message - Actionable invitation message.
 * @param {string} decision - `accept` or `reject`.
 * @returns {Promise<boolean>} Whether the message center should commit the local action state.
 */
async function handleMavenInvitation(message, decision) {
    const payload = mavenInvitationPayload(message);
    if (!payload || (decision !== 'accept' && decision !== 'reject')) {
        throw new Error('Invalid Maven domain invitation');
    }
    const endpoint = `/api/maven/invitations/${encodeURIComponent(message.id)}/${decision}`;
    const response = await apiRequest(endpoint, {method: 'POST'});
    if (!response.ok) throw await localizedResponseError(response, 'messages.actionFailed');
    showAlert(t(decision === 'accept' ? 'maven.inviteAccepted' : 'maven.inviteRejected'), 'success');
    window.dispatchEvent(new CustomEvent('mavenMembershipChanged', {
        detail: {domain: payload.domain}
    }));
    return true;
}

registerMessageRenderer('maven_domain_invite', renderMavenInvitation);
registerMessageActionHandler('maven_domain_invite', handleMavenInvitation);
