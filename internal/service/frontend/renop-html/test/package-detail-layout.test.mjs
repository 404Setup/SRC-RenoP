/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {dirname, join} from 'node:path';
import test from 'node:test';
import {fileURLToPath} from 'node:url';

const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), '..');

/**
 * Read one embedded frontend source file.
 * @param {...string} parts - Path below the frontend root.
 * @returns {string} UTF-8 source.
 */
function source(...parts) {
    return readFileSync(join(frontendRoot, ...parts), 'utf8');
}

test('all package details use one accessible sectioned layout instead of a long page', () => {
    const tabs = source('js', 'browser', 'package-detail-tabs.js');
    assert.match(tabs, /role: 'tablist'/);
    assert.match(tabs, /role: 'tabpanel'/);
    assert.match(tabs, /ArrowRight/);
    assert.match(tabs, /morphElementHeight/);
    assert.match(tabs, /panel\.replaceChildren\(\.\.\.tab\.content\)/);
    for (const format of ['npm', 'cargo', 'maven', 'docker']) {
        const script = source('js', 'browser', `${format}.js`);
        assert.match(script, /createPackageDetailTabs/);
    }
    const styles = source('css', 'browser', 'package-detail-tabs.css');
    assert.match(styles, /\.package-detail-panel\s*\{[^}]*grid-template-columns: repeat\(2,/s);
    assert.match(styles, /> :only-child,[\s\S]*?grid-column: 1 \/ -1/);
    assert.doesNotMatch(styles, /min-height:\s*(?:[1-9]\d*)/);
});

test('empty optional cards are omitted while actionable collections remain', () => {
    const npm = source('js', 'browser', 'npm.js');
    const cargo = source('js', 'browser', 'cargo.js');
    const docker = source('js', 'browser', 'docker.js');
    assert.match(npm, /if \(tags\.length === 0\) return null/);
    assert.match(npm, /if \(!project\.readme\) return null/);
    assert.match(cargo, /if \(!readme\) return null/);
    assert.match(docker, /String\(image\.description \|\| ''\)\.trim\(\) \|\| canManageL2/);
});

test('Maven exposes copy-ready dependency declarations', () => {
    const maven = source('js', 'browser', 'maven.js');
    assert.match(maven, /function mavenImportSection/);
    assert.match(maven, /<dependency>\\n  <groupId>/);
    assert.match(maven, /Gradle Kotlin DSL/);
    assert.match(maven, /Gradle Groovy DSL/);
    assert.match(maven, /copyText\(copy, current\.value\)/);
});

test('Docker digest feedback preserves the stable SHA pill', () => {
    const helper = source('js', 'browser', 'copy-feedback.js');
    const docker = source('js', 'browser', 'docker.js');
    const styles = source('css', 'browser', 'docker.css');
    assert.match(helper, /preserveContent = false/);
    assert.match(helper, /if \(preserveContent\) button\.appendChild\(toast\)/);
    assert.match(docker, /preserveContent: element\.classList\.contains\('docker-tag-digest'\)/);
    assert.match(styles, /\.docker-tag-digest\.copied/);
});

test('all package details share one irreversible deprecation control', () => {
    const helper = source('js', 'package-deprecation.js');
    const browserStyles = source('css', 'browser.css');
    assert.match(helper, /window\.showConfirm\(t\('package\.deprecateConfirm'\), \{danger: true}\)/);
    assert.match(helper, /responseErrorMessage/);
    assert.match(helper, /createPackageDeprecationNotice/);
    assert.match(browserStyles, /package-deprecation\.css/);
    for (const format of ['npm', 'cargo', 'maven', 'docker']) {
        const script = source('js', 'browser', `${format}.js`);
        assert.match(script, /from '\.\.\/package-deprecation\.js'/, format);
        assert.match(script, /createDeprecatePackageButton/, format);
        assert.match(script, /createPackageDeprecationBadge/, format);
        assert.match(script, /createPackageDeprecationNotice/, format);
        assert.match(script, /deprecated/, format);
    }
});
