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
import {dirname, join, resolve} from 'node:path';
import test from 'node:test';
import {fileURLToPath} from 'node:url';

const frontendRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');

test('Maven artifact pages expose bounded project, dependency, and file metadata', () => {
    const source = readFileSync(join(frontendRoot, 'js/browser/maven.js'), 'utf8');
    for (const required of [
        'mavenProjectInformationSection(details)',
        'mavenDependencySection(project)',
        'mavenVersionFiles(version)',
        'details.file_count',
        'details.signed_file_count',
        'version.total_file_size',
        'project.dependencies_truncated',
        'version.files_truncated',
        'mavenArtifactReadmeSection',
        'openArtifactReadmeEditor',
        'setSafeMarkdown',
    ]) {
        assert.ok(source.includes(required), `Maven artifact UI is missing ${required}`);
    }
});

test('published Maven project links reject unsafe URL forms', () => {
    const source = readFileSync(join(frontendRoot, 'js/browser/maven.js'), 'utf8');
    for (const required of [
        'safeMarkdownURL(raw)',
        "rel: 'noopener noreferrer nofollow'",
    ]) {
        assert.ok(source.includes(required), `Maven project link handling is missing ${required}`);
    }
});

test('Maven metadata layouts remain responsive and bounded', () => {
    const styles = readFileSync(join(frontendRoot, 'css/browser/maven.css'), 'utf8');
    for (const selector of [
        '.maven-dependency-list',
        '.maven-dependency-row',
        '.maven-version-files',
        '.maven-version-file-row',
        '.maven-version-file-meta',
    ]) {
        assert.ok(styles.includes(selector), `Maven metadata styles are missing ${selector}`);
    }
});

test('Maven domains close permanently and released claims require administrator review', () => {
    const source = readFileSync(join(frontendRoot, 'js/browser/maven.js'), 'utf8');
    assert.match(source, /\/domains\/\$\{encodeURIComponent\(domain\.domain\)\}\/close`, \{method: 'POST'}\)/);
    assert.match(source, /\/domains\/\$\{encodeURIComponent\(domain\.domain\)\}\/claim`/);
    assert.match(source, /body: JSON\.stringify\(\{decision}\)/);
    assert.match(source, /domain\.claim_status === 'pending'/);
    assert.match(source, /Number\(domain\?\.closed_at\) > 0/);
    assert.doesNotMatch(source, /deleteDomain|deleteDomainConfirm/);
});
