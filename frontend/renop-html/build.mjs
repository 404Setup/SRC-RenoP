/**
 * Frontend production build:
 * 0. Generate protobuf static modules from proto/api/v1/api.proto
 * 1. Bundle JS with Rolldown into dist/js/ (+ code-split chunks)
 * 2. Bundle CSS with lightningcss into dist/css/style.css
 *
 * Note: Rolldown 1.x removed experimental native CSS bundling
 * (https://github.com/rolldown/rolldown/issues/4271), so CSS is handled by lightningcss.
 */
import {
  existsSync,
  mkdirSync,
  readdirSync,
  rmSync,
  statSync,
  writeFileSync,
} from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { spawnSync } from 'node:child_process';
import { rolldown } from 'rolldown';
import { bundle } from 'lightningcss';

const root = dirname(fileURLToPath(import.meta.url));
const outDir = join(root, 'dist');
const repoRoot = resolve(root, '../..');
const protoFile = join(repoRoot, 'proto', 'api', 'v1', 'api.proto');
const protoOutDir = join(root, 'js', 'proto');
const protoOutFile = join(protoOutDir, 'api.js');

function walk(dir, acc = []) {
  if (!existsSync(dir)) return acc;
  for (const name of readdirSync(dir)) {
    const p = join(dir, name);
    if (statSync(p).isDirectory()) walk(p, acc);
    else acc.push(p);
  }
  return acc;
}

function generateProtobuf() {
  if (!existsSync(protoFile)) {
    throw new Error(`proto schema not found: ${protoFile}`);
  }
  mkdirSync(protoOutDir, { recursive: true });

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
      '-o', protoOutFile,
      protoFile,
    ],
    { cwd: root, stdio: 'inherit' },
  );
  if (result.status !== 0) {
    throw new Error(`pbjs failed with exit code ${result.status ?? 'unknown'}`);
  }
  if (!existsSync(protoOutFile)) {
    throw new Error('pbjs did not produce js/proto/api.js');
  }
}

const protoOnly = process.argv.includes('--proto-only');

generateProtobuf();
if (protoOnly) {
  console.log('Protobuf JS generated:', protoOutFile.replaceAll('\\', '/'));
  process.exit(0);
}

if (existsSync(outDir)) {
  rmSync(outDir, { recursive: true, force: true });
}
mkdirSync(outDir, { recursive: true });

const configUrl = pathToFileURL(join(root, 'rolldown.config.mjs')).href;
const { default: rolldownConfig } = await import(configUrl);
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

const styleEntry = join(root, 'css', 'style.css');
const cssDir = join(outDir, 'css');
mkdirSync(cssDir, { recursive: true });

const { code, warnings } = bundle({
  filename: styleEntry,
  minify: true,
});

if (warnings && warnings.length) {
  for (const w of warnings) {
    console.warn('CSS warning:', w.message);
  }
}

writeFileSync(join(cssDir, 'style.css'), code);

const all = walk(outDir);
console.log(`Frontend build OK (${all.length} files):`);
for (const f of all) {
  console.log('  ' + f.slice(outDir.length + 1).replaceAll('\\', '/'));
}
