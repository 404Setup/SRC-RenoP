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
import {createIcon, createUserIdentity, RenopDialog, runButtonAction} from './components.js';
import {t} from './i18n.js';
import {caughtErrorMessage, localizedResponseError} from './response-errors.js';
import {RepositoryUserSuggestions} from './browser/user-suggestions.js';
import {SUPER_TEAM_ERROR_KEYS} from './super-team-errors.js';
import {exitProtectedRouteOnDenial} from './protected-route.js';
import {createPublicationQuotaPanel, openPublicationQuotaDialog} from './publication-quota.js';

const routeRoot = '/account/teams';
const publicRouteRoot = '/team';
const pageSize = 12;
let loadGeneration = 0;
let listOffset = 0;
let activePrefix = '';

/**
 * Validate the immutable ASCII prefix accepted by every package engine.
 * @param {string} prefix - Candidate prefix.
 * @returns {boolean} Whether the prefix follows the shared contract.
 */
function validPrefix(prefix) {
    return prefix.length >= 2 && prefix.length <= 64 &&
        /^[a-z0-9](?:[a-z0-9_-]*[a-z0-9])$/.test(prefix);
}

const userSuggestions = new RepositoryUserSuggestions({
    id: 'super-team-user-suggestions',
    async fetchUsers(query) {
        if (!activePrefix) return [];
        const response = await apiRequest(`/api/super-teams/${encodeURIComponent(activePrefix)}/users/search?q=${encodeURIComponent(query)}`);
        if (!response.ok) throw await localizedResponseError(response, 'superTeam.searchFailed', {}, SUPER_TEAM_ERROR_KEYS);
        const payload = await response.json();
        return Array.isArray(payload.users) ? payload.users : [];
    },
    onError(error) {
        console.error('Failed to search global team users', error);
    },
});

/**
 * Parse a global-team account route.
 * @param {string} [pathname=window.location.pathname] - Candidate pathname.
 * @returns {{prefix: string}|null} Parsed route or null.
 */
export function superTeamRouteFromPath(pathname = window.location.pathname) {
    const normalized = String(pathname || '/').replace(/\/+$/, '') || '/';
    if (normalized === routeRoot) return {prefix: ''};
    if (!normalized.startsWith(`${routeRoot}/`)) return null;
    const segment = normalized.slice(routeRoot.length + 1);
    if (!segment || segment.includes('/')) return null;
    try {
        const prefix = decodeURIComponent(segment).trim().toLowerCase();
        return validPrefix(prefix) ? {prefix} : null;
    } catch {
        return null;
    }
}

/**
 * Parse a public global-team route.
 * @param {string} [pathname=window.location.pathname] - Candidate pathname.
 * @returns {{prefix: string}|null} Parsed route or null.
 */
export function publicSuperTeamRouteFromPath(pathname = window.location.pathname) {
    const normalized = String(pathname || '/').replace(/\/+$/, '') || '/';
    if (!normalized.startsWith(`${publicRouteRoot}/`)) return null;
    const segment = normalized.slice(publicRouteRoot.length + 1);
    if (!segment || segment.includes('/')) return null;
    try {
        const prefix = decodeURIComponent(segment).trim().toLowerCase();
        return validPrefix(prefix) ? {prefix} : null;
    } catch {
        return null;
    }
}

/** Open the global-team account center. */
export function openSuperTeamCenter() {
    navigate('');
}

/**
 * Navigate inside the global-team account center.
 * @param {string} prefix - Optional immutable team prefix.
 * @returns {void}
 */
function navigate(prefix) {
    const path = prefix ? `${routeRoot}/${encodeURIComponent(prefix)}` : routeRoot;
    if (window.location.pathname !== path || window.location.search || window.location.hash) {
        window.history.pushState(null, '', path);
    }
    window.dispatchEvent(new PopStateEvent('popstate'));
}

/**
 * Navigate through the application shell.
 * @param {string} path - Absolute application path.
 * @returns {void}
 */
function navigateApplicationPath(path) {
    if (!path || !path.startsWith('/')) return;
    if (window.location.pathname !== path || window.location.search || window.location.hash) {
        window.history.pushState(null, '', path);
    }
    window.dispatchEvent(new PopStateEvent('popstate'));
}

/** @returns {HTMLElement|null} Stable account-page content host. */
function contentHost() {
    return document.getElementById('super-team-page-content');
}

/** @returns {HTMLElement|null} Stable public team-page content host. */
function publicContentHost() {
    return document.getElementById('public-super-team-page-content');
}

/**
 * Height-morph one team page to a new set of nodes.
 * @param {HTMLElement|null} host - Content host.
 * @param {...(Node|null|undefined)} nodes - Replacement nodes.
 * @returns {Promise<void>} Animation completion.
 */
async function replaceHostContent(host, ...nodes) {
    if (!host) return;
    await morphElementHeight(host, () => host.replaceChildren(...nodes.filter(Boolean)), {duration: 300});
}

/**
 * Height-morph the account page to a new set of nodes.
 * @param {...(Node|null|undefined)} nodes - Replacement nodes.
 * @returns {Promise<void>} Animation completion.
 */
async function replaceContent(...nodes) {
    await replaceHostContent(contentHost(), ...nodes);
}

/**
 * Format one T1–T4 role.
 * @param {number|string} level - Team role level.
 * @returns {string} Localized role label.
 */
function roleLabel(level) {
    const role = Math.max(1, Math.min(4, Number(level) || 1));
    return `T${role} · ${t(`superTeam.roleT${role}`)}`;
}

/**
 * Build bounded role options.
 * @param {number} [maximum=4] - Highest role offered.
 * @returns {Array<{value: string, label: string}>} Select options.
 */
function roleOptions(maximum = 4) {
    const options = [];
    for (let level = 1; level <= maximum; level++) {
        options.push({value: String(level), label: roleLabel(level)});
    }
    return options;
}

/** @returns {HTMLElement} Loading state. */
function loadingState() {
    return el('div', {class: 'super-team-state'},
        el('div', {class: 'sessions-loading-spinner', 'aria-hidden': 'true'}),
        el('span', {}, t('common.loading'))
    );
}

/** @returns {HTMLElement} Empty team-list state. */
function emptyState() {
    return el('div', {class: 'super-team-state super-team-empty'},
        el('span', {class: 'super-team-empty-icon'}, createIcon('identity')),
        el('strong', {}, t('superTeam.empty')),
        el('span', {}, t('superTeam.emptyHint'))
    );
}

/**
 * Build one quota-style team limit card.
 * @param {string} label - Metric label.
 * @param {number|string} used - Current usage.
 * @param {number|string} limit - Effective limit.
 * @param {boolean} inherited - Whether the global default applies.
 * @returns {HTMLElement} Limit card.
 */
function limitCard(label, used, limit, inherited) {
    const boundedLimit = Math.max(0, Number(limit) || 0);
    const boundedUsed = Math.max(0, Number(used) || 0);
    const percent = boundedLimit > 0 ? Math.min(100, (boundedUsed / boundedLimit) * 100) : 100;
    return el('div', {class: 'super-team-limit-card'},
        el('div', {class: 'super-team-limit-heading'},
            el('span', {}, label),
            inherited ? el('span', {class: 'super-team-inherited'}, t('superTeam.inherited')) : null
        ),
        el('strong', {}, `${boundedUsed} / ${boundedLimit}`),
        el('span', {class: 'super-team-limit-track', 'aria-hidden': 'true'},
            el('span', {style: {width: `${percent}%`}})
        )
    );
}

/**
 * Build one global-team catalog card.
 * @param {object} team - Team summary.
 * @returns {HTMLButtonElement} Navigable card.
 */
function teamCard(team) {
    const prefix = String(team.prefix || '');
    const card = el('button', {
            type: 'button', class: 'super-team-card', onclick: () => navigate(prefix)
        },
        el('span', {class: 'super-team-card-icon', 'aria-hidden': 'true'}, createIcon('identity')),
        el('span', {class: 'super-team-card-main'},
            el('span', {class: 'super-team-card-title'},
                el('strong', {}, team.name || prefix),
                el('code', {}, prefix)
            ),
            el('span', {class: 'super-team-card-description'}, team.description || t('superTeam.noDescription'))
        ),
        el('span', {class: 'super-team-card-meta'},
            team.role_level ? el('span', {class: 'super-team-role-badge'}, roleLabel(team.role_level)) : null,
            el('span', {}, t('superTeam.memberCount', {count: Number(team.member_count) || 0})),
            createIcon('chevron')
        ));
    return card;
}

/**
 * Build server-backed previous/next pagination.
 * @param {number} total - Total visible teams.
 * @returns {HTMLElement|null} Pagination control.
 */
function pager(total) {
    const pages = Math.max(1, Math.ceil(total / pageSize));
    const page = Math.floor(listOffset / pageSize);
    if (pages <= 1) return null;
    const previous = el('button', {
        type: 'button', class: 'renop-pagination-btn', disabled: page === 0,
        onclick: () => {
            listOffset = Math.max(0, listOffset - pageSize);
            void loadList();
        }
    }, t('common.prev'));
    const next = el('button', {
        type: 'button', class: 'renop-pagination-btn', disabled: page >= pages - 1,
        onclick: () => {
            listOffset += pageSize;
            void loadList();
        }
    }, t('common.next'));
    const summary = t('superTeam.pageSummary', {page: page + 1, pages, total});
    return el('nav', {class: 'renop-pagination', 'aria-label': summary},
        previous, el('span', {class: 'renop-pagination-summary'}, summary), next
    );
}

/** @returns {void} */
function openCreateDialog() {
    const prefix = el('input', {class: 'profile-input', maxlength: '64', autocomplete: 'off'});
    const name = el('input', {class: 'profile-input', maxlength: '80', autocomplete: 'off'});
    const description = el('textarea', {
        class: 'profile-input super-team-description-input',
        maxlength: '512',
        rows: '3'
    });
    const form = el('div', {class: 'super-team-dialog-form'},
        el('label', {}, el('span', {}, t('superTeam.prefix')), prefix,
            el('small', {}, t('superTeam.prefixHint'))),
        el('label', {}, el('span', {}, t('superTeam.name')), name),
        el('label', {}, el('span', {}, t('superTeam.description')), description)
    );
    RenopDialog.show({
        id: 'super-team-create-dialog', maxWidth: '560px', icon: 'identity',
        title: t('superTeam.create'), body: form,
        footer: [
            {text: t('common.cancel'), className: 'action-btn', onClick: (event, dialog) => dialog.close(false)},
            {
                text: t('common.create'), className: 'action-btn primary-btn',
                onClick: async (event, dialog) => runButtonAction(event.currentTarget, async () => {
                    const payload = {
                        prefix: prefix.value.trim().toLowerCase(),
                        name: name.value.trim(),
                        description: description.value.trim(),
                    };
                    if (!payload.prefix || !payload.name) {
                        (payload.prefix ? name : prefix).focus();
                        showAlert(t('superTeam.invalidRequest'), 'error');
                        return;
                    }
                    try {
                        const response = await apiRequest('/api/super-teams', {
                            method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify(payload)
                        });
                        if (!response.ok) throw await localizedResponseError(response, 'superTeam.createFailed', {}, SUPER_TEAM_ERROR_KEYS);
                        const created = await response.json();
                        dialog.close(true);
                        showAlert(t('superTeam.created'), 'success');
                        navigate(created.prefix || payload.prefix);
                    } catch (error) {
                        console.error('Failed to create global team', error);
                        showAlert(caughtErrorMessage(error, 'superTeam.createFailed'), 'error');
                    }
                })
            }
        ]
    });
    requestAnimationFrame(() => prefix.focus());
}

/** @returns {Promise<void>} */
async function loadList() {
    const generation = ++loadGeneration;
    activePrefix = '';
    userSuggestions.detach();
    await replaceContent(loadingState());
    try {
        const [teamsResponse, limitsResponse] = await Promise.all([
            apiRequest(`/api/super-teams?limit=${pageSize}&offset=${listOffset}`),
            apiRequest('/api/super-teams/limits'),
        ]);
        if (exitProtectedRouteOnDenial(teamsResponse) || exitProtectedRouteOnDenial(limitsResponse)) return;
        if (!teamsResponse.ok) throw await localizedResponseError(teamsResponse, 'superTeam.loadFailed', {}, SUPER_TEAM_ERROR_KEYS);
        if (!limitsResponse.ok) throw await localizedResponseError(limitsResponse, 'superTeam.loadFailed', {}, SUPER_TEAM_ERROR_KEYS);
        const [payload, limits] = await Promise.all([teamsResponse.json(), limitsResponse.json()]);
        if (generation !== loadGeneration) return;
        const teams = Array.isArray(payload.teams) ? payload.teams : [];
        const total = Math.max(0, Number(payload.total) || 0);
        if (listOffset > 0 && listOffset >= total) {
            listOffset = Math.max(0, (Math.ceil(total / pageSize) - 1) * pageSize);
            await loadList();
            return;
        }
        const toolbar = el('div', {class: 'super-team-toolbar'},
            el('div', {class: 'super-team-limits'},
                limitCard(t('superTeam.createUsage'), limits.created_count, limits.create_limit, limits.create_limit_inherited),
                limitCard(t('superTeam.joinUsage'), limits.joined_count, limits.join_limit, limits.join_limit_inherited)
            ),
            el('button', {
                type: 'button', class: 'pill-btn pill-btn--primary',
                disabled: Number(limits.created_count) >= Number(limits.create_limit),
                onclick: openCreateDialog
            }, createIcon('plus'), el('span', {}, t('superTeam.create')))
        );
        const list = el('div', {class: 'super-team-grid'}, ...(teams.length ? teams.map(teamCard) : [emptyState()]));
        await replaceContent(toolbar, list, pager(total));
    } catch (error) {
        if (generation !== loadGeneration) return;
        if (error?.message === 'Unauthorized') return;
        console.error('Failed to load global teams', error);
        await replaceContent(el('div', {class: 'super-team-state is-error'},
            createIcon('warning'), el('span', {}, caughtErrorMessage(error, 'superTeam.loadFailed'))));
    }
}

/**
 * Open mutable team metadata controls.
 * @param {object} details - Team details response.
 * @returns {void}
 */
function openEditDialog(details) {
    const name = el('input', {class: 'profile-input', maxlength: '80', value: details.team.name || ''});
    const description = el('textarea', {
        class: 'profile-input super-team-description-input',
        maxlength: '512',
        rows: '3',
        value: details.team.description || ''
    });
    RenopDialog.show({
        id: 'super-team-edit-dialog', maxWidth: '560px', icon: 'edit', title: t('superTeam.edit'),
        body: el('div', {class: 'super-team-dialog-form'},
            el('label', {}, el('span', {}, t('superTeam.name')), name),
            el('label', {}, el('span', {}, t('superTeam.description')), description)
        ),
        footer: [
            {text: t('common.cancel'), className: 'action-btn', onClick: (event, dialog) => dialog.close(false)},
            {
                text: t('common.save'), className: 'action-btn primary-btn',
                onClick: async (event, dialog) => runButtonAction(event.currentTarget, async () => {
                    try {
                        const response = await apiRequest(`/api/super-teams/${encodeURIComponent(details.team.prefix)}`, {
                            method: 'PUT', headers: {'Content-Type': 'application/json'},
                            body: JSON.stringify({name: name.value.trim(), description: description.value.trim()})
                        });
                        if (!response.ok) throw await localizedResponseError(response, 'superTeam.updateFailed', {}, SUPER_TEAM_ERROR_KEYS);
                        dialog.close(true);
                        showAlert(t('superTeam.updated'), 'success');
                        await loadDetails(details.team.prefix);
                    } catch (error) {
                        showAlert(caughtErrorMessage(error, 'superTeam.updateFailed'), 'error');
                    }
                })
            }
        ]
    });
}

/**
 * Open a team invitation dialog with shared user suggestions.
 * @param {object} details - Team details response.
 * @returns {void}
 */
function openInviteDialog(details) {
    activePrefix = details.team.prefix;
    const input = el('input', {class: 'profile-input', maxlength: '255', autocomplete: 'off'});
    const maximum = details.administrator || Number(details.team.role_level) >= 4 ? 4 : 2;
    let selectedLevel = '1';
    const roleSelect = makeCustomSelect(roleOptions(maximum), selectedLevel, value => {
        selectedLevel = value;
    });
    RenopDialog.show({
        id: 'super-team-invite-dialog', maxWidth: '520px', icon: 'userPlus', title: t('superTeam.invite'),
        onClose: () => userSuggestions.detach(),
        body: el('div', {class: 'super-team-dialog-form'},
            el('label', {}, el('span', {}, t('superTeam.inviteUser')), input),
            el('label', {}, el('span', {}, t('superTeam.inviteRole')), roleSelect)
        ),
        footer: [
            {text: t('common.cancel'), className: 'action-btn', onClick: (event, current) => current.close(false)},
            {
                text: t('superTeam.invite'), className: 'action-btn primary-btn',
                onClick: async (event, current) => runButtonAction(event.currentTarget, async () => {
                    const username = input.value.trim().toLowerCase();
                    if (!username) {
                        input.focus();
                        showAlert(t('superTeam.userRequired'), 'error');
                        return;
                    }
                    try {
                        const response = await apiRequest(`/api/super-teams/${encodeURIComponent(details.team.prefix)}/members`, {
                            method: 'POST', headers: {'Content-Type': 'application/json'},
                            body: JSON.stringify({users: [username], level: Number(selectedLevel)})
                        });
                        if (!response.ok) throw await localizedResponseError(response, 'superTeam.inviteFailed', {}, SUPER_TEAM_ERROR_KEYS);
                        current.close(true);
                        showAlert(t(details.administrator ? 'superTeam.memberAdded' : 'superTeam.invited'), 'success');
                        await loadDetails(details.team.prefix);
                    } catch (error) {
                        showAlert(caughtErrorMessage(error, 'superTeam.inviteFailed'), 'error');
                    }
                })
            }
        ]
    });
    userSuggestions.attach(input);
    requestAnimationFrame(() => input.focus());
}

/**
 * Persist one member role change.
 * @param {object} details - Team details response.
 * @param {object} member - Target member.
 * @param {number|string} level - Replacement role.
 * @returns {Promise<void>}
 */
async function changeMemberLevel(details, member, level) {
    try {
        const response = await apiRequest(
            `/api/super-teams/${encodeURIComponent(details.team.prefix)}/members/${encodeURIComponent(member.username)}`,
            {method: 'PUT', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({level: Number(level)})}
        );
        if (!response.ok) throw await localizedResponseError(response, 'superTeam.memberUpdateFailed', {}, SUPER_TEAM_ERROR_KEYS);
        showAlert(t('superTeam.memberUpdated'), 'success');
        await loadDetails(details.team.prefix);
    } catch (error) {
        showAlert(caughtErrorMessage(error, 'superTeam.memberUpdateFailed'), 'error');
        await loadDetails(details.team.prefix);
    }
}

/**
 * Confirm and remove one managed member.
 * @param {object} details - Team details response.
 * @param {object} member - Target member.
 * @returns {Promise<void>}
 */
async function removeTeamMember(details, member) {
    if (!await showConfirm(t('superTeam.removeConfirm', {name: member.username}))) return;
    try {
        const response = await apiRequest(
            `/api/super-teams/${encodeURIComponent(details.team.prefix)}/members/${encodeURIComponent(member.username)}`,
            {method: 'DELETE'}
        );
        if (!response.ok) throw await localizedResponseError(response, 'superTeam.memberRemoveFailed', {}, SUPER_TEAM_ERROR_KEYS);
        showAlert(t('superTeam.memberRemoved'), 'success');
        await loadDetails(details.team.prefix);
    } catch (error) {
        showAlert(caughtErrorMessage(error, 'superTeam.memberRemoveFailed'), 'error');
    }
}

/**
 * Open the current member's dedicated team exit dialog.
 * @param {object} details - Team details response.
 * @returns {void}
 */
function openLeaveTeamDialog(details) {
    const prefix = details.team.prefix;
    RenopDialog.show({
        id: 'super-team-leave-dialog', maxWidth: '460px', icon: 'logout',
        title: t('superTeam.leave'),
        body: el('p', {class: 'super-team-leave-copy'}, t('team.leaveConfirm')),
        footer: [
            {text: t('common.cancel'), className: 'action-btn', onClick: (event, current) => current.close(false)},
            {
                text: t('superTeam.leave'), className: 'action-btn primary-btn btn-danger',
                onClick: async (event, current) => runButtonAction(event.currentTarget, async () => {
                    try {
                        const response = await apiRequest(
                            `/api/super-teams/${encodeURIComponent(prefix)}/membership`, {method: 'DELETE'}
                        );
                        if (!response.ok) throw await localizedResponseError(
                            response, 'superTeam.leaveFailed', {}, SUPER_TEAM_ERROR_KEYS
                        );
                        current.close(true);
                        showAlert(t('team.left'), 'success');
                        navigate('');
                    } catch (error) {
                        showAlert(caughtErrorMessage(error, 'superTeam.leaveFailed'), 'error');
                    }
                })
            }
        ]
    });
}

/**
 * Build one responsive team member row.
 * @param {object} details - Team details response.
 * @param {object} member - Member record.
 * @param {object} [options]
 * @param {boolean} [options.readOnly=false] - Suppress account-management controls.
 * @returns {HTMLElement} Member row.
 */
function memberRow(details, member, {readOnly = false} = {}) {
    const actorLevel = Number(details.team.role_level) || 0;
    const memberLevel = Number(member.level) || 1;
    const currentUsername = String(localStorage.getItem('username') || '').toLowerCase();
    const own = String(member.username || '').toLowerCase() === currentUsername;
    const canManage = !readOnly && (details.administrator || actorLevel >= 4 ||
        actorLevel >= 3 && memberLevel < 3);
    const controls = el('div', {class: 'super-team-member-controls'});
    if (canManage && !own) {
        const maximum = details.administrator || actorLevel >= 4 ? 4 : 2;
        controls.appendChild(makeCustomSelect(roleOptions(maximum), String(memberLevel),
            value => void changeMemberLevel(details, member, value)));
    } else {
        controls.appendChild(el('span', {class: 'super-team-role-badge'}, roleLabel(memberLevel)));
    }
    if (canManage && !own) {
        controls.appendChild(el('button', {
            type: 'button', class: 'icon-btn is-danger', title: t('common.remove'),
            ariaLabel: t('common.remove'),
            onclick: () => void removeTeamMember(details, member)
        }, createIcon('delete')));
    }
    return el('div', {class: 'super-team-member-row'},
        el('span', {class: 'super-team-member-name'},
            createUserIdentity(member.username, {avatar: true}),
            el('small', {}, t(`superTeam.roleT${memberLevel}Desc`))
        ),
        controls
    );
}

/**
 * Confirm and delete one global team.
 * @param {object} details - Team details response.
 * @returns {Promise<void>}
 */
async function deleteTeam(details) {
    if (!await showConfirm(t('superTeam.deleteConfirm', {prefix: details.team.prefix}))) return;
    try {
        const response = await apiRequest(`/api/super-teams/${encodeURIComponent(details.team.prefix)}`, {method: 'DELETE'});
        if (!response.ok) throw await localizedResponseError(response, 'superTeam.deleteFailed', {}, SUPER_TEAM_ERROR_KEYS);
        showAlert(t('superTeam.deleted'), 'success');
        navigate('');
    } catch (error) {
        showAlert(caughtErrorMessage(error, 'superTeam.deleteFailed'), 'error');
    }
}

/**
 * Build shared public or account content for one global team.
 * @param {object} details - Team details response.
 * @param {string} prefix - Requested immutable prefix.
 * @param {object} [options]
 * @param {boolean} [options.publicView=false] - Render a read-only public page.
 * @param {object|null} [options.quotaStatus=null] - Authorized quota status for account views.
 * @returns {(HTMLElement|null)[]} Detail content nodes.
 */
function teamDetailContent(details, prefix, {publicView = false, quotaStatus = null} = {}) {
    const team = details.team || {};
    const actorLevel = Number(team.role_level) || 0;
    const canOwn = !publicView && (details.administrator || actorLevel >= 4);
    const canManage = !publicView && (details.administrator || actorLevel >= 3);
    const actions = el('div', {class: 'super-team-detail-actions'});
    if (!publicView && actorLevel > 0 && actorLevel < 4) actions.appendChild(el('button', {
        type: 'button', class: 'pill-btn pill-btn--ghost-danger pill-btn--sm',
        onclick: () => openLeaveTeamDialog(details)
    }, createIcon('logout'), el('span', {}, t('superTeam.leave'))));
    if (canManage) actions.appendChild(el('button', {
        type: 'button', class: 'pill-btn pill-btn--primary pill-btn--sm', onclick: () => openInviteDialog(details)
    }, createIcon('userPlus'), el('span', {}, t('superTeam.invite'))));
    if (canOwn) {
        actions.append(
            el('button', {
                    type: 'button',
                    class: 'pill-btn pill-btn--soft pill-btn--sm',
                    onclick: () => openEditDialog(details)
                },
                createIcon('edit'), el('span', {}, t('common.edit'))),
            el('button', {
                    type: 'button',
                    class: 'pill-btn pill-btn--danger pill-btn--sm',
                    onclick: () => void deleteTeam(details)
                },
                createIcon('delete'), el('span', {}, t('common.delete')))
        );
    }
    const hero = el('section', {class: 'super-team-detail-hero'},
        el('button', {
            type: 'button', class: 'super-team-back',
            onclick: publicView ? () => navigateApplicationPath('/') : () => navigate('')
        }, createIcon('chevronLeft'), el('span', {}, t(publicView ? 'nav.backHome' : 'superTeam.back'))),
        el('div', {class: 'super-team-detail-heading'},
            el('span', {class: 'super-team-detail-icon'}, createIcon('identity')),
            el('div', {}, el('span', {class: 'super-team-prefix'}, team.prefix || prefix),
                el('h2', {}, team.name || prefix),
                el('p', {}, team.description || t('superTeam.noDescription'))),
            actions.childElementCount ? actions : null
        ),
        el('div', {class: 'super-team-facts'},
            actorLevel ? el('span', {}, roleLabel(actorLevel)) : null,
            el('span', {}, t('superTeam.memberCount', {count: Number(team.member_count) || 0})),
            team.created_by
                ? createUserIdentity(team.created_by, {template: 'superTeam.createdBy'})
                : el('span', {}, t('superTeam.createdBy', {name: t('common.unknown')}))
        )
    );
    const members = Array.isArray(details.members) ? details.members : [];
    const memberSection = el('section', {class: 'super-team-members-section'},
        el('header', {}, el('div', {}, el('h3', {}, t('superTeam.members')),
            el('p', {}, t('superTeam.membersHint')))),
        el('div', {class: 'super-team-member-list'},
            ...members.map(member => memberRow(details, member, {readOnly: publicView})))
    );
    const quotaPanel = quotaStatus ? createPublicationQuotaPanel(quotaStatus, {
        editable: Boolean(details.administrator),
        onEdit: () => void openPublicationQuotaDialog({
            ownerType: 'super_team', ownerKey: team.prefix || prefix,
            onSaved: () => void loadDetails(prefix),
        }),
    }) : null;
    return [hero, quotaPanel, memberSection];
}

/**
 * Load and render one global team route.
 * @param {string} prefix - Immutable team prefix.
 * @returns {Promise<void>}
 */
async function loadDetails(prefix) {
    const generation = ++loadGeneration;
    activePrefix = prefix;
    userSuggestions.detach();
    await replaceContent(loadingState());
    try {
        const [response, quotaResponse] = await Promise.all([
            apiRequest(`/api/super-teams/${encodeURIComponent(prefix)}`),
            apiRequest(`/api/publication-quota/super-teams/${encodeURIComponent(prefix)}`),
        ]);
        if (exitProtectedRouteOnDenial(response) || exitProtectedRouteOnDenial(quotaResponse)) return;
        if (!response.ok) throw await localizedResponseError(response, 'superTeam.loadFailed', {}, SUPER_TEAM_ERROR_KEYS);
        if (!quotaResponse.ok) throw await localizedResponseError(quotaResponse, 'publicationQuota.loadFailed');
        const details = await response.json();
        const quotaStatus = await quotaResponse.json();
        if (generation !== loadGeneration) return;
        await replaceContent(...teamDetailContent(details, prefix, {quotaStatus}));
    } catch (error) {
        if (generation !== loadGeneration) return;
        if (error?.message === 'Unauthorized') return;
        console.error('Failed to load global team', error);
        await replaceContent(el('div', {class: 'super-team-state is-error'},
            createIcon('warning'), el('span', {}, caughtErrorMessage(error, 'superTeam.loadFailed'))));
    }
}

/** Load the active global-team account page. */
export async function loadSuperTeamCenterPage() {
    const route = superTeamRouteFromPath();
    if (!route) return;
    if (route.prefix) await loadDetails(route.prefix);
    else await loadList();
}

/**
 * Load the active public global-team page.
 * @returns {Promise<void>}
 */
export async function loadPublicSuperTeamPage() {
    const route = publicSuperTeamRouteFromPath();
    const host = publicContentHost();
    if (!route || !host) return;
    const generation = ++loadGeneration;
    activePrefix = '';
    userSuggestions.detach();
    await replaceHostContent(host, loadingState());
    try {
        const response = await apiRequest(`/api/super-teams/${encodeURIComponent(route.prefix)}`);
        if (!response.ok) throw await localizedResponseError(response, 'superTeam.loadFailed', {}, SUPER_TEAM_ERROR_KEYS);
        const details = await response.json();
        if (generation !== loadGeneration) return;
        await replaceHostContent(host, ...teamDetailContent(details, route.prefix, {publicView: true}));
    } catch (error) {
        if (generation !== loadGeneration) return;
        console.error('Failed to load public global team', error);
        await replaceHostContent(host, el('div', {class: 'super-team-state is-error'},
            createIcon('warning'), el('span', {}, caughtErrorMessage(error, 'superTeam.loadFailed'))));
    }
}

/**
 * Build effective global-team limits for an authorized profile view.
 * @param {object|null|undefined} limits - Limits embedded in an authorized singular profile response.
 * @param {{showManage?: boolean}} [options] - Optional self-service action.
 * @returns {HTMLElement|null} Limits card or null when private limits are unavailable.
 */
export function createProfileSuperTeamLimits(limits, {showManage = false} = {}) {
    if (!limits) return null;
    const body = [
        limitCard(t('superTeam.createUsage'), limits.created_count, limits.create_limit, limits.create_limit_inherited),
        limitCard(t('superTeam.joinUsage'), limits.joined_count, limits.join_limit, limits.join_limit_inherited),
    ];
    if (showManage) body.push(el('button', {
        type: 'button', class: 'pill-btn pill-btn--soft', onclick: openSuperTeamCenter
    }, t('superTeam.openTeams')));
    return el('div', {class: 'profile-settings-section profile-super-team-limits'},
        el('div', {class: 'profile-section-card-header'},
            el('div', {class: 'profile-section-icon', 'aria-hidden': 'true'}, createIcon('identity')),
            el('div', {class: 'profile-section-meta'},
                el('h3', {class: 'profile-section-title'}, t('superTeam.profileLimitsTitle')),
                el('p', {class: 'profile-section-desc'}, t('superTeam.profileLimitsDesc'))
            )
        ),
        el('div', {class: 'profile-section-body super-team-profile-limit-body'}, ...body)
    );
}
