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
import {registerMessageRenderer} from './messages.js';

const updateEvents = new Set([
    'available', 'current', 'check_failed', 'downloading', 'ready',
    'install_failed', 'insufficient_space', 'restart_failed'
]);

/**
 * Return a trusted, bounded system-update payload.
 * @param {object} message - Message-center record.
 * @returns {{event: string, version: string}|null} Valid update payload.
 */
function systemUpdatePayload(message) {
    let payload = message?.payload;
    if (typeof payload === 'string' && payload) {
        try {
            payload = JSON.parse(payload);
        } catch (_) {
            return null;
        }
    }
    if (!payload || typeof payload !== 'object') return null;
    const event = String(payload.event || '').toLowerCase();
    const version = String(payload.version || '').trim();
    if (!updateEvents.has(event) || version.length > 128 || /[\0\r\n]/.test(version)) return null;
    return {event, version};
}

/**
 * Render one localized system-update notification without exposing backend errors.
 * @param {object} message - Message-center record.
 * @returns {{title?: string, body?: string}} Localized message presentation.
 */
function renderSystemUpdate(message) {
    const payload = systemUpdatePayload(message);
    if (!payload) return {};
    const values = {version: payload.version || t('common.unknown')};
    const keys = {
        available: ['updaterNotice.availableTitle', 'updaterNotice.availableBody'],
        current: ['updaterNotice.currentTitle', 'updaterNotice.currentBody'],
        check_failed: ['updaterNotice.checkFailedTitle', 'updaterNotice.checkFailedBody'],
        downloading: ['updaterNotice.downloadingTitle', 'updaterNotice.downloadingBody'],
        ready: ['updaterNotice.readyTitle', 'updaterNotice.readyBody'],
        install_failed: ['updaterNotice.installFailedTitle', 'updaterNotice.installFailedBody'],
        insufficient_space: ['updaterNotice.spaceTitle', 'updaterNotice.spaceBody'],
        restart_failed: ['updaterNotice.restartFailedTitle', 'updaterNotice.restartFailedBody']
    }[payload.event];
    return {title: t(keys[0], values), body: t(keys[1], values)};
}

registerMessageRenderer('system_update', renderSystemUpdate);
