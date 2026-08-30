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
import {makeCustomSelect} from '@renop/ui/custom-select';
import {apiRequest} from './api.js';
import {t} from './i18n.js';

/**
 * Create a lazily loaded global-team ownership field for package, domain, and transfer dialogs.
 * @param {object} [options={}] - Selector behavior.
 * @param {number} [options.minimumRole=3] - Minimum T-level returned by the server.
 * @param {boolean} [options.includePersonal=true] - Whether personal ownership is selectable.
 * @returns {{element: HTMLElement, ready: Promise<void>, value: function(): string}} Binding control.
 */
export function createSuperTeamBindingField({minimumRole = 3, includePersonal = true} = {}) {
    let selected = '';
    const host = el('div', {class: 'super-team-binding-select'},
        el('span', {class: 'super-team-binding-loading'}, t('common.loading')));
    const ready = (async () => {
        const boundedRole = Math.max(1, Math.min(4, Number(minimumRole) || 3));
        const response = await apiRequest(`/api/super-teams/eligible?minimum_role=${boundedRole}&limit=100&offset=0`);
        if (!response.ok) throw new Error('Failed to load eligible global teams');
        const payload = await response.json();
        const teams = Array.isArray(payload?.teams) ? payload.teams : [];
        const options = [{
            value: '', label: t(includePersonal ? 'superTeam.personalOwnership' : 'review.selectTeam')
        }];
        let teamCount = 0;
        for (const team of teams) {
            const prefix = String(team?.prefix || '');
            if (!prefix) continue;
            options.push({
                value: prefix,
                label: `${team.name || prefix} · ${prefix} · T${Number(team.role_level) || 3}`
            });
            teamCount++;
        }
        selected = '';
        if (!includePersonal && teamCount === 0) {
            host.replaceChildren(el('span', {class: 'error-text'}, t('review.noEligibleTeams')));
            return;
        }
        host.replaceChildren(makeCustomSelect(options, selected, value => {
            selected = String(value || '');
        }));
    })().catch(error => {
        console.error('Failed to load eligible global teams', error);
        host.replaceChildren(el('span', {class: 'error-text'}, t('superTeam.bindingLoadFailed')));
    });
    return {element: host, ready, value: () => selected};
}
