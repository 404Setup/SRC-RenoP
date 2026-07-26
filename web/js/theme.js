/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

const themeToggleBtn = document.getElementById('theme-toggle');

/**
 * Apply light or dark theme classes on `<html>`.
 * @param {'dark'|'light'|string} mode - Target color scheme.
 * @returns {void}
 */
export function applyTheme(mode) {
    if (mode === 'dark') {
        document.documentElement.classList.add('dark');
        document.documentElement.classList.remove('light');
    } else {
        document.documentElement.classList.add('light');
        document.documentElement.classList.remove('dark');
    }
}

/**
 * Restore theme from `localStorage` (`theme-mode`), falling back to system preference when set to `auto`.
 * @returns {void}
 */
export function initTheme() {
    let initialMode = localStorage.getItem('theme-mode') || 'auto';
    if (initialMode === 'auto') {
        initialMode = window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
    }
    applyTheme(initialMode);
}

if (themeToggleBtn) {
    themeToggleBtn.addEventListener('click', () => {
        const isDark = document.documentElement.classList.contains('dark');
        const newMode = isDark ? 'light' : 'dark';
        localStorage.setItem('theme-mode', newMode);
        playThemeTransition(newMode, themeToggleBtn);
    });
}

/**
 * Play a ripple overlay transition, then switch the theme mid-animation.
 * @param {'dark'|'light'|string} newMode - Theme to apply after the overlay starts.
 * @param {HTMLElement|null} btn - Theme toggle button (optional; used for ripple origin and spin class).
 * @returns {void}
 */
function playThemeTransition(newMode, btn) {
    if (btn) {
        btn.classList.add('theme-btn--switching');
        btn.addEventListener('animationend', () => {
            btn.classList.remove('theme-btn--switching');
        }, { once: true });
    }

    const overlay = document.createElement('div');
    overlay.className = 'theme-transition-overlay';
    overlay.style.setProperty('--theme-transition-color', newMode === 'dark' ? '#000000' : '#f3f4f6');

    if (btn) {
        const rect = btn.getBoundingClientRect();
        overlay.style.setProperty('--ripple-x', rect.left + rect.width / 2 + 'px');
        overlay.style.setProperty('--ripple-y', rect.top + rect.height / 2 + 'px');
    }

    document.body.appendChild(overlay);
    const animDuration = 380;
    setTimeout(() => applyTheme(newMode), animDuration * 0.4);
    setTimeout(() => overlay.remove(), animDuration + 50);
}
