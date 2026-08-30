/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

import {el} from '@renop/ui/dom';
import {makeCustomSelect} from '@renop/ui/custom-select';
import {createToggle} from '@renop/ui/toggle';
import {apiRequest} from './api.js';
import {showAlert} from './alert.js';
import {createIcon, RenopDialog, runButtonAction} from './components.js';
import {t} from './i18n.js';
import {caughtErrorMessage, localizedResponseError} from './response-errors.js';
import {formatBytes} from './browser/utils.js';

const mebibyte = 1024 * 1024;

/**
 * Build the API path for one publication quota owner.
 * @param {'user'|'super_team'} ownerType - Quota owner type.
 * @param {string} ownerKey - Username or global-team prefix.
 * @returns {string} API path.
 */
function quotaURL(ownerType, ownerKey) {
    const segment = ownerType === 'super_team' ? 'super-teams' : 'users';
    return `/api/publication-quota/${segment}/${encodeURIComponent(ownerKey)}`;
}

/**
 * Return one localized quota period label.
 * @param {string} period - Stable period identifier.
 * @returns {string} Localized label.
 */
function periodLabel(period) {
    return t(`publicationQuota.period.${period}`);
}

/**
 * Create one compact usage metric.
 * @param {string} label - Metric label.
 * @param {string} value - Usage summary.
 * @param {number} ratio - Used ratio from zero to one.
 * @returns {HTMLElement} Metric card.
 */
function quotaMetric(label, value, ratio) {
    const percent = Math.max(0, Math.min(100, ratio * 100));
    return el('div', {class: 'publication-quota-metric'},
        el('span', {class: 'publication-quota-metric-label'}, label),
        el('strong', {}, value),
        el('span', {class: 'publication-quota-track', 'aria-hidden': 'true'},
            el('span', {style: {width: `${percent}%`}})
        )
    );
}

/**
 * Render publication quota usage for a profile or global team.
 * @param {object|null|undefined} status - Quota status response.
 * @param {{editable?: boolean, onEdit?: function(): void}} [options={}] - Optional administrator controls.
 * @returns {HTMLElement|null} Quota panel, or null when unavailable.
 */
export function createPublicationQuotaPanel(status, {editable = false, onEdit = null} = {}) {
    if (!status) return null;
    const files = Number(status.files_used || 0) + Number(status.files_reserved || 0);
    const bytes = Number(status.bytes_used || 0) + Number(status.bytes_reserved || 0);
    const publications = Number(status.publications_used || 0) + Number(status.publications_reserved || 0);
    const fileLimit = Math.max(0, Number(status.file_limit) || 0);
    const byteLimit = Math.max(0, Number(status.byte_limit) || 0);
    const publicationLimit = Math.max(0, Number(status.publication_limit) || 0);
    const headerActions = editable && typeof onEdit === 'function'
        ? el('button', {
            type: 'button', class: 'pill-btn pill-btn--soft pill-btn--sm', onclick: onEdit
        }, createIcon('edit'), el('span', {}, t('publicationQuota.adjust')))
        : null;
    const badge = status.unlimited
        ? el('span', {class: 'publication-quota-badge is-unlimited'}, t('publicationQuota.unlimited'))
        : el('span', {class: 'publication-quota-badge'}, periodLabel(status.period));
    return el('section', {class: 'publication-quota-panel'},
        el('header', {class: 'publication-quota-header'},
            el('div', {}, el('h3', {}, createIcon('database'), t('publicationQuota.title')),
                el('p', {}, t('publicationQuota.subtitle'))),
            el('div', {class: 'publication-quota-header-actions'}, badge, headerActions)
        ),
        status.unlimited ? el('div', {class: 'publication-quota-unlimited'},
            createIcon('success'), el('span', {}, t('publicationQuota.unlimitedHint'))
        ) : el('div', {class: 'publication-quota-grid'},
            quotaMetric(t('publicationQuota.files'), `${files} / ${fileLimit}`, fileLimit ? files / fileLimit : 1),
            quotaMetric(t('publicationQuota.storage'), `${formatBytes(bytes)} / ${formatBytes(byteLimit)}`,
                byteLimit ? bytes / byteLimit : 1),
            quotaMetric(t('publicationQuota.publications'), `${publications} / ${publicationLimit}`,
                publicationLimit ? publications / publicationLimit : 1)
        ),
        status.inherited ? el('small', {class: 'publication-quota-inherited'},
            t('publicationQuota.inherited')) : null
    );
}

/**
 * Open the administrator quota editor for one account or global team.
 * @param {{ownerType: 'user'|'super_team', ownerKey: string, onSaved?: function(object): void}} options - Owner and refresh callback.
 * @returns {Promise<void>} Completion.
 */
export async function openPublicationQuotaDialog({ownerType, ownerKey, onSaved = null}) {
    let status;
    try {
        const response = await apiRequest(quotaURL(ownerType, ownerKey));
        if (!response.ok) throw await localizedResponseError(response, 'publicationQuota.loadFailed');
        status = await response.json();
    } catch (error) {
        showAlert(caughtErrorMessage(error, 'publicationQuota.loadFailed'), 'error');
        return;
    }

    let inherited = Boolean(status.inherited);
    let unlimited = Boolean(status.unlimited);
    let period = status.period || 'month';
    const fileInput = el('input', {
        class: 'profile-input', type: 'number', min: '0', max: '10000000', step: '1', value: String(status.file_limit ?? 600)
    });
    const byteInput = el('input', {
        class: 'profile-input', type: 'number', min: '0', max: String(1 << 30), step: '1',
        value: String(Math.round(Number(status.byte_limit || 0) / mebibyte))
    });
    const publicationInput = el('input', {
        class: 'profile-input', type: 'number', min: '0', max: '1000000', step: '1',
        value: String(status.publication_limit ?? 20)
    });
    const periodSelect = makeCustomSelect([
        {value: 'day', label: periodLabel('day')},
        {value: 'week', label: periodLabel('week')},
        {value: 'month', label: periodLabel('month')},
    ], period, value => { period = value; });
    const fields = el('div', {class: 'publication-quota-form-fields'},
        el('label', {}, el('span', {}, t('publicationQuota.filesLimit')), fileInput),
        el('label', {}, el('span', {}, t('publicationQuota.storageLimitMiB')), byteInput),
        el('label', {}, el('span', {}, t('publicationQuota.publicationLimit')), publicationInput),
        el('label', {}, el('span', {}, t('publicationQuota.period')), periodSelect)
    );
    /**
     * Synchronize inherited and unlimited state across mutable controls.
     * @returns {void}
     */
    const syncDisabled = () => {
        const disabled = inherited || unlimited;
        for (const input of [fileInput, byteInput, publicationInput]) input.disabled = disabled;
        periodSelect.classList.toggle('is-disabled', disabled);
        periodSelect.setAttribute('aria-disabled', String(disabled));
        const periodButton = periodSelect.querySelector('button');
        if (periodButton) periodButton.disabled = disabled;
        unlimitedToggle.toggleAttribute('disabled', inherited);
    };
    const inheritToggle = createToggle(inherited, checked => {
        inherited = checked;
        syncDisabled();
    });
    const unlimitedToggle = createToggle(unlimited, checked => {
        unlimited = checked;
        syncDisabled();
    });
    syncDisabled();

    RenopDialog.show({
        id: 'publication-quota-dialog', maxWidth: '620px', icon: 'database',
        title: t('publicationQuota.editTitle', {owner: ownerKey}),
        body: el('div', {class: 'publication-quota-form'},
            createPublicationQuotaPanel(status),
            el('div', {class: 'publication-quota-toggle-row'},
                el('span', {}, el('strong', {}, t('publicationQuota.useDefaults')),
                    el('small', {}, t('publicationQuota.useDefaultsHint'))), inheritToggle),
            el('div', {class: 'publication-quota-toggle-row'},
                el('span', {}, el('strong', {}, t('publicationQuota.unlimited')),
                    el('small', {}, t('publicationQuota.unlimitedAdminHint'))), unlimitedToggle),
            fields
        ),
        footer: [
            {text: t('common.cancel'), className: 'action-btn', onClick: (event, dialog) => dialog.close(false)},
            {
                text: t('common.save'), className: 'action-btn primary-btn',
                onClick: async (event, dialog) => runButtonAction(event.currentTarget, async () => {
                    const fileLimit = Number(fileInput.value);
                    const byteLimitMiB = Number(byteInput.value);
                    const publicationLimit = Number(publicationInput.value);
                    if (!inherited && (!Number.isSafeInteger(fileLimit) || fileLimit < 0 ||
                        !Number.isSafeInteger(byteLimitMiB) || byteLimitMiB < 0 ||
                        !Number.isSafeInteger(publicationLimit) || publicationLimit < 0)) {
                        showAlert(t('publicationQuota.invalid'), 'error');
                        return;
                    }
                    const payload = inherited ? {} : {
                        file_limit: fileLimit,
                        byte_limit: byteLimitMiB * mebibyte,
                        publication_limit: publicationLimit,
                        period,
                        unlimited,
                    };
                    try {
                        const response = await apiRequest(quotaURL(ownerType, ownerKey), {
                            method: 'PUT', headers: {'Content-Type': 'application/json'}, body: JSON.stringify(payload)
                        });
                        if (!response.ok) throw await localizedResponseError(response, 'publicationQuota.saveFailed');
                        const saved = await response.json();
                        dialog.close(true);
                        showAlert(t('publicationQuota.saved'), 'success');
                        if (typeof onSaved === 'function') onSaved(saved);
                    } catch (error) {
                        showAlert(caughtErrorMessage(error, 'publicationQuota.saveFailed'), 'error');
                    }
                })
            }
        ]
    });
}

/**
 * Add own-account publication quota usage to the profile editor.
 * @param {object|null|undefined} status - Embedded own-profile quota status.
 * @returns {void}
 */
export function renderProfilePublicationQuota(status) {
    const settings = document.querySelector('#profile-edit-view .profile-settings-card');
    if (!settings) return;
    settings.querySelector('.profile-publication-quota')?.remove();
    const panel = createPublicationQuotaPanel(status);
    if (!panel) return;
    panel.classList.add('profile-publication-quota');
    settings.appendChild(panel);
}
