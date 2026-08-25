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
import {apiRequest} from '../api.js';
import {cachedIsLoggedIn, cachedIsManager} from '../auth.js';
import {showAlert, showConfirm} from '../alert.js';
import {createIcon, createSkeleton, createUserIdentity, RenopDialog, runButtonAction} from '../components.js';
import {t} from '../i18n.js';
import {decodePathSegment, encodePathSegment, formatBytes} from './utils.js';
import {copyWithFeedback} from './copy-feedback.js';
import {
    createRepositoryBackButton,
    ensureRepositoryView,
    formatRepositoryTimestamp,
    hideRepositoryView,
    replaceRepositoryView
} from './repository-view.js';

let mavenContainer = null;
let mavenLoadSequence = 0;
let activeNavigate = null;

/**
 * Return the persistent Maven repository view container.
 * @returns {HTMLElement}
 */
function ensureContainer() {
    mavenContainer = ensureRepositoryView(mavenContainer, {id: 'maven-repository-view'});
    return mavenContainer;
}

/** Hide and clear the Maven view. */
export function hideMavenRepositoryView() {
    hideRepositoryView(mavenContainer);
}

/**
 * Format a Maven permission with its explicit level.
 * @param {number|string} level
 * @returns {string}
 */
function permissionLabel(level) {
    const normalized = Math.max(0, Math.min(4, Number(level) || 0));
    return `L${normalized} · ${t(`maven.permissionL${normalized}`)}`;
}

/**
 * Format a timestamp for the current locale.
 * @param {number|string} value
 * @returns {string}
 */
function formatDate(value) {
    return formatRepositoryTimestamp(value, {fallback: t('common.unknown')});
}

/**
 * Copy text with the same icon and toast feedback used by other repository views.
 * @param {HTMLButtonElement} button
 * @param {string} value
 * @returns {Promise<void>}
 */
async function copyText(button, value) {
    try {
        await copyWithFeedback(button, value, {copiedLabel: t('details.copied')});
    } catch {
        showAlert(t('maven.copyFailed'), 'error');
    }
}

/**
 * Create a standard back button for the Maven view.
 * @param {string} path
 * @param {string} label
 * @returns {HTMLButtonElement}
 */
function backButton(path, label) {
    return createRepositoryBackButton({path, label, navigate: activeNavigate, className: 'maven-back-btn'});
}

/**
 * Open the domain creation dialog.
 * @param {string} repository
 * @returns {void}
 */
function openCreateDomainDialog(repository) {
    const input = el('input', {
        type: 'text', class: 'profile-input', maxlength: '253', autocomplete: 'off',
        placeholder: t('maven.domainPlaceholder')
    });
    const hint = el('p', {class: 'maven-dialog-hint'}, t('maven.domainCreateHint'));
    RenopDialog.show({
        id: 'maven-domain-create-dialog',
        maxWidth: '520px',
        icon: 'network',
        title: t('maven.createDomain'),
        subtitle: t('maven.createDomainSubtitle'),
        body: el('div', {class: 'maven-dialog-form'}, input, hint),
        footer: [
            {text: t('common.cancel'), className: 'action-btn', onClick: (event, dialog) => dialog.close(false)},
            {
                text: t('maven.createDomain'), className: 'action-btn primary-btn',
                onClick: async (event, dialog) => {
                    const domain = input.value.trim().toLowerCase();
                    if (!domain) {
                        input.focus();
                        return;
                    }
                    const button = event.currentTarget;
                    await runButtonAction(button, async () => {
                        try {
                            const response = await apiRequest(`/api/maven/repositories/${encodeURIComponent(repository)}/domains`, {
                                method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({domain})
                            });
                            if (!response.ok) {
                                showAlert(await response.text() || t('maven.createDomainFailed'), 'error');
                                return;
                            }
                            const created = await response.json();
                            dialog.close(true);
                            showAlert(t('maven.domainCreated'), 'success');
                            activeNavigate?.(`/${encodePathSegment(repository)}/domains/${encodePathSegment(created.domain)}`);
                        } catch (error) {
                            showAlert(error.message || t('maven.createDomainFailed'), 'error');
                        }
                    });
                }
            }
        ]
    });
    requestAnimationFrame(() => input.focus());
}

/**
 * Build one domain catalog card.
 * @param {string} repository
 * @param {object} domain
 * @returns {HTMLElement}
 */
function domainCard(repository, domain) {
    const status = domain.verified ? 'verified' : 'pending';
    const card = el('button', {
        type: 'button', class: `maven-domain-card is-${status}`,
        onclick: () => activeNavigate?.(`/${encodePathSegment(repository)}/domains/${encodePathSegment(domain.domain)}`)
    },
    el('span', {class: 'maven-domain-card-icon'}, createIcon(domain.verified ? 'success' : 'clock')),
    el('span', {class: 'maven-domain-card-main'},
        el('strong', {}, domain.domain),
        el('span', {}, domain.verified ? t('maven.verified') : t('maven.pendingVerification'))
    ),
    el('span', {class: 'maven-domain-card-meta'},
        el('span', {class: `maven-status-badge is-${status}`}, domain.verified ? t('maven.verified') : t('maven.pending')),
        domain.member ? el('span', {class: 'maven-permission-badge'}, permissionLabel(domain.permission_level)) : null,
        el('span', {class: 'maven-artifact-count'}, t('maven.artifactCount', {count: Number(domain.artifact_count) || 0}))
    ));
    return card;
}

/**
 * Build one Maven artifact card.
 * @param {string} repository
 * @param {object} artifact
 * @returns {HTMLElement}
 */
function artifactCard(repository, artifact) {
    const coordinate = `${artifact.group_id}:${artifact.artifact_id}`;
    return el('button', {
        type: 'button', class: 'maven-artifact-card',
        onclick: () => activeNavigate?.(`/${encodePathSegment(repository)}/packages/${encodePathSegment(artifact.group_id)}/${encodePathSegment(artifact.artifact_id)}`)
    },
    el('span', {class: 'maven-artifact-icon'}, createIcon('filePackage')),
    el('span', {class: 'maven-artifact-main'},
        el('strong', {title: coordinate}, coordinate),
        el('span', {}, artifact.description || t('maven.noDescription'))
    ),
    el('span', {class: 'maven-artifact-meta'},
        artifact.latest_version ? el('code', {}, artifact.latest_version) : null,
        el('span', {}, t('maven.versionCount', {count: Number(artifact.version_count) || 0}))
    ));
}

/**
 * Render the Maven repository landing catalog.
 * @param {HTMLElement} container
 * @param {string} repository
 * @param {number} sequence
 * @returns {Promise<void>}
 */
async function renderCatalog(container, repository, sequence) {
    container.classList.add('is-updating');
    try {
        const [domainResponse, artifactResponse] = await Promise.all([
            apiRequest(`/api/maven/repositories/${encodeURIComponent(repository)}/domains`),
            apiRequest(`/api/maven/repositories/${encodeURIComponent(repository)}/packages?limit=30&offset=0`)
        ]);
        if (sequence !== mavenLoadSequence) return;
        if (!domainResponse.ok || !artifactResponse.ok) throw new Error(t('maven.loadFailed'));
        const domainData = await domainResponse.json();
        const artifactData = await artifactResponse.json();
        const domains = Array.isArray(domainData.domains) ? domainData.domains : [];
        const artifacts = Array.isArray(artifactData.artifacts) ? artifactData.artifacts : [];
        const verifiedCount = domains.filter(domain => domain.verified).length;
        const addButton = cachedIsLoggedIn ? el('button', {
            type: 'button', class: 'pill-btn pill-btn--primary', onclick: () => openCreateDomainDialog(repository)
        }, createIcon('plus'), el('span', {}, t('maven.createDomain'))) : null;
        const hero = el('section', {class: 'maven-hero'},
            el('div', {class: 'maven-hero-heading'},
                el('div', {}, el('span', {class: 'maven-kicker'}, t('maven.kicker')),
                    el('h2', {}, createIcon('fileJava'), el('span', {}, repository))),
                addButton
            ),
            el('p', {}, t('maven.subtitle')),
            el('div', {class: 'maven-stats'},
                el('span', {}, t('maven.totalDomains', {count: domains.length})),
                el('span', {}, t('maven.verifiedDomains', {count: verifiedCount})),
                el('span', {}, t('maven.totalArtifacts', {count: Number(artifactData.total) || 0}))
            )
        );
        const domainList = el('div', {class: 'maven-domain-list'});
        if (domains.length === 0) {
            domainList.appendChild(el('div', {class: 'maven-empty'}, createIcon('network'), el('span', {}, t('maven.noDomains'))));
        } else {
            domains.forEach(domain => domainList.appendChild(domainCard(repository, domain)));
        }
        const artifactList = el('div', {class: 'maven-artifact-list'});
        if (artifacts.length === 0) {
            artifactList.appendChild(el('div', {class: 'maven-empty'}, createIcon('filePackage'), el('span', {}, t('maven.noArtifacts'))));
        } else {
            artifacts.forEach(artifact => artifactList.appendChild(artifactCard(repository, artifact)));
        }
        const content = [
            hero,
            el('section', {class: 'maven-section'}, el('h3', {}, t('maven.domainsTitle')), domainList),
            el('section', {class: 'maven-section'}, el('h3', {}, t('maven.artifactsTitle')), artifactList)
        ];
        await replaceRepositoryView(container, content, {duration: 280, enterDuration: 380});
    } catch (error) {
        if (sequence !== mavenLoadSequence) return;
        await replaceRepositoryView(container,
            el('div', {class: 'maven-error'}, createIcon('alertCircle'), el('span', {}, error.message || t('maven.loadFailed'))),
            {duration: 240, enter: false});
    }
}

/**
 * Render provider-specific verification instructions.
 * @param {object} domain
 * @returns {HTMLElement}
 */
function verificationPanel(domain) {
    const instructionKey = domain.verification_type === 'dns'
        ? 'maven.verifyDnsInstruction'
        : (domain.verification_type === 'github' ? 'maven.verifyGithubInstruction' : 'maven.verifyGitlabInstruction');
    return el('div', {class: 'maven-verification-panel'},
        el('p', {}, t(instructionKey, {target: domain.verification_host})),
        el('div', {class: 'maven-verification-target'},
            el('span', {}, domain.verification_type === 'dns' ? t('maven.dnsRoot') : t('maven.account')),
            el('code', {}, domain.verification_host)
        ),
        el('div', {class: 'maven-verification-code'},
            el('code', {}, domain.verification_code || t('maven.codeHidden')),
            domain.verification_code ? el('button', {
                type: 'button', class: 'maven-icon-btn', title: t('details.copy'),
                onclick: event => copyText(event.currentTarget, domain.verification_code)
            }, createIcon('copy')) : null
        )
    );
}

/**
 * Render domain team controls.
 * @param {HTMLElement} container
 * @param {string} repository
 * @param {object} details
 * @param {number} sequence
 * @returns {HTMLElement|null}
 */
function teamPanel(container, repository, details, sequence) {
    const members = Array.isArray(details.members) ? details.members : [];
    if (members.length === 0) return null;
    const level = Number(details.domain.permission_level) || 0;
    const administrator = Boolean(details.administrator);
    const canManage = administrator || level >= 3;
    const canTransfer = administrator || level === 4;
    const currentUsername = String(localStorage.getItem('username') || '').toLowerCase();
    const list = el('div', {class: 'maven-team-list'});
    members.forEach(member => {
        const memberLevel = Number(member.level) || 0;
        const isSelf = String(member.username || '').toLowerCase() === currentUsername;
        const controls = el('div', {class: 'maven-team-controls'});
        if (canManage && memberLevel < 4) {
            const selector = makeCustomSelect(
                [0, 1, 2, 3].map(value => ({value: String(value), label: permissionLabel(value)}))
                    .concat(canTransfer ? [{value: '4', label: permissionLabel(4)}] : []),
                String(memberLevel),
                async value => {
                    const response = await apiRequest(`/api/maven/repositories/${encodeURIComponent(repository)}/domains/${encodeURIComponent(details.domain.domain)}/members/${encodeURIComponent(member.user_id || member.username)}`, {
                        method: 'PUT', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({level: Number(value)})
                    });
                    if (!response.ok) showAlert(await response.text() || t('maven.updateMemberFailed'), 'error');
                    else await renderDomain(container, repository, details.domain.domain, sequence);
                }
            );
            selector.classList.add('maven-level-select');
            controls.appendChild(selector);
        } else {
            controls.appendChild(el('span', {class: 'maven-permission-badge'}, permissionLabel(memberLevel)));
        }
        if ((canManage && memberLevel < 4) || (isSelf && memberLevel < 4)) {
            controls.appendChild(el('button', {
                type: 'button', class: 'maven-icon-btn is-danger', title: isSelf ? t('team.leave') : t('common.delete'),
                onclick: async () => {
                    if (!(await showConfirm(isSelf ? t('maven.leaveConfirm') : t('maven.removeMemberConfirm', {name: member.username})))) return;
                    const response = await apiRequest(`/api/maven/repositories/${encodeURIComponent(repository)}/domains/${encodeURIComponent(details.domain.domain)}/members/${encodeURIComponent(member.user_id || member.username)}`, {method: 'DELETE'});
                    if (!response.ok) showAlert(await response.text() || t('maven.removeMemberFailed'), 'error');
                    else await renderDomain(container, repository, details.domain.domain, sequence);
                }
            }, createIcon('delete')));
        }
        list.appendChild(el('div', {class: 'maven-team-row'}, createUserIdentity(member.username, {avatar: true}), controls));
    });
    let invite = null;
    if (canManage) {
        const input = el('input', {
            type: 'text', maxlength: '1024', placeholder: t('maven.invitePlaceholder'), autocomplete: 'off'
        });
        let inviteLevel = 1;
        const selector = makeCustomSelect(
            [0, 1, 2, 3].map(value => ({value: String(value), label: permissionLabel(value)}))
                .concat(canTransfer ? [{value: '4', label: permissionLabel(4)}] : []),
            '1', value => { inviteLevel = Number(value); }
        );
        const submit = el('button', {type: 'submit', class: 'pill-btn pill-btn--primary'}, t('maven.invite'));
        invite = el('form', {
            class: 'maven-invite-form', onsubmit: async event => {
                event.preventDefault();
                const users = input.value.split(/[\s,]+/).map(value => value.trim()).filter(Boolean);
                if (users.length === 0) return;
                submit.disabled = true;
                try {
                    const response = await apiRequest(`/api/maven/repositories/${encodeURIComponent(repository)}/domains/${encodeURIComponent(details.domain.domain)}/members`, {
                        method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({users, level: inviteLevel})
                    });
                    if (!response.ok) showAlert(await response.text() || t('maven.inviteFailed'), 'error');
                    else {
                        input.value = '';
                        showAlert(t('maven.inviteSent'), 'success');
                        await renderDomain(container, repository, details.domain.domain, sequence);
                    }
                } catch (error) {
                    console.error('Failed to invite Maven domain members', error);
                    showAlert(t('maven.inviteFailed'), 'error');
                } finally {
                    submit.disabled = false;
                }
            }
        }, input, selector, submit);
    }
    return el('section', {class: 'maven-section'}, el('h3', {}, t('maven.teamTitle')), list, invite);
}

/**
 * Render one Maven domain and its team/catalog.
 * @param {HTMLElement} container
 * @param {string} repository
 * @param {string} domainName
 * @param {number} sequence
 * @returns {Promise<void>}
 */
async function renderDomain(container, repository, domainName, sequence) {
    container.classList.add('is-updating');
    try {
        const [domainResponse, artifactsResponse] = await Promise.all([
            apiRequest(`/api/maven/repositories/${encodeURIComponent(repository)}/domains/${encodeURIComponent(domainName)}`),
            apiRequest(`/api/maven/repositories/${encodeURIComponent(repository)}/packages?domain=${encodeURIComponent(domainName)}&limit=50&offset=0`)
        ]);
        if (sequence !== mavenLoadSequence) return;
        if (!domainResponse.ok) throw new Error(await domainResponse.text() || t('maven.domainLoadFailed'));
        const details = await domainResponse.json();
        const artifactData = artifactsResponse.ok ? await artifactsResponse.json() : {artifacts: []};
        const domain = details.domain;
        const canOwn = details.administrator || Number(domain.permission_level) === 4;
        const actions = el('div', {class: 'maven-domain-actions'});
        if (!domain.verified && canOwn && domain.verification_code) {
            actions.appendChild(el('button', {
                type: 'button', class: 'pill-btn pill-btn--primary', onclick: async event => {
                    const button = event.currentTarget;
                    await runButtonAction(button, async () => {
                        try {
                            const response = await apiRequest(`/api/maven/repositories/${encodeURIComponent(repository)}/domains/${encodeURIComponent(domain.domain)}/verify`, {method: 'POST'});
                            if (!response.ok) {
                                showAlert(t(response.status === 429 ? 'maven.verifyRateLimited' : 'maven.verifyFailed'), 'error');
                            } else {
                                showAlert(t('maven.verifySuccess'), 'success');
                                await renderDomain(container, repository, domain.domain, sequence);
                            }
                        } catch (error) {
                            console.error('Failed to verify Maven domain', error);
                            showAlert(t('maven.verifyFailed'), 'error');
                        }
                    });
                }
            }, createIcon('check'), el('span', {}, t('maven.verifyNow'))));
        }
        if (!domain.verified && cachedIsManager && domain.verification_code) {
            actions.appendChild(el('button', {
                type: 'button', class: 'pill-btn pill-btn--soft', onclick: async event => {
                    const button = event.currentTarget;
                    if (!(await showConfirm(t('maven.forceVerifyConfirm', {domain: domain.domain})))) return;
                    await runButtonAction(button, async () => {
                        try {
                            const response = await apiRequest(`/api/maven/repositories/${encodeURIComponent(repository)}/domains/${encodeURIComponent(domain.domain)}/verify/force`, {method: 'POST'});
                            if (!response.ok) showAlert(await response.text() || t('maven.forceVerifyFailed'), 'error');
                            else {
                                showAlert(t('maven.forceVerifySuccess'), 'success');
                                await renderDomain(container, repository, domain.domain, sequence);
                            }
                        } catch (error) {
                            console.error('Failed to force Maven domain verification', error);
                            showAlert(t('maven.forceVerifyFailed'), 'error');
                        }
                    });
                }
            }, createIcon('warning'), el('span', {}, t('maven.forceVerify'))));
        }
        if (canOwn && Number(domain.artifact_count) === 0) {
            actions.appendChild(el('button', {
                type: 'button', class: 'pill-btn pill-btn--ghost-danger', onclick: async () => {
                    if (!(await showConfirm(t('maven.deleteDomainConfirm', {domain: domain.domain})))) return;
                    const response = await apiRequest(`/api/maven/repositories/${encodeURIComponent(repository)}/domains/${encodeURIComponent(domain.domain)}`, {method: 'DELETE'});
                    if (!response.ok) showAlert(await response.text() || t('maven.deleteDomainFailed'), 'error');
                    else activeNavigate?.(`/${encodePathSegment(repository)}`);
                }
            }, createIcon('delete'), el('span', {}, t('maven.deleteDomain'))));
        }
        const hero = el('section', {class: 'maven-hero'},
            backButton(`/${encodePathSegment(repository)}`, t('maven.backToRepository')),
            el('div', {class: 'maven-hero-heading'},
                el('div', {}, el('span', {class: 'maven-kicker'}, t('maven.domainKicker')),
                    el('h2', {}, createIcon('network'), el('span', {}, domain.domain))),
                actions
            ),
            el('div', {class: 'maven-stats'},
                el('span', {class: `maven-status-badge is-${domain.verified ? 'verified' : 'pending'}`}, domain.verified ? t('maven.verified') : t('maven.pending')),
                Number(domain.permission_level) > 0 ? el('span', {class: 'maven-permission-badge'}, permissionLabel(domain.permission_level)) : null,
                el('span', {}, t('maven.artifactCount', {count: Number(domain.artifact_count) || 0})),
                domain.verified_at ? el('span', {}, t('maven.verifiedAt', {date: formatDate(domain.verified_at)})) : null
            ),
            !domain.verified ? verificationPanel(domain) : null
        );
        const artifacts = Array.isArray(artifactData.artifacts) ? artifactData.artifacts : [];
        const artifactList = el('div', {class: 'maven-artifact-list'});
        if (artifacts.length === 0) artifactList.appendChild(el('div', {class: 'maven-empty'}, t('maven.noDomainArtifacts')));
        else artifacts.forEach(artifact => artifactList.appendChild(artifactCard(repository, artifact)));
        const sections = [hero, el('section', {class: 'maven-section'}, el('h3', {}, t('maven.artifactsTitle')), artifactList)];
        const team = teamPanel(container, repository, details, sequence);
        if (team) sections.push(team);
        await replaceRepositoryView(container, sections, {duration: 280, enter: false});
    } catch (error) {
        if (sequence !== mavenLoadSequence) return;
        await replaceRepositoryView(container, [
            backButton(`/${encodePathSegment(repository)}`, t('maven.backToRepository')),
            el('div', {class: 'maven-error'}, error.message || t('maven.domainLoadFailed'))
        ], {duration: 240, enter: false});
    }
}

/**
 * Open the artifact description editor.
 * @param {HTMLElement} container
 * @param {string} repository
 * @param {object} artifact
 * @param {number} sequence
 * @returns {void}
 */
function openDescriptionEditor(container, repository, artifact, sequence) {
    const textarea = el('textarea', {maxlength: '4000', rows: '8'}, artifact.description || '');
    RenopDialog.show({
        id: 'maven-description-dialog', maxWidth: '680px', icon: 'edit', title: t('maven.editDescription'),
        body: el('div', {class: 'maven-dialog-form'}, textarea),
        footer: [
            {text: t('common.cancel'), className: 'action-btn', onClick: (event, dialog) => dialog.close(false)},
            {
                text: t('common.save'), className: 'action-btn primary-btn', onClick: async (event, dialog) => {
                    const button = event.currentTarget;
                    await runButtonAction(button, async () => {
                        try {
                            const query = new URLSearchParams({group: artifact.group_id, artifact: artifact.artifact_id});
                            const response = await apiRequest(`/api/maven/repositories/${encodeURIComponent(repository)}/package?${query}`, {
                                method: 'PUT', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({description: textarea.value.trim()})
                            });
                            if (!response.ok) showAlert(await response.text() || t('maven.updateDescriptionFailed'), 'error');
                            else {
                                dialog.close(true);
                                await renderArtifact(container, repository, artifact.group_id, artifact.artifact_id, sequence);
                            }
                        } catch (error) {
                            console.error('Failed to update Maven artifact description', error);
                            showAlert(t('maven.updateDescriptionFailed'), 'error');
                        }
                    });
                }
            }
        ]
    });
}

/**
 * Render Maven artifact details and version management.
 * @param {HTMLElement} container
 * @param {string} repository
 * @param {string} groupID
 * @param {string} artifactID
 * @param {number} sequence
 * @returns {Promise<void>}
 */
async function renderArtifact(container, repository, groupID, artifactID, sequence) {
    container.classList.add('is-updating');
    try {
        const query = new URLSearchParams({group: groupID, artifact: artifactID});
        const response = await apiRequest(`/api/maven/repositories/${encodeURIComponent(repository)}/package?${query}`);
        if (sequence !== mavenLoadSequence) return;
        if (!response.ok) throw new Error(await response.text() || t('maven.artifactLoadFailed'));
        const details = await response.json();
        const artifact = details.artifact;
        const versions = Array.isArray(details.versions) ? details.versions : [];
        const canManageVersions = details.administrator || Number(artifact.permission_level) >= 2;
        const coordinate = `${artifact.group_id}:${artifact.artifact_id}:${artifact.latest_version || '<version>'}`;
        const hero = el('section', {class: 'maven-hero'},
            backButton(`/${encodePathSegment(repository)}`, t('maven.backToRepository')),
            el('div', {class: 'maven-hero-heading'},
                el('div', {}, el('span', {class: 'maven-kicker'}, t('maven.artifactKicker')),
                    el('h2', {}, createIcon('filePackage'), el('span', {}, `${artifact.group_id}:${artifact.artifact_id}`))),
                canManageVersions ? el('button', {
                    type: 'button', class: 'pill-btn pill-btn--soft', onclick: () => openDescriptionEditor(container, repository, artifact, sequence)
                }, createIcon('edit'), el('span', {}, t('maven.editDescription'))) : null
            ),
            el('p', {}, artifact.description || t('maven.noDescription')),
            el('div', {class: 'maven-coordinate-box'},
                el('code', {}, coordinate),
                el('button', {
                    type: 'button', class: 'maven-icon-btn', title: t('details.copy'),
                    onclick: event => copyText(event.currentTarget, coordinate)
                }, createIcon('copy'))
            ),
            el('div', {class: 'maven-stats'},
                el('span', {}, artifact.domain),
                el('span', {}, t('maven.versionCount', {count: versions.length})),
                artifact.publisher ? el('span', {}, t('maven.publishedBy'), createUserIdentity(artifact.publisher)) : null
            )
        );
        const versionList = el('div', {class: 'maven-version-list'});
        versions.forEach(version => {
            const actions = el('div', {class: 'maven-version-actions'});
            if (canManageVersions) {
                actions.appendChild(el('button', {
                    type: 'button', class: 'maven-icon-btn is-danger', title: t('maven.deleteVersion'), onclick: async () => {
                        if (!(await showConfirm(t('maven.deleteVersionConfirm', {version: version.version})))) return;
                        const deleteQuery = new URLSearchParams({group: artifact.group_id, artifact: artifact.artifact_id, version: version.version});
                        const deleteResponse = await apiRequest(`/api/maven/repositories/${encodeURIComponent(repository)}/versions?${deleteQuery}`, {method: 'DELETE'});
                        if (!deleteResponse.ok) showAlert(await deleteResponse.text() || t('maven.deleteVersionFailed'), 'error');
                        else await renderArtifact(container, repository, groupID, artifactID, sequence);
                    }
                }, createIcon('delete')));
            }
            versionList.appendChild(el('div', {class: 'maven-version-row'},
                el('div', {class: 'maven-version-main'}, el('code', {}, version.version),
                    el('span', {}, formatDate(version.created_at))),
                el('div', {class: 'maven-version-meta'},
                    version.publisher ? createUserIdentity(version.publisher) : null,
                    el('span', {}, formatBytes(Number(version.size) || 0)), actions)
            ));
        });
        if (versions.length === 0) versionList.appendChild(el('div', {class: 'maven-empty'}, t('maven.noVersions')));
        await replaceRepositoryView(container, [hero,
            el('section', {class: 'maven-section'}, el('h3', {}, t('maven.versionsTitle')), versionList)
        ], {duration: 280, enter: false});
    } catch (error) {
        if (sequence !== mavenLoadSequence) return;
        await replaceRepositoryView(container, [
            backButton(`/${encodePathSegment(repository)}`, t('maven.backToRepository')),
            el('div', {class: 'maven-error'}, error.message || t('maven.artifactLoadFailed'))
        ], {duration: 240, enter: false});
    }
}

/**
 * Render a format-aware Maven repository route.
 * @param {string} path
 * @param {object|null} repoDetails
 * @param {(path: string) => void} navigateToPath
 * @returns {Promise<void>}
 */
export async function renderMavenRepository(path, repoDetails, navigateToPath) {
    const sequence = ++mavenLoadSequence;
    const container = ensureContainer();
    if (!container) return;
    container.hidden = false;
    activeNavigate = navigateToPath;
    const parts = path.split('/').filter(Boolean).map(decodePathSegment);
    const repository = parts[0] || '';
    if (!container.firstElementChild) {
        container.replaceChildren(el('div', {class: 'maven-hero'}, createSkeleton({lines: 3})));
    }
    if (parts[1] === 'domains' && parts[2]) {
        await renderDomain(container, repository, parts.slice(2).join('/'), sequence);
    } else if (parts[1] === 'packages' && parts[2] && parts[3]) {
        await renderArtifact(container, repository, parts[2], parts.slice(3).join('/'), sequence);
    } else {
        await renderCatalog(container, repository, sequence);
    }
}
