import type { ProviderConnection, Settings } from '../../api/client'
import { proxyBadgeInfo } from './proxyBadge'

/** Minimal connection shape needed to decide whether it carries a proxy. */
export type ProxyProbeConnection = Pick<
  ProviderConnection,
  'provider' | 'providerSpecificData'
>

/**
 * True when a provider routes through a proxy, either because:
 *  - a provider-level strategy binds a proxy pool (`settings.providerStrategies[id].proxyPoolId`), or
 *  - at least one of its connections carries a proxy pool id or a legacy
 *    per-connection proxy.
 *
 * Mirrors the per-connection signal shown in the provider detail rows, so the
 * overview card and the detail view never disagree.
 */
export function providerUsesProxy(
  providerId: string,
  connections: ProxyProbeConnection[],
  settings?: Settings | null,
): boolean {
  const strategyPoolId = settings?.providerStrategies?.[providerId]?.proxyPoolId
  if (typeof strategyPoolId === 'string' && strategyPoolId) return true
  return connections.some(
    (c) => c.provider === providerId && proxyBadgeInfo(c).hasAnyProxy,
  )
}
