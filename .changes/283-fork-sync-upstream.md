### 🔧 chore(fork): sync from upstream without clobbering fork changes

- **The problem this closes.** An earlier sync of this fork used
  `git checkout --theirs` on every conflict, which resolves each one by throwing
  away *this* side. It silently deleted the `docker-publish` job from `ci.yml` —
  the workflow still parsed and looked correct, and the only symptom was that
  nothing was ever published again. Nothing in the repo checked for it, so
  nothing caught it.
- **`scripts/sync-upstream.sh`** replaces that. It merges `upstream/main` with
  `-X ours` (non-conflicting upstream work merges in; conflicting work keeps this
  fork's side), guards the two states where a bad merge costs uncommitted work
  (dirty tree, merge in progress), leaves a `backup/pre-sync-<stamp>` branch, and
  verifies the fork's customisations are still present before declaring success.
- **Why `-X ours` and not `-X theirs` is measured, not asserted.** Against a
  simulated upstream commit with a real conflict in `ci.yml`, one in `routes.go`,
  and a non-conflicting new file: `-X theirs` lost the fork's `docker-publish`
  while `-X ours` kept it; `-X ours` dropped upstream's new job and its
  `routes.go` edit. The script prints that dropped-file list, because `-X ours`
  resolves without conflict markers and the cost is otherwise invisible.
- **The invariant set is written as assertions** (the `docker-publish` job and
  `REGISTRY_IMAGE`, the `url.PathUnescape` fix, the `combos/auto-optimal` route,
  `upsertFamilyCombos`, `onAutoOptimalClick`, `buildAutoOptimalCombos`), so a
  future sync that removes one fails loudly with the exact `git reset --hard`
  command instead of shipping quietly.
- **The compile check needed a fix to be honest.** `web/dist` is gitignored, so on
  a fresh clone `go build ./...` fails inside `web/embed.go` before it ever looks
  at the merge — a red build that says nothing about the sync. The script stands
  in a throwaway `web/dist/index.html` so the check tests the merge, then removes
  it. `SKIP_BUILD=1` opts out.
- **Documented** in `docs/FORK_SYNC.md`, including the two things not to do
  (`git pull -X theirs`, GitHub's "Sync fork" button — it merges server-side,
  where `-X ours` does not apply) and how to shrink the conflicting surface.
- **Verified:** both paths exercised end-to-end on a clone of this repository
  against a simulated upstream. Happy path — merge succeeds, all eight invariants
  hold, `docker-publish` survives, upstream's non-conflicting file arrives,
  backup branch created, dropped files listed, `go build ./...` clean. Failure
  path — with an invariant genuinely missing, exit code `1` and the correct
  `git reset --hard backup/pre-sync-…` recovery command. No upstream file is
  changed by this entry; the script and doc are new files that cannot conflict.
