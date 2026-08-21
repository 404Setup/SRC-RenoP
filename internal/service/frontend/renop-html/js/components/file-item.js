/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {t} from '../i18n.js';
import {el} from '@renop/ui/dom';
import {canUpdateRepo, repoNameFromPath} from '../auth.js';
import {encodePathSegment} from '../browser/utils.js';
import {createIcon} from './icon.js';

/**
 * Unified file-type catalog.
 * Single source of truth for category id → icon + i18n key + default label.
 * Eliminates parallel if-chains / switch fallthroughs that drifted out of sync.
 */
export const FILE_TYPE_CATALOG = {
    diff: {icon: 'fileDiff', key: 'browser.fileTypeDiff', defaultLabel: 'Git Patch / Diff'},
    docker: {icon: 'fileDocker', key: 'browser.fileTypeDocker', defaultLabel: 'Docker Config'},
    pom: {icon: 'filePom', key: 'browser.fileTypePom', defaultLabel: 'Maven POM'},
    javadoc: {icon: 'fileJavadoc', key: 'browser.fileTypeJavadoc', defaultLabel: 'Javadoc Package'},
    sources: {icon: 'fileSources', key: 'browser.fileTypeSources', defaultLabel: 'Sources Package'},
    npm: {icon: 'fileNpm', key: 'browser.fileTypeNpm', defaultLabel: 'NPM Package'},
    cargo: {icon: 'fileCargo', key: 'browser.fileTypeCargo', defaultLabel: 'Cargo Manifest'},
    gomod: {icon: 'fileGomod', key: 'browser.fileTypeGomod', defaultLabel: 'Go Module'},
    gradle: {icon: 'fileGradle', key: 'browser.fileTypeGradle', defaultLabel: 'Gradle Build'},
    env: {icon: 'fileEnv', key: 'browser.fileTypeEnv', defaultLabel: 'Environment Config'},
    k8s: {icon: 'fileK8s', key: 'browser.fileTypeK8s', defaultLabel: 'Kubernetes Manifest'},
    makefile: {icon: 'fileMakefile', key: 'browser.fileTypeMakefile', defaultLabel: 'Makefile / CMake'},
    terraform: {icon: 'fileTerraform', key: 'browser.fileTypeTerraform', defaultLabel: 'Terraform HCL'},
    notebook: {icon: 'fileNotebook', key: 'browser.fileTypeNotebook', defaultLabel: 'Jupyter Notebook'},
    wasm: {icon: 'fileWasm', key: 'browser.fileTypeWasm', defaultLabel: 'WebAssembly Module'},
    composer: {icon: 'fileComposer', key: 'browser.fileTypeComposer', defaultLabel: 'Composer Manifest'},
    gemfile: {icon: 'fileGemfile', key: 'browser.fileTypeGemfile', defaultLabel: 'Ruby Gemfile'},
    pyproject: {icon: 'filePyproject', key: 'browser.fileTypePyproject', defaultLabel: 'Python Project'},
    gitignore: {icon: 'fileIgnore', key: 'browser.fileTypeIgnore', defaultLabel: 'Ignore Rules'},
    lockfile: {icon: 'fileLock', key: 'browser.fileTypeLock', defaultLabel: 'Lock File'},
    jar: {icon: 'fileJar', key: 'browser.fileTypeJar', defaultLabel: 'Java Archive'},
    war: {icon: 'fileWar', key: 'browser.fileTypeWar', defaultLabel: 'Web Application Archive'},
    ear: {icon: 'fileEar', key: 'browser.fileTypeEar', defaultLabel: 'Enterprise Archive'},
    aar: {icon: 'fileAar', key: 'browser.fileTypeAar', defaultLabel: 'Android Library Archive'},
    apk: {icon: 'fileApk', key: 'browser.fileTypeApk', defaultLabel: 'Android Package'},
    app: {icon: 'fileApp', key: 'browser.fileTypeApp', defaultLabel: 'Mobile App Package'},
    archive: {icon: 'fileZip', key: 'browser.fileTypeArchive', defaultLabel: 'Archive File'},
    disk: {icon: 'fileDisk', key: 'browser.fileTypeDisk', defaultLabel: 'Disk Image'},
    exe: {icon: 'fileExe', key: 'browser.fileTypeExe', defaultLabel: 'Installer Executable'},
    msi: {icon: 'fileMsi', key: 'browser.fileTypeMsi', defaultLabel: 'Installer Package'},
    pkg: {icon: 'filePkg', key: 'browser.fileTypePkg', defaultLabel: 'Package File'},
    binary: {icon: 'storage', key: 'browser.fileTypeBinary', defaultLabel: 'Binary Library'},
    html: {icon: 'fileHtml', key: 'browser.fileTypeHtml', defaultLabel: 'HTML Document'},
    style: {icon: 'fileCss', key: 'browser.fileTypeCss', defaultLabel: 'Style Sheet'},
    react: {icon: 'fileReact', key: 'browser.fileTypeReact', defaultLabel: 'React Component'},
    vue: {icon: 'fileVue', key: 'browser.fileTypeVue', defaultLabel: 'Vue Component'},
    svelte: {icon: 'fileSvelte', key: 'browser.fileTypeSvelte', defaultLabel: 'Svelte Component'},
    astro: {icon: 'fileAstro', key: 'browser.fileTypeAstro', defaultLabel: 'Astro Component'},
    xml: {icon: 'fileXml', key: 'browser.fileTypeXml', defaultLabel: 'XML Document'},
    script: {icon: 'fileShell', key: 'browser.fileTypeShell', defaultLabel: 'Shell Script'},
    batch: {icon: 'fileBatch', key: 'browser.fileTypeBatch', defaultLabel: 'Batch Script'},
    powershell: {icon: 'filePowerShell', key: 'browser.fileTypePowerShell', defaultLabel: 'PowerShell Script'},
    registry: {icon: 'fileRegistry', key: 'browser.fileTypeReg', defaultLabel: 'Windows Registry File'},
    rust: {icon: 'fileRust', key: 'browser.fileTypeRust', defaultLabel: 'Rust Source'},
    python: {icon: 'filePython', key: 'browser.fileTypePython', defaultLabel: 'Python Script'},
    golang: {icon: 'fileGo', key: 'browser.fileTypeGo', defaultLabel: 'Go Source'},
    java: {icon: 'fileJava', key: 'browser.fileTypeJava', defaultLabel: 'Java Source'},
    kotlin: {icon: 'fileKotlin', key: 'browser.fileTypeKotlin', defaultLabel: 'Kotlin Source'},
    clang: {icon: 'fileC', key: 'browser.fileTypeC', defaultLabel: 'C Source'},
    cpp: {icon: 'fileCpp', key: 'browser.fileTypeCpp', defaultLabel: 'C++ Source'},
    cheader: {icon: 'fileC', key: 'browser.fileTypeHeader', defaultLabel: 'Header File'},
    csharp: {icon: 'fileCsharp', key: 'browser.fileTypeCs', defaultLabel: 'C# Source'},
    php: {icon: 'filePhp', key: 'browser.fileTypePhp', defaultLabel: 'PHP Script'},
    ruby: {icon: 'fileRuby', key: 'browser.fileTypeRuby', defaultLabel: 'Ruby Script'},
    swift: {icon: 'fileSwift', key: 'browser.fileTypeSwift', defaultLabel: 'Swift Source'},
    sql: {icon: 'fileSql', key: 'browser.fileTypeSql', defaultLabel: 'SQL Script'},
    javascript: {icon: 'fileJs', key: 'browser.fileTypeJs', defaultLabel: 'JavaScript Code'},
    typescript: {icon: 'fileTs', key: 'browser.fileTypeTs', defaultLabel: 'TypeScript Code'},
    scala: {icon: 'fileScala', key: 'browser.fileTypeScala', defaultLabel: 'Scala Source'},
    dart: {icon: 'fileDart', key: 'browser.fileTypeDart', defaultLabel: 'Dart Source'},
    lua: {icon: 'fileLua', key: 'browser.fileTypeLua', defaultLabel: 'Lua Script'},
    zig: {icon: 'fileZig', key: 'browser.fileTypeZig', defaultLabel: 'Zig Source'},
    rlang: {icon: 'fileR', key: 'browser.fileTypeR', defaultLabel: 'R Script'},
    julia: {icon: 'fileJulia', key: 'browser.fileTypeJulia', defaultLabel: 'Julia Source'},
    haskell: {icon: 'fileHaskell', key: 'browser.fileTypeHaskell', defaultLabel: 'Haskell Source'},
    clojure: {icon: 'fileClojure', key: 'browser.fileTypeClojure', defaultLabel: 'Clojure Source'},
    elixir: {icon: 'fileElixir', key: 'browser.fileTypeElixir', defaultLabel: 'Elixir Source'},
    erlang: {icon: 'fileErlang', key: 'browser.fileTypeErlang', defaultLabel: 'Erlang Source'},
    perl: {icon: 'filePerl', key: 'browser.fileTypePerl', defaultLabel: 'Perl Script'},
    asm: {icon: 'fileAsm', key: 'browser.fileTypeAsm', defaultLabel: 'Assembly Source'},
    solidity: {icon: 'fileSolidity', key: 'browser.fileTypeSolidity', defaultLabel: 'Solidity Contract'},
    nim: {icon: 'fileNim', key: 'browser.fileTypeNim', defaultLabel: 'Nim Source'},
    objc: {icon: 'fileObjc', key: 'browser.fileTypeObjc', defaultLabel: 'Objective-C Source'},
    fsharp: {icon: 'fileFsharp', key: 'browser.fileTypeFsharp', defaultLabel: 'F# Source'},
    ocaml: {icon: 'fileOcaml', key: 'browser.fileTypeOcaml', defaultLabel: 'OCaml Source'},
    groovy: {icon: 'fileGroovy', key: 'browser.fileTypeGroovy', defaultLabel: 'Groovy Source'},
    crystal: {icon: 'fileCrystal', key: 'browser.fileTypeCrystal', defaultLabel: 'Crystal Source'},
    vlang: {icon: 'fileVlang', key: 'browser.fileTypeVlang', defaultLabel: 'V Source'},
    yaml: {icon: 'fileYaml', key: 'browser.fileTypeYaml', defaultLabel: 'YAML Config'},
    json: {icon: 'fileJson', key: 'browser.fileTypeJson', defaultLabel: 'JSON Data'},
    geojson: {icon: 'fileGeojson', key: 'browser.fileTypeGeojson', defaultLabel: 'GeoJSON Map'},
    proto: {icon: 'fileProto', key: 'browser.fileTypeProto', defaultLabel: 'Protobuf Schema'},
    graphql: {icon: 'fileGraphql', key: 'browser.fileTypeGraphql', defaultLabel: 'GraphQL Schema'},
    config: {icon: 'fileSettings', key: 'browser.fileTypeConfig', defaultLabel: 'Configuration'},
    toml: {icon: 'fileToml', key: 'browser.fileTypeToml', defaultLabel: 'TOML Config'},
    image: {icon: 'fileImage', key: 'browser.fileTypeImage', defaultLabel: 'Image File'},
    svg: {icon: 'fileSvg', key: 'browser.fileTypeSvg', defaultLabel: 'Vector Graphic (SVG)'},
    design: {icon: 'fileDesign', key: 'browser.fileTypeDesign', defaultLabel: 'Design File'},
    audio: {icon: 'fileAudio', key: 'browser.fileTypeAudio', defaultLabel: 'Audio File'},
    video: {icon: 'fileVideo', key: 'browser.fileTypeVideo', defaultLabel: 'Video File'},
    model: {icon: 'fileModel', key: 'browser.fileTypeModel', defaultLabel: '3D Model'},
    pdf: {icon: 'filePdf', key: 'browser.fileTypePdf', defaultLabel: 'PDF Document'},
    markdown: {icon: 'fileMarkdown', key: 'browser.fileTypeMarkdown', defaultLabel: 'Markdown Document'},
    word: {icon: 'fileWord', key: 'browser.fileTypeWord', defaultLabel: 'Word Document'},
    txt: {icon: 'fileTxt', key: 'browser.fileTypeTxt', defaultLabel: 'Text Document'},
    log: {icon: 'fileLog', key: 'browser.fileTypeLog', defaultLabel: 'Log File'},
    presentation: {icon: 'filePresentation', key: 'browser.fileTypePresentation', defaultLabel: 'Presentation'},
    ebook: {icon: 'fileBook', key: 'browser.fileTypeEbook', defaultLabel: 'E-Book Publication'},
    spreadsheet: {icon: 'fileSpreadsheet', key: 'browser.fileTypeSpreadsheet', defaultLabel: 'Spreadsheet'},
    csv: {icon: 'fileCsv', key: 'browser.fileTypeCsv', defaultLabel: 'CSV Dataset'},
    database: {icon: 'database', key: 'browser.fileTypeDatabase', defaultLabel: 'Database File'},
    checksum: {icon: 'fileHash', key: 'browser.fileTypeChecksum', defaultLabel: 'Checksum Hash'},
    security: {icon: 'fileKey', key: 'browser.fileTypeSecurity', defaultLabel: 'Certificate & Key'},
    font: {icon: 'fileFont', key: 'browser.fileTypeFont', defaultLabel: 'Font File'},
    code: {icon: 'fileCode', key: 'browser.fileTypeCode', defaultLabel: 'Source Code'},
    coffee: {icon: 'fileCoffee', key: 'browser.fileTypeCoffee', defaultLabel: 'CoffeeScript'},
    elm: {icon: 'fileElm', key: 'browser.fileTypeElm', defaultLabel: 'Elm Source'},
    fortran: {icon: 'fileFortran', key: 'browser.fileTypeFortran', defaultLabel: 'Fortran Source'},
    matlab: {icon: 'fileMatlab', key: 'browser.fileTypeMatlab', defaultLabel: 'MATLAB / Octave'},
    lisp: {icon: 'fileLisp', key: 'browser.fileTypeLisp', defaultLabel: 'Lisp / Scheme'},
    vb: {icon: 'fileVb', key: 'browser.fileTypeVb', defaultLabel: 'Visual Basic'},
    haxe: {icon: 'fileHaxe', key: 'browser.fileTypeHaxe', defaultLabel: 'Haxe Source'},
    dlang: {icon: 'fileDlang', key: 'browser.fileTypeDlang', defaultLabel: 'D Source'},
    pascal: {icon: 'filePascal', key: 'browser.fileTypePascal', defaultLabel: 'Pascal / Delphi'},
    gleam: {icon: 'fileGleam', key: 'browser.fileTypeGleam', defaultLabel: 'Gleam Source'},
    odin: {icon: 'fileOdin', key: 'browser.fileTypeOdin', defaultLabel: 'Odin Source'},
    rescript: {icon: 'fileRescript', key: 'browser.fileTypeRescript', defaultLabel: 'ReScript Source'},
    prolog: {icon: 'fileProlog', key: 'browser.fileTypeProlog', defaultLabel: 'Prolog Source'},
    scss: {icon: 'fileScss', key: 'browser.fileTypeScss', defaultLabel: 'SCSS Stylesheet'},
    less: {icon: 'fileLess', key: 'browser.fileTypeLess', defaultLabel: 'Less Stylesheet'},
    nupkg: {icon: 'fileNupkg', key: 'browser.fileTypeNupkg', defaultLabel: 'NuGet Package'},
    whl: {icon: 'fileWhl', key: 'browser.fileTypeWhl', defaultLabel: 'Python Wheel'},
    gem: {icon: 'fileGem', key: 'browser.fileTypeGem', defaultLabel: 'Ruby Gem'},
    vsix: {icon: 'fileVsix', key: 'browser.fileTypeVsix', defaultLabel: 'VS Code Extension'},
    crx: {icon: 'fileCrx', key: 'browser.fileTypeCrx', defaultLabel: 'Chrome Extension'},
    phar: {icon: 'filePhar', key: 'browser.fileTypePhar', defaultLabel: 'PHP Archive'},
    shader: {icon: 'fileShader', key: 'browser.fileTypeShader', defaultLabel: 'GPU Shader'},
    latex: {icon: 'fileLatex', key: 'browser.fileTypeLatex', defaultLabel: 'LaTeX Document'},
    rst: {icon: 'fileRst', key: 'browser.fileTypeRst', defaultLabel: 'reStructuredText'},
    adoc: {icon: 'fileAdoc', key: 'browser.fileTypeAdoc', defaultLabel: 'AsciiDoc'},
    mlmodel: {icon: 'fileMlmodel', key: 'browser.fileTypeMlmodel', defaultLabel: 'ML / AI Model'},
    parquet: {icon: 'fileParquet', key: 'browser.fileTypeParquet', defaultLabel: 'Parquet Dataset'},
    avro: {icon: 'fileAvro', key: 'browser.fileTypeAvro', defaultLabel: 'Avro Data'},
    pickle: {icon: 'filePickle', key: 'browser.fileTypePickle', defaultLabel: 'Python Pickle'},
    subtitle: {icon: 'fileSubtitle', key: 'browser.fileTypeSubtitle', defaultLabel: 'Subtitle File'},
    plist: {icon: 'filePlist', key: 'browser.fileTypePlist', defaultLabel: 'Property List'},
    calendar: {icon: 'fileCalendar', key: 'browser.fileTypeCalendar', defaultLabel: 'Calendar File'},
    email: {icon: 'fileEmail', key: 'browser.fileTypeEmail', defaultLabel: 'Email Message'},
    tsconfig: {icon: 'fileTsconfig', key: 'browser.fileTypeTsconfig', defaultLabel: 'TypeScript Config'},
    eslint: {icon: 'fileEslint', key: 'browser.fileTypeEslint', defaultLabel: 'ESLint Config'},
    prettier: {icon: 'filePrettier', key: 'browser.fileTypePrettier', defaultLabel: 'Prettier Config'},
    vite: {icon: 'fileVite', key: 'browser.fileTypeVite', defaultLabel: 'Vite Config'},
    webpack: {icon: 'fileWebpack', key: 'browser.fileTypeWebpack', defaultLabel: 'Webpack Config'},
    nix: {icon: 'fileNix', key: 'browser.fileTypeNix', defaultLabel: 'Nix Expression'},
    justfile: {icon: 'fileJust', key: 'browser.fileTypeJust', defaultLabel: 'Justfile'},
    procfile: {icon: 'fileProcfile', key: 'browser.fileTypeProcfile', defaultLabel: 'Procfile'},
    github: {icon: 'fileGithub', key: 'browser.fileTypeGithub', defaultLabel: 'GitHub Actions'},
    generic: {icon: 'file', key: 'browser.file', defaultLabel: 'File'}
};

/** Filename-first special rules (order matters). */
const SPECIAL_RULES = [
    {
        id: 'diff',
        match: (fn, e) => fn.endsWith('.patch') || fn.endsWith('.diff') || e === 'patch' || e === 'diff'
    },
    {
        id: 'docker',
        match: (fn) => fn === 'dockerfile' || fn.startsWith('dockerfile.') || fn === 'containerfile' || fn === '.dockerignore'
    },
    {id: 'pom', match: (fn, e) => fn === 'pom.xml' || e === 'pom'},
    {
        id: 'javadoc',
        match: (fn, e) => e === 'jar' && (fn.endsWith('-javadoc.jar') || fn.includes('-javadoc.'))
    },
    {
        id: 'sources',
        match: (fn, e) => e === 'jar' && (fn.endsWith('-sources.jar') || fn.includes('-sources.'))
    },
    {
        id: 'npm',
        match: (fn) => fn === 'package.json' || fn === 'package-lock.json' || fn === 'pnpm-lock.yaml' || fn === 'yarn.lock' || fn === 'bun.lock' || fn === 'bun.lockb' || fn === 'npm-shrinkwrap.json'
    },
    {id: 'cargo', match: (fn) => fn === 'cargo.toml' || fn === 'cargo.lock'},
    {id: 'gomod', match: (fn) => fn === 'go.mod' || fn === 'go.sum'},
    {
        id: 'gradle',
        match: (fn, e) => fn.includes('gradle') || e === 'gradle' || e === 'kts' && fn.includes('build')
    },
    {id: 'env', match: (fn) => fn === '.env' || fn.startsWith('.env.')},
    {
        id: 'k8s',
        match: (fn) => fn.includes('k8s') || fn.includes('kubernetes') || fn.includes('helm') || fn === 'chart.yaml' || fn === 'chart.yml'
    },
    {
        id: 'makefile',
        match: (fn, e) => fn === 'makefile' || fn === 'gnumakefile' || fn === 'cmakelists.txt' || fn.endsWith('.mk') || e === 'cmake' || e === 'make'
    },
    {
        id: 'composer',
        match: (fn) => fn === 'composer.json' || fn === 'composer.lock'
    },
    {
        id: 'gemfile',
        match: (fn) => fn === 'gemfile' || fn === 'gemfile.lock'
    },
    {
        id: 'pyproject',
        match: (fn) => fn === 'pyproject.toml' || fn === 'requirements.txt' || fn === 'requirements.in' || fn === 'poetry.lock' || fn === 'pipfile' || fn === 'pipfile.lock' || fn === 'setup.py' || fn === 'setup.cfg'
    },
    {
        id: 'gitignore',
        match: (fn) => fn === '.gitignore' || fn === '.gitattributes' || fn === '.gitmodules' || fn === '.npmignore' || fn === '.eslintignore' || fn === '.prettierignore' || fn === '.hgignore' || fn === '.cvsignore'
    },
    {
        id: 'lockfile',
        match: (fn) => fn.endsWith('.lock') && fn !== 'cargo.lock' && fn !== 'yarn.lock' && fn !== 'bun.lock' && fn !== 'poetry.lock' && fn !== 'composer.lock' && fn !== 'gemfile.lock' && fn !== 'pipfile.lock'
    },
    {
        id: 'tsconfig',
        match: (fn) => fn === 'tsconfig.json' || fn.startsWith('tsconfig.') && fn.endsWith('.json') || fn === 'jsconfig.json'
    },
    {
        id: 'eslint',
        match: (fn) => fn === 'eslint.config.js' || fn === 'eslint.config.mjs' || fn === 'eslint.config.cjs' || fn === 'eslint.config.ts' || fn.startsWith('.eslintrc')
    },
    {
        id: 'prettier',
        match: (fn) => fn === 'prettier.config.js' || fn === 'prettier.config.mjs' || fn === 'prettier.config.cjs' || fn.startsWith('.prettierrc')
    },
    {
        id: 'vite',
        match: (fn) => fn.startsWith('vite.config.') || fn.startsWith('vitest.config.')
    },
    {
        id: 'webpack',
        match: (fn) => fn.startsWith('webpack.config.') || fn.startsWith('webpack.')
    },
    {
        id: 'github',
        match: (fn) => fn === 'action.yml' || fn === 'action.yaml' || fn.endsWith('.github/workflows') || fn.includes('.github') && (fn.endsWith('.yml') || fn.endsWith('.yaml'))
    },
    {
        id: 'nix',
        match: (fn, e) => e === 'nix' || fn === 'flake.nix' || fn === 'default.nix' || fn === 'shell.nix' || fn === 'flake.lock'
    },
    {
        id: 'justfile',
        match: (fn) => fn === 'justfile' || fn === '.justfile'
    },
    {
        id: 'procfile',
        match: (fn) => fn === 'procfile'
    },
    {
        id: 'rescript',
        match: (fn, e) => e === 'resi' || fn.endsWith('.res.js') || (e === 'res' && !fn.endsWith('.rc.res'))
    }
];

/**
 * Extension → type id. Built once; no cascading if/includes chains.
 * More specific ids replace broad buckets (react≠component, apk≠app, etc.).
 */
const EXTENSION_TO_TYPE = {
    tf: 'terraform', tfvars: 'terraform', hcl: 'terraform',
    ipynb: 'notebook',
    wasm: 'wasm', wat: 'wasm',
    cmake: 'makefile', make: 'makefile', mk: 'makefile',
    jar: 'jar', war: 'war', ear: 'ear', aar: 'aar',
    apk: 'apk', aab: 'apk', ipa: 'app',
    zip: 'archive', tar: 'archive', gz: 'archive', tgz: 'archive',
    '7z': 'archive', rar: 'archive', bz2: 'archive', xz: 'archive',
    zst: 'archive', lz4: 'archive', sz: 'archive', cab: 'archive', wim: 'archive',
    iso: 'disk', img: 'disk', vmdk: 'disk', qcow2: 'disk', dmg: 'disk', vdi: 'disk', vhd: 'disk', vhdx: 'disk',
    exe: 'exe', com: 'exe', scr: 'exe',
    msi: 'msi', msp: 'msi', msu: 'msi',
    deb: 'pkg', rpm: 'pkg', pkg: 'pkg', snap: 'pkg', appimage: 'pkg', flatpak: 'pkg',
    dll: 'binary', so: 'binary', dylib: 'binary', bin: 'binary',
    class: 'binary', o: 'binary', a: 'binary', pyc: 'binary', pyo: 'binary', rlib: 'binary',
    jsx: 'react', tsx: 'react',
    vue: 'vue',
    svelte: 'svelte',
    astro: 'astro',
    html: 'html', htm: 'html', xhtml: 'html',
    css: 'style', styl: 'style', pcss: 'style',
    scss: 'scss', sass: 'scss',
    less: 'less',
    xml: 'xml', xsl: 'xml', xsd: 'xml', xslt: 'xml', dtd: 'xml', wsdl: 'xml',
    sh: 'script', bash: 'script', zsh: 'script', fish: 'script', ksh: 'script', csh: 'script',
    bat: 'batch', cmd: 'batch',
    ps1: 'powershell', psm1: 'powershell', psd1: 'powershell',
    reg: 'registry',
    rs: 'rust',
    go: 'golang',
    py: 'python', pyw: 'python', pyi: 'python', pyx: 'python',
    java: 'java',
    kt: 'kotlin', kts: 'kotlin',
    c: 'clang',
    cpp: 'cpp', cc: 'cpp', cxx: 'cpp', 'c++': 'cpp',
    h: 'cheader', hpp: 'cheader', hh: 'cheader', hxx: 'cheader',
    cs: 'csharp',
    php: 'php', phtml: 'php',
    rb: 'ruby', erb: 'ruby', rake: 'ruby',
    swift: 'swift',
    sql: 'sql', pgsql: 'sql', mysql: 'sql',
    js: 'javascript', mjs: 'javascript', cjs: 'javascript',
    ts: 'typescript', mts: 'typescript', cts: 'typescript',
    scala: 'scala', sc: 'scala',
    dart: 'dart',
    lua: 'lua',
    zig: 'zig',
    r: 'rlang', rmd: 'rlang',
    jl: 'julia',
    hs: 'haskell', lhs: 'haskell',
    clj: 'clojure', cljs: 'clojure', cljc: 'clojure', edn: 'clojure',
    ex: 'elixir', exs: 'elixir',
    erl: 'erlang', hrl: 'erlang',
    pl: 'perl', pm: 'perl', t: 'perl',
    asm: 'asm', s: 'asm', nasm: 'asm',
    sol: 'solidity',
    nim: 'nim', nims: 'nim',
    m: 'objc', mm: 'objc',
    fs: 'fsharp', fsx: 'fsharp', fsi: 'fsharp', fsscript: 'fsharp',
    ml: 'ocaml', mli: 'ocaml',
    groovy: 'groovy', gvy: 'groovy', gy: 'groovy', gsh: 'groovy',
    cr: 'crystal',
    v: 'vlang', vv: 'vlang',
    yaml: 'yaml', yml: 'yaml',
    json: 'json', json5: 'json', jsonc: 'json',
    geojson: 'geojson', topojson: 'geojson',
    proto: 'proto', protobuf: 'proto',
    graphql: 'graphql', gql: 'graphql',
    toml: 'toml',
    ini: 'config', conf: 'config', config: 'config', properties: 'config',
    editorconfig: 'config', cfg: 'config', prefs: 'config',
    png: 'image', jpg: 'image', jpeg: 'image', gif: 'image', webp: 'image',
    bmp: 'image', ico: 'image', tiff: 'image', tif: 'image', avif: 'image',
    heic: 'image', heif: 'image', raw: 'image', cr2: 'image', nef: 'image',
    svg: 'svg',
    psd: 'design', ai: 'design', eps: 'design', sketch: 'design', fig: 'design', xd: 'design',
    mp3: 'audio', wav: 'audio', ogg: 'audio', flac: 'audio', aac: 'audio',
    m4a: 'audio', wma: 'audio', opus: 'audio', mid: 'audio', midi: 'audio', aiff: 'audio',
    mp4: 'video', mkv: 'video', avi: 'video', mov: 'video', webm: 'video',
    flv: 'video', wmv: 'video', m4v: 'video', '3gp': 'video', ogv: 'video',
    gltf: 'model', glb: 'model', obj: 'model', fbx: 'model', stl: 'model',
    dae: 'model', '3ds': 'model', blend: 'model', usdz: 'model',
    pdf: 'pdf',
    md: 'markdown', markdown: 'markdown', mdown: 'markdown', mkd: 'markdown', mdx: 'markdown',
    doc: 'word', docx: 'word', rtf: 'word', odt: 'word', pages: 'word',
    txt: 'txt', text: 'txt',
    log: 'log',
    ppt: 'presentation', pptx: 'presentation', keynote: 'presentation', odp: 'presentation',
    epub: 'ebook', mobi: 'ebook', azw: 'ebook', azw3: 'ebook', cbz: 'ebook', cbr: 'ebook', fb2: 'ebook',
    csv: 'csv', tsv: 'csv',
    xlsx: 'spreadsheet', xls: 'spreadsheet', ods: 'spreadsheet', numbers: 'spreadsheet',
    db: 'database', sqlite: 'database', sqlite3: 'database', mdb: 'database',
    accdb: 'database', dbf: 'database', dump: 'database', sqlitedb: 'database',
    md5: 'checksum', sha1: 'checksum', sha256: 'checksum', sha512: 'checksum', sha3: 'checksum', sfv: 'checksum',
    key: 'security',
    asc: 'security', sig: 'security', pem: 'security', crt: 'security', cer: 'security',
    p12: 'security', pfx: 'security', keystore: 'security', jks: 'security',
    pub: 'security', gpg: 'security', der: 'security', csr: 'security',
    ttf: 'font', otf: 'font', woff: 'font', woff2: 'font', eot: 'font',
    coffee: 'coffee', litcoffee: 'coffee',
    elm: 'elm',
    f: 'fortran', for: 'fortran', f90: 'fortran', f95: 'fortran', f03: 'fortran', f08: 'fortran',
    mat: 'matlab', octave: 'matlab', mlx: 'matlab',
    lisp: 'lisp', cl: 'lisp', el: 'lisp', scm: 'lisp', ss: 'lisp', racket: 'lisp', rkt: 'lisp',
    vb: 'vb', vbs: 'vb', bas: 'vb', vba: 'vb',
    hx: 'haxe', hxml: 'haxe',
    d: 'dlang', di: 'dlang',
    pas: 'pascal', pp: 'pascal', dpr: 'pascal', lpr: 'pascal',
    gleam: 'gleam',
    odin: 'odin',
    resi: 'rescript',
    plg: 'prolog', pro: 'prolog',
    nupkg: 'nupkg', snupkg: 'nupkg',
    whl: 'whl',
    gem: 'gem',
    vsix: 'vsix',
    crx: 'crx',
    phar: 'phar',
    glsl: 'shader', vert: 'shader', frag: 'shader', comp: 'shader', geom: 'shader', tesc: 'shader', tese: 'shader',
    hlsl: 'shader', wgsl: 'shader', metal: 'shader', spv: 'shader',
    tex: 'latex', latex: 'latex', ltx: 'latex', sty: 'latex', cls: 'latex', bib: 'latex',
    rst: 'rst', rest: 'rst',
    adoc: 'adoc', asciidoc: 'adoc', asciidoctor: 'adoc',
    parquet: 'parquet',
    avro: 'avro',
    pkl: 'pickle', pickle: 'pickle',
    onnx: 'mlmodel', pt: 'mlmodel', pth: 'mlmodel', safetensors: 'mlmodel',
    gguf: 'mlmodel', ggml: 'mlmodel', pb: 'mlmodel', h5: 'mlmodel', hdf5: 'mlmodel',
    tflite: 'mlmodel', keras: 'mlmodel', joblib: 'mlmodel',
    srt: 'subtitle', vtt: 'subtitle', ass: 'subtitle', ssa: 'subtitle', sub: 'subtitle',
    plist: 'plist',
    ics: 'calendar', ical: 'calendar', ifb: 'calendar',
    eml: 'email', msg: 'email',
    nix: 'nix'
};

/**
 * Map a file name and extension to an internal type id (catalog key or generic).
 * @param {string} [fileName=''] - Full file name for special-rule matching.
 * @param {string} [ext=''] - Extension without leading dot (derived from name if empty).
 * @returns {string}
 */
function resolveTypeId(fileName = '', ext = '') {
    const fn = (fileName || '').toLowerCase();
    let e = (ext || '').toLowerCase();
    if (!e && fn) {
        const idx = fn.lastIndexOf('.');
        if (idx > 0) e = fn.substring(idx + 1);
    }

    for (const rule of SPECIAL_RULES) {
        if (rule.match(fn, e)) return rule.id;
    }

    if (e && EXTENSION_TO_TYPE[e]) return EXTENSION_TO_TYPE[e];
    return 'generic';
}

/**
 * Resolve a file-type category id from extension and optional file name.
 * @param {string} ext - File extension without leading dot.
 * @param {string} [fileName=''] - Full file name for special-rule matching.
 * @returns {string}
 */
export function getFileTypeCategory(ext, fileName = '') {
    return resolveTypeId(fileName, ext);
}

/**
 * Get the icon name for a file-type category id.
 * @param {string} category - Category id from the catalog.
 * @returns {string}
 */
export function getCategoryIconName(category) {
    return FILE_TYPE_CATALOG[category]?.icon || FILE_TYPE_CATALOG.generic.icon;
}

/** @deprecated Prefer FILE_TYPE_CATALOG + SPECIAL_RULES; kept for external consumers. */
export const SPECIAL_FILE_TYPES = SPECIAL_RULES.map((rule) => {
    const meta = FILE_TYPE_CATALOG[rule.id] || FILE_TYPE_CATALOG.generic;
    return {match: rule.match, key: meta.key, defaultLabel: meta.defaultLabel};
});

/** @deprecated Prefer FILE_TYPE_CATALOG + EXTENSION_TO_TYPE; kept for external consumers. */
export const EXTENSION_FILE_TYPES = Object.fromEntries(
    Object.entries(EXTENSION_TO_TYPE).map(([ext, id]) => {
        const meta = FILE_TYPE_CATALOG[id] || FILE_TYPE_CATALOG.generic;
        return [ext, [meta.key, meta.defaultLabel]];
    })
);

/**
 * Resolve i18n key, default label, and category id for a file.
 * @param {string} fileName - File name.
 * @param {string} [ext] - File extension without leading dot.
 * @returns {{key: string, defaultLabel: string, id: string}}
 */
export function getFileTypeInfo(fileName, ext) {
    const id = resolveTypeId(fileName, ext);
    const meta = FILE_TYPE_CATALOG[id] || FILE_TYPE_CATALOG.generic;
    return {key: meta.key, defaultLabel: meta.defaultLabel, id};
}

/**
 * Resolve a localized (or default) display label for a file type.
 * @param {string} fileName - File name.
 * @param {string} [ext] - File extension without leading dot.
 * @returns {string}
 */
export function getFileTypeLabel(fileName, ext) {
    const info = getFileTypeInfo(fileName, ext);
    return t(info.key) || info.defaultLabel;
}

const PREVIEWABLE_IMAGE_RE = /\.(png|jpg|jpeg|gif|webp|bmp|svg|avif|ico)$/i;
const PREVIEWABLE_TEXT_RE = /\.(pom|module|xml|json|jsonc|json5|txt|md|mdx|yml|yaml|properties|log|patch|diff|sh|bash|zsh|py|js|mjs|cjs|ts|mts|cts|go|rs|toml|ini|conf|env|gradle|hcl|tf|sql|css|scss|sass|less|html|htm|vue|svelte|jsx|tsx|kt|kts|java|c|cpp|h|hpp|cs|php|rb|swift|lua|dart|zig|r|jl|ex|exs|erl|pl|sol|nim|graphql|gql|proto|dockerfile|coffee|elm|f90|f95|lisp|scm|vb|hx|pas|gleam|odin|res|resi|tex|rst|adoc|nix|glsl|hlsl|wgsl|srt|vtt|plist|ics|eml|justfile|procfile)$/i;

/**
 * Browser file/directory list item custom element.
 */
export class RenopFileItem extends HTMLElement {
    /**
     * @returns {string[]}
     */
    static get observedAttributes() {
		return ['file-name', 'file-type', 'file-size', 'index', 'path', 'signed'];
    }

    /**
     * Subscribe to language changes and render.
     * @returns {void}
     */
    connectedCallback() {
        this._onLangChange = () => this.render();
        window.addEventListener('languageChanged', this._onLangChange);
        this.render();
    }

    /**
     * Re-render when the server changes the verified-signature state.
     * @param {string} name - Changed attribute name.
     * @param {string|null} oldValue - Previous attribute value.
     * @param {string|null} newValue - New attribute value.
     * @returns {void}
     */
    attributeChangedCallback(name, oldValue, newValue) {
        if (name === 'signed' && oldValue !== newValue && this.isConnected) {
            this.render();
        }
    }

    /**
     * Unsubscribe language listener when removed from the DOM.
     * @returns {void}
     */
    disconnectedCallback() {
        if (this._onLangChange) {
            window.removeEventListener('languageChanged', this._onLangChange);
            this._onLangChange = null;
        }
    }

    /**
     * Rebuild the list item link, icon, type badge, and actions.
     * @returns {void}
     */
    render() {
        const fileName = this.getAttribute('file-name') || '';
        const fileType = this.getAttribute('file-type') || 'FILE';
        const fileSize = this.getAttribute('file-size');
        const index = parseInt(this.getAttribute('index') || '0', 10);
        const path = this.getAttribute('path') || '/';
		const signed = this.hasAttribute('signed');
        const isDir = fileType === 'DIRECTORY';
        const isRootRepo = (path === '/' || path === '' || path === undefined) && isDir;

        let category = 'generic';
        let iconName = 'file';
        let typeLabel = '';
        let typeI18nKey = '';

        if (isRootRepo) {
            category = 'repo';
            iconName = 'box';
            typeI18nKey = 'browser.repository';
            typeLabel = t('browser.repository') || t('repos.repository') || 'Repository';
        } else if (isDir) {
            category = 'dir';
            iconName = 'folder';
            typeI18nKey = 'browser.folder';
            typeLabel = t('browser.folder') || 'Folder';
        } else {
            const ext = (fileName.lastIndexOf('.') > 0) ? fileName.substring(fileName.lastIndexOf('.') + 1) : '';
            category = getFileTypeCategory(ext, fileName);
            iconName = getCategoryIconName(category);
            const info = getFileTypeInfo(fileName, ext);
            typeI18nKey = info.key;
            typeLabel = t(info.key) || info.defaultLabel;
        }

        const keepAdded = this.classList.contains('file-item--added');
        const keepRemoving = this.classList.contains('is-removing');
        this.className = `file-item ${isRootRepo ? 'file-item--repo' : isDir ? 'file-item--dir' : 'file-item--file'}`;
        if (keepAdded) this.classList.add('file-item--added');
        if (keepRemoving) this.classList.add('is-removing');
        this.dataset.fileName = fileName;
        this.style.setProperty('--item-index', String(Math.min(index, 14)));
        this.innerHTML = '';

        const fullPath = (path.endsWith('/') ? path : path + '/') + encodePathSegment(fileName);

        const a = el('a', {class: 'file-item-link', href: fullPath});
        if (isDir) {
            a.addEventListener('click', (e) => {
                this.dispatchEvent(new CustomEvent('navigate', {bubbles: true, detail: {path: fullPath, event: e}}));
            });
        } else {
            a.target = '_blank';
        }

        const iconSpan = el('span', {class: `file-icon file-icon--${category}`}, createIcon(iconName));

        const nameSpan = el('span', {class: 'file-name'}, fileName);
        const typeBadge = el('span', {
            class: `file-type-badge file-type-badge--${category}`,
            'data-i18n': typeI18nKey
        }, typeLabel);
        const nameWrap = el('div', {class: 'file-name-wrap'}, nameSpan, typeBadge);

        const leftDiv = el('div', {class: 'file-entry-left'}, iconSpan, nameWrap);
        a.appendChild(leftDiv);

        const rightDiv = el('div', {class: 'file-entry-right'});
        const lowerName = fileName.toLowerCase();
        const isJavadoc = category === 'javadoc' || lowerName.endsWith('-javadoc.jar');
        const isImage = category === 'image' || category === 'svg' || PREVIEWABLE_IMAGE_RE.test(fileName);
        const isPreviewableText = PREVIEWABLE_TEXT_RE.test(fileName) || category === 'markdown' || category === 'txt' || category === 'log' || category === 'yaml' || category === 'json' || category === 'xml' || category === 'config' || category === 'toml' || category === 'script' || category === 'diff';

		if (!isDir && signed) {
			const signatureBtn = el('button', {
				type: 'button',
				class: 'file-action-btn file-action-btn--signature',
				title: t('browser.signatureDetails'),
				ariaLabel: `${t('browser.signatureDetails')} ${fileName}`
			}, createIcon('fileLock'));
			signatureBtn.addEventListener('click', e => {
				e.preventDefault();
				e.stopPropagation();
				this.dispatchEvent(new CustomEvent('signature', {
					bubbles: true,
					detail: {fileName, fullPath, event: e}
				}));
			});
			rightDiv.appendChild(signatureBtn);
		}

        if (!isDir && (isImage || isJavadoc || isPreviewableText)) {
            const previewHref = isJavadoc ? (`/javadoc` + fullPath) : (fullPath + '?preview=true');
            let previewTitle = t('browser.previewImage');
            if (isJavadoc) {
                previewTitle = t('browser.openJavadoc');
            } else if (isPreviewableText) {
                previewTitle = t('browser.previewFile') || 'Preview file';
            }

            const previewBtn = el('a', {
                class: `file-action-btn ${isJavadoc ? 'file-action-btn--docs' : 'file-action-btn--preview'}`,
                target: '_blank',
                rel: 'noopener noreferrer',
                title: previewTitle,
                ariaLabel: `${previewTitle} ${fileName}`,
                href: previewHref
            }, createIcon(isJavadoc ? 'docs' : 'eye'));
            rightDiv.appendChild(previewBtn);
        }

        if (!isDir && fileSize !== null && fileSize !== undefined) {
            rightDiv.appendChild(el('span', {class: 'file-size'}, fileSize));
        } else if (isDir) {
            rightDiv.appendChild(el('span', {class: 'file-chevron', 'aria-hidden': 'true'}, createIcon('chevron')));
        }

        if (!isRootRepo && canUpdateRepo(repoNameFromPath(path))) {
            const deleteBtn = el('button', {
                type: 'button',
                class: 'file-action-btn file-action-btn--delete',
                title: t('common.delete') || 'Delete',
                ariaLabel: `${t('common.delete') || 'Delete'} ${fileName}`
            }, createIcon('delete'));

            deleteBtn.onclick = (e) => {
                e.preventDefault();
                e.stopPropagation();
                this.dispatchEvent(new CustomEvent('delete', {bubbles: true, detail: {fileName, fullPath, event: e}}));
            };

            rightDiv.appendChild(deleteBtn);
        }

        a.appendChild(rightDiv);
        this.appendChild(a);
    }
}

if (!customElements.get('renop-file-item')) {
    customElements.define('renop-file-item', RenopFileItem);
}

/**
 * Create a browser file/directory list item.
 * @param {{name: string, type: string, signed?: boolean}} file - File entry with name, type, and signature state.
 * @param {number} index - List index for staggered animation.
 * @param {string} path - Parent path of the entry.
 * @param {object} [options={}] - Extra options.
 * @param {string} [options.formattedSize] - Pre-formatted size for files.
 * @param {Function} [options.onNavigate] - Handler for directory navigate events.
 * @param {Function} [options.onDelete] - Handler for delete events.
 * @param {Function} [options.onSignature] - Handler for signature-detail events.
 * @returns {HTMLElement}
 */
export function createFileItem(file, index, path, {formattedSize, onNavigate, onDelete, onSignature} = {}) {
    const item = document.createElement('renop-file-item');
    item.setAttribute('file-name', file.name);
    item.setAttribute('file-type', file.type);
    if (formattedSize) {
        item.setAttribute('file-size', formattedSize);
    }
	if (file.signed === true) {
		item.setAttribute('signed', '');
	}
    item.setAttribute('index', String(index));
    item.setAttribute('path', path);

    if (onNavigate) item.addEventListener('navigate', e => onNavigate(e.detail));
    if (onDelete) item.addEventListener('delete', e => onDelete(e.detail));
	if (onSignature) item.addEventListener('signature', e => onSignature(e.detail));
    return item;
}
