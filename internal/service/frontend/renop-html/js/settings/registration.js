/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {el} from '@renop/ui/dom';
import {makeCustomSelect} from '@renop/ui/custom-select';
import {buildInput, createSection} from '../cfg-ui.js';
import {createFieldRow, createIcon, createToggleRow} from '../components.js';
import {t} from '../i18n.js';

/** Render live registration policy and its persistent account and provider limits. */
export function renderRegistrationSettings(container, data, changed) {
    const section = createSection(createIcon('user'), t('registration.settingsTitle'), t('registration.settingsHint'), {defaultCollapsed: true});
    section.id = 'settings-registration';
    const fields = section.querySelector('.cfg-fields');
    fields.appendChild(createToggleRow(t('registration.enabled'), '', data.enabled === true, checked => { data.enabled = checked; changed(); }));
    const limit = buildInput('number', data.ip_limit, '1', event => { data.ip_limit = Number(event.target.value); changed(); });
    limit.min = '1'; limit.max = '10000'; limit.step = '1';
    fields.appendChild(createFieldRow(t('registration.ipLimit'), '', limit));
    for (const key of ['ip_interval', 'provider_cooldown']) {
        const row = el('div', {class: 'mail-inline'});
        const amount = buildInput('number', data[key].value, '', event => { data[key].value = Number(event.target.value); changed(); });
        amount.min = '1'; amount.max = '525600'; amount.step = '1';
        amount.setAttribute('aria-label', t('mail.count'));
        const unit = makeCustomSelect(['minute', 'hour', 'day', 'week', 'month'].map(value => ({value, label: t(`mail.${value}`)})),
            data[key].unit, value => { data[key].unit = value; changed(); });
        unit.querySelector('button')?.setAttribute('aria-label', t('mail.unit'));
        row.append(createFieldRow(t('mail.count'), '', amount), createFieldRow(t('mail.unit'), '', unit));
        fields.appendChild(createFieldRow(t(`registration.${key}`), t('registration.intervalHint'), row));
    }
    container.appendChild(section);
}
