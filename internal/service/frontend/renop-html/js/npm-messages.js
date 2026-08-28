/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

import {apiRequest} from './api.js';
import {showAlert} from './alert.js';
import {t} from './i18n.js';
import {registerMessageActionHandler, registerMessageRenderer} from './messages.js';
import {npmResponseError} from './npm-errors.js';

/**
 * Validate a bounded npm invitation message payload.
 * @param {object} message - Message-center record.
 * @returns {{repository: string, package: string, inviter: string, level: number}|null} Trusted payload.
 */
function npmInvitationPayload(message) {
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
    if (!packageName || packageName.length > 214 || /[\0\r\n]/.test(packageName)) return null;
    if (!inviter || inviter.length > 255 || /[\0\r\n]/.test(inviter)) return null;
    if (!Number.isInteger(level) || level < 0 || level > 4) return null;
    return {repository, package: packageName, inviter, level};
}

/**
 * Render localized npm invitation copy.
 * @param {object} message - Message-center record.
 * @returns {{title?: string, body?: string}} Localized presentation.
 */
function renderNPMInvitation(message) {
    const payload = npmInvitationPayload(message);
    if (!payload) return {};
    return {
        title: t('npm.inviteTitle'),
        body: t('npm.inviteBody', payload)
    };
}

/**
 * Accept or reject an npm package invitation.
 * @param {object} message - Actionable message.
 * @param {string} decision - `accept` or `reject`.
 * @returns {Promise<boolean>} Whether the local message action should commit.
 */
async function handleNPMInvitation(message, decision) {
    const payload = npmInvitationPayload(message);
    if (!payload || (decision !== 'accept' && decision !== 'reject')) {
        throw new Error('Invalid npm package invitation');
    }
    const response = await apiRequest(
        `/api/npm/repositories/${encodeURIComponent(payload.repository)}/invitations/${encodeURIComponent(message.id)}/${decision}`,
        {method: 'POST'}, {logoutOnForbidden: false}
    );
    if (!response.ok) throw new Error(npmResponseError(response, 'messages.actionFailed'));
    showAlert(t(decision === 'accept' ? 'npm.inviteAccepted' : 'npm.inviteRejected'), 'success');
    window.dispatchEvent(new CustomEvent('npmMembershipChanged', {
        detail: {repository: payload.repository, package: payload.package}
    }));
    return true;
}

registerMessageRenderer('npm_package_invite', renderNPMInvitation);
registerMessageActionHandler('npm_package_invite', handleNPMInvitation);
