/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {el} from '@renop/ui/dom';
import {makeCustomSelect} from '@renop/ui/custom-select';
import {createFieldRow, RenopDialog, runButtonAction} from './components.js';
import {showAlert} from './alert.js';
import {t} from './i18n.js';
import {caughtErrorMessage, localizedResponseError} from './response-errors.js';

const reasons = ['hold', 'prohibited', 'expired', 'trojan', 'abuse', 'dmca', 'reup', 'squatting', 'quality', 'custom'];

/** Report whether a package or version has any effective write restriction. */
export function resourceWriteLocked(...resources) {
    return resources.some(resource => resource?.locks?.length > 0);
}

/** Report whether files are unavailable for every viewer, including staff. */
export function resourceReadLocked(...resources) {
    return resources.some(resource => resource?.locks?.some(lock => lock.mode === 'read'));
}

/** Return a predefined translation or administrator-authored plain-text reason. */
export function resourceLockReason(lock) {
    return lock.reason === 'custom' ? String(lock.reason_text || t('resourceLock.reason.custom')) : t(`resourceLock.reason.${lock.reason}`);
}

/** Summarize a visible package or version restriction beside its identity. */
export function createResourceLockBadge(...resources) {
    if (!resourceWriteLocked(...resources) && !resources.some(resource => resource?.version_locked)) return null;
    const key = resourceReadLocked(...resources) ? 'read' : resourceWriteLocked(...resources) ? 'write' : 'versionsLocked';
    return el('span', {class: 'resource-lock-badge'}, t(`resourceLock.${key}`));
}

/** Render public lock modes and localized reasons without exposing operator identity. */
export function createResourceLockNotices(locks = []) {
    if (!locks.length) return null;
    return el('div', {class: 'resource-lock-notices'}, ...locks.map(lock =>
        el('div', {class: 'package-deprecation-notice', role: 'status'},
            el('span', {}, `${t(`resourceLock.${lock.mode}`)} · ${resourceLockReason(lock)}`),
            lock.source === 'system' ? el('span', {}, t('resourceLock.system')) : null,
            lock.inherited ? el('span', {}, t('resourceLock.inherited')) : null
        )
    ));
}

/** Create a staff action that changes only the selected resource's manual lock. */
export function createResourceLockButton({locks = [], inheritedLocks = [], name, request, onSuccess}) {
    locks = [...inheritedLocks.map(lock => ({...lock, inherited: true})), ...locks];
    const manual = locks.find(lock => lock.source === 'manual' && !lock.inherited);
    return el('button', {
        type: 'button', class: 'pill-btn pill-btn--soft pill-btn--sm',
        onclick: () => {
            let mode = manual?.mode || '';
            let reason = manual?.reason || 'quality';
            const custom = el('input', {type: 'text', class: 'cfg-input', maxLength: 256, value: manual?.reason_text || ''});
            const customField = createFieldRow(t('resourceLock.reason.custom'), t('resourceLock.customHint'), custom);
            const updateCustom = () => {
                const visible = reason === 'custom' && mode !== '';
                customField.hidden = !visible;
                custom.disabled = !visible;
                custom.required = visible;
            };
            const modeSelect = makeCustomSelect(['', 'write', 'read'].map(value => ({
                value, label: t(`resourceLock.${value || 'none'}`)
            })), mode, value => {
                mode = value;
                updateCustom();
            });
            const reasonSelect = makeCustomSelect(reasons.map(value => ({
                value, label: t(`resourceLock.reason.${value}`)
            })), reason, value => {
                reason = value;
                updateCustom();
            });
            updateCustom();
            const apply = (event, dialog, selectedMode) => runButtonAction(event.currentTarget, async () => {
                if (selectedMode && !custom.reportValidity()) return;
                try {
                    const response = await request(selectedMode, reason, reason === 'custom' ? custom.value.trim() : '');
                    if (!response.ok) throw await localizedResponseError(response, 'resourceLock.failed');
                    dialog.close(true);
                    showAlert(t('resourceLock.saved'), 'success');
                    await onSuccess?.();
                } catch (error) {
                    showAlert(caughtErrorMessage(error, 'resourceLock.failed'), 'error');
                }
            });
            void RenopDialog.show({
                id: 'resource-lock-dialog', glass: true, size: 'sm',
                title: t('resourceLock.manage'), subtitle: name,
                subtitleStyle: {overflowWrap: 'anywhere'},
                body: [
                    el('p', {}, t('resourceLock.explanation')),
                    createFieldRow(t('resourceLock.mode'), '', modeSelect),
                    createFieldRow(t('resourceLock.reason'), '', reasonSelect), customField,
                    locks.some(lock => lock.source === 'system') ? el('p', {}, t('resourceLock.systemNotice')) : null,
                    locks.some(lock => lock.inherited) ? el('p', {}, t('resourceLock.inheritedNotice')) : null
                ].filter(Boolean),
                footer: [
                    {
                        text: t('common.cancel'),
                        className: 'action-btn',
                        onClick: (_event, dialog) => dialog.close(false)
                    },
                    ...(manual ? [{
                        text: t('resourceLock.unlock'),
                        onClick: (event, dialog) => apply(event, dialog, '')
                    }] : []),
                    {text: t('common.save'), variant: 'primary', onClick: (event, dialog) => apply(event, dialog, mode)}
                ]
            });
        }
    }, t(manual ? 'resourceLock.unlock' : 'resourceLock.manage'));
}
