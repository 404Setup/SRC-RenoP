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

/** Render API documentation view while preserving the selected article in the URL. */
export async function renderAPI({root, params, soft = false}) {
    const slug = (params.splat || 'README').replace(/^\/+|\/+$/g, '');
    let content = root.querySelector('.api-content');
    if (!soft || !content) {
        root.replaceChildren(el('header', {class: 'page-hero'},
            el('h1', {'data-i18n': 'api.title'}, t('api.title')),
            el('p', {'data-i18n': 'api.lead'}, t('api.lead'))));
        content = el('div', {class: 'api-content'});
        root.append(content);
    }
    document.title = `${t('api.title')} — RenoP`;
    return renderDocs({root: content, params: {splat: `api/${slug}`}, soft, section: 'api'});
}
