/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

const FORMAT_CATALOG = Object.freeze({
    maven: Object.freeze({
        id: 'maven',
        labelKey: 'repos.formatMaven',
        descriptionKey: 'repos.formatMavenDesc',
        supportsBrowserUpload: true,
        supportsRedeployment: true,
        supportsGpg: true,
        supportsArtifactTemplate: false,
        snippetTabs: Object.freeze(['maven', 'gradle-kotlin', 'gradle-groovy', 'sbt'])
    }),
    cargo: Object.freeze({
        id: 'cargo',
        labelKey: 'repos.formatCargo',
        descriptionKey: 'repos.formatCargoDesc',
        supportsBrowserUpload: false,
        supportsRedeployment: false,
        supportsGpg: false,
        supportsArtifactTemplate: true,
        snippetTabs: Object.freeze(['cargo-registry', 'cargo-source', 'cargo-login', 'cargo-publish'])
    })
});

const RESERVED_REPOSITORY_NAMES = new Set(['api', 'assets', 'css', 'js', 'svg', 'javadoc', 'javadocs']);

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
    return Object.values(FORMAT_CATALOG);
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
