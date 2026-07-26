/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {el} from '../cfg-ui.js';
import {createIcon} from './icon.js';

/**
 * Create a styled button element with optional icon and click handler.
 * @param {string} text - Button label text.
 * @param {object} [props={}] - Button configuration.
 * @param {string} [props.type='button'] - Native button type.
 * @param {string} [props.class] - CSS class string.
 * @param {string} [props.id] - Element id.
 * @param {string} [props.title] - Title tooltip.
 * @param {boolean} [props.disabled] - Disabled state.
 * @param {string|object} [props.style] - Inline styles.
 * @param {string} [props.icon] - Icon name from ICONS.
 * @param {object} [props.iconProps] - Props forwarded to createIcon.
 * @param {Function} [props.onClick] - Click handler.
 * @returns {HTMLElement}
 */
export function createButton(text, props = {}) {
    const btnProps = {
        type: props.type || 'button',
        class: props.class || 'pill-btn pill-btn--primary'
    };
    if (props.id) btnProps.id = props.id;
    if (props.title) btnProps.title = props.title;
    if (props.disabled) btnProps.disabled = true;
    if (props.style) btnProps.style = props.style;

    const children = [];
    if (props.icon) {
        children.push(createIcon(props.icon, props.iconProps || {}));
    }
    if (text) {
        children.push(document.createTextNode((props.icon ? ' ' : '') + text));
    }

    const btn = el('button', btnProps, ...children);
    if (props.onClick) btn.onclick = props.onClick;
    return btn;
}
