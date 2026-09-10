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
import {t} from './i18n.js';
import {readLegalTextResponse} from './legal-response.js';

export const LEGAL_COOKIE = 'renop_legal_consent';
export const LEGAL_DOCUMENTS = Object.freeze({
    'privacy-policy': 'footer.privacyPolicy',
    'terms-of-service': 'legal.termsTitle',
    'legal-notice': 'footer.legalNotice',
});
let metadata, pending;
let consentEpoch = 0;

/** Return the public policy page selected by a pathname. */
export function legalPageFromPath(path = window.location.pathname) {
    const name = path.toLowerCase().replace(/^\//, '').replace(/\/$/, '');
    return Object.hasOwn(LEGAL_DOCUMENTS, name) ? name : '';
}

/** Read the browser's explicit acceptance without storing account identifiers. */
function acceptedRevision() {
    return document.cookie.split(';').map(part => part.trim()).find(part => part.startsWith(LEGAL_COOKIE + '='))?.slice(LEGAL_COOKIE.length + 1) || '';
}

/** Align consent controls with the live policy revision and browser choice. */
function updateConsentControls() {
    for (const input of document.querySelectorAll('[data-legal-consent]')) {
        input.disabled = !metadata;
        if (metadata) {
            if (input.dataset.revision !== metadata.revision || acceptedRevision() !== metadata.revision) input.checked = false;
            input.dataset.revision = metadata.revision;
        }
    }
}

/** Require a new explicit choice when an account-entry page is left. */
export function resetLegalConsent(kind) {
    const input = document.getElementById(kind + '-legal-consent');
    if (input?.checked) {
        input.checked = false;
        consentEpoch++;
    }
}

/** Fetch the small public policy descriptor, coalescing concurrent reads. */
export async function loadLegalMetadata(refresh = false) {
    if (metadata && !refresh) return metadata;
    if (!pending) {
        pending = fetch('/api/legal', {credentials: 'omit', cache: 'no-store', signal: AbortSignal.timeout(15000)})
            .then(response => readLegalTextResponse(response, 'application/json', 4096))
            .then(text => {
                const value = JSON.parse(text);
                if (!/^[a-f0-9]{64}$/.test(value?.revision) || typeof value.cookie_banner !== 'boolean') throw new Error('invalid legal metadata');
                const changed = !metadata || metadata.revision !== value.revision || metadata.cookie_banner !== value.cookie_banner;
                metadata = {revision: value.revision, cookie_banner: value.cookie_banner};
                updateConsentControls();
                if (changed) window.dispatchEvent(new CustomEvent('legalConfigurationChanged', {detail: metadata}));
                return metadata;
            }).finally(() => { pending = null; });
    }
    return pending;
}

/** Show a localized account-entry error beside the agreement control. */
function consentError(kind, key) {
    const target = document.getElementById(kind + '-legal-error');
    if (!target) return;
    target.textContent = t(key);
    target.dataset.i18n = key;
    target.hidden = false;
}

/** Require an explicit checkbox choice for the current privacy policy and terms. */
export async function ensureLegalConsent(kind = 'login') {
    const epoch = consentEpoch;
    try {
        await loadLegalMetadata(true);
    } catch {
        consentError(kind, 'legal.loadFailed');
        return false;
    }
    if (epoch !== consentEpoch) return false;
    const input = document.getElementById(kind + '-legal-consent');
    if (!input?.checked || input.dataset.revision !== metadata.revision || acceptedRevision() !== metadata.revision) {
        consentError(kind, 'legal.consentRequired');
        input?.reportValidity();
        input?.focus({preventScroll: true});
        return false;
    }
    document.getElementById(kind + '-legal-error').hidden = true;
    return true;
}

/** Install shared consent controls for password, Passkey, provider, and registration flows. */
export function initializeLegalConsent() {
    for (const kind of ['login', 'registration']) {
        const slot = document.getElementById(kind + '-legal-consent-slot');
        if (!slot) continue;
        const input = el('input', {type: 'checkbox', id: kind + '-legal-consent', required: true, disabled: true, 'data-legal-consent': ''});
        input.addEventListener('change', () => {
            consentEpoch++;
            if (!metadata || input.dataset.revision !== metadata.revision) return;
            document.cookie = LEGAL_COOKIE + '=' + (input.checked ? metadata.revision : '') +
                '; Path=/; SameSite=Lax; Max-Age=' + (input.checked ? 900 : 0) +
                (window.location.protocol === 'https:' ? '; Secure' : '');
            if (input.checked && acceptedRevision() !== metadata.revision) consentError(kind, 'legal.cookiesRequired');
            else document.getElementById(kind + '-legal-error').hidden = true;
        });
        const links = el('div', {class: 'legal-consent-links'});
        for (const name of ['privacy-policy', 'terms-of-service']) {
            links.appendChild(el('a', {href: '/' + name, target: '_blank', rel: 'noopener', 'data-i18n': LEGAL_DOCUMENTS[name]}, t(LEGAL_DOCUMENTS[name])));
        }
        slot.replaceChildren(
            el('label', {class: 'legal-consent'}, input, el('span', {'data-i18n': 'legal.consentLabel'}, t('legal.consentLabel'))),
            links, el('p', {id: kind + '-legal-error', class: 'account-form-error', role: 'alert', hidden: true}),
        );
    }
    void loadLegalMetadata().catch(() => {
        for (const kind of ['login', 'registration']) consentError(kind, 'legal.loadFailed');
    });
    window.addEventListener('legalSettingsChanged', () => { void loadLegalMetadata(true).catch(() => {}); });
}
