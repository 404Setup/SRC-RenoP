/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {openModalWithAnim} from '@renop/ui/modal';
import {closeModalWithAnim} from './app-ui.js';
import {t} from './i18n.js';
import {readPrivacyPolicyResponse} from './privacy-policy-response.js';

let initialized = false;
let cachedPolicy = null;
let pendingPolicy = null;

/**
 * Fetch and coalesce the instance privacy policy without caching failures.
 * @returns {Promise<string>} Validated policy text.
 */
async function loadPrivacyPolicy() {
    if (cachedPolicy !== null) return cachedPolicy;
    if (!pendingPolicy) {
        pendingPolicy = fetch('/api/privacy-policy')
            .then(readPrivacyPolicyResponse)
            .then(policy => {
                cachedPolicy = policy;
                return policy;
            })
            .finally(() => {
                pendingPolicy = null;
            });
    }
    return pendingPolicy;
}

/**
 * Initialize the optional privacy-policy link and modal once.
 * @returns {void}
 */
export function initPrivacyPolicy() {
    if (initialized) return;
    initialized = true;
    const link = document.getElementById('privacy-policy-link');
    const separator = document.getElementById('privacy-policy-separator');
    const modal = document.getElementById('privacy-policy-modal');
    const content = document.getElementById('privacy-policy-content');
    const backdrop = document.getElementById('privacy-policy-backdrop');
    const close = document.getElementById('btn-close-privacy-policy');
    if (!link || !separator || !modal || !content) return;

    fetch('/api/privacy-policy', {method: 'HEAD'})
        .then(response => {
            if (response.ok) {
                link.style.display = 'inline';
                separator.style.display = 'inline';
            }
        })
        .catch(error => console.error('Failed to check privacy policy', error));

    link.addEventListener('click', event => {
        event.preventDefault();
        openModalWithAnim(modal);
        if (cachedPolicy !== null) {
            content.textContent = cachedPolicy;
            return;
        }
        content.textContent = t('privacy.loading');
        void loadPrivacyPolicy()
            .then(policy => {
                content.textContent = policy;
            })
            .catch(error => {
                console.error('Failed to load privacy policy', error);
                content.textContent = t('privacy.failedLoad');
            });
    });
    const closeModal = () => closeModalWithAnim(modal);
    close?.addEventListener('click', closeModal);
    backdrop?.addEventListener('click', closeModal);
}
