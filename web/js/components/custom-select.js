/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

/*
 * Custom select (slimmed from frontend/renop-html cfg-ui makeCustomSelect).
 */

import { el } from '../lib/dom.js';

/**
 * Dropdown chevron icon (SVG).
 * @returns {SVGSVGElement}
 */
function chevronSvg() {
    const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    svg.setAttribute('class', 'custom-select-arrow');
    svg.setAttribute('viewBox', '0 0 24 24');
    svg.setAttribute('width', '14');
    svg.setAttribute('height', '14');
    svg.setAttribute('fill', 'none');
    svg.setAttribute('stroke', 'currentColor');
    svg.setAttribute('stroke-width', '2.2');
    svg.setAttribute('stroke-linecap', 'round');
    svg.setAttribute('stroke-linejoin', 'round');
    svg.setAttribute('aria-hidden', 'true');
    const p = document.createElementNS('http://www.w3.org/2000/svg', 'polyline');
    p.setAttribute('points', '6 9 12 15 18 9');
    svg.appendChild(p);
    return svg;
}

/**
 * Selected-item checkmark icon (SVG).
 * @returns {SVGSVGElement}
 */
function checkSvg() {
    const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    svg.setAttribute('class', 'custom-select-checkmark');
    svg.setAttribute('viewBox', '0 0 24 24');
    svg.setAttribute('width', '14');
    svg.setAttribute('height', '14');
    svg.setAttribute('fill', 'none');
    svg.setAttribute('stroke', 'currentColor');
    svg.setAttribute('stroke-width', '2.5');
    svg.setAttribute('stroke-linecap', 'round');
    svg.setAttribute('stroke-linejoin', 'round');
    svg.setAttribute('aria-hidden', 'true');
    const p = document.createElementNS('http://www.w3.org/2000/svg', 'polyline');
    p.setAttribute('points', '20 6 9 17 4 12');
    svg.appendChild(p);
    return svg;
}

/**
 * Build a custom dropdown select (button + body-attached menu).
 * @param {Array<{value:string,label:string}|string>} options - Option values and labels.
 * @param {string} current - Initial selected value (or label match).
 * @param {(value: string) => void} [onChange] - Called when the user picks a new value.
 * @returns {{
 *   wrap: HTMLElement,
 *   getValue: () => string,
 *   setValue: (v: string) => void,
 *   setOptions: (options: Array<{value:string,label:string}|string>, preferredValue?: string) => string,
 *   destroy: () => void
 * }} Controller: mount `wrap`, call `destroy` on teardown.
 */
export function makeCustomSelect(options, current, onChange) {
    const wrap = el('div', { class: 'custom-select-wrapper' });
    const btn = el('button', {
        type: 'button',
        class: 'custom-select-btn',
    });

    /**
     * Normalize raw option list to `{ value, label }` objects.
     * @param {Array<{value:string,label:string}|string>} opts
     * @returns {Array<{value: string, label: string}>}
     */
    function normalizeOptions(opts) {
        return (opts || []).map((opt) => {
            if (typeof opt === 'object' && opt !== null) {
                return { value: opt.value, label: opt.label ?? opt.value };
            }
            return { value: opt, label: opt };
        });
    }

    let normalized = normalizeOptions(options);

    let currentVal = current;
    let selectedOpt =
        normalized.find((o) => o.value === currentVal || o.label === currentVal) || normalized[0];
    if (selectedOpt) currentVal = selectedOpt.value;

    const textSpan = el('span', { class: 'custom-select-label' }, selectedOpt ? selectedOpt.label : '');
    const arrow = el('span', { class: 'custom-select-arrow-wrap' });
    arrow.appendChild(chevronSvg());

    btn.appendChild(textSpan);
    btn.appendChild(arrow);

    const dropdown = el('div', { class: 'custom-select-dropdown' });
    document.body.appendChild(dropdown);

    /**
     * Rebuild dropdown list items from `normalized` options.
     * @returns {void}
     */
    function renderItems() {
        dropdown.innerHTML = '';
        normalized.forEach((opt) => {
            const isSelected = selectedOpt && opt.value === selectedOpt.value;
            const item = el('div', {
                class: `custom-select-dropdown-item${isSelected ? ' is-selected' : ''}`,
            });
            item.appendChild(el('span', { class: 'custom-select-item-text' }, opt.label));
            if (isSelected) {
                const check = el('span', { class: 'custom-select-checkmark-wrap' });
                check.appendChild(checkSvg());
                item.appendChild(check);
            }
            item.addEventListener('click', (e) => {
                e.stopPropagation();
                selectedOpt = opt;
                currentVal = opt.value;
                textSpan.textContent = opt.label;
                closeDropdown();
                renderItems();
                if (typeof onChange === 'function') onChange(opt.value);
            });
            dropdown.appendChild(item);
        });
    }

    /**
     * Apply a resolved option to the button label and internal selection state.
     * @param {{value: string, label: string}|undefined} opt
     * @returns {void}
     */
    function applySelection(opt) {
        selectedOpt = opt;
        currentVal = opt ? opt.value : '';
        textSpan.textContent = opt ? opt.label : '';
    }

    /**
     * Position the body-level dropdown under (or above) the trigger button.
     * @returns {void}
     */
    function positionDropdown() {
        const rect = btn.getBoundingClientRect();
        dropdown.style.left = `${rect.left}px`;
        dropdown.style.width = `${Math.max(rect.width, 160)}px`;

        dropdown.style.display = 'block';
        dropdown.style.visibility = 'hidden';
        const dropH = dropdown.offsetHeight;
        dropdown.style.visibility = 'visible';

        if (rect.bottom + dropH + 6 > window.innerHeight && rect.top - dropH - 6 > 0) {
            dropdown.style.top = `${rect.top - dropH - 6}px`;
        } else {
            dropdown.style.top = `${rect.bottom + 6}px`;
        }
    }

    /**
     * Hide this select's dropdown and clear open state classes.
     * @returns {void}
     */
    function closeDropdown() {
        dropdown.style.display = 'none';
        btn.classList.remove('is-open');
        wrap.classList.remove('is-open');
    }

    /**
     * Open this select, closing any other custom-select dropdowns first.
     * @returns {void}
     */
    function openDropdown() {
        document.querySelectorAll('.custom-select-dropdown').forEach((d) => {
            if (d !== dropdown) d.style.display = 'none';
        });
        document.querySelectorAll('.custom-select-wrapper, .custom-select-btn').forEach((b) => {
            if (b !== wrap && b !== btn) b.classList.remove('is-open');
        });
        dropdown.style.display = 'block';
        btn.classList.add('is-open');
        wrap.classList.add('is-open');
        positionDropdown();
    }

    btn.addEventListener('click', (e) => {
        e.stopPropagation();
        if (btn.classList.contains('is-open')) closeDropdown();
        else openDropdown();
    });

    /** @param {MouseEvent} e */
    const onDocClick = (e) => {
        if (!wrap.contains(e.target) && !dropdown.contains(e.target)) {
            closeDropdown();
        }
    };
    /** Reposition open dropdown while the page scrolls. */
    const onScroll = () => {
        if (btn.classList.contains('is-open')) positionDropdown();
    };
    /** Close dropdown on viewport resize. */
    const onResize = () => closeDropdown();

    document.addEventListener('click', onDocClick);
    window.addEventListener('scroll', onScroll, true);
    window.addEventListener('resize', onResize);

    renderItems();
    wrap.appendChild(btn);

    return {
        wrap,
        /** @returns {string} Currently selected option value. */
        getValue: () => currentVal,
        /**
         * Programmatically select an option by value.
         * @param {string} v
         * @returns {void}
         */
        setValue: (v) => {
            const opt = normalized.find((o) => o.value === v);
            if (!opt) return;
            applySelection(opt);
            renderItems();
        },
        /**
         * Replace the option list. Keeps the current value when still valid;
         * otherwise prefers `preferredValue`, then the first option.
         * Does not fire `onChange` (caller can react to the returned value).
         * @param {Array<{value:string,label:string}|string>} nextOptions
         * @param {string} [preferredValue]
         * @returns {string} The value after applying the new options.
         */
        setOptions: (nextOptions, preferredValue) => {
            normalized = normalizeOptions(nextOptions);
            const keep =
                normalized.find((o) => o.value === currentVal) ||
                (preferredValue
                    ? normalized.find((o) => o.value === preferredValue)
                    : undefined) ||
                normalized[0];
            applySelection(keep);
            closeDropdown();
            renderItems();
            return currentVal;
        },
        /**
         * Remove listeners and detach the body-level dropdown.
         * @returns {void}
         */
        destroy: () => {
            document.removeEventListener('click', onDocClick);
            window.removeEventListener('scroll', onScroll, true);
            window.removeEventListener('resize', onResize);
            dropdown.remove();
        },
    };
}
