import { PgButton as Button, PgText as TextField } from '@amnafzar/common-ui-lib/forms';
import {
  ReactGeereedMaterial,
  type GeereedTablePassthrough,
} from '@amnafzar/common-ui-lib/geereed';
import {
  PgBox as Box,
  PgDivider as Divider,
  PgPaper as Paper,
  PgTypography as Typography,
} from '@amnafzar/common-ui-lib/primitives';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { LTR_INPUT, NS, SUBTREES } from './model';
import { PortBlockCalculator } from './PortBlockCalculator';
import { useDet44Lookup, useDet44Sessions } from './queries';
import { SubtreeForm } from './SubtreeForm';

function Det44Sessions() {
  const { t } = useTranslation(NS);
  const [draft, setDraft] = useState('');
  const [user, setUser] = useState('');
  const q = useDet44Sessions(user);
  return (
    <Box sx={{ mb: 3 }}>
      <Typography variant="h6" component="h3" sx={{ mb: 1 }}>
        {t('det44.sessions')}
      </Typography>
      <Box sx={{ display: 'flex', gap: 1, mb: 1 }}>
        <TextField
          size="small"
          label={t('det44.user')}
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          slotProps={LTR_INPUT}
        />
        <Button variant="outlined" onClick={() => setUser(draft.trim())}>
          {t('show')}
        </Button>
      </Box>
      {q.isError && <ProblemAlert error={q.error} sx={{ mb: 1 }} />}
      {q.data && (
        <>
          <Typography sx={{ mb: 1 }}>
            {t('det44.block', {
              outside: q.data.outsideAddress,
              lo: q.data.portLo,
              hi: q.data.portHi,
              total: q.data.total,
            })}
          </Typography>
          <Paper variant="outlined">
            <ReactGeereedMaterial
              tableProps={{ size: 'small', 'aria-label': t('det44.sessions') }}
              columns={[
                {
                  key: 'insidePort',
                  headerRenderer: () => t('col.insidePort'),
                  filterable: false,
                  renderer: (value) => <>{value}</>,
                },
                {
                  key: 'outsidePort',
                  headerRenderer: () => t('col.outsidePort'),
                  filterable: false,
                  renderer: (value) => <>{value}</>,
                },
                {
                  key: 'external',
                  headerRenderer: () => t('col.external'),
                  filterable: false,
                  renderer: (value) => <>{value}</>,
                },
                {
                  key: 'state',
                  headerRenderer: () => t('col.state'),
                  filterable: false,
                  renderer: (value) => <>{value}</>,
                },
              ]}
              items={[
                ...(q.data.items.map((s) => ({
                  id: `${s.insidePort}|${s.externalAddress}|${s.externalPort}`,
                  insidePort: s.insidePort,
                  outsidePort: s.outsidePort,
                  external: `${s.externalAddress}:${s.externalPort}`,
                  state: s.state,
                  cellProps: { external: { dir: 'ltr' } } satisfies Record<
                    string,
                    NonNullable<ReturnType<NonNullable<GeereedTablePassthrough['cellProps']>>>
                  >,
                })) ?? []),
              ]}
              rowKey={(item) => item.id}
              rowProps={(item) => item.rowProps ?? {}}
              cellProps={(item, column) => item.cellProps?.[column.key] ?? {}}
              emptyContent={q.data.items.length === 0 ? t('empty') : null}
              noFormWrapper
              embedded
              hideToolbar
              hidePagination
              hideFilterBuilderComponent
              search={false}
              sortable={false}
              exportToCSV={false}
              footer={false}
              memoEnable={false}
            />
          </Paper>
        </>
      )}
    </Box>
  );
}

function Det44LookupForm() {
  const { t } = useTranslation(NS);
  const [inside, setInside] = useState('');
  const [outside, setOutside] = useState('');
  const [port, setPort] = useState('');
  const m = useDet44Lookup();
  return (
    <Box sx={{ mb: 3 }}>
      <Typography variant="h6" component="h3" sx={{ mb: 1 }}>
        {t('det44.lookup')}
      </Typography>
      <Box sx={{ display: 'flex', gap: 1, flexWrap: 'wrap', mb: 1 }}>
        <TextField
          size="small"
          label={t('det44.inside')}
          value={inside}
          onChange={(e) => setInside(e.target.value)}
          slotProps={LTR_INPUT}
        />
        <Button variant="outlined" onClick={() => m.mutate({ inside: inside.trim() })}>
          {t('det44.forward')}
        </Button>
        <TextField
          size="small"
          label={t('det44.outside')}
          value={outside}
          onChange={(e) => setOutside(e.target.value)}
          slotProps={LTR_INPUT}
        />
        <TextField
          size="small"
          type="number"
          label={t('det44.port')}
          value={port}
          onChange={(e) => setPort(e.target.value)}
        />
        <Button
          variant="outlined"
          onClick={() => m.mutate({ outside: outside.trim(), port: Number(port) })}
        >
          {t('det44.reverse')}
        </Button>
      </Box>
      {m.isError && <ProblemAlert error={m.error} sx={{ mb: 1 }} />}
      {m.data && (
        <Typography data-testid="det44-lookup-result">
          {m.data.portLo === null
            ? t('det44.reverseResult', { inside: m.data.inside, outside: m.data.outside })
            : t('det44.forwardResult', {
                inside: m.data.inside,
                outside: m.data.outside,
                lo: m.data.portLo,
                hi: m.data.portHi,
              })}
        </Typography>
      )}
    </Box>
  );
}

/** CGNAT: DET44 (form, port-block calculator, per-user sessions, lookup) and DS-Lite (form). */
export function CgnatTab() {
  const { t } = useTranslation(NS);
  return (
    <Box>
      <Typography color="text.secondary" sx={{ mb: 2 }}>
        {t('det44.intro')}
      </Typography>
      <SubtreeForm subtree={SUBTREES.det44} />
      <Divider sx={{ my: 3 }} />
      <PortBlockCalculator />
      <Det44Sessions />
      <Det44LookupForm />
      <Divider sx={{ my: 3 }} />
      <Typography variant="h6" component="h3" sx={{ mb: 1 }}>
        {t('dslite.title')}
      </Typography>
      <Typography color="text.secondary" sx={{ mb: 2 }}>
        {t('dslite.intro')}
      </Typography>
      <SubtreeForm subtree={SUBTREES.dslite} />
    </Box>
  );
}
