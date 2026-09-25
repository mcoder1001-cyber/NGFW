import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import { SchemaForm } from '@ngfw/ui-kit/schema-form';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { useInterfaceNames } from './api';
import { problemFor } from './common';
import { localizeSchema, NS, pickSchema, type MplsConfig } from './model';
import { useMpls } from './useMpls';

/** Pointer of `routing.mpls` in the document (server problems under it land on the form). */
const MPLS_POINTER = '/routing/mpls';
/**
 * MPLS-enabled interfaces and additional MPLS tables (`routing.mpls.interfaces`, `routing.mpls.tables`): one
 * schema-driven form. MPLS table 0 is the default table and is never listed; enabling MPLS on an interface needs it
 * (only the globals owner creates it, D-071).
 */
export function InterfacesTab() {
  const { t } = useTranslation(NS);
  const perms = usePermissions();
  const { routing, mpls, update, write } = useMpls();
  const ifNames = useInterfaceNames();
  const schema = useMemo(
    () => localizeSchema(pickSchema('interfaces', 'tables'), (k, o) => t(k, o ?? {})),
    [t],
  );
  const value = { interfaces: mpls?.interfaces ?? [], tables: mpls?.tables ?? {} };
  return (
    <Box>
      <Typography color="text.secondary" sx={{ mb: 2 }}>
        {t('interfaces.intro')}
      </Typography>
      <Alert severity="info" sx={{ mb: 2 }}>
        {t('interfaces.table0')}
      </Alert>
      {routing.isError && <ProblemAlert error={routing.error} sx={{ mb: 1 }} />}
      {write.isError && <ProblemAlert error={write.error} sx={{ mb: 1 }} />}
      <SchemaForm
        key={routing.dataUpdatedAt}
        schema={schema}
        value={value}
        readOnly={!perms.editConfig}
        interfaceOptions={ifNames}
        submitLabel={t('save')}
        resetLabel={t('reset')}
        problem={problemFor(write.error, MPLS_POINTER)}
        onSubmit={async (v) => {
          const next = v as Pick<MplsConfig, 'interfaces' | 'tables'>;
          await update((m) => ({ ...m, interfaces: next.interfaces, tables: next.tables }));
        }}
      />
    </Box>
  );
}
