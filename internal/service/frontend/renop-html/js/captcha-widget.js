/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

let started = false;

/** Run one provider inside its disposable frame without installing provider code in the application shell. */
window.addEventListener('message', async event => {
    if (started || parent === window || event.source !== parent || event.origin !== location.origin || event.data?.kind !== 'renop-captcha-init') return;
    const options = event.data;
    if (typeof options.nonce !== 'string' || options.nonce.length > 64 || typeof options.site_key !== 'string' || options.site_key.length > 256) return;
    started = true;
    const send = (kind, extra = {}) => parent.postMessage({kind, nonce: options.nonce, ...extra}, location.origin);
    const complete = response => {
        if (typeof response === 'string' && response.length > 0 && response.length <= 16384) send('renop-captcha-complete', {response});
        else send('renop-captcha-error');
    };
    const failed = () => send('renop-captcha-error');
    const expired = () => send('renop-captcha-expired');
    document.documentElement.lang = options.language || 'en';
    document.body.style.cssText = 'margin:0;padding:8px;box-sizing:border-box;min-width:0;background:transparent';
    const widget = document.getElementById('widget');
    const size = window.innerWidth < 340 ? 'compact' : 'normal';
    const language = options.language?.startsWith('zh-') ? options.language.replace('zh-YUE', 'zh-HK')
        : options.language === 'pt-PT' ? 'pt-PT' : String(options.language || 'en').split('-')[0];
    let lastHeight = 0;
    const observer = new MutationObserver(() => {
        let height = 110;
        for (const child of document.querySelectorAll('iframe')) {
            const rectangle = child.getBoundingClientRect();
            if (rectangle.width && getComputedStyle(child).visibility !== 'hidden') height = Math.max(height, rectangle.height + 32);
        }
        height = Math.min(900, Math.ceil(height));
        if (height !== lastHeight) { lastHeight = height; send('renop-captcha-size', {height}); }
    });
    observer.observe(document.body, {childList: true, subtree: true, attributes: true, attributeFilter: ['style', 'height', 'class']});

    /** Load a pinned provider entry point and wait for its supported readiness callback. */
    const load = (src, {module = false, callback = false} = {}) => new Promise((resolve, reject) => {
        const script = document.createElement('script');
        const timer = setTimeout(() => reject(new Error('Provider load timed out')), 20000);
        const done = () => { clearTimeout(timer); resolve(); };
        if (callback) window.renopCaptchaReady = done;
        else script.onload = done;
        script.onerror = () => { clearTimeout(timer); reject(new Error('Provider load failed')); };
        if (module) script.type = 'module';
        script.src = src;
        document.head.appendChild(script);
    });
    try {
        if (options.provider === 'friendlycaptcha') {
            widget.className = 'frc-captcha';
            Object.assign(widget.dataset, {sitekey: options.site_key, start: 'auto', theme: options.theme, apiEndpoint: options.friendly_region || 'global'});
            widget.addEventListener('frc:widget.complete', event => complete(event.detail?.response));
            widget.addEventListener('frc:widget.error', failed);
            widget.addEventListener('frc:widget.expire', expired);
            await load('https://cdn.jsdelivr.net/npm/@friendlycaptcha/sdk@1.1.1/site.min.js', {module: true});
        } else if (options.provider === 'turnstile') {
            await load('https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit&onload=renopCaptchaReady', {callback: true});
            window.turnstile.render(widget, {sitekey: options.site_key, action: options.action, theme: options.theme, size,
                language, callback: complete, 'error-callback': failed, 'expired-callback': expired});
        } else if (options.provider === 'hcaptcha') {
            await load('https://js.hcaptcha.com/1/api.js?render=explicit&recaptchacompat=off&onload=renopCaptchaReady&hl=' + encodeURIComponent(language), {callback: true});
            window.hcaptcha.render(widget, {sitekey: options.site_key, theme: options.theme, size,
                callback: complete, 'error-callback': failed, 'expired-callback': expired});
        } else if (['recaptcha_v2', 'recaptcha_invisible', 'recaptcha_v3'].includes(options.provider)) {
            const render = options.provider === 'recaptcha_v3' ? options.site_key : 'explicit';
            await load('https://www.google.com/recaptcha/api.js?' + new URLSearchParams({render, hl: language, onload: 'renopCaptchaReady'}), {callback: true});
            if (options.provider === 'recaptcha_v3') {
                window.grecaptcha.ready(() => window.grecaptcha.execute(options.site_key, {action: options.action}).then(complete, failed));
            } else {
                const id = window.grecaptcha.render(widget, {sitekey: options.site_key, theme: options.theme,
                    size: options.provider === 'recaptcha_invisible' ? 'invisible' : size, badge: 'inline',
                    callback: complete, 'error-callback': failed, 'expired-callback': expired});
                if (options.provider === 'recaptcha_invisible') window.grecaptcha.execute(id);
            }
        } else failed();
    } catch { failed(); }
});
