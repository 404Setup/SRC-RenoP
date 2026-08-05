/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {showConfirm} from './alert.js';

export const MIN_PASSWORD_LENGTH = 6;
export const MAX_PASSWORD_LENGTH = 72;

/**
 * Validate password length is within [6, 72].
 * @param {string} password - Password to validate.
 * @returns {string|null} Error message, or null when valid.
 */
export function getPasswordLengthError(password) {
    const t = window.i18n?.t || ((k) => k);
    if (!password) {
        return t('strength.enterPassword');
    }
    if (password.length < MIN_PASSWORD_LENGTH) {
        return t('strength.minCharError', {min: MIN_PASSWORD_LENGTH});
    }
    if (password.length > MAX_PASSWORD_LENGTH) {
        return t('strength.maxCharError', {max: MAX_PASSWORD_LENGTH});
    }
    return null;
}

/**
 * Evaluate password strength on a 0–4 scale (0 = empty).
 * All scoring is client-side only.
 * @param {string} password - Password to evaluate.
 * @returns {{level: number, label: string, hint: string}} Strength level (0–4), label, and hint.
 */
export function evaluatePasswordStrength(password) {
    if (!password) {
        return {level: 0, label: '', hint: ''};
    }

    const len = password.length;
    let score = 0;

    if (len >= 6) score += 1;
    if (len >= 8) score += 1;
    if (len >= 12) score += 1;
    if (len >= 16) score += 1;

    if (/[a-z]/.test(password)) score += 1;
    if (/[A-Z]/.test(password)) score += 1;
    if (/[0-9]/.test(password)) score += 1;
    if (/[^A-Za-z0-9]/.test(password)) score += 1;

    if (/^(.)\1+$/.test(password)) {
        score = Math.min(score, 1);
    }
    if (/^(0123|1234|2345|3456|4567|5678|6789|7890|abcd|qwer|asdf|password|admin|letmein)/i.test(password)) {
        score = Math.max(0, score - 2);
    }
    if (/^[a-zA-Z]+$/.test(password) && len < 12) {
        score = Math.max(0, score - 1);
    }
    if (/^[0-9]+$/.test(password)) {
        score = Math.min(score, 2);
    }

    let level;
    if (score <= 2) level = 1;
    else if (score <= 4) level = 2;
    else if (score <= 6) level = 3;
    else level = 4;

    if (len < MIN_PASSWORD_LENGTH) level = 1;
    else if (len < 8 && level > 2) level = 2;

    const t = window.i18n?.t || ((k) => k);
    let label = t(`strength.level${level}`);
    let hint = t(`strength.hint${level}`);
    if (len > 0 && len < MIN_PASSWORD_LENGTH) {
        hint = t('strength.minChars', {min: MIN_PASSWORD_LENGTH});
    } else if (len > MAX_PASSWORD_LENGTH) {
        hint = t('strength.maxChars', {max: MAX_PASSWORD_LENGTH});
    }

    return {
        level,
        label,
        hint
    };
}

/**
 * If password strength is below 2, show three sequential warning dialogs.
 * Returns true when the user may proceed (strong enough, or confirmed all warnings).
 * Returns false if the user cancels any warning.
 * @param {string} password - Password to check.
 * @returns {Promise<boolean>} Whether the user may proceed.
 */
export async function confirmWeakPasswordIfNeeded(password) {
    const {level} = evaluatePasswordStrength(password);
    if (level >= 2) return true;

    const t = window.i18n?.t || ((k) => k);
    const warnings = [
        { title: t('strength.warnTitle1'), message: t('strength.warnMsg1') },
        { title: t('strength.warnTitle2'), message: t('strength.warnMsg2') },
        { title: t('strength.warnTitle3'), message: t('strength.warnMsg3') }
    ];

    for (const warning of warnings) {
        const ok = await showConfirm(warning.message, {
            title: warning.title,
            confirmText: t('strength.understandBtn')
        });
        if (!ok) return false;
    }
    return true;
}

/**
 * Insert a live password-strength meter after the given input element.
 * Returns a small controller with update/reset helpers.
 * @param {HTMLInputElement|null} input - Password input to attach the meter to.
 * @returns {{update: Function, reset: Function, getLevel: Function, meter: HTMLElement}|null}
 *   Controller for the meter, or null if input is missing.
 */
export function attachPasswordStrength(input) {
    if (!input || input.dataset.strengthAttached === 'true') {
        return input?._passwordStrengthController || null;
    }

    input.maxLength = MAX_PASSWORD_LENGTH;

    const meter = document.createElement('div');
    meter.className = 'password-strength';
    meter.setAttribute('data-level', '0');
    meter.hidden = true;
    meter.setAttribute('aria-live', 'polite');

    const segments = document.createElement('div');
    segments.className = 'password-strength-segments';
    segments.setAttribute('aria-hidden', 'true');
    for (let i = 0; i < 4; i++) {
        const seg = document.createElement('span');
        seg.className = 'password-strength-seg';
        seg.style.setProperty('--seg-i', String(i));
        segments.appendChild(seg);
    }

    const meta = document.createElement('div');
    meta.className = 'password-strength-meta';

    const label = document.createElement('span');
    label.className = 'password-strength-label';

    const hint = document.createElement('span');
    hint.className = 'password-strength-hint';

    meta.appendChild(label);
    meta.appendChild(hint);
    meter.appendChild(segments);
    meter.appendChild(meta);

    input.insertAdjacentElement('afterend', meter);

    let prevLevel = 0;
    let hideTimer = null;
    let isLeaving = false;
    const EXIT_MS = 280;
    const prefersReducedMotion = () =>
        typeof window !== 'undefined' &&
        window.matchMedia &&
        window.matchMedia('(prefers-reduced-motion: reduce)').matches;

    const clearHideTimer = () => {
        if (hideTimer) {
            clearTimeout(hideTimer);
            hideTimer = null;
        }
    };

    const finishHide = () => {
        clearHideTimer();
        meter.hidden = true;
        meter.setAttribute('data-level', '0');
        meter.classList.remove(
            'password-strength--leaving',
            'password-strength--animate',
            'password-strength--level-change'
        );
        isLeaving = false;
        label.textContent = '';
        hint.textContent = '';
    };

    const showMeter = () => {
        clearHideTimer();
        const wasHidden = meter.hidden;
        const wasLeaving = isLeaving;
        isLeaving = false;
        meter.classList.remove('password-strength--leaving');
        meter.hidden = false;

        if (wasHidden || wasLeaving) {
            meter.classList.remove('password-strength--animate');
            void meter.offsetWidth;
            meter.classList.add('password-strength--animate');
        }
    };

    const hideMeter = (animated = true) => {
        if (meter.hidden && !isLeaving) return;

        if (!animated || prefersReducedMotion()) {
            finishHide();
            return;
        }

        if (isLeaving) return;

        isLeaving = true;
        meter.classList.remove('password-strength--animate', 'password-strength--level-change');
        meter.classList.add('password-strength--leaving');
        clearHideTimer();
        hideTimer = setTimeout(finishHide, EXIT_MS);
    };

    const update = () => {
        const result = evaluatePasswordStrength(input.value);

        if (result.level > 0) {
            meter.setAttribute('data-level', String(result.level));
            label.textContent = result.label;
            hint.textContent = result.hint;
            showMeter();

            if (result.level !== prevLevel) {
                meter.classList.remove('password-strength--level-change');
                void meter.offsetWidth;
                meter.classList.add('password-strength--level-change');
            }
        } else {
            hideMeter(true);
        }

        prevLevel = result.level;
    };

    const reset = () => {
        prevLevel = 0;
        hideMeter(true);
    };

    input.addEventListener('input', update);
    input.addEventListener('change', update);
    input.dataset.strengthAttached = 'true';

    const controller = {
        update,
        reset,
        getLevel: () => evaluatePasswordStrength(input.value).level,
        meter
    };
    input._passwordStrengthController = controller;
    return controller;
}

if (typeof window !== 'undefined') {
    window.addEventListener('languageChanged', () => {
        document.querySelectorAll('[data-strength-attached="true"]').forEach(input => {
            if (input._passwordStrengthController) {
                input._passwordStrengthController.update();
            }
        });
    });
}
