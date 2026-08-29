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
import {$} from './jquery.js';

let customSelectSequence = 0;

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
 * Returns an HTMLDivElement (wrapper) with attached controller methods and `wrap` reference.
 *
 * @param {Array<{value:string,label:string}|string>} options - Option values and labels.
 * @param {string} current - Initial selected value (or label match).
 * @param {(value: string) => void} [onChange] - Called when the user picks a new value.
 * @returns {HTMLDivElement & {
 *   wrap: HTMLDivElement,
 *   getValue: () => string,
 *   setValue: (v: string) => void,
 *   setOptions: (options: Array<{value:string,label:string}|string>, preferredValue?: string) => string,
 *   destroy: () => void
 * }}
 */
export function makeCustomSelect(options, current, onChange) {
    const wrap = el('div', {class: 'custom-select-wrapper'});
    const btn = el('button', {
        type: 'button',
        class: 'custom-select-btn cfg-input',
    });

    /**
     * Normalize raw option list to `{ value, label }` objects.
     * @param {Array<{value:string,label:string}|string>} opts
     * @returns {Array<{value: string, label: string}>}
     */
    function normalizeOptions(opts) {
        return (opts || []).map((opt) => {
            if (typeof opt === 'object' && opt !== null) {
                return {value: opt.value, label: opt.label ?? opt.value};
            }
            return {value: opt, label: opt};
        });
    }

    let normalized = normalizeOptions(options);

    let currentVal = current;
    let selectedOpt =
        normalized.find((o) => o.value === currentVal || o.label === currentVal) || normalized[0];
    if (selectedOpt) currentVal = selectedOpt.value;

    const textSpan = el('span', {class: 'custom-select-label'}, selectedOpt ? selectedOpt.label : '');
    const arrow = el('span', {class: 'custom-select-arrow-wrap'});
    arrow.appendChild(chevronSvg());

    btn.appendChild(textSpan);
    btn.appendChild(arrow);

    const dropdown = el('div', {class: 'custom-select-dropdown'});
    $('body').append(dropdown);
    const eventNamespace = `.renopCustomSelect${++customSelectSequence}`;

    /**
     * Rebuild dropdown list items from `normalized` options.
     * @returns {void}
     */
    function renderItems() {
        $(dropdown).empty();
        normalized.forEach((opt) => {
            const isSelected = selectedOpt && opt.value === selectedOpt.value;
            const item = el('div', {
                class: `custom-select-dropdown-item${isSelected ? ' is-selected' : ''}`,
            });
            item.appendChild(el('span', {class: 'custom-select-item-text'}, opt.label));
            if (isSelected) {
                const check = el('span', {class: 'custom-select-checkmark-wrap'});
                check.appendChild(checkSvg());
                item.appendChild(check);
            }
            $(item).on('click', (e) => {
                e.stopPropagation();
                selectedOpt = opt;
                currentVal = opt.value;
                $(textSpan).text(opt.label);
                closeDropdown();
                renderItems();
                if (typeof onChange === 'function') onChange(opt.value);
            });
            $(dropdown).append(item);
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
        $(textSpan).text(opt ? opt.label : '');
    }

    /**
     * Position the body-level dropdown under (or above) the trigger button.
     * @returns {void}
     */
    function positionDropdown() {
        const rect = btn.getBoundingClientRect();
        $(dropdown).css({
            left: `${rect.left}px`,
            width: `${Math.max(rect.width, 160)}px`,
            display: 'block',
            visibility: 'hidden',
        });
        const dropH = dropdown.offsetHeight;
        $(dropdown).css('visibility', 'visible');

        if (rect.bottom + dropH + 6 > window.innerHeight && rect.top - dropH - 6 > 0) {
            $(dropdown).css('top', `${rect.top - dropH - 6}px`);
        } else {
            $(dropdown).css('top', `${rect.bottom + 6}px`);
        }
    }

    let closeTimeout = null;

    /**
     * Hide this select's dropdown and clear open state classes.
     * @param {boolean} [immediate=false]
     * @returns {void}
     */
    function closeDropdown(immediate = false) {
        $(btn).removeClass('is-open');
        $(wrap).removeClass('is-open');
        if (!dropdown || dropdown.style.display === 'none' || $(dropdown).hasClass('is-leaving')) return;

        if (immediate) {
            $(dropdown).css('display', 'none').removeClass('is-leaving');
            return;
        }

        $(dropdown).addClass('is-leaving');
        if (closeTimeout) clearTimeout(closeTimeout);
        closeTimeout = setTimeout(() => {
            $(dropdown).css('display', 'none').removeClass('is-leaving');
            closeTimeout = null;
        }, 150);
    }

    /**
     * Open this select, closing any other custom-select dropdowns first.
     * @returns {void}
     */
    function openDropdown() {
        if (closeTimeout) {
            clearTimeout(closeTimeout);
            closeTimeout = null;
        }
        $('.custom-select-dropdown').each((index, d) => {
            if (d !== dropdown) {
                $(d).css('display', 'none').removeClass('is-leaving');
            }
        });
        $('.custom-select-wrapper, .custom-select-btn').each((index, b) => {
            if (b !== wrap && b !== btn) $(b).removeClass('is-open');
        });
        $(dropdown).removeClass('is-leaving').css('display', 'block');
        $(btn).addClass('is-open');
        $(wrap).addClass('is-open');
        positionDropdown();
    }

    $(btn).on('click', (e) => {
        e.stopPropagation();
        const isOpen = dropdown.style.display === 'block' && !$(dropdown).hasClass('is-leaving');
        if (isOpen) closeDropdown();
        else openDropdown();
    });

    /** @param {MouseEvent} e */
    const onDocClick = (e) => {
        if (!wrap.contains(e.target) && !dropdown.contains(e.target)) {
            closeDropdown();
        }
    };
    const onScroll = () => {
        if ($(btn).hasClass('is-open')) positionDropdown();
    };
    const onResize = () => closeDropdown();

    $(document).on(`click${eventNamespace}`, onDocClick);
    window.addEventListener('scroll', onScroll, {passive: true});
    $(window).on(`resize${eventNamespace}`, onResize);

    // MutationObserver to clean up dropdown if `wrap` is removed from DOM
    const observer = new MutationObserver(() => {
        if (!document.body.contains(wrap)) {
            destroy();
        }
    });
    observer.observe(document.body, {childList: true, subtree: true});

    function destroy() {
        $(document).off(`click${eventNamespace}`, onDocClick);
        window.removeEventListener('scroll', onScroll);
        $(window).off(`resize${eventNamespace}`, onResize);
        observer.disconnect();
        if (dropdown && dropdown.parentNode) {
            $(dropdown).remove();
        }
    }

    renderItems();
    wrap.appendChild(btn);

    wrap.wrap = wrap;
    wrap.getValue = () => currentVal;
    wrap.setValue = (v) => {
        const opt = normalized.find((o) => o.value === v);
        if (!opt) return;
        applySelection(opt);
        renderItems();
    };
    wrap.setOptions = (nextOptions, preferredValue) => {
        normalized = normalizeOptions(nextOptions);
        const keep =
            normalized.find((o) => o.value === currentVal) ||
            (preferredValue
                ? normalized.find((o) => o.value === preferredValue)
                : undefined) ||
            normalized[0];
        applySelection(keep);
        closeDropdown(true);
        renderItems();
        return currentVal;
    };
    wrap.destroy = destroy;

    return wrap;
}
