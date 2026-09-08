/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import test from 'node:test';
import vm from 'node:vm';

const source = readFileSync(new URL('../js/settings/mail.js', import.meta.url), 'utf8');
const declaration = source.match(/^export function applyMailPreset\([\s\S]*?^}/m)?.[0];
assert.ok(declaration);
const applyPreset = vm.runInNewContext(declaration.replace('export ', '') + '; applyMailPreset', {structuredClone});

test('email provider presets preserve account policy and do not share mutable pricing', () => {
    const quota = {limit: 40, period: 'week'};
    const account = {id: 'primary', name: 'Main', enabled: true, provider: 'smtp', from: 'sender@example.com', scenes: ['test'], password: 'old-password', quota, overage: {limit: 2, period: 'month'}, force_send: true, balance_micros: 5000000, pricing: {currency: 'USD'}};
    const preset = {id: 'api', account: {provider: 'sendgrid', endpoint: 'https://api.sendgrid.com/v3', quota: {limit: 0, period: 'month'}, pricing: {currency: 'USD', tiers: [{up_to: 0, amount_micros: 100, batch_size: 1}]}}};
    applyPreset(account, preset);
    assert.equal(account.id, 'primary');
    assert.equal(account.quota, quota);
    assert.equal(account.overage.limit, 2);
    assert.equal(account.force_send, true);
    assert.equal(account.balance_micros, 5000000);
    assert.deepEqual(account.scenes, ['test']);
    assert.equal(account.password, undefined);
    account.pricing.tiers[0].amount_micros = 99;
    assert.equal(preset.account.pricing.tiers[0].amount_micros, 100);
    preset.account.pricing.currency = 'CNY';
    applyPreset(account, preset);
    assert.equal(account.balance_micros, null);
});
