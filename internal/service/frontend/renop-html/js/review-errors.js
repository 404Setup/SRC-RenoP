/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

export const REVIEW_ERROR_KEYS = Object.freeze({
    authentication_required: 'review.authenticationRequired',
    invalid_request: 'review.invalidRequest',
    resource_changed: 'review.resourceChanged',
    review_decided: 'review.alreadyDecided',
    review_exists: 'review.alreadyPending',
    review_failed: 'review.operationFailed',
    review_not_found: 'review.notFound',
    review_permission: 'review.permissionDenied',
    publication_active: 'review.publicationActive',
    publication_sealed: 'review.publicationSealed',
    review_file_not_found: 'review.fileNotFound',
    review_limit: 'review.limitReached',
    service_unavailable: 'review.serviceUnavailable',
    super_team_mismatch: 'superTeam.bindingMismatch',
    transfer_restricted: 'review.transferRestricted'
});
