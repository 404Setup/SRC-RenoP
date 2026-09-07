# Update publishing

`build.yml` builds platform packages, validates the payload with `test-release-payload.ps1`, and invokes
`publish-update.ps1`. Only raw Brotli executables and `manifest.json` belong to
the build artifact; release documents are attached separately to GitHub releases.

The workflow uses one repository-wide `renop-actions` concurrency group with `queue: max` and
`cancel-in-progress: false`. Only one workflow run executes at a time across all branches and release channels;
other runs wait in FIFO order based on when they entered the concurrency queue. Within a run, `needs` orders
metadata, build, publish, and release jobs. GitHub supports at most 100 pending runs per group; further runs
are canceled when the queue is full. Queue arrival order can differ from dispatch or commit order. See
[GitHub's concurrency contract](https://docs.github.com/en/actions/how-tos/write-workflows/choose-when-workflows-run/control-workflow-concurrency).

For nightly builds, `nightly-info.ps1` first removes every historical `targets` field and adds the current build's
fresh target metadata. It sorts known releases by Git topology, newest first, then inserts missing eligible
commits at their positions. Git history is authoritative: at most 100 eligible commits reachable from the
publishing commit are listed. Unreachable or unresolvable records are discarded. Existing release dates and
notes are preserved; backfilled entries use the commit date and subject and have no download targets.

Nightly backfill excludes `[web]` and `[release]` entries and GitHub skip markers: `[skip ci]`, `[ci skip]`,
`[no ci]`, `[skip actions]`, `[actions skip]`, and `skip-checks: true`. Matching is case insensitive. See
[GitHub's skip instructions](https://docs.github.com/en/actions/how-tos/manage-workflow-runs/skip-workflow-runs)
and [Git's topology ordering](https://git-scm.com/docs/git-log#Documentation/git-log.txt---topo-order).

A full checkout and PowerShell 7.5 or later are required. Remote JSON dates remain strings when decoded.
Git history failures stop publication. A build whose commit is an ancestor of
an already published nightly is rejected, so a delayed job cannot replace a newer download. Historical
package directories for the first nine entries may remain on the update host, but only the current build
advertises targets. Existing bounded cleanup removes at most five obsolete directories per publication.
Stable release metadata and its current/previous package retention are unchanged.

Run `pwsh -NoProfile -File .github/scripts/test-nightly-info.ps1` to test ordering, backfill, retention, legacy
metadata, target replacement, repeatability, and stale-build rejection against a disposable Git repository.
The same check runs through `pnpm run test:web`.
