import { describe, expect, test } from 'bun:test'
import { providerUsesProxy } from './providerProxy'
import type { ProviderConnection, Settings } from '../../api/client'

/** Builds a connection with only the fields the proxy probe reads. */
function conn(
  provider: string,
  providerSpecificData: Record<string, unknown>,
): ProviderConnection {
  return { provider, providerSpecificData } as unknown as ProviderConnection
}

describe('providerUsesProxy', () => {
  test('false when no strategy pool and no connection proxy', () => {
    const connections = [conn('openai', {})]
    expect(providerUsesProxy('openai', connections, {})).toBe(false)
  })

  test('false when settings omitted entirely', () => {
    expect(providerUsesProxy('openai', [conn('openai', {})])).toBe(false)
  })

  test('true when a connection binds a proxy pool', () => {
    const connections = [conn('openai', { proxyPoolId: 'pool-1' })]
    expect(providerUsesProxy('openai', connections, {})).toBe(true)
  })

  test('true when a connection uses a legacy per-connection proxy', () => {
    const connections = [
      conn('openai', { connectionProxyEnabled: true, connectionProxyUrl: 'http://p:8080' }),
    ]
    expect(providerUsesProxy('openai', connections, {})).toBe(true)
  })

  test('false when legacy proxy is disabled', () => {
    const connections = [
      conn('openai', { connectionProxyEnabled: false, connectionProxyUrl: 'http://p:8080' }),
    ]
    expect(providerUsesProxy('openai', connections, {})).toBe(false)
  })

  test('false when legacy proxy is enabled but the url is empty', () => {
    const connections = [conn('openai', { connectionProxyEnabled: true, connectionProxyUrl: '' })]
    expect(providerUsesProxy('openai', connections, {})).toBe(false)
  })

  test('true when the provider strategy binds a proxy pool', () => {
    const settings: Settings = { providerStrategies: { openai: { proxyPoolId: 'pool-9' } } }
    expect(providerUsesProxy('openai', [], settings)).toBe(true)
  })

  test('false when the strategy pool id is an empty string', () => {
    const settings: Settings = { providerStrategies: { openai: { proxyPoolId: '' } } }
    expect(providerUsesProxy('openai', [], settings)).toBe(false)
  })

  test('ignores proxies belonging to other providers', () => {
    const connections = [conn('anthropic', { proxyPoolId: 'pool-1' })]
    expect(providerUsesProxy('openai', connections, {})).toBe(false)
  })

  test('true when only one of several connections carries a proxy', () => {
    const connections = [
      conn('openai', {}),
      conn('openai', { proxyPoolId: 'pool-1' }),
      conn('openai', {}),
    ]
    expect(providerUsesProxy('openai', connections, {})).toBe(true)
  })

  test('ignores a strategy bound to a different provider', () => {
    const settings: Settings = { providerStrategies: { anthropic: { proxyPoolId: 'pool-9' } } }
    expect(providerUsesProxy('openai', [], settings)).toBe(false)
  })
})
