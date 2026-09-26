import type { RouteObject } from 'react-router';
import { DEV_ROUTES } from '../../build-flags';

/** A developer preview page: its route and its entry in the "Developer" navigation group. */
export interface DevPreview {
  id: string;
  /** Absolute path of the nav entry (`/dev/…`). */
  path: string;
  labelKey: string;
  fallbackLabel: string;
  /** Child route of the app shell (code-split). */
  route: RouteObject;
}

/**
 * WEB-2 previews (dev builds only): the config kit's data widgets with synthetic values, and System › Secrets on its
 * real API. `DEV_ROUTES` is a build-time literal, so a production build holds an empty list and emits neither chunk.
 *
 * Not routed yet: the router and navigation have no `// web: WEB-2` anchor (WEB-2 envelope), and the Secrets page is not
 * registered anywhere. Wiring = one line under each anchor once it exists:
 *   router.tsx DEV_ROUTE_OBJECTS: `...DEV_PREVIEWS.map((p) => p.route),`
 *   nav.ts DEV_NAV_ITEMS:         `...DEV_PREVIEWS.map(({ id, path, labelKey, fallbackLabel }) => ({ id, path, labelKey, fallbackLabel, available: true })),`
 */
export const DEV_PREVIEWS: readonly DevPreview[] = DEV_ROUTES
  ? [
      {
        id: 'dev-config-kit',
        path: '/dev/config-kit',
        labelKey: 'dev:kitPreview.title',
        fallbackLabel: 'Config kit preview',
        route: { path: 'dev/config-kit', lazy: async () => ({ Component: (await import('../../config/widgets/KitPreview')).KitPreview }) },
      },
      {
        id: 'dev-secrets',
        path: '/dev/secrets',
        labelKey: 'config:kit.secrets.title',
        fallbackLabel: 'Secrets',
        route: { path: 'dev/secrets', lazy: async () => ({ Component: (await import('../SecretsPage')).SecretsPage }) },
      },
    ]
  : [];
