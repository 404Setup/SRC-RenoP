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

export const DOCKER_ERROR_CODE_HEADER = 'X-Renop-Error-Code';

const dockerErrorKeys = Object.freeze({
    authentication_required: 'docker.authenticationRequired',
    cannot_invite_self: 'docker.cannotInviteSelf',
    image_not_found: 'docker.imageNotFound',
    invalid_permission_level: 'docker.invalidPermissionLevel',
    invalid_request: 'docker.invalidRequest',
    invitation_invalid: 'docker.invitationInvalid',
    invitation_pending: 'docker.invitationAlreadyPending',
    member_exists: 'docker.memberAlreadyExists',
    permission_denied: 'docker.permissionDenied',
    publication_file_quota: 'publicationQuota.fileExceeded',
    publication_byte_quota: 'publicationQuota.byteExceeded',
    publication_count_quota: 'publicationQuota.countExceeded',
    readme_too_large: 'docker.readmeTooLarge',
    repository_not_found: 'docker.repositoryNotFound',
    review_pending: 'review.alreadyPending',
    review_unavailable: 'review.serviceUnavailable',
    service_unavailable: 'docker.serviceUnavailable',
    super_team_mismatch: 'superTeam.bindingMismatch',
    super_team_permission: 'superTeam.bindingPermission',
    super_team_required: 'superTeam.bindingRequired',
    user_not_found: 'docker.userNotFound'
});

/**
 * Resolve a Docker management API response to a localized message.
 * @param {Response|null|undefined} response - Failed API response.
 * @param {string} fallbackKey - Localized key for an unmapped failure.
 * @param {Object.<string, *>} [params={}] - Interpolation values.
 * @returns {string} Localized error message.
 */
export function dockerResponseError(response, fallbackKey, params = {}) {
    const code = response?.headers?.get(DOCKER_ERROR_CODE_HEADER) || '';
    return t(dockerErrorKeys[code] || fallbackKey, params);
}
