/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {el} from './dom.js';
import {$} from './jquery.js';

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
 * @param {Node} [props.iconNode] - SVG Icon element node.
 * @param {Function} [props.createIcon] - Icon resolver function if `props.icon` name string is given.
 * @param {string} [props.icon] - Icon name string.
 * @param {object} [props.iconProps] - Props for icon resolver.
 * @param {Function} [props.onClick] - Click handler.
 * @returns {HTMLElement}
 */
export function createButton(text, props = {}) {
    const btnProps = {
        type: props.type || 'button',
        class: props.class || 'pill-btn pill-btn--primary',
    };
    if (props.id) btnProps.id = props.id;
    if (props.title) btnProps.title = props.title;
    if (props.disabled) btnProps.disabled = true;
    if (props.style) btnProps.style = props.style;

    const children = [];
    let hasIcon = false;
    if (props.iconNode) {
        children.push(props.iconNode);
        hasIcon = true;
    } else if (props.icon && typeof props.createIcon === 'function') {
        children.push(props.createIcon(props.icon, props.iconProps || {}));
        hasIcon = true;
    }

    if (text) {
        children.push(document.createTextNode((hasIcon ? ' ' : '') + text));
    }

    const btn = el('button', btnProps, ...children);
    if (props.onClick) $(btn).on('click', props.onClick);
    return btn;
}
