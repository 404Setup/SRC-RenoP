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
    const wrap = el('div', {class: 'cfg-layout'});
    const notice = createSection(createIcon('compliance'), t('legal.title'), t('legal.settingsHint'), {defaultCollapsed: true});
    notice.querySelector('.cfg-fields').appendChild(createToggleRow(t('legal.cookieBanner'), t('legal.cookieBannerHint'),
        data.cookie_banner === true, checked => { data.cookie_banner = checked; changed(); }));
    wrap.appendChild(notice);

    for (const [key, title] of [['privacy_policy', 'footer.privacyPolicy'], ['terms_of_service', 'legal.termsTitle'], ['legal_notice', 'footer.legalNotice']]) {
        const section = createSection(createIcon('compliance'), t(title), t('legal.documentHint'), {defaultCollapsed: true});
        const fields = section.querySelector('.cfg-fields');

        const input = el('textarea', {
            class: 'legal-editor',
            rows: 16,
            value: data[key] || '',
            'aria-label': t(title),
            maxLength: MAX_LEGAL_DOCUMENT_BYTES,
            placeholder: t('legal.documentHint') || 'Markdown content...'
        });

        const previewContent = el('article', {class: 'legal-document markdown-body'});
        const previewPlaceholder = el('div', {class: 'legal-preview-placeholder'},
            createIcon('fileText'),
            el('span', {}, t('common.none'))
        );
        const previewBox = el('div', {class: 'legal-preview-box', hidden: true},
            previewContent,
            previewPlaceholder
        );

        let previewing = false;

        const byteInfo = el('span', {class: 'legal-byte-counter'});
        const updateByteInfo = () => {
            const bytes = new TextEncoder().encode(input.value).byteLength;
            byteInfo.textContent = `${(bytes / 1024).toFixed(1)} KiB / 512 KiB`;
        };
        updateByteInfo();

        const editTab = el('button', {
            type: 'button',
            class: 'legal-tab-btn is-active',
            'aria-pressed': 'true',
            onclick: () => setMode(false)
        }, createIcon('edit'), el('span', {}, t('legal.edit')));

        const previewTab = el('button', {
            type: 'button',
            class: 'legal-tab-btn',
            'aria-pressed': 'false',
            onclick: () => setMode(true)
        }, createIcon('eye'), el('span', {}, t('legal.preview')));

        const setMode = (toPreview) => {
            if (toPreview && !input.reportValidity()) return;
            previewing = toPreview;
            if (previewing) {
                if (input.value.trim()) {
                    setSafeMarkdown(previewContent, input.value);
                    previewContent.hidden = false;
                    previewPlaceholder.hidden = true;
                } else {
                    previewContent.replaceChildren();
                    previewContent.hidden = true;
                    previewPlaceholder.hidden = false;
                }
            }
            previewBox.hidden = !previewing;
            input.hidden = previewing;
            editTab.classList.toggle('is-active', !previewing);
            editTab.setAttribute('aria-pressed', String(!previewing));
            previewTab.classList.toggle('is-active', previewing);
            previewTab.setAttribute('aria-pressed', String(previewing));
            if (!previewing) input.focus({preventScroll: true});
        };

        const toolbar = el('div', {class: 'legal-editor-toolbar'},
            el('div', {class: 'legal-toolbar-left'},
                el('span', {class: 'legal-format-badge'}, 'Markdown'),
                byteInfo
            ),
            el('div', {class: 'legal-tab-group', role: 'tablist'},
                editTab,
                previewTab
            )
        );

        input.addEventListener('input', () => {
            const valid = new TextEncoder().encode(input.value).byteLength <= MAX_LEGAL_DOCUMENT_BYTES;
            input.setCustomValidity(valid ? '' : t('legal.invalid'));
            data[key] = input.value;
            updateByteInfo();
            changed();
        });

        const editorWrap = el('div', {class: 'legal-editor-container'},
            toolbar,
            input,
            previewBox
        );

        fields.append(editorWrap);
        wrap.appendChild(section);
    }
    container.appendChild(wrap);
}
