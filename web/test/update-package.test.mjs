/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {resolve} from 'node:path';
import {fileURLToPath} from 'node:url';
import {brotliCompressSync} from 'node:zlib';
import {unzipSync} from 'fflate';

import {
    chooseUpdateDownloadWorkers,
    isBrotliUpdateTarget,
    legacyZipFilename,
} from '../js/lib/update-package.js';
import {brotliExecutableToZip} from '../js/lib/brotli-zip.js';

test('Brotli update targets are recognized from metadata or extension', () => {
    assert.equal(isBrotliUpdateTarget({file: 'renop-linux-amd64.br'}), true);
    assert.equal(isBrotliUpdateTarget({file: 'package.bin', format: 'brotli'}), true);
    assert.equal(isBrotliUpdateTarget({file: 'legacy.zip'}), false);
    assert.equal(legacyZipFilename('renop-v2-windows-amd64.br'), 'renop-v2-windows-amd64.zip');
});

test('download worker scheduling stays within CPU, memory, and four-worker bounds', () => {
    assert.equal(chooseUpdateDownloadWorkers(1 << 20, 16, 16), 1);
    assert.equal(chooseUpdateDownloadWorkers(20 << 20, 16, 16), 3);
    assert.equal(chooseUpdateDownloadWorkers(200 << 20, 16, 16), 4);
    assert.equal(chooseUpdateDownloadWorkers(200 << 20, 2, 16), 1);
    assert.equal(chooseUpdateDownloadWorkers(200 << 20, 16, 2), 1);
    assert.equal(chooseUpdateDownloadWorkers(200 << 20, 16, 4), 2);
});

test('pure JS Brotli conversion produces a standard executable ZIP', () => {
    const executable = Buffer.from('RenoP executable fixture\n'.repeat(1024));
    const compressed = new Uint8Array(brotliCompressSync(executable));
    const license = Buffer.from('license fixture');
    const zip = brotliExecutableToZip(compressed, 'renop', executable.length, {LICENSE: license});
    const entries = unzipSync(zip);
    assert.deepEqual(Object.keys(entries).sort(), ['LICENSE', 'renop']);
    assert.deepEqual(Buffer.from(entries.renop), executable);
    assert.deepEqual(Buffer.from(entries.LICENSE), license);
    assert.throws(() => brotliExecutableToZip(compressed, 'renop', executable.length + 1), /uncompressed_size_mismatch/);
});

test('release tooling emits raw Brotli packages through the installed Go CLI', () => {
    const repositoryRoot = resolve(fileURLToPath(new URL('../..', import.meta.url)));
    const build = readFileSync(resolve(repositoryRoot, 'build.ps1'), 'utf8');
    const publish = readFileSync(resolve(repositoryRoot, '.github/scripts/publish-update.ps1'), 'utf8');
    const workflow = readFileSync(resolve(repositoryRoot, '.github/workflows/build.yml'), 'utf8');
    assert.match(build, /go install \.\/cmd\/renop-brotli/);
    assert.match(build, /Join-Path \$dist "\$name\.br"/);
    assert.match(build, /-quality 11/);
    assert.match(build, /version\.PreviousCommit=\$previousCommitFull/);
    assert.doesNotMatch(build, /Compress-Archive/);
    assert.match(publish, /-Filter '\*\.br'/);
    assert.match(publish, /application\/x-brotli/);
    assert.match(publish, /previous_commit/);
    assert.match(workflow, /dist\/\*\.br/);
    assert.match(workflow, /dist\/THIRD_PARTY_NOTICES\.md/);
    assert.match(workflow, /previous_commit/);
});
