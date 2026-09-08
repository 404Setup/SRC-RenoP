/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {t} from './i18n.js';

export const MAIL_STATUSES = ['queued', 'paused', 'sending', 'checking', 'accepted', 'sent', 'delivered', 'failed', 'unknown', 'expired', 'cancelled', 'queued_provider'];

/** Localize a delivery state without rendering provider text. */
export function mailStatusLabel(status) {
    return t(`mail.status.${MAIL_STATUSES.includes(status) ? status : 'unknown'}`);
}
