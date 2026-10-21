### ✨ feat(proxy): auto-fetch residential proxies from stormsia.github.io

- **New package `internal/proxy/fetcher`**: fetches residential proxy list from `https://stormsia.github.io/proxy-list/proxies.json`, parses the JSON response, and supports `http`, `socks4`, and `socks5` proxies.
- **Speed test integration**: reuses the existing `probeViaProxy` pattern to test fetched proxies concurrently, sorting by latency and keeping only the fastest.
- **Auto-refresh background loop**: `StartProxyRefreshLoop` runs on a configurable interval (default 1 hour), updating the `stormsia-residential-auto` proxy pool in the database. Fail-open: a failed refresh is logged and retried on the next tick.
- **Configuration via environment variables**:
  - `PROXY_REFRESH_ENABLED=true` to enable
  - `PROXY_REFRESH_INTERVAL_HOURS=1` to set refresh interval
  - `PROXY_REFRESH_TOP_N=15` to set number of proxies to keep
- **Integration**: the refresh loop starts automatically in `ProvideServer` when `ProxyRefreshEnabled` is true, alongside the existing retention and catalog sync loops.
