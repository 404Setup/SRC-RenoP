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
 * Wrap paths in the shared stroke-based SVG frame.
 * @param {string} body - SVG child markup.
 * @param {number} [size=18] - Default rendered size.
 * @param {number} [strokeWidth=2] - Default stroke width.
 * @returns {string} Complete inline SVG markup.
 */
function strokeIcon(body, size = 18, strokeWidth = 2) {
    return `<svg viewBox="0 0 24 24" width="${size}" height="${size}" fill="none" stroke="currentColor" stroke-width="${strokeWidth}" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${body}</svg>`;
}

const baseIcons = Object.freeze({
    user: strokeIcon('<path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/>', 20),
    bell: strokeIcon('<path d="M18 8a6 6 0 0 0-12 0c0 7-3 7-3 9h18c0-2-3-2-3-9"/><path d="M10 21h4"/>'),
    send: strokeIcon('<path d="m22 2-7 20-4-9-9-4Z"/><path d="M22 2 11 13"/>'),
    logout: strokeIcon('<path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><path d="m16 17 5-5-5-5"/><path d="M21 12H9"/>'),
    userPlus: strokeIcon('<path d="M16 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"/><circle cx="8.5" cy="7" r="4"/><path d="M20 8v6M23 11h-6"/>', 15, 2.2),
    edit: strokeIcon('<path d="M12 20h9"/><path d="M16.5 3.5a2.12 2.12 0 0 1 3 3L7 19l-4 1 1-4Z"/>', 14, 2.2),
    delete: strokeIcon('<path d="M3 6h18M8 6V4h8v2M19 6l-1 14H6L5 6M10 11v5M14 11v5"/>', 14, 2.2),
    check: strokeIcon('<path d="m20 6-11 11-5-5"/>', 12, 3),
    success: strokeIcon('<circle cx="12" cy="12" r="9"/><path d="m8 12 2.5 2.5L16 9"/>', 22, 2.3),
    upload: strokeIcon('<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M17 8l-5-5-5 5M12 3v12"/>', 20, 1.8),
    download: strokeIcon('<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4M7 10l5 5 5-5M12 15V3"/>', 14, 2.2),
    box: strokeIcon('<path d="m21 8-9 5-9-5 9-5 9 5Z"/><path d="m3 8 9 5 9-5v8l-9 5-9-5Z"/><path d="M12 13v8"/>', 20, 1.8),
    plus: strokeIcon('<path d="M12 5v14M5 12h14"/>', 14, 2.2),
    close: strokeIcon('<path d="m18 6-12 12M6 6l12 12"/>', 14, 2.5),
    updater: strokeIcon('<path d="M21 12a9 9 0 1 1-2.64-6.36L21 8M21 3v5h-5"/>', 20),
    identity: strokeIcon('<rect x="3" y="4" width="18" height="16" rx="3"/><circle cx="9" cy="10" r="2.5"/><path d="M5.5 17a3.5 3.5 0 0 1 7 0M15 9h3M15 13h3"/>', 20, 1.8),
    branding: strokeIcon('<rect x="3" y="3" width="18" height="18" rx="3"/><circle cx="9" cy="9" r="2"/><path d="m21 15-3-3a2 2 0 0 0-3 0l-9 9"/>', 20),
    compliance: strokeIcon('<path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10Z"/><path d="m9 12 2 2 4-5"/>', 20),
    ssl: strokeIcon('<rect x="3" y="11" width="18" height="10" rx="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/>', 20),
    performance: strokeIcon('<path d="M13 2 3 14h9l-1 8 10-12h-9Z"/>', 20),
    network: strokeIcon('<circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3a14 14 0 0 1 0 18M12 3a14 14 0 0 0 0 18"/>', 20, 1.8),
    storage: strokeIcon('<ellipse cx="12" cy="5" rx="9" ry="3"/><path d="M3 5v14c0 1.7 4 3 9 3s9-1.3 9-3V5M3 12c0 1.7 4 3 9 3s9-1.3 9-3"/>', 20),
    info: strokeIcon('<circle cx="12" cy="12" r="9"/><path d="M12 11v5M12 8h.01"/>'),
    warning: strokeIcon('<path d="m12 3 10 18H2L12 3Z"/><path d="M12 9v5M12 18h.01"/>'),
    refresh: strokeIcon('<path d="M20 7V3h-4M4 17v4h4M19 11a7 7 0 0 0-12-4L4 10M5 13a7 7 0 0 0 12 4l3-3"/>'),
    chevron: strokeIcon('<path d="m9 18 6-6-6-6"/>', 14, 2.2),
    chevronLeft: strokeIcon('<path d="m15 18-6-6 6-6"/>', 14, 2.2),
    chevronRight: strokeIcon('<path d="m9 18 6-6-6-6"/>', 14, 2.2),
    chevronDown: strokeIcon('<path d="m6 9 6 6 6-6"/>', 14),
    clock: strokeIcon('<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>', 14),
    alertCircle: strokeIcon('<circle cx="12" cy="12" r="9"/><path d="M12 8v5M12 17h.01"/>', 16),
    folder: strokeIcon('<path d="M3 6a2 2 0 0 1 2-2h5l2 3h7a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2Z"/>', 20, 1.8),
    docs: strokeIcon('<path d="M3 4h6a3 3 0 0 1 3 3v14a3 3 0 0 0-3-3H3ZM21 4h-6a3 3 0 0 0-3 3v14a3 3 0 0 1 3-3h6Z"/>'),
    eye: strokeIcon('<path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12Z"/><circle cx="12" cy="12" r="3"/>'),
    copy: strokeIcon('<rect x="9" y="9" width="12" height="12" rx="2"/><path d="M6 15H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h8a2 2 0 0 1 2 2v1"/>', 16),
    settings: strokeIcon('<path d="M4 6h6M14 6h6M4 12h2M10 12h10M4 18h9M17 18h3"/><circle cx="12" cy="6" r="2"/><circle cx="8" cy="12" r="2"/><circle cx="15" cy="18" r="2"/>', 18, 1.8),
    viewport: strokeIcon('<rect x="3" y="5" width="18" height="14" rx="2"/><path d="M8 21h8M12 19v2M7 9h10"/>', 20),
    file: strokeIcon('<path d="M6 2h8l4 4v16H6a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2Z"/><path d="M14 2v6h6"/>', 20, 1.8),
    fileText: strokeIcon('<path d="M6 2h8l4 4v16H6a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2Z"/><path d="M14 2v6h6M8 13h8M8 17h6"/>', 18, 1.8),
    fileArchive: strokeIcon('<path d="M6 2h8l4 4v16H6a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2Z"/><path d="M14 2v6h6M10 6h3M10 10h3M10 14h3M10 18h3"/>', 18, 1.8),
    fileCode: strokeIcon('<path d="M6 2h8l4 4v16H6a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2Z"/><path d="M14 2v6h6m-9 5-3 3 3 3m3-6 3 3-3 3"/>', 18, 1.8),
    fileConfig: strokeIcon('<path d="M6 2h8l4 4v16H6a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2Z"/><path d="M14 2v6h6M8 13h8M8 17h8M11 11v4M14 15v4"/>', 18, 1.8),
    filePackage: strokeIcon('<path d="m12 3 9 5-9 5-9-5 9-5Z"/><path d="m3 8 9 5 9-5v8l-9 5-9-5ZM12 13v8"/>', 18, 1.8),
    fileImage: strokeIcon('<rect x="3" y="3" width="18" height="18" rx="3"/><circle cx="8.5" cy="8.5" r="1.5"/><path d="m21 15-5-5L5 21"/>'),
    fileAudio: strokeIcon('<path d="M9 18V5l11-2v13"/><circle cx="6" cy="18" r="3"/><circle cx="17" cy="16" r="3"/>'),
    fileVideo: strokeIcon('<rect x="3" y="4" width="14" height="16" rx="2"/><path d="m17 9 4-2v10l-4-2Z"/>'),
    fileData: strokeIcon('<ellipse cx="12" cy="5" rx="8" ry="3"/><path d="M4 5v14c0 1.7 3.6 3 8 3s8-1.3 8-3V5M4 12c0 1.7 3.6 3 8 3s8-1.3 8-3"/>', 18, 1.8),
    fileSecurity: strokeIcon('<path d="M12 22s7-3.5 7-9V6l-7-3-7 3v7c0 5.5 7 9 7 9Z"/><rect x="9" y="11" width="6" height="5" rx="1"/><path d="M10 11V9a2 2 0 0 1 4 0v2"/>', 18, 1.8),
    fileBinary: strokeIcon('<rect x="3" y="4" width="18" height="16" rx="3"/><path d="M7 9h2v6H7ZM15 9h2v6h-2ZM11 9h2M11 15h2"/>', 18, 1.8),
    fileFont: strokeIcon('<path d="m5 20 7-16 7 16M8 14h8"/>'),
    fileModel: strokeIcon('<path d="m12 2 9 5v10l-9 5-9-5V7Z"/><path d="m3 7 9 5 9-5M12 12v10"/>', 18, 1.8),
    fileHash: strokeIcon('<path d="M4 9h16M4 15h16M10 3 8 21M16 3l-2 18"/>'),
    repositoryMaven: strokeIcon('<path d="M4 19V6l4-2 4 5 4-5 4 2v13M8 8v7M16 8v7M4 19h16"/>', 20, 1.8),
    repositoryCargo: strokeIcon('<circle cx="12" cy="12" r="4"/><path d="M12 2v3M12 19v3M2 12h3M19 12h3M5 5l2 2M17 17l2 2M5 19l2-2M17 7l2-2"/>', 20, 1.8),
    repositoryDocker: strokeIcon('<path d="M3 11h4v4H3ZM8 11h4v4H8ZM13 11h4v4h-4ZM8 6h4v4H8ZM13 6h4v4h-4ZM18 11h3v4h-3Z"/><path d="M2 16c2 3 5 4 9 4 6 0 10-3 11-7"/>', 20, 1.8),
    repositoryFiles: strokeIcon('<path d="M3 7a2 2 0 0 1 2-2h5l2 3h7a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2Z"/><path d="M8 12h8M8 16h5"/>', 20, 1.8),
    repositoryNpm: strokeIcon('<path d="M3 5h18v14H3Z"/><path d="M7 15V9h4l3 6V9h3v6"/>', 20, 1.8),
    externalLink: strokeIcon('<path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6M15 3h6v6M10 14 21 3"/>', 14, 2),
    oauthGithub: '<svg viewBox="0 0 24 24" width="20" height="20" fill="currentColor" aria-hidden="true"><path d="M12 2C6.477 2 2 6.484 2 12.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.53 1.032 1.53 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0 1 12 6.844c.85.004 1.705.115 2.504.337 1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.202 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.943.359.309.678.92.678 1.855 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.019 10.019 0 0 0 22 12.017C22 6.484 17.522 2 12 2z"/></svg>',
    oauthGoogle: '<svg viewBox="0 0 24 24" width="20" height="20" aria-hidden="true"><path fill="#4285F4" d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z"/><path fill="#34A853" d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z"/><path fill="#FBBC05" d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.06H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.94l2.85-2.22.81-.63z"/><path fill="#EA4335" d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.06l3.66 2.84c.87-2.6 3.3-4.52 6.16-4.52z"/></svg>',
    oauthMicrosoft: '<svg viewBox="0 0 24 24" width="20" height="20" aria-hidden="true"><rect x="1" y="1" width="10" height="10" fill="#F25022"/><rect x="13" y="1" width="10" height="10" fill="#7FBA00"/><rect x="1" y="13" width="10" height="10" fill="#00A4EF"/><rect x="13" y="13" width="10" height="10" fill="#FFB900"/></svg>',
    oauthGitlab: '<svg viewBox="0 0 24 24" width="20" height="20" fill="none" aria-hidden="true"><path fill="#E24329" d="m12 20.7-3.8-11.7h7.6L12 20.7Z"/><path fill="#FC6D26" d="m12 20.7-3.8-11.7H2.6L12 20.7Z"/><path fill="#FCA326" d="M2.6 9h5.6L5.7 1.4c-.2-.6-1-.6-1.2 0L2.6 9Z"/><path fill="#E24329" d="M2.6 9 1.1 13.6c-.2.6 0 1.3.6 1.7L12 20.7 2.6 9Z"/><path fill="#FC6D26" d="m12 20.7 3.8-11.7h5.6L12 20.7Z"/><path fill="#FCA326" d="M21.4 9h-5.6l2.5-7.6c.2-.6 1-.6 1.2 0L21.4 9Z"/><path fill="#E24329" d="m21.4 9 1.5 4.6c.2.6 0 1.3-.6 1.7L12 20.7l9.4-11.7Z"/></svg>',
    oauthCloudflare: '<svg viewBox="0 0 24 24" width="20" height="20" fill="none" aria-hidden="true"><path fill="#F38020" d="M18.27 9.87a6.22 6.22 0 0 0-11.83.69 4.29 4.29 0 0 0-2.69 3.97c0 2.37 1.93 4.29 4.3 4.29h10.3a3.86 3.86 0 0 0 .19-7.71c-.09-.44-.19-.85-.27-1.24Z"/><path fill="#FAAE40" d="M18.46 10.74c-.09-.29-.18-.58-.27-.87a6.22 6.22 0 0 0-11.83.69 4.29 4.29 0 0 0-2.69 3.97c0 .24.02.48.06.71a4.28 4.28 0 0 1 3.52-2.73 5.48 5.48 0 0 1 10.23-.74c.36-.36.69-.71.98-1.03Z"/></svg>',
    oauthStackexchange: '<svg viewBox="0 0 24 24" width="20" height="20" fill="currentColor" aria-hidden="true"><path d="M4 17h16v4H4v-4Zm1.1-3.6 15.6 3.3.7-3.2-15.6-3.3-.7 3.2Zm2.5-6.5 14.1 7.5 1.5-2.9-14.1-7.5-1.5 2.9ZM12.1 2l11.3 11.3 2.3-2.3L14.4-.3 12.1 2Z"/></svg>',
    oauthCustom: strokeIcon('<circle cx="12" cy="12" r="10"/><path d="m4.93 4.93 4.24 4.24M14.83 9.17l4.24-4.24M14.83 14.83l4.24 4.24M9.17 14.83l-4.24 4.24"/>', 20, 1.8),
});

const iconAliasGroups = Object.freeze({
    user: ['username'],
    oauthGithub: ['github'],
    oauthGoogle: ['google'],
    oauthMicrosoft: ['microsoft', 'entra'],
    oauthGitlab: ['gitlab'],
    oauthCloudflare: ['cloudflare'],
    oauthStackexchange: ['stackexchange'],
    oauthCustom: ['oauth', 'custom'],
    fileText: ['description', 'fileBook', 'filePresentation', 'fileJavadoc', 'filePdf', 'fileMarkdown', 'fileWord', 'fileTxt', 'fileLog', 'fileLatex', 'fileRst', 'fileAdoc', 'fileSubtitle', 'fileCalendar', 'fileEmail'],
    fileArchive: ['fileZip'],
    fileCode: ['fileTerminal', 'fileWeb', 'fileDiff', 'fileRust', 'filePython', 'fileGo', 'fileJava', 'fileKotlin', 'fileC', 'fileCpp', 'fileCsharp', 'filePhp', 'fileRuby', 'fileSwift', 'fileSql', 'fileShell', 'fileBatch', 'filePowerShell', 'fileJs', 'fileTs', 'fileScala', 'fileDart', 'fileLua', 'fileZig', 'fileR', 'fileJulia', 'fileHaskell', 'fileClojure', 'fileElixir', 'fileErlang', 'filePerl', 'fileAsm', 'fileSolidity', 'fileNim', 'fileObjc', 'fileFsharp', 'fileOcaml', 'fileGroovy', 'fileCrystal', 'fileVlang', 'fileCoffee', 'fileElm', 'fileFortran', 'fileMatlab', 'fileLisp', 'fileVb', 'fileHaxe', 'fileDlang', 'filePascal', 'fileGleam', 'fileOdin', 'fileRescript', 'fileProlog', 'fileScss', 'fileLess', 'fileShader', 'fileHtml', 'fileCss', 'fileReact', 'fileVue', 'fileSvelte', 'fileAstro', 'fileProto', 'fileGraphql'],
    fileConfig: ['fileDocker', 'fileYaml', 'fileSettings', 'fileK8s', 'fileGradle', 'fileEnv', 'fileMakefile', 'fileTerraform', 'fileIgnore', 'fileRegistry', 'fileXml', 'fileJson', 'fileToml', 'filePlist', 'fileTsconfig', 'fileEslint', 'filePrettier', 'fileVite', 'fileWebpack', 'fileNix', 'fileJust', 'fileProcfile', 'fileGithub'],
    filePackage: ['fileJar', 'filePom', 'fileNpm', 'fileCargo', 'fileGomod', 'fileComposer', 'fileGemfile', 'filePyproject', 'fileSources', 'fileWar', 'fileEar', 'fileAar', 'fileApk', 'fileApp', 'fileExe', 'fileMsi', 'filePkg', 'fileNupkg', 'fileWhl', 'fileGem', 'fileVsix', 'fileCrx', 'filePhar'],
    fileImage: ['fileSvg', 'fileDesign'],
    fileData: ['fileSpreadsheet', 'database', 'fileCsv', 'fileGeojson', 'fileNotebook', 'fileMlmodel', 'fileParquet', 'fileAvro', 'filePickle'],
    fileSecurity: ['fileSecurity', 'fileKey', 'fileLock'],
    fileBinary: ['fileDisk', 'fileWasm'],
});

const iconAliases = Object.fromEntries(
    Object.entries(iconAliasGroups).flatMap(([target, names]) => names.map(name => [name, baseIcons[target]]))
);

/** Named SVG icon markup map used by RenopIcon and createIcon. */
export const ICONS = Object.freeze({...baseIcons, ...iconAliases});

/** Inline SVG icon custom element backed by the canonical icon map. */
export class RenopIcon extends HTMLElement {
    /** @returns {string[]} */
    static get observedAttributes() {
        return ['name', 'width', 'height', 'color'];
    }

    /** @returns {void} */
    connectedCallback() {
        this.render();
    }

    /** @returns {void} */
    attributeChangedCallback() {
        if (this.isConnected) this.render();
    }

    /**
     * Inject SVG markup and apply size/color attributes.
     * @returns {void}
     */
    render() {
        const name = this.getAttribute('name');
        this.innerHTML = ICONS[name] || '';
        const svg = this.querySelector('svg');
        if (!svg) return;
        if (this.hasAttribute('width')) svg.setAttribute('width', this.getAttribute('width'));
        if (this.hasAttribute('height')) svg.setAttribute('height', this.getAttribute('height'));
        if (this.hasAttribute('color')) svg.setAttribute('stroke', this.getAttribute('color'));
    }
}

if (!customElements.get('renop-icon')) {
    customElements.define('renop-icon', RenopIcon);
}

/**
 * Create an icon element by name.
 * @param {string} name - Key in the canonical icon map.
 * @param {object} [props={}] - Optional size, color, and class.
 * @param {string|number} [props.width] - SVG width.
 * @param {string|number} [props.height] - SVG height.
 * @param {string} [props.color] - Stroke color override.
 * @param {string} [props.class] - CSS class on the host.
 * @returns {HTMLElement}
 */
export function createIcon(name, props = {}) {
    const icon = document.createElement('renop-icon');
    icon.setAttribute('name', name);
    if (props.width) icon.setAttribute('width', props.width);
    if (props.height) icon.setAttribute('height', props.height);
    if (props.color) icon.setAttribute('color', props.color);
    if (props.class) icon.className = props.class;
    return icon;
}
