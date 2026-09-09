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
import {apiRequest} from '../api.js';
import {buildInput, createSection} from '../cfg-ui.js';
import {createFieldRow, createIcon, createToggleRow, runButtonAction} from '../components.js';
import {showAlert} from '../alert.js';
import {responseErrorMessage} from '../response-errors.js';
import {t} from '../i18n.js';

/** Render cache backend settings and a connection check without exposing stored credentials. */
export function renderCacheSettings(container, data, changed) {
    const wrap = el('div', {class: 'cfg-layout'});
    const section = createSection(createIcon('database'), t('cache.title'), t('cache.restart'), {defaultCollapsed: true});
    const fields = section.querySelector('.cfg-fields');
    const mode = makeCustomSelect([
        {value: 'memory', label: t('cache.memory')},
        {value: 'redis', label: 'Redis'},
        {value: 'valkey', label: 'Valkey'},
    ], data.mode || 'memory', value => {
        data.mode = value;
        changed();
    });
    fields.appendChild(createFieldRow(t('cache.mode'), t('cache.scope'), mode));
    for (const [key, type, placeholder, min, max] of [
        ['address', 'text', 'localhost:6379'], ['username', 'text', ''],
        ['password', 'password', ''], ['database', 'number', '0', 0, 65535],
        ['timeout_ms', 'number', '1000', 10, 10000],
    ]) {
        const input = buildInput(type, key === 'password' ? '' : (data[key] ?? ''), placeholder, event => {
            const value = type === 'number' ? Number(event.target.value) : event.target.value;
            if (type === 'number' && (!Number.isSafeInteger(value) || value < min || value > max)) return;
            data[key] = value;
            changed();
        });
        if (type === 'number') {
            input.min = String(min);
            input.max = String(max);
            input.step = '1';
        }
        if (key === 'password') {
            input.id = 'settings-cache-password';
            input.autocomplete = 'new-password';
        }
        fields.appendChild(createFieldRow(t(`cache.${key}`), key === 'password' ? t('cache.keepPassword') : '', input));
    }
    const clearPassword = createToggleRow(t('cache.clearPassword'), '', false, checked => {
        data.clear_password = checked;
        changed();
    });
    clearPassword.id = 'settings-cache-clear-password';
    fields.append(createToggleRow(t('cache.tls'), '', data.tls === true, checked => {
        data.tls = checked;
        changed();
    }), clearPassword);
    const test = el('button', {type: 'button', class: 'pill-btn pill-btn--soft'}, t('cache.test'));
    test.addEventListener('click', () => runButtonAction(test, async () => {
        try {
            const response = await apiRequest('/api/settings/cache/test', {
                method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify(data),
            });
            showAlert(response.ok ? t('cache.testPassed') : await responseErrorMessage(response, 'cache.testFailed'),
                response.ok ? 'success' : 'error');
        } catch {
            showAlert(t('cache.testFailed'), 'error');
        }
    }));
    fields.appendChild(test);
    wrap.appendChild(section);
    container.appendChild(wrap);
}
