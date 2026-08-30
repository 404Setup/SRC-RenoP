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
import {morphElementHeight} from '@renop/ui/height-anim';
import {apiRequest} from './api.js';
import {showAlert, showConfirm} from './alert.js';
import {createIcon, RenopDialog, runButtonAction} from './components.js';
import {t} from './i18n.js';
import {REVIEW_ERROR_KEYS} from './review-errors.js';
import {caughtErrorMessage, localizedResponseError} from './response-errors.js';
import {createSuperTeamBindingField} from './super-team-selector.js';
import {formatTimestamp} from './time.js';
import {exitProtectedRouteOnDenial} from './protected-route.js';

const routeRoot = '/account/reviews';
const pageSize = 15;
const resourceTypes = Object.freeze([
    'docker_image', 'npm_package', 'cargo_package', 'maven_artifact', 'maven_domain'
]);
let loadGeneration = 0;
let pageOffset = 0;
let activeView = 'reviewer';
let activeStatus = 'pending';
const activeTypes = new Set();

/**
 * Send a review request without treating an authorization denial as an invalid browser session.
 * @param {string} url - Review API URL.
 * @param {RequestInit} [options={}] - Fetch options.
 * @returns {Promise<Response>} Review response.
 */
function requestReview(url, options = {}) {
    return apiRequest(url, options, {logoutOnForbidden: false});
}

/**
 * Parse the routed review center path.
 * @param {string} [pathname=window.location.pathname] - Candidate path.
 * @returns {boolean} Whether the path belongs to the review center.
 */
export function reviewRouteFromPath(pathname = window.location.pathname) {
    return (String(pathname || '/').replace(/\/+$/, '') || '/') === routeRoot;
}

/**
 * Open the routed review center in one supported view.
 * @param {'reviewer'|'requested'} [view='reviewer'] - Initial task perspective.
 * @returns {void}
 */
export function openReviewCenter(view = 'reviewer') {
    activeView = view === 'requested' ? 'requested' : 'reviewer';
    pageOffset = 0;
    if (window.location.pathname !== routeRoot || window.location.search || window.location.hash) {
        window.history.pushState(null, '', routeRoot);
    }
    window.dispatchEvent(new PopStateEvent('popstate'));
}

/**
 * Report whether a bound resource may return to personal ownership.
 * @param {string} resourceType - Stable review resource type.
 * @param {string} resourceKey - Canonical resource key.
 * @returns {boolean} Whether transfer-out is allowed.
 */
export function canReturnToPersonalOwnership(resourceType, resourceKey) {
    if (resourceType === 'docker_image') return !String(resourceKey || '').includes('/');
    if (resourceType === 'npm_package') return !String(resourceKey || '').startsWith('@');
    return true;
}

/**
 * Open the shared project or publishing-domain transfer request dialog.
 * @param {object} options - Resource context.
 * @param {string} options.resourceType - Stable review resource type.
 * @param {string} [options.repository=''] - Repository name when applicable.
 * @param {string} options.resourceKey - Canonical resource key.
 * @param {string} options.resourceName - Visible resource name.
 * @param {string} [options.currentTeamPrefix=''] - Current owner or empty for personal ownership.
 * @returns {void}
 */
export function openSuperTeamTransferDialog({
                                                resourceType,
                                                repository = '',
                                                resourceKey,
                                                resourceName,
                                                currentTeamPrefix = '',
                                            }) {
    const transferringOut = Boolean(currentTeamPrefix);
    if (transferringOut && !canReturnToPersonalOwnership(resourceType, resourceKey)) {
        showAlert(t('review.transferRestricted'), 'error');
        return;
    }
    const binding = transferringOut
        ? null
        : createSuperTeamBindingField({minimumRole: 1, includePersonal: false});
    const body = el('div', {class: 'review-transfer-form'},
        el('div', {class: 'review-transfer-resource'},
            createIcon('filePackage'),
            el('div', {}, el('strong', {}, resourceName),
                el('span', {}, t(`review.type.${resourceType}`)))
        ),
        transferringOut
            ? el('p', {class: 'review-transfer-copy'},
                t('review.transferOutHint', {team: currentTeamPrefix}))
            : el('label', {}, el('span', {}, t('review.targetTeam')), binding.element,
                el('small', {}, t('review.transferInHint')))
    );
    RenopDialog.show({
        id: 'super-team-transfer-dialog', maxWidth: '560px', icon: 'refresh',
        title: t(transferringOut ? 'review.transferOut' : 'review.transferIn'),
        body,
        footer: [
            {text: t('common.cancel'), className: 'action-btn', onClick: (event, dialog) => dialog.close(false)},
            {
                text: t('review.submitRequest'), className: 'action-btn primary-btn',
                onClick: async (event, dialog) => runButtonAction(event.currentTarget, async () => {
                    try {
                        if (binding) await binding.ready;
                        const targetTeam = binding?.value() || '';
                        if (!transferringOut && !targetTeam) {
                            showAlert(t('review.selectTeam'), 'error');
                            return;
                        }
                        const response = await requestReview('/api/reviews/super-team-transfers', {
                            method: 'POST', headers: {'Content-Type': 'application/json'},
                            body: JSON.stringify({
                                resource_type: resourceType,
                                repository,
                                resource_key: resourceKey,
                                target_team_prefix: targetTeam,
                            })
                        });
                        if (!response.ok) {
                            throw await localizedResponseError(response, 'review.operationFailed', {}, REVIEW_ERROR_KEYS);
                        }
                        dialog.close(true);
                        showAlert(t('review.requestSubmitted'), 'success');
                    } catch (error) {
                        showAlert(caughtErrorMessage(error, 'review.operationFailed'), 'error');
                    }
                })
            }
        ]
    });
}

/**
 * Build or retrieve the stable review results region.
 * @param {boolean} [refreshToolbar=false] - Whether localized toolbar labels must be rebuilt.
 * @returns {HTMLElement|null} Stable review results host.
 */
function ensureReviewShell(refreshToolbar = false) {
    const host = document.getElementById('review-page-content');
    if (!host) return null;
    let results = host.querySelector(':scope > .review-results');
    if (!results || refreshToolbar) {
        results = el('div', {class: 'review-results'});
        host.replaceChildren(toolbar(), results);
    }
    return results;
}

/** @param {...Node} nodes - Replacement nodes. @returns {Promise<void>} Animation completion. */
async function replaceContent(...nodes) {
    const host = ensureReviewShell();
    if (!host) return;
    const visibleNodes = nodes.filter(node => node !== null && node !== undefined && node !== false);
    await morphElementHeight(host, () => host.replaceChildren(...visibleNodes), {duration: 280});
}

/** @returns {HTMLElement} Review loading surface. */
function loadingState() {
    return el('div', {class: 'review-state'},
        el('div', {class: 'sessions-loading-spinner', 'aria-hidden': 'true'}),
        el('span', {}, t('common.loading')));
}

/** @param {string} status - Stable task status. @returns {string} Localized status. */
function statusLabel(status) {
    return t(`review.status.${status}`);
}

/** @param {object} task - Review task. @returns {string} Localized transfer direction. */
function directionLabel(task) {
    if (task.kind === 'publication') {
        if (task.resource_version === '@create') {
            if (task.review_team_prefix) {
                return t('review.teamCreationApproval', {team: task.review_team_prefix});
            }
            if (task.target_team_prefix) return t('review.repositoryCreationApproval');
            return t('review.packageCreation');
        }
        return t('review.publicationVersion', {version: task.resource_version});
    }
    return task.target_team_prefix
        ? t('review.directionIn', {team: task.target_team_prefix})
        : t('review.directionOut', {team: task.source_team_prefix});
}

/**
 * Submit a task decision and refresh the active page.
 * @param {object} task - Pending review task.
 * @param {string} decision - `approved` or `rejected`.
 * @param {string} [reason=''] - Optional decision reason.
 * @param {string} [reasonCode=''] - Optional stable publication rejection reason.
 * @returns {Promise<void>} Completion.
 */
async function submitDecision(task, decision, reason = '', reasonCode = '') {
    const response = await requestReview(`/api/reviews/${encodeURIComponent(task.id)}/decision`, {
        method: 'POST', headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({decision, reason, reason_code: reasonCode})
    });
    if (!response.ok) throw await localizedResponseError(response, 'review.operationFailed', {}, REVIEW_ERROR_KEYS);
    const result = await response.json();
    const publication = task.kind === 'publication';
    const creation = publication && task.resource_version === '@create';
    const resultKey = creation && decision === 'approved' && result?.status === 'pending'
        ? 'review.creationForwarded'
        : creation
            ? decision === 'approved' ? 'review.creationApproved' : 'review.creationRejected'
            : publication
                ? decision === 'approved' ? 'review.publicationApproved' : 'review.publicationRejected'
                : decision === 'approved' ? 'review.approved' : 'review.rejected';
    showAlert(t(resultKey), 'success');
    await loadTasks();
}

/** @param {object} task - Pending task. @returns {void} */
function openRejectDialog(task) {
    const reason = el('textarea', {
        class: 'profile-input',
        maxlength: task.kind === 'publication' ? '505' : '512',
        rows: '4'
    });
    const publication = task.kind === 'publication';
    const creation = publication && task.resource_version === '@create';
    let reasonCode = publication ? 'invalid_metadata' : '';
    const reasonField = el('label', {class: 'review-reject-field'},
        el('span', {}, t('review.rejectReason')), reason);
    if (publication) reasonField.hidden = true;
    const reasonSelect = publication ? makeCustomSelect([
        'invalid_metadata', 'quality', 'policy_violation', 'copyright', 'malware', 'custom'
    ].map(value => ({value, label: t(`review.rejectPreset.${value}`)})), reasonCode, value => {
        reasonCode = value;
        reasonField.hidden = value !== 'custom';
        if (!reasonField.hidden) requestAnimationFrame(() => reason.focus());
    }) : null;
    RenopDialog.show({
        id: 'review-reject-dialog', maxWidth: '520px', icon: 'warning',
        title: t(creation ? 'review.rejectCreationTitle' : publication ? 'review.rejectPublicationTitle' : 'review.rejectTitle'),
        body: el('div', {class: 'review-reject-form'},
            publication ? el('label', {class: 'review-reject-field'},
                el('span', {}, t('review.rejectPresetLabel')), reasonSelect) : null,
            reasonField),
        footer: [
            {text: t('common.cancel'), className: 'action-btn', onClick: (event, dialog) => dialog.close(false)},
            {
                text: t('review.reject'), className: 'action-btn danger-btn',
                onClick: async (event, dialog) => runButtonAction(event.currentTarget, async () => {
                    if ((!publication || reasonCode === 'custom') && !reason.value.trim()) {
                        reason.focus();
                        showAlert(t('review.reasonRequired'), 'error');
                        return;
                    }
                    try {
                        await submitDecision(task, 'rejected', reason.value.trim(), reasonCode);
                        dialog.close(true);
                    } catch (error) {
                        showAlert(caughtErrorMessage(error, 'review.operationFailed'), 'error');
                    }
                })
            }
        ]
    });
    if (!publication) requestAnimationFrame(() => reason.focus());
}

/**
 * Download one review file with two retries after the initial attempt.
 * @param {object} task - Publication review task.
 * @param {object} file - Review file metadata.
 * @returns {Promise<Uint8Array>} File bytes.
 */
async function downloadReviewFile(task, file) {
    const url = `/api/reviews/${encodeURIComponent(task.id)}/files/${encodeURIComponent(file.id)}`;
    let lastError = null;
    for (let attempt = 0; attempt < 3; attempt += 1) {
        try {
            const response = await requestReview(url);
            if (!response.ok) {
                throw await localizedResponseError(response, 'review.downloadFailed', {}, REVIEW_ERROR_KEYS);
            }
            const bytes = new Uint8Array(await response.arrayBuffer());
            const expectedSize = Math.max(0, Number(file.size) || 0);
            if (expectedSize > 0 && bytes.byteLength !== expectedSize) {
                throw new Error(t('review.downloadFailed'));
            }
            return bytes;
        } catch (error) {
            lastError = error;
        }
    }
    throw lastError || new Error(t('review.downloadFailed'));
}

/**
 * Select an adaptive bounded download concurrency.
 * @param {number} fileCount - Number of files to fetch.
 * @returns {number} Worker count between one and four.
 */
function reviewDownloadConcurrency(fileCount) {
    const connection = navigator.connection || navigator.mozConnection || navigator.webkitConnection;
    const slow = connection?.saveData || ['slow-2g', '2g'].includes(connection?.effectiveType);
    const hardware = Math.max(1, Number(navigator.hardwareConcurrency) || 2);
    return Math.max(1, Math.min(fileCount, slow ? 1 : hardware >= 4 ? 4 : 2));
}

/**
 * Fetch review files through a bounded cooperative worker pool.
 * @param {object} task - Publication review task.
 * @param {object[]} files - Review file metadata.
 * @returns {Promise<{entries: Map<string, Uint8Array>, failed: object[]}>} Download result.
 */
async function fetchReviewFiles(task, files) {
    const entries = new Map();
    const failed = [];
    let cursor = 0;
    const worker = async () => {
        while (cursor < files.length) {
            const file = files[cursor];
            cursor += 1;
            try {
                entries.set(file.path, await downloadReviewFile(task, file));
            } catch {
                failed.push(file);
            }
        }
    };
    await Promise.all(Array.from({length: reviewDownloadConcurrency(files.length)}, worker));
    return {entries, failed};
}

/**
 * Trigger a browser download for one generated blob.
 * @param {Blob} blob - Download payload.
 * @param {string} filename - Suggested filename.
 * @returns {void}
 */
function saveReviewBlob(blob, filename) {
    const url = URL.createObjectURL(blob);
    const anchor = el('a', {href: url, download: filename});
    document.body.appendChild(anchor);
    anchor.click();
    anchor.remove();
    setTimeout(() => URL.revokeObjectURL(url), 30_000);
}

/**
 * Fall back to direct browser downloads for critical files after bounded retries fail.
 * @param {object} task - Publication review task.
 * @param {object[]} files - Critical files.
 * @returns {void}
 */
function triggerCriticalReviewDownloads(task, files) {
    for (const file of files) {
        const anchor = el('a', {
            href: `/api/reviews/${encodeURIComponent(task.id)}/files/${encodeURIComponent(file.id)}`,
            download: file.name || 'artifact'
        });
        document.body.appendChild(anchor);
        anchor.click();
        anchor.remove();
    }
}

/**
 * Download all review files and assemble the standard repository layout as a ZIP archive.
 * @param {object} task - Publication review task.
 * @returns {Promise<void>} Completion.
 */
async function downloadPublicationBundle(task) {
    const response = await requestReview(`/api/reviews/${encodeURIComponent(task.id)}/files`);
    if (!response.ok) throw await localizedResponseError(response, 'review.downloadFailed', {}, REVIEW_ERROR_KEYS);
    const payload = await response.json();
    const files = Array.isArray(payload?.files) ? payload.files : [];
    if (files.length === 0) throw new Error(t('review.noFiles'));
    const {entries, failed} = await fetchReviewFiles(task, files);
    if (failed.length > 0) {
        const critical = files.filter(file => file.critical === true);
        triggerCriticalReviewDownloads(task, critical.length > 0 ? critical : files.slice(0, 1));
        throw new Error(t('review.downloadPartial', {count: failed.length}));
    }
    const archiveEntries = Object.fromEntries(entries);
    const {zip} = await import('fflate');
    const archive = await new Promise((resolve, reject) => {
        zip(archiveEntries, {level: 6}, (error, data) => error ? reject(error) : resolve(data));
    });
    const safeName = String(task.resource_name || 'publication').replace(/[^a-zA-Z0-9._-]+/g, '-');
    saveReviewBlob(new Blob([archive], {type: 'application/zip'}),
        `${safeName}-${task.resource_version || 'review'}.zip`);
}

/** @param {object} task - Review task. @returns {HTMLElement} Task card. */
function taskCard(task) {
    const actions = el('div', {class: 'review-card-actions'});
    if (task.kind === 'publication' && task.status === 'pending') {
        actions.appendChild(el('button', {
            type: 'button', class: 'pill-btn pill-btn--soft pill-btn--sm', onclick: event => {
                void runButtonAction(event.currentTarget, async () => {
                    try {
                        await downloadPublicationBundle(task);
                    } catch (error) {
                        showAlert(caughtErrorMessage(error, 'review.downloadFailed'), 'error');
                    }
                });
            }
        }, createIcon('download'), el('span', {}, t('review.downloadBundle'))));
    }
    if (task.status === 'pending' && activeView === 'reviewer') {
        actions.append(
            el('button', {
                type: 'button', class: 'pill-btn pill-btn--primary pill-btn--sm', onclick: async event => {
                    const confirmKey = task.kind === 'publication'
                        ? task.resource_version === '@create'
                            ? 'review.approveCreationConfirm' : 'review.approvePublicationConfirm'
                        : 'review.approveConfirm';
                    if (!await showConfirm(t(confirmKey, {
                        resource: task.resource_name, version: task.resource_version
                    }))) return;
                    await runButtonAction(event.currentTarget, async () => {
                        try {
                            await submitDecision(task, 'approved');
                        } catch (error) {
                            showAlert(caughtErrorMessage(error, 'review.operationFailed'), 'error');
                        }
                    });
                }
            }, createIcon('check'), el('span', {}, t('review.approve'))),
            el('button', {
                type: 'button', class: 'pill-btn pill-btn--danger pill-btn--sm', onclick: () => openRejectDialog(task)
            }, createIcon('close'), el('span', {}, t('review.reject')))
        );
    } else if (task.kind !== 'publication' && task.status === 'pending' && activeView === 'requested') {
        actions.appendChild(el('button', {
            type: 'button', class: 'pill-btn pill-btn--soft pill-btn--sm', onclick: async event => {
                if (!await showConfirm(t('review.cancelConfirm'))) return;
                await runButtonAction(event.currentTarget, async () => {
                    try {
                        const response = await requestReview(`/api/reviews/${encodeURIComponent(task.id)}`, {method: 'DELETE'});
                        if (!response.ok) {
                            throw await localizedResponseError(response, 'review.operationFailed', {}, REVIEW_ERROR_KEYS);
                        }
                        showAlert(t('review.cancelled'), 'success');
                        await loadTasks();
                    } catch (error) {
                        showAlert(caughtErrorMessage(error, 'review.operationFailed'), 'error');
                    }
                });
            }
        }, t('review.cancelRequest')));
    }
    return el('article', {class: 'review-card'},
        el('div', {class: 'review-card-icon'}, createIcon(task.kind === 'publication' ? 'filePackage' : 'refresh')),
        el('div', {class: 'review-card-main'},
            el('div', {class: 'review-card-heading'},
                el('strong', {}, task.resource_name),
                el('span', {class: `review-status is-${task.status}`}, statusLabel(task.status))
            ),
            el('span', {class: 'review-card-type'}, t(`review.type.${task.resource_type}`),
                task.repository ? ` · ${task.repository}` : ''),
            el('p', {}, directionLabel(task)),
            el('div', {class: 'review-card-meta'},
                el('span', {}, t('review.requestedBy', {name: task.requested_by})),
                el('time', {}, formatTimestamp(task.created_at, {fallback: t('common.unknown')})),
                task.decided_by ? el('span', {}, t('review.decidedBy', {name: task.decided_by})) : null,
                task.status === 'rejected' && task.decision_reason
                    ? el('span', {}, task.decision_reason.startsWith('preset:')
                        ? t(`review.rejectPreset.${task.decision_reason.slice(7)}`)
                        : task.decision_reason.startsWith('custom:')
                            ? task.decision_reason.slice(7)
                            : task.decision_reason) : null
            )
        ),
        actions
    );
}

/** @returns {HTMLElement} Filter and view toolbar. */
function toolbar() {
    const viewSelect = makeCustomSelect([
        {value: 'reviewer', label: t('review.assignedToMe')},
        {value: 'requested', label: t('review.requestedByMe')}
    ], activeView, value => {
        activeView = value;
        pageOffset = 0;
        void loadTasks();
    });
    const statusSelect = makeCustomSelect([
        'pending', 'approved', 'rejected', 'cancelled', 'all'
    ].map(value => ({value, label: t(`review.status.${value}`)})), activeStatus, value => {
        activeStatus = value;
        pageOffset = 0;
        void loadTasks();
    });
    const filters = el('div', {class: 'review-type-filters'});
    for (const type of resourceTypes) {
        const button = el('button', {
            type: 'button', class: 'review-filter-chip', 'aria-pressed': activeTypes.has(type),
            onclick: () => {
                if (activeTypes.has(type)) activeTypes.delete(type);
                else activeTypes.add(type);
                button.setAttribute('aria-pressed', String(activeTypes.has(type)));
                pageOffset = 0;
                void loadTasks();
            }
        }, t(`review.type.${type}`));
        filters.appendChild(button);
    }
    return el('div', {class: 'review-toolbar'},
        el('div', {class: 'review-toolbar-selects'}, viewSelect, statusSelect), filters);
}

/** @param {number} total - Total matching tasks. @returns {HTMLElement|null} Pager. */
function pager(total) {
    if (total <= pageSize) return null;
    const page = Math.floor(pageOffset / pageSize);
    const pages = Math.ceil(total / pageSize);
    return el('nav', {class: 'renop-pagination', 'aria-label': t('review.pageSummary', {page: page + 1, pages, total})},
        el('button', {
            type: 'button', class: 'renop-pagination-btn', disabled: page === 0, onclick: () => {
                pageOffset = Math.max(0, pageOffset - pageSize);
                void loadTasks();
            }
        }, t('common.prev')),
        el('span', {class: 'renop-pagination-summary'}, t('review.pageSummary', {page: page + 1, pages, total})),
        el('button', {
            type: 'button', class: 'renop-pagination-btn', disabled: page >= pages - 1, onclick: () => {
                pageOffset += pageSize;
                void loadTasks();
            }
        }, t('common.next'))
    );
}

/**
 * Load and render the active review page.
 * @param {object} [options={}] - Rendering options.
 * @param {boolean} [options.refreshToolbar=false] - Rebuild localized toolbar labels before loading.
 * @returns {Promise<void>} Completion.
 */
async function loadTasks({refreshToolbar = false} = {}) {
    const generation = ++loadGeneration;
    ensureReviewShell(refreshToolbar);
    await replaceContent(loadingState());
    const query = new URLSearchParams({
        view: activeView, status: activeStatus, limit: String(pageSize), offset: String(pageOffset)
    });
    if (activeTypes.size > 0) query.set('types', [...activeTypes].join(','));
    try {
        const response = await requestReview(`/api/reviews?${query}`);
        if (exitProtectedRouteOnDenial(response)) return;
        if (!response.ok) throw await localizedResponseError(response, 'review.loadFailed', {}, REVIEW_ERROR_KEYS);
        const payload = await response.json();
        if (generation !== loadGeneration) return;
        const tasks = Array.isArray(payload?.tasks) ? payload.tasks : [];
        const total = Math.max(0, Number(payload?.total) || 0);
        if (pageOffset >= total && pageOffset > 0) {
            pageOffset = Math.max(0, (Math.ceil(total / pageSize) - 1) * pageSize);
            await loadTasks();
            return;
        }
        const list = tasks.length
            ? el('div', {class: 'review-list'}, ...tasks.map(taskCard))
            : el('div', {class: 'review-state review-empty'}, createIcon('success'),
                el('strong', {}, t('review.empty')), el('span', {}, t('review.emptyHint')));
        await replaceContent(list, pager(total));
    } catch (error) {
        if (generation !== loadGeneration) return;
        if (error?.message === 'Unauthorized') return;
        await replaceContent(el('div', {class: 'review-state is-error'},
            createIcon('warning'), el('span', {}, caughtErrorMessage(error, 'review.loadFailed'))));
    }
}

/** Render the routed review center. */
export async function loadReviewCenterPage() {
    if (!reviewRouteFromPath()) return;
    await loadTasks({refreshToolbar: true});
}
