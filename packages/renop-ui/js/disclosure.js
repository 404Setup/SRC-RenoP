/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {collapseElement, expandElement} from './height-anim.js';
import {$} from './jquery.js';

const animatedDisclosures = new WeakSet();

/**
 * Bind a details/summary disclosure to reversible height and opacity transitions.
 * @param {HTMLDetailsElement} details - Details host.
 * @param {object} [options={}] - Disclosure options.
 * @param {HTMLElement} options.content - Single content wrapper below the summary.
 * @param {number} [options.expandDuration=240] - Expand duration in milliseconds.
 * @param {number} [options.collapseDuration=210] - Collapse duration in milliseconds.
 * @param {string} [options.marginTop=''] - Expanded top margin animated with the content.
 * @returns {{destroy: () => void}|null} Binding controller.
 */
export function bindAnimatedDetails(details, {
    content,
    expandDuration = 240,
    collapseDuration = 210,
    marginTop = '',
} = {}) {
    if (!(details instanceof HTMLDetailsElement) || !(content instanceof HTMLElement) ||
        animatedDisclosures.has(details)) {
        return null;
    }
    const summary = $(details).children('summary').get(0);
    if (!(summary instanceof HTMLElement)) return null;
    animatedDisclosures.add(details);
    let desiredOpen = details.open;
    let operation = 0;
    $(summary).attr('aria-expanded', String(desiredOpen));
    if (desiredOpen) {
        content.hidden = false;
        $(content).addClass('is-visible');
    } else {
        content.hidden = true;
    }

    const handleToggle = event => {
        event.preventDefault();
        desiredOpen = !desiredOpen;
        const currentOperation = ++operation;
        $(summary).attr('aria-expanded', String(desiredOpen));
        if (desiredOpen) {
            details.open = true;
            void expandElement(content, {
                duration: expandDuration,
                marginTop,
            });
            return;
        }
        void collapseElement(content, {
            duration: collapseDuration,
            marginTop: Boolean(marginTop),
        }).then(() => {
            if (currentOperation === operation && !desiredOpen) details.open = false;
        });
    };
    $(summary).on('click.renopDisclosure', handleToggle);
    return {
        destroy() {
            operation += 1;
            $(summary).off('.renopDisclosure');
            animatedDisclosures.delete(details);
        },
    };
}
