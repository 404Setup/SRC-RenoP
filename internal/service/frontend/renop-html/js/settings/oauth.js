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
import {createCallout, createFieldRow, createIcon, createToggleRow} from '../components.js';
import {t} from '../i18n.js';

/** Render provider presets and one bounded OAuth client editor with write-only credentials. */
export function renderOAuthSettings(container, data, changed) {
    const wrap = el('div', {class: 'cfg-layout'});
    const section = createSection(createIcon('user'), t('oauth.settingsTitle'), t('oauth.settingsHint'), {defaultCollapsed: true});
    section.id = 'settings-oauth';
    const fields = section.querySelector('.cfg-fields');

    let preset = data.presets[0];
    let savedIDs = new Set(data.providers.map(value => value.id));

    const presetSelect = makeCustomSelect(data.presets.map(value => ({
        value: value.type,
        label: value.name
    })), preset.type, value => {
        preset = data.presets.find(item => item.type === value);
    });
    presetSelect.querySelector('button')?.setAttribute('aria-label', t('oauth.provider'));

    const add = el('button', {type: 'button', class: 'pill-btn pill-btn--soft'}, t('oauth.add'));

    const providersList = el('div', {class: 'cfg-service-stack', style: {marginTop: '1rem'}});

    add.addEventListener('click', () => {
        if (data.providers.filter(value => value.type !== 'github').length >= 32) return;
        let id = preset.type, suffix = 2;
        while (data.providers.some(value => value.id === id)) id = preset.type + '-' + suffix++;
        const newProvider = {
            ...structuredClone(preset),
            id,
            callback_url: window.location.origin + '/api/auth/oauth/' + id + '/callback'
        };
        data.providers.push(newProvider);
        changed();
        render(newProvider.id);
    });

    fields.append(
        createFieldRow(t('oauth.add'), '', el('div', {class: 'mail-inline'}, presetSelect, add)),
        providersList
    );

    wrap.appendChild(section);
    container.appendChild(wrap);

    function render(newId = null) {
        add.disabled = data.providers.filter(value => value.type !== 'github').length >= 32;
        providersList.replaceChildren();

        if (data.providers.length === 0) {
            providersList.appendChild(createCallout('info', t('oauth.none')));
            return;
        }

        data.providers.forEach(prov => {
            const isNew = prov.id === newId;
            const getOAuthIcon = type => {
                const raw = (type || '').toLowerCase();
                if (['github', 'google', 'microsoft', 'entra', 'gitlab', 'cloudflare', 'stackexchange'].includes(raw)) {
                    return raw;
                }
                return 'oauthCustom';
            };
            const provSection = createSection(
                createIcon(getOAuthIcon(prov.type)),
                prov.name || prov.id,
                '',
                {defaultCollapsed: !isNew}
            );
            const provFields = provSection.querySelector('.cfg-fields');
            const provTitle = provSection.querySelector('.cfg-section-title');
            const provSubtitle = provSection.querySelector('.cfg-section-subtitle');

            const updateHeader = () => {
                const providerType = prov.type || 'custom';
                const displayName = prov.name || prov.id;
                if (provTitle) {
                    provTitle.replaceChildren(
                        el('span', {class: 'cfg-service-title-text'}, displayName),
                        el('span', {class: `cfg-service-badge ${prov.enabled ? 'cfg-service-badge--active' : 'cfg-service-badge--disabled'}`},
                            prov.enabled ? t('oauth.enabled') : t('common.no')
                        ),
                        el('span', {class: 'cfg-service-badge cfg-service-badge--provider'}, providerType)
                    );
                }
                if (provSubtitle) {
                    const metaParts = [prov.id];
                    if (prov.callback_url) metaParts.push(prov.callback_url);
                    provSubtitle.textContent = metaParts.join(' · ');
                }
                const iconDiv = provSection.querySelector('.cfg-section-icon');
                if (iconDiv) {
                    iconDiv.replaceChildren(createIcon(getOAuthIcon(prov.type), {width: 20, height: 20}));
                }
                provSection.classList.toggle('cfg-service-card--active', Boolean(prov.enabled));
            };
            updateHeader();

            function makeInput(object, key, {type = 'text', max = 2048, hint = '', required = false, label = key} = {}) {
                const control = buildInput(type, object[key], '', event => {
                    object[key] = event.target.value;
                    changed();
                });
                control.maxLength = max;
                control.required = required;
                control.dataset.oauthField = key;
                control.setAttribute('aria-label', t(`oauth.${label}`));
                control.addEventListener('blur', () => {
                    if (typeof object[key] === 'string' && ['client_id', 'client_secret', 'api_key', 'callback_url'].includes(key)) {
                        const trimmed = object[key].trim();
                        if (trimmed !== object[key]) {
                            object[key] = trimmed;
                            control.value = trimmed;
                            changed();
                        }
                    }
                });
                provFields.appendChild(createFieldRow(t(`oauth.${label}`), hint, control));
                return control;
            }

            provFields.appendChild(createToggleRow(t('oauth.enabled'), '', prov.enabled === true, value => {
                prov.enabled = value;
                updateHeader();
                changed();
            }));

            const idInput = makeInput(prov, 'id', {max: 32, required: true, hint: t('oauth.idHint')});
            idInput.pattern = '[a-z][a-z0-9_\\-]{0,31}';
            idInput.disabled = prov.type === 'github' || savedIDs.has(prov.id);
            let previousID = idInput.value;
            idInput.addEventListener('input', () => {
                const callback = provFields.querySelector('[data-oauth-field="callback_url"]');
                if (prov.callback_url === window.location.origin + '/api/auth/oauth/' + previousID + '/callback') {
                    prov.callback_url = window.location.origin + '/api/auth/oauth/' + prov.id + '/callback';
                    if (callback) callback.value = prov.callback_url;
                }
                previousID = prov.id;
                updateHeader();
            });

            const nameInput = makeInput(prov, 'name', {max: 80, required: true});
            nameInput.disabled = prov.type === 'github';
            nameInput.addEventListener('input', updateHeader);

            makeInput(prov, 'client_id', {max: prov.type === 'github' ? 128 : 512, required: prov.enabled});
            if (prov.token_auth !== 'none') {
                makeInput(prov, 'client_secret', {
                    type: 'password',
                    max: prov.type === 'github' ? 512 : 4096,
                    hint: t(prov.client_secret_configured ? 'oauth.secretSaved' : 'oauth.secretHint')
                });

                if (prov.client_secret_configured) {
                    provFields.appendChild(createToggleRow(t('oauth.clearSecret'), '', prov.clear_client_secret === true, value => {
                        prov.clear_client_secret = value;
                        changed();
                    }));
                }
            }

            if (prov.type === 'github' && !prov.callback_url) {
                prov.callback_url = window.location.origin + '/api/auth/github/callback';
            }
            makeInput(prov, 'callback_url', {
                type: 'url',
                required: prov.enabled,
                hint: t(prov.type === 'github' ? 'settings.githubOAuthCallbackHint' : 'oauth.callbackHint')
            });

            if (prov.type === 'microsoft') makeInput(prov, 'tenant', {max: 253, hint: t('oauth.tenantHint')});
            if (prov.type === 'gitlab') makeInput(prov, 'base_url', {type: 'url', hint: t('oauth.gitlabHint')});
            if (prov.type === 'stackexchange') {
                makeInput(prov, 'site', {max: 128});
                makeInput(prov, 'api_key', {
                    type: 'password',
                    max: 4096,
                    hint: t(prov.api_key_configured ? 'oauth.secretSaved' : 'oauth.keyHint')
                });
                if (prov.api_key_configured) {
                    provFields.appendChild(createToggleRow(t('oauth.clearKey'), '', prov.clear_api_key === true, value => {
                        prov.clear_api_key = value;
                        changed();
                    }));
                }
            }
            if (prov.type !== 'github') makeInput(prov, 'scopes', {max: 1024, hint: t('oauth.scopesHint')});

            if (prov.type === 'custom') {
                for (const key of ['authorize_url', 'token_url', 'userinfo_url', 'issuer', 'jwks_url']) {
                    makeInput(prov, key, {
                        type: 'url',
                        required: prov.enabled && !['issuer', 'jwks_url'].includes(key),
                        hint: ['issuer', 'jwks_url'].includes(key) ? t('oauth.oidcHint') : ''
                    });
                }
            }

            if (prov.type === 'custom' || prov.type === 'cloudflare') {
                const defaultTokenAuth = 'client_secret_post';
                const tokenAuth = makeCustomSelect(['client_secret_post', 'client_secret_basic', 'none'].map(value => ({
                    value,
                    label: value
                })), prov.token_auth || defaultTokenAuth, value => {
                    prov.token_auth = value;
                    if (value === 'none') {
                        prov.disable_pkce = false;
                    }
                    changed();
                    render(prov.id);
                });
                tokenAuth.querySelector('button')?.setAttribute('aria-label', t('oauth.token_auth'));
                provFields.appendChild(createFieldRow(t('oauth.token_auth'), '', tokenAuth));
                if (prov.token_auth !== 'none') {
                    provFields.appendChild(createToggleRow(t('oauth.disable_pkce'), t('oauth.pkceHint'), prov.disable_pkce === true, value => {
                        prov.disable_pkce = value;
                        changed();
                    }));
                }
            }

            if (prov.type === 'custom') {
                prov.claims ||= {};
                for (const key of ['subject', 'username', 'name', 'email', 'email_verified', 'avatar']) {
                    makeInput(prov.claims, key, {
                        max: 128,
                        required: key === 'subject' && prov.enabled,
                        label: 'claim_' + key,
                        hint: t('oauth.claimHint')
                    });
                }
            }

            if (prov.type !== 'github') {
                const remove = el('button', {type: 'button', class: 'pill-btn pill-btn--soft'}, t('oauth.remove'));
                remove.addEventListener('click', async () => {
                    if (!(await window.showConfirm(t('oauth.removeConfirm', {provider: prov.name || prov.id})))) return;
                    data.providers.splice(data.providers.indexOf(prov), 1);
                    changed();
                    render();
                });
                provFields.appendChild(el('div', {class: 'mail-actions'}, remove));
            }

            providersList.appendChild(provSection);
            if (isNew) {
                window.requestAnimationFrame(() => provSection.querySelector('input')?.focus());
            }
        });
    }

    section.addEventListener('oauth-saved', () => {
        savedIDs = new Set(data.providers.map(value => value.id));
        render();
    });

    render();
}
