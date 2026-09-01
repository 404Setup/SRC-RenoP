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

test('profile avatars use one sanitized server cache and shared identity renderer', () => {
    const backend = readFileSync(join(repositoryRoot, 'internal/service/auth/avatar.go'), 'utf8');
    const database = readFileSync(join(repositoryRoot, 'internal/database/avatar.go'), 'utf8');
    const profile = readFileSync(join(frontendRoot, 'js/profile.js'), 'utf8');
    const editor = readFileSync(join(frontendRoot, 'js/profile-avatar.js'), 'utf8');
    const identities = readFileSync(join(frontendRoot, 'js/user-profiles.js'), 'utf8');
    const avatar = readFileSync(join(frontendRoot, 'js/components/user-avatar.js'), 'utf8');

    assert.match(backend, /minAvatarDimension = 256/);
    assert.match(backend, /maxAvatarDimension = 1000/);
    assert.match(backend, /validatePNGContainer/);
    assert.match(backend, /jpeg\.Encode\(writer, decoded/);
    assert.match(backend, /png\.Encode\(writer, decoded\)/);
    assert.match(backend, /Files: 1, Bytes: avatar\.Size, Publications: 0/);
    assert.match(backend, /provider\.AvatarURL/);
    assert.match(database, /CREATE TABLE IF NOT EXISTS user_avatars/);
    assert.match(profile, /createProfileAvatarEditor\(profile/);
    assert.match(profile, /function renderPublicProfile[\s\S]*const publicAvatar = el[\s\S]*renderProfileAvatar\(publicAvatar/);
    assert.match(editor, /accept: 'image\/png,image\/jpeg,image\/webp'/);
    assert.match(editor, /\/api\/auth\/profile\/avatar\/github/);
    assert.match(identities, /export function renderProfileAvatar/);
    assert.match(identities, /element\.dataset\.avatarUrl !== avatarURL/);
    assert.match(avatar, /renderProfileAvatar\(this, this\._profile/);
});

test('avatar limit is configurable and complete in every locale', () => {
    const config = readFileSync(join(repositoryRoot, 'internal/config/server.go'), 'utf8');
    const proto = readFileSync(join(repositoryRoot, 'proto/api/v1/api.proto'), 'utf8');
    const settings = readFileSync(join(frontendRoot, 'js/settings.js'), 'utf8');
    assert.match(config, /DefaultAvatarMaxSizeBytes uint32 = 1 << 20/);
    assert.match(proto, /uint32 avatar_max_size_bytes = 17/);
    assert.match(settings, /currentConfig\.avatar_max_size_bytes = Math\.round\(n \* 1048576\)/);
    for (const locale of ['en-US', 'zh-CN', 'zh-HK', 'zh-TW', 'zh-YUE', 'ja-JP', 'ko-KR', 'de-DE', 'fr-FR', 'es-ES', 'pt-PT', 'ru-RU']) {
        const profile = readFileSync(join(frontendRoot, 'js/i18n', locale, 'profile.js'), 'utf8');
        const localeSettings = readFileSync(join(frontendRoot, 'js/i18n', locale, 'settings.js'), 'utf8');
        assert.match(profile, /"profile\.avatarRequirements"/);
        assert.match(localeSettings, /"settings\.avatarMaxSize"/);
    }
});
