import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/Delete';
import EditIcon from '@mui/icons-material/Edit';
import Button from '@mui/material/Button';
import Chip from '@mui/material/Chip';
import Dialog from '@mui/material/Dialog';
import DialogContent from '@mui/material/DialogContent';
import DialogContentText from '@mui/material/DialogContentText';
import DialogTitle from '@mui/material/DialogTitle';
import IconButton from '@mui/material/IconButton';
import MenuItem from '@mui/material/MenuItem';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import TextField from '@mui/material/TextField';
import Tooltip from '@mui/material/Tooltip';
import Typography from '@mui/material/Typography';
import { StatusChip } from '@ngfw/ui-kit';
import { SchemaForm } from '@ngfw/ui-kit/schema-form';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { PageHeader } from '../../../shell/PageHeader';
import { problemFor } from '../InterfaceDrawer';
import { localizeSchema } from '../model';
import { useCandidateInterfaces, useInterfacesState, usePatchInterfaces } from '../queries';
import {
  allNames,
  END_CELL,
  formSchemas,
  localizeAll,
  mirrorPatch,
  MONO,
  parentNames,
  presence,
  sessionsOf,
  withSession,
  type MirrorSessionConfig,
  type SessionRow,
} from './model';

const NS = 'loopback-bvi-gso-lldp-span';
const LTR = { dir: 'ltr' } as const;
const esc = (s: string) => s.replace(/~/g, '~0').replace(/\//g, '~1');

type Editing = { source: string | null; index: number } | null;

/**
 * Port mirroring (F-loopback-bvi-gso-lldp-span): every session of `interfaces.<source>.mirror` in the candidate, its live
 * status from `/state/interfaces` (the agent's Retrieve view = what VPP mirrors) and the add / edit / remove dialog,
 * saved as a merge patch of `/config/interfaces` (the generic route). ERSPAN = a GRE tunnel of type erspan as the
 * destination.
 */
export function MirroringPage() {
  const { t } = useTranslation(NS);
  const perms = usePermissions();
  const ifs = useCandidateInterfaces();
  const state = useInterfacesState();
  const patch = usePatchInterfaces();
  const [editing, setEditing] = useState<Editing>(null);
  const rows = sessionsOf(ifs.data, state.data?.items);

  const remove = (r: SessionRow) =>
    void patch
      .mutateAsync(mirrorPatch(r.source, withSession(ifs.data?.[r.source]?.mirror, r.index, null)))
      .catch(() => undefined);

  return (
    <PageHeader title={t('mirror.title')}>
      <Typography color="text.secondary" sx={{ mb: 2 }}>
        {t('mirror.intro')}
      </Typography>
      <Tooltip title={perms.editConfig ? '' : t('readonly')}>
        <span>
          <Button
            variant="contained"
            startIcon={<AddIcon />}
            sx={{ mb: 1 }}
            disabled={!perms.editConfig}
            onClick={() => setEditing({ source: null, index: -1 })}
          >
            {t('mirror.add')}
          </Button>
        </span>
      </Tooltip>
      {patch.isError && editing === null && <ProblemAlert error={patch.error} sx={{ mb: 1 }} />}
      {rows.length === 0 ? (
        <Typography color="text.secondary">{t('mirror.none')}</Typography>
      ) : (
        <Paper variant="outlined">
          <Table size="small" aria-label={t('mirror.title')}>
            <TableHead>
              <TableRow>
                <TableCell>{t('mirror.col.source')}</TableCell>
                <TableCell>{t('mirror.col.destination')}</TableCell>
                <TableCell>{t('mirror.col.direction')}</TableCell>
                <TableCell>{t('mirror.col.level')}</TableCell>
                <TableCell>{t('mirror.col.status')}</TableCell>
                <TableCell />
              </TableRow>
            </TableHead>
            <TableBody>
              {rows.map((r) => (
                <TableRow key={r.id}>
                  <TableCell dir="ltr" sx={MONO}>
                    {r.source}
                  </TableCell>
                  <TableCell dir="ltr" sx={MONO}>
                    {r.destination}
                  </TableCell>
                  <TableCell>{t(`direction.${r.direction}`)}</TableCell>
                  <TableCell>{t(`level.${r.level}`)}</TableCell>
                  <TableCell>
                    <Stack direction="row" gap={0.5}>
                      <StatusChip
                        size="small"
                        status={presence(r.active)}
                        label={r.active ? t('mirror.active') : t('mirror.missing')}
                      />
                      {r.pending && (
                        <Chip
                          size="small"
                          color="warning"
                          variant="outlined"
                          label={t('pending')}
                        />
                      )}
                    </Stack>
                  </TableCell>
                  <TableCell sx={END_CELL}>
                    <IconButton
                      size="small"
                      aria-label={`${t('edit')} ${r.source} → ${r.destination}`}
                      disabled={!perms.editConfig}
                      onClick={() => setEditing({ source: r.source, index: r.index })}
                    >
                      <EditIcon fontSize="small" />
                    </IconButton>
                    <IconButton
                      size="small"
                      aria-label={`${t('remove')} ${r.source} → ${r.destination}`}
                      disabled={!perms.editConfig || patch.isPending}
                      onClick={() => remove(r)}
                    >
                      <DeleteIcon fontSize="small" />
                    </IconButton>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Paper>
      )}
      {editing !== null && (
        <SessionDialog
          editing={editing}
          sources={parentNames(ifs.data)}
          destinations={allNames(ifs.data)}
          sessionsOf={(src) => ifs.data?.[src]?.mirror}
          onClose={() => setEditing(null)}
        />
      )}
    </PageHeader>
  );
}

/** Add / edit one session: the source is picked (fixed when editing), the session is the schema-driven form. */
function SessionDialog({
  editing,
  sources,
  destinations,
  sessionsOf: sessionsOfSource,
  onClose,
}: {
  editing: NonNullable<Editing>;
  sources: string[];
  destinations: string[];
  sessionsOf: (source: string) => MirrorSessionConfig[] | undefined;
  onClose: () => void;
}) {
  const { t } = useTranslation(NS);
  const perms = usePermissions();
  const patch = usePatchInterfaces();
  const [source, setSource] = useState(editing.source ?? '');
  const schema = useMemo(() => formSchemas.mirror(), []);
  const sessions = source ? sessionsOfSource(source) : undefined;
  const index = editing.index >= 0 ? editing.index : (sessions?.length ?? 0);
  const current = editing.index >= 0 ? sessions?.[editing.index] : undefined;
  const pointer = `/interfaces/${esc(source)}/mirror/${index}`;
  const problem = problemFor(patch.error, pointer);
  return (
    <Dialog open onClose={onClose} fullWidth maxWidth="sm">
      <DialogTitle>{editing.index >= 0 ? t('mirror.editTitle') : t('mirror.addTitle')}</DialogTitle>
      <DialogContent>
        <Stack gap={2} sx={{ pt: 1 }}>
          <DialogContentText>{t('mirror.help')}</DialogContentText>
          <TextField
            select
            label={t('mirror.col.source')}
            value={source}
            disabled={editing.source !== null}
            onChange={(e) => setSource(e.target.value)}
            helperText={t('mirror.sourceHelp')}
            slotProps={{ htmlInput: LTR }}
          >
            {sources.map((o) => (
              <MenuItem key={o} value={o} dir="ltr">
                {o}
              </MenuItem>
            ))}
          </TextField>
          {patch.isError && problem === null && <ProblemAlert error={patch.error} />}
          <SchemaForm
            schema={localizeAll(schema, (k, o) => t(k, o ?? {}), localizeSchema)}
            value={current}
            readOnly={!perms.editConfig}
            interfaceOptions={destinations.filter((d) => d !== source)}
            problem={problem}
            submitLabel={t('save')}
            resetLabel={t('reset')}
            onSubmit={(v) => {
              if (!source || patch.isPending) return;
              const next = withSession(sessions, editing.index, v as MirrorSessionConfig);
              void patch
                .mutateAsync(mirrorPatch(source, next))
                .then(onClose)
                .catch(() => undefined);
            }}
          >
            <Button onClick={onClose}>{t('cancel')}</Button>
          </SchemaForm>
        </Stack>
      </DialogContent>
    </Dialog>
  );
}
