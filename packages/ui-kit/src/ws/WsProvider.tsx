import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import { VrxWsClient, type VrxWsClientOptions, type WsStatus } from './client.js';

const WsContext = createContext<VrxWsClient | null>(null);

export interface WsProviderProps {
  /** Either a ready client (tests) or the options to build the single app-wide client. */
  client?: VrxWsClient;
  options?: VrxWsClientOptions;
  children: ReactNode;
}

/** Provides THE WebSocket client. Nothing connects until the first `useTopic()` subscriber mounts. */
export function WsProvider({ client, options, children }: WsProviderProps) {
  const value = useMemo(() => {
    if (client) return client;
    if (!options) throw new Error('WsProvider needs `client` or `options`');
    return new VrxWsClient(options);
  }, [client, options]);
  useEffect(() => () => value.close(), [value]);
  return <WsContext.Provider value={value}>{children}</WsContext.Provider>;
}

export function useWsClient(): VrxWsClient {
  const c = useContext(WsContext);
  if (!c) throw new Error('useWsClient must be used inside <WsProvider>');
  return c;
}

/** Live connection status of the shared client. */
export function useWsStatus(): WsStatus {
  const client = useWsClient();
  const [status, setStatus] = useState<WsStatus>(client.status);
  useEffect(() => {
    setStatus(client.status);
    return client.onStatus(setStatus);
  }, [client]);
  return status;
}

/** Derive the stream URL from the page location (same origin, `/api/v1/stream`). */
export function defaultStreamUrl(loc: { protocol: string; host: string } = window.location): string {
  const proto = loc.protocol === 'https:' ? 'wss:' : 'ws:';
  return `${proto}//${loc.host}/api/v1/stream`;
}
