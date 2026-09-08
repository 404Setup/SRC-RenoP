/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {el} from '@renop/ui/dom';
import {makeCustomSelect} from '@renop/ui/custom-select';
import {apiRequest} from '../api.js';
import {buildInput, createSection, makeTagListInput} from '../cfg-ui.js';
import {createFieldRow, createIcon, createToggleRow, runButtonAction} from '../components.js';
import {showAlert} from '../alert.js';
import {LocalizedResponseError, responseErrorMessage} from '../response-errors.js';
import {formatTimestamp} from '../time.js';
import {t} from '../i18n.js';

const PROVIDERS = {smtp: 'SMTP', cloudflare: 'Cloudflare Email', graph: 'Microsoft Graph', ses: 'Amazon SES', sendgrid: 'Twilio SendGrid', gmail: 'Gmail', aliyun: 'Alibaba Cloud Direct Mail', tencent: 'Tencent Cloud SES', feishu: 'Feishu / Lark Mail'};
const STATUSES = ['queued', 'paused', 'sending', 'checking', 'accepted', 'sent', 'delivered', 'failed', 'unknown', 'expired', 'cancelled', 'queued_provider'];
const SECRET_KEYS = ['password', 'api_key', 'api_secret', 'session_token', 'client_secret', 'access_token', 'refresh_token'];

/** Request JSON while exposing only localized, stable failure messages. */
async function requestJSON(path, options) {
    const response = await apiRequest('/api/settings/mail' + path, options);
    if (!response.ok) throw new LocalizedResponseError(await responseErrorMessage(response, 'mail.requestFailed'), response.status);
    return response.json();
}

/** Build a button with shared async feedback and localized failures. */
function action(label, callback) {
    const button = el('button', {type: 'button', class: 'pill-btn pill-btn--soft'}, label);
    button.addEventListener('click', () => runButtonAction(button, async () => {
        try { await callback(); } catch (error) { showAlert(error instanceof LocalizedResponseError ? error.message : t('mail.requestFailed'), 'error'); }
    }));
    return button;
}

/** Apply provider defaults without retaining another provider's credentials. */
export function applyMailPreset(account, preset) {
    const identity = Object.fromEntries(['id', 'name', 'enabled', 'from', 'from_name', 'scenes', 'quota', 'force_send', 'overage', 'balance_micros', 'fetch_balance']
        .filter(key => Object.hasOwn(account, key)).map(key => [key, account[key]]));
    if (account.pricing?.currency && account.pricing.currency !== preset.account.pricing?.currency) identity.balance_micros = null;
    for (const key of Object.keys(account)) delete account[key];
    Object.assign(account, structuredClone(preset.account), identity, {preset: preset.id});
}

/** Render global queue policy, provider accounts, previews, and delivery history. */
export function renderMailSettings(container, data, changed) {
    const wrap = el('div', {class: 'cfg-layout', id: 'settings-mail'});
    const section = createSection(createIcon('send'), t('mail.title'), t('mail.description'), {defaultCollapsed: true});
    const fields = section.querySelector('.cfg-fields');
    let presets = [], scenes = [], selectedID = data.accounts?.[0]?.id || '', testReceipt = null;
    data.accounts ||= [];
    const accountPicker = el('div', {class: 'mail-picker'});
    const accountEditor = el('div', {class: 'cfg-fields'});
    const accountStatus = el('div', {class: 'mail-status', role: 'status'});
    const testStatus = el('div', {class: 'mail-status', role: 'status', 'aria-live': 'polite'});
    const preview = el('iframe', {class: 'mail-preview', title: t('mail.preview'), sandbox: '', referrerpolicy: 'no-referrer'});
    const history = el('div', {class: 'mail-history', role: 'status'});
    let historyPage = 0, historyStatus = '', historyGeneration = 0, historyBusy = false, statusGeneration = 0;
    let previewScene = 'registration_verify';

    /** Add a bounded input and keep invalid edits visible to native validation. */
    function input(parent, object, key, {type = 'text', hint = '', min, max, scale = 1, optional = false, label = key} = {}) {
        const value = object[key] == null ? '' : type === 'number' ? object[key] / scale : object[key];
        const control = buildInput(type, type === 'number' ? value : object[key], '', event => {
            const raw = event.target.value;
            let next = raw;
            if (type === 'number') {
                next = optional && raw.trim() === '' ? null : Math.round(Number(raw) * scale);
                if (next !== null && (!Number.isSafeInteger(next) || min != null && next < min || max != null && next > max)) {
                    control.setCustomValidity(t('mail.invalidNumber'));
                    changed();
                    return;
                }
            }
            control.setCustomValidity('');
            object[key] = next;
            changed();
        });
        if (type === 'number') {
            control.step = String(1 / scale);
            if (min != null) control.min = String(min / scale);
            if (max != null) control.max = String(max / scale);
        }
        control.dataset.mailField = key;
        control.setAttribute('aria-label', t(`mail.${label}`));
        if (type === 'email') control.required = true;
        if (type === 'password') control.autocomplete = 'new-password';
        if (type === 'text') control.maxLength = 1024;
        parent.appendChild(createFieldRow(t(`mail.${label}`), hint, control));
        return control;
    }

    /** Add one select using existing keyboard and focus behavior. */
    function select(parent, object, key, values, label = key, hint = '') {
        const control = makeCustomSelect(values.map(value => typeof value === 'string' ? {value, label: t(`mail.${value}`)} : value), object[key], value => { object[key] = value; changed(); });
        control.dataset.mailSelect = key;
        control.querySelector('button')?.setAttribute('aria-label', t(`mail.${label}`));
        parent.appendChild(createFieldRow(t(`mail.${label}`), hint, control));
        return control;
    }

    /** Render a duration as separately labelled count and unit controls. */
    function interval(parent, value, label, units, allowDisabled = false) {
        const row = el('div', {class: 'mail-inline'});
        input(row, value, 'value', {type: 'number', min: allowDisabled ? undefined : 1, max: 31536000, label: 'count'});
        select(row, value, 'unit', units, 'unit');
        parent.appendChild(createFieldRow(t(`mail.${label}`), allowDisabled ? t(`mail.${label}Hint`) : '', row));
    }

    fields.appendChild(createToggleRow(t('mail.enabled'), '', data.enabled === true, checked => { data.enabled = checked; changed(); }));
    input(fields, data, 'public_url', {hint: t('mail.publicURLHint')});
    input(fields, data, 'site_name');
    select(fields, data, 'template_style', ['card', 'compact', 'notice']);
    select(fields, data, 'locale', [{value: 'en-US', label: 'English'}, {value: 'zh-CN', label: '简体中文'}]);
    interval(fields, data.delay, 'delay', ['second', 'minute', 'hour'], true);
    interval(fields, data.calibration, 'calibration', ['minute', 'hour', 'day'], true);
    for (const [key, units] of [['manual_rate', ['minute', 'hour', 'day']], ['account_rate', ['second', 'minute', 'hour', 'day']]]) {
        input(fields, data[key], 'limit', {type: 'number', min: 1, max: 1000000, label: key, hint: t(`mail.${key}Hint`)});
        interval(fields, data[key].interval, key + 'Interval', units);
    }
    select(fields, data, 'list_mode', ['blacklist', 'whitelist']);
    fields.appendChild(createFieldRow(t('mail.addresses'), t('mail.addressesHint'), makeTagListInput({
        items: data.addresses || [], placeholder: 'name@example.com, @example.com',
        onChange: values => { data.addresses = values; changed(); },
    })));

    const accountsSection = createSection(createIcon('user'), t('mail.accounts'), t('mail.routingHint'), {defaultCollapsed: true});
    const accountsFields = accountsSection.querySelector('.cfg-fields');
    const add = action(t('mail.addAccount'), async () => {
        if (data.accounts.length >= 64 || !presets.length) return;
        const preset = presets.find(value => value.id === 'smtp-custom') || presets[0];
        const id = Array.from(crypto.getRandomValues(new Uint32Array(4)), part => part.toString(16).padStart(8, '0')).join('');
        const account = {id, name: '', enabled: true, from: '', from_name: '', scenes: []};
        applyMailPreset(account, preset);
        data.accounts.push(account);
        selectedID = account.id;
        changed();
        renderAccounts();
        accountEditor.querySelector('input')?.focus();
    });
    add.disabled = true;
    accountsFields.append(createFieldRow(t('mail.accounts'), '', accountPicker), el('div', {class: 'mail-actions'}, add), accountEditor, accountStatus);

    /** Display the selected provider with only its relevant transport fields. */
    function renderAccounts() {
        accountPicker.replaceChildren();
        accountEditor.replaceChildren();
        accountStatus.replaceChildren();
        ++statusGeneration;
        add.disabled = !presets.length || data.accounts.length >= 64;
        const account = data.accounts.find(value => value.id === selectedID) || data.accounts[0];
        selectedID = account?.id || '';
        if (!account) {
            accountEditor.appendChild(el('p', {}, t('mail.noAccounts')));
            return;
        }
        accountPicker.appendChild(makeCustomSelect(data.accounts.map(value => ({value: value.id, label: value.name || value.from || PROVIDERS[value.provider]})), selectedID, value => {
            selectedID = value; renderAccounts();
        }));
        input(accountEditor, account, 'name');
        accountEditor.appendChild(createToggleRow(t('mail.accountEnabled'), '', account.enabled === true, checked => { account.enabled = checked; changed(); }));
        const provider = makeCustomSelect(Object.entries(PROVIDERS).map(([value, label]) => ({value, label})), account.provider, value => {
            const preset = presets.find(item => item.account.provider === value);
            if (!preset) return;
            applyMailPreset(account, preset);
            if (data.clear_secrets) delete data.clear_secrets[account.id];
            if (data.secrets_configured) delete data.secrets_configured[account.id];
            changed(); renderAccounts();
        });
        accountEditor.appendChild(createFieldRow(t('mail.provider'), '', provider));
        const choices = presets.filter(value => value.account.provider === account.provider);
        const picker = makeCustomSelect([{value: '', label: t('mail.custom')}, ...choices.map(value => ({value: value.id, label: value.name}))], account.preset || '', id => {
            const preset = choices.find(value => value.id === id);
            if (preset) {
                const keepSecrets = account.provider !== 'smtp' || account.smtp_host === preset.account.smtp_host && account.username === preset.account.username;
                const secrets = keepSecrets ? Object.fromEntries(SECRET_KEYS.map(key => [key, account[key] || ''])) : {};
                applyMailPreset(account, preset);
                Object.assign(account, secrets);
                if (!keepSecrets && data.secrets_configured) delete data.secrets_configured[account.id];
            } else account.preset = '';
            changed(); renderAccounts();
        });
        accountEditor.appendChild(createFieldRow(t('mail.preset'), t('mail.presetHint'), picker));
        const preset = choices.find(value => value.id === account.preset);
        if (preset) {
            const capabilities = ['quota_api', 'balance_api', 'status_api', 'paid_overage'].map(key => t(`mail.${key}`) + ': ' + t(preset[key] ? 'common.yes' : 'common.no')).join(' · ');
            accountEditor.appendChild(el('p', {class: 'hint-text'}, capabilities));
            if (preset.price_source?.startsWith('https://')) accountEditor.appendChild(el('a', {href: preset.price_source, target: '_blank', rel: 'noopener noreferrer'}, t('mail.priceSource') + ' · ' + preset.price_checked));
        }
        input(accountEditor, account, 'from', {type: 'email'});
        input(accountEditor, account, 'from_name');
        if (account.provider === 'smtp') {
            input(accountEditor, account, 'smtp_host');
            input(accountEditor, account, 'smtp_port', {type: 'number', min: 1, max: 65535});
            select(accountEditor, account, 'smtp_security', [{value: 'plain', label: 'SMTP'}, {value: 'tls', label: 'SMTP SSL/TLS'}, {value: 'starttls', label: 'STARTTLS'}]);
            input(accountEditor, account, 'username');
        } else {
            input(accountEditor, account, 'endpoint');
        }
        if (['ses', 'aliyun', 'tencent'].includes(account.provider)) input(accountEditor, account, 'region');
        if (account.provider === 'cloudflare') input(accountEditor, account, 'account_id');
        if (['graph', 'feishu'].includes(account.provider)) input(accountEditor, account, 'mailbox', {hint: t('mail.mailboxHint')});
        if (['graph', 'smtp'].includes(account.provider)) input(accountEditor, account, 'tenant', {hint: t('mail.tenantHint')});
        if (['graph', 'gmail', 'feishu', 'smtp'].includes(account.provider)) input(accountEditor, account, 'client_id');
        const secrets = account.provider === 'smtp' ? ['password', 'client_secret', 'access_token', 'refresh_token']
            : ['graph', 'gmail', 'feishu'].includes(account.provider) ? ['client_secret', 'access_token', 'refresh_token']
            : ['ses', 'aliyun', 'tencent'].includes(account.provider) ? ['api_key', 'api_secret', 'session_token'] : ['api_key'];
        for (const key of secrets) {
            const configured = data.secrets_configured?.[account.id]?.includes(key);
            input(accountEditor, account, key, {type: 'password', hint: configured ? t('mail.secretStored') : ''});
            if (configured) accountEditor.appendChild(createToggleRow(t('mail.clearSecret', {name: t(`mail.${key}`)}), '', data.clear_secrets?.[account.id]?.includes(key) || false, checked => {
                data.clear_secrets ||= {};
                const values = new Set(data.clear_secrets[account.id] || []);
                if (checked) values.add(key); else values.delete(key);
                data.clear_secrets[account.id] = [...values];
                changed();
            }));
        }
        const sceneList = el('div', {class: 'mail-scenes'});
        for (const scene of ['*', ...scenes]) {
            const checkbox = el('input', {type: 'checkbox', checked: account.scenes?.includes(scene) || false});
            checkbox.addEventListener('change', () => {
                const values = new Set(account.scenes || []);
                if (checkbox.checked) values.add(scene); else values.delete(scene);
                account.scenes = [...values]; changed();
            });
            sceneList.appendChild(el('label', {}, checkbox, el('span', {}, t(`mail.scene.${scene === '*' ? 'all' : scene}`))));
        }
        accountEditor.appendChild(createFieldRow(t('mail.scenes'), t('mail.routingHint'), sceneList));
        input(accountEditor, account.quota, 'limit', {type: 'number', max: 1000000000, label: 'quota', hint: t('mail.quotaHint')});
        select(accountEditor, account.quota, 'period', ['hour', 'day', 'week', 'month'], 'quotaPeriod');
        accountEditor.appendChild(createToggleRow(t('mail.force_send'), t('mail.forceHint'), account.force_send === true, value => { account.force_send = value; changed(); }));
        input(accountEditor, account.overage, 'limit', {type: 'number', max: 1000000000, label: 'overage', hint: t('mail.overageHint')});
        select(accountEditor, account.overage, 'period', ['hour', 'day', 'week', 'month'], 'overagePeriod');
        input(accountEditor, account, 'balance_micros', {type: 'number', scale: 1000000, optional: true, min: -1e15, max: 1e15, hint: t('mail.balanceHint')});
        if (['aliyun', 'tencent'].includes(account.provider)) {
            accountEditor.appendChild(createToggleRow(t('mail.fetch_balance'), '', account.fetch_balance === true, value => { account.fetch_balance = value; changed(); }));
            input(accountEditor, account, 'billing_endpoint');
        }
        input(accountEditor, account.pricing, 'currency');
        select(accountEditor, account.pricing, 'rounding', ['proportional', 'batch'], 'rounding', t('mail.pricingHint'));
        const tiers = el('div', {class: 'mail-tiers'});
        /** Rebuild ordered tiers after adding or removing a pricing boundary. */
        function renderTiers() {
            tiers.replaceChildren();
            account.pricing.tiers ||= [];
            account.pricing.tiers.forEach((tier, index) => {
                const row = el('div', {class: 'mail-tier'});
                input(row, tier, 'up_to', {type: 'number', min: 0, max: 1000000000, hint: t('mail.upToHint')});
                input(row, tier, 'amount_micros', {type: 'number', min: 0, max: 1e12, scale: 1000000});
                input(row, tier, 'batch_size', {type: 'number', min: 1, max: 1000000000});
                row.appendChild(action(t('common.remove'), async () => { account.pricing.tiers.splice(index, 1); changed(); renderTiers(); }));
                tiers.appendChild(row);
            });
            const addTier = action(t('mail.addTier'), async () => { account.pricing.tiers.push({up_to: 0, amount_micros: 0, batch_size: 1}); changed(); renderTiers(); });
            addTier.disabled = account.pricing.tiers.length >= 20;
            tiers.appendChild(addTier);
        }
        renderTiers();
        accountEditor.append(tiers, el('div', {class: 'mail-actions'}, action(t('mail.refreshStatus'), async () => {
            const generation = ++statusGeneration;
            const value = await requestJSON('/accounts/' + encodeURIComponent(account.id));
            if (generation !== statusGeneration || selectedID !== account.id || !wrap.isConnected) return;
            accountStatus.replaceChildren();
            for (const [key, amount] of [
                ['attempts', value.attempts], ['charged', value.charged], ['remaining', value.remaining_quota], ['overage', value.remaining_overage],
                ['balance_micros', value.balance_micros == null ? null : value.balance_micros / 1000000],
                ['spent', (value.spent_micros || 0) / 1000000],
                ['calibrationAt', formatTimestamp(value.calibration_at, {fallback: t('common.none')})],
            ]) accountStatus.appendChild(el('p', {}, t(`mail.${key}`) + ': ' + (amount ?? t('common.unknown'))));
            if (value.calibration_error) accountStatus.appendChild(el('p', {}, t('mail.calibrationFailed')));
        }), action(t('mail.removeAccount'), async () => {
            data.accounts.splice(data.accounts.indexOf(account), 1);
            if (data.clear_secrets) delete data.clear_secrets[account.id];
            changed(); renderAccounts();
        })));
    }

    const operations = createSection(createIcon('send'), t('mail.delivery'), t('mail.savedOnly'), {defaultCollapsed: true});
    const operationFields = operations.querySelector('.cfg-fields');
    const testTo = buildInput('email', '', 'name@example.com', () => {});
    testTo.required = true;
    testTo.dataset.mailTest = 'true';
    testTo.setAttribute('aria-label', t('mail.testTo'));
    operationFields.appendChild(createFieldRow(t('mail.testTo'), '', testTo));

    /** Refresh a test receipt by its private capability without exposing it in a URL. */
    async function updateTestStatus() {
        if (!testReceipt) return false;
        const receipt = testReceipt;
        const response = await apiRequest('/api/auth/mail/' + encodeURIComponent(receipt.id), {headers: {'X-Renop-Mail-Ticket': receipt.ticket}});
        if (!response.ok) throw new Error(t('mail.requestFailed'));
        const job = await response.json();
        if (testReceipt !== receipt || !wrap.isConnected) return false;
        testStatus.textContent = t(`mail.status.${STATUSES.includes(job.status) ? job.status : 'unknown'}`);
        return ['queued', 'sending', 'checking'].includes(job.status);
    }
    operationFields.append(el('div', {class: 'mail-actions'}, action(t('mail.sendTest'), async () => {
        if (!testTo.reportValidity()) return;
        const receipt = await requestJSON('/test', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({account_id: selectedID, to: testTo.value.trim()})});
        testReceipt = receipt;
        testStatus.textContent = t('mail.status.queued');
        const until = Date.now() + 120000;
        while (Date.now() < until && wrap.isConnected && testReceipt === receipt) {
            await new Promise(resolve => setTimeout(resolve, 2500));
            if (!wrap.isConnected || testReceipt !== receipt) break;
            if (!await updateTestStatus()) break;
        }
    }), action(t('mail.refreshStatus'), updateTestStatus)), testStatus);
    const scenePicker = el('div', {class: 'mail-picker'});
    operationFields.append(createFieldRow(t('mail.scenes'), '', scenePicker), el('div', {class: 'mail-actions'}, action(t('mail.preview'), async () => {
        const value = await requestJSON('/templates/' + encodeURIComponent(previewScene) + '?style=' + encodeURIComponent(data.template_style));
        if (wrap.isConnected) preview.srcdoc = value.html;
    })), el('div', {class: 'mail-padded'}, preview));
    const filter = makeCustomSelect([{value: '', label: t('mail.allStatuses')}, ...STATUSES.map(value => ({value, label: t(`mail.status.${value}`)}))], '', value => { historyStatus = value; historyPage = 0; void loadHistory(); });
    const previous = action(t('common.prev'), async () => { historyPage--; await loadHistory(); });
    const next = action(t('common.next'), async () => { historyPage++; await loadHistory(); });
    previous.disabled = next.disabled = true;

    /** Load one bounded page; later filter requests supersede older responses. */
    async function loadHistory() {
        const generation = ++historyGeneration;
        historyBusy = true;
        previous.disabled = next.disabled = true;
        history.textContent = t('common.loading');
        try {
            const value = await requestJSON('/jobs?limit=20&offset=' + (historyPage * 20) + '&status=' + encodeURIComponent(historyStatus));
            if (generation !== historyGeneration || !wrap.isConnected) return;
            history.replaceChildren();
            if (!value.items.length) history.appendChild(el('p', {}, t('mail.noJobs')));
            for (const job of value.items) {
                history.appendChild(el('div', {class: 'mail-job'},
                    el('strong', {}, data.accounts.find(account => account.id === job.account_id)?.name || job.account_id),
                    el('span', {}, t(`mail.scene.${job.scene}`)),
                    el('span', {}, t(`mail.status.${STATUSES.includes(job.status) ? job.status : 'unknown'}`)),
                    el('time', {}, formatTimestamp(job.created_at))));
            }
            history.appendChild(el('p', {}, t('common.pagination', {page: historyPage + 1, pages: Math.max(1, Math.ceil(value.total / 20)), total: value.total})));
            previous.disabled = historyPage === 0;
            next.disabled = (historyPage + 1) * 20 >= value.total || historyPage >= 500;
        } catch {
            if (generation === historyGeneration) history.textContent = t('mail.requestFailed');
        } finally { if (generation === historyGeneration) historyBusy = false; }
    }
    operationFields.append(createFieldRow(t('mail.allStatuses'), '', filter), el('div', {class: 'mail-actions'}, action(t('mail.refreshHistory'), async () => { if (!historyBusy) await loadHistory(); })), history, el('div', {class: 'mail-actions'}, previous, next));
    wrap.addEventListener('mail-saved', () => { renderAccounts(); });
    wrap.append(section, accountsSection, operations);
    container.appendChild(wrap);
    renderAccounts();
    void requestJSON('/presets').then(value => {
        if (!wrap.isConnected) return;
        presets = value.presets;
        scenes = value.scenes;
        scenePicker.appendChild(makeCustomSelect(scenes.map(value => ({value, label: t(`mail.scene.${value}`)})), previewScene, value => { previewScene = value; }));
        renderAccounts();
    }).catch(() => { if (wrap.isConnected) accountStatus.textContent = t('mail.requestFailed'); });
}
