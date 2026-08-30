/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {createButton as createButtonBase} from '@renop/ui/button';
import {createIcon} from './icon.js';

/**
 * Create a styled button element with optional icon and click handler.
 * @param {string} text - Button label text.
 * @param {object} [props={}] - Button configuration.
 * @returns {HTMLElement}
 */
export function createButton(text, props = {}) {
    return createButtonBase(text, {createIcon, ...props});
}

/**
 * Disable a button for one asynchronous action and always restore it.
 * The element is captured before the first await because DOM event
 * currentTarget is cleared after event dispatch completes.
 * @template T
 * @param {HTMLButtonElement} button - Button that initiated the action.
 * @param {() => Promise<T>|T} action - Work to run while the button is disabled.
 * @returns {Promise<T|undefined>} Action result, or undefined for a duplicate invocation.
 */
export async function runButtonAction(button, action) {
    if (!button || typeof action !== 'function' || button.disabled) return undefined;
    button.disabled = true;
    try {
        return await action();
    } finally {
        button.disabled = false;
    }
}
