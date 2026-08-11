/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

export {
    applyTheme,
    bindThemeToggle,
    initTheme,
    playThemeTransition,
} from './theme.js';

export {
    collapseElement,
    expandElement,
    lockElementHeight,
    measureNaturalHeight,
    morphElementHeight,
    prefersReducedMotion,
} from './height-anim.js';

export {
    clear,
    el,
    iconCheck,
} from './dom.js';

export {
    makeCustomSelect,
} from './custom-select.js';

export {
    RenopToggle,
    createToggle,
} from './toggle.js';

export {
    createButton,
} from './button.js';

export {
    RenopLangCard,
    createLangCard,
    defineLangCard,
    fillLangCardBody,
    langShortCode,
} from './lang-card.js';

export {
    MODAL_ANIM,
    bindModalChrome,
    closeModalWithAnim,
    configureModalInert,
    isModalOpen,
    openModalWithAnim,
    updateModalInertState,
} from './modal.js';

export {
    enableDragToScroll,
    smoothScrollToTop,
    wait,
} from './scroll.js';

export {
    registerTabContainer,
    scrollTabIntoView,
    updateTabIndicator,
} from './tabs.js';

export {
    animateLangButtonLabel,
    detectLanguage,
    interpolateMessage,
    matchLocaleKey,
    syncLangCardsActive,
    translateKey,
} from './i18n-util.js';

