/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {el} from '@renop/ui/dom';
import {makeCustomSelect} from '@renop/ui/custom-select';
import {RenopDialog, runButtonAction} from './components.js';
import {showAlert} from './alert.js';
import {t} from './i18n.js';
import {caughtErrorMessage, localizedResponseError} from './response-errors.js';

const reasons = ['hold', 'prohibited', 'expired', 'trojan', 'abuse', 'dmca', 'reup', 'squatting', 'quality'];

/** Report whether a package or version has any effective write restriction. */
export function resourceWriteLocked(...resources) {
    return resources.some(resource => resource?.locks?.length > 0);
}

/** Report whether files are unavailable for every viewer, including staff. */
export function resourceReadLocked(...resources) {
    return resources.some(resource => resource?.locks?.some(lock => lock.mode === 'read'));
}

/** Render public lock modes and localized reasons without exposing operator identity. */
export function createResourceLockNotices(locks = []) {
    if (!locks.length) return null;
    return el('div', {class: 'resource-lock-notices'}, ...locks.map(lock =>
        el('div', {class: 'package-deprecation-notice', role: 'status'},
            el('span', {}, `${t(`resourceLock.${lock.mode}`)} · ${t(`resourceLock.reason.${lock.reason}`)}`),
            lock.source === 'system' ? el('span', {}, t('resourceLock.system')) : null
        )
    ));
}

/** Create a staff action that changes only the selected resource's manual lock. */
export function createResourceLockButton({locks = [], name, request, onSuccess}) {
    const manual = locks.find(lock => lock.source === 'manual' && !lock.inherited);
    return el('button', {
        type: 'button', class: 'pill-btn pill-btn--soft pill-btn--sm',
        onclick: () => {
            let mode = manual?.mode || '';
            let reason = manual?.reason || 'quality';
            const modeSelect = makeCustomSelect(['', 'write', 'read'].map(value => ({
                value, label: t(`resourceLock.${value || 'none'}`)
            })), mode, value => { mode = value; });
            const reasonSelect = makeCustomSelect(reasons.map(value => ({
                value, label: t(`resourceLock.reason.${value}`)
            })), reason, value => { reason = value; });
            void RenopDialog.show({
                id: 'resource-lock-dialog', glass: true, size: 'sm',
                title: t('resourceLock.manage'), subtitle: name,
                subtitleStyle: {overflowWrap: 'anywhere'},
                body: [
                    el('p', {}, t('resourceLock.explanation')),
                    el('div', {class: 'form-group'}, el('label', {for: modeSelect.querySelector('button').id}, t('resourceLock.mode')), modeSelect),
                    el('div', {class: 'form-group'}, el('label', {for: reasonSelect.querySelector('button').id}, t('resourceLock.reason')), reasonSelect),
                    locks.some(lock => lock.source === 'system') ? el('p', {}, t('resourceLock.systemNotice')) : null,
                    locks.some(lock => lock.inherited) ? el('p', {}, t('resourceLock.inheritedNotice')) : null
                ].filter(Boolean),
                footer: [
                    {text: t('common.cancel'), className: 'action-btn', onClick: (_event, dialog) => dialog.close(false)},
                    {text: t('common.save'), variant: 'primary', onClick: (event, dialog) =>
                        runButtonAction(event.currentTarget, async () => {
                            try {
                                const response = await request(mode, reason);
                                if (!response.ok) throw await localizedResponseError(response, 'resourceLock.failed');
                                dialog.close(true);
                                showAlert(t('resourceLock.saved'), 'success');
                                await onSuccess?.();
                            } catch (error) {
                                showAlert(caughtErrorMessage(error, 'resourceLock.failed'), 'error');
                            }
                        })
                    }
                ]
            });
        }
    }, t('resourceLock.manage'));
}
