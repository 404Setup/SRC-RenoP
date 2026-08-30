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
const repositoryRoot = resolve(frontendRoot, '..', '..', '..', '..');

test('API token target editors and errors animate natural dialog height', () => {
    const source = readFileSync(join(frontendRoot, 'js/api-tokens.js'), 'utf8');
    const styles = readFileSync(join(frontendRoot, 'css/manager/profile.css'), 'utf8');
    const createModalBlock = styles.match(/\.profile-api-token-create-modal\s*\{[^}]*\}/s)?.[0] || '';

    assert.match(source, /collapseElement, expandElement, morphElementHeight/);
    assert.match(source, /expandElement\(targetEditor, \{duration: 240/);
    assert.match(source, /collapseElement\(targetEditor, \{duration: 210/);
    assert.match(source, /morphElementHeight\(container, \(\) => setInlineError/);
    assert.match(source, /data-api-token-error-key/);
    assert.match(createModalBlock, /height: auto;[\s\S]*max-height:/);
    assert.doesNotMatch(createModalBlock, /(?:^|\n)\s*height:\s*min\(/);
    assert.match(styles, /\.profile-api-token-target-editor\s*\{[^}]*overflow: hidden;/s);
});

test('server-approved API token scopes retain exact repository, package, team, and domain targets', () => {
    const backend = readFileSync(join(repositoryRoot, 'internal/service/auth/api_token.go'), 'utf8');
    const definitions = backend.match(/var apiTokenScopeDefinitions = \[\]apiTokenScopeDefinition\{([\s\S]*?)\n\}/)?.[1] || '';
    const expected = new Map([
        ['APITokenScopeRepositoryRead', 'repository'],
        ['APITokenScopeRepositoryPublish', 'repository'],
        ['APITokenScopeRepositoryDelete', 'repository'],
        ['APITokenScopePackageCreate', 'repository'],
        ['APITokenScopePackageMetadata', 'package'],
        ['APITokenScopePackageLifecycle', 'package'],
        ['APITokenScopeTeamManage', 'team'],
        ['APITokenScopeDomainRead', 'domain'],
        ['APITokenScopeDomainCreate', 'domain'],
        ['APITokenScopeDomainVerify', 'domain'],
        ['APITokenScopeDomainDelete', 'domain'],
    ]);
    for (const [scope, targetKind] of expected) {
        assert.match(definitions, new RegExp(`Scope: ${scope}, TargetKind: "${targetKind}"`), scope);
    }
    assert.doesNotMatch(definitions, /APITokenScope(?:Package|Domain)Manage/);
    assert.match(backend, /return target != "" && slices\.Contains\(restricted, target\)/);
});
