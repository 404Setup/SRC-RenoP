/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

let reversedFileOrder = localStorage.getItem('reversedFileOrder') === 'true';
let displayHashFiles = localStorage.getItem('displayHashFiles') !== 'false';

/**
 * Encode a single path segment for use in repository URLs.
 * Like `encodeURIComponent`, but keeps '+' raw so Maven/Fabric versions
 * such as `1.2.0+26.1` are not turned into `1.2.0%2B26.1`.
 * @param {string} segment
 * @returns {string}
 */
export function encodePathSegment(segment) {
    return encodeURIComponent(segment).replace(/%2B/g, '+');
}

/**
 * Encode each non-empty path segment (preserves '+'; see encodePathSegment).
 * @param {string} path
 * @returns {string}
 */
export function encodeRelativePath(path) {
    return String(path || '')
        .split('/')
        .filter(part => part.length > 0)
        .map(encodePathSegment)
        .join('/');
}

/**
 * Decode a single URL path segment; returns the original string on failure.
 * @param {string} segment
 * @returns {string}
 */
export function decodePathSegment(segment) {
    try {
        return decodeURIComponent(segment);
    } catch {
        return segment;
    }
}

/**
 * Split a browser route into logical breadcrumb segments for its repository format.
 * Docker image names may contain slashes, so every segment after the repository
 * belongs to one image rather than representing navigable directories.
 * @param {string} path
 * @param {string} format
 * @returns {string[]}
 */
export function breadcrumbPathParts(path, format) {
    const parts = String(path || '').split('/').filter(part => part.length > 0);
    if (format === 'docker' && parts.length > 1) {
        return [parts[0], parts.slice(1).join('/')];
    }
    return parts;
}

/**
 * Whether the file list sort order is reversed.
 * @returns {boolean}
 */
export function getReversedFileOrder() {
    return reversedFileOrder;
}

/**
 * Whether utility/hash sidecar files are shown in the file list.
 * @returns {boolean}
 */
export function getDisplayHashFiles() {
    return displayHashFiles;
}

/**
 * Initialize the adjustments menu (sort order, utility files) and wire refresh.
 * @param {(() => void)|null|undefined} onRefresh called when a preference changes
 * @returns {void}
 */
export function initUtils(onRefresh) {
    const adjustmentsBtn = document.getElementById('adjustments-btn');
    const adjustmentsMenu = document.getElementById('adjustments-menu');
    const sortOrderCheckbox = document.getElementById('sort-order-checkbox');
    const utilityFilesCheckbox = document.getElementById('utility-files-checkbox');

    if (sortOrderCheckbox) sortOrderCheckbox.checked = reversedFileOrder;
    if (utilityFilesCheckbox) utilityFilesCheckbox.checked = displayHashFiles;

    let panelOpen = false;

    /**
     * Open the adjustments popover and position it above or below the button.
     * @returns {void}
     */
    function openPanel() {
        if (panelOpen) return;
        panelOpen = true;
        adjustmentsMenu.classList.remove('is-closing');
        adjustmentsMenu.classList.add('is-open');
        adjustmentsMenu.setAttribute('aria-hidden', 'false');
        adjustmentsBtn.classList.add('active');
        adjustmentsBtn.setAttribute('aria-expanded', 'true');

        const rect = adjustmentsBtn.getBoundingClientRect();
        if (rect.bottom + 240 > window.innerHeight) {
            adjustmentsMenu.style.top = 'auto';
            adjustmentsMenu.style.bottom = 'calc(100% + 10px)';
            adjustmentsMenu.style.transformOrigin = 'bottom right';
        } else {
            adjustmentsMenu.style.top = 'calc(100% + 10px)';
            adjustmentsMenu.style.bottom = 'auto';
            adjustmentsMenu.style.transformOrigin = 'top right';
        }
    }

    /**
     * Close the adjustments popover with a closing animation.
     * @returns {void}
     */
    function closePanel() {
        if (!panelOpen) return;
        panelOpen = false;
        adjustmentsMenu.classList.remove('is-open');
        adjustmentsMenu.classList.add('is-closing');
        adjustmentsMenu.setAttribute('aria-hidden', 'true');
        adjustmentsBtn.classList.remove('active');
        adjustmentsBtn.setAttribute('aria-expanded', 'false');
        adjustmentsMenu.addEventListener('animationend', () => {
            adjustmentsMenu.classList.remove('is-closing');
        }, {once: true});
    }

    if (adjustmentsBtn && adjustmentsMenu) {
        adjustmentsBtn.addEventListener('click', (e) => {
            e.stopPropagation();
            if (panelOpen) {
                closePanel();
            } else {
                openPanel();
            }
        });
    }

    document.addEventListener('click', (e) => {
        if (panelOpen && !adjustmentsMenu.contains(e.target) && e.target !== adjustmentsBtn) {
            closePanel();
        }
    });

    if (sortOrderCheckbox) {
        sortOrderCheckbox.addEventListener('change', () => {
            reversedFileOrder = sortOrderCheckbox.checked;
            localStorage.setItem('reversedFileOrder', reversedFileOrder);
            if (onRefresh) onRefresh();
        });
    }

    if (utilityFilesCheckbox) {
        utilityFilesCheckbox.addEventListener('change', () => {
            displayHashFiles = utilityFilesCheckbox.checked;
            localStorage.setItem('displayHashFiles', displayHashFiles);
            if (onRefresh) onRefresh();
        });
    }
}

/**
 * Compare two version-like strings (numeric segments, then pre-release suffix).
 * @param {string} rawA
 * @param {string} rawB
 * @returns {number} negative if a < b, 0 if equal, positive if a > b
 */
export const compareSemver = (rawA, rawB) => {
    const a = rawA.split('-');
    const b = rawB.split('-');
    const pa = a[0].split('.');
    const pb = b[0].split('.');

    for (let idx = 0; idx < Math.max(pa.length, pb.length); idx++) {
        const pStrA = pa[idx];
        const pStrB = pb[idx];
        if (pStrA === undefined) return -1;
        if (pStrB === undefined) return 1;

        const na = Number(pStrA);
        const nb = Number(pStrB);

        if (!isNaN(na) && !isNaN(nb)) {
            if (na > nb) return 1;
            if (nb > na) return -1;
        } else {
            const strCmp = pStrA.localeCompare(pStrB, undefined, {numeric: true, sensitivity: 'base'});
            if (strCmp !== 0) return strCmp;
        }
    }

    if (a[1] && b[1]) {
        return a.slice(1).join('-').localeCompare(b.slice(1).join('-'), undefined, {
            numeric: true,
            sensitivity: 'base'
        });
    }
    return !a[1] && b[1] ? 1 : (a[1] && !b[1] ? -1 : 0);
};

/**
 * Filter hash sidecars and sort files by type, version, and name preferences.
 * @param {Array<{name: string, type: string, signed?: boolean}>} files
 * @returns {Array<{name: string, type: string, signed?: boolean}>}
 */
export function applyAdjustments(files) {
    let result = files;

    if (!displayHashFiles) {
        result = result.filter(file =>
            !['.asc', '.md5', '.sha1', '.sha256', '.sha512'].some(ext => file.name.endsWith(ext))
        );
    }

    result = [...result].sort((a, b) => {
        if (a.type === 'DIRECTORY' && b.type !== 'DIRECTORY') return -1;
        if (a.type !== 'DIRECTORY' && b.type === 'DIRECTORY') return 1;

        let cmp = 0;
        if (a.type === 'DIRECTORY' && b.type === 'DIRECTORY') {
            cmp = compareSemver(a.name, b.name);
        }
        if (cmp === 0) {
            cmp = a.name.localeCompare(b.name, undefined, {numeric: true, sensitivity: 'base'});
        }

        return reversedFileOrder ? -cmp : cmp;
    });

    return result;
}

/**
 * Format a byte count as a human-readable size string.
 * @param {number} bytes
 * @param {number} [decimals=2]
 * @returns {string}
 */
export function formatBytes(bytes, decimals = 2) {
    if (!+bytes) return '0 Bytes';
    const k = 1024;
    const dm = decimals < 0 ? 0 : decimals;
    const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB', 'PB', 'EB', 'ZB', 'YB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return `${parseFloat((bytes / Math.pow(k, i)).toFixed(dm))} ${sizes[i]}`;
}
