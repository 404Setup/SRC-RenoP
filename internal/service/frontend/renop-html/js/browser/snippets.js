/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {t} from '../i18n.js';
import {createIcon} from '../components.js';
import {getRepositoryFormat} from '../repository-formats.js';
import {decodePathSegment} from './utils.js';

let currentSnippets = {};
let snippetUpdateSequence = 0;
let tabsResizeObserver = null;

/**
 * Read the first text value of an XML element.
 * @param {Document} documentNode - Parsed Maven metadata document.
 * @param {string} tag - Element name.
 * @returns {string} First text value, or an empty string.
 */
function xmlText(documentNode, tag) {
    const node = documentNode.getElementsByTagName(tag)[0];
    return node?.textContent || '';
}

/**
 * Resolve Maven coordinates for the current path when metadata is available.
 * @param {string} path - Browser path.
 * @param {string[]} pathParts - Decoded path segments.
 * @returns {Promise<{groupId: string, artifactId: string, version: string}|null>} Maven coordinates.
 */
async function detectMavenCoordinates(path, pathParts) {
    if (pathParts.length <= 3) return null;
    try {
        const directoryPath = path.endsWith('/') ? path : `${path}/`;
        let metadataPath = `${directoryPath}maven-metadata.xml`;
        let metadataResponse = await fetch(`/api/repositories/details${metadataPath}`);
        let version = '';
        if (!metadataResponse.ok) {
            const parentPath = `/${pathParts.slice(0, -1).join('/')}/`;
            metadataPath = `${parentPath}maven-metadata.xml`;
            metadataResponse = await fetch(`/api/repositories/details${metadataPath}`);
            version = pathParts[pathParts.length - 1];
        }
        if (!metadataResponse.ok) return null;
        const artifactResponse = await fetch(metadataPath);
        if (!artifactResponse.ok) return null;
        const documentNode = new DOMParser().parseFromString(await artifactResponse.text(), 'text/xml');
        if (documentNode.querySelector('parsererror')) return null;
        const groupId = xmlText(documentNode, 'groupId');
        const artifactId = xmlText(documentNode, 'artifactId');
        if (!version) {
            const versions = documentNode.getElementsByTagName('version');
            version = versions.length > 0 ? versions[versions.length - 1].textContent || '' : '';
        }
        return groupId && artifactId && version ? {groupId, artifactId, version} : null;
    } catch (error) {
        console.error('Failed to resolve Maven metadata', error);
        return null;
    }
}

/**
 * Build Maven dependency or repository configuration snippets.
 * @param {string} path - Browser path.
 * @param {string[]} pathParts - Decoded path segments.
 * @returns {Promise<{snippets: Object.<string, string>, artifact: boolean}>} Maven snippet state.
 */
async function buildMavenSnippets(path, pathParts) {
    const coordinates = await detectMavenCoordinates(path, pathParts);
    if (coordinates) {
        const {groupId, artifactId, version} = coordinates;
        return {
            artifact: true,
            snippets: {
                maven: `<dependency>\n  <groupId>${groupId}</groupId>\n  <artifactId>${artifactId}</artifactId>\n  <version>${version}</version>\n</dependency>`,
                'gradle-kotlin': `implementation("${groupId}:${artifactId}:${version}")`,
                'gradle-groovy': `implementation '${groupId}:${artifactId}:${version}'`,
                sbt: `libraryDependencies += "${groupId}" % "${artifactId}" % "${version}"`
            }
        };
    }

    const repositoryPath = pathParts.length > 0 ? `/${encodeURIComponent(pathParts[0])}` : '';
    const repositoryURL = window.location.origin + repositoryPath;
    const titleElement = document.querySelector('.nav-title a') || document.querySelector('title');
    const instanceName = titleElement ? titleElement.textContent.trim() : 'Renop';
    const cleanName = instanceName.replace(/[^a-zA-Z0-9-]/g, '-').toLowerCase();
    const repositoryID = pathParts.length > 0 ? `${cleanName}-${pathParts[0]}` : cleanName;
    const repositoryName = pathParts.length > 0 ? `${instanceName} - ${pathParts[0]}` : instanceName;
    return {
        artifact: false,
        snippets: {
            maven: `<repository>\n  <id>${repositoryID}</id>\n  <name>${repositoryName}</name>\n  <url>${repositoryURL}</url>\n</repository>`,
            'gradle-kotlin': `maven {\n  url = uri("${repositoryURL}")\n}`,
            'gradle-groovy': `maven {\n  url "${repositoryURL}"\n}`,
            sbt: `resolvers += "${repositoryName}" at "${repositoryURL}"`
        }
    };
}

/**
 * Build sparse-registry configuration and command snippets for Cargo.
 * @param {string} repositoryName - Repository slug.
 * @returns {Object.<string, string>} Cargo snippets keyed by format tab ID.
 */
function buildCargoSnippets(repositoryName) {
    const encodedName = encodeURIComponent(repositoryName);
    const registryURL = `${window.location.origin}/${encodedName}/`;
    const sparseURL = `sparse+${registryURL}`;
    return {
        'cargo-registry': `[registries.${repositoryName}]\nindex = "${sparseURL}"`,
        'cargo-source': `[source.crates-io]\nreplace-with = "${repositoryName}"\n\n[source.${repositoryName}]\nregistry = "${sparseURL}"`,
        'cargo-login': `cargo login --registry ${repositoryName}`,
        'cargo-publish': `cargo publish --registry ${repositoryName}`
    };
}

/**
 * Return the localized tab label for a snippet type.
 * @param {string} type - Catalog snippet tab ID.
 * @returns {string} Localized or product-standard tab label.
 */
function snippetTabLabel(type) {
    switch (type) {
        case 'gradle-kotlin': return 'Gradle Kotlin';
        case 'gradle-groovy': return 'Gradle Groovy';
        case 'sbt': return 'SBT';
        case 'cargo-registry': return t('details.cargoRegistryTab');
        case 'cargo-source': return t('details.cargoSourceTab');
        case 'cargo-login': return t('details.cargoLoginTab');
        case 'cargo-publish': return t('details.cargoPublishTab');
        default: return 'Maven';
    }
}

/**
 * Scroll the activated tab into view and update the sliding indicator.
 * @param {HTMLButtonElement} tab - Activated snippet tab.
 * @returns {void}
 */
function revealSnippetTab(tab) {
    const container = tab.parentElement;
    if (!container) return;
    const left = tab.offsetLeft;
    const right = left + tab.offsetWidth;
    if (left < container.scrollLeft) {
        container.scrollTo({left, behavior: 'smooth'});
    } else if (right > container.scrollLeft + container.clientWidth) {
        container.scrollTo({left: right - container.clientWidth, behavior: 'smooth'});
    }
    syncTabIndicator(container);
}

/**
 * Release an explicit snippet-content height after its transition.
 * @param {HTMLElement} container - Snippet content container.
 * @returns {void}
 */
function releaseSnippetHeight(container) {
    container.style.height = 'auto';
    container.style.transition = '';
    container.__heightTimeout = 0;
}

/**
 * Replace code text and animate the content container to its new height.
 * @param {HTMLElement} container - Snippet content container.
 * @param {HTMLElement} code - Code node.
 * @param {string} snippetType - Selected snippet type.
 * @param {number} oldHeight - Previous content height.
 * @returns {void}
 */
function applySnippetChange(container, code, snippetType, oldHeight) {
    code.textContent = currentSnippets[snippetType] || '';
    container.style.height = 'auto';
    const newHeight = container.getBoundingClientRect().height;
    container.style.height = `${oldHeight}px`;
    void container.offsetHeight;
    code.classList.remove('code-changing');
    container.style.transition = 'height 0.22s cubic-bezier(0.2, 0.8, 0.2, 1)';
    container.style.height = `${newHeight}px`;
    container.__heightTimeout = setTimeout(releaseSnippetHeight.bind(null, container), 220);
    container.__fadeTimeout = 0;
}

/**
 * Activate a snippet tab and animate the code replacement.
 * @param {MouseEvent} event - Snippet tab click.
 * @returns {void}
 */
function handleSnippetTabClick(event) {
    const tab = event.currentTarget;
    if (!(tab instanceof HTMLButtonElement)) return;
    const code = document.getElementById('snippet-code');
    const container = code?.closest('.snippet-content');
    if (!code || !container) return;
    for (const candidate of document.querySelectorAll('.snippet-tab')) {
        candidate.classList.toggle('active', candidate === tab);
    }
    revealSnippetTab(tab);
    if (container.__heightTimeout) clearTimeout(container.__heightTimeout);
    if (container.__fadeTimeout) clearTimeout(container.__fadeTimeout);
    container.style.transition = '';
    const oldHeight = container.getBoundingClientRect().height;
    container.style.height = `${oldHeight}px`;
    code.classList.add('code-changing');
    container.__fadeTimeout = setTimeout(
        applySnippetChange.bind(null, container, code, tab.dataset.snippet || '', oldHeight),
        75
    );
}

/**
 * Keep the active tab indicator aligned after container resizing.
 * @param {ResizeObserverEntry[]} entries - Resize observer entries.
 * @returns {void}
 */
function handleSnippetTabsResize(entries) {
    const container = entries[0]?.target;
    if (container instanceof HTMLElement) syncTabIndicator(container);
}

/**
 * Render format-specific snippet tabs and select the first one.
 * @param {string[]} types - Catalog tab IDs.
 * @returns {void}
 */
function renderSnippetTabs(types) {
    const container = document.querySelector('.snippet-tabs');
    if (!container) return;
    container.innerHTML = '';
    for (let index = 0; index < types.length; index++) {
        const type = types[index];
        const tab = document.createElement('button');
        tab.type = 'button';
        tab.className = `snippet-tab${index === 0 ? ' active' : ''}`;
        tab.dataset.snippet = type;
        tab.textContent = snippetTabLabel(type);
        tab.addEventListener('click', handleSnippetTabClick);
        container.appendChild(tab);
    }
    syncTabIndicator(container);
    if (tabsResizeObserver) tabsResizeObserver.disconnect();
    tabsResizeObserver = new ResizeObserver(handleSnippetTabsResize);
    tabsResizeObserver.observe(container);
}

/**
 * Restore the copy button after its success feedback.
 * @param {HTMLButtonElement} button - Copy button.
 * @param {string} title - Previous title.
 * @returns {void}
 */
function restoreCopyButton(button, title) {
    button.classList.remove('copied');
    button.title = title;
    button.innerHTML = '';
    button.appendChild(createIcon('copy', {class: 'icon-svg'}));
}

/**
 * Copy the currently visible snippet and show bounded success feedback.
 * @param {MouseEvent} event - Copy button click.
 * @returns {Promise<void>}
 */
async function copyCurrentSnippet(event) {
    const button = event.currentTarget;
    const code = document.getElementById('snippet-code');
    if (!(button instanceof HTMLButtonElement) || !code) return;
    try {
        await navigator.clipboard.writeText(code.textContent || '');
        const originalTitle = button.title;
        button.classList.add('copied');
        button.title = t('details.copied');
        button.innerHTML = '';
        button.appendChild(createIcon('check', {class: 'icon-svg'}));
        const toast = document.createElement('span');
        toast.className = 'copy-toast';
        toast.textContent = t('details.copied');
        button.appendChild(toast);
        setTimeout(restoreCopyButton.bind(null, button, originalTitle), 2000);
    } catch (error) {
        console.error('Failed to copy repository snippet', error);
    }
}

/**
 * Bind the stable copy button without accumulating listeners across navigation.
 * @returns {void}
 */
function bindCopyButton() {
    const button = document.getElementById('copy-snippet-btn');
    if (!button) return;
    button.removeEventListener('click', copyCurrentSnippet);
    button.addEventListener('click', copyCurrentSnippet);
}

/**
 * Update dependency or registry snippets for the current repository format.
 * @param {string} path - Browser path.
 * @param {Promise<object|null>} [detailsPromise] - Shared repository details request.
 * @returns {Promise<void>}
 */
export async function updateSnippets(path, detailsPromise) {
	const sequence = ++snippetUpdateSequence;
    const code = document.getElementById('snippet-code');
    if (!code) return;
    const title = document.getElementById('details-card-title');
    const subtitle = document.getElementById('details-card-subtitle');
    const pathParts = path.split('/').filter(Boolean).map(decodePathSegment);
    let details = null;
    try {
        details = detailsPromise ? await detailsPromise : null;
    } catch (error) {
        console.error('Failed to load repository format for snippets', error);
    }
	if (sequence !== snippetUpdateSequence) return;
    const format = getRepositoryFormat(details?.format);
    if (format.id === 'cargo' && pathParts.length > 0) {
        currentSnippets = buildCargoSnippets(pathParts[0]);
        if (title) title.textContent = t('details.cargoTitle');
        if (subtitle) subtitle.textContent = t('details.cargoSubtitle');
    } else {
        const snippetState = await buildMavenSnippets(path, pathParts);
		if (sequence !== snippetUpdateSequence) return;
        currentSnippets = snippetState.snippets;
        if (title) title.textContent = t(snippetState.artifact ? 'details.artifactTitle' : 'details.title');
        if (subtitle) subtitle.textContent = t('details.subtitle');
    }
    renderSnippetTabs(format.snippetTabs);
    code.textContent = currentSnippets[format.snippetTabs[0]] || '';
    bindCopyButton();
}

/**
 * Position the sliding active-tab indicator under the current snippet tab.
 * @param {HTMLElement|null} container - Snippet tabs container.
 * @returns {void}
 */
function syncTabIndicator(container) {
    if (!container) return;
    const activeTab = container.querySelector('.snippet-tab.active');
    if (!(activeTab instanceof HTMLElement)) return;
    let indicator = container.querySelector('.snippet-tab-indicator');
    if (!indicator) {
        indicator = document.createElement('div');
        indicator.className = 'snippet-tab-indicator';
        container.appendChild(indicator);
    }
    indicator.style.left = `${activeTab.offsetLeft}px`;
    indicator.style.width = `${activeTab.offsetWidth}px`;
}
