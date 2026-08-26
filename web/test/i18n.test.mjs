/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

import test from 'node:test';
import assert from 'node:assert/strict';

import enUS from '../js/i18n/en-US.js';
import frFR from '../js/i18n/fr-FR.js';
import jaJP from '../js/i18n/ja-JP.js';
import ruRU from '../js/i18n/ru-RU.js';
import zhCN from '../js/i18n/zh-CN.js';

const locales = {enUS, frFR, jaJP, ruRU, zhCN};

/**
 * Return the sorted interpolation fields used by one translation.
 * @param {string} value
 * @returns {string[]}
 */
function placeholders(value) {
    return [...String(value).matchAll(/\{([A-Za-z0-9_]+)\}/g)].map(match => match[1]).sort();
}

test('website locales match English keys and interpolation fields', () => {
    const expectedKeys = Object.keys(enUS).sort();
    for (const [locale, messages] of Object.entries(locales)) {
        assert.deepEqual(Object.keys(messages).sort(), expectedKeys, `${locale} key set`);
        for (const key of expectedKeys) {
            assert.deepEqual(placeholders(messages[key]), placeholders(enUS[key]), `${locale}:${key} placeholders`);
        }
    }
});
