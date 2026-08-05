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
import {t} from './i18n.js';
import {
    createFieldRow as buildFieldRow,
    createIcon,
    createToggle,
    createToggleRow as buildToggleRow
} from './components.js';

export {el};

export const makeCfgToggle = createToggle;

/**
 * Creates a config-styled text/password input that fires onChange with the string value on change.
 * @param {string|null|undefined} value - Initial input value.
 * @param {string|null|undefined} placeholder - Placeholder text.
 * @param {string} [type='text'] - Input type (e.g. `text`, `password`).
 * @param {function(string): void} onChange - Called with the new value on change.
 * @param {Object.<string, *>} [extraAttrs={}] - Extra attributes merged onto the input.
 * @returns {HTMLInputElement} The input element.
 */
export function makeCfgInput(value, placeholder, type = 'text', onChange, extraAttrs = {}) {
    const defaultAutocomplete = type === 'password' ? 'new-password' : 'off';
    const attrs = {
        class: 'cfg-input',
        type,
        value: value ?? '',
        placeholder: placeholder ?? '',
        autocomplete: defaultAutocomplete,
        ...extraAttrs
    };
    const input = el('input', attrs);
    input.addEventListener('change', e => onChange(e.target.value));
    return input;
}

/**
 * Creates a config-styled input that forwards the native `input` event to onChange.
 * @param {string} type - Input type (e.g. `text`, `number`, `password`).
 * @param {string|number|null|undefined} value - Initial value.
 * @param {string|null|undefined} placeholder - Placeholder text.
 * @param {function(Event): void} onChange - Input event listener.
 * @returns {HTMLInputElement} The input element.
 */
export function buildInput(type, value, placeholder, onChange) {
    const defaultAutocomplete = type === 'password' ? 'new-password' : 'off';
    const input = el('input', {
        class: 'cfg-input',
        type,
        value: value ?? '',
        placeholder: placeholder ?? '',
        autocomplete: defaultAutocomplete
    });
    input.addEventListener('input', onChange);
    return input;
}

/**
 * Creates a compact labeled toggle for inline option rows.
 * @param {string} labelText - Label shown beside the toggle.
 * @param {boolean} checked - Initial checked state.
 * @param {function(boolean): void} onChange - Called when the toggle value changes.
 * @returns {HTMLLabelElement} Wrapper label containing text and toggle.
 */
export function makeInlineToggle(labelText, checked, onChange) {
    const wrap = el('label', {
        style: {
            display: 'flex',
            alignItems: 'center',
            gap: '0.4rem',
            cursor: 'pointer',
            fontSize: '0.82rem',
            whiteSpace: 'nowrap'
        }
    });
    wrap.appendChild(el('span', {}, labelText));
    wrap.appendChild(makeCfgToggle(checked, onChange));
    return wrap;
}

/**
 * Creates a compact labeled number input for inline option rows.
 * @param {string} labelText - Label shown beside the number field.
 * @param {number|null|undefined} value - Initial numeric value.
 * @param {function(number): void} onChange - Called with parsed integer on change.
 * @returns {HTMLDivElement} Wrapper containing label and number input.
 */
export function makeInlineNumber(labelText, value, onChange) {
    const wrap = el('div', {style: {display: 'flex', alignItems: 'center', gap: '0.4rem'}});
    wrap.appendChild(el('span', {style: {fontSize: '0.82rem', whiteSpace: 'nowrap', opacity: '0.75'}}, labelText));
    const inp = el('input', {
        type: 'number',
        class: 'cfg-input',
        value: value ?? 0,
        autocomplete: 'off',
        style: {maxWidth: '80px', padding: '0.35rem 0.5rem', fontSize: '0.82rem'}
    });
    inp.addEventListener('change', e => onChange(parseInt(e.target.value, 10)));
    wrap.appendChild(inp);
    return wrap;
}

/**
 * Creates an editable chip/tag list for allow or deny artifact rules.
 * Supports Enter/comma/semicolon add, paste multi-add, backspace remove, and clear-all.
 * @param {object} options - Tag list configuration.
 * @param {string[]} [options.items=[]] - Initial tag strings.
 * @param {'allow'|'deny'|string} [options.type='allow'] - Chip style modifier (`tag-chip--${type}`).
 * @param {string} [options.placeholder=''] - Input placeholder (falls back to i18n).
 * @param {string} [options.emptyText=''] - Message shown when the list is empty.
 * @param {function(string[]): void} options.onChange - Called with the updated tag list after mutations.
 * @returns {HTMLDivElement} Tag list editor container.
 */
export function makeTagListInput({
                                     items = [],
                                     type = 'allow',
                                     placeholder = '',
                                     emptyText = '',
                                     onChange
                                 }) {
    const list = Array.isArray(items) ? [...items] : [];

    const container = el('div', {class: 'tag-list-editor'});

    const chipsWrap = el('div', {class: 'tag-list-chips'});
    const inputRow = el('div', {class: 'tag-list-input-row'});
    const input = el('input', {
        type: 'text',
        class: 'cfg-input tag-list-input',
        placeholder: placeholder || t('repos.addRulePlaceholder'),
        autocomplete: 'off'
    });
    const addBtn = el('button', {
        type: 'button',
        class: 'pill-btn pill-btn--soft pill-btn--sm tag-list-add-btn'
    }, createIcon('plus'), el('span', {}, t('repos.addRuleBtn')));

    const clearBtn = el('button', {
        type: 'button',
        class: 'tag-list-clear-btn',
        title: t('repos.clearAllRules')
    }, t('repos.clearAllRules'));

    const errorBox = el('div', {class: 'tag-list-error', style: {display: 'none'}});

    /**
     * Shows a validation error message and marks the input invalid.
     * @param {string} msg - Error message text.
     * @returns {void}
     */
    function showError(msg) {
        errorBox.textContent = msg;
        errorBox.style.display = 'flex';
        input.classList.add('is-invalid');
    }

    /**
     * Clears the validation error state.
     * @returns {void}
     */
    function clearError() {
        errorBox.textContent = '';
        errorBox.style.display = 'none';
        input.classList.remove('is-invalid');
    }

    /**
     * Captures bounding rects of current chips keyed by item for FLIP animation.
     * @returns {Map<string, DOMRect>} Map of item key to first-frame rect.
     */
    function captureChipRects() {
        const rects = new Map();
        chipsWrap.querySelectorAll('.tag-chip').forEach(c => {
            if (c.dataset.itemKey) {
                rects.set(c.dataset.itemKey, c.getBoundingClientRect());
            }
        });
        return rects;
    }

    /**
     * Animates chip position changes from previously captured rects (FLIP technique).
     * @param {Map<string, DOMRect>|null} firstRects - First-frame rects from {@link captureChipRects}.
     * @returns {void}
     */
    function animateFLIP(firstRects) {
        if (!firstRects || firstRects.size === 0) return;

        const currentChips = Array.from(chipsWrap.querySelectorAll('.tag-chip'));

        currentChips.forEach(chip => {
            const key = chip.dataset.itemKey;
            if (!key) return;
            const firstRect = firstRects.get(key);
            if (!firstRect) return;

            const lastRect = chip.getBoundingClientRect();
            const dx = firstRect.left - lastRect.left;
            const dy = firstRect.top - lastRect.top;

            if (dx !== 0 || dy !== 0) {
                chip.style.transition = 'none';
                chip.style.transform = `translate3d(${dx}px, ${dy}px, 0)`;

                requestAnimationFrame(() => {
                    requestAnimationFrame(() => {
                        chip.style.transition = 'transform 0.28s cubic-bezier(0.16, 1, 0.3, 1)';
                        chip.style.transform = 'translate3d(0, 0, 0)';

                        setTimeout(() => {
                            chip.style.transition = '';
                            chip.style.transform = '';
                        }, 290);
                    });
                });
            }
        });
    }

    /**
     * Re-renders chip elements from the internal list, optionally animating layout/height.
     * @param {boolean} [animateHeight=true] - Whether to animate chips wrap height changes.
     * @param {Map<string, DOMRect>|null} [firstRects=null] - Prior chip rects for FLIP; chips missing from the map are treated as new.
     * @returns {void}
     */
    function renderChips(animateHeight = true, firstRects = null) {
        const oldHeight = chipsWrap.offsetHeight;

        chipsWrap.innerHTML = '';
        if (list.length === 0) {
            clearBtn.style.display = 'none';
            const emptyEl = el('span', {class: 'tag-list-empty'}, emptyText);
            chipsWrap.appendChild(emptyEl);
        } else {
            clearBtn.style.display = 'inline-block';
            list.forEach((item, idx) => {
                const isNew = !firstRects || !firstRects.has(item);
                const chip = el('span', {
                    class: `tag-chip tag-chip--${type}${isNew ? ' tag-chip--new' : ''}`
                });
                chip.dataset.itemKey = item;

                const textSpan = el('span', {class: 'tag-chip-text'}, item);
                const removeBtn = el('button', {
                    type: 'button',
                    class: 'tag-chip-remove',
                    ariaLabel: 'Remove rule',
                    title: t('common.delete') || 'Remove'
                }, createIcon('close', {width: '12', height: '12'}));
                removeBtn.addEventListener('click', (e) => {
                    e.stopPropagation();
                    const rects = captureChipRects();

                    chip.style.transition = 'opacity 0.14s ease, transform 0.14s ease';
                    chip.style.opacity = '0';
                    chip.style.transform = 'scale(0.6)';

                    setTimeout(() => {
                        list.splice(idx, 1);
                        clearError();
                        renderChips(true, rects);
                        onChange(list);
                    }, 140);
                });

                chip.appendChild(textSpan);
                chip.appendChild(removeBtn);
                chipsWrap.appendChild(chip);
            });
        }

        if (firstRects) {
            animateFLIP(firstRects);
        }

        chipsWrap.scrollTop = chipsWrap.scrollHeight;

        if (animateHeight && oldHeight > 0) {
            const targetHeight = Math.min(chipsWrap.scrollHeight, 108);
            if (Math.abs(oldHeight - targetHeight) > 2) {
                chipsWrap.style.height = oldHeight + 'px';
                chipsWrap.style.transition = 'height 0.3s cubic-bezier(0.16, 1, 0.3, 1)';
                void chipsWrap.offsetHeight;
                chipsWrap.style.height = targetHeight + 'px';

                setTimeout(() => {
                    chipsWrap.style.height = '';
                    chipsWrap.style.transition = '';
                }, 310);
            }
        }
    }

    /**
     * Parses raw text into tags (split on commas, semicolons, newlines, tabs), skips duplicates, and notifies onChange.
     * @param {string} rawText - Raw user input possibly containing multiple rules.
     * @returns {void}
     */
    function addItemsFromRaw(rawText) {
        clearError();
        if (!rawText) return;

        const firstRects = captureChipRects();
        const parts = rawText.split(/[,;\n\r\t]+/).map(s => s.trim()).filter(Boolean);
        let addedCount = 0;
        let hasDuplicate = false;

        parts.forEach(part => {
            if (list.includes(part)) {
                hasDuplicate = true;
            } else {
                list.push(part);
                addedCount++;
            }
        });

        if (hasDuplicate && addedCount === 0) {
            showError(t('repos.ruleExists'));
        }

        if (addedCount > 0) {
            input.value = '';
            renderChips(true, firstRects);
            onChange(list);
        }
    }

    input.addEventListener('keydown', (e) => {
        if (e.key === 'Enter') {
            e.preventDefault();
            addItemsFromRaw(input.value);
        } else if (e.key === ',' || e.key === ';') {
            e.preventDefault();
            addItemsFromRaw(input.value);
        } else if (e.key === 'Backspace' && input.value === '' && list.length > 0) {
            const firstRects = captureChipRects();
            list.pop();
            clearError();
            renderChips(true, firstRects);
            onChange(list);
        } else {
            clearError();
        }
    });

    input.addEventListener('paste', () => {
        setTimeout(() => {
            if (input.value.includes(',') || input.value.includes(';') || input.value.includes('\n')) {
                addItemsFromRaw(input.value);
            }
        }, 10);
    });

    addBtn.addEventListener('click', () => {
        addItemsFromRaw(input.value);
    });

    clearBtn.addEventListener('click', () => {
        if (list.length > 0) {
            const firstRects = captureChipRects();
            list.length = 0;
            clearError();
            renderChips(true, firstRects);
            onChange(list);
        }
    });

    const header = el('div', {class: 'tag-list-header'});
    header.appendChild(clearBtn);

    inputRow.appendChild(input);
    inputRow.appendChild(addBtn);

    container.appendChild(chipsWrap);
    container.appendChild(inputRow);
    container.appendChild(errorBox);
    container.appendChild(header);

    renderChips();
    return container;
}

export const makeFieldRow = buildFieldRow;
export const makeToggleRow = buildToggleRow;


/**
 * Creates a custom dropdown select (portal dropdown on body) with value/label options.
 * Cleans up listeners and dropdown when the wrapper is removed from the DOM.
 * @param {Array<string|{value: *, label?: string}>} options - Option values or `{value, label}` objects.
 * @param {*} current - Currently selected value (or matching label).
 * @param {function(*): void} onChange - Called with the selected option `value` when the user picks an item.
 * @returns {HTMLDivElement} Select wrapper containing the trigger button.
 */
export function makeCustomSelect(options, current, onChange) {
    const wrap = el('div', {class: 'custom-select-wrapper'});
    const btn = el('button', {
        type: 'button',
        class: 'custom-select-btn cfg-input',
    });

    const normalized = options.map(opt => {
        if (typeof opt === 'object' && opt !== null) {
            return {value: opt.value, label: opt.label ?? opt.value};
        }
        return {value: opt, label: opt};
    });

    let currentVal = current;
    let selectedOpt = normalized.find(o => o.value === currentVal || o.label === currentVal) || normalized[0];

    const textSpan = el('span', {class: 'custom-select-label'}, selectedOpt ? selectedOpt.label : '');
    const arrow = el('span', {class: 'custom-select-arrow-wrap'}, createIcon('chevronDown', {class: 'custom-select-arrow'}));

    btn.appendChild(textSpan);
    btn.appendChild(arrow);

    const dropdown = el('div', {class: 'custom-select-dropdown'});

    /**
     * Rebuilds dropdown items and binds selection handlers for the current selection.
     * @returns {void}
     */
    function renderItems() {
        dropdown.innerHTML = '';
        normalized.forEach(opt => {
            const isSelected = selectedOpt && opt.value === selectedOpt.value;
            const item = el('div', {
                class: `custom-select-dropdown-item${isSelected ? ' is-selected' : ''}`
            });

            const itemText = el('span', {class: 'custom-select-item-text'}, opt.label);
            item.appendChild(itemText);

            if (isSelected) {
                const check = el('span', {class: 'custom-select-checkmark-wrap'}, createIcon('check', {class: 'custom-select-checkmark'}));
                item.appendChild(check);
            }

            item.addEventListener('click', e => {
                e.stopPropagation();
                selectedOpt = opt;
                currentVal = opt.value;
                textSpan.textContent = opt.label;
                closeDropdown();
                renderItems();
                onChange(opt.value);
            });

            dropdown.appendChild(item);
        });
    }

    renderItems();
    document.body.appendChild(dropdown);

    /**
     * Positions the portal dropdown below (or above) the trigger to fit the viewport.
     * @returns {void}
     */
    function positionDropdown() {
        const rect = btn.getBoundingClientRect();
        dropdown.style.left = rect.left + 'px';
        dropdown.style.width = Math.max(rect.width, 160) + 'px';

        dropdown.style.display = 'block';
        dropdown.style.visibility = 'hidden';
        const dropH = dropdown.offsetHeight;
        dropdown.style.visibility = 'visible';

        if (rect.bottom + dropH + 6 > window.innerHeight && rect.top - dropH - 6 > 0) {
            dropdown.style.top = (rect.top - dropH - 6) + 'px';
        } else {
            dropdown.style.top = (rect.bottom + 6) + 'px';
        }
    }

    let closeTimeout = null;

    /**
     * Hides the dropdown with exit animation and clears open state classes.
     * @returns {void}
     */
    function closeDropdown() {
        if (dropdown.style.display === 'none' || dropdown.classList.contains('is-closing')) return;
        btn.classList.remove('is-open');
        wrap.classList.remove('is-open');
        dropdown.classList.add('is-closing');
        if (closeTimeout) clearTimeout(closeTimeout);
        closeTimeout = setTimeout(() => {
            dropdown.style.display = 'none';
            dropdown.classList.remove('is-closing');
            closeTimeout = null;
        }, 150);
    }

    /**
     * Opens this dropdown, closes other custom selects, and repositions against the trigger.
     * @returns {void}
     */
    function openDropdown() {
        if (closeTimeout) {
            clearTimeout(closeTimeout);
            closeTimeout = null;
        }
        document.querySelectorAll('.custom-select-dropdown').forEach(d => {
            if (d !== dropdown) {
                d.style.display = 'none';
                d.classList.remove('is-closing');
            }
        });
        document.querySelectorAll('.custom-select-wrapper, .custom-select-btn').forEach(b => {
            if (b !== wrap && b !== btn) b.classList.remove('is-open');
        });
        document.querySelectorAll('.user-dropdown').forEach(d => {
            if (d.id !== 'adjustments-menu') d.style.display = 'none';
        });

        dropdown.classList.remove('is-closing');
        dropdown.style.display = 'block';
        btn.classList.add('is-open');
        wrap.classList.add('is-open');
        positionDropdown();
    }

    btn.addEventListener('click', e => {
        e.stopPropagation();
        const isOpen = dropdown.style.display === 'block' && !dropdown.classList.contains('is-closing');
        if (isOpen) {
            closeDropdown();
        } else {
            openDropdown();
        }
    });

    const onDocClick = (e) => {
        if (!wrap.contains(e.target) && !dropdown.contains(e.target)) {
            closeDropdown();
        }
    };
    const onScrollResize = () => {
        if (dropdown.style.display === 'block') {
            positionDropdown();
        }
    };

    document.addEventListener('click', onDocClick);
    window.addEventListener('scroll', onScrollResize, {passive: true});
    window.addEventListener('resize', onScrollResize, {passive: true});

    const observer = new MutationObserver(() => {
        if (!document.body.contains(wrap)) {
            dropdown.remove();
            document.removeEventListener('click', onDocClick);
            window.removeEventListener('scroll', onScrollResize);
            window.removeEventListener('resize', onScrollResize);
            observer.disconnect();
        }
    });
    observer.observe(document.body, {childList: true, subtree: true});

    wrap.appendChild(btn);
    return wrap;
}

/**
 * Creates a localized visibility badge pill for a repository visibility level.
 * @param {string} visibility - Visibility key: `PUBLIC`, `HIDDEN`, or `PRIVATE` (defaults to PUBLIC styling).
 * @returns {HTMLSpanElement} Badge element.
 */
export function makeVisibilityBadge(visibility) {
    const modifierMap = {
        PUBLIC: 'badge-pill--public',
        HIDDEN: 'badge-pill--hidden',
        PRIVATE: 'badge-pill--private'
    };
    const modifier = modifierMap[visibility] || modifierMap.PUBLIC;
    const keyMap = {
        PUBLIC: 'repos.visibilityPublic',
        HIDDEN: 'repos.visibilityHidden',
        PRIVATE: 'repos.visibilityPrivate'
    };
    const labelKey = keyMap[visibility] || 'repos.visibilityPublic';
    const labelText = t(labelKey);
    return el('span', {
        'data-i18n': labelKey,
        class: `badge-pill ${modifier}`
    }, labelText);
}


/**
 * Creates a config section with header (icon, title, subtitle) and a `.cfg-fields` body.
 * Collapsed by default when collapsible; `appendChild` is patched to append into the body.
 * @param {string|Node|null|undefined} iconSvg - Icon HTML string or Node for the header.
 * @param {string} title - Section title text.
 * @param {string|null|undefined} subtitle - Optional subtitle paragraph.
 * @param {object} [options={}] - Section options.
 * @param {boolean} [options.collapsible=true] - Whether the header toggles collapse.
 * @param {boolean} [options.defaultCollapsed=true] - Initial collapsed state when collapsible.
 * @returns {HTMLDivElement} Section element with `.cfg-fields` inside the body.
 */
export function createSection(iconSvg, title, subtitle, options = {}) {
    const collapsible = options.collapsible !== false;
    const defaultCollapsed = options.defaultCollapsed !== false;

    const sectionClasses = ['cfg-section'];
    if (collapsible && defaultCollapsed) {
        sectionClasses.push('is-collapsed');
    }

    const section = el('div', {class: sectionClasses.join(' ')});
    const header = el('div', {class: 'cfg-section-header'});

    const iconDiv = el('div', {class: 'cfg-section-icon'});
    if (typeof iconSvg === 'string') {
        iconDiv.innerHTML = iconSvg;
    } else if (iconSvg instanceof Node) {
        iconDiv.appendChild(iconSvg);
    }
    header.appendChild(iconDiv);

    const meta = el('div', {class: 'cfg-section-meta'},
        el('h3', {class: 'cfg-section-title'}, title),
        subtitle ? el('p', {class: 'cfg-section-subtitle'}, subtitle) : null
    );
    header.appendChild(meta);

    if (collapsible) {
        const chevronBox = el('div', {class: 'cfg-section-chevron'}, createIcon('chevronDown'));
        header.appendChild(chevronBox);
        header.addEventListener('click', (e) => {
            if (e.target.closest('button, a, input, select')) return;
            section.classList.toggle('is-collapsed');
        });
    }

    section.appendChild(header);

    const fields = el('div', {class: 'cfg-fields'});
    const bodyInner = el('div', {class: 'cfg-section-body-inner'}, fields);
    const body = el('div', {class: 'cfg-section-body'}, bodyInner);
    section.appendChild(body);

    const originalAppendChild = section.appendChild.bind(section);
    section.appendChild = function (child) {
        if (child === header || child === body) {
            return originalAppendChild(child);
        }
        return bodyInner.appendChild(child);
    };

    return section;
}

export const createFieldRow = buildFieldRow;
export const createToggleRow = buildToggleRow;

/**
 * Smoothly animates the appearance or disappearance of a fields container
 * matching the database engine DSN transition in Settings (using height + field entering/leaving keyframes).
 * @param {HTMLElement} container - Fields container element
 * @param {boolean} show - Target visibility state
 * @returns {void}
 */
export function animateFieldsToggle(container, show) {
    if (!container) return;
    if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
        container.style.display = show ? '' : 'none';
        container.style.height = '';
        container.style.transition = '';
        container.style.overflow = '';
        return;
    }

    const isCurrentlyVisible = container.style.display !== 'none' && container.offsetHeight > 0;
    if (show === isCurrentlyVisible) return;

    if (container._animTimer1) clearTimeout(container._animTimer1);
    if (container._animTimer2) clearTimeout(container._animTimer2);
    container._animTimer1 = null;
    container._animTimer2 = null;

    if (!show) {
        const startHeight = container.getBoundingClientRect().height;
        const rows = Array.from(container.children);
        rows.forEach(row => {
            row.classList.remove('cfg-field-row--entering');
            row.classList.add('cfg-field-row--leaving');
        });

        container.style.height = `${startHeight}px`;
        container.style.overflow = 'hidden';
        void container.offsetHeight;

        container.style.transition = 'height 0.3s cubic-bezier(0.16, 1, 0.3, 1)';
        container.style.height = '0px';

        container._animTimer1 = setTimeout(() => {
            container.style.display = 'none';
            container.style.height = '';
            container.style.transition = '';
            container.style.overflow = '';
            rows.forEach(row => row.classList.remove('cfg-field-row--leaving'));
        }, 300);
    } else {
        container.style.display = '';
        container.style.overflow = 'hidden';
        container.style.height = 'auto';
        const targetHeight = container.getBoundingClientRect().height;
        container.style.height = '0px';

        const rows = Array.from(container.children);
        rows.forEach((row, idx) => {
            row.style.setProperty('--field-index', idx);
            row.classList.remove('cfg-field-row--leaving');
            row.classList.add('cfg-field-row--entering');
        });

        void container.offsetHeight;

        container.style.transition = 'height 0.35s cubic-bezier(0.16, 1, 0.3, 1)';
        container.style.height = `${targetHeight}px`;

        container._animTimer1 = setTimeout(() => {
            container.style.height = '';
            container.style.transition = '';
            container.style.overflow = '';
            rows.forEach(row => row.classList.remove('cfg-field-row--entering'));
        }, 350);
    }
}

