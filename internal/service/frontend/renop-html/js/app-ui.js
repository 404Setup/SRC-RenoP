/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {closeModalWithAnim as closeModalWithAnimShared, configureModalInert,} from '@renop/ui/modal';

const FRONTEND_MODAL_IDS = [
    'user-editor-modal',
    'user-password-result-modal',
    'login-modal',
    'privacy-policy-modal',
    'repo-mirrors-modal',
    'language-modal',
    'message-center-modal',
    'message-compose-modal',
    'renop-confirm-container',
    'renop-prompt-container',
];

configureModalInert({
    modalIds: FRONTEND_MODAL_IDS,
    rootSelectors: ['#app', '.top-nav'],
    installGlobal: true,
});

/**
 * Close a modal with fade-out animation (product UI fast timings).
 * @param {HTMLElement|null} modal
 * @param {Function} [callback]
 * @returns {void}
 */
export function closeModalWithAnim(modal, callback) {
    closeModalWithAnimShared(modal, callback, {fast: true});
}
