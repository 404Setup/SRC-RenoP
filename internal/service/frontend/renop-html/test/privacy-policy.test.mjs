/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {dirname, join, resolve} from 'node:path';
import test from 'node:test';
import {fileURLToPath} from 'node:url';

import {readPrivacyPolicyResponse} from '../js/privacy-policy-response.js';

const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), '..');
const repositoryRoot = resolve(frontendRoot, '..', '..', '..', '..');

test('privacy policy accepts only bounded successful UTF-8 plain text', async () => {
    const valid = new Response('Privacy policy\n', {headers: {'Content-Type': 'text/plain; charset=utf-8'}});
    assert.equal(await readPrivacyPolicyResponse(valid), 'Privacy policy\n');

    await assert.rejects(readPrivacyPolicyResponse(new Response('failure', {status: 500})));
    await assert.rejects(readPrivacyPolicyResponse(new Response('<html></html>', {
        headers: {'Content-Type': 'text/html'}
    })));
    await assert.rejects(readPrivacyPolicyResponse(new Response('small', {
        headers: {'Content-Type': 'text/plain', 'Content-Length': String((512 << 10) + 1)}
    })));
    await assert.rejects(readPrivacyPolicyResponse(new Response(new Uint8Array([0xff, 0xfe]), {
        headers: {'Content-Type': 'text/plain'}
    })));
    await assert.rejects(readPrivacyPolicyResponse(new Response(new Uint8Array((512 << 10) + 1), {
        headers: {'Content-Type': 'text/plain'}
    })));
});

test('privacy policy frontend and backend share explicit streaming boundaries', () => {
    const main = readFileSync(join(frontendRoot, 'js', 'main.js'), 'utf8');
    const frontend = readFileSync(join(frontendRoot, 'js', 'privacy-policy-response.js'), 'utf8');
    const backend = readFileSync(join(repositoryRoot, 'internal', 'api', 'routes.go'), 'utf8');
    assert.match(main, /initPrivacyPolicy\(\)/);
    assert.doesNotMatch(main, /\/api\/privacy-policy|\.text\(\)/);
    assert.match(frontend, /MAX_PRIVACY_POLICY_BYTES = 512 << 10/);
    assert.match(frontend, /response\.body\.getReader\(\)/);
    assert.match(backend, /maxPrivacyPolicyBytes = 512 << 10/);
    assert.match(backend, /io\.LimitReader\(file, maxPrivacyPolicyBytes\+1\)/);
    assert.match(backend, /utf8\.Valid\(data\)/);
});
