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
import {createIcon} from '../components/icon.js';
import {writeClipboardText} from '../clipboard.js';

const activeCopyFeedback = new WeakMap();

/**
 * Copy text and apply the shared repository copy-button success feedback.
 * @param {HTMLButtonElement} button - Copy button receiving the temporary state.
 * @param {string} text - Non-empty clipboard value.
 * @param {{copiedLabel: string, duration?: number}} options - Localized label and feedback duration.
 * @returns {Promise<void>}
 */
export async function copyWithFeedback(button, text, {copiedLabel, duration = 2000}) {
    if (!(button instanceof HTMLButtonElement) || !text) return;
    await writeClipboardText(text);
    const current = activeCopyFeedback.get(button);
    if (current) clearTimeout(current.timer);
    const originalTitle = current?.originalTitle ?? button.title;
    const originalChildren = current?.originalChildren ?? Array.from(button.childNodes);
    button.classList.add('copied');
    button.title = copiedLabel;
    button.replaceChildren(
        createIcon('check', {class: 'icon-svg'}),
        el('span', {class: 'copy-toast'}, copiedLabel)
    );
    const timer = setTimeout(() => {
        activeCopyFeedback.delete(button);
        if (!button.isConnected) return;
        button.classList.remove('copied');
        button.title = originalTitle;
        button.replaceChildren(...originalChildren);
    }, duration);
    activeCopyFeedback.set(button, {originalChildren, originalTitle, timer});
}
