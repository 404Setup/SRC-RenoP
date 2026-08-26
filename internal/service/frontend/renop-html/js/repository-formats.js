/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

const FORMAT_CATALOG = Object.freeze({
    maven: Object.freeze({
        id: 'maven',
        protocol: 'maven',
        layout: 'modern',
        icon: 'repositoryMaven',
        labelKey: 'repos.formatMaven',
        descriptionKey: 'repos.formatMavenDesc',
        supportsBrowserUpload: false,
        supportsRedeployment: true,
        supportsGpg: true,
        supportsArtifactTemplate: false,
        snippetTabs: Object.freeze(['maven', 'gradle-kotlin', 'gradle-groovy', 'sbt'])
    }),
    'maven-classic': Object.freeze({
        id: 'maven-classic',
        protocol: 'maven',
        layout: 'classic',
        icon: 'repositoryMaven',
        offered: false,
        labelKey: 'repos.formatMaven',
        descriptionKey: 'repos.formatMavenDesc',
        supportsBrowserUpload: false,
        supportsRedeployment: true,
        supportsGpg: true,
        supportsArtifactTemplate: false,
        snippetTabs: Object.freeze(['maven', 'gradle-kotlin', 'gradle-groovy', 'sbt'])
    }),
    files: Object.freeze({
        id: 'files',
        protocol: 'files',
        icon: 'repositoryFiles',
        labelKey: 'repos.formatFiles',
        descriptionKey: 'repos.formatFilesDesc',
        supportsBrowserUpload: true,
        supportsRedeployment: false,
        supportsGpg: false,
        supportsArtifactTemplate: false,
        supportsUploadHelpers: false,
        snippetTabs: Object.freeze([])
    }),
    cargo: Object.freeze({
        id: 'cargo',
        protocol: 'cargo',
        icon: 'repositoryCargo',
        labelKey: 'repos.formatCargo',
        descriptionKey: 'repos.formatCargoDesc',
        supportsBrowserUpload: false,
        supportsRedeployment: false,
        supportsGpg: false,
        supportsArtifactTemplate: true,
        snippetTabs: Object.freeze(['cargo-registry', 'cargo-source', 'cargo-login', 'cargo-publish'])
    }),
    docker: Object.freeze({
        id: 'docker',
        protocol: 'docker',
        icon: 'repositoryDocker',
        labelKey: 'repos.formatDocker',
        descriptionKey: 'repos.formatDockerDesc',
        supportsBrowserUpload: false,
        supportsRedeployment: true,
        supportsGpg: false,
        supportsArtifactTemplate: false,
        snippetTabs: Object.freeze(['docker-pull', 'docker-tag', 'docker-push', 'docker-login'])
    })
});

const RESERVED_REPOSITORY_NAMES = new Set(['api', 'assets', 'css', 'js', 'svg', 'javadoc', 'javadocs', 'cargodoc', 'cargodocs', 'cratedoc', 'cratedocs', 'v2']);

/**
 * Return the canonical descriptor for a repository format.
 * @param {string} format - Stored repository format.
 * @returns {object} Format descriptor; Maven is the legacy default.
 */
export function getRepositoryFormat(format) {
    const normalized = String(format || 'maven').trim().toLowerCase();
    return FORMAT_CATALOG[normalized] || FORMAT_CATALOG.maven;
}

/**
 * Return every repository format offered during creation.
 * @returns {object[]} Immutable format descriptors.
 */
export function listRepositoryFormats() {
    return Object.values(FORMAT_CATALOG).filter(format => format.offered !== false);
}

/**
 * Build a new repository payload with only fields supported by its format.
 * @param {string} name - Valid repository slug.
 * @param {string} format - Selected format identifier.
 * @returns {object} Repository creation payload.
 */
export function createRepositoryDraft(name, format) {
    const descriptor = getRepositoryFormat(format);
    const repository = {
        name,
        format: descriptor.id,
        visibility: 'PUBLIC',
        mirrors: []
    };
    if (descriptor.id === 'maven') {
        repository.allow_redeployment = false;
        repository.require_gpg_signature = false;
    } else if (descriptor.id === 'files') {
        repository.allow_redeployment = true;
    }
    return repository;
}

/**
 * Validate the strict top-level slug required for newly created repositories.
 * @param {string} name - Candidate repository name.
 * @returns {boolean} Whether the name is lowercase ASCII and route-safe.
 */
export function isValidRepositorySlug(name) {
    const value = String(name || '');
    return value.length <= 64
        && /^[a-z]+(?:-[a-z]+)*$/.test(value)
        && !RESERVED_REPOSITORY_NAMES.has(value);
}
