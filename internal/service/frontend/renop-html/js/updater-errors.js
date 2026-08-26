/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

import {t} from './i18n.js';

const updaterErrorKeys = Object.freeze({
    forbidden: 'error.forbidden',
    insufficient_space: 'updater.insufficientDiskSpace',
    missing_file: 'updater.selectZipFirst',
    install_busy: 'error.installInProgress',
    invalid_package: 'updater.invalidZip',
    incompatible_binary: 'error.incompatibleBinary',
    package_too_large: 'error.requestEntityTooLarge',
    package_processing_failed: 'updaterNotice.installFailedTitle',
    check_failed: 'updaterNotice.checkFailedTitle',
    notification_failed: 'common.error',
    restart_failed: 'updaterNotice.restartFailedTitle',
});

/**
 * Resolve an updater Response or upload result to a trusted localized error.
 * @param {Response|{status?: number, errorCode?: string}|null|undefined} source - Failed request metadata.
 * @param {string} fallbackKey - Translation key used when no stable code is available.
 * @returns {string}
 */
export function updaterErrorMessage(source, fallbackKey) {
    const code = source?.headers?.get?.('X-Renop-Error-Code') || source?.errorCode || '';
    if (updaterErrorKeys[code]) return t(updaterErrorKeys[code]);
    const status = Number(source?.status) || 0;
    if (status === 0) return t(fallbackKey);
    if (status === 413) return t('error.requestEntityTooLarge');
    if (status === 507) return t('updater.insufficientDiskSpace');
    if (status === 409) return t('error.installInProgress');
    if (status === 403) return t('error.forbidden');
    return t(fallbackKey);
}
