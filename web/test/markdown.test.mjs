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
import test from 'node:test';

import {renderMarkdown} from '../js/lib/markdown.js';

test('heading emphasis delimiters are not repeated in the table of contents', () => {
    const result = renderMarkdown('## __init__ and _value_\n');

    assert.equal(result.toc.length, 1);
    assert.equal(result.toc[0].text, 'init and value');
    assert.equal(result.toc[0].id, 'init-and-value');
    assert.match(result.html, /<strong>init<\/strong> and <em>value<\/em>/);
});

test('literal identifier underscores remain exactly once', () => {
    const result = renderMarkdown('## require_gpg_signature\n');

    assert.equal(result.toc[0].text, 'require_gpg_signature');
    assert.equal(result.toc[0].id, 'require-gpg-signature');
    assert.equal(result.html.match(/require_gpg_signature/g)?.length, 1);
});

test('linked heading labels exclude Markdown destinations', () => {
    const result = renderMarkdown('## [link_name](https://example.com/docs)\n');

    assert.equal(result.toc[0].text, 'link_name');
    assert.equal(result.toc[0].id, 'link-name');
    assert.doesNotMatch(result.toc[0].text, /https:/);
});
