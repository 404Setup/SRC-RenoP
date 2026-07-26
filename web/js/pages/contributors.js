/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import { t } from '../i18n.js';
import { el, clear } from '../lib/dom.js';
import { fetchContributors, GITHUB_REPO } from '../lib/github.js';

/**
 * Render the contributors page: load GitHub contributors and show a card grid.
 * @param {{ root: HTMLElement }} ctx - Route context.
 * @returns {Promise<() => void>} Cleanup that cancels in-flight UI updates when navigating away.
 */
export async function renderContributors({ root }) {
    root.innerHTML = '';
    document.title = `RenoP — ${t('contributors.title')}`;

    root.appendChild(
        el('header', { class: 'page-hero' },
            el('h1', {}, t('contributors.title')),
            el('p', {}, t('contributors.lead')),
        ),
    );

    const meta = el('p', { class: 'contributors-meta muted' },
        t('contributors.source', { repo: GITHUB_REPO }),
    );
    root.appendChild(meta);

    const grid = el('div', { class: 'contributors-grid' },
        el('p', { class: 'contributors-loading' }, t('contributors.loading')),
    );
    root.appendChild(grid);

    let cancelled = false;

    try {
        const list = await fetchContributors();
        if (cancelled) return () => { cancelled = true; };

        clear(grid);
        if (!list.length) {
            grid.appendChild(el('p', { class: 'contributors-empty' }, t('contributors.empty')));
        } else {
            for (const c of list) {
                const card = el('a', {
                    class: 'card contributor-card',
                    href: c.htmlUrl,
                    target: '_blank',
                    rel: 'noopener noreferrer',
                    title: c.login,
                },
                    el('img', {
                        class: 'contributor-avatar',
                        src: c.avatarUrl,
                        alt: '',
                        loading: 'lazy',
                        width: '64',
                        height: '64',
                    }),
                    el('div', { class: 'contributor-info' },
                        el('span', { class: 'contributor-login' }, c.login),
                        el('span', { class: 'contributor-commits muted' },
                            t('contributors.commits', { count: c.contributions }),
                        ),
                    ),
                );
                grid.appendChild(card);
            }
        }

        const note = el('p', { class: 'contributors-note muted' }, t('contributors.note'));
        root.appendChild(note);
    } catch (err) {
        if (cancelled) return () => { cancelled = true; };
        console.error(err);
        clear(grid);
        grid.appendChild(el('p', { class: 'contributors-error' }, t('contributors.loadError')));
        grid.appendChild(
            el('p', { class: 'contributors-note' },
                el('a', {
                    href: `https://github.com/${GITHUB_REPO}/graphs/contributors`,
                    target: '_blank',
                    rel: 'noopener noreferrer',
                }, t('contributors.viewOnGithub')),
            ),
        );
    }

    return () => {
        cancelled = true;
    };
}
