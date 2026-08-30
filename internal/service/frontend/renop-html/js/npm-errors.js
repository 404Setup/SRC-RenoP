/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

import {t} from './i18n.js';

export const NPM_ERROR_CODE_HEADER = 'X-Renop-Error-Code';

const npmErrorKeys = Object.freeze({
    authentication_required: 'npm.authenticationRequired',
    cannot_invite_self: 'npm.cannotInviteSelf',
    internal_error: 'npm.operationFailed',
    invalid_package_name: 'npm.invalidPackageName',
    invalid_request: 'npm.invalidRequest',
    invitation_invalid: 'npm.invitationInvalid',
    invitation_pending: 'npm.invitationPending',
    last_owner: 'npm.lastOwner',
    member_exists: 'npm.memberExists',
    package_exists: 'npm.packageExists',
    package_not_found: 'npm.packageNotFound',
    permission_denied: 'npm.permissionDenied',
    publication_file_quota: 'publicationQuota.fileExceeded',
    publication_byte_quota: 'publicationQuota.byteExceeded',
    publication_count_quota: 'publicationQuota.countExceeded',
    private_requires_scope: 'npm.privateRequiresScope',
    repository_not_found: 'npm.repositoryNotFound',
    review_pending: 'review.alreadyPending',
    review_unavailable: 'review.serviceUnavailable',
    super_team_mismatch: 'superTeam.bindingMismatch',
    super_team_permission: 'superTeam.bindingPermission',
    super_team_required: 'superTeam.bindingRequired',
    user_not_found: 'npm.userNotFound'
});

/**
 * Resolve an npm management failure through its stable response code.
 * @param {Response|null|undefined} response - Failed response.
 * @param {string} fallbackKey - Translation key used for unknown failures.
 * @returns {string} Localized message.
 */
export function npmResponseError(response, fallbackKey) {
    const code = response?.headers?.get(NPM_ERROR_CODE_HEADER) || '';
    return t(npmErrorKeys[code] || fallbackKey);
}
