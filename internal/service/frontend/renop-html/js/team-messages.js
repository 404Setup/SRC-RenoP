/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {t} from './i18n.js';
import {registerMessageRenderer} from './messages.js';

/**
 * Return a trusted package-team removal payload without accepting operator metadata.
 * @param {object} message - Message-center record.
 * @returns {{format: string, repository: string, package: string}|null} Valid removal payload.
 */
function teamRemovalPayload(message) {
    let payload = message?.payload;
    if (typeof payload === 'string' && payload) {
        try {
            payload = JSON.parse(payload);
        } catch (_) {
            return null;
        }
    }
    if (!payload || typeof payload !== 'object') return null;
    const format = String(payload.format || '').toLowerCase();
    const repository = String(payload.repository || '');
    const packageName = String(payload.package || '');
    if (!['cargo', 'docker', 'maven'].includes(format)) return null;
    if (!packageName || packageName.length > 255 || /[\0\r\n]/.test(packageName)) return null;
    if (format !== 'maven' && (!repository || repository.length > 64 || /[\0\r\n]/.test(repository))) return null;
    return {format, repository, package: packageName};
}

/**
 * Render a localized removal notice that intentionally omits the operator.
 * @param {object} message - Message-center record.
 * @returns {{title?: string, body?: string}} Localized message presentation.
 */
function renderTeamRemoval(message) {
    const payload = teamRemovalPayload(message);
    if (!payload) return {};
    return {
        title: t('team.removedTitle'),
        body: payload.format === 'maven'
            ? t('team.removedMavenBody', {domain: payload.package})
            : t('team.removedRepositoryBody', {repository: payload.repository, package: payload.package})
    };
}

registerMessageRenderer('package_team_removed', renderTeamRemoval);
