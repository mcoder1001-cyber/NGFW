import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { WsProvider, defaultStreamUrl, type NgfwWsClient } from '@ngfw/ui-kit/ws';
import { useMemo } from 'react';
import { RouterProvider } from 'react-router';
import { AuthProvider } from './auth/AuthProvider';
import { session as appSession, streamProtocols, type Session } from './auth/session';
import { createAppRouter } from './router';
import { UiSettingsProvider } from './settings/UiSettings';
import { InterfaceDisplayNamesProvider } from './domains/interfaces/InterfaceDisplayNamesProvider';

const defaultQueryClient = new QueryClient({
  defaultOptions: { queries: { retry: 1, staleTime: 5_000 } },
});

export interface AppProps {
  /** Injected by tests (memory router); the browser router is created otherwise. */
  wsClient?: NgfwWsClient;
  router?: ReturnType<typeof createAppRouter>;
  /** Injected by tests; defaults to `wss://<host>/api/v1/stream`. */
  streamUrl?: string;
  /** Injected by tests (e.g. with queries disabled); one app-wide client otherwise. */
  queryClient?: QueryClient;
  /** Injected by tests; the app-wide session otherwise. */
  session?: Session;
}

/** Providers in dependency order: server state → UI settings/theme/i18n → the single WebSocket → session → routes. */
export function App({ router, streamUrl, queryClient, wsClient, session = appSession }: AppProps) {
  const appRouter = useMemo(() => router ?? createAppRouter(), [router]);
  // The stream authenticates with subprotocols (ngfw.v1 + bearer.<access token>), read on every (re)connect.
  const wsOptions = useMemo(
    () => ({ url: streamUrl ?? defaultStreamUrl(), protocols: () => streamProtocols(session) }),
    [streamUrl, session],
  );
  return (
    <QueryClientProvider client={queryClient ?? defaultQueryClient}>
      <UiSettingsProvider>
        <WsProvider options={wsOptions} {...(wsClient ? { client: wsClient } : {})}>
          <AuthProvider session={session}>
            <InterfaceDisplayNamesProvider>
              <RouterProvider router={appRouter} />
            </InterfaceDisplayNamesProvider>
          </AuthProvider>
        </WsProvider>
      </UiSettingsProvider>
    </QueryClientProvider>
  );
}
