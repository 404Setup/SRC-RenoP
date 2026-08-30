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
import {SUPER_TEAM_ERROR_KEYS} from './super-team-errors.js';

/**
 * Parse a bounded server-owned global-team invitation payload.
 * @param {object} message - Message-center record.
 * @returns {{prefix: string, inviter: string, level: number}|null} Valid payload.
 */
function invitationPayload(message) {
    let payload = message?.payload;
    if (typeof payload === 'string' && payload) {
        try {
            payload = JSON.parse(payload);
        } catch {
            return null;
        }
    }
    if (!payload || typeof payload !== 'object') return null;
    const prefix = String(payload.prefix || '').toLowerCase();
    const inviter = String(payload.inviter || '');
    const level = Number(payload.level);
    if (prefix.length < 2 || prefix.length > 64 || !/^[a-z0-9](?:[a-z0-9_-]*[a-z0-9])$/.test(prefix) || inviter.length > 255 ||
        !Number.isInteger(level) || level < 1 || level > 4) return null;
    return {prefix, inviter, level};
}

/**
 * Render a localized global-team invitation or completed membership addition.
 * @param {object} message - Message-center record.
 * @returns {{title?: string, body?: string}} Localized presentation.
 */
function renderInvitation(message) {
    const payload = invitationPayload(message);
    if (!payload) return {};
    const accepted = message.action_status === 'accepted';
    return accepted ? {
        title: t('superTeam.membershipAddedTitle'),
        body: t('superTeam.membershipAddedBody', payload)
    } : {
        title: t('superTeam.invitationTitle'),
        body: t('superTeam.invitationBody', payload)
    };
}

/**
 * Accept or reject one global-team invitation.
 * @param {object} message - Actionable message.
 * @param {string} decision - `accept` or `reject`.
 * @returns {Promise<boolean>} Whether the message center should commit the action state.
 */
async function handleInvitation(message, decision) {
    const payload = invitationPayload(message);
    if (!payload || (decision !== 'accept' && decision !== 'reject')) {
        throw new Error('Invalid global team invitation');
    }
    const response = await apiRequest(
        `/api/super-teams/invitations/${encodeURIComponent(message.id)}/${decision}`,
        {method: 'POST'}
    );
    if (!response.ok) {
        throw await localizedResponseError(response, 'messages.actionFailed', {}, SUPER_TEAM_ERROR_KEYS);
    }
    showAlert(t(`messages.action.${decision === 'accept' ? 'accepted' : 'rejected'}`), 'success');
    window.dispatchEvent(new CustomEvent('superTeamMembershipChanged', {detail: {prefix: payload.prefix}}));
    return true;
}

registerMessageRenderer('super_team_invite', renderInvitation);
registerMessageActionHandler('super_team_invite', handleInvitation);
