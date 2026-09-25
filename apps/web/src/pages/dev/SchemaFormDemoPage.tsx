import Accordion from '@mui/material/Accordion';
import AccordionDetails from '@mui/material/AccordionDetails';
import AccordionSummary from '@mui/material/AccordionSummary';
import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import MenuItem from '@mui/material/MenuItem';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import { useTheme } from '@mui/material/styles';
import { SchemaForm, isRecordSchema, type JsonSchema, type ProblemDetails } from '@ngfw/ui-kit/schema-form';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { domainSchemas, domains, rootSchema } from '../../schema/registry';
import { PageHeader } from '../../shell/PageHeader';
import { demoSchema, demoValue } from './demoSchema';

interface Choice {
  id: string;
  label: string;
  schema: JsonSchema;
  value: unknown;
  /** Per-path i18n keys for the form (I18N-1), e.g. the users screen's own `users:field.*` keys. */
  i18nPrefix?: string;
}

const INTERFACE_OPTIONS = ['GigabitEthernet0/8/0', 'TenGigabitEthernet0/0/0', 'loop0'];

/** `management.users[]` item — the generated schema, rendered with the users namespace's per-path keys. */
const USER_ITEM = (domainSchemas.management as { properties?: { users?: { items?: JsonSchema } } }).properties?.users?.items;

export function SchemaFormDemoPage() {
  const { t } = useTranslation(['dev', 'common']);
  const theme = useTheme();
  const choices = useMemo<Choice[]>(
    () => [
      { id: 'demo', label: t('dev:schemaForm.demo'), schema: demoSchema as JsonSchema, value: demoValue },
      { id: 'root', label: t('dev:schemaForm.root'), schema: rootSchema, value: undefined },
      ...(USER_ITEM
        ? [{ id: 'users-item', label: t('dev:schemaForm.domain', { title: t('users:title') }), schema: USER_ITEM, value: undefined, i18nPrefix: 'users:field' }]
        : []),
      ...domains.map((d) => ({ id: d.key, label: t('dev:schemaForm.domain', { title: d.title }), schema: d.schema, value: undefined })),
    ],
    [t],
  );
  const [selectedId, setSelectedId] = useState('demo');
  const [submitted, setSubmitted] = useState<unknown>(undefined);
  const [problem, setProblem] = useState<ProblemDetails | null>(null);
  const selected = choices.find((c) => c.id === selectedId) ?? choices[0]!;
  const isEmptyGenerated = selected.id !== 'demo' && isRecordSchema(selected.schema);

  const simulatedProblem: ProblemDetails = {
    type: 'about:blank',
    title: t('dev:schemaForm.problem.title'),
    status: 422,
    detail: t('dev:schemaForm.problem.detail'),
    errors: [
      { pointer: '/mtu', detail: t('dev:schemaForm.problem.mtu') },
      { pointer: '/subinterfaces/GigabitEthernet0~18~10.100/vlanId', detail: t('dev:schemaForm.problem.vlan') },
      { pointer: '/unknown/field', detail: t('dev:schemaForm.problem.unmapped') },
    ],
  };

  return (
    <PageHeader title={t('dev:schemaForm.title')}>
      <Stack gap={2} sx={{ maxInlineSize: 960 }}>
        <Alert severity="warning">{t('dev:banner')}</Alert>
        <TextField
          select
          label={t('dev:schemaForm.pick')}
          value={selected.id}
          onChange={(e) => {
            setSelectedId(e.target.value);
            setSubmitted(undefined);
            setProblem(null);
          }}
          sx={{ maxInlineSize: 480 }}
        >
          {choices.map((c) => (
            <MenuItem key={c.id} value={c.id}>
              {c.label}
            </MenuItem>
          ))}
        </TextField>
        {isEmptyGenerated && <Alert severity="info">{t('dev:schemaForm.emptyHint')}</Alert>}
        <Paper sx={{ p: 2 }}>
          <SchemaForm
            key={selected.id}
            schema={selected.schema}
            value={selected.value}
            problem={problem}
            interfaceOptions={INTERFACE_OPTIONS}
            i18nPrefix={selected.i18nPrefix}
            onSubmit={(v) => {
              setSubmitted(v);
              setProblem(null);
            }}
          >
            {selected.id === 'demo' && (
              <Button variant="outlined" color="warning" onClick={() => setProblem(problem ? null : simulatedProblem)}>
                {t(problem ? 'dev:schemaForm.clearProblem' : 'dev:schemaForm.simulateProblem')}
              </Button>
            )}
          </SchemaForm>
        </Paper>
        <Typography component="h3" variant="subtitle1">
          {t('dev:schemaForm.submitted')}
        </Typography>
        <Box component="pre" sx={{ m: 0, p: 2, overflow: 'auto', bgcolor: 'background.paper', border: 1, borderColor: 'divider', borderRadius: 1, fontFamily: theme.vrx.monoFontFamily, fontSize: 12 }}>
          {submitted === undefined ? t('dev:schemaForm.none') : JSON.stringify(submitted, null, 2)}
        </Box>
        <Accordion disableGutters>
          <AccordionSummary>{t('dev:schemaForm.source')}</AccordionSummary>
          <AccordionDetails>
            <Box component="pre" sx={{ m: 0, overflow: 'auto', fontFamily: theme.vrx.monoFontFamily, fontSize: 12 }}>
              {JSON.stringify(selected.schema, null, 2)}
            </Box>
          </AccordionDetails>
        </Accordion>
      </Stack>
    </PageHeader>
  );
}
