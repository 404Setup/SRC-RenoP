/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

import assert from 'node:assert/strict';
import {readdirSync, readFileSync} from 'node:fs';
import {dirname, join, resolve} from 'node:path';
import test from 'node:test';
import {fileURLToPath, pathToFileURL} from 'node:url';

const frontendRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const repositoryRoot = resolve(frontendRoot, '..', '..', '..', '..');

test('package version and tag collections use one responsive non-numbered pager', () => {
    const uiPackage = JSON.parse(readFileSync(join(repositoryRoot, 'packages/renop-ui/package.json'), 'utf8'));
    const pagination = readFileSync(join(repositoryRoot, 'packages/renop-ui/js/pagination.js'), 'utf8');
    const styles = readFileSync(join(repositoryRoot, 'packages/renop-ui/css/components/pagination.css'), 'utf8');
    const frontendStyles = readFileSync(join(frontendRoot, 'css/components.css'), 'utf8');
    const maven = readFileSync(join(frontendRoot, 'js/browser/maven.js'), 'utf8');
    const npm = readFileSync(join(frontendRoot, 'js/browser/npm.js'), 'utf8');
    const docker = readFileSync(join(frontendRoot, 'js/browser/docker.js'), 'utf8');

    assert.equal(uiPackage.exports['./pagination'], './js/pagination.js');
    assert.equal(uiPackage.exports['./css/components/pagination.css'], './css/components/pagination.css');
    assert.match(frontendStyles, /@renop\/ui\/css\/components\/pagination\.css/);
    assert.match(pagination, /clampCollectionPage/);
    assert.match(pagination, /previousLabel/);
    assert.match(pagination, /nextLabel/);
    assert.doesNotMatch(pagination, /for \(let page|Array\.from\(\{length: pages/);
    assert.match(styles, /@media \(max-width: 520px\)/);
    assert.match(styles, /@media \(prefers-reduced-motion: reduce\)/);

    assert.match(maven, /pageSize: 8/);
    assert.match(maven, /renderItem: version => mavenVersionEntry/);
    assert.match(npm, /const versionPageSize = 8/);
    assert.match(npm, /initialPage: versionPage/);
    assert.match(docker, /const dockerTagPageSize = 10/);
    assert.match(docker, /initialPage: dockerTagPage/);
});

test('pagination summaries remain complete in every locale', async () => {
    const localeRoot = join(frontendRoot, 'js/i18n');
    for (const locale of readdirSync(localeRoot, {withFileTypes: true}).filter(entry => entry.isDirectory())) {
        const common = await import(pathToFileURL(join(localeRoot, locale.name, 'common.js')).href);
        const value = common.default['common.pagination'];
        assert.equal(typeof value, 'string', locale.name);
        for (const placeholder of ['{page}', '{pages}', '{total}']) {
            assert.ok(value.includes(placeholder), `${locale.name} ${placeholder}`);
        }
    }
});

test('npm catalog repairs an empty latest tag when published versions exist', () => {
    const database = readFileSync(join(repositoryRoot, 'internal/database/npm.go'), 'utf8');
    const tests = readFileSync(join(repositoryRoot, 'internal/database/npm_test.go'), 'utf8');
    assert.match(database, /fillMissingNPMLatestVersions/);
    assert.match(database, /ROW_NUMBER\(\) OVER \(/);
    assert.match(database, /if result\.VersionCount > 0 && result\.LatestVersion == ""/);
    assert.match(database, /latest, err = latestNPMVersionTx\(tx, repository, packageName\)/);
    assert.match(tests, /TestNPMPackageLatestVersionFallsBackWithoutDistTag/);
});
