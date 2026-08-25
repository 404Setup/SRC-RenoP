/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the terms of the Mozilla Public License, v. 2.0.
 */

import {el} from '@renop/ui/dom';

/**
 * Body-level, keyboard-accessible username autocomplete shared by repository teams.
 */
export class RepositoryUserSuggestions {
    /**
     * Initialize a reusable autocomplete controller.
     * @param {object} options - Controller configuration.
     * @param {string} options.id - Stable listbox id.
     * @param {(query: string) => Promise<string[]>} options.fetchUsers - Bounded user search.
     * @param {(error: unknown) => void} [options.onError] - Search failure observer.
     * @param {number} [options.searchDelay=150] - Input debounce in milliseconds.
     * @param {number} [options.closeDelay=180] - Exit animation duration.
     * @param {number} [options.maxResults=8] - Maximum rendered results.
     */
    constructor({id, fetchUsers, onError = null, searchDelay = 150, closeDelay = 180, maxResults = 8}) {
        if (!id || typeof fetchUsers !== 'function') {
            throw new TypeError('Repository user suggestions require an id and fetchUsers callback');
        }
        this.id = id;
        this.fetchUsers = fetchUsers;
        this.onError = onError;
        this.searchDelay = searchDelay;
        this.closeDelay = closeDelay;
        this.maxResults = maxResults;
        this.input = null;
        this.panel = null;
        this.suggestions = [];
        this.activeIndex = -1;
        this.searchTimer = 0;
        this.closeTimer = 0;
        this.requestVersion = 0;
        this.handleInput = this.handleInput.bind(this);
        this.handleKeydown = this.handleKeydown.bind(this);
        this.handlePanelClick = this.handlePanelClick.bind(this);
        this.handleDocumentClick = this.handleDocumentClick.bind(this);
        this.handleViewportChange = this.handleViewportChange.bind(this);
    }

    /**
     * Create or return the body-level listbox.
     * @returns {HTMLElement} Suggestion panel.
     */
    ensurePanel() {
        if (this.panel?.isConnected) return this.panel;
        this.panel = el('div', {
            id: this.id,
            class: 'repository-user-suggestions',
            role: 'listbox',
            hidden: true
        });
        this.panel.addEventListener('click', this.handlePanelClick);
        document.body.appendChild(this.panel);
        return this.panel;
    }

    /**
     * Attach autocomplete behavior to a newly rendered input.
     * @param {HTMLInputElement} input - Username input.
     * @returns {void}
     */
    attach(input) {
        this.detach();
        if (!(input instanceof HTMLInputElement)) return;
        this.input = input;
        input.setAttribute('role', 'combobox');
        input.setAttribute('aria-autocomplete', 'list');
        input.setAttribute('aria-controls', this.id);
        input.setAttribute('aria-expanded', 'false');
        input.addEventListener('input', this.handleInput);
        input.addEventListener('keydown', this.handleKeydown);
        document.addEventListener('click', this.handleDocumentClick);
        window.addEventListener('resize', this.handleViewportChange, {passive: true});
        window.addEventListener('scroll', this.handleViewportChange, {passive: true, capture: true});
    }

    /**
     * Detach from the current input and cancel stale asynchronous work.
     * @returns {void}
     */
    detach() {
        this.requestVersion++;
        if (this.searchTimer) clearTimeout(this.searchTimer);
        this.searchTimer = 0;
        this.close(true);
        if (!this.input) return;
        this.input.removeEventListener('input', this.handleInput);
        this.input.removeEventListener('keydown', this.handleKeydown);
        this.input.removeAttribute('aria-activedescendant');
        document.removeEventListener('click', this.handleDocumentClick);
        window.removeEventListener('resize', this.handleViewportChange);
        window.removeEventListener('scroll', this.handleViewportChange, true);
        this.input = null;
    }

    /**
     * Debounce user search after input changes.
     * @param {InputEvent} event - Input event.
     * @returns {void}
     */
    handleInput(event) {
        if (this.searchTimer) clearTimeout(this.searchTimer);
        this.searchTimer = 0;
        const version = ++this.requestVersion;
        const query = String(event.currentTarget.value || '').trim();
        if (!query) {
            this.render([]);
            this.close(true);
            return;
        }
        this.searchTimer = setTimeout(() => {
            this.searchTimer = 0;
            void this.load(query, version);
        }, this.searchDelay);
    }

    /**
     * Load suggestions while rejecting stale responses.
     * @param {string} query - Username prefix.
     * @param {number} version - Request generation.
     * @returns {Promise<void>}
     */
    async load(query, version) {
        try {
            const users = await this.fetchUsers(query);
            if (version !== this.requestVersion || String(this.input?.value || '').trim() !== query) return;
            this.render(Array.isArray(users) ? users : []);
        } catch (error) {
            if (typeof this.onError === 'function') this.onError(error);
            if (version === this.requestVersion) this.render([]);
        }
    }

    /**
     * Render a bounded result list and open it when non-empty.
     * @param {string[]} users - Suggested usernames.
     * @returns {void}
     */
    render(users) {
        const panel = this.ensurePanel();
        this.suggestions = users.slice(0, this.maxResults).map(String);
        this.activeIndex = -1;
        panel.replaceChildren(...this.suggestions.map((username, index) => el('button', {
            id: `${this.id}-option-${index}`,
            type: 'button',
            role: 'option',
            class: 'repository-user-suggestion',
            'data-repository-user-suggestion': username,
            'aria-selected': 'false'
        }, username)));
        if (this.suggestions.length === 0) {
            this.close();
            return;
        }
        if (this.closeTimer) clearTimeout(this.closeTimer);
        this.closeTimer = 0;
        panel.hidden = false;
        panel.classList.remove('is-leaving');
        this.input?.setAttribute('aria-expanded', 'true');
        this.position();
        requestAnimationFrame(() => panel.classList.add('is-visible'));
    }

    /**
     * Position the fixed listbox below the input or above it when space is constrained.
     * @returns {void}
     */
    position() {
        if (!(this.input instanceof HTMLInputElement) || !this.panel || this.panel.hidden) return;
        const rect = this.input.getBoundingClientRect();
        this.panel.style.left = `${Math.max(10, rect.left)}px`;
        this.panel.style.width = `${Math.max(0, Math.min(rect.width, window.innerWidth - 20))}px`;
        this.panel.style.top = `${rect.bottom + 6}px`;
        const height = this.panel.getBoundingClientRect().height;
        const opensUpward = rect.bottom + height + 12 > window.innerHeight && rect.top > height + 12;
        if (opensUpward) this.panel.style.top = `${rect.top - height - 6}px`;
        this.panel.classList.toggle('opens-upward', opensUpward);
    }

    /**
     * Close the listbox with an optional immediate teardown.
     * @param {boolean} [immediate=false] - Skip the exit transition.
     * @returns {void}
     */
    close(immediate = false) {
        this.suggestions = [];
        this.activeIndex = -1;
        this.input?.setAttribute('aria-expanded', 'false');
        this.input?.removeAttribute('aria-activedescendant');
        if (!this.panel) return;
        if (this.closeTimer) clearTimeout(this.closeTimer);
        this.panel.classList.remove('is-visible');
        if (immediate || this.panel.hidden) {
            this.panel.hidden = true;
            this.panel.classList.remove('is-leaving', 'opens-upward');
            this.closeTimer = 0;
            return;
        }
        this.panel.classList.add('is-leaving');
        this.closeTimer = setTimeout(() => {
            this.panel.hidden = true;
            this.panel.classList.remove('is-leaving', 'opens-upward');
            this.closeTimer = 0;
        }, this.closeDelay);
    }

    /**
     * Apply one username and return focus to the input.
     * @param {string} username - Selected username.
     * @returns {void}
     */
    apply(username) {
        if (this.input instanceof HTMLInputElement) {
            this.input.value = username;
            this.input.focus();
        }
        this.close();
    }

    /**
     * Handle pointer selection from the body-level listbox.
     * @param {MouseEvent} event - Panel click event.
     * @returns {void}
     */
    handlePanelClick(event) {
        if (!this.panel || this.panel.hidden || !this.panel.classList.contains('is-visible')) return;
        const option = event.target.closest('[data-repository-user-suggestion]');
        if (!(option instanceof HTMLElement)) return;
        this.apply(option.dataset.repositoryUserSuggestion || '');
    }

    /**
     * Support escape, arrow, and enter keys for the active listbox.
     * @param {KeyboardEvent} event - Input key event.
     * @returns {void}
     */
    handleKeydown(event) {
        if (event.key === 'Escape') {
            this.close();
            return;
        }
        if (this.suggestions.length === 0) return;
        if (event.key === 'ArrowDown') {
            event.preventDefault();
            this.activeIndex = (this.activeIndex + 1) % this.suggestions.length;
        } else if (event.key === 'ArrowUp') {
            event.preventDefault();
            this.activeIndex = (this.activeIndex - 1 + this.suggestions.length) % this.suggestions.length;
        } else if (event.key === 'Enter' && this.activeIndex >= 0) {
            event.preventDefault();
            this.apply(this.suggestions[this.activeIndex]);
            return;
        } else {
            return;
        }
        const options = this.panel?.querySelectorAll('.repository-user-suggestion') || [];
        for (let index = 0; index < options.length; index++) {
            const active = index === this.activeIndex;
            options[index].classList.toggle('is-active', active);
            options[index].setAttribute('aria-selected', active ? 'true' : 'false');
        }
        const activeOption = options[this.activeIndex];
        if (activeOption) this.input?.setAttribute('aria-activedescendant', activeOption.id);
    }

    /**
     * Close when a document click occurs outside the input and listbox.
     * @param {MouseEvent} event - Document click event.
     * @returns {void}
     */
    handleDocumentClick(event) {
        if (this.input?.contains(event.target) || this.panel?.contains(event.target)) return;
        this.close();
    }

    /**
     * Reposition an open listbox after viewport movement.
     * @returns {void}
     */
    handleViewportChange() {
        if (this.panel && !this.panel.hidden) this.position();
    }

    /**
     * Clear the active input after a successful invitation.
     * @returns {void}
     */
    clear() {
        if (this.input instanceof HTMLInputElement) this.input.value = '';
        this.close();
    }

    /**
     * Permanently release listeners and the body-level panel.
     * @returns {void}
     */
    destroy() {
        this.detach();
        this.panel?.removeEventListener('click', this.handlePanelClick);
        this.panel?.remove();
        this.panel = null;
    }
}
