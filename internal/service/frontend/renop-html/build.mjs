/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

/**
 * Frontend production build:
 * 0. Validate and collect modular locale fragments
 * 1. Generate protobuf static modules from proto/api/v1/api.proto
 * 2. Bundle JS with Rolldown into dist/js/ (+ code-split chunks)
 * 3. Bundle CSS with lightningcss into dist/css/style.css
 *
 * Note: Rolldown 1.x removed experimental native CSS bundling
 * (https://github.com/rolldown/rolldown/issues/4271), so CSS is handled by lightningcss.
 */
import {existsSync, mkdirSync, readdirSync, rmSync, statSync, writeFileSync,} from 'node:fs';
import {dirname, join, resolve} from 'node:path';
import {fileURLToPath, pathToFileURL} from 'node:url';
import {spawnSync} from 'node:child_process';
import {rolldown} from 'rolldown';
import {bundleAsync} from 'lightningcss';
import {generateI18nCatalog} from './scripts/i18n-catalog.mjs';

const root = dirname(fileURLToPath(import.meta.url));
const outDir = join(root, 'dist');
const repoRoot = resolve(root, '../../../..');
const protoFile = join(repoRoot, 'proto', 'api', 'v1', 'api.proto');
const protoOutDir = join(root, 'js', 'proto');
const protoOutFile = join(protoOutDir, 'api.js');
const i18nDir = join(root, 'js', 'i18n');
const i18nCatalogName = 'catalog.generated.js';
const i18nCatalogFile = join(i18nDir, i18nCatalogName);
const i18nReferenceLocale = 'en-US';
const maxInitialJavaScriptBytes = 1280 * 1024;
const maxAsyncJavaScriptBytes = 256 * 1024;

/**
 * Recursively collects file paths below a directory.
 *
 * @param {string} dir Directory to walk.
 * @param {string[]} acc Accumulator used by recursive calls.
 * @returns {string[]} Collected file paths.
 */
function walk(dir, acc = []) {
    if (!existsSync(dir)) return acc;
    for (const name of readdirSync(dir)) {
        const p = join(dir, name);
        if (statSync(p).isDirectory()) walk(p, acc);
        else acc.push(p);
    }
    return acc;
}

/**
 * Regenerates the browser-side protobuf static module.
 *
 * @returns {void}
 */
function generateProtobuf() {
    if (!existsSync(protoFile)) {
        throw new Error(`proto schema not found: ${protoFile}`);
    }
    mkdirSync(protoOutDir, {recursive: true});

    const pbjsBin = join(root, 'node_modules', 'protobufjs-cli', 'bin', 'pbjs');
    if (!existsSync(pbjsBin)) {
        throw new Error('protobufjs-cli not installed; run npm install in frontend/renop-html');
    }

    console.log('Generating protobuf JS from', protoFile.replaceAll('\\', '/'));
    const result = spawnSync(
        process.execPath,
        [
            pbjsBin,
            '-t', 'static-module',
            '-w', 'es6',
            '--keep-case',
            '--no-create',
            '--no-verify',
            '--no-delimited',
            '--no-typeurl',
            '--no-comments',
            '--no-service',
            '-o', protoOutFile,
            protoFile,
        ],
        {cwd: root, stdio: 'inherit'},
    );
    if (result.status !== 0) {
        throw new Error(`pbjs failed with exit code ${result.status ?? 'unknown'}`);
    }
    if (!existsSync(protoOutFile)) {
        throw new Error('pbjs did not produce js/proto/api.js');
    }
}

/** Generate HTTP content-coding sidecars with the host Go toolchain. */
function precompressAssets() {
    const host = spawnSync('go', ['env', 'GOHOSTOS', 'GOHOSTARCH'], {
        cwd: repoRoot,
        encoding: 'utf8',
    });
    const [goos, goarch] = String(host.stdout || '').trim().split(/\r?\n/);
    if (host.status !== 0 || !goos || !goarch) {
        throw new Error(`go env failed with exit code ${host.status ?? 'unknown'}`);
    }
    const env = {...process.env, GOOS: goos, GOARCH: goarch};
    delete env.GOAMD64;
    const result = spawnSync('go', ['run', './cmd/renop-precompress', '-root', outDir], {
        cwd: repoRoot,
        env,
        stdio: 'inherit',
    });
    if (result.status !== 0) {
        throw new Error(`frontend precompression failed with exit code ${result.status ?? 'unknown'}`);
    }
}

/** @param {string} path Built asset path. */
function isPrecompressedAsset(path) {
    return ['.br', '.gz', '.zst', '.deflate'].some(suffix => path.endsWith(suffix));
}

const protoOnly = process.argv.includes('--proto-only');
const i18nOnly = process.argv.includes('--i18n-only');

await generateI18nCatalog({
    i18nDir,
    catalogFile: i18nCatalogFile,
    referenceLocale: i18nReferenceLocale,
    catalogName: i18nCatalogName,
    sourceRoots: [join(root, 'js'), join(root, 'index.html')],
});
if (i18nOnly) {
    console.log('Frontend i18n sources are complete.');
    process.exit(0);
}
generateProtobuf();
if (protoOnly) {
    console.log('Frontend generated sources are up to date.');
    process.exit(0);
}

if (existsSync(outDir)) {
    rmSync(outDir, {recursive: true, force: true});
}
mkdirSync(outDir, {recursive: true});

const configUrl = pathToFileURL(join(root, 'rolldown.config.mjs')).href;
const {default: rolldownConfig} = await import(configUrl);
const build = await rolldown(rolldownConfig);
try {
    await build.write(rolldownConfig.output);
} finally {
    await build.close();
}

const mainJs = join(outDir, 'js', 'main.js');
if (!existsSync(mainJs)) {
    console.error('missing dist/js/main.js after Rolldown build');
    process.exit(1);
}
const mainJsBytes = statSync(mainJs).size;
if (mainJsBytes > maxInitialJavaScriptBytes) {
    throw new Error(`dist/js/main.js exceeds ${maxInitialJavaScriptBytes} bytes: ${mainJsBytes}`);
}
for (const file of walk(join(outDir, 'js', 'chunks'))) {
    if (!file.endsWith('.js')) continue;
    const bytes = statSync(file).size;
    if (bytes > maxAsyncJavaScriptBytes) {
        throw new Error(`${file.slice(outDir.length + 1)} exceeds ${maxAsyncJavaScriptBytes} bytes: ${bytes}`);
    }
}

const styleEntry = join(root, 'css', 'style.css');
const cssDir = join(outDir, 'css');
mkdirSync(cssDir, {recursive: true});

const {code, warnings} = await bundleAsync({
    filename: styleEntry,
    minify: true,
    resolver: {
        resolve(specifier, originatingFile) {
            if (specifier.startsWith('@renop/ui/')) {
                return join(repoRoot, 'packages/renop-ui', specifier.slice('@renop/ui/'.length));
            }
            return resolve(dirname(originatingFile), specifier);
        },
    },
});

if (warnings && warnings.length) {
    for (const w of warnings) {
        console.warn('CSS warning:', w.message);
    }
}

writeFileSync(join(cssDir, 'style.css'), code);

precompressAssets();
const all = walk(outDir);
const sourceAssets = all.filter(file => !isPrecompressedAsset(file));
console.log(`Frontend build OK (${sourceAssets.length} files, ${all.length - sourceAssets.length} precompressed variants):`);
for (const f of sourceAssets) {
    console.log('  ' + f.slice(outDir.length + 1).replaceAll('\\', '/'));
}
