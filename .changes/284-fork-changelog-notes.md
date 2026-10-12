### 📝 Catatan implementasi fork — Auto Group combos by model family

- Fixed a latent bug the new feature exposed: `isLlmCombo` rejected any `kind !== 'llm'`, so the auto free-tier combo was invisible on the combos page. It now accepts the `auto-*` kinds while still excluding the upstream `search-combo` media combos.

### 📝 Catatan implementasi fork — dashboard backup import

- `ProfileSettingsView.svelte` posted the chosen file as **`multipart/form-data`**, but `HandleImportDatabase` (like upstream `src/app/api/settings/database/route.js`) does `request.json()` on the body, so every import died at the decode with HTTP 400 and the user saw "Failed to import database: Import failed with status 400". The client now reads `file.text()`, `JSON.parse`s it, and PUTs the raw JSON body — verified against `E:\9router-backup-2026-09-25T15-07-00-715Z.json`.
- The destructive op also re-authenticates against the stored bcrypt hash, and the client never sent a password at all: once a dashboard password was set, import (and the plain anchor-click export, which is likewise an `x-9r-password`-gated route) returned 401. Upstream prompts in a modal first; the port now does the same — both buttons open a password modal, and the password travels in the `x-9r-password` header, matching upstream exactly.
- Backup filename stamp regained full precision (`T15-07-00-715Z` instead of `T00-00-00-000Z`-style day-only), matching upstream's `toISOString().replace(/[.:]/g, '-')`.
- Server side accepts the password from **either** the body field or the header, so the pre-existing body-embedded contract keeps working; `TestHandleImportDatabase_ClientContract` pins both the multipart rejection and the header auth. The upstream post-import `applyOutboundProxyEnv` has no Go equivalent yet (no `outboundProxyUrl` handling exists in this runtime), so it is noted in a comment rather than faked.

> These bullets describe **this fork's** implementation of two entries that
> upstream has already released (v1.9.6 and v1.9.5). They existed only inside the
> stale `## [Unreleased]` block that duplicated already-released content, so they
> are moved here before `CHANGELOG.md` is taken back to upstream's own version.
> Without this file the next sync would have dropped them, silently — see
> `docs/FORK_SYNC.md`. Both entries themselves are unchanged and remain in
> upstream's release sections.
