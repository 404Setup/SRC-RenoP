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
import {showAlert} from './alert.js';
import {responseErrorMessage} from './response-errors.js';
import {createIcon, runButtonAction} from './components.js';
import {el} from '@renop/ui/dom';

export function createPackageDeprecationBadge() {
    return el('span', {class: 'package-deprecation-badge'}, t('package.deprecated'));
}

export function createPackageDeprecationNotice() {
    return el('div', {class: 'package-deprecation-notice', role: 'status'},
        createIcon('warning', {width: '18', height: '18'}),
        el('span', {}, t('package.deprecationNotice'))
    );
}

/**
 * Create the irreversible package-deprecation action.
 * @param {() => Promise<Response>} request - Format-specific mutation request.
 * @param {() => void|Promise<void>} onSuccess - Detail-page refresh callback.
 * @returns {HTMLButtonElement}
 */
export function createDeprecatePackageButton(request, onSuccess) {
    const button = el('button', {
        type: 'button', class: 'pill-btn pill-btn--danger pill-btn--sm'
    }, t('package.deprecate'));
    button.addEventListener('click', () => {
        void runButtonAction(button, async () => {
            if (!(await window.showConfirm(t('package.deprecateConfirm'), {danger: true}))) return;
            const response = await request();
            if (!response.ok) {
                showAlert(await responseErrorMessage(response, 'package.deprecateFailed'), 'error');
                return;
            }
            showAlert(t('package.deprecatedSuccess'), 'success');
            await onSuccess?.();
        });
    });
    return button;
}
