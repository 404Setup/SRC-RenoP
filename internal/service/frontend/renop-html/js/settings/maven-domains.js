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
import {buildInput, createSection} from '../cfg-ui.js';
import {createFieldRow, createIcon} from '../components.js';
import {t} from '../i18n.js';

/** Render the reservation period applied when a publishing domain becomes unsafe. */
export function renderMavenDomainSettings(container, data, changed) {
    const wrap = el('div', {class: 'cfg-layout'});
    const section = createSection(createIcon('network'), t('maven.healthSettings'), t('maven.healthSettingsHint'), {defaultCollapsed: true});
    section.id = 'settings-maven-domains';
    const value = buildInput('number', data.release_value, '2', event => {
        data.release_value = Number(event.target.value);
        changed();
    });
    value.min = '1';
    value.max = '100';
    value.step = '1';
    value.setAttribute('aria-label', t('maven.releasePeriod'));
    const unit = makeCustomSelect(['month', 'year'].map(value => ({value, label: t(`maven.releaseUnit.${value}`)})),
        data.release_unit, unit => {
            data.release_unit = unit;
            changed();
        });
    unit.querySelector('button')?.setAttribute('aria-label', t('mail.unit'));
    section.querySelector('.cfg-fields').append(
        createFieldRow(t('maven.releasePeriod'), '', value), createFieldRow(t('mail.unit'), '', unit));
    wrap.appendChild(section);
    container.appendChild(wrap);
}
