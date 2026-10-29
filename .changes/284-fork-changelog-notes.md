### ✨ feat(connections): kartu provider di overview menandai yang routing lewat proxy

Sebelumnya satu-satunya indikasi bahwa sebuah provider memakai proxy ada di baris
tiap koneksi pada halaman detail (`ProviderDetailView`). Kartu pada grid overview
tidak menunjukkan apa pun, jadi operator harus masuk ke detail satu per satu untuk
mengetahui provider mana yang egress-nya lewat proxy.

`ProviderCard` sekarang menerima prop `hasProxy` dan menampilkan badge `info`
(ikon `vpn_lock`) di luar rantai if/else status, sehingga tanda ikut muncul pada
provider yang `Disabled`, ber-`Error`, maupun `No connections` — bukan hanya pada
yang `Connected`.

Sinyalnya dihitung `providerUsesProxy` (`web/src/components/connections/providerProxy.ts`),
yang menandai provider bila **salah satu** terpenuhi:

- `settings.providerStrategies[id].proxyPoolId` terisi — binding pool level provider,
  yang dipakai provider free tier tanpa koneksi.
- salah satu koneksi provider itu membawa `proxyPoolId` atau legacy per-connection
  proxy (`connectionProxyEnabled` + `connectionProxyUrl`).

Sinyal per-koneksi sengaja dibaca lewat `proxyBadgeInfo` yang sudah dipakai baris
detail, jadi overview dan detail tidak bisa berbeda pendapat. Helper diekstrak ke
modul terpisah (bukan inline di grid) agar bisa diuji tanpa merender komponen.

Plumbing: `settings` diteruskan `App.svelte` → `ConnectionsView` →
`ProvidersOverviewGrid`; semuanya prop opsional, jadi komponen tetap bisa dipakai
sendirian.

Diverifikasi: `bun test src/components/connections/` 74 pass (11 test baru untuk
`providerUsesProxy`, termasuk kasus negatif — pool id kosong, proxy disabled, dan
proxy milik provider lain), `bun run build` hijau, dan
`bun run ratchet:svelte` hijau (0 unresolved identifier, baseline tetap 83).

### 🐛 fix(changelog): entri pending tidak lagi masuk ke tengah prosa atau di atas judul

Dua cacat pada `internal/changelogfrag/splice`. Keduanya baru terpicu setelah `CHANGELOG.md` menjadi upstream-owned: upstream tidak punya section `## [Unreleased]` setelah rilis, jadi jalur *fallback*-nya terpakai di **setiap** request.

- **Judul `# Changelog` terdorong ke bawah.** Jalur fallback menyisipkan blok `## [Unreleased]` di offset 0, jadi dokumen yang dirender terbuka dengan `## [Unreleased]` dan judulnya seolah menjadi heading milik entri pending. Sekarang blok itu mendarat tepat di bawah judul (`prependAfterTitle`), dan seluruh section rilis tetap di bawahnya.
- **Heading yang hanya dikutip di prosa dianggap section.** `splice` mencari `## [Unreleased]` dengan `strings.Index`, sedangkan catatan rilis v1.9.11 menulis heading itu di tengah kalimat ("baris 5-6 (`## [Unreleased]` + entry pertama)"). Akibatnya setiap fragment disisipkan ke tengah bullet catatan rilis dan kalimatnya terpotong. `findUnreleasedHeading` sekarang menuntut heading sebagai **satu baris utuh** (`\r` di ujung ditoleransi karena working tree di-checkout CRLF).
- **Ketahuan dari CI, bukan dari mata.** Suite integrasi (`TestChangelogShowsPendingFragments`) menuntut dokumen `/api/changelog` tetap diawali `# Changelog` dan tetap memuat section rilis. Test itu gagal di CI untuk `583de20` dan `94f081d` **sementara `go test ./...` lokal tetap hijau**, karena suite integrasi hanya jalan dengan build tag.
