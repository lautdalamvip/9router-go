# Keeping this fork in sync with upstream

Upstream (`luqman-v1/9router-go`) is the source of truth for behaviour, and this
fork carries its own changes on top (the Auto Optimal combo builder, the
escaped-route-param fix, Docker publishing to `lautdalam/9router-go`). Syncing has
to bring upstream in **without removing any of that**.

## The one rule

```bash
bash scripts/sync-upstream.sh
```

That merges `upstream/main` with `-X ours`: everything that does not conflict is
merged in, and anything that *does* conflict keeps this fork's side.

Two things never to do:

- **`git pull -X theirs`** (and `git checkout --theirs`) resolves every conflict
  by discarding *your* side. This is how an earlier sync silently deleted the
  `docker-publish` job — the endpoint existed, the workflow looked fine, and the
  only symptom was that nothing was ever published again.
- **GitHub's "Sync fork" button.** It merges server-side, where `.gitattributes`
  and `-X ours` do not apply, so it fails or clobbers the same way.

## Why `-X ours` and not `-X theirs`

Measured on this repository's real history, with a simulated upstream commit that
appended a job to `ci.yml`, edited the `getURLParam` function in `routes.go`, and
added one new file:

| after the sync | `-X theirs` | `-X ours` |
| :--- | :--- | :--- |
| fork's `docker-publish` job | **LOST** | kept |
| fork's `REGISTRY_IMAGE: lautdalam/9router-go` | kept | kept |
| fork's `url.PathUnescape` fix | kept | kept |
| fork's `combos/auto-optimal` route | kept | kept |
| upstream's new job in `ci.yml` | arrived | **dropped** |
| upstream's edit to `routes.go` | arrived | **dropped** |
| upstream's new file (no conflict) | arrived | arrived |

Neither column is free. `-X theirs` protects upstream's work and deletes yours;
`-X ours` does the reverse, which is what a fork needs. The **dropped** rows are
the price: where upstream edited the same lines you did, upstream's version is
discarded, and it stays discarded until someone re-applies it by hand.

That is why the script *prints* those files at the end. Read that list — it is the
whole trade-off, and it is invisible otherwise, because `-X ours` leaves no
conflict markers.

## What the script does

1. Refuses to start on a dirty working tree or with a merge already in progress —
   those are the two states where a bad merge can cost you uncommitted work.
2. Fetches `upstream` and exits early if there is nothing new.
3. Creates `backup/pre-sync-<timestamp>` at the current commit.
4. Merges `upstream/main` with `-X ours`.
5. **Verifies the fork's customisations survived** (`grep` assertions below) and
   exits non-zero with the exact `git reset --hard <backup>` command if any is
   missing. `-X ours` resolves silently, so without this check a wrong resolution
   is invisible.
6. Reports which files upstream also changed where this side won — the cost,
   listed with the `git diff` command to review each one.
7. Compiles (`go build ./...`). A merge can be textually clean and semantically
   broken: upstream renames a symbol, your call site survives, and nothing
   compiles. `web/dist` is gitignored, so on a clean checkout the script stands in
   a throwaway `web/dist/index.html` to get past `web/embed.go`, then removes it.
8. Stops. It does not push — review first.

Set `SKIP_BUILD=1` to skip step 7. `UPSTREAM_REMOTE`, `UPSTREAM_BRANCH` and
`LOCAL_BRANCH` override the defaults (`upstream`, `main`, current branch).

## What is protected

If a sync removes any of these, the script fails:

| File | Must contain |
| :--- | :--- |
| `.github/workflows/ci.yml` | `REGISTRY_IMAGE: lautdalam/9router-go`, `docker-publish` |
| `.github/workflows/release.yml` | `REGISTRY_IMAGE: lautdalam/9router-go` |
| `scripts/notify-telegram.sh` | `lautdalam/9router-go` |
| `internal/handlers/dashboard/combos_optimal.go` | `autoOptimalComboName` |
| `internal/handlers/dashboard/routes.go` | `combos/auto-optimal`, `url.PathUnescape` |
| `internal/handlers/dashboard/combos_family.go` | `upsertFamilyCombos` |
| `web/src/components/combos/CombosView.svelte` | `onAutoOptimalClick` |
| `web/src/api/client.ts` | `buildAutoOptimalCombos` |

When you add a fork-specific feature, add its marker here — an assertion is the
only thing that turns a silent divergence into a failed sync.

## If a sync goes wrong

Nothing is unrecoverable: each run leaves a backup branch.

```bash
git branch --list 'backup/pre-sync-*'      # what to go back to
git reset --hard backup/pre-sync-<stamp>   # undo the merge entirely
git diff upstream/main -- <file>           # what upstream wanted in a file you won
```

Delete old backups when you no longer need them: `git branch -D backup/pre-sync-<stamp>`.

## Reducing what a sync can clobber

`-X ours` protects your work, but every upstream improvement to lines you have
also edited is dropped. The way to shrink that set is to keep your changes in
places upstream does not touch:

- New behaviour in **new files** merges cleanly forever (nothing to conflict
  with) — upstream's edits to the files it owns then arrive untouched.
- Fork config that must *replace* upstream config (e.g. `REGISTRY_IMAGE`) has to
  live in upstream's file and will keep conflicting. Keep such edits to as few
  lines as possible, and read the dropped-file list when it appears.
