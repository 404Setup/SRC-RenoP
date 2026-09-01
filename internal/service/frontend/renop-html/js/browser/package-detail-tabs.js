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
import {morphElementHeight} from '@renop/ui/height-anim';

/**
 * Build an accessible package-detail subpage switcher that mounts only the active section group.
 * @param {object} options - Detail view options.
 * @param {string} options.id - Stable per-format identifier.
 * @param {{id: string, label: string, content: Array<Node|null|undefined>}[]} options.tabs - Available subpages.
 * @param {string} options.active - Preferred active subpage.
 * @param {(id: string): void} options.onChange - Active-state observer.
 * @returns {HTMLElement|null} Tabbed detail layout or null when no content exists.
 */
export function createPackageDetailTabs({id, tabs, active, onChange}) {
    const available = tabs.map(tab => ({...tab, content: tab.content.filter(Boolean)}))
        .filter(tab => tab.content.length > 0);
    if (available.length === 0) return null;
    let current = available.some(tab => tab.id === active) ? active : available[0].id;
    const tablist = el('div', {class: 'package-detail-tablist', role: 'tablist'});
    const panel = el('div', {
        class: 'package-detail-panel', id: `${id}-panel`, role: 'tabpanel', tabindex: '0'
    });
    const buttons = [];

    /**
     * Mount one subpage and synchronize accessible tab state.
     * @param {string} next - Requested subpage identifier.
     * @param {boolean} [animate=true] - Whether to morph panel height.
     * @returns {Promise<void>} Render completion.
     */
    async function select(next, animate = true) {
        const tab = available.find(candidate => candidate.id === next) || available[0];
        current = tab.id;
        panel.setAttribute('aria-labelledby', `${id}-tab-${current}`);
        for (const entry of buttons) {
            const selected = entry.id === current;
            entry.button.classList.toggle('is-active', selected);
            entry.button.setAttribute('aria-selected', String(selected));
            entry.button.tabIndex = selected ? 0 : -1;
        }
        const replace = () => panel.replaceChildren(...tab.content);
        if (animate) await morphElementHeight(panel, replace, {duration: 260});
        else replace();
        if (typeof onChange === 'function') onChange(current);
    }

    for (const tab of available) {
        const button = el('button', {
            type: 'button', class: 'package-detail-tab', role: 'tab',
            id: `${id}-tab-${tab.id}`, 'aria-controls': panel.id
        }, tab.label);
        button.addEventListener('click', () => void select(tab.id));
        button.addEventListener('keydown', event => {
            const index = buttons.findIndex(entry => entry.id === current);
            let target = index;
            if (event.key === 'ArrowRight') target = (index + 1) % buttons.length;
            else if (event.key === 'ArrowLeft') target = (index - 1 + buttons.length) % buttons.length;
            else if (event.key === 'Home') target = 0;
            else if (event.key === 'End') target = buttons.length - 1;
            else return;
            event.preventDefault();
            buttons[target].button.focus();
            void select(buttons[target].id);
        });
        buttons.push({id: tab.id, button});
        tablist.appendChild(button);
    }
    void select(current, false);
    return el('div', {class: 'package-detail-tabs'}, tablist, panel);
}
