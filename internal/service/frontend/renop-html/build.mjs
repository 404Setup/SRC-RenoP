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
import {
  existsSync,
  mkdirSync,
  readFileSync,
  readdirSync,
  rmSync,
  statSync,
  writeFileSync,
} from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { spawnSync } from 'node:child_process';
import { rolldown } from 'rolldown';
import { bundleAsync } from 'lightningcss';

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

/**
 * Lists the translation fragment modules for one locale.
 *
 * @param {string} locale Locale directory name.
 * @returns {string[]} Sorted fragment file names.
 */
function listLocaleFragments(locale) {
  const localeDir = join(i18nDir, locale);
  const fragments = [];
  for (const entry of readdirSync(localeDir, { withFileTypes: true })) {
    if (!entry.isFile() || !/^[a-z][a-z0-9-]*\.js$/.test(entry.name)) {
      throw new Error(`invalid i18n fragment in ${locale}: ${entry.name}`);
    }
    fragments.push(entry.name);
  }
  fragments.sort();
  return fragments;
}

/**
 * Loads and validates one locale fragment's default export.
 *
 * @param {string} locale Locale directory name.
 * @param {string} fragment Fragment file name.
 * @returns {Promise<string[]>} Sorted translation keys.
 */
async function loadLocaleFragment(locale, fragment) {
  const fragmentPath = join(i18nDir, locale, fragment);
  const module = await import(pathToFileURL(fragmentPath).href);
  const translations = module.default;
  if (!translations || typeof translations !== 'object' || Array.isArray(translations)) {
    throw new Error(`i18n fragment must default-export an object: ${locale}/${fragment}`);
  }

  const keys = Object.keys(translations);
  for (const key of keys) {
    if (!key || typeof translations[key] !== 'string') {
      throw new Error(`invalid i18n entry in ${locale}/${fragment}: ${key || '<empty key>'}`);
    }
  }
  keys.sort();
  return keys;
}

/**
 * Creates a stable JavaScript identifier for a locale fragment import.
 *
 * @param {string} locale Locale directory name.
 * @param {string} fragment Fragment file name.
 * @returns {string} Import identifier.
 */
function localeImportIdentifier(locale, fragment) {
  return `${locale}_${fragment.slice(0, -3)}`.replaceAll('-', '_');
}

/**
 * Validates locale parity and generates the static locale catalog imported by the browser.
 *
 * @returns {Promise<void>}
 */
async function generateI18nCatalog() {
  if (!existsSync(i18nDir)) {
    throw new Error(`i18n directory not found: ${i18nDir}`);
  }

  const locales = [];
  for (const entry of readdirSync(i18nDir, { withFileTypes: true })) {
    if (entry.isFile() && entry.name === i18nCatalogName) {
      continue;
    }
    if (!entry.isDirectory() || !/^[A-Za-z]{2,3}(?:-[A-Za-z0-9]{2,8})+$/.test(entry.name)) {
      throw new Error(`invalid entry in i18n directory: ${entry.name}`);
    }
    locales.push(entry.name);
  }
  locales.sort();
  if (!locales.includes(i18nReferenceLocale)) {
    throw new Error(`reference locale is missing: ${i18nReferenceLocale}`);
  }

  const referenceFragments = listLocaleFragments(i18nReferenceLocale);
  if (!referenceFragments.includes('core.js')) {
    throw new Error(`${i18nReferenceLocale} must provide core.js`);
  }

  const referenceKeys = new Map();
  const referenceCombinedKeys = new Set();
  for (const fragment of referenceFragments) {
    const keys = await loadLocaleFragment(i18nReferenceLocale, fragment);
    for (const key of keys) {
      if (referenceCombinedKeys.has(key)) {
        throw new Error(`duplicate i18n key in ${i18nReferenceLocale}: ${key}`);
      }
      referenceCombinedKeys.add(key);
    }
    referenceKeys.set(fragment, keys);
  }

  for (const locale of locales) {
    const fragments = listLocaleFragments(locale);
    if (fragments.join('\0') !== referenceFragments.join('\0')) {
      throw new Error(
        `i18n fragments for ${locale} must match ${i18nReferenceLocale}: ${referenceFragments.join(', ')}`,
      );
    }

    const combinedKeys = new Set();
    for (const fragment of fragments) {
      const keys = await loadLocaleFragment(locale, fragment);
      const expectedKeys = referenceKeys.get(fragment);
      if (keys.join('\0') !== expectedKeys.join('\0')) {
        throw new Error(`i18n keys for ${locale}/${fragment} must match ${i18nReferenceLocale}/${fragment}`);
      }
      for (const key of keys) {
        if (combinedKeys.has(key)) {
          throw new Error(`duplicate i18n key in ${locale}: ${key}`);
        }
        combinedKeys.add(key);
      }
    }
  }

  const imports = [];
  const catalogEntries = [];
  for (const locale of locales) {
    const identifiers = [];
    for (const fragment of referenceFragments) {
      const identifier = localeImportIdentifier(locale, fragment);
      identifiers.push(identifier);
      imports.push(`import ${identifier} from './${locale}/${fragment}';`);
    }
    catalogEntries.push(`    '${locale}': Object.freeze(Object.assign({}, ${identifiers.join(', ')})),`);
  }

  const generated = [
    '/* This file is generated by build.mjs. Do not edit it directly. */',
    '',
    ...imports,
    '',
    'const localeCatalog = Object.freeze({',
    ...catalogEntries,
    '});',
    '',
    'export default localeCatalog;',
    '',
  ].join('\n');
  if (!existsSync(i18nCatalogFile) || readFileSync(i18nCatalogFile, 'utf8') !== generated) {
    writeFileSync(i18nCatalogFile, generated, 'utf8');
  }
  console.log('Generated i18n catalog:', i18nCatalogFile.replaceAll('\\', '/'));
}

const protoOnly = process.argv.includes('--proto-only');

await generateI18nCatalog();
generateProtobuf();
if (protoOnly) {
  console.log('Frontend generated sources are up to date.');
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

const { code, warnings } = await bundleAsync({
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

const all = walk(outDir);
console.log(`Frontend build OK (${all.length} files):`);
for (const f of all) {
  console.log('  ' + f.slice(outDir.length + 1).replaceAll('\\', '/'));
}
