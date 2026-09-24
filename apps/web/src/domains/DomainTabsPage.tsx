import Box from '@mui/material/Box';
import CircularProgress from '@mui/material/CircularProgress';
import Tab from '@mui/material/Tab';
import Tabs from '@mui/material/Tabs';
import { Suspense, type ComponentType } from 'react';
import { useTranslation } from 'react-i18next';
import { useSearchParams } from 'react-router';
import { DomainPlaceholderPage } from '../pages/DomainPlaceholderPage';
import type { RootKey } from '../schema/registry';
import { PageHeader } from '../shell/PageHeader';

/** One tab of a shared domain page, registered by a feature: one entry under its anchor in `<domain>/tabs.ts`. */
export interface DomainTab {
  /** Unique within the page; selects the tab through `?tab=<id>`. */
  id: string;
  /** i18n key of the label in the feature's own namespace, e.g. `wireguard:tab`. */
  labelKey: string;
  /** The screen; `lazy(() => import('…'))` keeps it code-split. */
  Component: ComponentType;
}

export interface DomainTabsPageProps {
  domainKey: RootKey;
  /** The page's own i18n namespace (`title`, `tabs`, `loading`). */
  ns: string;
  tabs: readonly DomainTab[];
}

/**
 * A domain screen shared by several features (W-seed shell; wave-A launch plan §5.2): vpn (P11, F-wireguard) and services
 * (F-kea-dhcp-relay, F-unbound-chrony-syslog). With no tab registered it renders exactly the domain placeholder, so the route
 * exists without a user-visible change; the feature that registers the first tab also adds the domain to BUILT_DOMAINS.
 */
export function DomainTabsPage({ domainKey, ns, tabs }: DomainTabsPageProps) {
  const { t } = useTranslation([ns, 'nav']);
  const [params, setParams] = useSearchParams();
  const first = tabs[0];
  if (first === undefined) return <DomainPlaceholderPage domainKey={domainKey} />;
  const current = tabs.find((tab) => tab.id === params.get('tab')) ?? first;
  const Current = current.Component;
  return (
    <PageHeader title={t('title')}>
      <Tabs
        value={current.id}
        onChange={(_, id: string) => setParams({ tab: id }, { replace: true })}
        aria-label={t('tabs')}
        variant="scrollable"
        sx={{ mb: 2 }}
      >
        {tabs.map((tab) => (
          <Tab key={tab.id} value={tab.id} label={t(tab.labelKey)} id={`${domainKey}-tab-${tab.id}`} aria-controls={`${domainKey}-panel-${tab.id}`} />
        ))}
      </Tabs>
      <Box role="tabpanel" id={`${domainKey}-panel-${current.id}`} aria-labelledby={`${domainKey}-tab-${current.id}`}>
        <Suspense fallback={<CircularProgress aria-label={t('loading')} />}>
          <Current />
        </Suspense>
      </Box>
    </PageHeader>
  );
}
