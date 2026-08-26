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
import {getRepositoryFormat} from '../repository-formats.js';
import {caughtErrorMessage, localizedResponseError, responseErrorMessage} from '../response-errors.js';
import {decodePathSegment, encodePathSegment, formatBytes} from './utils.js';
import {copyWithFeedback} from './copy-feedback.js';
import {
    createRepositoryBackButton,
    createRepositoryFactsSection,
    createRepositoryMirrorBadge,
    ensureRepositoryView,
    formatRepositoryTimestamp,
    hideRepositoryView,
    replaceRepositoryView,
    setRepositoryViewBusy
} from './repository-view.js';

const mavenRepositoryIcon = getRepositoryFormat('maven').icon;
let mavenContainer = null;
let mavenLoadSequence = 0;
let activeNavigate = null;
let domainCenterBody = null;
let domainCenterSequence = 0;
let domainCenterOffset = 0;
const domainCenterPageSize = 12;
const domainCenterFilters = new Set();
const domainCenterRouteRoot = '/account/maven-domains';

/**
 * Parse the account Maven-domain route.
 * @param {string} [pathname=window.location.pathname]
 * @returns {{domain: string}|null}
 */
export function mavenDomainRouteFromPath(pathname = window.location.pathname) {
    const normalized = String(pathname || '/').replace(/\/+$/, '') || '/';
    if (normalized === domainCenterRouteRoot) return {domain: ''};
    if (!normalized.startsWith(`${domainCenterRouteRoot}/`)) return null;
    const segment = normalized.slice(domainCenterRouteRoot.length + 1);
    if (!segment || segment.includes('/')) return null;
    try {
        const domain = decodeURIComponent(segment).trim().toLowerCase();
        return domain ? {domain} : null;
    } catch {
        return null;
    }
}

/**
 * Change the Maven-domain subpage route and let the app shell render it.
 * @param {string} [domain='']
 * @returns {void}
 */
function navigateMavenDomainCenter(domain = '') {
    const path = domain
        ? `${domainCenterRouteRoot}/${encodeURIComponent(String(domain).trim().toLowerCase())}`
        : domainCenterRouteRoot;
    if (window.location.pathname !== path || window.location.search || window.location.hash) {
        window.history.pushState(null, '', path);
    }
    window.dispatchEvent(new PopStateEvent('popstate'));
}

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
 * @param {Function} [onCreated]
 * @returns {void}
 */
function openCreateDomainDialog(onCreated) {
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
                            const response = await apiRequest('/api/maven/domains', {
                                method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({domain})
                            });
                            if (!response.ok) {
                                showAlert(await responseErrorMessage(response, 'maven.createDomainFailed'), 'error');
                                return;
                            }
                            const created = await response.json();
                            dialog.close(true);
                            showAlert(t('maven.domainCreated'), 'success');
                            if (typeof onCreated === 'function') await onCreated(created);
                        } catch (error) {
                            console.error('Failed to create Maven domain', error);
                            showAlert(caughtErrorMessage(error, 'maven.createDomainFailed'), 'error');
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
 * @param {Function} [onSelect]
 * @returns {HTMLElement}
 */
function domainCard(repository, domain, onSelect) {
    const status = domain.verified ? 'verified' : 'pending';
    const card = el('button', {
        type: 'button', class: `maven-domain-card is-${status}`,
        onclick: () => {
            if (typeof onSelect === 'function') onSelect(domain);
            else activeNavigate?.(`/${encodePathSegment(repository)}/domains/${encodePathSegment(domain.domain)}`);
        }
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
        artifact.mirrored ? createRepositoryMirrorBadge(t('common.fromMirror')) : null,
        artifact.latest_version ? el('code', {}, artifact.latest_version) : null,
        el('span', {}, t('maven.versionCount', {count: Number(artifact.version_count) || 0})),
        Number(artifact.total_size) > 0 ? el('span', {}, formatBytes(Number(artifact.total_size))) : null
    ));
}

/**
 * Read and validate a Maven artifact page without treating request failures as an empty catalog.
 * @param {Response} response
 * @returns {Promise<{artifacts: object[], total: number}>}
 */
async function readArtifactPage(response) {
    if (!response.ok) throw new Error(t('maven.loadFailed'));
    const data = await response.json();
    if (!data || !Array.isArray(data.artifacts) || !Number.isFinite(Number(data.total))) {
        throw new Error(t('maven.loadFailed'));
    }
    return data;
}

/**
 * Return a localized Maven domain verification method.
 * @param {string} verificationType
 * @returns {string}
 */
function verificationMethodLabel(verificationType) {
    const labels = {
        dns: 'maven.verificationDns',
        github: 'maven.verificationGithub',
        gitlab: 'maven.verificationGitlab',
        legacy: 'maven.verificationLegacy',
        mirror: 'maven.verificationMirror'
    };
    return t(labels[String(verificationType || '').toLowerCase()] || 'common.unknown');
}

/**
 * Build globally consistent domain facts, plus an optional repository-local artifact count.
 * @param {object} details
 * @param {object} [options={}]
 * @param {string} [options.repository='']
 * @param {number|null} [options.repositoryArtifactCount=null]
 * @returns {HTMLElement}
 */
function domainInformationSection(details, {repository = '', repositoryArtifactCount = null} = {}) {
    const domain = details.domain;
    const canViewGlobalCounts = Boolean(details.administrator || domain.member);
    const access = details.administrator
        ? t('maven.administratorAccess')
        : (domain.member ? permissionLabel(domain.permission_level) : t('maven.readOnlyAccess'));
    const facts = [
        {label: t('maven.domainScope'), value: t('maven.domainScopeGlobal')},
        {label: t('maven.verificationMethod'), value: verificationMethodLabel(domain.verification_type)},
        {label: t('maven.verificationTarget'), value: domain.verification_host, code: true},
        {label: t('maven.domainStatus'), value: domain.verified ? t('maven.verified') : t('maven.pending')},
        {label: t('maven.createdAt'), value: formatDate(domain.created_at)},
        {label: t('maven.verifiedAtLabel'), value: domain.verified_at ? formatDate(domain.verified_at) : null},
        {label: t('maven.lastChecked'), value: domain.last_check_at ? formatDate(domain.last_check_at) : null},
        {label: t('maven.teamMembers'), value: Number(domain.member_count) || 0},
        {label: t('maven.repositoryCount'), value: canViewGlobalCounts ? Number(domain.repository_count) || 0 : null},
        {label: t('maven.globalArtifactCount'), value: canViewGlobalCounts ? Number(domain.artifact_count) || 0 : null},
        {
            label: repository ? t('maven.repositoryArtifactCount', {repository}) : '',
            value: repository && Number.isFinite(Number(repositoryArtifactCount)) ? Number(repositoryArtifactCount) : null
        },
        {label: t('maven.accessLevel'), value: access}
    ];
    return createRepositoryFactsSection(t('maven.domainInformation'), facts);
}

/**
 * Build Maven artifact facts from durable catalog metadata.
 * @param {object} details
 * @param {string} repository
 * @returns {HTMLElement}
 */
function artifactInformationSection(details, repository) {
    const artifact = details.artifact;
    const access = details.administrator
        ? t('maven.administratorAccess')
        : (Number(artifact.permission_level) > 0
            ? permissionLabel(artifact.permission_level)
            : t('maven.readOnlyAccess'));
    return createRepositoryFactsSection(t('maven.artifactInformation'), [
        {label: t('maven.repositoryLabel'), value: repository, code: true},
        {label: t('maven.domainLabel'), value: artifact.domain, code: true},
        {label: t('maven.groupId'), value: artifact.group_id, code: true},
        {label: t('maven.artifactId'), value: artifact.artifact_id, code: true},
        {label: t('maven.latestVersion'), value: artifact.latest_version || t('common.unknown'), code: Boolean(artifact.latest_version)},
        {label: t('maven.versionCountLabel'), value: Number(artifact.version_count) || 0},
        {label: t('maven.fileCount'), value: Number(details.file_count) || 0},
        {label: t('maven.totalSize'), value: formatBytes(Number(details.total_file_size) || Number(artifact.total_size) || 0)},
        {label: t('maven.signedFileCount'), value: Number(details.signed_file_count) || 0},
        {label: t('maven.createdAt'), value: formatDate(artifact.created_at)},
        {label: t('maven.lastUpdated'), value: formatDate(artifact.updated_at)},
        {
            label: t('maven.publisher'),
            value: artifact.publisher ? createUserIdentity(artifact.publisher) : t('common.unknown')
        },
        {label: t('maven.accessLevel'), value: access}
    ]);
}

/**
 * Build a safe external link from published POM metadata.
 * @param {string} value - Absolute project URL.
 * @param {string} [label=value] - Visible label.
 * @returns {HTMLElement} Safe anchor or inert text when the URL is invalid.
 */
function mavenExternalLink(value, label = value) {
    const raw = String(value || '').trim();
    const text = String(label || raw).trim();
    try {
        const parsed = new URL(raw);
        if ((parsed.protocol !== 'https:' && parsed.protocol !== 'http:') || parsed.username || parsed.password) {
            throw new TypeError('Unsupported project URL');
        }
        return el('a', {
            href: parsed.href,
            target: '_blank',
            rel: 'noopener noreferrer',
            title: raw
        }, text || raw);
    } catch {
        return el('span', {}, text || raw);
    }
}

/**
 * Join trusted text or element values into a compact metadata row.
 * @param {Array<Node|string>} values - Values to join.
 * @returns {HTMLElement|null} Joined value wrapper.
 */
function mavenMetadataList(values) {
    const filtered = values.filter(Boolean);
    if (filtered.length === 0) return null;
    const wrapper = el('span', {class: 'maven-metadata-list'});
    filtered.forEach((value, index) => {
        if (index > 0) wrapper.appendChild(document.createTextNode(' · '));
        wrapper.appendChild(value?.nodeType ? value : document.createTextNode(String(value)));
    });
    return wrapper;
}

/**
 * Build rich project facts from the latest indexed POM.
 * @param {object} details - Maven artifact detail payload.
 * @returns {HTMLElement|null} Project metadata section.
 */
function mavenProjectInformationSection(details) {
    const project = details?.project;
    if (!project) return null;
    const parent = project.parent
        ? [project.parent.group_id, project.parent.artifact_id, project.parent.version].filter(Boolean).join(':')
        : '';
    const licenses = mavenMetadataList((project.licenses || []).map(license =>
        license.url
            ? mavenExternalLink(license.url, license.name || license.url)
            : String(license.name || '')
    ));
    const developers = mavenMetadataList((project.developers || []).map(developer => {
        const name = developer.name || developer.id || developer.organization || '';
        return developer.url ? mavenExternalLink(developer.url, name || developer.url) : name;
    }));
    const organization = project.organization_url
        ? mavenExternalLink(project.organization_url, project.organization_name || project.organization_url)
        : project.organization_name;
    const issueTracker = project.issue_management_url
        ? mavenExternalLink(project.issue_management_url,
            project.issue_management_system || project.issue_management_url)
        : project.issue_management_system;
    return createRepositoryFactsSection(t('maven.projectInformation'), [
        {label: t('maven.projectName'), value: project.name},
        {label: t('maven.packaging'), value: project.packaging, code: true},
        {label: t('maven.modelVersion'), value: project.model_version, code: true},
        {label: t('maven.parentProject'), value: parent, code: true},
        {label: t('maven.projectUrl'), value: project.url ? mavenExternalLink(project.url) : null},
        {label: t('maven.organization'), value: organization},
        {label: t('maven.inceptionYear'), value: project.inception_year},
        {label: t('maven.licenses'), value: licenses, wide: true},
        {label: t('maven.scm'), value: project.scm_url ? mavenExternalLink(project.scm_url) : null, wide: true},
        {label: t('maven.issueTracker'), value: issueTracker, wide: true},
        {label: t('maven.developers'), value: developers, wide: true}
    ]);
}

/**
 * Build direct dependency rows from the latest indexed POM.
 * @param {object|null|undefined} project - Project metadata payload.
 * @returns {HTMLElement|null} Dependency section.
 */
function mavenDependencySection(project) {
    if (!project) return null;
    const dependencies = Array.isArray(project.dependencies) ? project.dependencies : [];
    const managedCount = Number(project.managed_dependency_count) || 0;
    if (dependencies.length === 0 && managedCount === 0) return null;
    const list = el('div', {class: 'maven-dependency-list'});
    dependencies.forEach(dependency => {
        const coordinate = [dependency.group_id, dependency.artifact_id].filter(Boolean).join(':');
        const metadata = [
            dependency.version ? el('code', {}, dependency.version) : null,
            dependency.scope ? el('span', {class: 'maven-dependency-badge'}, dependency.scope) : null,
            dependency.type && dependency.type !== 'jar'
                ? el('span', {class: 'maven-dependency-badge'}, dependency.type)
                : null,
            dependency.classifier ? el('span', {class: 'maven-dependency-badge'}, dependency.classifier) : null,
            dependency.optional ? el('span', {class: 'maven-dependency-badge is-optional'}, t('maven.optionalDependency')) : null
        ].filter(Boolean);
        list.appendChild(el('div', {class: 'maven-dependency-row'},
            el('code', {class: 'maven-dependency-coordinate', title: coordinate}, coordinate),
            el('div', {class: 'maven-dependency-meta'}, ...metadata)
        ));
    });
    return el('section', {class: 'maven-section'},
        el('h3', {}, t('maven.dependenciesTitle')),
        el('p', {class: 'maven-section-summary'}, t('maven.dependenciesSummary', {
            count: dependencies.length,
            managed: managedCount
        })),
        list,
        project.dependencies_truncated
            ? el('p', {class: 'maven-section-note'}, t('maven.dependenciesTruncated'))
            : null
    );
}

/**
 * Build an expandable primary-file list for one Maven version.
 * @param {object} version - Version metadata payload.
 * @returns {HTMLElement|null} File disclosure.
 */
function mavenVersionFiles(version) {
    const files = Array.isArray(version.files) ? version.files : [];
    if (files.length === 0) return null;
    const list = el('div', {class: 'maven-version-file-list'});
    files.forEach(file => {
        const integrity = [
            file.signed ? t('maven.fileSigned') : '',
            ...(Array.isArray(file.checksums) ? file.checksums : [])
        ].filter(Boolean).join(' · ');
        list.appendChild(el('div', {class: 'maven-version-file-row'},
            el('div', {class: 'maven-version-file-name'},
                createIcon(file.extension === 'pom' ? 'fileCode' : 'filePackage'),
                el('code', {title: file.name}, file.name)
            ),
            el('div', {class: 'maven-version-file-meta'},
                file.classifier ? el('span', {class: 'maven-file-classifier'}, file.classifier) : null,
                integrity ? el('span', {title: t('maven.fileIntegrity')}, integrity) : null,
                el('span', {}, formatBytes(Number(file.size) || 0)),
                file.modified_at ? el('span', {}, formatDate(file.modified_at)) : null
            )
        ));
    });
    return el('details', {class: 'maven-version-files'},
        el('summary', {}, createIcon('folder'),
            el('span', {}, t('maven.versionFiles', {count: Number(version.file_count) || files.length}))),
        list,
        version.files_truncated ? el('p', {class: 'maven-section-note'}, t('maven.filesTruncated')) : null
    );
}

/**
 * Render the Maven repository landing catalog.
 * @param {HTMLElement} container
 * @param {string} repository
 * @param {number} sequence
 * @returns {Promise<void>}
 */
async function renderCatalog(container, repository, sequence) {
    setRepositoryViewBusy(container, true);
    try {
        const [domainResponse, artifactResponse] = await Promise.all([
            apiRequest(`/api/maven/repositories/${encodeURIComponent(repository)}/domains`),
            apiRequest(`/api/maven/repositories/${encodeURIComponent(repository)}/packages?limit=30&offset=0`)
        ]);
        if (sequence !== mavenLoadSequence) return;
        if (!domainResponse.ok) throw new Error(t('maven.loadFailed'));
        const domainData = await domainResponse.json();
        const artifactData = await readArtifactPage(artifactResponse);
        const domains = Array.isArray(domainData.domains) ? domainData.domains : [];
        const artifacts = Array.isArray(artifactData.artifacts) ? artifactData.artifacts : [];
        const verifiedCount = domains.filter(domain => domain.verified).length;
        const hero = el('section', {class: 'maven-hero'},
            el('div', {class: 'maven-hero-heading'},
                el('div', {}, el('span', {class: 'maven-kicker'}, t('maven.kicker')),
                    el('h2', {}, createIcon(mavenRepositoryIcon), el('span', {}, repository)))
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
        await replaceRepositoryView(container, content, {duration: 280, enterDuration: 420});
    } catch (error) {
        if (sequence !== mavenLoadSequence) return;
        await replaceRepositoryView(container,
            el('div', {class: 'maven-error'}, createIcon('alertCircle'),
                el('span', {}, caughtErrorMessage(error, 'maven.loadFailed'))),
            {duration: 240, enter: false});
    }
}

/**
 * Render provider-specific verification instructions.
 * @param {object} domain
 * @returns {HTMLElement}
 */
function verificationPanel(domain) {
    if (domain.verification_type === 'mirror') {
        return el('div', {class: 'maven-verification-panel'},
            el('p', {}, t('maven.mirrorDomainHint')),
            el('div', {class: 'maven-verification-target'},
                el('span', {}, t('maven.domainLabel')),
                el('code', {}, domain.domain)
            )
        );
    }
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
 * @param {object} details
 * @param {Function} refresh
 * @returns {HTMLElement|null}
 */
function teamPanel(details, refresh) {
    const members = Array.isArray(details.members) ? details.members : [];
    const administrator = Boolean(details.administrator);
    if (members.length === 0 && !administrator) return null;
    const level = Number(details.domain.permission_level) || 0;
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
                    const response = await apiRequest(`/api/maven/domains/${encodeURIComponent(details.domain.domain)}/members/${encodeURIComponent(member.user_id || member.username)}`, {
                        method: 'PUT', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({level: Number(value)})
                    });
                    if (!response.ok) {
                        showAlert(await responseErrorMessage(response, 'maven.updateMemberFailed'), 'error');
                    }
                    else await refresh();
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
                    const response = await apiRequest(`/api/maven/domains/${encodeURIComponent(details.domain.domain)}/members/${encodeURIComponent(member.user_id || member.username)}`, {method: 'DELETE'});
                    if (!response.ok) {
                        showAlert(await responseErrorMessage(response, 'maven.removeMemberFailed'), 'error');
                    }
                    else await refresh();
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
                if (users.length === 0) {
                    showAlert(t('maven.inviteRequired'), 'error');
                    input.focus();
                    return;
                }
                submit.disabled = true;
                try {
                    const response = await apiRequest(`/api/maven/domains/${encodeURIComponent(details.domain.domain)}/members`, {
                        method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({users, level: inviteLevel})
                    });
                    if (!response.ok) {
                        showAlert(await responseErrorMessage(response, 'maven.inviteFailed'), 'error');
                    }
                    else {
                        input.value = '';
                        showAlert(t('maven.inviteSent'), 'success');
                        await refresh();
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
 * Build the API URL for the active permission and source filters.
 * @returns {string}
 */
function managedDomainListURL() {
    if (cachedIsManager) domainCenterFilters.delete('level-0');
    else {
        domainCenterFilters.delete('state-unverified');
        domainCenterFilters.delete('state-mirror');
    }
    const params = new URLSearchParams({
        view: 'managed', limit: String(domainCenterPageSize), offset: String(domainCenterOffset)
    });
    const levels = [...domainCenterFilters]
        .filter(value => value.startsWith('level-'))
        .map(value => value.slice('level-'.length));
    const states = [...domainCenterFilters]
        .filter(value => value.startsWith('state-'))
        .map(value => value.slice('state-'.length));
    if (levels.length > 0) params.set('levels', levels.join(','));
    if (states.length > 0) params.set('states', states.join(','));
    return `/api/maven/domains?${params}`;
}

/**
 * Build one toggle button for the multi-select domain filter.
 * @param {string} filter
 * @param {string} label
 * @param {HTMLElement} container
 * @returns {HTMLButtonElement}
 */
function domainFilterButton(filter, label, container) {
    const selected = domainCenterFilters.has(filter);
    return el('button', {
        type: 'button', class: `maven-domain-filter${selected ? ' is-active' : ''}`,
        'aria-pressed': String(selected),
        onclick: event => {
            const active = domainCenterFilters.has(filter);
            if (active) domainCenterFilters.delete(filter);
            else domainCenterFilters.add(filter);
            event.currentTarget.classList.toggle('is-active', !active);
            event.currentTarget.setAttribute('aria-pressed', String(!active));
            domainCenterOffset = 0;
            void renderDomainCenterList(container);
        }
    }, label);
}

/**
 * Build bounded previous/next controls for the domain list.
 * @param {HTMLElement} container
 * @param {number} total
 * @returns {HTMLElement|null}
 */
function domainCenterPagination(container, total) {
    if (total <= domainCenterPageSize) return null;
    const pageCount = Math.max(1, Math.ceil(total / domainCenterPageSize));
    const page = Math.floor(domainCenterOffset / domainCenterPageSize) + 1;
    const previous = el('button', {
        type: 'button', class: 'maven-pagination-btn', disabled: domainCenterOffset === 0,
        onclick: () => {
            domainCenterOffset = Math.max(0, domainCenterOffset - domainCenterPageSize);
            void renderDomainCenterList(container);
        }
    }, createIcon('chevronLeft'), el('span', {}, t('common.prev')));
    const next = el('button', {
        type: 'button', class: 'maven-pagination-btn',
        disabled: domainCenterOffset + domainCenterPageSize >= total,
        onclick: () => {
            domainCenterOffset += domainCenterPageSize;
            void renderDomainCenterList(container);
        }
    }, el('span', {}, t('common.next')), createIcon('chevronRight'));
    return el('nav', {class: 'maven-domain-pagination', 'aria-label': t('maven.paginationLabel')},
        previous,
        el('span', {class: 'maven-pagination-info'}, t('maven.pagination', {page, pages: pageCount, total})),
        next
    );
}

/**
 * Build the stable toolbar and filter controls for the Maven-domain list.
 * @param {HTMLElement} container - Routed domain center container.
 * @param {boolean} administrator - Whether administrator-only filters are available.
 * @param {HTMLElement} results - Persistent result and pagination host.
 * @returns {HTMLElement[]} Stable list-shell nodes.
 */
function domainCenterListShell(container, administrator, results) {
    const createButton = el('button', {
        type: 'button', class: 'pill-btn pill-btn--primary',
        onclick: () => openCreateDomainDialog(created => navigateMavenDomainCenter(created.domain))
    }, createIcon('plus'), el('span', {}, t('maven.createDomain')));
    const header = el('div', {class: 'maven-domain-center-toolbar'},
        el('div', {},
            el('h3', {}, t('maven.domainSettings')),
            el('p', {}, t('maven.domainCenterHint'))
        ),
        createButton
    );
    const filters = el('div', {class: 'maven-domain-filter-bar'},
        el('span', {class: 'maven-domain-filter-label'}, t('maven.filterLabel'))
    );
    const permissionLevels = administrator ? [1, 2, 3, 4] : [0, 1, 2, 3, 4];
    permissionLevels.forEach(level => filters.appendChild(
        domainFilterButton(`level-${level}`, permissionLabel(level), container)
    ));
    if (administrator) {
        filters.append(
            domainFilterButton('state-unverified', t('maven.filterUnverified'), container),
            domainFilterButton('state-mirror', t('maven.filterMirrored'), container)
        );
    }
    return [header, filters, results];
}

/**
 * Build the replaceable domain cards and pagination controls.
 * @param {HTMLElement} container - Routed domain center container.
 * @param {object[]} domains - Current bounded domain page.
 * @param {number} total - Filtered domain count.
 * @returns {HTMLElement[]} Result host children.
 */
function domainCenterResultNodes(container, domains, total) {
    const list = el('div', {class: 'maven-domain-list'});
    if (domains.length === 0) {
        list.appendChild(el('div', {class: 'maven-empty'}, createIcon('network'),
            el('span', {}, t(domainCenterFilters.size > 0 ? 'maven.noFilteredDomains' : 'maven.noManagedDomains'))));
    } else {
        domains.forEach(domain => list.appendChild(domainCard('', domain, selected => {
            navigateMavenDomainCenter(selected.domain);
        })));
    }
    return [list, domainCenterPagination(container, total)].filter(Boolean);
}

/**
 * Render the global Maven domain-management list.
 * @param {HTMLElement} container
 * @returns {Promise<void>}
 */
async function renderDomainCenterList(container) {
    const sequence = ++domainCenterSequence;
    const existingResults = container.querySelector(':scope > .maven-domain-results');
    const busyTarget = existingResults || container;
    if (!container.firstElementChild) container.replaceChildren(createSkeleton('list', 3));
    setRepositoryViewBusy(busyTarget, true);
    try {
        const response = await apiRequest(managedDomainListURL());
        if (sequence !== domainCenterSequence || container !== domainCenterBody) return;
        if (!response.ok) throw await localizedResponseError(response, 'maven.loadFailed');
        const payload = await response.json();
        const domains = Array.isArray(payload.domains) ? payload.domains : [];
        const total = Number(payload.total);
        if (!Number.isInteger(total) || total < 0) throw new Error(t('maven.loadFailed'));
        const administrator = Boolean(payload.administrator);
        const activeLanguage = window.i18n?.currentLanguage?.() || '';
        if (administrator) domainCenterFilters.delete('level-0');
        else {
            domainCenterFilters.delete('state-unverified');
            domainCenterFilters.delete('state-mirror');
        }
        if (total > 0 && domainCenterOffset >= total) {
            domainCenterOffset = Math.floor((total - 1) / domainCenterPageSize) * domainCenterPageSize;
            await renderDomainCenterList(container);
            return;
        }
        const preserveShell = existingResults?.dataset.administrator === String(administrator) &&
            existingResults.dataset.language === activeLanguage;
        if (preserveShell) {
            await replaceRepositoryView(existingResults, domainCenterResultNodes(container, domains, total), {
                duration: 260,
                enterDuration: 380
            });
        } else {
            const results = el('div', {
                class: 'maven-domain-results',
                'data-administrator': String(administrator),
                'data-language': activeLanguage
            }, ...domainCenterResultNodes(container, domains, total));
            await replaceRepositoryView(container, domainCenterListShell(container, administrator, results), {
                duration: 300,
                enterDuration: 420
            });
        }
    } catch (error) {
        if (sequence !== domainCenterSequence || container !== domainCenterBody) return;
        await replaceRepositoryView(existingResults || container,
            el('div', {class: 'maven-error'}, createIcon('alertCircle'),
                el('span', {}, caughtErrorMessage(error, 'maven.loadFailed'))),
            {duration: 240, enterDuration: 340});
    } finally {
        if (sequence === domainCenterSequence) setRepositoryViewBusy(busyTarget, false);
    }
}

/**
 * Render one globally managed Maven domain.
 * @param {HTMLElement} container
 * @param {string} domainName
 * @returns {Promise<void>}
 */
async function renderManagedDomain(container, domainName) {
    const sequence = ++domainCenterSequence;
    if (!container.firstElementChild) container.replaceChildren(createSkeleton('form', 2));
    setRepositoryViewBusy(container, true);
    const refresh = () => renderManagedDomain(container, domainName);
    try {
        const response = await apiRequest(`/api/maven/domains/${encodeURIComponent(domainName)}`);
        if (sequence !== domainCenterSequence || container !== domainCenterBody) return;
        if (!response.ok) throw await localizedResponseError(response, 'maven.domainLoadFailed');
        const details = await response.json();
        const domain = details.domain;
        const canOwn = details.administrator || Number(domain.permission_level) === 4;
        const actions = el('div', {class: 'maven-domain-actions'});
        if (!domain.verified && canOwn && domain.verification_code) {
            actions.appendChild(el('button', {
                type: 'button', class: 'pill-btn pill-btn--primary', onclick: async event => {
                    await runButtonAction(event.currentTarget, async () => {
                        const verifyResponse = await apiRequest(`/api/maven/domains/${encodeURIComponent(domain.domain)}/verify`, {method: 'POST'});
                        if (!verifyResponse.ok) {
                            showAlert(t(verifyResponse.status === 429 ? 'maven.verifyRateLimited' : 'maven.verifyFailed'), 'error');
                            return;
                        }
                        showAlert(t('maven.verifySuccess'), 'success');
                        await refresh();
                    });
                }
            }, createIcon('check'), el('span', {}, t('maven.verifyNow'))));
        }
        if (!domain.verified && cachedIsManager) {
            actions.appendChild(el('button', {
                type: 'button', class: 'pill-btn pill-btn--soft', onclick: async event => {
                    if (!(await showConfirm(t('maven.forceVerifyConfirm', {domain: domain.domain})))) return;
                    await runButtonAction(event.currentTarget, async () => {
                        const verifyResponse = await apiRequest(`/api/maven/domains/${encodeURIComponent(domain.domain)}/verify/force`, {method: 'POST'});
                        if (!verifyResponse.ok) {
                            showAlert(await responseErrorMessage(verifyResponse, 'maven.forceVerifyFailed'), 'error');
                        }
                        else {
                            showAlert(t('maven.forceVerifySuccess'), 'success');
                            await refresh();
                        }
                    });
                }
            }, createIcon('warning'), el('span', {}, t('maven.forceVerify'))));
        }
        if (canOwn && Number(domain.artifact_count) === 0) {
            actions.appendChild(el('button', {
                type: 'button', class: 'pill-btn pill-btn--ghost-danger', onclick: async () => {
                    if (!(await showConfirm(t('maven.deleteDomainConfirm', {domain: domain.domain})))) return;
                    const deleteResponse = await apiRequest(`/api/maven/domains/${encodeURIComponent(domain.domain)}`, {method: 'DELETE'});
                    if (!deleteResponse.ok) {
                        showAlert(await responseErrorMessage(deleteResponse, 'maven.deleteDomainFailed'), 'error');
                    }
                    else navigateMavenDomainCenter();
                }
            }, createIcon('delete'), el('span', {}, t('maven.deleteDomain'))));
        }
        const back = el('button', {
            type: 'button', class: 'maven-back-btn', onclick: () => navigateMavenDomainCenter()
        }, createIcon('chevronLeft'), el('span', {}, t('maven.backToDomains')));
        const hero = el('section', {class: 'maven-hero'}, back,
            el('div', {class: 'maven-hero-heading'},
                el('div', {}, el('span', {class: 'maven-kicker'}, t('maven.domainKicker')),
                    el('h2', {}, createIcon('network'), el('span', {}, domain.domain))),
                actions
            ),
            el('div', {class: 'maven-stats'},
                el('span', {class: `maven-status-badge is-${domain.verified ? 'verified' : 'pending'}`}, domain.verified ? t('maven.verified') : t('maven.pending')),
                domain.member ? el('span', {class: 'maven-permission-badge'}, permissionLabel(domain.permission_level)) : null,
                el('span', {}, t('maven.artifactCount', {count: Number(domain.artifact_count) || 0})),
                domain.verified_at ? el('span', {}, t('maven.verifiedAt', {date: formatDate(domain.verified_at)})) : null
            ),
            !domain.verified ? verificationPanel(domain) : null
        );
        const team = teamPanel(details, refresh);
        await replaceRepositoryView(container, [hero, domainInformationSection(details), team], {
            duration: 300,
            enterDuration: 420
        });
    } catch (error) {
        if (sequence !== domainCenterSequence || container !== domainCenterBody) return;
        await replaceRepositoryView(container, [
            el('button', {type: 'button', class: 'maven-back-btn', onclick: () => navigateMavenDomainCenter()},
                createIcon('chevronLeft'), el('span', {}, t('maven.backToDomains'))),
            el('div', {class: 'maven-error'}, caughtErrorMessage(error, 'maven.domainLoadFailed'))
        ], {duration: 240, enterDuration: 380});
    } finally {
        if (sequence === domainCenterSequence) setRepositoryViewBusy(container, false);
    }
}

/**
 * Render the current account Maven-domain subpage.
 * @returns {Promise<void>}
 */
export async function loadMavenDomainCenterPage() {
    if (!cachedIsLoggedIn) return;
    const container = document.getElementById('maven-domain-page-content');
    const homeButton = document.getElementById('maven-domain-home');
    if (!container) return;
    domainCenterBody = container;
    if (homeButton && homeButton.dataset.bound !== 'true') {
        homeButton.dataset.bound = 'true';
        homeButton.addEventListener('click', () => {
            if (window.location.pathname !== '/' || window.location.search || window.location.hash) {
                window.history.pushState(null, '', '/');
            }
            window.dispatchEvent(new PopStateEvent('popstate'));
        });
    }
    const route = mavenDomainRouteFromPath();
    if (!route) return;
    if (route.domain) await renderManagedDomain(container, route.domain);
    else await renderDomainCenterList(container);
}

/**
 * Open global Maven domain configuration from the account menu.
 * @returns {void}
 */
export function openMavenDomainCenter() {
    if (!cachedIsLoggedIn) return;
    domainCenterOffset = 0;
    navigateMavenDomainCenter();
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
    setRepositoryViewBusy(container, true);
    try {
        const [domainResponse, artifactsResponse] = await Promise.all([
            apiRequest(`/api/maven/domains/${encodeURIComponent(domainName)}`),
            apiRequest(`/api/maven/repositories/${encodeURIComponent(repository)}/packages?domain=${encodeURIComponent(domainName)}&limit=50&offset=0`)
        ]);
        if (sequence !== mavenLoadSequence) return;
        if (!domainResponse.ok) throw await localizedResponseError(domainResponse, 'maven.domainLoadFailed');
        const details = await domainResponse.json();
        const artifactData = await readArtifactPage(artifactsResponse);
        const domain = details.domain;
        const hero = el('section', {class: 'maven-hero'},
            backButton(`/${encodePathSegment(repository)}`, t('maven.backToRepository')),
            el('div', {class: 'maven-hero-heading'},
                el('div', {}, el('span', {class: 'maven-kicker'}, t('maven.domainKicker')),
                    el('h2', {}, createIcon('network'), el('span', {}, domain.domain)))
            ),
            el('div', {class: 'maven-stats'},
                el('span', {class: `maven-status-badge is-${domain.verified ? 'verified' : 'pending'}`}, domain.verified ? t('maven.verified') : t('maven.pending')),
                Number(domain.permission_level) > 0 ? el('span', {class: 'maven-permission-badge'}, permissionLabel(domain.permission_level)) : null,
                el('span', {}, t('maven.artifactCount', {count: Number(artifactData.total) || 0})),
                domain.verified_at ? el('span', {}, t('maven.verifiedAt', {date: formatDate(domain.verified_at)})) : null
            )
        );
        const artifacts = Array.isArray(artifactData.artifacts) ? artifactData.artifacts : [];
        const artifactList = el('div', {class: 'maven-artifact-list'});
        if (artifacts.length === 0) artifactList.appendChild(el('div', {class: 'maven-empty'}, t('maven.noDomainArtifacts')));
        else artifacts.forEach(artifact => artifactList.appendChild(artifactCard(repository, artifact)));
        const sections = [
            hero,
            domainInformationSection(details, {repository, repositoryArtifactCount: artifactData.total}),
            el('section', {class: 'maven-section'}, el('h3', {}, t('maven.artifactsTitle')), artifactList)
        ];
        await replaceRepositoryView(container, sections, {duration: 280, enterDuration: 420});
    } catch (error) {
        if (sequence !== mavenLoadSequence) return;
        await replaceRepositoryView(container, [
            backButton(`/${encodePathSegment(repository)}`, t('maven.backToRepository')),
            el('div', {class: 'maven-error'}, caughtErrorMessage(error, 'maven.domainLoadFailed'))
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
                            if (!response.ok) {
                                showAlert(await responseErrorMessage(response, 'maven.updateDescriptionFailed'), 'error');
                            }
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
    setRepositoryViewBusy(container, true);
    try {
        const query = new URLSearchParams({group: groupID, artifact: artifactID});
        const response = await apiRequest(`/api/maven/repositories/${encodeURIComponent(repository)}/package?${query}`);
        if (sequence !== mavenLoadSequence) return;
        if (!response.ok) throw await localizedResponseError(response, 'maven.artifactLoadFailed');
        const details = await response.json();
        const artifact = details.artifact;
        const project = details.project || null;
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
            el('p', {}, artifact.description || project?.description || t('maven.noDescription')),
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
                Number(details.file_count) > 0
                    ? el('span', {}, t('maven.versionFiles', {count: Number(details.file_count)}))
                    : null,
                Number(details.signed_file_count) > 0
                    ? el('span', {}, t('maven.signedFiles', {count: Number(details.signed_file_count)}))
                    : null,
                project?.packaging ? el('code', {}, project.packaging) : null,
                artifact.mirrored ? createRepositoryMirrorBadge(t('common.fromMirror')) : null,
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
                        if (!deleteResponse.ok) {
                            showAlert(await responseErrorMessage(deleteResponse, 'maven.deleteVersionFailed'), 'error');
                        }
                        else await renderArtifact(container, repository, groupID, artifactID, sequence);
                    }
                }, createIcon('delete')));
            }
            const row = el('div', {class: 'maven-version-row'},
                el('div', {class: 'maven-version-main'}, el('code', {}, version.version),
                    el('span', {}, formatDate(version.created_at))),
                el('div', {class: 'maven-version-meta'},
                    version.mirrored ? createRepositoryMirrorBadge(t('common.fromMirror')) : null,
                    version.publisher ? createUserIdentity(version.publisher) : null,
                    Number(version.file_count) > 0
                        ? el('span', {}, t('maven.versionFiles', {count: Number(version.file_count)}))
                        : null,
                    Number(version.signed_file_count) > 0
                        ? el('span', {}, t('maven.signedFiles', {count: Number(version.signed_file_count)}))
                        : null,
                    el('span', {}, formatBytes(Number(version.total_file_size) || Number(version.size) || 0)),
                    version.last_modified ? el('span', {}, formatDate(version.last_modified)) : null,
                    actions)
            );
            versionList.appendChild(el('div', {class: 'maven-version-entry'}, row, mavenVersionFiles(version)));
        });
        if (versions.length === 0) versionList.appendChild(el('div', {class: 'maven-empty'}, t('maven.noVersions')));
        await replaceRepositoryView(container, [
            hero,
            artifactInformationSection(details, repository),
            mavenProjectInformationSection(details),
            mavenDependencySection(project),
            el('section', {class: 'maven-section'}, el('h3', {}, t('maven.versionsTitle')), versionList)
        ], {duration: 280, enterDuration: 420});
    } catch (error) {
        if (sequence !== mavenLoadSequence) return;
        await replaceRepositoryView(container, [
            backButton(`/${encodePathSegment(repository)}`, t('maven.backToRepository')),
            el('div', {class: 'maven-error'}, caughtErrorMessage(error, 'maven.artifactLoadFailed'))
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
