/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync, mkdtempSync, mkdirSync, writeFileSync, rmSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {createHash} from 'node:crypto';
import {createServer} from 'node:http';
import {dirname, resolve} from 'node:path';
import {fileURLToPath} from 'node:url';
import {brotliCompressSync} from 'node:zlib';
import {spawn, spawnSync} from 'node:child_process';
import {unzipSync} from 'fflate';

import {chooseUpdateDownloadWorkers, isBrotliUpdateTarget, legacyZipFilename,} from '../js/lib/update-package.js';
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

test('release tooling decouples bounded compilation from raw Brotli packaging', () => {
    const repositoryRoot = resolve(fileURLToPath(new URL('../..', import.meta.url)));
    const build = readFileSync(resolve(repositoryRoot, 'build.ps1'), 'utf8');
    const targetWorker = readFileSync(resolve(repositoryRoot, 'scripts/build-target.ps1'), 'utf8');
    const compressionWorker = readFileSync(resolve(repositoryRoot, 'scripts/compress-target.ps1'), 'utf8');
    const publish = readFileSync(resolve(repositoryRoot, '.github/scripts/publish-update.ps1'), 'utf8');
    const workflow = readFileSync(resolve(repositoryRoot, '.github/workflows/build.yml'), 'utf8');
    assert.match(build, /go install \.\/cmd\/renop-brotli/);
    assert.match(build, /Join-Path \$dist "\$name\.br"/);
    assert.match(build, /\[ValidateRange\(1, 4\)\]/);
    assert.match(build, /\[ValidateRange\(1, 8\)\]/);
    assert.match(build, /\$activeCompileWorkers\.Count -lt \$BuildConcurrency/);
    assert.match(build, /\$activeCompressionWorkers\.Count -lt \$CompressionConcurrency/);
    assert.match(build, /Start-TargetWorker -Job \$job -WorkerScript \$workerScript -Phase compile/);
    assert.match(build, /Start-TargetWorker -Job \$job -WorkerScript \$compressionWorkerScript -Phase compress/);
    assert.match(targetWorker, /& go build/);
    assert.doesNotMatch(targetWorker, /brotli|quality 11/i);
    assert.match(compressionWorker, /& \$brotliTool/);
    assert.match(compressionWorker, /-quality 11/);
    assert.match(build, /version\.PreviousCommit=\$previousCommitFull/);
    assert.doesNotMatch(build, /Compress-Archive/);
    assert.match(publish, /-Filter '\*\.br'/);
    assert.match(publish, /application\/x-brotli/);
    assert.match(publish, /previous_commit/);
    assert.match(workflow, /dist\/\*\.br/);
    assert.match(workflow, /^\s+THIRD_PARTY_NOTICES\.md$/m);
    assert.doesNotMatch(publish, /README\.md|THIRD_PARTY_NOTICES\.md|LICENSE/);
    assert.match(workflow, /previous_commit/);
    assert.match(publish, /\$nightlyPackageRetention = 9/);
    assert.match(publish, /Get-NightlyReleases -CurrentRelease \$currentRelease -ExistingReleases/);
});

test('Actions matrices compile every target before packaging and assemble only a complete verified payload', () => {
    const repositoryRoot = resolve(fileURLToPath(new URL('../..', import.meta.url)));
    const workflow = readFileSync(resolve(repositoryRoot, '.github/workflows/build.yml'), 'utf8');
    const compile = workflow.slice(workflow.indexOf('  build:'), workflow.indexOf('  package:'));
    const packaging = workflow.slice(workflow.indexOf('  package:'), workflow.indexOf('  assemble:'));
    assert.match(compile, /needs: \[metadata, prepare\]/);
    assert.match(packaging, /needs: \[metadata, build\]/);
    for (const job of [compile, packaging]) {
        assert.match(job, /fail-fast: false/);
        assert.match(job, /include: \$\{\{ fromJSON\(needs.metadata.outputs.targets\) \}\}/);
        assert.doesNotMatch(job, /max-parallel:|continue-on-error:/);
    }
    assert.match(compile, /-Target \$env:BUILD_TARGET -SkipPreparation -nb/);
    assert.match(workflow, /needs: \[metadata, assemble\]/);
    assert.doesNotMatch(workflow, /pattern: ['"]\*['"]/);
    const directory = mkdtempSync(resolve(tmpdir(), 'renop-matrix-test-'));
    try {
        const matrix = spawnSync('pwsh', ['-NoProfile', '-Command',
            '(Import-PowerShellDataFile ./scripts/build-targets.psd1).Targets | ConvertTo-Json -Compress'],
        {cwd: repositoryRoot, encoding: 'utf8'});
        assert.equal(matrix.status, 0, matrix.stderr);
        const targets = JSON.parse(matrix.stdout);
        assert.equal(targets.length, 31);
        assert.equal(new Set(targets.map(t => t.GOOS + '/' + t.GOARCH)).size, targets.length);
        const packages = resolve(directory, 'packages');
        mkdirSync(packages);
        const raw = Buffer.from('executable fixture'), compressed = brotliCompressSync(raw);
        for (const target of targets) {
            const {GOOS: os, GOARCH: arch} = target;
            const file = `renop-fixture-${os}-${arch}.br`;
            writeFileSync(resolve(packages, file), compressed);
            writeFileSync(resolve(packages, `${os}-${arch}.json`), JSON.stringify({os, arch, file,
                sha256: createHash('sha256').update(compressed).digest('hex'), size: compressed.length,
                uncompressed_size: raw.length, format: 'brotli', executable: os === 'windows' ? 'renop.exe' : 'renop'}));
        }
        const run = name => spawnSync('pwsh', ['-NoProfile', '-File', '.github/scripts/assemble-matrix.ps1',
            '-PackageDir', packages, '-DistDir', resolve(directory, name), '-Version', 'fixture',
            '-Development', 'true', '-Commit', 'a'.repeat(40)], {cwd: repositoryRoot, encoding: 'utf8', timeout: 30_000});
        const success = run('complete');
        assert.equal(success.status, 0, success.stdout + success.stderr);
        const manifest = JSON.parse(readFileSync(resolve(directory, 'complete/manifest.json'), 'utf8').replace(/^\uFEFF/, ''));
        assert.equal(manifest.targets.length, targets.length);
        assert.equal(manifest.commit, 'a'.repeat(40));
        assert.equal(manifest.development, true);
        assert.deepEqual(manifest.targets.map(t => t.os + '/' + t.arch), targets.map(t => t.GOOS + '/' + t.GOARCH));
        const payload = spawnSync('pwsh', ['-NoProfile', '-File', '.github/scripts/test-release-payload.ps1',
            '-DistDir', resolve(directory, 'complete')], {cwd: repositoryRoot, encoding: 'utf8'});
        assert.equal(payload.status, 0, payload.stdout + payload.stderr);
        const target = targets[0];
        writeFileSync(resolve(packages, `renop-fixture-${target.GOOS}-${target.GOARCH}.br`), Buffer.alloc(compressed.length));
        const corrupted = run('corrupt');
        assert.notEqual(corrupted.status, 0);
        assert.match(corrupted.stderr, /hash or size mismatch/);
        rmSync(resolve(packages, `${target.GOOS}-${target.GOARCH}.json`));
        const missing = run('missing');
        assert.notEqual(missing.status, 0);
        assert.match(missing.stderr, /Expected exactly 31/);
    } finally {
        assert.equal(dirname(directory), resolve(tmpdir()));
        assert.match(directory.split(/[\\/]/).at(-1), /^renop-matrix-test-/);
        rmSync(directory, {recursive: true, force: true});
    }
});

test('publishing cleans newest obsolete trees after publication and skips missing directories', async () => {
    const repositoryRoot = resolve(fileURLToPath(new URL('../..', import.meta.url)));
    const directory = mkdtempSync(resolve(tmpdir(), 'renop-publish-test-'));
    const requests = [], commits = [];
    let mode = 'success', published, deleteCount = 0;
    const git = (...args) => {
        const result = spawnSync('git', args, {cwd: directory, encoding: 'utf8'});
        assert.equal(result.status, 0, result.stderr);
        return result.stdout.trim();
    };
    const server = createServer(async (request, response) => {
        let body = '';
        for await (const chunk of request) body += chunk;
        requests.push([request.method, request.url]);
        if (request.method === 'GET') {
            response.writeHead(mode === 'read-failure' ? 503 : 200, {'Content-Type': 'application/json'});
            response.end(JSON.stringify({releases: [
                {version: 'unknown', commit: 'a'.repeat(40)},
                ...commits.map(commit => ({commit, version: commit.slice(0, 7)})),
            ]}));
        } else if (request.method === 'PUT') {
            if (request.url.endsWith('/info.json')) published = JSON.parse(body);
            response.writeHead(mode === 'upload-failure' ? 500 : 201).end();
        } else {
            assert.equal(request.method, 'DELETE');
            response.writeHead(mode === 'delete-failure' ? 403 : (++deleteCount <= 2 ? 404 : 204)).end();
        }
    });
    try {
        git('init', '--quiet');
        git('config', 'core.abbrev', '7');
        for (let i = 0; i < 20; i++) {
            git('-c', 'user.name=Test', '-c', 'user.email=test@example.invalid', '-c', 'commit.gpgsign=false',
                'commit', '--quiet', '--allow-empty', '-m', `fix: published ${i}`);
            commits.push(git('rev-parse', 'HEAD'));
        }
        mkdirSync(resolve(directory, 'dist'));
        writeFileSync(resolve(directory, 'dist/renop-test-linux-amd64.br'), brotliCompressSync(Buffer.from('fixture')));
        await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
        const run = () => new Promise((resolve, reject) => {
            const child = spawn('pwsh', ['-NoProfile', '-File', `${repositoryRoot}/.github/scripts/publish-update.ps1`,
                '-Channel', 'nightly', '-DistDir', `${directory}/dist`, '-Version', commits.at(-1).slice(0, 7),
                '-Commit', commits.at(-1), '-Changelog', 'fixture', '-BaseUrl', `http://127.0.0.1:${server.address().port}`],
            {cwd: directory, env: {...process.env, RENOP_PUBLISH_TOKEN: 'isolated-test-token'}, timeout: 30_000});
            let output = '';
            child.stdout.on('data', chunk => { output += chunk; });
            child.stderr.on('data', chunk => { output += chunk; });
            child.on('error', reject);
            child.on('close', code => resolve({code, output}));
        });
        const success = await run();
        assert.equal(success.code, 0, success.output);
        assert.equal(published.releases[0].commit, commits.at(-1));
        const obsolete = commits.toReversed().slice(9, 16).map(commit => `/update/renop/nightly/${commit.slice(0, 7)}`);
        assert.deepEqual(requests.filter(([method]) => method === 'DELETE').map(([, path]) => path), obsolete);
        const firstDelete = requests.findIndex(([method]) => method === 'DELETE');
        assert.ok(firstDelete > requests.findIndex(([method, path]) => method === 'PUT' && path.endsWith('/info.json')));
        for (const failure of ['read-failure', 'upload-failure', 'delete-failure']) {
            mode = failure;
            requests.length = 0;
            const result = await run();
            assert.notEqual(result.code, 0, `${failure} must fail the workflow`);
            if (failure !== 'delete-failure') assert.ok(!requests.some(([method]) => method === 'DELETE'));
            if (failure === 'read-failure') assert.ok(!requests.some(([method]) => method === 'PUT'));
        }
    } finally {
        await new Promise(resolve => server.close(resolve));
        assert.equal(dirname(directory), resolve(tmpdir()));
        assert.match(directory.split(/[\\/]/).at(-1), /^renop-publish-test-/);
        rmSync(directory, {recursive: true, force: true});
    }
});

test('Go protobuf generation includes API and durable session schemas', () => {
    const repositoryRoot = resolve(fileURLToPath(new URL('../..', import.meta.url)));
    const build = readFileSync(resolve(repositoryRoot, 'build.ps1'), 'utf8');
    for (const path of [
        'proto/api/v1/api.proto',
        'proto/storage/v1/session.proto',
        'pkg/pb/api.pb.go',
        'pkg/pb/session.pb.go',
    ]) assert.ok(build.includes(path), `build.ps1 is missing ${path}`);
});

test('nightly metadata rebuilds targets and missing releases in Git order', () => {
    const repositoryRoot = resolve(fileURLToPath(new URL('../..', import.meta.url)));
    const result = spawnSync('pwsh', ['-NoProfile', '-File', '.github/scripts/test-nightly-info.ps1'], {
        cwd: repositoryRoot, encoding: 'utf8', timeout: 120_000,
    });
    assert.equal(result.status, 0, result.error?.message || result.stdout + result.stderr);
});
