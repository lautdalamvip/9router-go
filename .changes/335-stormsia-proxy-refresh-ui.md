### Proxy Pools
- Added "Get Proxies from Stormsia" button to the Proxy Pools dashboard for one-click manual refresh.
- Exposed `POST /api/proxy-pools/stormsia/refresh` to fetch residential proxies from stormsia.github.io, speed-test them, and create/update the auto-generated pool.
- Auto-generated pool (`stormsia-residential-auto`) now appears in the proxy pools list immediately after a successful fetch.
