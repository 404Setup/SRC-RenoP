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

/**
 * Return a trusted, bounded Docker invitation payload from a server message.
 * @param {object} message - Message-center record.
 * @returns {{repository: string, image: string, inviter: string, level: number}|null} Valid invitation payload.
 */
function dockerInvitationPayload(message) {
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
    const image = String(payload.image || '');
    const inviter = String(payload.inviter || '');
    const level = Number(payload.level);
    if (!/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(repository) || repository.length > 64) return null;
    if (!image || image.length > 255 || !inviter || inviter.length > 255) return null;
    if (!Number.isInteger(level) || level < 1 || level > 3) return null;
    return {repository, image, inviter, level};
}

/**
 * Render localized Docker invitation text.
 * @param {object} message - Message-center record.
 * @returns {{title?: string, body?: string}} Localized message presentation.
 */
function renderDockerInvitation(message) {
    const payload = dockerInvitationPayload(message);
    if (!payload) return {};
    return {
        title: t('docker.inviteTitle') || 'Docker Image Invitation',
        body: t('docker.inviteBody', {
            inviter: payload.inviter,
            image: payload.image,
            level: payload.level
        }) || `${payload.inviter} invited you to collaborate on ${payload.image} (L${payload.level}).`
    };
}

/**
 * Accept or reject a Docker image invitation.
 * @param {object} message - Actionable invitation message.
 * @param {string} decision - `accept` or `reject`.
 * @returns {Promise<boolean>} Whether the message center should commit the local action state.
 */
async function handleDockerInvitation(message, decision) {
    const payload = dockerInvitationPayload(message);
    if (!payload || (decision !== 'accept' && decision !== 'reject')) {
        throw new Error('Invalid Docker image invitation');
    }
    const endpoint = `/api/docker/repositories/${encodeURIComponent(payload.repository)}/invitations/${encodeURIComponent(message.id)}/${decision}`;
    const response = await apiRequest(endpoint, {method: 'POST'});
    if (!response.ok) {
        const detail = (await response.text()).slice(0, 512);
        throw new Error(detail || `HTTP ${response.status}`);
    }
    showAlert(t(decision === 'accept' ? 'docker.inviteAccepted' : 'docker.inviteRejected') || `Invitation ${decision}ed.`, 'success');
    window.dispatchEvent(new CustomEvent('dockerMembershipChanged', {
        detail: {repository: payload.repository, image: payload.image}
    }));
    return true;
}

registerMessageRenderer('docker_image_invite', renderDockerInvitation);
registerMessageActionHandler('docker_image_invite', handleDockerInvitation);
