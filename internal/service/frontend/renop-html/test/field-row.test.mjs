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
import test from 'node:test';
import vm from 'node:vm';

test('field rows label their own controls and preserve explicit or nested labels', () => {
    const source = readFileSync(new URL('../js/components/field-row.js', import.meta.url), 'utf8')
        .replace(/^import .*;\r?\n/gm, '').replaceAll('export ', '');
    const Row = vm.runInNewContext(source + '; RenopFieldRow', {
        HTMLElement: class {
        }, customElements: {get: () => true},
        el: (tag, props, text) => ({tag, ...props, text}), queueMicrotask: callback => callback(),
    });
    const row = new Row(), label = {
        appendChild() {
        }
    }, controls = [];
    const attributes = {label: 'Server address', hint: 'Enter a host name.'};
    Object.assign(row, {
        getAttribute: key => attributes[key], isConnected: true,
        querySelector: selector => selector === '.cfg-field-label' ? label : {querySelectorAll: () => controls}
    });
    const control = (attrs = {}, owner = row, labels = []) => {
        const node = {
            attrs, labels, closest: () => owner, hasAttribute: key => Object.hasOwn(attrs, key),
            setAttribute: (key, value) => {
                attrs[key] = value;
            }
        };
        controls.push(node);
        return node;
    };
    const unnamed = control(), explicit = control({'aria-label': 'Port'}), nested = control({}, {});
    const native = control({}, row, [{textContent: 'Enabled'}]);
    row.connectedCallback();
    assert.match(unnamed.attrs['aria-labelledby'], /^cfg-field-label-\d+$/);
    assert.equal(unnamed.attrs['aria-describedby'], unnamed.attrs['aria-labelledby'] + '-hint');
    assert.equal(explicit.attrs['aria-label'], 'Port');
    assert.equal(explicit.attrs['aria-labelledby'], undefined);
    assert.deepEqual(nested.attrs, {});
    assert.equal(native.attrs['aria-labelledby'], undefined);
    const id = unnamed.attrs['aria-labelledby'];
    attributes.label = 'Adresse du serveur';
    row.render();
    assert.equal(unnamed.attrs['aria-labelledby'], id);
});
