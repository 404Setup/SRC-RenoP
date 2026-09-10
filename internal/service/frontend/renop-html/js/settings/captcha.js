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
import {morphElementHeight} from '@renop/ui/height-anim';
import {createSection, buildInput} from '../cfg-ui.js';
import {createFieldRow, createToggleRow, createIcon, createCallout} from '../components.js';
import {t} from '../i18n.js';

/** Render one provider and its independent browser-action scopes with a write-only secret. */
export function renderCaptchaSettings(container, data, changed) {
    const section = createSection(createIcon('compliance'), t('captcha.title'), t('captcha.settingsHint'));
    const fields = section.querySelector('.cfg-fields');
    const details = el('div', {class: 'cfg-fields'});
    const provider = makeCustomSelect([
        {value: 'disabled', label: t('captcha.disabled')},
        {value: 'recaptcha_v2', label: 'Google reCAPTCHA v2 · ' + t('captcha.checkbox')},
        {value: 'recaptcha_invisible', label: 'Google reCAPTCHA v2 · ' + t('captcha.invisible')},
        {value: 'recaptcha_v3', label: 'Google reCAPTCHA v3'},
        {value: 'turnstile', label: 'Cloudflare Turnstile'},
        {value: 'hcaptcha', label: 'hCaptcha'},
        {value: 'friendlycaptcha', label: 'Friendly Captcha v2'},
    ], data.provider || 'disabled', value => {
        if (value !== data.provider) {
            data.provider = value; data.secret_key = ''; data.secret_configured = false;
            changed(); render();
        }
    });
    fields.append(createFieldRow(t('captcha.provider'), '', provider), details);
    data.scopes ||= {};
    const scopes = createSection(createIcon('compliance'), t('captcha.scopes'), t('captcha.automationHint'));
    for (const scope of ['password_login', 'registration', 'manual_mail', 'super_team_create', 'domain_create', 'package_create']) {
        scopes.querySelector('.cfg-fields').append(createToggleRow(t(`captcha.scope.${scope}`), '', data.scopes[scope] === true,
            value => { data.scopes[scope] = value; changed(); }));
    }
    container.append(section, scopes);

    /** Keep provider-only fields and native validity synchronized with the selected configuration. */
    function render() {
        void morphElementHeight(details, () => {
            details.replaceChildren();
            if (data.provider === 'disabled') { details.append(createCallout('info', t('captcha.disabled'))); return; }
            const secret = buildInput('password', data.secret_key || '', '', event => { data.secret_key = event.target.value; changed(); });
            secret.maxLength = 1024; secret.autocomplete = 'new-password'; secret.required = !data.secret_configured;
            const siteKey = buildInput('text', data.site_key || '', '', event => {
                if (data.site_key !== event.target.value) data.secret_configured = false;
                data.site_key = event.target.value; secret.required = !data.secret_configured; changed();
            });
            siteKey.maxLength = 256; siteKey.required = true;
            details.append(createFieldRow(t('captcha.siteKey'), t('captcha.hostHint'), siteKey),
                createFieldRow(t('captcha.secretKey'), t('captcha.secretHint'), secret));
            if (data.provider === 'recaptcha_v3') {
                const score = buildInput('number', data.min_score ?? 0.5, '0.5', event => { data.min_score = Number(event.target.value); changed(); });
                Object.assign(score, {min: '0', max: '1', step: '0.01', required: true});
                details.append(createFieldRow(t('captcha.score'), t('captcha.scoreHint'), score));
            }
            if (data.provider === 'friendlycaptcha') {
                const region = makeCustomSelect([{value: 'global', label: t('captcha.global')}, {value: 'eu', label: t('captcha.eu')}],
                    data.friendly_region || 'global', value => { data.friendly_region = value; changed(); });
                details.append(createFieldRow(t('captcha.region'), '', region));
            }
        }, {duration: 240});
    }
    render();
}
