/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

const jqueryModule = await import('jquery');
const jquery = jqueryModule.default || jqueryModule.jQuery || jqueryModule.$;
if (typeof jquery !== 'function' || !String(jquery.fn?.jquery || '').startsWith('4.')) {
    throw new Error('RenoP requires jQuery 4');
}

const installedTargets = new WeakSet();

/** The shared jQuery 4 function. */
export const jQuery = jquery;

/** The shared jQuery 4 shorthand. */
export const $ = jquery;

/**
 * Install jQuery 4 on one browser global without silently replacing another `$` library.
 * @param {Window} target - Browser window that receives the globals.
 * @returns {Function} The installed jQuery function.
 */
export function installJQueryGlobals(target = window) {
    if (installedTargets.has(target)) return jquery;
    const existing = target.jQuery;
    if (existing && existing !== jquery) {
        throw new Error('A different jQuery runtime is already installed');
    }
    target.jQuery = jquery;
    if (target.$ == null || target.$ === existing) target.$ = jquery;
    installedTargets.add(target);
    if (typeof target.dispatchEvent === 'function' && typeof target.CustomEvent === 'function') {
        target.dispatchEvent(new target.CustomEvent('jqueryReady', {
            detail: {version: jquery.fn.jquery},
        }));
    }
    return jquery;
}

/**
 * Resolve the already loaded shared runtime for compatibility with asynchronous callers.
 * @param {Window} target - Browser window that receives the globals.
 * @returns {Promise<Function>} The installed jQuery function.
 */
export function loadJQuery(target = window) {
    return Promise.resolve(installJQueryGlobals(target));
}

installJQueryGlobals(window);
