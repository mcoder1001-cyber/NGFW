import EditIcon from '@mui/icons-material/Edit';
import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Chip from '@mui/material/Chip';
import Dialog from '@mui/material/Dialog';
import DialogActions from '@mui/material/DialogActions';
import DialogContent from '@mui/material/DialogContent';
import DialogTitle from '@mui/material/DialogTitle';
import IconButton from '@mui/material/IconButton';
import LinearProgress from '@mui/material/LinearProgress';
import Paper from '@mui/material/Paper';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import Typography from '@mui/material/Typography';
import { SchemaForm } from '@ngfw/ui-kit/schema-form';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { PageHeader } from '../../../shell/PageHeader';
import {
  adlText,
  localize,
  NS,
  problemUnder,
  schemas,
  securityPatch,
  securityRows,
  urpfText,
  withChoices,
  type AdlConfig,
  type AutoSdlConfig,
  type UrpfConfig,
} from './model';
import { useCandidate, usePatch, useRunning } from './queries';

const MONO = { fontFamily: 'monospace' } as const;
const AUTO_SDL_POINTER = '/services/autoSdl';

type IfDoc = Record<string, { urpf?: UrpfConfig; adl?: AdlConfig }>;
const esc = (s: string) => s.replace(/~/g, '~0').replace(/\//g, '~1');

/**
 * Firewall › ADL / Auto-SDL (F-rpf-adl-pbr): the anti-spoofing settings of every interface — uRPF (VPP urpf) and the
 * allow/deny list (VPP adl, sources checked against an allow-list VRF) — and the host stack's automatic source deny
 * list (services.autoSdl, a VPP-global applied by the globals owner only). The same uRPF/ADL fields are in the interface
 * drawer's security group. Edits are merge patches of the candidate; the pending-change bar commits them.
 */
export function AdlPage() {
  const { t } = useTranslation([NS, 'config']);
  const perms = usePermissions();
  const ifs = useCandidate<IfDoc>('interfaces');
  const running = useRunning<IfDoc>('interfaces');
  const vrfs = useCandidate<Record<string, unknown>>('vrfs');
  const services = useCandidate<{ autoSdl?: AutoSdlConfig }>('services');
  const patchIfs = usePatch('interfaces');
  const patchSvc = usePatch('services');
  const [editing, setEditing] = useState<string | null>(null);
  const readOnly = !perms.editConfig;
  const tr = (k: string, o?: Record<string, unknown>) => t(k, o ?? {});

  const rows = useMemo(() => securityRows(ifs.data), [ifs.data]);
  const runRows = useMemo(
    () => new Map(securityRows(running.data).map((r) => [r.name, r])),
    [running.data],
  );
  const vrfNames = useMemo(
    () => [
      'default',
      ...Object.keys(vrfs.data ?? {})
        .filter((v) => v !== 'default')
        .sort(),
    ],
    [vrfs.data],
  );
  const secSchema = useMemo(
    () => localize(withChoices(schemas.security(), { 'adl.allowVrf': vrfNames }), tr, 'form'),
    [t, vrfNames],
  );
  const sdlSchema = useMemo(() => localize(schemas.autoSdl(), tr, 'form.autoSdl'), [t]);

  const current = editing ? ifs.data?.[editing] : undefined;

  const saveSecurity = async (value: unknown) => {
    if (!editing) return;
    try {
      await patchIfs.mutateAsync({
        [editing]: securityPatch(value as { urpf?: UrpfConfig; adl?: AdlConfig }),
      });
      setEditing(null);
    } catch {
      // pointers are mapped onto the form
    }
  };

  const pending = (name: string, urpf: UrpfConfig | undefined, adl: AdlConfig | undefined) => {
    const r = runRows.get(name);
    return (
      JSON.stringify(r?.urpf ?? null) !== JSON.stringify(urpf ?? null) ||
      JSON.stringify(r?.adl ?? null) !== JSON.stringify(adl ?? null)
    );
  };
  const base = editing ? `/interfaces/${esc(editing)}` : '';

  return (
    <PageHeader title={t('adl.title')}>
      <Typography color="text.secondary" sx={{ mb: 2 }}>
        {t('adl.intro')}
      </Typography>
      {(ifs.isPending || services.isPending) && <LinearProgress aria-label={t('config:loading')} />}
      {ifs.isError && <ProblemAlert error={ifs.error} sx={{ mb: 1 }} />}
      {!editing && patchIfs.isError && <ProblemAlert error={patchIfs.error} sx={{ mb: 1 }} />}

      <Typography component="h3" variant="h6" sx={{ mb: 1 }}>
        {t('adl.interfaces')}
      </Typography>
      <TableContainer component={Paper} variant="outlined" sx={{ mb: 1 }}>
        <Table size="small" aria-label={t('adl.interfaces')}>
          <TableHead>
            <TableRow>
              <TableCell>{t('col.interface')}</TableCell>
              <TableCell>{t('col.urpf')}</TableCell>
              <TableCell>{t('col.adl')}</TableCell>
              <TableCell>{t('col.pending')}</TableCell>
              <TableCell sx={{ textAlign: 'end' }}>{t('col.actions')}</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {rows.length === 0 && ifs.isSuccess && (
              <TableRow>
                <TableCell colSpan={5}>
                  <Typography color="text.secondary">{t('adl.noInterfaces')}</Typography>
                </TableCell>
              </TableRow>
            )}
            {rows.map((r) => (
              <TableRow key={r.name} data-testid={`sec-${r.name}`}>
                <TableCell>
                  <Box component="span" dir="ltr" sx={MONO}>
                    {r.name}
                  </Box>
                </TableCell>
                <TableCell>{urpfText(r.urpf, tr)}</TableCell>
                <TableCell>{adlText(r.adl, tr)}</TableCell>
                <TableCell>
                  {pending(r.name, r.urpf, r.adl) && (
                    <Chip size="small" color="warning" label={t('pending.changed')} />
                  )}
                </TableCell>
                <TableCell sx={{ textAlign: 'end' }}>
                  <IconButton
                    size="small"
                    aria-label={t('adl.edit', { name: r.name })}
                    disabled={readOnly}
                    onClick={() => {
                      patchIfs.reset();
                      setEditing(r.name);
                    }}
                  >
                    <EditIcon fontSize="small" />
                  </IconButton>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </TableContainer>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
        {t('adl.note')}
      </Typography>

      <Typography component="h3" variant="h6" sx={{ mb: 1 }}>
        {t('autoSdl.title')}
      </Typography>
      <Alert severity="info" sx={{ mb: 2 }}>
        {t('autoSdl.globals')}
      </Alert>
      {services.isSuccess && (
        <Paper variant="outlined" sx={{ p: 2, maxWidth: 560 }}>
          <SchemaForm
            id="autosdl-form"
            schema={sdlSchema}
            value={services.data?.autoSdl ?? undefined}
            readOnly={readOnly}
            onSubmit={async (v) => {
              await patchSvc.mutateAsync({ autoSdl: v }).catch(() => undefined);
            }}
            problem={patchSvc.error ? problemUnder(patchSvc.error, AUTO_SDL_POINTER) : null}
            submitLabel={t('saveToCandidate')}
          />
        </Paper>
      )}

      <Dialog
        open={editing !== null}
        onClose={patchIfs.isPending ? undefined : () => setEditing(null)}
        maxWidth="sm"
        fullWidth
        aria-labelledby="sec-title"
      >
        <DialogTitle id="sec-title">{t('adl.editTitle', { name: editing ?? '' })}</DialogTitle>
        <DialogContent dividers>
          <Alert severity="info" sx={{ mb: 2 }}>
            {t('adl.physicalOnly')}
          </Alert>
          {editing && (
            <SchemaForm
              id="sec-form"
              schema={secSchema}
              value={{
                ...(current?.urpf ? { urpf: current.urpf } : {}),
                ...(current?.adl ? { adl: current.adl } : {}),
              }}
              onSubmit={saveSecurity}
              problem={patchIfs.error ? problemUnder(patchIfs.error, base) : null}
              hideActions
            />
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setEditing(null)} disabled={patchIfs.isPending}>
            {t('config:cancel')}
          </Button>
          <Button type="submit" form="sec-form" variant="contained" disabled={patchIfs.isPending}>
            {t('saveToCandidate')}
          </Button>
        </DialogActions>
      </Dialog>
    </PageHeader>
  );
}
