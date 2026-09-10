/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {t} from '../i18n.js';
import {el} from '@renop/ui/dom';
import {renderDocs} from './docs.js';

/** Render either API documentation view while preserving the selected article in the URL. */
export async function renderAPI({root, params, soft = false}) {
    const openapi = new URLSearchParams(location.search).get('view') === 'openapi';
    const slug = (params.splat || 'README').replace(/^\/+|\/+$/g, '');
    let content = root.querySelector('.api-content');
    if (!soft || !content) {
        root.replaceChildren(el('header', {class: 'page-hero'},
            el('h1', {'data-i18n': 'api.title'}, t('api.title')),
            el('p', {'data-i18n': 'api.lead'}, t('api.lead'))));
        const modes = el('nav', {class: 'api-modes', 'aria-label': t('api.renderer')});
        modes.append(
            el('a', {class: 'pill-btn', 'data-link': '', 'data-api-view': 'renop'}, t('api.renop')),
            el('a', {class: 'pill-btn', 'data-link': '', 'data-api-view': 'openapi'}, t('api.openapi')),
            el('a', {class: 'pill-btn', href: '/assets/openapi.yaml', download: 'openapi.yaml'}, t('api.download')));
        content = el('div', {class: 'api-content'});
        root.append(modes, content);
    }
    for (const link of root.querySelectorAll('[data-api-view]')) {
        const selected = (link.dataset.apiView === 'openapi') === openapi;
        link.href = `/api/${slug}${link.dataset.apiView === 'openapi' ? '?view=openapi' : ''}`;
        link.classList.toggle('pill-btn--primary', selected);
        if (selected) link.setAttribute('aria-current', 'page');
        else link.removeAttribute('aria-current');
    }
    document.title = `${t('api.title')} — RenoP`;
    if (openapi) {
        const frame = el('iframe', {
            class: 'api-openapi', title: t('api.openapi'),
            src: '/assets/openapi-view.html',
        });
        content.replaceChildren(frame);
        return () => frame.remove();
    }
    return renderDocs({root: content, params: {splat: `api/${slug}`}, soft, section: 'api'});
}
