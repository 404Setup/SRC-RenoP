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
import {createFieldRow, createIcon, createToggleRow} from '../components.js';
import {t} from '../i18n.js';

/** Render provider presets and one bounded OAuth client editor with write-only credentials. */
export function renderOAuthSettings(container, data, changed) {
    const section = createSection(createIcon('user'), t('oauth.settingsTitle'), t('oauth.settingsHint'), {defaultCollapsed: true});
    section.id = 'settings-oauth';
    const fields = section.querySelector('.cfg-fields');
    const picker = el('div', {class: 'mail-picker'}), editor = el('div', {class: 'cfg-fields'});
    let selected = data.providers[0], preset = data.presets[0];
    let savedIDs = new Set(data.providers.map(value => value.id));
    const presetSelect = makeCustomSelect(data.presets.map(value => ({
            value: value.type,
            label: value.name
        })), preset.type,
        value => {
            preset = data.presets.find(item => item.type === value);
        });
    presetSelect.querySelector('button')?.setAttribute('aria-label', t('oauth.provider'));
    const add = el('button', {type: 'button', class: 'pill-btn pill-btn--soft'}, t('oauth.add'));
    add.addEventListener('click', () => {
        if (data.providers.length >= 32) return;
        let id = preset.type, suffix = 2;
        while (data.providers.some(value => value.id === id)) id = preset.type + '-' + suffix++;
        selected = {
            ...structuredClone(preset),
            id,
            callback_url: window.location.origin + '/api/auth/oauth/' + id + '/callback'
        };
        data.providers.push(selected);
        changed();
        render();
        editor.querySelector('input')?.focus();
    });
    fields.append(createFieldRow(t('oauth.add'), '', el('div', {class: 'mail-inline'}, presetSelect, add)),
        createFieldRow(t('oauth.provider'), '', picker), editor);

    /** Bind native input limits and labels without exposing saved secrets. */
    function input(object, key, {type = 'text', max = 2048, hint = '', required = false, label = key} = {}) {
        const control = buildInput(type, object[key], '', event => {
            object[key] = event.target.value;
            changed();
        });
        control.maxLength = max;
        control.required = required;
        control.dataset.oauthField = key;
        control.setAttribute('aria-label', t(`oauth.${label}`));
        editor.appendChild(createFieldRow(t(`oauth.${label}`), hint, control));
        return control;
    }

    /** Rebuild the selected provider after list changes or a successful secret write. */
    function render() {
        add.disabled = data.providers.length >= 32;
        picker.replaceChildren();
        editor.replaceChildren();
        if (!selected || !data.providers.includes(selected)) selected = data.providers[0];
        if (!selected) {
            editor.append(el('p', {class: 'cfg-hint'}, t('oauth.none')));
            return;
        }
        const select = makeCustomSelect(data.providers.map((value, index) => ({
                value: String(index),
                label: value.name || value.id
            })),
            String(data.providers.indexOf(selected)), value => {
                selected = data.providers[Number(value)];
                render();
                picker.querySelector('button')?.focus();
            });
        select.querySelector('button')?.setAttribute('aria-label', t('oauth.provider'));
        picker.appendChild(select);
        editor.appendChild(createToggleRow(t('oauth.enabled'), '', selected.enabled === true,
            value => {
                selected.enabled = value;
                changed();
                render();
                editor.querySelector('renop-toggle input')?.focus();
            }));
        const id = input(selected, 'id', {max: 32, required: true, hint: t('oauth.idHint')});
        id.pattern = '[a-z][a-z0-9_\\-]{0,31}';
        id.disabled = savedIDs.has(selected.id);
        let previousID = id.value;
        id.addEventListener('input', () => {
            const callback = editor.querySelector('[data-oauth-field="callback_url"]');
            if (selected.callback_url === window.location.origin + '/api/auth/oauth/' + previousID + '/callback') {
                selected.callback_url = window.location.origin + '/api/auth/oauth/' + selected.id + '/callback';
                callback.value = selected.callback_url;
            }
            previousID = selected.id;
        });
        input(selected, 'name', {max: 80, required: true});
        input(selected, 'client_id', {max: 512, required: selected.enabled});
        input(selected, 'client_secret', {
            type: 'password',
            max: 4096,
            hint: t(selected.client_secret_configured ? 'oauth.secretSaved' : 'oauth.secretHint')
        });
        if (selected.client_secret_configured) editor.appendChild(createToggleRow(t('oauth.clearSecret'), '', selected.clear_client_secret === true,
            value => {
                selected.clear_client_secret = value;
                changed();
            }));
        input(selected, 'callback_url', {type: 'url', required: selected.enabled, hint: t('oauth.callbackHint')});
        if (selected.type === 'microsoft') input(selected, 'tenant', {max: 253, hint: t('oauth.tenantHint')});
        if (selected.type === 'gitlab') input(selected, 'base_url', {type: 'url', hint: t('oauth.gitlabHint')});
        if (selected.type === 'stackexchange') {
            input(selected, 'site', {max: 128});
            input(selected, 'api_key', {
                type: 'password',
                max: 4096,
                hint: t(selected.api_key_configured ? 'oauth.secretSaved' : 'oauth.keyHint')
            });
            if (selected.api_key_configured) editor.appendChild(createToggleRow(t('oauth.clearKey'), '', selected.clear_api_key === true,
                value => {
                    selected.clear_api_key = value;
                    changed();
                }));
        }
        input(selected, 'scopes', {max: 1024, hint: t('oauth.scopesHint')});
        if (selected.type === 'custom') {
            for (const key of ['authorize_url', 'token_url', 'userinfo_url', 'issuer', 'jwks_url']) {
                input(selected, key, {
                    type: 'url', required: selected.enabled && !['issuer', 'jwks_url'].includes(key),
                    hint: ['issuer', 'jwks_url'].includes(key) ? t('oauth.oidcHint') : ''
                });
            }
            const tokenAuth = makeCustomSelect(['client_secret_post', 'client_secret_basic', 'none'].map(value => ({
                    value,
                    label: value
                })),
                selected.token_auth, value => {
                    selected.token_auth = value;
                    changed();
                });
            tokenAuth.querySelector('button')?.setAttribute('aria-label', t('oauth.token_auth'));
            editor.appendChild(createFieldRow(t('oauth.token_auth'), '', tokenAuth));
            editor.appendChild(createToggleRow(t('oauth.disable_pkce'), t('oauth.pkceHint'), selected.disable_pkce === true,
                value => {
                    selected.disable_pkce = value;
                    changed();
                }));
            selected.claims ||= {};
            for (const key of ['subject', 'username', 'name', 'email', 'email_verified', 'avatar']) {
                input(selected.claims, key, {
                    max: 128,
                    required: key === 'subject' && selected.enabled,
                    label: 'claim_' + key,
                    hint: t('oauth.claimHint')
                });
            }
        }
        const remove = el('button', {type: 'button', class: 'pill-btn pill-btn--soft'}, t('oauth.remove'));
        remove.addEventListener('click', async () => {
            const target = selected;
            if (!(await window.showConfirm(t('oauth.removeConfirm', {provider: target.name})))) return;
            data.providers.splice(data.providers.indexOf(target), 1);
            selected = data.providers[0];
            changed();
            render();
            (picker.querySelector('button') || add).focus();
        });
        editor.appendChild(el('div', {class: 'mail-actions'}, remove));
    }

    section.addEventListener('oauth-saved', () => {
        selected = data.providers.find(value => value.id === selected?.id);
        savedIDs = new Set(data.providers.map(value => value.id));
        render();
    });
    render();
    container.appendChild(section);
}
