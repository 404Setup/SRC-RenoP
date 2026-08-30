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

const resourceTypes = new Set([
    'docker_image', 'npm_package', 'cargo_package', 'maven_artifact', 'maven_domain'
]);

/**
 * Parse one bounded review notification payload.
 * @param {object} message - Message-center record.
 * @returns {object|null} Validated review payload.
 */
function reviewPayload(message) {
    let payload = message?.payload;
    if (typeof payload === 'string' && payload) {
        try {
            payload = JSON.parse(payload);
        } catch (_) {
            return null;
        }
    }
    if (!payload || typeof payload !== 'object') return null;
    const resourceType = String(payload.resource_type || '');
    const resourceName = String(payload.resource_name || '');
    const status = String(payload.status || '');
    if (!resourceTypes.has(resourceType) || !resourceName || resourceName.length > 512 ||
        /[\0\r\n]/.test(resourceName) || status && !['approved', 'rejected', 'cancelled'].includes(status)) {
        return null;
    }
    return {
        resourceType,
        resourceName,
        status,
        reason: String(payload.decision_reason || '').slice(0, 512)
    };
}

/** @param {string} reason - Stable or custom rejection reason. @returns {string} Localized reason. */
function decisionReason(reason) {
    if (reason.startsWith('preset:')) return t(`review.rejectPreset.${reason.slice(7)}`);
    if (reason.startsWith('custom:')) return reason.slice(7);
    return reason;
}

/** @param {object} message - Pending-review message. @returns {{title?: string, body?: string}} Presentation. */
function renderPendingReview(message) {
    const payload = reviewPayload(message);
    if (!payload) return {};
    return {
        title: t('review.message.pendingTitle'),
        body: t('review.message.pendingBody', {
            type: t(`review.type.${payload.resourceType}`), resource: payload.resourceName
        })
    };
}

/** @param {object} message - Review-result message. @returns {{title?: string, body?: string}} Presentation. */
function renderReviewResult(message) {
    const payload = reviewPayload(message);
    if (!payload || !payload.status) return {};
    let body = t('review.message.resultBody', {
        type: t(`review.type.${payload.resourceType}`), resource: payload.resourceName,
        status: t(`review.status.${payload.status}`)
    });
    const reason = decisionReason(payload.reason);
    if (reason) body += ` ${t('review.message.resultReason', {reason})}`;
    return {title: t(`review.message.resultTitle.${payload.status}`), body};
}

registerMessageRenderer('review_pending', renderPendingReview);
registerMessageRenderer('review_result', renderReviewResult);
