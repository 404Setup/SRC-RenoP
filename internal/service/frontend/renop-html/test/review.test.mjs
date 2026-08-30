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

const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), '..');
const repositoryRoot = resolve(frontendRoot, '..', '..', '..', '..');

/**
 * Read one embedded frontend source file.
 * @param {...string} parts - Path below the frontend root.
 * @returns {string} UTF-8 source.
 */
function frontendSource(...parts) {
    return readFileSync(join(frontendRoot, ...parts), 'utf8');
}

/**
 * Read one repository source file.
 * @param {...string} parts - Path below the repository root.
 * @returns {string} UTF-8 source.
 */
function repositorySource(...parts) {
    return readFileSync(join(repositoryRoot, ...parts), 'utf8');
}

test('ownership transfers use one routed review center outside the message system', () => {
    const html = frontendSource('index.html');
    const main = frontendSource('js', 'main.js');
    const reviews = frontendSource('js', 'reviews.js');
    assert.match(html, /data-account-action="reviews"/);
    assert.match(html, /id="tab-content-reviews"/);
    assert.match(main, /reviewRouteFromPath/);
    assert.match(main, /loadReviewCenterPage/);
    assert.match(reviews, /routeRoot = '\/account\/reviews'/);
    assert.match(reviews, /morphElementHeight/);
    assert.match(reviews, /ensureReviewShell\(refreshToolbar\)/);
    assert.match(reviews, /replaceContent\(loadingState\(\)\)/);
    assert.match(reviews, /nodes\.filter\(node => node !== null/);
    assert.match(reviews, /requestReview[\s\S]*?logoutOnForbidden: false/);
    assert.match(reviews, /view: activeView/);
    assert.match(reviews, /types', \[\.\.\.activeTypes\]\.join\(','\)/);
    assert.doesNotMatch(reviews, /messages\.js|registerMessage|notification/);
});

test('all package engines and Maven domains share the transfer request dialog', () => {
    const reviews = frontendSource('js', 'reviews.js');
    for (const file of ['docker.js', 'npm.js', 'cargo.js', 'maven.js']) {
        const source = frontendSource('js', 'browser', file);
        assert.match(source, /openSuperTeamTransferDialog/);
        assert.match(source, /review\.transferOwnership/);
    }
    for (const type of ['docker_image', 'npm_package', 'cargo_package', 'maven_artifact', 'maven_domain']) {
        assert.ok(reviews.includes(`'${type}'`), `missing frontend transfer type ${type}`);
    }
    assert.match(reviews, /minimumRole: 1, includePersonal: false/);
    assert.match(reviews, /resourceType === 'docker_image'[\s\S]*?includes\('\/'\)/);
    assert.match(reviews, /resourceType === 'npm_package'[\s\S]*?startsWith\('@'\)/);
});

test('review decisions are durable, administrator-aware, and compare-and-set', () => {
    const schema = repositorySource('internal', 'database', 'review_schema.go');
    const clickhouse = repositorySource('internal', 'database', 'clickhouse_schema.go');
    const database = repositorySource('internal', 'database', 'review.go');
    const routes = repositorySource('internal', 'service', 'review', 'routes.go');
    assert.match(schema, /CREATE TABLE IF NOT EXISTS review_tasks/);
    assert.match(schema, /CREATE TABLE IF NOT EXISTS review_task_files/);
    assert.match(schema, /CREATE TABLE IF NOT EXISTS review_task_payloads/);
    assert.match(schema, /UNIQUE \(active_key\)/);
    assert.match(clickhouse, /name: "review_tasks"/);
    assert.doesNotMatch(database, /task\.RequestedByID == actorID/);
    assert.match(database, /options\.Administrator/);
    assert.match(database, /reviewer\.IsManager\(\)/);
    assert.match(database, /reviewer\.CheckModeratePermission\(task\.Repository\)/);
    assert.match(database, /requireSuperTeamRoleTx\([\s\S]*?SuperTeamRoleManage/);
    assert.match(database, /WHERE id = \? AND status = \?/);
    assert.match(routes, /CurrentCredentialKind\(c\) != "session"/);
    assert.doesNotMatch(routes, /service\/message|CreateMessage|SendMessage/);
});

test('publication reviews use bounded parallel downloads and preset rejection reasons', () => {
    const reviews = frontendSource('js', 'reviews.js');
    const chunked = frontendSource('js', 'chunked-upload.js');
    const upload = frontendSource('js', 'browser', 'upload.js');
    const storage = repositorySource('internal', 'service', 'storage', 'publication_review.go');
    assert.match(reviews, /await import\('fflate'\)/);
    assert.match(reviews, /Math\.min\(fileCount, slow \? 1 : hardware >= 4 \? 4 : 2\)/);
    assert.match(reviews, /attempt < 3/);
    assert.match(reviews, /triggerCriticalReviewDownloads/);
    assert.match(reviews, /reason_code: reasonCode/);
    assert.match(storage, /RestorePublicationReviewState/);
    assert.match(storage, /ServePublicationReviewFile/);
    const npmReview = repositorySource('internal', 'service', 'npm', 'publication_review.go');
    assert.match(npmReview, /RollbackNPMPublicationReview/);
    const cargoReview = repositorySource('internal', 'service', 'cargo', 'publication_review.go');
    const cargoBrowser = frontendSource('js', 'browser', 'cargo.js');
    assert.match(cargoReview, /RollbackCargoPublicationReview/);
    assert.match(cargoReview, /rollbackCargoIndexVersion/);
    assert.match(cargoBrowser, /version\.review_status === 'pending'/);
    assert.match(cargoBrowser, /cargo\.reviewPending/);
    const dockerReview = repositorySource('internal', 'service', 'docker', 'publication_review.go');
    const dockerBrowser = frontendSource('js', 'browser', 'docker.js');
    assert.match(dockerReview, /ApproveDockerPublicationReview/);
    assert.match(dockerReview, /ServePublicationReviewManifest/);
    assert.match(dockerBrowser, /tag\.review_status !== 'pending'/);
    assert.match(dockerBrowser, /docker\.reviewPending/);
    assert.match(chunked, /X-RenoP-Review-ID/);
    assert.match(upload, /browser\.uploadQueuedReview/);
});

test('review layouts remain bounded on narrow viewports', () => {
    const styles = frontendSource('css', 'manager', 'reviews.css');
    assert.match(styles, /\.review-card\s*\{[\s\S]*?grid-template-columns:\s*auto minmax\(0, 1fr\) auto/);
    assert.match(styles, /@media \(max-width: 680px\)/);
    assert.match(styles, /\.review-card\s*\{[\s\S]*?grid-template-columns:\s*minmax\(0, 1fr\)/);
    assert.match(styles, /\.review-toolbar-selects \.custom-select-wrapper/);
    assert.match(styles, /\.review-transfer-resource[\s\S]*?min-width:\s*0/);
    assert.match(styles, /\.review-reject-field\s*\{[\s\S]*?gap:\s*0\.6rem/);
    assert.match(styles, /@media \(prefers-reduced-motion: reduce\)/);
});

test('reviewed package creation uses the application router and keeps Docker actions separated', () => {
    const docker = frontendSource('js', 'browser', 'docker.js');
    const npm = frontendSource('js', 'browser', 'npm.js');
    const reviews = frontendSource('js', 'reviews.js');
    const creation = repositorySource('internal', 'database', 'review_creation.go');
    const styles = frontendSource('css', 'browser', 'docker.css');
    assert.match(docker, /openReviewCenter\('requested'\)/);
    assert.match(npm, /openReviewCenter\('requested'\)/);
    assert.doesNotMatch(docker, /activeNavigate\?\.\('\/account\/reviews'\)/);
    assert.doesNotMatch(npm, /activeNavigate\?\.\('\/account\/reviews'\)/);
    assert.match(reviews, /task\.resource_version === '@create'/);
    assert.match(reviews, /review\.packageCreation/);
    assert.match(creation, /createDockerImageTx/);
    assert.match(creation, /createNPMPackageTx/);
    assert.match(creation, /WHERE id = \? AND status = \?/);
    assert.match(styles, /\.docker-page-actions\s*\{[\s\S]*?gap:\s*0\.65rem/);
});
