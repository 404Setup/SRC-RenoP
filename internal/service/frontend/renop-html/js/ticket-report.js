/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {el} from '@renop/ui/dom';
import {cachedIsLoggedIn} from './auth.js';
import {runButtonAction} from './components.js';
import {t} from './i18n.js';
import {openTicketComposer} from './tickets.js';

/** @param {object} target - Visible resource coordinates. @param {boolean} unavailable - Own or unpublished resource. @returns {HTMLButtonElement} Report action. */
export function createTicketReportButton(target, unavailable = false) {
    return el('button', {
        type: 'button', class: 'pill-btn pill-btn--soft pill-btn--sm', hidden: unavailable || !cachedIsLoggedIn,
        onclick: event => {
            if (!unavailable && cachedIsLoggedIn) return runButtonAction(event.currentTarget, () => openTicketComposer(target));
        }
    }, t(target.version ? 'ticket.reportVersion' : 'ticket.report'));
}
