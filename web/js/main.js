/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {initTheme} from '@renop/ui/theme';
import {initI18n, t, updatePageTranslations} from './i18n.js';
import {initRouter, registerRoute, renderRoute} from './router.js';
import {renderHome} from './pages/home.js';
import {renderPricing} from './pages/pricing.js';
import {invalidateDocsCache, renderDocs} from './pages/docs.js';
import {renderDownload} from './pages/download.js';
import {renderContributors} from './pages/contributors.js';
import {$} from '@renop/ui/jquery';

initTheme();
initI18n();

/**
 * Fill the footer copyright line with the current year and translated notice.
 * @returns {void}
 */
function updateCopyrightFooter() {
    const el = document.getElementById('footer-copyright');
    if (!el) return;
    const year = new Date().getFullYear();
    el.innerHTML = '';
    const a = document.createElement('a');
    a.href = 'https://github.com/404Setup/SRC-RenoP';
    a.target = '_blank';
    a.rel = 'noopener noreferrer';
    a.textContent = 'RenoP';
    el.append(a, document.createTextNode(` © ${year}. ${t('footer.copyright')}`));
}

registerRoute('/', renderHome);
registerRoute('/pricing', renderPricing);
registerRoute('/download', renderDownload);
registerRoute('/contributors', renderContributors);
registerRoute('/docs', renderDocs);
registerRoute('/docs/*', renderDocs);

initRouter();
updateCopyrightFooter();

$(window).on('languageChanged', async () => {
    updateCopyrightFooter();
    updatePageTranslations();
    invalidateDocsCache();
    await renderRoute();
});

$(renderRoute);
