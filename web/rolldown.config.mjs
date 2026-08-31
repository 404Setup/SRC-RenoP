/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {defineConfig} from 'rolldown';
import {dirname, join} from 'node:path';
import {fileURLToPath} from 'node:url';

const root = dirname(fileURLToPath(import.meta.url));
const outDir = join(root, 'dist');

export default defineConfig({
    input: {
        main: join(root, 'js', 'main.js'),
        'update-package-worker': join(root, 'js', 'workers', 'update-package-worker.js'),
    },
    output: {
        dir: outDir,
        format: 'esm',
        codeSplitting: true,
        entryFileNames: 'js/[name].js',
        chunkFileNames: 'js/chunks/[name]-[hash].js',
        assetFileNames: 'assets/[name]-[hash][extname]',
        minify: true,
    },
    platform: 'browser',
    treeshake: true,
});
