import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import LinearProgress from '@mui/material/LinearProgress';
import Tab from '@mui/material/Tab';
import Tabs from '@mui/material/Tabs';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { ExportersPanel } from './ExportersPanel';
import { FlowprobePanel } from './FlowprobePanel';
import { SflowPanel } from './SflowPanel';
import {
  useCandidateIpfix,
  useInterfaceNames,
  useIpfixState,
  usePatchIpfix,
  type IpfixConfig,
  type IpfixState,
} from './queries';

export const NS = 'ipfix-sflow';
const SECTIONS = ['exporters', 'flowprobe', 'sflow'] as const;
type Section = (typeof SECTIONS)[number];

/** Services → Flow export (F-ipfix-sflow): IPFIX exporters, flowprobe and sFlow sampling; config in the candidate, state live. */
export default function FlowExportTab() {
  const { t } = useTranslation(NS);
  const [section, setSection] = useState<Section>('exporters');
  const perms = usePermissions();
  const cfg = useCandidateIpfix();
  const state = useIpfixState();
  const names = useInterfaceNames();
  const patch = usePatchIpfix();
  const props = {
    cfg: cfg.data ?? {},
    state: state.data,
    interfaces: names.data ?? [],
    readOnly: !perms.editConfig,
    pending: patch.isPending,
    save: (p: Record<string, unknown>) => patch.mutateAsync(p),
  };
  return (
    <Box>
      <Alert severity="info" sx={{ mb: 2 }} data-testid="hsflowd-banner">
        {t('hsflowdBanner')}
      </Alert>
      {state.data !== undefined && !state.data.globalsOwner && (
        <Alert severity="warning" sx={{ mb: 2 }}>
          {t('notGlobalsOwner')}
        </Alert>
      )}
      {state.isError && <ProblemAlert error={state.error} sx={{ mb: 2 }} />}
      {patch.isError && <ProblemAlert error={patch.error} sx={{ mb: 2 }} />}
      {(cfg.isPending || state.isPending) && <LinearProgress aria-label={t('loading')} />}
      <Tabs
        value={section}
        onChange={(_, v: Section) => setSection(v)}
        aria-label={t('sections')}
        sx={{ mb: 2 }}
      >
        {SECTIONS.map((s) => (
          <Tab
            key={s}
            value={s}
            label={t(`section.${s}`)}
            id={`ipfix-tab-${s}`}
            aria-controls={`ipfix-panel-${s}`}
          />
        ))}
      </Tabs>
      <Box role="tabpanel" id={`ipfix-panel-${section}`} aria-labelledby={`ipfix-tab-${section}`}>
        {section === 'exporters' && <ExportersPanel {...props} />}
        {section === 'flowprobe' && <FlowprobePanel {...props} />}
        {section === 'sflow' && <SflowPanel {...props} />}
      </Box>
    </Box>
  );
}

export interface PanelProps {
  cfg: IpfixConfig;
  state: IpfixState | undefined;
  interfaces: string[];
  readOnly: boolean;
  pending: boolean;
  save: (patch: Record<string, unknown>) => Promise<unknown>;
}
