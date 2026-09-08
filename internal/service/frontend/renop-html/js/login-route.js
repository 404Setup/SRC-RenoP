/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 * If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

/** Return the public account page represented by a pathname. */
export function accountPageFromPath(pathname = window.location.pathname) {
    const path = pathname.toLowerCase().replace(/\/$/, '');
    if (path === '/account/login') return 'login';
    if (path === '/account/register') return 'registration';
    if (path === '/account/recovery') return 'recovery';
    if (path === '/account/forgot-password') return 'password-recovery';
    return '';
}

/** Whether a pathname belongs to the sign-in page. */
export function isLoginPath(pathname = window.location.pathname) {
    return accountPageFromPath(pathname) === 'login';
}

/** Keep login return paths local, bounded, and outside authentication endpoints. */
export function safeLoginReturnTo(value) {
    if (typeof value !== 'string' || value.length > 1024 || !value.startsWith('/') ||
        value.startsWith('//') || /[\\\x00-\x20]/.test(value)) return '/';
    try {
        const target = new URL(value, 'https://renop.invalid');
        const decoded = decodeURIComponent(target.pathname).toLowerCase();
        if (target.origin !== 'https://renop.invalid' || decoded.startsWith('//') ||
            /[\\\x00-\x20]/.test(decoded) || accountPageFromPath(decoded) ||
            decoded === '/api' || decoded.startsWith('/api/') || target.pathname.length > 1024) return '/';
        return target.pathname;
    } catch {
        return '/';
    }
}

/** Read the destination shared by password, Passkey, and provider sign-in. */
export function loginReturnTo(search = window.location.search) {
    return safeLoginReturnTo(new URLSearchParams(search).get('return_to'));
}

/** Open sign-in while retaining the page to return to after authentication. */
export function navigateToLogin(returnTo = window.location.pathname, {replace = false, reauth = false} = {}) {
    if (isLoginPath()) return;
    const target = safeLoginReturnTo(returnTo);
    const query = new URLSearchParams();
    if (target !== '/') query.set('return_to', target);
    if (reauth) query.set('reauth', '1');
    const route = '/account/login' + (query.size ? '?' + query : '');
    if (replace) window.history.replaceState(null, '', route);
    else window.history.pushState(null, '', route);
    window.dispatchEvent(new PopStateEvent('popstate'));
}

/** Replace the completed sign-in entry so Back does not reopen the form. */
export function leaveLoginPage() {
    if (!isLoginPath()) return;
    window.history.replaceState(null, '', loginReturnTo());
    window.dispatchEvent(new PopStateEvent('popstate'));
}
