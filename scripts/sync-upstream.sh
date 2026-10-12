#!/usr/bin/env bash
#
# Sync this fork with the upstream project without letting the merge overwrite
# the fork's own customisations.
#
# Usage:
#   bash scripts/sync-upstream.sh
#
# Env overrides: UPSTREAM_REMOTE (upstream), UPSTREAM_BRANCH (main),
#                LOCAL_BRANCH (current branch), SKIP_BUILD (0)
#
# ---------------------------------------------------------------------------
# Why `-X ours`, and never `-X theirs`
#
# A plain `git pull` stops on every conflict and leaves you resolving them by
# hand; `git pull -X theirs` resolves them by discarding YOUR side, which is how
# this fork lost its docker-publish job and its Cloudflare routes during an
# earlier sync. `-X ours` resolves a conflict by keeping your side and still
# merges everything that did not conflict.
#
# Measured against a simulated upstream change, on this repository's real
# history, with a conflict in ci.yml, in routes.go and a non-conflicting new
# file:
#
#                                      -X theirs      -X ours
#   fork's docker-publish job          LOST           kept
#   fork's REGISTRY_IMAGE              kept           kept
#   fork's url.PathUnescape fix        kept           kept
#   upstream's new job                 arrived        DROPPED
#   upstream's edit to routes.go       arrived        DROPPED
#   upstream's new file (no conflict)  arrived        arrived
#
# So `-X ours` delivers exactly "sync in, never change mine". The cost is the
# two DROPPED rows: where upstream edited the same lines you did, upstream's
# version is discarded. The script prints those files at the end so the cost is
# visible instead of silent -- read that list, it is the whole trade-off.
#
# Do NOT use GitHub's "Sync fork" button for this repo: it merges server-side,
# which ignores .gitattributes and gives no `-X ours`, so it will conflict or
# clobber the same way `git pull` does.
# ---------------------------------------------------------------------------

set -euo pipefail

UPSTREAM_REMOTE="${UPSTREAM_REMOTE:-upstream}"
UPSTREAM_BRANCH="${UPSTREAM_BRANCH:-main}"
LOCAL_BRANCH="${LOCAL_BRANCH:-$(git rev-parse --abbrev-ref HEAD)}"
SKIP_BUILD="${SKIP_BUILD:-0}"

die() { printf '\nsync-upstream: %s\n' "$1" >&2; exit 1; }
note() { printf '%s\n' "$1"; }

# Fail before touching anything: a dirty tree makes `git reset --hard` the only
# way back out of a bad merge, and that is how uncommitted work gets lost.
[ -z "$(git status --porcelain --untracked-files=no)" ] || die "working tree is dirty — commit or stash first."
if git rev-parse -q --verify MERGE_HEAD >/dev/null; then
  die "a merge is already in progress — finish it or 'git merge --abort' first."
fi

note "fetching $UPSTREAM_REMOTE ..."
git fetch --tags --quiet "$UPSTREAM_REMOTE"
UP="$UPSTREAM_REMOTE/$UPSTREAM_BRANCH"

BEHIND=$(git rev-list --count "HEAD..$UP")
if [ "$BEHIND" -eq 0 ]; then
  note "already up to date with $UP — nothing to do."
  exit 0
fi
note "$BEHIND commit(s) to bring in from $UP."

# A backup branch per run. `-X ours` never leaves conflict markers, so without
# this there is no point to return to if the merge is wrong.
STAMP=$(date +%Y%m%d-%H%M%S)
BACKUP="backup/pre-sync-$STAMP"
git branch -f "$BACKUP" HEAD
note "backup:        $BACKUP -> $(git rev-parse --short HEAD)"

# Recorded before the merge, because merging moves the merge base.
BASE=$(git merge-base HEAD "$UP")
UPSTREAM_CHANGED=$(git diff --name-only "$BASE" "$UP" | sort)

note ""
note "upstream changed $([ "$(printf '%s' "$UPSTREAM_CHANGED" | grep -c .)" -gt 0 ] && printf '%s' "$UPSTREAM_CHANGED" | grep -c .) file(s); merging with -X ours ..."
git merge -X ours --no-edit "$UP" >/dev/null

# ---------------------------------------------------------------------------
# The fork's invariants. These are the things a sync must never remove, written
# as assertions rather than trusted to review: `-X ours` resolves silently, so a
# wrongly-resolved file is invisible without a check like this one.
# ---------------------------------------------------------------------------
MISSING=0
want() {
  if [ ! -f "$1" ]; then
    printf '  MISSING FILE   %s\n' "$1" >&2; MISSING=1; return
  fi
  if ! grep -qF -- "$2" "$1"; then
    printf '  MISSING IN %s : %s\n' "$1" "$2" >&2; MISSING=1
  fi
}

# ci.yml is upstream-owned: this fork keeps no edits in it, so every upstream
# change to the CI workflow merges in instead of colliding. The fork's own
# Docker job lives in docker-publish.yml, a file upstream does not have, which
# is why it can never be dropped by a merge.
same_as_upstream() {
  local ours theirs
  ours=$(git ls-files -s -- "$1" | awk '{print $2}')
  theirs=$(git ls-tree "$UP" -- "$1" | awk '{print $3}')
  if [ "$ours" != "$theirs" ]; then
    printf '  DRIFTED FROM UPSTREAM: %s\n' "$1" >&2
    printf '    this file is supposed to stay upstream-owned so upstream edits merge in;\n' >&2
    printf '    for workflows: put the fork edit in a fork-owned workflow file instead;\n' >&2
    printf '    for CHANGELOG.md: put the entry in a .changes/ fragment instead\n' >&2
    MISSING=1
  fi
}
same_as_upstream .github/workflows/ci.yml
# CHANGELOG.md is upstream-owned for a different reason: upstream's release
# tooling rewrites it wholesale (`make changelog-merge` files every fragment
# under a new `## [TAG]` heading). A fork edit there makes every future release
# section conflict, and `-X ours` would then keep the fork's copy and drop
# upstream's release notes. Fork entries belong in .changes/ fragments, which
# upstream does not have and which therefore never collide -- this repo's own
# rule too (AGENTS.md 7.6). Running `make changelog-merge` in the fork will trip
# this assertion, deliberately: that is a decision, not an accident.
same_as_upstream CHANGELOG.md

want .github/workflows/docker-publish.yml 'REGISTRY_IMAGE: lautdalam/9router-go'
want .github/workflows/docker-publish.yml 'docker-publish'
# Without this the multi-arch build fails: BuildKit has no binfmt handler for
# the foreign architecture. Caught once already, after the job was written
# without it.
want .github/workflows/docker-publish.yml 'setup-qemu-action'
want .github/workflows/release.yml        'REGISTRY_IMAGE: lautdalam/9router-go'
want scripts/notify-telegram.sh           'lautdalam/9router-go'
want internal/handlers/dashboard/combos_optimal.go 'autoOptimalComboName'
want internal/handlers/dashboard/routes.go 'combos/auto-optimal'
want internal/handlers/dashboard/routes.go 'url.PathUnescape'
want internal/handlers/dashboard/combos_family.go 'upsertFamilyCombos'
want web/src/components/combos/CombosView.svelte 'onAutoOptimalClick'
want web/src/api/client.ts                'buildAutoOptimalCombos'

if [ "$MISSING" -ne 0 ]; then
  printf '\nsync-upstream: invariants do not hold after the merge — one of the states above.\n' >&2
  printf 'Nothing is lost — go back with:\n\n    git reset --hard %s\n\n' "$BACKUP" >&2
  exit 1
fi
note "fork customisations: all present."

# ---------------------------------------------------------------------------
# What the -X ours resolution cost, made visible.
#
# Files upstream changed that still differ from upstream after the merge are
# exactly where our side won. Some of that difference is your own code (fine);
# some is an upstream change that was discarded (the price). The only way to
# tell them apart is to read the diff, so print the list and the command.
# ---------------------------------------------------------------------------
DROPPED=$(comm -12 <(printf '%s\n' "$UPSTREAM_CHANGED") <(git diff --name-only "$UP" HEAD | sort))

note ""
if [ -n "$DROPPED" ]; then
  note "Upstream also changed these files, and your version won:"
  printf '%s\n' "$DROPPED" | sed 's/^/  /'
  note ""
  note "Review what upstream wanted there (do this — it is the trade-off):"
  printf '%s\n' "$DROPPED" | sed "s|^|    git diff $UP -- |"
else
  note "No overlapping files: upstream's work merged in clean and nothing was dropped."
fi

# `web/dist` is gitignored, so on a fresh clone `go build ./...` dies inside
# web/embed.go before it ever looks at the merge -- a failure that says nothing
# about the sync. Stand in a throwaway index.html so the compile check tests the
# merge rather than the missing frontend, and take it away again afterwards.
PLACEHOLDER=""
cleanup_placeholder() {
  [ -n "$PLACEHOLDER" ] || return 0
  rm -f "$PLACEHOLDER"
  rmdir web/dist 2>/dev/null || true
}

if [ "$SKIP_BUILD" != "1" ] && command -v go >/dev/null 2>&1; then
  note ""
  if [ -z "$(ls -A web/dist 2>/dev/null)" ]; then
    mkdir -p web/dist
    printf '<!doctype html>\n' > web/dist/index.html
    PLACEHOLDER=web/dist/index.html
    note "web/dist is empty (the SPA is gitignored) — using a temporary placeholder."
  fi

  note "compiling (a textual merge can still be semantically broken) ..."
  BUILD_LOG=$(mktemp)
  if go build ./... >"$BUILD_LOG" 2>&1; then
    cleanup_placeholder
    rm -f "$BUILD_LOG"
    note "go build ./... ok"
  else
    cleanup_placeholder
    tail -20 "$BUILD_LOG" >&2
    rm -f "$BUILD_LOG"
    die "go build ./... FAILED — the merge is not usable. Inspect, or 'git reset --hard $BACKUP'."
  fi
fi

note ""
note "Not pushed. Review, then:"
note "    git log --oneline -1"
note "    git push origin $LOCAL_BRANCH"
