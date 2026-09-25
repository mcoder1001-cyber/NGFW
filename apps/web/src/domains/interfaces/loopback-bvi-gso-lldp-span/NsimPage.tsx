import Alert from '@mui/material/Alert';
import Button from '@mui/material/Button';
import Chip from '@mui/material/Chip';
import FormControlLabel from '@mui/material/FormControlLabel';
import Switch from '@mui/material/Switch';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import Typography from '@mui/material/Typography';
import { SchemaForm } from '@ngfw/ui-kit/schema-form';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { PageHeader } from '../../../shell/PageHeader';
import { problemFor } from '../InterfaceDrawer';
import { createMergePatch, localizeSchema } from '../model';
import { useCandidateInterfaces } from '../queries';
import { formSchemas, localizeAll, NSIM_POINTER, parentNames, withoutProps } from './model';
import { useCandidateServices, usePatchServices } from './queries';

const NS = 'loopback-bvi-gso-lldp-span';

/**
 * Network delay simulator (F-loopback-bvi-gso-lldp-span, WBS D1.9 — a lab tool, marked as such, under Tools): the
 * `services.nsim` form. VPP has no getter for nsim (write-only) and holds one model and one cross-connect pair, so only
 * the globals owner applies it; VPP cannot unconfigure the model (removing it leaves it inert).
 */
export function NsimPage() {
  const { t } = useTranslation(NS);
  const perms = usePermissions();
  const services = useCandidateServices();
  const ifs = useCandidateInterfaces();
  const patch = usePatchServices();
  const full = useMemo(() => formSchemas.nsim(), []);
  const current = services.data?.nsim;
  // the optional cross-connect is a switch of its own (review M6): SchemaForm would otherwise materialise it with its
  // required interfaces empty, and a model without a cross-connect could not be saved
  const [xcToggled, setXc] = useState<boolean | null>(null);
  const xc = xcToggled ?? current?.crossConnect !== undefined;
  const schema = useMemo(() => (xc ? full : withoutProps(full, ['crossConnect'])), [full, xc]);
  const formValue = useMemo(() => {
    if (xc || current === undefined) return current;
    const rest: Record<string, unknown> = { ...current };
    delete rest['crossConnect'];
    return rest;
  }, [current, xc]);

  const save = async (v: unknown) => {
    await patch
      .mutateAsync({ nsim: current === undefined ? v : createMergePatch(current, v) })
      .catch(() => undefined);
  };

  return (
    <PageHeader title={t('nsim.title')}>
      <Stack direction="row" gap={1} alignItems="center" sx={{ mb: 1 }}>
        <Chip size="small" color="secondary" label={t('nsim.lab')} />
        <Typography color="text.secondary">{t('nsim.intro')}</Typography>
      </Stack>
      <Alert severity="warning" sx={{ mb: 2 }}>
        {t('nsim.warning')}
      </Alert>
      {patch.isError && problemFor(patch.error, NSIM_POINTER) === null && (
        <ProblemAlert error={patch.error} sx={{ mb: 1 }} />
      )}
      {services.isSuccess && (
        <Paper variant="outlined" sx={{ p: 2 }}>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
            {current === undefined ? t('nsim.off') : t('nsim.on')}
          </Typography>
          <FormControlLabel
            sx={{ mb: 1 }}
            control={
              <Switch checked={xc} disabled={!perms.editConfig} onChange={(_e, on) => setXc(on)} />
            }
            label={t('nsim.crossConnectToggle')}
          />
          <SchemaForm
            schema={localizeAll(schema, (k, o) => t(k, o ?? {}), localizeSchema)}
            value={formValue}
            readOnly={!perms.editConfig}
            interfaceOptions={parentNames(ifs.data)}
            submitLabel={t('save')}
            resetLabel={t('reset')}
            problem={problemFor(patch.error, NSIM_POINTER)}
            onSubmit={(v) => void save(v)}
          >
            {current !== undefined && (
              <Button
                color="error"
                variant="outlined"
                disabled={!perms.editConfig || patch.isPending}
                onClick={() => void patch.mutateAsync({ nsim: null }).catch(() => undefined)}
              >
                {t('nsim.remove')}
              </Button>
            )}
          </SchemaForm>
        </Paper>
      )}
    </PageHeader>
  );
}
