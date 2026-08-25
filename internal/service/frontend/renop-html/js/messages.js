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
import {makeCustomSelect} from '@renop/ui/custom-select';
import {collapseElement, expandElement, morphElementHeight, prefersReducedMotion} from '@renop/ui/height-anim';
import {openModalWithAnim} from '@renop/ui/modal';
import {createToggle} from '@renop/ui/toggle';
import {fetchProto, postProto, sendProto} from './api.js';
import {closeModalWithAnim} from './app-ui.js';
import {cachedIsLoggedIn, cachedIsManager} from './auth.js';
import {showAlert, showConfirm} from './alert.js';
import {t} from './i18n.js';
import {createUserIdentity} from './components.js';
import {
    ClearMessagesResponse,
    MarkAllReadResponse,
    SendNotificationRequest,
    SendNotificationResponse,
    StatusOk,
    UnreadCountResponse,
    UserMessageList,
    UserSearchResponse
} from './proto/index.js';
import {timestampMilliseconds} from './time.js';

const actionHandlers = new Map();
const messageRenderers = new Map();
let messages = [];
let nextCursor = '';
let unreadCount = 0;
let pollTimer = 0;
let initialized = false;
let loading = false;
let severitySelect = null;
let recipientSuggestions = [];
let activeRecipientSuggestion = -1;
let recipientSuggestionTimer = 0;
let recipientSuggestionVersion = 0;
let notificationSending = false;
let notificationCompletionTimer = 0;
let broadcastToggle = null;
let composerOpen = false;
let clearingMessages = false;
let messageDialogHeightAnimation = null;
let messageDialogHeightGeneration = 0;
let composerContentAnimation = null;

/**
 * Register a trusted handler for one typed message action.
 * @param {string} kind - Server-defined action kind.
 * @param {(message: object, decision: string) => Promise<boolean>} handler - Feature-owned action handler.
 * @returns {void}
 */
export function registerMessageActionHandler(kind, handler) {
    if (kind && typeof handler === 'function') {
        actionHandlers.set(kind, handler);
        renderMessages();
    }
}

/**
 * Register localized presentation for one server-defined message kind.
 * @param {string} kind - Message kind.
 * @param {(message: object) => {title?: string, body?: string}} renderer - Safe text renderer.
 * @returns {void}
 */
export function registerMessageRenderer(kind, renderer) {
    if (kind && typeof renderer === 'function') {
        messageRenderers.set(kind, renderer);
        renderMessages();
    }
}

/**
 * Initialize message-center controls and authentication-aware polling once.
 * @returns {void}
 */
export function initMessageCenter() {
    if (initialized) return;
    initialized = true;
    document.getElementById('message-center-btn')?.addEventListener('click', openMessageCenter);
    document.getElementById('message-center-close')?.addEventListener('click', closeMessageCenter);
    document.getElementById('message-center-backdrop')?.addEventListener('click', closeMessageCenter);
    document.getElementById('message-mark-all-read')?.addEventListener('click', markAllRead);
    document.getElementById('message-clear-all')?.addEventListener('click', clearAllMessages);
    document.getElementById('message-compose-toggle')?.addEventListener('click', toggleComposer);
    document.getElementById('message-compose-cancel')?.addEventListener('click', hideComposer);
    document.getElementById('message-compose-form')?.addEventListener('submit', submitNotification);
    document.getElementById('message-compose-recipients')?.addEventListener('input', handleRecipientInput);
    document.getElementById('message-compose-recipients')?.addEventListener('keydown', handleRecipientKeydown);
    document.getElementById('message-recipient-suggestions')?.addEventListener('click', handleRecipientSuggestionClick);
    document.getElementById('message-center-list')?.addEventListener('click', handleMessageListClick);
    const loadMore = document.getElementById('message-load-more');
    bindLoadMoreButton(loadMore);
    document.addEventListener('click', handleRecipientDocumentClick);
    window.addEventListener('authChanged', handleAuthChanged);
    window.addEventListener('languageChanged', handleLanguageChanged);
    initializeSeveritySelect();
    initializeBroadcastToggle();
    if (cachedIsLoggedIn) {
        startPolling();
    }
}

/**
 * Open the modal and refresh its first page.
 * @returns {Promise<void>}
 */
async function openMessageCenter() {
    const modal = document.getElementById('message-center-modal');
    if (!modal || !cachedIsLoggedIn) return;
    openModalWithAnim(modal);
    updateComposerVisibility();
    await fetchMessages(true);
}

/**
 * Close the message-center modal with the shared animation.
 * @returns {void}
 */
function closeMessageCenter() {
    const modal = document.getElementById('message-center-modal');
    void hideComposer();
    if (modal) closeModalWithAnim(modal);
}

/**
 * Fetch a message page and update pagination state.
 * @param {boolean} reset - Whether to replace the current list.
 * @param {string} [requestedCursor=''] - Cursor captured by the clicked button.
 * @returns {Promise<void>}
 */
async function fetchMessages(reset, requestedCursor = '') {
    if (loading) return;
    if (!reset && !requestedCursor) return;
    loading = true;
    const loadingNode = document.getElementById('message-center-loading');
    const loadMore = document.getElementById('message-load-more');
    if (loadingNode && reset) loadingNode.hidden = false;
    if (loadMore) {
        loadMore.setAttribute('aria-busy', 'true');
        loadMore.classList.toggle('is-loading', !reset);
    }
    try {
        const cursor = reset ? '' : requestedCursor;
        const url = cursor ? `/api/messages?limit=30&cursor=${encodeURIComponent(cursor)}` : '/api/messages?limit=30';
        const {response, data: payload} = await fetchProto(url, UserMessageList);
        if (!response.ok) throw new Error(await responseError(response));
        const page = Array.isArray(payload?.messages) ? payload.messages : [];
        let firstAddedMessageID = '';
        if (reset) {
            messages = page;
        } else {
            firstAddedMessageID = appendUniqueMessagePage(page);
        }
        nextCursor = typeof payload?.next_cursor === 'string' ? payload.next_cursor : '';
        setUnreadCount(Number(payload?.unread_count) || 0);
        renderMessages(firstAddedMessageID, true, reset);
    } catch (error) {
        console.error('Failed to load messages', error);
        showAlert(t('messages.loadFailed'), 'error');
    } finally {
        loading = false;
        if (loadingNode) loadingNode.hidden = true;
        if (loadMore) {
            loadMore.disabled = false;
            loadMore.setAttribute('aria-busy', 'false');
            loadMore.classList.remove('is-loading');
        }
        updateMessageToolbarState();
    }
}

/**
 * Append a cursor page once per server-owned message ID.
 * @param {Array<object>} page - Newly fetched message page.
 * @returns {string} First newly appended message ID.
 */
function appendUniqueMessagePage(page) {
    const existingIDs = new Set();
    for (const message of messages) existingIDs.add(String(message.id || ''));
    let firstAddedMessageID = '';
    for (const message of page) {
        const id = String(message.id || '');
        if (!id || existingIDs.has(id)) continue;
        if (!firstAddedMessageID) firstAddedMessageID = id;
        existingIDs.add(id);
        messages.push(message);
    }
    return firstAddedMessageID;
}

/**
 * Attach pagination directly to the stable button node.
 * @param {HTMLElement|null} button - Pagination button.
 * @returns {void}
 */
function bindLoadMoreButton(button) {
    if (!button) return;
    button.disabled = false;
    button.onclick = handleLoadMoreButtonClick;
    button.dataset.paginationBound = 'true';
}

/**
 * Load the cursor carried by the activated pagination button.
 * @param {MouseEvent} event - Native button activation.
 * @returns {void}
 */
function handleLoadMoreButtonClick(event) {
    event.preventDefault();
    const button = event.currentTarget;
    if (!(button instanceof HTMLElement)) return;
    void loadMoreMessages(button.dataset.nextCursor || nextCursor);
}

/**
 * Load the exact cursor stored on the visible pagination control.
 * @param {string} cursor - Server-issued next-page cursor.
 * @returns {Promise<void>}
 */
async function loadMoreMessages(cursor) {
    if (cursor && !loading) await fetchMessages(false, cursor);
}

/**
 * Render current message state using text-only DOM nodes.
 * @param {string} [focusMessageID=''] - Newly appended message to reveal.
 * @param {boolean} [animateEntries=false] - Whether newly rendered cards should enter visibly.
 * @param {boolean} [hideLoading=false] - Whether loading should be replaced in the same resize.
 * @returns {void}
 */
function renderMessages(focusMessageID = '', animateEntries = false, hideLoading = false) {
    const body = document.getElementById('message-center-body');
    const list = document.getElementById('message-center-list');
    const empty = document.getElementById('message-center-empty');
    const loadMore = document.getElementById('message-load-more');
    if (!list || !empty || !loadMore) return;
    let focusedCard = null;

    /**
     * Mutate message DOM while the shared height helper holds the previous body size.
     * @returns {void}
     */
    function mutateMessageDOM() {
        if (hideLoading) {
            const loadingNode = document.getElementById('message-center-loading');
            if (loadingNode) loadingNode.hidden = true;
        }
        empty.hidden = messages.length !== 0;
        loadMore.hidden = nextCursor === '';
        loadMore.disabled = false;
        loadMore.dataset.nextCursor = nextCursor;
        bindLoadMoreButton(loadMore);
        let startIndex = 0;
        if (focusMessageID) {
            for (let index = 0; index < messages.length; index++) {
                if (messages[index].id === focusMessageID) {
                    startIndex = index;
                    break;
                }
            }
        }
        const appendOnly = Boolean(focusMessageID) && startIndex > 0 && list.children.length === startIndex;
        if (!appendOnly) {
            list.innerHTML = '';
            startIndex = 0;
        }
        for (let index = startIndex; index < messages.length; index++) {
            const message = messages[index];
            const card = buildMessageCard(message);
            if (animateEntries) card.classList.add('is-entering');
            if (focusMessageID && message.id === focusMessageID) {
                focusedCard = card;
            }
            list.appendChild(card);
        }
    }

    const resize = morphElementHeight(body, mutateMessageDOM, {duration: 320});
    updateMessageToolbarState();
    if (focusedCard) void revealFocusedMessage(resize, focusedCard);
}

/**
 * Scroll a newly added message into view after the dialog body finishes resizing.
 * @param {Promise<void>} resize - In-flight body resize.
 * @param {HTMLElement} card - Newly rendered card.
 * @returns {Promise<void>}
 */
async function revealFocusedMessage(resize, card) {
    await resize;
    if (card.isConnected) card.scrollIntoView({behavior: 'smooth', block: 'nearest'});
}

/**
 * Build one safe, interactive message card.
 * @param {object} message - Message API object.
 * @returns {HTMLElement} Message card.
 */
function buildMessageCard(message) {
    const presentation = messagePresentation(message);
    const card = el('article', {
        class: `message-card${message.read_at ? '' : ' is-unread'}`,
        'data-message-id': String(message.id || ''),
        'data-severity': String(message.severity || 'info')
    });
    const heading = el('div', {class: 'message-card-heading'},
        el('div', {class: 'message-card-title-row'},
            el('span', {class: 'message-severity-badge'}, messageSeverityLabel(message.severity)),
            el('h3', {class: 'message-card-title'}, presentation.title)
        )
    );
    if (!message.read_at) {
        heading.appendChild(el('span', {class: 'message-card-unread-dot', 'aria-label': t('messages.unread')}));
    }
    card.appendChild(heading);
    card.appendChild(el('p', {class: 'message-card-body'}, presentation.body));
    const sender = message.sender
        ? createUserIdentity(message.sender, {template: 'messages.from'})
        : el('span', {}, t('messages.system'));
    const messageMeta = el('span', {class: 'message-card-meta'}, sender);
    messageMeta.appendChild(document.createTextNode(` · ${formatMessageDate(message.created_at)}`));
    const footer = el('div', {class: 'message-card-footer'},
        messageMeta,
        buildMessageActions(message)
    );
    card.appendChild(footer);
    return card;
}

/**
 * Translate a message severity into the composer terminology.
 * @param {string} severity - Server-defined severity.
 * @returns {string} Localized severity label.
 */
function messageSeverityLabel(severity) {
    switch (severity) {
        case 'success': return t('messages.severitySuccess');
        case 'warning': return t('messages.severityWarning');
        case 'error': return t('messages.severityError');
        default: return t('messages.severityInfo');
    }
}

/**
 * Resolve optional feature-specific text while retaining persisted fallback text.
 * @param {object} message - Message API object.
 * @returns {{title: string, body: string}} Safe presentation strings.
 */
function messagePresentation(message) {
    const renderer = messageRenderers.get(message.kind);
    if (renderer) {
        const rendered = renderer(message) || {};
        return {
            title: String(rendered.title || message.title || ''),
            body: String(rendered.body || message.body || '')
        };
    }
    return {title: String(message.title || ''), body: String(message.body || '')};
}

/**
 * Build status and action controls for one message.
 * @param {object} message - Message API object.
 * @returns {HTMLElement} Actions container.
 */
function buildMessageActions(message) {
    const actions = el('div', {class: 'message-card-actions'});
    const pending = message.action_kind && message.action_status === 'pending';
    const handler = actionHandlers.get(message.action_kind);
    if (pending && handler) {
        actions.appendChild(messageButton('reject', t('messages.reject'), 'pill-btn pill-btn--soft pill-btn--sm'));
        actions.appendChild(messageButton('accept', t('messages.accept'), 'pill-btn pill-btn--primary pill-btn--sm'));
    } else if (message.action_status) {
        actions.appendChild(el('span', {class: 'message-action-status'}, actionStatusLabel(message.action_status)));
    }
    if (!pending) {
        actions.appendChild(messageButton('delete', t('common.delete'), 'pill-btn pill-btn--danger pill-btn--sm'));
    }
    return actions;
}

/**
 * Create a delegated message operation button.
 * @param {string} operation - Operation name.
 * @param {string} label - Visible text.
 * @param {string} className - CSS classes.
 * @returns {HTMLButtonElement} Button element.
 */
function messageButton(operation, label, className) {
    return el('button', {type: 'button', class: className, 'data-message-operation': operation}, label);
}

/**
 * Translate a terminal or pending action status.
 * @param {string} status - Server action status.
 * @returns {string} Localized label.
 */
function actionStatusLabel(status) {
    const key = `messages.action.${status}`;
    const translated = t(key);
    return translated === key ? status : translated;
}

/**
 * Format a millisecond timestamp in the active browser locale.
 * @param {number|string} timestamp - Unix milliseconds.
 * @returns {string} Localized date/time.
 */
function formatMessageDate(timestamp) {
    const value = timestampMilliseconds(timestamp);
    if (!Number.isFinite(value) || value <= 0) return '';
    return new Intl.DateTimeFormat(undefined, {dateStyle: 'medium', timeStyle: 'short'}).format(new Date(value));
}

/**
 * Handle delegated read, delete, accept, and reject operations.
 * @param {MouseEvent} event - List click event.
 * @returns {Promise<void>}
 */
async function handleMessageListClick(event) {
    const card = event.target.closest('.message-card');
    if (!card) return;
    const message = messages.find(candidate => candidate.id === card.dataset.messageId);
    if (!message) return;
    const operationButton = event.target.closest('[data-message-operation]');
    if (operationButton) {
        const operation = operationButton.dataset.messageOperation;
        if (operation === 'delete') {
            await deleteMessage(message);
        } else if (operation === 'accept' || operation === 'reject') {
            await executeMessageAction(message, operation);
        }
        return;
    }
    if (!message.read_at) await markMessageRead(message);
}

/**
 * Mark one message read and update local unread state.
 * @param {object} message - Message to update.
 * @returns {Promise<void>}
 */
async function markMessageRead(message) {
    try {
        const {response} = await postProto(`/api/messages/${encodeURIComponent(message.id)}/read`, null, null, StatusOk);
        if (!response.ok) throw new Error(await responseError(response));
        message.read_at = Date.now();
        setUnreadCount(Math.max(0, unreadCount - 1));
        renderMessages();
    } catch (error) {
        console.error('Failed to mark message read', error);
    }
}

/**
 * Mark all visible and server-side messages read.
 * @returns {Promise<void>}
 */
async function markAllRead() {
    try {
        const {response, data: payload} = await postProto('/api/messages/read-all', null, null, MarkAllReadResponse);
        if (!response.ok) throw new Error(await responseError(response));
        const now = Date.now();
        for (const message of messages) message.read_at = message.read_at || now;
        setUnreadCount(0);
        renderMessages();
    } catch (error) {
        console.error('Failed to mark all messages read', error);
        showAlert(t('messages.updateFailed'), 'error');
    }
}

/**
 * Clear every dismissible notification after explicit confirmation.
 * Pending workflow requests are intentionally retained by the server.
 * @returns {Promise<void>}
 */
async function clearAllMessages() {
    if (clearingMessages || loading) return;
    clearingMessages = true;
    updateMessageToolbarState();
    try {
        const confirmed = await showConfirm(t('messages.clearAllConfirm'), {
            title: t('messages.clearAll'),
            confirmText: t('messages.clearAll'),
            danger: true
        });
        if (!confirmed) return;
        setClearMessagesBusy(true);
        const {response, data: payload} = await sendProto('/api/messages', 'DELETE', null, null, ClearMessagesResponse);
        if (!response.ok) throw new Error(await responseError(response));
        await animateDismissibleMessagesOut();
        await fetchMessages(true);
        showAlert(t('messages.cleared', {count: Number(payload?.deleted) || 0}), 'success');
    } catch (error) {
        console.error('Failed to clear messages', error);
        showAlert(t('messages.clearFailed'), 'error');
    } finally {
        clearingMessages = false;
        setClearMessagesBusy(false);
        updateMessageToolbarState();
    }
}

/**
 * Toggle clear-all progress feedback without replacing its translated label.
 * @param {boolean} busy - Whether the clear request is in flight.
 * @returns {void}
 */
function setClearMessagesBusy(busy) {
    const button = document.getElementById('message-clear-all');
    if (!button) return;
    button.classList.toggle('is-loading', busy);
    button.setAttribute('aria-busy', String(busy));
}

/**
 * Keep bulk message controls synchronized with loaded and server-side state.
 * @returns {void}
 */
function updateMessageToolbarState() {
    const markAll = document.getElementById('message-mark-all-read');
    const clearAll = document.getElementById('message-clear-all');
    if (markAll) markAll.disabled = loading || unreadCount === 0;
    if (clearAll) clearAll.disabled = clearingMessages || loading || (messages.length === 0 && !nextCursor);
}

/**
 * Delete a non-pending message owned by the current user.
 * @param {object} message - Message to remove.
 * @returns {Promise<void>}
 */
async function deleteMessage(message) {
    try {
        const {response} = await sendProto(`/api/messages/${encodeURIComponent(message.id)}`, 'DELETE', null, null, StatusOk);
        if (!response.ok) throw new Error(await responseError(response));
        const remainingMessages = [];
        for (const candidate of messages) {
            if (candidate.id !== message.id) remainingMessages.push(candidate);
        }
        const card = findMessageCard(message.id);
        const showEmpty = remainingMessages.length === 0 && !nextCursor;
        const resizePlan = card ? prepareMessageRemovalDialogResize([card], showEmpty) : null;
        const empty = document.getElementById('message-center-empty');
        const emptyAnimation = showEmpty ? expandMessageEmptyState(empty, 220) : Promise.resolve();
        await Promise.all([
            collapseMessageCard(message.id),
            emptyAnimation,
            animateMessageDialogResize(resizePlan, 220)
        ]);
        if (!message.read_at) setUnreadCount(Math.max(0, unreadCount - 1));
        messages = remainingMessages;
        card?.remove();
        syncMessageListControls();
        if (messages.length === 0 && nextCursor) await fetchMessages(true);
    } catch (error) {
        console.error('Failed to delete message', error);
        showAlert(t('messages.deleteFailed'), 'error');
    }
}


/**
 * Collapse and fade a message before removing it from local state.
 * @param {string} messageID - Server-owned message identifier.
 * @returns {Promise<void>}
 */
async function collapseMessageCard(messageID) {
    const card = findMessageCard(messageID);
    if (!card) return;
    card.classList.add('is-leaving');
    await collapseMessageCardElement(card, 220);
}

/**
 * Collapse a card's box and vertical padding together so no padding-height
 * plateau remains at the end of the exit animation.
 * @param {HTMLElement} card - Rendered message card.
 * @param {number} duration - Animation duration in milliseconds.
 * @returns {Promise<void>}
 */
async function collapseMessageCardElement(card, duration) {
    if (prefersReducedMotion() || typeof card.animate !== 'function') {
        card.hidden = true;
        card.style.display = 'none';
        return;
    }
    const computed = getComputedStyle(card);
    const animation = card.animate([
        {
            height: `${card.getBoundingClientRect().height}px`,
            paddingTop: computed.paddingTop,
            paddingBottom: computed.paddingBottom,
            borderBottomWidth: computed.borderBottomWidth,
            opacity: 1
        },
        {
            height: '0px',
            paddingTop: '0px',
            paddingBottom: '0px',
            borderBottomWidth: '0px',
            opacity: 0
        }
    ], {
        duration,
        easing: 'cubic-bezier(0.4, 0, 0.2, 1)',
        fill: 'both'
    });
    try {
        await animation.finished;
    } catch (error) {
        if (error?.name !== 'AbortError') console.warn('Message card exit did not finish', error);
    }
    card.hidden = true;
    card.style.display = 'none';
    try {
        animation.cancel();
    } catch (error) {
        console.warn('Failed to release message card exit', error);
    }
}

/**
 * Reveal the empty state while its padding and height grow from zero.
 * @param {HTMLElement|null} empty - Empty-state element.
 * @param {number} duration - Animation duration in milliseconds.
 * @returns {Promise<void>}
 */
async function expandMessageEmptyState(empty, duration) {
    if (!empty) return;
    empty.hidden = false;
    empty.style.display = 'block';
    empty.classList.add('is-visible');
    if (prefersReducedMotion() || typeof empty.animate !== 'function') return;
    const computed = getComputedStyle(empty);
    const targetHeight = empty.getBoundingClientRect().height;
    empty.style.overflow = 'hidden';
    const animation = empty.animate([
        {height: '0px', paddingTop: '0px', paddingBottom: '0px', opacity: 0},
        {
            height: `${targetHeight}px`,
            paddingTop: computed.paddingTop,
            paddingBottom: computed.paddingBottom,
            opacity: 1
        }
    ], {
        duration,
        easing: 'cubic-bezier(0.4, 0, 0.2, 1)',
        fill: 'both'
    });
    try {
        await animation.finished;
    } catch (error) {
        if (error?.name !== 'AbortError') console.warn('Message empty state entry did not finish', error);
    }
    try {
        animation.cancel();
    } catch (error) {
        console.warn('Failed to release message empty state entry', error);
    }
    empty.style.display = '';
    empty.style.overflow = '';
}

/**
 * Find a rendered message card without interpolating an unescaped selector.
 * @param {string} messageID - Server-owned message identifier.
 * @returns {HTMLElement|null} Matching rendered card.
 */
function findMessageCard(messageID) {
    const escapedID = CSS.escape(String(messageID || ''));
    return document.querySelector(`.message-card[data-message-id="${escapedID}"]`);
}

/**
 * Identify workflow messages that cannot be dismissed yet.
 * @param {object} message - Message API object.
 * @returns {boolean} Whether the action is still pending.
 */
function isPendingActionMessage(message) {
    return Boolean(message.action_kind) && message.action_status === 'pending';
}

/**
 * Animate all currently loaded dismissible notifications out together.
 * @returns {Promise<void>}
 */
async function animateDismissibleMessagesOut() {
    const retainedMessages = [];
    const removableCards = [];
    for (const message of messages) {
        if (isPendingActionMessage(message)) {
            retainedMessages.push(message);
            continue;
        }
        const card = findMessageCard(message.id);
        if (card) removableCards.push(card);
    }
    if (removableCards.length > 0) {
        const showEmpty = retainedMessages.length === 0;
        const resizePlan = prepareMessageRemovalDialogResize(removableCards, showEmpty);
        const animations = [animateMessageDialogResize(resizePlan, 220)];
        const empty = document.getElementById('message-center-empty');
        if (showEmpty) animations.push(expandMessageEmptyState(empty, 220));
        for (const card of removableCards) {
            card.classList.add('is-leaving');
            animations.push(collapseMessageCardElement(card, 220));
        }
        await Promise.all(animations);
        for (const card of removableCards) card.remove();
    }
    messages = retainedMessages;
    nextCursor = '';
    syncMessageListControls();
}

/**
 * Update empty and pagination controls without rebuilding cards after an exit.
 * @returns {void}
 */
function syncMessageListControls() {
    const empty = document.getElementById('message-center-empty');
    const loadMore = document.getElementById('message-load-more');
    if (empty) empty.hidden = messages.length !== 0;
    if (loadMore) {
        loadMore.hidden = nextCursor === '';
        loadMore.dataset.nextCursor = nextCursor;
        bindLoadMoreButton(loadMore);
    }
    updateMessageToolbarState();
}

/**
 * Restore an element's exact previous inline-style attribute.
 * @param {HTMLElement} element - Element whose measurement styles changed.
 * @param {string|null} inlineStyle - Previously captured style attribute.
 * @returns {void}
 */
function restoreInlineStyle(element, inlineStyle) {
    if (inlineStyle === null) {
        element.removeAttribute('style');
    } else {
        element.setAttribute('style', inlineStyle);
    }
}

/**
 * Start a no-paint measurement transaction for the message dialog shell.
 * @returns {{dialog: HTMLElement, from: number, to: number, generation: number}|null} Resize plan.
 */
function beginMessageDialogResize() {
    const dialog = document.querySelector('#message-center-modal .message-center-modal-content');
    if (!(dialog instanceof HTMLElement) || prefersReducedMotion() || typeof dialog.animate !== 'function') return null;
    const from = dialog.getBoundingClientRect().height;
    messageDialogHeightGeneration++;
    if (messageDialogHeightAnimation) {
        try {
            messageDialogHeightAnimation.cancel();
        } catch (error) {
            console.warn('Failed to cancel message dialog resize', error);
        }
        messageDialogHeightAnimation = null;
    }
    dialog.style.height = '';
    return {dialog, from, to: from, generation: messageDialogHeightGeneration};
}

/**
 * Measure the shell height after the composer reaches its target state.
 * @param {HTMLElement} region - Composer height wrapper.
 * @param {boolean} opening - Whether to measure the expanded state.
 * @returns {{dialog: HTMLElement, from: number, to: number, generation: number}|null} Resize plan.
 */
function prepareComposerDialogResize(region, opening) {
    const previousStyle = region.getAttribute('style');
    const previousHidden = region.hidden;
    const previousVisible = region.classList.contains('is-visible');
    const plan = beginMessageDialogResize();
    if (!plan) return null;
    region.hidden = !opening;
    region.style.display = opening ? 'block' : 'none';
    region.style.height = opening ? 'auto' : '0px';
    region.style.opacity = opening ? '1' : '0';
    region.style.overflow = opening ? 'visible' : 'hidden';
    region.classList.toggle('is-visible', opening);
    void plan.dialog.offsetHeight;
    plan.to = plan.dialog.getBoundingClientRect().height;
    restoreInlineStyle(region, previousStyle);
    region.hidden = previousHidden;
    region.classList.toggle('is-visible', previousVisible);
    plan.dialog.style.height = `${plan.from}px`;
    return plan;
}

/**
 * Measure the shell after selected cards disappear and the empty state settles.
 * @param {Array<HTMLElement>} cards - Cards that are about to be removed.
 * @param {boolean} showEmpty - Whether the empty state will replace the cards.
 * @returns {{dialog: HTMLElement, from: number, to: number, generation: number}|null} Resize plan.
 */
function prepareMessageRemovalDialogResize(cards, showEmpty) {
    const empty = document.getElementById('message-center-empty');
    const list = document.getElementById('message-center-list');
    const cardSnapshots = [];
    for (const card of cards) {
        cardSnapshots.push({card, hidden: card.hidden, style: card.getAttribute('style')});
    }
    const emptyStyle = empty?.getAttribute('style') ?? null;
    const emptyHidden = empty?.hidden ?? true;
    const emptyVisible = Boolean(empty?.classList.contains('is-visible'));
    const listStyle = list?.getAttribute('style') ?? null;
    const removesWholeList = Boolean(list) && cards.length === list.children.length;
    const plan = beginMessageDialogResize();
    if (!plan) return null;
    for (const card of cards) {
        card.hidden = true;
        card.style.display = 'none';
    }
    if (list && removesWholeList) list.style.display = 'none';
    if (empty) {
        empty.hidden = !showEmpty;
        empty.style.display = showEmpty ? 'block' : 'none';
        empty.style.height = showEmpty ? 'auto' : '0px';
        empty.style.opacity = showEmpty ? '1' : '0';
        empty.classList.toggle('is-visible', showEmpty);
    }
    void plan.dialog.offsetHeight;
    plan.to = plan.dialog.getBoundingClientRect().height;
    for (const snapshot of cardSnapshots) {
        restoreInlineStyle(snapshot.card, snapshot.style);
        snapshot.card.hidden = snapshot.hidden;
    }
    if (list) restoreInlineStyle(list, listStyle);
    if (empty) {
        restoreInlineStyle(empty, emptyStyle);
        empty.hidden = emptyHidden;
        empty.classList.toggle('is-visible', emptyVisible);
    }
    plan.dialog.style.height = `${plan.from}px`;
    return plan;
}

/**
 * Resolve on the next painted animation frame.
 * @returns {Promise<void>} Frame completion.
 */
function waitForAnimationFrame() {
    return new Promise(resolveOnAnimationFrame);
}

/**
 * Schedule a promise resolution on the next animation frame.
 * @param {(value?: unknown) => void} resolve - Promise resolver.
 * @returns {void}
 */
function resolveOnAnimationFrame(resolve) {
    window.requestAnimationFrame(resolve);
}

/**
 * Animate a prepared dialog shell resize and then release its inline height.
 * @param {{dialog: HTMLElement, from: number, to: number, generation: number}|null} plan - Measured resize plan.
 * @param {number} duration - Animation duration in milliseconds.
 * @returns {Promise<void>}
 */
async function animateMessageDialogResize(plan, duration) {
    if (!plan) return;
    if (Math.abs(plan.from - plan.to) < 0.5) {
        if (plan.generation === messageDialogHeightGeneration) plan.dialog.style.height = '';
        return;
    }
    let animation;
    try {
        animation = plan.dialog.animate([
            {height: `${plan.from}px`},
            {height: `${plan.to}px`}
        ], {
            duration,
            easing: 'cubic-bezier(0.4, 0, 0.2, 1)',
            fill: 'forwards'
        });
    } catch (error) {
        console.warn('Failed to animate message dialog resize', error);
        if (plan.generation === messageDialogHeightGeneration) plan.dialog.style.height = '';
        return;
    }
    messageDialogHeightAnimation = animation;
    try {
        await animation.finished;
    } catch (error) {
        if (error?.name !== 'AbortError') console.warn('Message dialog resize did not finish', error);
    }
    if (plan.generation !== messageDialogHeightGeneration) return;
    plan.dialog.style.height = `${plan.to}px`;
    messageDialogHeightAnimation = null;
    try {
        animation.cancel();
    } catch (error) {
        console.warn('Failed to release message dialog resize', error);
    }
    await waitForAnimationFrame();
    if (plan.generation === messageDialogHeightGeneration) plan.dialog.style.height = '';
}

/**
 * Delegate an action decision to its trusted feature module.
 * @param {object} message - Actionable message.
 * @param {string} decision - `accept` or `reject`.
 * @returns {Promise<void>}
 */
async function executeMessageAction(message, decision) {
    const handler = actionHandlers.get(message.action_kind);
    if (!handler) return;
    try {
        if (await handler(message, decision)) {
            message.action_status = decision === 'accept' ? 'accepted' : 'rejected';
            message.acted_at = Date.now();
            if (!message.read_at) {
                message.read_at = message.acted_at;
                setUnreadCount(Math.max(0, unreadCount - 1));
            }
            renderMessages();
        }
    } catch (error) {
        console.error('Failed to execute message action', error);
        showAlert(t('messages.actionFailed'), 'error');
    }
}

/**
 * Fetch and display only the unread badge count.
 * @returns {Promise<void>}
 */
async function refreshUnreadCount() {
    if (!cachedIsLoggedIn) return;
    try {
        const {response, data: payload} = await fetchProto('/api/messages/unread-count', UnreadCountResponse);
        if (!response.ok || !payload) return;
        setUnreadCount(Number(payload.unread_count) || 0);
    } catch (error) {
        console.error('Failed to refresh unread messages', error);
    }
}


/**
 * Update the bell badge with a bounded display value.
 * @param {number} count - Unread message count.
 * @returns {void}
 */
function setUnreadCount(count) {
    unreadCount = Math.max(0, Number.isFinite(count) ? Math.floor(count) : 0);
    const badge = document.getElementById('message-unread-badge');
    if (badge) {
        badge.hidden = unreadCount === 0;
        badge.textContent = unreadCount > 99 ? '99+' : String(unreadCount);
    }
    updateMessageToolbarState();
}

/**
 * Start one-minute unread polling and perform an immediate refresh.
 * @returns {void}
 */
function startPolling() {
    stopPolling();
    void refreshUnreadCount();
    pollTimer = window.setInterval(refreshUnreadCount, 60000);
}

/**
 * Stop unread polling.
 * @returns {void}
 */
function stopPolling() {
    if (pollTimer) window.clearInterval(pollTimer);
    pollTimer = 0;
}

/**
 * React to login/logout state emitted by the auth module.
 * @param {CustomEvent} event - Auth state event.
 * @returns {void}
 */
function handleAuthChanged(event) {
    if (event.detail?.isLoggedIn) {
        startPolling();
    } else {
        stopPolling();
        messages = [];
        nextCursor = '';
        setUnreadCount(0);
        closeMessageCenter();
    }
    updateComposerVisibility();
}

/**
 * Re-render localized labels and feature message text.
 * @returns {void}
 */
function handleLanguageChanged() {
    refreshSeveritySelect();
    syncBroadcastToggleLabel();
    renderMessages();
}

/**
 * Mount the shared custom-select control used by the notification composer.
 * @returns {void}
 */
function initializeSeveritySelect() {
    const host = document.getElementById('message-compose-severity');
    if (!host || severitySelect) return;
    severitySelect = makeCustomSelect(severitySelectOptions(), 'info');
    host.replaceChildren(severitySelect);
}

/**
 * Build localized options for the severity custom select.
 * @returns {Array<{value: string, label: string}>} Severity options.
 */
function severitySelectOptions() {
    return [
        {value: 'info', label: t('messages.severityInfo')},
        {value: 'success', label: t('messages.severitySuccess')},
        {value: 'warning', label: t('messages.severityWarning')},
        {value: 'error', label: t('messages.severityError')}
    ];
}

/**
 * Refresh severity labels after a locale change without losing selection.
 * @returns {void}
 */
function refreshSeveritySelect() {
    if (severitySelect) severitySelect.setOptions(severitySelectOptions(), 'info');
}

/**
 * Mount the shared switch control used for broadcast notifications.
 * @returns {void}
 */
function initializeBroadcastToggle() {
    const host = document.getElementById('message-compose-all');
    if (!host || broadcastToggle) return;
    broadcastToggle = createToggle(false, handleBroadcastToggleChange);
    broadcastToggle.classList.add('message-compose-all-control');
    host.replaceChildren(broadcastToggle);
    syncBroadcastToggleLabel();
}

/**
 * Apply a broadcast switch change to the recipient input.
 * @param {boolean} checked - Current switch state.
 * @returns {void}
 */
function handleBroadcastToggleChange(checked) {
    if (broadcastToggle && broadcastToggle.checked !== checked) {
        broadcastToggle.checked = checked;
    }
    handleBroadcastChange();
}

/**
 * Keep the switch's native checkbox label synchronized with the active locale.
 * @returns {void}
 */
function syncBroadcastToggleLabel() {
    const input = broadcastToggle?.querySelector('input[type="checkbox"]');
    if (input) input.setAttribute('aria-label', t('messages.sendAll'));
}

/**
 * Show or hide the admin compose trigger and close unauthorized composer UI.
 * @returns {void}
 */
function updateComposerVisibility() {
    const toggle = document.getElementById('message-compose-toggle');
    if (toggle) toggle.style.display = cachedIsManager ? 'inline-flex' : 'none';
    if (!cachedIsManager) void hideComposer();
}

/**
 * Toggle the administrator notification composer.
 * @returns {Promise<void>}
 */
async function toggleComposer() {
    if (!cachedIsManager || notificationSending) return;
    if (composerOpen) {
        await hideComposer();
    } else {
        await showComposer();
    }
}

/**
 * Reveal the administrator composer while its measured height and opacity animate.
 * @returns {Promise<void>}
 */
async function showComposer() {
    const region = document.getElementById('message-compose-region');
    const form = document.getElementById('message-compose-form');
    const toggle = document.getElementById('message-compose-toggle');
    if (!region) return;
    composerOpen = true;
    toggle?.setAttribute('aria-expanded', 'true');
    const resizePlan = prepareComposerDialogResize(region, true);
    const expansion = expandElement(region, {
        duration: 280,
        easing: 'cubic-bezier(0.4, 0, 0.2, 1)'
    });
    const contentAnimation = animateComposerContent(form, true);
    const dialogAnimation = animateMessageDialogResize(resizePlan, 280);
    await Promise.all([expansion, contentAnimation, dialogAnimation]);
    if (composerOpen) document.getElementById('message-compose-recipients')?.focus();
}

/**
 * Hide and reset the administrator notification composer.
 * @returns {Promise<void>}
 */
async function hideComposer() {
    const region = document.getElementById('message-compose-region');
    const form = document.getElementById('message-compose-form');
    const toggle = document.getElementById('message-compose-toggle');
    composerOpen = false;
    toggle?.setAttribute('aria-expanded', 'false');
    if (notificationCompletionTimer) {
        window.clearTimeout(notificationCompletionTimer);
        notificationCompletionTimer = 0;
    }
    hideRecipientSuggestions();
    const resizePlan = region ? prepareComposerDialogResize(region, false) : null;
    const collapse = collapseElement(region, {
        duration: 240,
        easing: 'cubic-bezier(0.4, 0, 0.2, 1)',
        marginTop: false
    });
    const contentAnimation = animateComposerContent(form, false);
    const dialogAnimation = animateMessageDialogResize(resizePlan, 240);
    await Promise.all([collapse, contentAnimation, dialogAnimation]);
    if (composerOpen || !form) return;
    form.reset();
    severitySelect?.setValue('info');
    if (broadcastToggle) broadcastToggle.checked = false;
    handleBroadcastChange();
    if (!notificationSending) setNotificationSubmitState('idle');
}

/**
 * Animate composer content independently from its measured-height wrapper.
 * @param {HTMLElement|null} form - Notification form.
 * @param {boolean} opening - Whether the form is entering.
 * @returns {Promise<void>}
 */
async function animateComposerContent(form, opening) {
    if (!form || prefersReducedMotion() || typeof form.animate !== 'function') return;
    if (composerContentAnimation) {
        try {
            composerContentAnimation.cancel();
        } catch (error) {
            console.warn('Failed to cancel composer content animation', error);
        }
        composerContentAnimation = null;
    }
    const keyframes = opening
        ? [
            {opacity: 0, transform: 'translateY(-10px) scale(0.985)'},
            {opacity: 1, transform: 'translateY(0) scale(1)'}
        ]
        : [
            {opacity: 1, transform: 'translateY(0) scale(1)'},
            {opacity: 0, transform: 'translateY(-8px) scale(0.985)'}
        ];
    const animation = form.animate(keyframes, {
        duration: opening ? 280 : 240,
        easing: 'cubic-bezier(0.4, 0, 0.2, 1)',
        fill: 'both'
    });
    composerContentAnimation = animation;
    try {
        await animation.finished;
    } catch (error) {
        if (error?.name !== 'AbortError') console.warn('Composer animation did not finish', error);
    }
    if (composerContentAnimation !== animation) return;
    composerContentAnimation = null;
    try {
        animation.cancel();
    } catch (error) {
        console.warn('Failed to release composer content animation', error);
    }
}

/**
 * Disable explicit recipients while broadcast delivery is selected.
 * @returns {void}
 */
function handleBroadcastChange() {
    const recipients = document.getElementById('message-compose-recipients');
    if (!recipients) return;
    recipients.disabled = Boolean(broadcastToggle?.checked);
    if (recipients.disabled) hideRecipientSuggestions();
}

/**
 * Debounce manager-only recipient prefix searches as the current token changes.
 * @returns {void}
 */
function handleRecipientInput() {
    hideRecipientSuggestions();
    const input = document.getElementById('message-compose-recipients');
    if (!cachedIsManager || !input || input.disabled || !currentRecipientQuery(input)) return;
    recipientSuggestionTimer = window.setTimeout(fetchRecipientSuggestions, 160);
}

/**
 * Resolve the token currently being typed at the input caret.
 * @param {HTMLInputElement} input - Recipient text field.
 * @returns {string} Lowercase username prefix.
 */
function currentRecipientQuery(input) {
    const caret = Number.isInteger(input.selectionStart) ? input.selectionStart : input.value.length;
    let start = caret;
    while (start > 0 && !isRecipientSeparator(input.value[start - 1])) start--;
    return input.value.slice(start, caret).trim().toLowerCase();
}

/**
 * Identify separators accepted by the notification recipient parser.
 * @param {string} character - One input character.
 * @returns {boolean} Whether the character separates usernames.
 */
function isRecipientSeparator(character) {
    return character === ',' || /\s/u.test(character);
}

/**
 * Fetch a bounded username prefix page and discard stale responses.
 * @returns {Promise<void>}
 */
async function fetchRecipientSuggestions() {
    recipientSuggestionTimer = 0;
    const input = document.getElementById('message-compose-recipients');
    const query = input ? currentRecipientQuery(input) : '';
    if (!cachedIsManager || !input || input.disabled || !query) return;
    const version = recipientSuggestionVersion;
    try {
        const {response, data: payload} = await fetchProto(`/api/messages/admin/users?q=${encodeURIComponent(query)}`, UserSearchResponse);
        if (!response.ok) throw new Error(await responseError(response));
        if (version !== recipientSuggestionVersion || query !== currentRecipientQuery(input)) return;
        renderRecipientSuggestions(Array.isArray(payload?.users) ? payload.users : []);
    } catch (error) {
        if (version !== recipientSuggestionVersion) return;
        console.error('Failed to suggest notification recipients', error);
        hideRecipientSuggestions();
    }
}


/**
 * Render username suggestions using the application's dropdown item style.
 * @param {Array<string>} users - Suggested usernames.
 * @returns {void}
 */
function renderRecipientSuggestions(users) {
    const dropdown = document.getElementById('message-recipient-suggestions');
    const input = document.getElementById('message-compose-recipients');
    if (!dropdown || !input) return;
    recipientSuggestions = [];
    dropdown.innerHTML = '';
    for (const rawUser of users) {
        const username = String(rawUser || '');
        if (!username) continue;
        recipientSuggestions.push(username);
        dropdown.appendChild(el('button', {
            type: 'button',
            class: 'custom-select-dropdown-item message-recipient-suggestion',
            role: 'option',
            'aria-selected': 'false',
            'data-recipient-suggestion': username
        }, el('span', {class: 'custom-select-item-text'}, username)));
    }
    activeRecipientSuggestion = -1;
    const visible = recipientSuggestions.length > 0;
    dropdown.style.display = visible ? 'block' : 'none';
    input.setAttribute('aria-expanded', String(visible));
}

/**
 * Navigate or accept the visible recipient suggestion list from the keyboard.
 * @param {KeyboardEvent} event - Recipient input key event.
 * @returns {void}
 */
function handleRecipientKeydown(event) {
    const dropdown = document.getElementById('message-recipient-suggestions');
    if (!dropdown || dropdown.style.display !== 'block' || recipientSuggestions.length === 0) return;
    if (event.key === 'ArrowDown') {
        event.preventDefault();
        setActiveRecipientSuggestion(activeRecipientSuggestion + 1);
    } else if (event.key === 'ArrowUp') {
        event.preventDefault();
        const previousIndex = activeRecipientSuggestion < 0 ? recipientSuggestions.length - 1 : activeRecipientSuggestion - 1;
        setActiveRecipientSuggestion(previousIndex);
    } else if (event.key === 'Enter') {
        event.preventDefault();
        const selectedIndex = activeRecipientSuggestion < 0 ? 0 : activeRecipientSuggestion;
        insertRecipientSuggestion(recipientSuggestions[selectedIndex]);
    } else if (event.key === 'Escape') {
        event.preventDefault();
        hideRecipientSuggestions();
    }
}

/**
 * Move the active option with wraparound and keep it visible.
 * @param {number} requestedIndex - Unbounded next option index.
 * @returns {void}
 */
function setActiveRecipientSuggestion(requestedIndex) {
    if (recipientSuggestions.length === 0) return;
    activeRecipientSuggestion = (requestedIndex + recipientSuggestions.length) % recipientSuggestions.length;
    const items = document.querySelectorAll('[data-recipient-suggestion]');
    let index = 0;
    for (const item of items) {
        const selected = index === activeRecipientSuggestion;
        item.classList.toggle('is-selected', selected);
        item.setAttribute('aria-selected', String(selected));
        if (selected) item.scrollIntoView({block: 'nearest'});
        index++;
    }
}

/**
 * Accept a clicked username suggestion.
 * @param {MouseEvent} event - Suggestion list click event.
 * @returns {void}
 */
function handleRecipientSuggestionClick(event) {
    const item = event.target.closest('[data-recipient-suggestion]');
    if (item) insertRecipientSuggestion(item.dataset.recipientSuggestion || '');
}

/**
 * Replace only the username token at the caret and retain other recipients.
 * @param {string} username - Selected server-returned username.
 * @returns {void}
 */
function insertRecipientSuggestion(username) {
    const input = document.getElementById('message-compose-recipients');
    if (!input || !username) return;
    const caret = Number.isInteger(input.selectionStart) ? input.selectionStart : input.value.length;
    let start = caret;
    let end = caret;
    while (start > 0 && !isRecipientSeparator(input.value[start - 1])) start--;
    while (end < input.value.length && !isRecipientSeparator(input.value[end])) end++;
    const prefix = input.value.slice(0, start).replace(/[\s,]+$/u, '');
    const suffix = input.value.slice(end).replace(/^[\s,]+/u, '');
    let value = prefix ? `${prefix}, ${username}` : username;
    const nextCaret = value.length + 2;
    value += suffix ? `, ${suffix}` : ', ';
    input.value = value.slice(0, input.maxLength);
    input.focus();
    input.setSelectionRange(Math.min(nextCaret, input.value.length), Math.min(nextCaret, input.value.length));
    hideRecipientSuggestions();
}

/**
 * Hide autocomplete when a click lands outside its input and menu.
 * @param {MouseEvent} event - Document click event.
 * @returns {void}
 */
function handleRecipientDocumentClick(event) {
    const wrapper = document.querySelector('.message-recipient-input-wrap');
    if (wrapper && !wrapper.contains(event.target)) hideRecipientSuggestions();
}

/**
 * Clear pending autocomplete work and reset combobox accessibility state.
 * @returns {void}
 */
function hideRecipientSuggestions() {
    if (recipientSuggestionTimer) window.clearTimeout(recipientSuggestionTimer);
    recipientSuggestionTimer = 0;
    recipientSuggestionVersion++;
    recipientSuggestions = [];
    activeRecipientSuggestion = -1;
    const dropdown = document.getElementById('message-recipient-suggestions');
    const input = document.getElementById('message-compose-recipients');
    if (dropdown) {
        dropdown.style.display = 'none';
        dropdown.innerHTML = '';
    }
    if (input) input.setAttribute('aria-expanded', 'false');
}

/**
 * Parse the same comma-or-whitespace recipient syntax accepted by the server.
 * @param {string} value - Raw recipient field value.
 * @returns {Array<string>} Non-empty recipient names.
 */
function parseRecipientNames(value) {
    const recipients = [];
    for (const candidate of value.split(/[\s,]+/u)) {
        if (candidate) recipients.push(candidate);
    }
    return recipients;
}

/**
 * Apply visible busy and success feedback to the send action.
 * @param {'idle'|'sending'|'success'} state - Current submission state.
 * @returns {void}
 */
function setNotificationSubmitState(state) {
    const submit = document.getElementById('message-compose-submit');
    const cancel = document.getElementById('message-compose-cancel');
    const toggle = document.getElementById('message-compose-toggle');
    const form = document.getElementById('message-compose-form');
    const sending = state === 'sending';
    if (submit) {
        submit.disabled = sending;
        submit.classList.toggle('is-sending', sending);
        submit.classList.toggle('is-success', state === 'success');
        submit.setAttribute('aria-busy', String(sending));
    }
    if (cancel) cancel.disabled = sending;
    if (toggle) toggle.disabled = sending;
    broadcastToggle?.toggleAttribute('disabled', sending);
    if (form) form.classList.toggle('is-sent', state === 'success');
}

/**
 * Finish the success animation before resetting and closing the composer.
 * @returns {void}
 */
function completeSuccessfulNotification() {
    notificationCompletionTimer = 0;
    void hideComposer();
}

/**
 * Submit a plain-text administrator notification.
 * @param {SubmitEvent} event - Compose form submission.
 * @returns {Promise<void>}
 */
async function submitNotification(event) {
    event.preventDefault();
    if (!cachedIsManager || notificationSending) return;
    const all = Boolean(broadcastToggle?.checked);
    const recipientsText = document.getElementById('message-compose-recipients')?.value || '';
    const recipients = parseRecipientNames(recipientsText);
    const severity = severitySelect?.getValue() || 'info';
    const title = document.getElementById('message-compose-title')?.value.trim() || '';
    const body = document.getElementById('message-compose-body')?.value.trim() || '';
    if ((!all && recipients.length === 0) || !title || !body) {
        showAlert(t('messages.invalidNotification'), 'error');
        return;
    }
    notificationSending = true;
    setNotificationSubmitState('sending');
    try {
        const {response, data: payload} = await postProto(
            '/api/messages/admin',
            SendNotificationRequest,
            {all, recipients, severity, title, body},
            SendNotificationResponse
        );
        if (!response.ok) throw new Error(await responseError(response));
        notificationSending = false;
        setNotificationSubmitState('success');
        showAlert(t('messages.sent', {count: Number(payload?.sent) || 0}), 'success');
        notificationCompletionTimer = window.setTimeout(completeSuccessfulNotification, 650);
    } catch (error) {
        notificationSending = false;
        setNotificationSubmitState('idle');
        console.error('Failed to send notification', error);
        showAlert(t('messages.sendFailed'), 'error');
    }
}


/**
 * Extract a bounded error string from an API response.
 * @param {Response} response - Failed response.
 * @returns {Promise<string>} Error text.
 */
async function responseError(response) {
    const text = await response.text();
    return text.slice(0, 512) || `HTTP ${response.status}`;
}
