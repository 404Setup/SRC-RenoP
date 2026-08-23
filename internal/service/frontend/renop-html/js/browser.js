/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {t, updatePageTranslations} from './i18n.js';
import {showAlert} from './alert.js';
import {fetchProto, getAuthHeaders} from './api.js';
import {
	createBreadcrumbLink,
	createBreadcrumbSep,
	createFileItem,
	createMetaGrid,
	RenopDialog
} from './components.js';
import {lockElementHeight, morphElementHeight} from '@renop/ui/height-anim';
import {
    applyAdjustments,
    decodePathSegment,
    encodePathSegment,
    formatBytes,
    initUtils,
} from './browser/utils.js';
import {updateSnippets} from './browser/snippets.js';
import {initUpload, updateUploadZone} from './browser/upload.js';
import {fetchRepoDetails, hideRepoStats, updateRepoStats} from './browser/stats.js';
import {hideCargoRepositoryView, renderCargoRepository} from './browser/cargo.js';
import {hideDockerRepositoryView, renderDockerRepository} from './browser/docker.js';
import {localizeRepositorySearch, updateRepositorySearch} from './browser/search.js';
import {FileDetails, GpgSignatureDetails} from './proto/index.js';
import {getRepositoryFormat} from './repository-formats.js';

const fileList = document.getElementById('file-list');
const fileListContainer = document.getElementById('file-list-container');
const emptyState = document.getElementById('empty-state');
const errorState = document.getElementById('error-state');
const breadcrumbLinks = document.getElementById('breadcrumb-links');
const browserAdjustments = document.querySelector('.browser-adjustments');

let currentLoadSeq = 0;
let listTransitionTimer = null;
let lastDirectoryPath = '';
let currentRepositoryFormat = 'maven';
let currentRepoDetailsPromise = null;
let currentRepoDetails = null;
let currentRepositoryName = '';

const prefetchCache = new Set();

/**
 * Prefetch a URL once via a `<link rel="prefetch">` tag (deduped).
 * @param {string} url
 * @returns {void}
 */
function prefetchUrl(url) {
    if (!url || prefetchCache.has(url)) return;
    prefetchCache.add(url);
    const link = document.createElement('link');
    link.rel = 'prefetch';
    link.href = url;
    document.head.appendChild(link);
}

/**
 * Wait for a CSS transition on `el` to end, or fall back after `timeoutMs`.
 * @param {HTMLElement|null} el
 * @param {number} [timeoutMs=280]
 * @returns {Promise<void>}
 */
function waitForTransition(el, timeoutMs = 280) {
    if (!el) return Promise.resolve();
    return new Promise(resolve => {
        let settled = false;
        const done = () => {
            if (settled) return;
            settled = true;
            el.removeEventListener('transitionend', onEnd);
            resolve();
        };
        const onEnd = (e) => {
            if (e.target === el) done();
        };
        el.addEventListener('transitionend', onEnd);
        setTimeout(done, timeoutMs);
    });
}

/**
 * Start a file-list navigation transition (exit / loading state).
 * @param {'fade'|'forward'|'backward'} [direction='fade']
 * @returns {Promise<void>}
 */
async function beginListTransition(direction = 'fade') {
    if (!fileListContainer) return;
    if (listTransitionTimer) {
        clearTimeout(listTransitionTimer);
        listTransitionTimer = null;
    }

    const hasContent = fileListContainer.classList.contains('is-ready')
        || fileListContainer.classList.contains('is-empty')
        || fileListContainer.classList.contains('is-error');

    lockElementHeight(fileListContainer);

    fileListContainer.classList.remove('is-ready', 'is-empty', 'is-error', 'is-nav-forward', 'is-nav-backward', 'is-nav-fade', 'is-exiting');
    fileListContainer.classList.add('is-loading', `is-nav-${direction}`);

    if (direction === 'backward') {
        return;
    }

    if (hasContent) {
        fileListContainer.classList.add('is-exiting');
        await waitForTransition(fileListContainer.querySelector('.file-list-viewport'), 220);
        fileListContainer.classList.remove('is-exiting');
    }
}

/**
 * Finish a file-list transition into ready, empty, or error mode.
 * @param {'ready'|'empty'|'error'} [mode='ready']
 * @returns {void}
 */
function finishListTransition(mode = 'ready') {
    if (!fileListContainer) return;

    fileListContainer.classList.remove('is-loading', 'is-exiting', 'is-ready', 'is-empty', 'is-error');
    void fileListContainer.offsetWidth;
    fileListContainer.classList.add('is-entering');
    if (mode === 'empty') fileListContainer.classList.add('is-empty');
    else if (mode === 'error') fileListContainer.classList.add('is-error');
    else fileListContainer.classList.add('is-ready');

    morphElementHeight(fileListContainer, null, {duration: 340});

    if (listTransitionTimer) clearTimeout(listTransitionTimer);
    listTransitionTimer = setTimeout(() => {
        fileListContainer.classList.remove('is-entering', 'is-nav-forward', 'is-nav-backward', 'is-nav-fade');
        listTransitionTimer = null;
    }, 420);
}

/**
 * Whether `path` is a single-segment repository root (not a static asset path).
 * @param {string} path
 * @returns {boolean}
 */
function isRepoRootPath(path) {
    const raw = (path || '').split('?')[0].split('#')[0];
    const parts = raw.split('/').filter(p => p.length > 0);
    if (parts.length !== 1) return false;
    const name = parts[0];
    if (name === 'index.html') return false;
    return !['css', 'js', 'svg', 'api', 'javadocs', 'assets'].includes(name);

}

/**
 * Set the empty-state copy for either a repo root or a nested directory.
 * @param {string} path
 * @returns {void}
 */
function updateEmptyStateMessage(path) {
    if (!emptyState) return;
    const isRepoRoot = isRepoRootPath(path);
    const key = isRepoRoot ? 'browser.repoEmptyState' : 'browser.emptyState';
    emptyState.setAttribute('data-i18n', key);
    emptyState.textContent = t(key);
}

/**
 * Toggle visibility of the empty, error, and file-list panels.
 * @param {{empty?: boolean, error?: boolean}} [options]
 * @returns {void}
 */
function setStateVisibility({empty = false, error = false} = {}) {
    if (emptyState) {
        emptyState.hidden = !empty;
        if (!empty) {
            emptyState.style.removeProperty('display');
        }
    }
    if (errorState) {
        errorState.hidden = !error;
        if (!error) {
            errorState.style.removeProperty('display');
        }
    }
    if (fileList) {
        fileList.hidden = empty || error;
        if (empty || error) {
            fileList.style.removeProperty('display');
        }
    }
}

/**
 * Switch the main browser column between Maven's file tree and Cargo's package pages.
 * @param {boolean} cargoMode - Whether the active repository uses Cargo.
 * @returns {void}
 */
function setRepositoryContentMode(customMode) {
    if (fileListContainer) {
        fileListContainer.hidden = customMode;
        if (customMode) {
            fileListContainer.style.removeProperty('height');
            fileListContainer.style.removeProperty('overflow');
            fileListContainer.style.removeProperty('box-sizing');
            fileListContainer.classList.remove(
                'is-loading', 'is-exiting', 'is-entering', 'is-ready', 'is-empty', 'is-error',
                'is-nav-forward', 'is-nav-backward', 'is-nav-fade'
            );
        }
    }
    if (browserAdjustments instanceof HTMLElement) browserAdjustments.hidden = customMode;
    if (currentRepositoryFormat !== 'cargo') hideCargoRepositoryView();
    if (currentRepositoryFormat !== 'docker') hideDockerRepositoryView();
}

/**
 * Animate a file list item out, then remove it from the DOM.
 * @param {HTMLElement|null} item
 * @returns {Promise<void>}
 */
function removeFileItemAnimated(item) {
    return new Promise(resolve => {
        if (!item || !item.parentNode) {
            resolve();
            return;
        }
        if (!item.classList.contains('is-removing')) {
            item.classList.add('is-removing');
        }
        item.style.setProperty('--remove-index', '0');
        let settled = false;
        const finish = () => {
            if (settled) return;
            settled = true;
            if (item.parentNode) item.remove();
            resolve();
        };
        item.addEventListener('animationend', finish, {once: true});
        setTimeout(finish, 420);
    });
}

/**
 * If no non-removing file items remain, show the empty state for `path`.
 * @param {string} path
 * @returns {void}
 */
function showEmptyIfNoFiles(path) {
    if (!fileList) return;
    const remaining = Array.from(fileList.children).filter(li => !li.classList.contains('is-removing'));
    if (remaining.length === 0) {
        fileList.innerHTML = '';
        updateEmptyStateMessage(path);
        setStateVisibility({empty: true});
        finishListTransition('empty');
    }
}

initUtils(() => loadDirectory(window.location.pathname));
initUpload();

/**
 * Render or reconcile the breadcrumb trail and up-directory control for `path`.
 * @param {string} path
 * @returns {void}
 */
export function renderBreadcrumb(path) {
    if (!breadcrumbLinks) return;

    const upDirBtn = document.getElementById('up-dir-btn');
    if (upDirBtn) {
        upDirBtn.removeEventListener('click', navigate);
        upDirBtn.addEventListener('click', navigate);

        if (!path || path === '/') {
            const hideNow = () => {
                upDirBtn.hidden = true;
                upDirBtn.style.display = 'none';
                upDirBtn.classList.remove('is-visible', 'is-hiding');
            };
            if (upDirBtn.classList.contains('is-visible') && !upDirBtn.classList.contains('is-hiding')) {
                upDirBtn.classList.remove('is-visible');
                upDirBtn.classList.add('is-hiding');
                const onHideEnd = (e) => {
                    if (e && e.target !== upDirBtn) return;
                    hideNow();
                };
                upDirBtn.addEventListener('animationend', onHideEnd, {once: true});
                setTimeout(hideNow, 220);
            } else if (!upDirBtn.classList.contains('is-hiding')) {
                hideNow();
            }
        } else {
            upDirBtn.classList.remove('is-hiding');
            upDirBtn.hidden = false;
            upDirBtn.style.display = 'inline-flex';
            if (!upDirBtn.classList.contains('is-visible')) {
                requestAnimationFrame(() => upDirBtn.classList.add('is-visible'));
            }
            const pathParts = path.split('/').filter(p => p.length > 0);
            if (pathParts.length <= 1) {
                upDirBtn.href = '/';
            } else {
                pathParts.pop();
                upDirBtn.href = '/' + pathParts.join('/');
            }
        }
    }

    const parts = path && path !== '/'
        ? path.split('/').filter(p => p.length > 0)
        : [];

    const desired = [];
    desired.push({
        type: 'link',
        href: '/',
        text: t('browser.root'),
        isCurrent: parts.length === 0,
        i18nKey: 'browser.root'
    });
    let currentPath = '';
    for (let i = 0; i < parts.length; i++) {
        currentPath += '/' + parts[i];
        const isLast = i === parts.length - 1;
        const decoded = decodePathSegment(parts[i]);
        const packagesRoute = currentRepositoryFormat === 'cargo' && i === 1 && decoded === 'packages';
        desired.push({type: 'sep', text: '/'});
        desired.push({
            type: 'link',
            href: currentPath,
            text: packagesRoute ? t('cargo.packagesTitle') : decoded,
            isCurrent: isLast,
            i18nKey: packagesRoute ? 'cargo.packagesTitle' : undefined
        });
    }

    const existing = Array.from(breadcrumbLinks.children);
    let matchCount = 0;
    for (let i = 0; i < Math.min(existing.length, desired.length); i++) {
        const el = existing[i];
        const des = desired[i];
        if (des.type === 'sep' && el.classList.contains('breadcrumb-sep')) {
            matchCount++;
        } else if (des.type === 'link' && el.classList.contains('breadcrumb-seg') && el.getAttribute('href') === des.href) {
            el.classList.toggle('is-current', des.isCurrent);
            if (des.isCurrent) el.setAttribute('aria-current', 'page');
            else el.removeAttribute('aria-current');
            if (des.i18nKey) {
                el.setAttribute('data-i18n', des.i18nKey);
                el.textContent = des.text;
            } else {
                el.removeAttribute('data-i18n');
            }
            matchCount++;
        } else {
            break;
        }
    }

    for (let i = existing.length - 1; i >= matchCount; i--) {
        existing[i].remove();
    }

    for (let i = matchCount; i < desired.length; i++) {
        const des = desired[i];
        if (des.type === 'sep') {
            breadcrumbLinks.appendChild(createBreadcrumbSep(i));
        } else {
            breadcrumbLinks.appendChild(createBreadcrumbLink({
                href: des.href,
                text: des.text,
                isCurrent: des.isCurrent,
                i18nKey: des.i18nKey,
                index: i,
                onClick: navigate
            }));
        }
    }

    requestAnimationFrame(() => {
        breadcrumbLinks.scrollLeft = breadcrumbLinks.scrollWidth;
    });
}

/**
 * Build a file/directory list item with navigate, delete, and prefetch hooks.
 * @param {{name: string, type: string, content_length?: number, signed?: boolean, format?: string}} file
 * @param {number} index
 * @param {string} path current directory path
 * @returns {HTMLElement}
 */
function createFileItemElement(file, index, path) {
    const formattedSize = file.type === 'FILE' && file.content_length !== undefined ? formatBytes(file.content_length) : null;
    const item = createFileItem(file, index, path, {
        formattedSize,
        allowDelete: currentRepositoryFormat !== 'cargo',
        onNavigate: (detail) => navigate(detail.event),
        onDelete: async (detail) => {
            if (await window.showConfirm(t('browser.confirmDelete', {name: detail.fileName}))) {
                try {
                    const delResp = await fetch(detail.fullPath, {method: 'DELETE', headers: getAuthHeaders()});
                    if (delResp.ok) {
                        showAlert(t('browser.deleteSuccess', {name: detail.fileName}), 'success');
                        await removeFileItemAnimated(item);
                        showEmptyIfNoFiles(path);
                    } else {
                        showAlert(t('browser.failedDelete'), 'error');
                    }
                } catch (err) {
                    console.error('Delete failed', err);
                    showAlert(t('browser.failedDelete'), 'error');
                }
            }
		},
		onSignature: detail => openSignatureDetails(detail)
    });

    const fullPath = (path.endsWith('/') ? path : path + '/') + encodePathSegment(file.name);
    const link = item.querySelector('.file-item-link');
    if (link && file.type === 'DIRECTORY') {
        /**
         * Prefetch directory metadata when the user approaches its link.
         * @returns {void}
         */
        function prefetchDirectoryDetails() {
            prefetchUrl(`/api/repositories/details${fullPath}`);
        }
        link.addEventListener('mouseenter', prefetchDirectoryDetails, {once: true});
    }

    const previewBtn = item.querySelector('.file-action-btn--docs, .file-action-btn--preview');
    if (previewBtn) {
        previewBtn.addEventListener('mouseenter', () => prefetchUrl(previewBtn.href), {once: true});
    }
    return item;
}

/**
 * Format a signature timestamp for the current browser locale.
 * @param {number|string} value Unix milliseconds
 * @returns {string}
 */
function formatSignatureTime(value) {
	const timestamp = Number(value);
	if (!Number.isFinite(timestamp) || timestamp <= 0) return t('common.none');
	return new Date(timestamp).toLocaleString();
}

/**
 * Fetch and display verified GPG signature details for an artifact path.
 * @param {{fullPath: string}} detail
 * @returns {Promise<void>}
 */
async function openSignatureDetails({fullPath}) {
	try {
		const {response, data} = await fetchProto(`/api/repositories/signatures${fullPath}`, GpgSignatureDetails);
		if (!response.ok || !data) {
			showAlert(t('browser.signatureLoadFailed'), 'error');
			return;
		}
		const details = createMetaGrid([
			{label: t('browser.signatureIdentity'), value: data.primary_identity || t('common.none')},
			{label: t('browser.signatureFingerprint'), value: data.fingerprint || '', isCode: true},
			{label: t('browser.signatureKeyId'), value: data.key_id || '', isCode: true},
			{label: t('browser.signatureUploader'), value: data.uploader || ''},
			{label: t('browser.signatureCreated'), value: formatSignatureTime(data.signature_created_at)},
			{label: t('browser.signatureVerified'), value: formatSignatureTime(data.verified_at)},
			{label: t('browser.signatureHash'), value: data.hash_algorithm || ''},
			{label: t('browser.signatureAlgorithm'), value: data.public_key_algorithm || ''}
		]);
		RenopDialog.show({
			id: 'gpg-signature-details-dialog',
			maxWidth: '620px',
			icon: 'fileLock',
			title: t('browser.signatureDetails'),
			subtitle: data.artifact_path || '',
			body: details,
			footer: [{
				text: t('common.ok'),
				className: 'action-btn primary-btn',
				onClick: (event, dialog) => dialog.close(true)
			}]
		});
	} catch (error) {
		console.error('Failed to load GPG signature details', error);
		showAlert(t('browser.signatureLoadFailed'), 'error');
	}
}

/**
 * Insert `node` at a visible (non-removing) index in `list`.
 * @param {HTMLElement} list
 * @param {HTMLElement} node
 * @param {number} visibleIndex
 * @returns {void}
 */
function insertAtVisibleIndex(list, node, visibleIndex) {
    const visible = Array.from(list.children).filter(
        c => c !== node && !c.classList.contains('is-removing')
    );
    const ref = visible[visibleIndex];
    if (ref) {
        list.insertBefore(node, ref);
        return;
    }
    const firstRemoving = Array.from(list.children).find(
        c => c !== node && c.classList.contains('is-removing')
    );
    if (firstRemoving) {
        list.insertBefore(node, firstRemoving);
    } else {
        list.appendChild(node);
    }
}

/**
 * Capture bounding rects of visible file items keyed by `data-file-name`.
 * @param {HTMLElement|null} list
 * @returns {Map<string, DOMRect>}
 */
function captureFileItemRects(list) {
    const rects = new Map();
    if (!list) return rects;
    Array.from(list.children).forEach(li => {
        if (li.classList.contains('is-removing')) return;
        const name = li.dataset.fileName;
        if (!name) return;
        rects.set(name, li.getBoundingClientRect());
    });
    return rects;
}

/**
 * FLIP-animate surviving file items from previous rects to their new positions.
 * @param {HTMLElement|null} list
 * @param {Map<string, DOMRect>|null} firstRects
 * @returns {void}
 */
function animateFileListFLIP(list, firstRects) {
    if (!list || !firstRects || firstRects.size === 0) return;
    if (typeof window !== 'undefined' && window.matchMedia
        && window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
        return;
    }

    Array.from(list.children).forEach(li => {
        if (li.classList.contains('is-removing') || li.classList.contains('file-item--added')) {
            return;
        }
        const name = li.dataset.fileName;
        if (!name) return;
        const firstRect = firstRects.get(name);
        if (!firstRect) return;

        const lastRect = li.getBoundingClientRect();
        const dx = firstRect.left - lastRect.left;
        const dy = firstRect.top - lastRect.top;
        if (dx === 0 && dy === 0) return;

        li.style.transition = 'none';
        li.style.transform = `translate3d(${dx}px, ${dy}px, 0)`;

        requestAnimationFrame(() => {
            requestAnimationFrame(() => {
                li.style.transition = 'transform 0.32s cubic-bezier(0.16, 1, 0.3, 1)';
                li.style.transform = 'translate3d(0, 0, 0)';
                const clear = () => {
                    li.style.transition = '';
                    li.style.transform = '';
                    li.removeEventListener('transitionend', onEnd);
                };
                const onEnd = (e) => {
                    if (e.target === li && e.propertyName === 'transform') clear();
                };
                li.addEventListener('transitionend', onEnd);
                setTimeout(clear, 360);
            });
        });
    });
}

/**
 * Diff-update the file list in place (add/remove/reorder) with animations.
 * @param {Array<{name: string, type: string, content_length?: number, signed?: boolean}>} filesToDisplay
 * @param {string} path
 * @returns {void}
 */
function renderFileListReconciled(filesToDisplay, path) {
    if (!fileList) return;

    const firstRects = captureFileItemRects(fileList);
    const lockedHeight = fileListContainer
        ? lockElementHeight(fileListContainer)
        : 0;

    const existingMap = new Map();
    Array.from(fileList.children).forEach(li => {
        const name = li.dataset.fileName;
        if (name) existingMap.set(name, li);
    });

    const newNames = new Set(filesToDisplay.map(f => f.name));

    let removedCount = 0;
    existingMap.forEach((li, name) => {
        if (!newNames.has(name) && !li.classList.contains('is-removing')) {
            li.classList.add('is-removing');
            const idx = removedCount++;
            li.style.setProperty('--remove-index', String(idx));
            const totalTimeout = 380 + idx * 35;
            li.addEventListener('animationend', () => li.remove(), {once: true});
            setTimeout(() => {
                if (li.parentNode) li.remove();
            }, totalTimeout);
        }
    });

    let addedCount = 0;
    filesToDisplay.forEach((file, index) => {
        const existingLi = existingMap.get(file.name);
        if (existingLi && !existingLi.classList.contains('is-removing')) {
            existingLi.dataset.index = String(index);
            const shouldBeSigned = file.signed === true;
            const isSigned = existingLi.hasAttribute('signed');
            if (shouldBeSigned !== isSigned) {
                if (shouldBeSigned) {
                    existingLi.setAttribute('signed', '');
                } else {
                    existingLi.removeAttribute('signed');
                }
            }
            const badge = existingLi.querySelector('.file-type-badge');
            if (badge) {
                const i18nKey = badge.getAttribute('data-i18n');
                if (i18nKey) {
                    const translation = t(i18nKey);
                    if (translation) {
                        badge.textContent = translation;
                    }
                }
            }
            insertAtVisibleIndex(fileList, existingLi, index);
        } else {
            const li = createFileItemElement(file, index, path);
            li.classList.add('file-item--added');
            li.style.setProperty('--add-index', String(addedCount++));
            insertAtVisibleIndex(fileList, li, index);

            li.addEventListener('animationend', () => {
                li.classList.remove('file-item--added');
            }, {once: true});
            setTimeout(() => li.classList.remove('file-item--added'), 480 + addedCount * 35);
        }
    });

    animateFileListFLIP(fileList, firstRects);

    if (fileListContainer && lockedHeight > 0) {
        morphElementHeight(fileListContainer, null, {duration: 340});
    }
}

/**
 * Fetch and display directory contents for `path`, with transitions and stats.
 * @param {string} path
 * @returns {Promise<void>}
 */
export async function loadDirectory(path) {
    const seq = ++currentLoadSeq;
    const pathParts = path.split('/').filter(p => p.length > 0);
    const repositoryName = pathParts[0] || '';
    const canReuseCargoDetails = (currentRepositoryFormat === 'cargo' || currentRepositoryFormat === 'docker') &&
        repositoryName !== '' && repositoryName === currentRepositoryName && currentRepoDetails !== null;

    let direction = 'fade';
    const isSameDirectory = (lastDirectoryPath === path);
    if (lastDirectoryPath && !isSameDirectory) {
        const normLast = lastDirectoryPath.endsWith('/') ? lastDirectoryPath : lastDirectoryPath + '/';
        const normPath = path.endsWith('/') ? path : path + '/';
        if (normPath.startsWith(normLast)) {
            direction = 'forward';
        } else if (normLast.startsWith(normPath)) {
            direction = 'backward';
        }
    }
    lastDirectoryPath = path;

    const exitPromise = (isSameDirectory || canReuseCargoDetails)
        ? Promise.resolve()
        : beginListTransition(direction);

    renderBreadcrumb(path);

    let repoDetailsPromise;
    if (pathParts.length >= 1 && pathParts[0] !== 'index.html') {
        repoDetailsPromise = canReuseCargoDetails
            ? Promise.resolve(currentRepoDetails)
            : fetchRepoDetails(pathParts[0]);
        if (!canReuseCargoDetails) void updateRepoStats(pathParts[0], repoDetailsPromise);
    } else {
        currentRepoDetails = null;
        currentRepositoryName = '';
        void hideRepoStats();
    }
	currentRepoDetailsPromise = repoDetailsPromise || null;
    if (!canReuseCargoDetails) {
        void updateSnippets(path, repoDetailsPromise);
        void updateUploadZone(path, repoDetailsPromise);
    }

    try {
        const [directoryResult, repoDetails] = await Promise.all([
            canReuseCargoDetails
                ? Promise.resolve(null)
                : fetchProto(`/api/repositories/details${path}`, FileDetails),
            repoDetailsPromise || Promise.resolve(null)
        ]);
        await exitPromise;
        if (seq !== currentLoadSeq) return;

        currentRepositoryFormat = getRepositoryFormat(repoDetails?.format).id;
		currentRepoDetails = repoDetails;
		currentRepositoryName = repoDetails ? repositoryName : '';
		const isCargoRepository = currentRepositoryFormat === 'cargo' && pathParts.length >= 1;
		const isDockerRepository = currentRepositoryFormat === 'docker' && pathParts.length >= 1;
		setRepositoryContentMode(isCargoRepository || isDockerRepository);
		updateRepositorySearch(repoDetails ? pathParts[0] : '', currentRepositoryFormat, navigateToPath);
		renderBreadcrumb(path);

		if (isCargoRepository) {
			setStateVisibility({empty: false, error: false});
			await renderCargoRepository(path, repoDetails, navigateToPath);
			return;
		}

		if (isDockerRepository) {
			setStateVisibility({empty: false, error: false});
			await renderDockerRepository(path, repoDetails, navigateToPath);
			return;
		}

        const {response, data} = directoryResult || {};

        if (!response?.ok || !data) {
            let msg = `HTTP error! status: ${response?.status || 500}`;
            try {
                const text = await response.text();
                if (text && text.includes('Artifact blocked')) msg = text;
            } catch {
            }
            throw new Error(msg);
        }

        if (seq !== currentLoadSeq) return;

        setStateVisibility({empty: false, error: false});

        let filesToDisplay = data.files || [];
        filesToDisplay = applyAdjustments(filesToDisplay);

        if (filesToDisplay.length === 0) {
            if (fileList) fileList.innerHTML = '';
            updateEmptyStateMessage(path);
            setStateVisibility({empty: true});
            finishListTransition('empty');
            return;
        }

        if (isSameDirectory && fileList && fileList.children.length > 0) {
            renderFileListReconciled(filesToDisplay, path);
        } else {
            if (fileList) fileList.innerHTML = '';
            filesToDisplay.forEach((file, index) => {
                const li = createFileItemElement(file, index, path);
                fileList.appendChild(li);
            });
            finishListTransition('ready');
        }

    } catch (error) {
        await exitPromise;
        if (seq !== currentLoadSeq) return;
        console.error('Error fetching directory details:', error);
		setRepositoryContentMode(false);
		updateRepositorySearch('', 'maven', navigateToPath);
        if (fileList) fileList.innerHTML = '';
        if (errorState) {
            errorState.textContent = (error.message && error.message.includes('Artifact blocked'))
                ? error.message
                : t('browser.errorState');
        }
        setStateVisibility({error: true});
        finishListTransition('error');
    }
}

/**
 * Navigate to an application path and refresh the format-aware repository view.
 * @param {string} path - Absolute same-origin path.
 * @param {boolean} [replace=false] - Replace rather than append browser history.
 * @returns {void}
 */
export function navigateToPath(path, replace = false) {
    if (!path || typeof path !== 'string' || !path.startsWith('/')) return;
    if (replace) window.history.replaceState(null, '', path);
    else window.history.pushState(null, '', path);
    void loadDirectory(path);
}

/**
 * Handle in-app navigation from a breadcrumb or file link click.
 * @param {Event} event
 * @returns {void}
 */
export function navigate(event) {
    event.preventDefault();
    const url = new URL(event.currentTarget.href);
    navigateToPath(url.pathname);
}

window.addEventListener('popstate', () => {
    loadDirectory(window.location.pathname);
});

/**
 * Re-render browser text and format-specific snippets after a locale change.
 * @returns {void}
 */
function handleBrowserLanguageChanged() {
    const path = lastDirectoryPath || window.location.pathname;
    renderBreadcrumb(path);
    updatePageTranslations();
    if (emptyState && !emptyState.hidden) {
        updateEmptyStateMessage(path);
    }
    if (errorState && !errorState.hidden && errorState.getAttribute('data-i18n') === 'browser.errorState') {
        errorState.textContent = t('browser.errorState');
    }
    if (fileList) {
        fileList.querySelectorAll('renop-file-item').forEach(item => {
            if (typeof item.render === 'function') {
                item.render();
            }
        });
    }
	void updateSnippets(path, currentRepoDetailsPromise);
	localizeRepositorySearch();
}

window.addEventListener('languageChanged', handleBrowserLanguageChanged);
