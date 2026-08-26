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

const frontendRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');

test('repository entrance state is prepared before replacement content is painted', () => {
    const source = readFileSync(join(frontendRoot, 'js/browser/repository-view.js'), 'utf8');
    const prepareIndex = source.indexOf("container.classList.add('is-entering')");
    const morphIndex = source.indexOf('await morphElementHeight(container, mutate');
    assert.ok(prepareIndex >= 0, 'repository view does not prepare its entrance state');
    assert.ok(morphIndex > prepareIndex, 'repository view applies its entrance state after content was painted');
    assert.ok(source.includes("container.classList.remove('is-entering')"),
        'repository view does not reset an interrupted entrance');
});

test('Maven domain filters preserve the shell and animate only bounded results', () => {
    const source = readFileSync(join(frontendRoot, 'js/browser/maven.js'), 'utf8');
    const styles = readFileSync(join(frontendRoot, 'css/browser/maven.css'), 'utf8');
    for (const required of [
        "container.querySelector(':scope > .maven-domain-results')",
        'domainCenterResultNodes(container, domains, total)',
        'existingResults.dataset.language === activeLanguage',
        'replaceRepositoryView(existingResults',
    ]) {
        assert.ok(source.includes(required), `Maven domain transition is missing ${required}`);
    }
    assert.ok(styles.includes('.maven-domain-results.is-entering > *'),
        'Maven domain results have no entrance animation');
    assert.ok(styles.includes('.maven-domain-results.is-updating > *'),
        'Maven domain results have no scoped loading state');
});

test('Maven repository domain and artifact routes opt into shared entrances', () => {
    const source = readFileSync(join(frontendRoot, 'js/browser/maven.js'), 'utf8');
    assert.match(source, /replaceRepositoryView\(container, sections, \{duration: 280, enterDuration: 420\}\)/,
        'Maven domain route does not animate');
    assert.match(source, /maven\.versionsTitle[\s\S]{0,180}\{duration: 280, enterDuration: 420\}/,
        'Maven artifact route does not animate');
});
