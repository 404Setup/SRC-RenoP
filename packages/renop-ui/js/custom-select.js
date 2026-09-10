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
let closeActiveSelect;

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
    const eventNamespace = `.renopCustomSelect${++customSelectSequence}`;
    btn.id = `renop-select-${customSelectSequence}`;
    dropdown.id = `${btn.id}-options`;
    btn.setAttribute('aria-haspopup', 'listbox');
    btn.setAttribute('aria-controls', dropdown.id);
    btn.setAttribute('aria-expanded', 'false');
    dropdown.setAttribute('role', 'listbox');
    dropdown.setAttribute('aria-labelledby', btn.id);

    /**
     * Rebuild dropdown list items from `normalized` options.
     * @returns {void}
     */
    function renderItems() {
        if (!dropdown.isConnected) return;
        $(dropdown).empty();
        normalized.forEach((opt) => {
            const isSelected = selectedOpt && opt.value === selectedOpt.value;
            const item = el('div', {
                class: `custom-select-dropdown-item${isSelected ? ' is-selected' : ''}`,
                role: 'option', tabindex: '-1', 'aria-selected': String(Boolean(isSelected)),
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
                btn.focus({preventScroll: true});
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
        btn.setAttribute('aria-expanded', 'false');
        btn.classList.remove('is-open');
        wrap.classList.remove('is-open');
        dropdown.inert = true;
        if (closeActiveSelect === closeDropdown) closeActiveSelect = undefined;
        $(document).off(`click${eventNamespace}`, onDocClick);
        window.removeEventListener('scroll', onScroll, true);
        $(window).off(`resize${eventNamespace}`, onResize);
        observer?.disconnect();
        if (closeTimeout) clearTimeout(closeTimeout);
        const finish = () => {
            dropdown.remove();
            $(dropdown).empty().css('display', 'none').removeClass('is-leaving');
            closeTimeout = null;
        };
        if (immediate || !dropdown.isConnected || window.matchMedia('(prefers-reduced-motion: reduce)').matches) finish();
        else {
            dropdown.classList.add('is-leaving');
            closeTimeout = setTimeout(finish, 150);
        }
    }

    /** Open the only active menu; closed controls retain no body menu or global listeners. */
    function openDropdown() {
        if (destroyed || !wrap.isConnected || btn.disabled || btn.closest('[inert]')) return;
        if (closeActiveSelect && closeActiveSelect !== closeDropdown) closeActiveSelect(true);
        closeActiveSelect = closeDropdown;
        if (closeTimeout) clearTimeout(closeTimeout);
        closeTimeout = null;
        document.body.appendChild(dropdown);
        dropdown.inert = false;
        renderItems();
        $(dropdown).removeClass('is-leaving').css('display', 'block');
        btn.classList.add('is-open');
        wrap.classList.add('is-open');
        positionDropdown();
        btn.setAttribute('aria-expanded', 'true');
        $(document).on(`click${eventNamespace}`, onDocClick);
        window.addEventListener('scroll', onScroll, {passive: true, capture: true});
        $(window).on(`resize${eventNamespace}`, onResize);
        observer ||= new MutationObserver(() => {
            if (!wrap.isConnected || btn.closest('[inert]')) closeDropdown(true);
        });
        observer.observe(document.body, {childList: true, subtree: true, attributes: true, attributeFilter: ['inert']});
        dropdown.querySelector('.is-selected, .custom-select-dropdown-item')?.focus({preventScroll: true});
    }

    $(btn).on('keydown', e => {
        if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
            e.preventDefault();
            openDropdown();
        }
    });
    $(dropdown).on('keydown', e => {
        if (e.key === 'Escape' || e.key === 'Tab') {
            if (e.key === 'Escape') e.preventDefault();
            e.stopPropagation();
            closeDropdown(true);
            btn.focus({preventScroll: true});
            return;
        }
        const items = Array.from(dropdown.children);
        const index = items.indexOf(document.activeElement);
        if (index < 0) return;
        if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault();
            e.stopPropagation();
            items[index].click();
        } else if (['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(e.key)) {
            e.preventDefault();
            const next = e.key === 'Home' ? 0 : e.key === 'End' ? items.length - 1 : (index + (e.key === 'ArrowDown' ? 1 : -1) + items.length) % items.length;
            items[next].focus({preventScroll: true});
            items[next].scrollIntoView({block: 'nearest'});
        }
    });

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
    const onScroll = event => {
        if (!(event.target instanceof Node) || !dropdown.contains(event.target)) positionDropdown();
    };
    const onResize = () => closeDropdown();

    let observer, destroyed = false;

    /** Release an explicitly discarded control, including an in-flight close animation. */
    function destroy() {
        destroyed = true;
        closeDropdown(true);
        $(btn).off();
        $(dropdown).off();
    }

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
