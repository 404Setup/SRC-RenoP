/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {t} from '../i18n.js';
import {createIcon} from '../components.js';
import {decodePathSegment} from './utils.js';

/**
 * Update dependency/repository snippets for the current browser path.
 * Detects Maven artifacts via maven-metadata.xml when possible.
 * @param {string} path
 * @returns {Promise<void>}
 */
export async function updateSnippets(path) {
    const snippetCode = document.getElementById('snippet-code');
    const detailsTitle = document.getElementById('details-card-title');
    if (!snippetCode) return;

    const pathParts = path.split('/').filter(p => p.length > 0).map(decodePathSegment);
    const repoPath = pathParts.length > 0 ? '/' + pathParts[0] : '';
    const currentUrl = window.location.origin + repoPath;

    let isArtifact = false;
    let artifactGroupId = '';
    let artifactId = '';
    let artifactVersion = '';

    if (pathParts.length > 3) {
        try {
            let metaResp = await fetch(`/api/maven/details${path.endsWith('/') ? path : path + '/'}maven-metadata.xml`);
            let metadataXml = '';

            if (metaResp.ok) {
                metaResp = await fetch(`${path.endsWith('/') ? path : path + '/'}maven-metadata.xml`);
                if (metaResp.ok) metadataXml = await metaResp.text();
            } else {
                const parentPath = '/' + pathParts.slice(0, -1).join('/');
                metaResp = await fetch(`/api/maven/details${parentPath}/maven-metadata.xml`);
                if (metaResp.ok) {
                    metaResp = await fetch(`${parentPath}/maven-metadata.xml`);
                    if (metaResp.ok) {
                        metadataXml = await metaResp.text();
                        artifactVersion = pathParts[pathParts.length - 1];
                    }
                }
            }

            if (metadataXml) {
                const parser = new DOMParser();
                const xmlDoc = parser.parseFromString(metadataXml, "text/xml");

                /**
                 * Read the first text value of a tag in the parsed metadata document.
                 * @param {string} tag
                 * @returns {string}
                 */
                const getTagValue = (tag) => {
                    const el = xmlDoc.getElementsByTagName(tag)[0];
                    return el && el.childNodes.length > 0 ? el.childNodes[0].nodeValue : '';
                };

                artifactGroupId = getTagValue("groupId");
                artifactId = getTagValue("artifactId");

                if (!artifactVersion) {
                    const versionsNodes = xmlDoc.getElementsByTagName("version");
                    if (versionsNodes.length > 0) {
                        artifactVersion = versionsNodes[versionsNodes.length - 1].childNodes[0].nodeValue;
                    }
                }

                if (artifactGroupId && artifactId && artifactVersion) {
                    isArtifact = true;
                }
            }
        } catch (e) {
            console.error("Failed to parse metadata", e);
        }
    }

    let snippets = {};
    if (isArtifact) {
        if (detailsTitle) detailsTitle.textContent = t('details.artifactTitle');
        snippets = {
            'maven': `<dependency>\n  <groupId>${artifactGroupId}</groupId>\n  <artifactId>${artifactId}</artifactId>\n  <version>${artifactVersion}</version>\n</dependency>`,
            'gradle-kotlin': `implementation("${artifactGroupId}:${artifactId}:${artifactVersion}")`,
            'gradle-groovy': `implementation '${artifactGroupId}:${artifactId}:${artifactVersion}'`,
            'sbt': `libraryDependencies += "${artifactGroupId}" % "${artifactId}" % "${artifactVersion}"`
        };
    } else {
        if (detailsTitle) detailsTitle.textContent = t('details.title');

        const titleElement = document.querySelector('.nav-title a') || document.querySelector('title');
        const repoNameBase = titleElement ? titleElement.textContent.trim() : 'Renop';
        const cleanRepoName = repoNameBase.replace(/[^a-zA-Z0-9-]/g, '-').toLowerCase();

        const repoIdBase = pathParts.length > 0 ? `${cleanRepoName}-${pathParts[0]}` : cleanRepoName;
        const repoNameStr = pathParts.length > 0 ? `${repoNameBase} - ${pathParts[0]}` : repoNameBase;

        snippets = {
            'maven': `<repository>\n  <id>${repoIdBase}</id>\n  <name>${repoNameStr}</name>\n  <url>${currentUrl}</url>\n</repository>`,
            'gradle-kotlin': `maven {\n  url = uri("${currentUrl}")\n}`,
            'gradle-groovy': `maven {\n  url "${currentUrl}"\n}`,
            'sbt': `resolvers += "${repoNameStr}" at "${currentUrl}"`
        };
    }

    const tabs = document.querySelectorAll('.snippet-tab');
    let activeType = 'maven';
    tabs.forEach(tab => {
        if (tab.classList.contains('active')) {
            activeType = tab.dataset.snippet;
        }

        const newTab = tab.cloneNode(true);
        tab.parentNode.replaceChild(newTab, tab);
    });

    const refreshedTabs = document.querySelectorAll('.snippet-tab');
    const tabsContainer = document.querySelector('.snippet-tabs');

    snippetCode.textContent = snippets[activeType];
    if (tabsContainer) {
        syncTabIndicator(tabsContainer);
        if (!tabsContainer.__resizeObserved) {
            tabsContainer.__resizeObserved = true;
            const ro = new ResizeObserver(() => syncTabIndicator(tabsContainer));
            ro.observe(tabsContainer);
        }
    }

    refreshedTabs.forEach(tab => {
        tab.addEventListener('click', (e) => {
            const snippetType = tab.dataset.snippet;

            refreshedTabs.forEach(t => t.classList.remove('active'));
            tab.classList.add('active');

            if (tabsContainer) {
                const tabLeft = tab.offsetLeft;
                const tabWidth = tab.offsetWidth;
                const containerWidth = tabsContainer.clientWidth;
                const scrollLeft = tabsContainer.scrollLeft;

                if (tabLeft < scrollLeft) {
                    tabsContainer.scrollTo({left: tabLeft, behavior: 'smooth'});
                } else if (tabLeft + tabWidth > scrollLeft + containerWidth) {
                    tabsContainer.scrollTo({left: tabLeft + tabWidth - containerWidth, behavior: 'smooth'});
                }
                syncTabIndicator(tabsContainer);
            }

            const container = snippetCode.closest('.snippet-content');

            if (container.__heightTimeout) clearTimeout(container.__heightTimeout);
            if (container.__fadeTimeout) clearTimeout(container.__fadeTimeout);
            container.style.transition = '';

            const oldHeight = container.getBoundingClientRect().height;
            container.style.height = oldHeight + 'px';

            snippetCode.classList.add('code-changing');

            container.__fadeTimeout = setTimeout(() => {
                container.style.transition = '';
                snippetCode.textContent = snippets[snippetType];

                container.style.height = 'auto';
                const newHeight = container.getBoundingClientRect().height;
                container.style.height = oldHeight + 'px';

                void container.offsetHeight;

                snippetCode.classList.remove('code-changing');

                container.style.transition = 'height 0.22s cubic-bezier(0.2, 0.8, 0.2, 1)';
                container.style.height = newHeight + 'px';

                container.__heightTimeout = setTimeout(() => {
                    container.style.height = 'auto';
                    container.style.transition = '';
                }, 220);
            }, 75);
        });
    });

    const copyBtn = document.getElementById('copy-snippet-btn');
    if (copyBtn) {
        const newCopyBtn = copyBtn.cloneNode(true);
        copyBtn.parentNode.replaceChild(newCopyBtn, copyBtn);

        newCopyBtn.onclick = () => {
            navigator.clipboard.writeText(snippetCode.textContent);
            newCopyBtn.classList.add('copied');
            const originalTitle = newCopyBtn.title;
            newCopyBtn.title = t('details.copied');
            newCopyBtn.innerHTML = '';
            newCopyBtn.appendChild(createIcon('check', {class: 'icon-svg'}));

            let toast = document.createElement('span');
            toast.className = 'copy-toast';
            toast.textContent = t('details.copied');
            newCopyBtn.appendChild(toast);

            setTimeout(() => {
                newCopyBtn.classList.remove('copied');
                newCopyBtn.title = originalTitle;
                newCopyBtn.innerHTML = '';
                newCopyBtn.appendChild(createIcon('copy', {class: 'icon-svg'}));
                toast.remove();
            }, 2000);
        };
    }
}

/**
 * Position the sliding active-tab indicator under the current snippet tab.
 * @param {HTMLElement|null} tabsContainer
 * @returns {void}
 */
function syncTabIndicator(tabsContainer) {
    if (!tabsContainer) return;
    const activeTab = tabsContainer.querySelector('.snippet-tab.active');
    if (!activeTab) return;

    let indicator = tabsContainer.querySelector('.snippet-tab-indicator');
    let isNew = false;
    if (!indicator) {
        indicator = document.createElement('div');
        indicator.className = 'snippet-tab-indicator';
        tabsContainer.appendChild(indicator);
        isNew = true;
    }

    requestAnimationFrame(() => {
        if (isNew) {
            indicator.style.transition = 'none';
            indicator.style.left = `${activeTab.offsetLeft}px`;
            indicator.style.width = `${activeTab.offsetWidth}px`;
            void indicator.offsetHeight;
            indicator.style.transition = '';
        } else {
            indicator.style.left = `${activeTab.offsetLeft}px`;
            indicator.style.width = `${activeTab.offsetWidth}px`;
        }
    });
}
