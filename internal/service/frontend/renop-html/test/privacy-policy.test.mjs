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

import {readLegalTextResponse} from '../js/legal-response.js';

const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), '..');
const repositoryRoot = resolve(frontendRoot, '..', '..', '..', '..');

test('privacy policy accepts only bounded successful UTF-8 plain text', async () => {
    const valid = new Response('Privacy policy\n', {headers: {'Content-Type': 'text/plain; charset=utf-8'}});
    assert.equal(await readLegalTextResponse(valid), 'Privacy policy\n');

    await assert.rejects(readLegalTextResponse(new Response('failure', {status: 500})));
    await assert.rejects(readLegalTextResponse(new Response('<html></html>', {
        headers: {'Content-Type': 'text/html'}
    })));
    await assert.rejects(readLegalTextResponse(new Response('small', {
        headers: {'Content-Type': 'text/plain', 'Content-Length': String((512 << 10) + 1)}
    })));
    await assert.rejects(readLegalTextResponse(new Response(new Uint8Array([0xff, 0xfe]), {
        headers: {'Content-Type': 'text/plain'}
    })));
    await assert.rejects(readLegalTextResponse(new Response(new Uint8Array((512 << 10) + 1), {
        headers: {'Content-Type': 'text/plain'}
    })));
});

test('legal documents use independent pages and bounded safe Markdown', () => {
    const main = readFileSync(join(frontendRoot, 'js', 'main.js'), 'utf8');
    const frontend = readFileSync(join(frontendRoot, 'js', 'legal-response.js'), 'utf8');
    const pages = readFileSync(join(frontendRoot, 'js', 'legal-pages.js'), 'utf8');
    const backend = readFileSync(join(repositoryRoot, 'internal', 'config', 'legal.go'), 'utf8');
    assert.match(main, /initializeLegalPages\(\)/);
    assert.doesNotMatch(main, /initPrivacyPolicy|privacy-policy-modal/);
    assert.match(frontend, /MAX_LEGAL_DOCUMENT_BYTES = 512 << 10/);
    assert.match(frontend, /response\.body\.getReader\(\)/);
    assert.match(backend, /MaxLegalDocumentBytes = 512 << 10/);
    assert.match(pages, /setSafeMarkdown\(body, content\)/);
});
