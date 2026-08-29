/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the terms of the Mozilla Public License, v. 2.0.
 */

import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {dirname, join} from 'node:path';
import test from 'node:test';
import {fileURLToPath} from 'node:url';

const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), '..');
const variables = readFileSync(join(frontendRoot, 'css', 'variables.css'), 'utf8');
const navigation = readFileSync(join(frontendRoot, 'css', 'layout', 'navigation.css'), 'utf8');
const structure = readFileSync(join(frontendRoot, 'css', 'layout', 'structure.css'), 'utf8');

test('navigation and routed content use the same desktop shell geometry', () => {
    assert.match(variables, /--app-shell-max-width:\s*1264px/);
    assert.match(variables, /--app-shell-gutter:\s*2rem/);
    assert.match(navigation, /\.top-nav\s*\{[\s\S]*?max-width:\s*var\(--app-shell-max-width\)[\s\S]*?box-sizing:\s*border-box/);
    assert.match(navigation, /\.top-nav\s*\{[\s\S]*?padding:\s*1\.75rem var\(--app-shell-gutter\) 0\.75rem/);
    assert.match(structure, /#app\s*\{[\s\S]*?max-width:\s*var\(--app-shell-max-width\)[\s\S]*?padding:\s*0 var\(--app-shell-gutter\) 2rem[\s\S]*?box-sizing:\s*border-box/);
});
