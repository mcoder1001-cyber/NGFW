import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { WsProvider, defaultStreamUrl } from '@ngfw/ui-kit/ws';
import { useMemo } from 'react';
import { RouterProvider } from 'react-router';
import { createAppRouter } from './router';
import { UiSettingsProvider } from './settings/UiSettings';

const defaultQueryClient = new QueryClient({ defaultOptions: { queries: { retry: 1, staleTime: 5_000 } } });

export interface AppProps {
  /** Injected by tests (memory router); the browser router is created otherwise. */
  router?: ReturnType<typeof createAppRouter>;
  /** Injected by tests; defaults to `wss://<host>/api/v1/stream`. */
  streamUrl?: string;
  /** Injected by tests (e.g. with queries disabled); one app-wide client otherwise. */
  queryClient?: QueryClient;
}

/** Providers in dependency order: server state → UI settings/theme/i18n → the single WebSocket → routes. */
export function App({ router, streamUrl, queryClient }: AppProps) {
  const appRouter = useMemo(() => router ?? createAppRouter(), [router]);
  const wsOptions = useMemo(() => ({ url: streamUrl ?? defaultStreamUrl() }), [streamUrl]);
  return (
    <QueryClientProvider client={queryClient ?? defaultQueryClient}>
      <UiSettingsProvider>
        <WsProvider options={wsOptions}>
          <RouterProvider router={appRouter} />
        </WsProvider>
      </UiSettingsProvider>
    </QueryClientProvider>
  );
}
