/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {dirname, join, resolve} from 'node:path';
import test from 'node:test';
import {fileURLToPath} from 'node:url';

const frontendRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');

test('repository settings expose localized download-statistics controls', () => {
    const source = readFileSync(join(frontendRoot, 'js/repositories.js'), 'utf8');
    for (const required of [
        "apiRequest('/api/settings/repositories/download-statistics')",
        'buildDownloadStatisticsControls(repoKey, repo)',
        '/download-statistics',
        "method: 'PUT'",
        "{method: 'DELETE'}",
        'repos.resetDownloadStatisticsConfirm',
        'responseErrorMessage',
    ]) {
        assert.ok(source.includes(required), `repository statistics UI is missing ${required}`);
    }
});
