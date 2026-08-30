/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

import {el} from '@renop/ui/dom';
import {makeCustomSelect} from '@renop/ui/custom-select';
import {apiRequest} from './api.js';
import {t} from './i18n.js';

/**
 * Create a lazily loaded T3+ global-team ownership field for package and domain dialogs.
 * @returns {{element: HTMLElement, ready: Promise<void>, value: function(): string}} Binding control.
 */
export function createSuperTeamBindingField() {
    let selected = '';
    const host = el('div', {class: 'super-team-binding-select'},
        el('span', {class: 'super-team-binding-loading'}, t('common.loading')));
    const ready = (async () => {
        const response = await apiRequest('/api/super-teams/eligible?minimum_role=3&limit=100&offset=0');
        if (!response.ok) throw new Error('Failed to load eligible global teams');
        const payload = await response.json();
        const teams = Array.isArray(payload?.teams) ? payload.teams : [];
        const options = [{value: '', label: t('superTeam.personalOwnership')}];
        for (const team of teams) {
            const prefix = String(team?.prefix || '');
            if (!prefix) continue;
            options.push({value: prefix, label: `${team.name || prefix} · ${prefix} · T${Number(team.role_level) || 3}`});
        }
        host.replaceChildren(makeCustomSelect(options, '', value => {
            selected = String(value || '');
        }));
    })().catch(error => {
        console.error('Failed to load eligible global teams', error);
        host.replaceChildren(el('span', {class: 'error-text'}, t('superTeam.bindingLoadFailed')));
    });
    return {element: host, ready, value: () => selected};
}
