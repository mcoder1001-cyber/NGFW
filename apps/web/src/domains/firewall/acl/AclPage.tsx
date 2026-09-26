import Box from '@mui/material/Box';
import Tab from '@mui/material/Tab';
import Tabs from '@mui/material/Tabs';
import Typography from '@mui/material/Typography';
import { useTranslation } from 'react-i18next';
import { useSearchParams } from 'react-router';
import { PageHeader } from '../../../shell/PageHeader';
import { AttachmentsTab } from './AttachmentsTab';
import { ListsTab } from './ListsTab';
import { MacipTab } from './MacipTab';
import { ACL_TABS, isAclTab, type AclTab } from './model';
import { AdlInfo } from './parts';
import { RulesTab } from './RulesTab';

const TAB_PARAM = 'tab';
const LIST_PARAM = 'list';

/**
 * F-acl: the ACL page (`/firewall/acl`) — L3/L4 lists with live VPP status, the rule editor of one list (`?list=`) on a
 * server-paged grid that stays usable at 100 000 rules, CSV import/export, attachments (configured and as VPP holds
 * them, other owners' ACLs included) and MACIP lists. Every edit goes to the candidate; commit from the pending bar.
 */
export function AclPage() {
  const { t } = useTranslation('acl');
  const [params, setParams] = useSearchParams();
  const tabParam = params.get(TAB_PARAM);
  const tab: AclTab = isAclTab(tabParam) ? tabParam : 'lists';
  const list = params.get(LIST_PARAM);

  const selectTab = (next: AclTab) =>
    setParams(
      (p) => {
        p.set(TAB_PARAM, next);
        return p;
      },
      { replace: true },
    );
  const openRules = (name: string, replace = false) =>
    setParams(
      (p) => {
        p.set(TAB_PARAM, 'rules');
        p.set(LIST_PARAM, name);
        return p;
      },
      { replace },
    );

  return (
    <PageHeader title={t('title')}>
      <Typography color="text.secondary" sx={{ mb: 2 }}>
        {t('intro')}
      </Typography>
      <AdlInfo />
      <Tabs
        value={tab}
        onChange={(_e, v: AclTab) => selectTab(v)}
        variant="scrollable"
        allowScrollButtonsMobile
        aria-label={t('tabsLabel')}
        sx={{ mb: 2, borderBlockEnd: 1, borderColor: 'divider' }}
      >
        {ACL_TABS.map((k) => (
          <Tab
            key={k}
            value={k}
            label={t(`tabs.${k}`)}
            id={`acl-tab-${k}`}
            aria-controls={`acl-panel-${k}`}
          />
        ))}
      </Tabs>
      <Box role="tabpanel" id={`acl-panel-${tab}`} aria-labelledby={`acl-tab-${tab}`}>
        {tab === 'lists' && <ListsTab onOpenRules={(name) => openRules(name)} />}
        {tab === 'rules' && <RulesTab list={list} onSelectList={(name) => openRules(name, true)} />}
        {tab === 'attachments' && <AttachmentsTab />}
        {tab === 'macip' && <MacipTab />}
      </Box>
    </PageHeader>
  );
}
