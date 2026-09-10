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
import {loadLegalMetadata} from './legal-consent.js';
import {parseCookiePreferences} from './cookie-preferences.js';

const storageKey = 'renop_cookie_preferences';
let metadata, choice, banner;

/** Read preferences without treating blocked browser storage as consent. */
function readChoice() {
    if (!metadata) return null;
    const current = parseCookiePreferences(JSON.stringify(choice), metadata.revision);
    if (current) return current;
    try {
        return parseCookiePreferences(localStorage.getItem(storageKey), metadata.revision);
    } catch {
        return null;
    }
}

/** Whether this browser explicitly permits the configured third-party verification service. */
export async function optionalServicesAllowed() {
    metadata = await loadLegalMetadata();
    return readChoice()?.optional === true;
}

/** Apply the selected categories and notify integrations before closing the notice. */
function saveChoice(optional) {
    choice = {revision: metadata.revision, optional, expires: Date.now() + 31536000000};
    try { localStorage.setItem(storageKey, JSON.stringify(choice)); } catch {}
    renderNotice();
    window.dispatchEvent(new CustomEvent('cookiePreferencesChanged', {detail: {optional}}));
}

/** Build a translated text node without replacing interactive children on language changes. */
function text(tag, key, attributes = {}) {
    return el(tag, {...attributes, 'data-i18n': key}, t(key));
}

/** Show an accessible category dialog and return focus to its opener on close. */
export async function openCookiePreferences() {
    if (document.getElementById('cookie-preferences-dialog')) return;
    try { metadata = await loadLegalMetadata(true); } catch {
        if (banner) {
            banner.hidden = false;
            banner.replaceChildren(text('p', 'legal.loadFailed', {role: 'alert'}),
                text('button', 'offline.retryBtn', {type: 'button', class: 'pill-btn pill-btn--soft', onclick: () => void openCookiePreferences()}));
        }
        return;
    }
    const policyRevision = metadata.revision;
    const previousFocus = document.activeElement;
    const optional = el('input', {type: 'checkbox', checked: readChoice()?.optional === true});
    const dialog = el('dialog', {id: 'cookie-preferences-dialog', class: 'cookie-preferences', 'aria-labelledby': 'cookie-preferences-title'});
    const close = () => dialog.close();
    const save = value => {
        if (policyRevision === metadata.revision) saveChoice(value);
        close();
    };
    dialog.append(
        text('h2', 'legal.cookiePreferences', {id: 'cookie-preferences-title'}),
        text('p', 'legal.cookieDescription'),
        el('label', {class: 'cookie-category'}, el('input', {type: 'checkbox', checked: true, disabled: true}),
            el('span', {}, text('strong', 'legal.necessary'), text('span', 'legal.necessaryDescription'))),
        el('label', {class: 'cookie-category'}, optional,
            el('span', {}, text('strong', 'legal.optional'), text('span', 'legal.optionalDescription'))),
        el('a', {href: '/privacy-policy', target: '_blank', rel: 'noopener'}, t('footer.privacyPolicy')),
        el('div', {class: 'cookie-actions'},
            text('button', 'legal.necessaryOnly', {type: 'button', class: 'pill-btn pill-btn--soft', onclick: () => save(false)}),
            text('button', 'legal.acceptAll', {type: 'button', class: 'pill-btn pill-btn--soft', onclick: () => save(true)}),
            text('button', 'legal.savePreferences', {type: 'button', class: 'pill-btn pill-btn--soft', onclick: () => save(optional.checked)}),
            text('button', 'modal.close', {type: 'button', class: 'pill-btn pill-btn--soft', onclick: close})),
    );
    dialog.addEventListener('close', () => {
        dialog.remove();
        const target = previousFocus?.isConnected ? previousFocus : document.querySelector('[data-cookie-preferences]');
        target?.focus({preventScroll: true});
    }, {once: true});
    document.body.appendChild(dialog);
    dialog.showModal();
}

/** Render the floating notice only while a category choice is still needed. */
function renderNotice() {
    if (!banner) return;
    banner.hidden = !metadata?.cookie_banner || Boolean(readChoice());
    if (banner.hidden) return;
    banner.replaceChildren(text('h2', 'legal.cookieTitle'), text('p', 'legal.cookieDescription'),
        el('div', {class: 'cookie-actions'},
            text('button', 'legal.necessaryOnly', {type: 'button', class: 'pill-btn pill-btn--soft', onclick: () => saveChoice(false)}),
            text('button', 'legal.acceptAll', {type: 'button', class: 'pill-btn pill-btn--soft', onclick: () => saveChoice(true)}),
            text('button', 'legal.cookiePreferences', {type: 'button', class: 'pill-btn pill-btn--soft', onclick: () => void openCookiePreferences()})),
        el('a', {href: '/privacy-policy', 'data-legal-link': '', 'data-i18n': 'footer.privacyPolicy'}, t('footer.privacyPolicy')));
}

/** Initialize persistent categories and synchronize choices across browser tabs. */
export function initializeCookieConsent() {
    if (banner) return;
    banner = el('aside', {class: 'cookie-banner', id: 'cookie-banner', role: 'region', 'aria-label': t('legal.cookieTitle'), 'data-i18n-aria-label': 'legal.cookieTitle', hidden: true});
    document.body.appendChild(banner);
    document.addEventListener('click', event => {
        if (event.target.closest?.('[data-cookie-preferences]')) void openCookiePreferences();
    });
    window.addEventListener('legalConfigurationChanged', event => {
        metadata = event.detail;
        renderNotice();
        window.dispatchEvent(new CustomEvent('cookiePreferencesChanged', {detail: {optional: readChoice()?.optional === true}}));
    });
    window.addEventListener('storage', event => {
        if (event.key !== storageKey && event.key !== null) return;
        choice = undefined;
        renderNotice();
        window.dispatchEvent(new CustomEvent('cookiePreferencesChanged', {detail: {optional: readChoice()?.optional === true}}));
    });
    void loadLegalMetadata().then(value => { metadata = value; renderNotice(); }).catch(() => {});
}
