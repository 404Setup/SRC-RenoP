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
import {createBadge} from './badge.js';
import {createCallout} from './callout.js';

/**
 * Index / feature card host custom element.
 */
export class RenopCard extends HTMLElement {
    /**
     * Ensure the card base class is applied when connected.
     * @returns {void}
     */
    connectedCallback() {
        if (!this.classList.contains('cfg-index-card')) {
            this.classList.add('cfg-index-card');
        }
    }
}

if (!customElements.get('renop-card')) {
    customElements.define('renop-card', RenopCard);
}

/**
 * Create an index card with icon, badge, description, optional callout, and action button.
 * @param {object} options - Card configuration.
 * @param {string} options.badgeText - Header badge label.
 * @param {string} [options.badgeType='success'] - Badge tone.
 * @param {string} [options.iconName='refresh'] - Icon name for header and button.
 * @param {string} [options.iconVariant='success'] - Icon container variant class.
 * @param {string} options.title - Card title.
 * @param {string} options.desc - Card description.
 * @param {{type?: string, text: string, icon?: string}} [options.callout] - Optional callout config.
 * @param {string} [options.buttonId] - Action button id.
 * @param {string} options.buttonText - Action button label.
 * @param {string} [options.buttonVariant='primary'] - Action button variant.
 * @param {string} [options.buttonTitle] - Action button title tooltip.
 * @param {Function} [options.onButtonClick] - Action button click handler.
 * @returns {HTMLElement}
 */
export function createIndexCard({
                                    badgeText,
                                    badgeType = 'success',
                                    iconName = 'refresh',
                                    iconVariant = 'success',
                                    title,
                                    desc,
                                    callout,
                                    buttonId,
                                    buttonText,
                                    buttonVariant = 'primary',
                                    buttonTitle,
                                    onButtonClick
                                }) {
    const card = document.createElement('renop-card');
    card.className = 'cfg-index-card';

    const header = el('div', {class: 'cfg-index-card-header'},
        el('div', {class: `cfg-index-card-icon cfg-index-card-icon--${iconVariant}`}, createIcon(iconName)),
        createBadge(badgeText, badgeType, {class: `cfg-badge cfg-badge--${badgeType}`})
    );

    const titleEl = el('h3', {class: 'cfg-index-card-title'}, title);
    const descEl = el('p', {class: 'cfg-index-card-desc'}, desc);

    card.appendChild(header);
    card.appendChild(titleEl);
    card.appendChild(descEl);

    if (callout) {
        const calloutEl = createCallout(callout.type || 'danger', callout.text, callout.icon || 'warning');
        calloutEl.style.marginBottom = '1rem';
        card.appendChild(calloutEl);
    }

    const btn = el('button', {
        id: buttonId,
        class: `cfg-index-action-btn cfg-index-action-btn--${buttonVariant}`
    }, createIcon(iconName), document.createTextNode(' ' + buttonText));

    if (buttonTitle) btn.title = buttonTitle;
    if (onButtonClick) btn.onclick = onButtonClick;

    card.appendChild(btn);
    return card;
}
