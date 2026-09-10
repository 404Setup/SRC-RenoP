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
import {optionalServicesAllowed, openCookiePreferences} from './cookie-consent.js';
import {readLegalTextResponse} from './legal-response.js';
import {LocalizedResponseError} from './response-errors.js';

const scopes = new Set(['password_login', 'registration', 'manual_mail', 'super_team_create', 'domain_create', 'package_create']);
const providers = new Set(['recaptcha_v2', 'recaptcha_invisible', 'recaptcha_v3', 'turnstile', 'hcaptcha', 'friendlycaptcha']);
let queue = Promise.resolve(), generation = 0, pending = 0, activeController;

/** Invalidate queued actions and remove active provider frames when their initiating context is left. */
function cancelChallenges() {
    generation++;
    activeController?.abort();
}
for (const event of ['popstate', 'pagehide', 'appNavigation', 'authChanged', 'captchaSettingsChanged']) window.addEventListener(event, cancelChallenges);
window.addEventListener('cookiePreferencesChanged', event => {
    if (!event.detail?.optional && activeController) cancelChallenges();
});

/** Obtain a single-use approval while keeping third-party scripts within a disposable frame. */
async function challenge(scope, signal, epoch) {
    if (epoch !== generation || signal?.aborted) throw new LocalizedResponseError(t('captcha.cancelled'));
    const metadataResponse = await fetch('/api/captcha', {credentials: 'include', cache: 'no-store', signal: AbortSignal.any([AbortSignal.timeout(15000), ...(signal ? [signal] : [])])});
    const metadata = JSON.parse(await readLegalTextResponse(metadataResponse, 'application/json', 4096));
    if (metadata.provider === 'disabled' || metadata.scopes?.[scope] !== true) return '';
    if (!providers.has(metadata.provider) || typeof metadata.site_key !== 'string') throw new LocalizedResponseError(t('captcha.unavailable'));
    if (epoch !== generation || signal?.aborted) throw new LocalizedResponseError(t('captcha.cancelled'));
    const controller = new AbortController();
    activeController = controller;
    const abort = () => controller.abort();
    signal?.addEventListener('abort', abort, {once: true});
    const previousFocus = document.activeElement;
    const dialog = el('dialog', {class: 'captcha-dialog', 'aria-labelledby': 'captcha-title'});
    const status = el('p', {role: 'status', 'aria-live': 'polite'}, t('captcha.loading'));
    const host = el('div', {class: 'captcha-widget-host'});
    const preferences = el('button', {type: 'button', class: 'pill-btn pill-btn--soft', onclick: () => void openCookiePreferences()}, t('legal.cookiePreferences'));
    const retry = el('button', {type: 'button', class: 'pill-btn pill-btn--soft', hidden: true}, t('offline.retryBtn'));
    const cancel = el('button', {type: 'button', class: 'pill-btn pill-btn--soft', onclick: abort}, t('common.cancel'));
    dialog.append(el('h2', {id: 'captcha-title'}, t('captcha.title')), status, host,
        el('div', {class: 'captcha-actions'}, preferences, retry, cancel));
    document.body.appendChild(dialog);
    dialog.showModal();
    let frame, nonce, frameTimer, verifying = false;
    try {
        return await new Promise((resolve, reject) => {
            let settled = false;
            const finish = (proof, error) => {
                if (settled) return;
                settled = true;
                clearTimeout(timer);
                clearTimeout(frameTimer);
                window.removeEventListener('message', onMessage);
                window.removeEventListener('cookiePreferencesChanged', onPreferences);
                error ? reject(error) : resolve(proof);
            };
            const failed = () => {
                if (settled) return;
                clearTimeout(frameTimer);
                frame?.remove(); frame = undefined; verifying = false;
                status.textContent = t('captcha.failed'); retry.hidden = false;
            };
            const render = async () => {
                try {
                    if (frame || verifying || settled) return;
                    if (!await optionalServicesAllowed()) { status.textContent = t('captcha.consent'); return; }
                    if (frame || verifying || settled || controller.signal.aborted) return;
                    retry.hidden = true;
                    status.textContent = t('captcha.complete');
                    nonce = Array.from(crypto.getRandomValues(new Uint8Array(16)), byte => byte.toString(16).padStart(2, '0')).join('');
                    frame = el('iframe', {class: 'captcha-frame', title: t('captcha.title'), src: '/api/captcha/widget',
                        sandbox: 'allow-scripts allow-same-origin allow-forms allow-popups', referrerPolicy: 'same-origin'});
                    const currentFrame = frame, currentNonce = nonce;
                    frameTimer = setTimeout(failed, 25000);
                    frame.addEventListener('load', () => {
                        if (frame !== currentFrame || controller.signal.aborted) return;
                        currentFrame.contentWindow?.postMessage({...metadata, kind: 'renop-captcha-init', nonce: currentNonce,
                            action: 'renop_' + scope, language: document.documentElement.lang,
                            theme: document.documentElement.classList.contains('dark') ? 'dark' : 'light'}, location.origin);
                    }, {once: true});
                    host.replaceChildren(frame);
                } catch { failed(); }
            };
            const onMessage = async event => {
                const message = event.data;
                if (settled || !frame || event.source !== frame.contentWindow || event.origin !== location.origin || message?.nonce !== nonce) return;
                clearTimeout(frameTimer);
                if (message.kind === 'renop-captcha-size') {
                    if (Number.isFinite(message.height) && message.height >= 100 && message.height <= 900) frame.style.height = message.height + 'px';
                } else if (message.kind === 'renop-captcha-error' || message.kind === 'renop-captcha-expired') failed();
                else if (message.kind === 'renop-captcha-complete' && !verifying && typeof message.response === 'string' && message.response.length <= 16384) {
                    verifying = true;
                    status.textContent = t('captcha.verifying');
                    frame.remove(); frame = undefined;
                    try {
                        const response = await fetch('/api/captcha/verify', {method: 'POST', credentials: 'include', cache: 'no-store',
                            headers: {'Content-Type': 'application/json'}, signal: AbortSignal.any([controller.signal, AbortSignal.timeout(15000)]),
                            body: JSON.stringify({scope, provider: metadata.provider, site_key: metadata.site_key, response: message.response})});
                        if (!response.ok) {
                            if (response.status === 409) finish('', new LocalizedResponseError(t('captcha.changed')));
                            else failed();
                            return;
                        }
                        const result = JSON.parse(await readLegalTextResponse(response, 'application/json', 4096));
                        if (typeof result.proof !== 'string' || result.proof.length > 128 || !result.proof) { failed(); return; }
                        if (epoch !== generation || controller.signal.aborted || !await optionalServicesAllowed()) { abort(); return; }
                        finish(result.proof);
                    } catch { if (!controller.signal.aborted) failed(); }
                }
            };
            const onPreferences = event => { if (event.detail?.optional) void render(); };
            const timer = setTimeout(() => finish('', new LocalizedResponseError(t('captcha.failed'))), 300000);
            controller.signal.addEventListener('abort', () => finish('', new LocalizedResponseError(t('captcha.cancelled'))), {once: true});
            dialog.addEventListener('cancel', event => { event.preventDefault(); abort(); });
            retry.addEventListener('click', () => void render());
            window.addEventListener('message', onMessage);
            window.addEventListener('cookiePreferencesChanged', onPreferences);
            void render();
        });
    } finally {
        controller.abort();
        frame?.remove();
        dialog.close(); dialog.remove();
        signal?.removeEventListener('abort', abort);
        if (activeController === controller) activeController = undefined;
        if (previousFocus?.isConnected) previousFocus.focus({preventScroll: true});
    }
}

/** Return retry headers only for a server denial emitted before its mutation begins. */
export async function captchaHeaders(response, url, signal) {
    if (response.status !== 428 || response.headers.get('X-Renop-Error-Code') !== 'captcha_required') return null;
    if (new URL(url, location.href).origin !== location.origin) return null;
    const scope = response.headers.get('X-Renop-Captcha-Scope');
    if (!scopes.has(scope)) throw new LocalizedResponseError(t('captcha.unavailable'));
    if (pending >= 8) throw new LocalizedResponseError(t('captcha.unavailable'));
    const epoch = generation;
    pending++;
    const task = queue.then(() => challenge(scope, signal, epoch)).finally(() => { pending--; });
    queue = task.catch(() => {});
    const proof = await task;
    if (epoch !== generation || signal?.aborted) throw new LocalizedResponseError(t('captcha.cancelled'));
    return proof ? {'X-Renop-Captcha': proof} : {};
}

/** Retry exactly once after CAPTCHA's explicit pre-mutation denial; other responses are never replayed. */
export async function captchaFetch(url, options = {}) {
    const response = await fetch(url, options);
    if (typeof ReadableStream !== 'undefined' && options.body instanceof ReadableStream) return response;
    let approval;
    try { approval = await captchaHeaders(response, url, options.signal); } catch (error) {
        await response.body?.cancel().catch(() => {});
        throw error;
    }
    if (approval === null) return response;
    await response.body?.cancel().catch(() => {});
    const headers = new Headers(options.headers);
    for (const [key, value] of Object.entries(approval)) headers.set(key, value);
    return fetch(url, {...options, headers});
}
