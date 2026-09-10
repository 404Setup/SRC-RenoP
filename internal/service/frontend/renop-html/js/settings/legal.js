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
import {createSection} from '../cfg-ui.js';
import {createIcon, createToggleRow} from '../components.js';
import {setSafeMarkdown} from '../markdown.js';
import {t} from '../i18n.js';
import {MAX_LEGAL_DOCUMENT_BYTES} from '../legal-response.js';

/** Render all legal documents with the same Markdown editor and safe preview. */
export function renderLegalSettings(container, data, changed) {
    const notice = createSection(createIcon('compliance'), t('legal.title'), t('legal.settingsHint'));
    notice.querySelector('.cfg-fields').appendChild(createToggleRow(t('legal.cookieBanner'), t('legal.cookieBannerHint'),
        data.cookie_banner === true, checked => { data.cookie_banner = checked; changed(); }));
    container.appendChild(notice);
    for (const [key, title] of [['privacy_policy', 'footer.privacyPolicy'], ['terms_of_service', 'legal.termsTitle'], ['legal_notice', 'footer.legalNotice']]) {
        const section = createSection(createIcon('compliance'), t(title), t('legal.documentHint'));
        const fields = section.querySelector('.cfg-fields');
        const input = el('textarea', {class: 'legal-editor', rows: 16, value: data[key] || '', 'aria-label': t(title), maxLength: MAX_LEGAL_DOCUMENT_BYTES});
        const preview = el('article', {class: 'legal-document markdown-body', hidden: true});
        let previewing = false;
        const toggle = el('button', {type: 'button', class: 'pill-btn pill-btn--soft', 'aria-expanded': 'false'}, t('legal.preview'));
        toggle.addEventListener('click', () => {
            if (!previewing && !input.reportValidity()) return;
            previewing = !previewing;
            if (previewing) setSafeMarkdown(preview, input.value);
            preview.hidden = !previewing;
            input.hidden = previewing;
            toggle.textContent = t(previewing ? 'legal.edit' : 'legal.preview');
            toggle.setAttribute('aria-expanded', String(previewing));
            if (!previewing) input.focus({preventScroll: true});
        });
        input.addEventListener('input', () => {
            const valid = new TextEncoder().encode(input.value).byteLength <= MAX_LEGAL_DOCUMENT_BYTES;
            input.setCustomValidity(valid ? '' : t('legal.invalid'));
            data[key] = input.value;
            changed();
        });
        fields.append(toggle, input, preview);
        container.appendChild(section);
    }
}
