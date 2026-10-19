import { PgButton as Button, PgText as TextField } from '@amnafzar/common-ui-lib/forms';
import { ReactGeereedMaterial } from '@amnafzar/common-ui-lib/geereed';
import {
  PgAddIcon as AddIcon,
  PgDeleteIcon as DeleteIcon,
  PgEditIcon as EditIcon,
} from '@amnafzar/common-ui-lib/icons';
import {
  PgAlert as Alert,
  PgBox as Box,
  PgChip as Chip,
  PgDialog as Dialog,
  PgDialogActions as DialogActions,
  PgDialogContent as DialogContent,
  PgDialogTitle as DialogTitle,
  PgIconButton as IconButton,
  PgLinearProgress as LinearProgress,
  PgPaper as Paper,
  PgStack as Stack,
  PgTableContainer as TableContainer,
  PgTypography as Typography,
} from '@amnafzar/common-ui-lib/primitives';
import { useInterfaceLabel } from '@ngfw/ui-kit';
import { objectName } from '@ngfw/schema';
import { StatusChip, useFormatters } from '@ngfw/ui-kit';
import { SchemaForm } from '@ngfw/ui-kit/schema-form';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { PageHeader } from '../../../shell/PageHeader';
import {
  attachmentKey,
  localize,
  NS,
  pathText,
  policyViews,
  problemUnder,
  schemas,
  statusColour,
  withChoices,
  type PbrAttachmentConfig,
  type PbrConfig,
  type PbrPolicyConfig,
} from './model';
import { useCandidate, useFreshPbr, usePatch, usePbrState, useRunning } from './queries';

const MONO = { fontFamily: 'monospace' } as const;
const NAME_INPUT = { dir: 'ltr', 'data-testid': 'policy-name' } as const;

interface RoutingDoc {
  pbr?: PbrConfig;
}

/**
 * Routing › Policy routing (F-rpf-adl-pbr, VPP ABF): the policies of `routing.pbr` — ACL, priority, paths — with the live
 * status from GET /api/v1/state/pbr, and their interface attachments. Edits are merge patches of the candidate's
 * `routing` (the generic pointer route); the pending-change bar shows the diff and commits it.
 */
export function PbrPage() {
  const interfaceLabel = useInterfaceLabel();
  const { t } = useTranslation([NS, 'config']);
  const fmt = useFormatters();
  const perms = usePermissions();
  const routing = useCandidate<RoutingDoc>('routing');
  const running = useRunning<RoutingDoc>('routing');
  const acls = useCandidate<{ lists?: Record<string, unknown> }>('acl');
  const vrfs = useCandidate<Record<string, unknown>>('vrfs');
  const ifs =
    useCandidate<Record<string, { subinterfaces?: Record<string, unknown> }>>('interfaces');
  const state = usePbrState();
  const patch = usePatch('routing');
  const fresh = useFreshPbr();
  const [editing, setEditing] = useState<{
    name: string | null;
    value: PbrPolicyConfig | undefined;
  } | null>(null);
  const [newName, setNewName] = useState('');
  const [attaching, setAttaching] = useState(false);

  const readOnly = !perms.editConfig;
  const pbr = routing.data?.pbr;
  const views = useMemo(
    () => policyViews(pbr, state.data, running.data?.pbr),
    [pbr, state.data, running.data],
  );
  const aclNames = useMemo(() => Object.keys(acls.data?.lists ?? {}).sort(), [acls.data]);
  const vrfNames = useMemo(
    () => [
      'default',
      ...Object.keys(vrfs.data ?? {})
        .filter((v) => v !== 'default')
        .sort(),
    ],
    [vrfs.data],
  );
  const ifNames = useMemo(() => {
    const out: string[] = [];
    for (const [name, itf] of Object.entries(ifs.data ?? {})) {
      out.push(name);
      for (const id of Object.keys(itf.subinterfaces ?? {})) out.push(`${name}.${id}`);
    }
    return out.sort();
  }, [ifs.data]);
  const policyNames = useMemo(() => Object.keys(pbr?.policies ?? {}).sort(), [pbr]);

  const policySchema = useMemo(
    () =>
      localize(
        withChoices(schemas.policy(), {
          acl: aclNames,
          'paths.items.vrf': vrfNames,
        }),
        (k, o) => t(k, o ?? {}),
        'form.policy',
      ),
    [aclNames, vrfNames, t],
  );
  const attachSchema = useMemo(
    () =>
      localize(
        withChoices(schemas.attachment(), { policy: policyNames }),
        (k, o) => t(k, o ?? {}),
        'form.attachment',
      ),
    [policyNames, t],
  );
  const nameError = newName !== '' && !objectName.safeParse(newName).success;
  const liveAttach = new Map(
    (state.data?.attachments.items ?? []).map((a) => [attachmentKey(a), a]),
  );

  const savePolicy = async (value: unknown) => {
    if (!editing) return;
    const name = editing.name ?? newName;
    try {
      await patch.mutateAsync({ pbr: { policies: { [name]: value } } });
      setEditing(null);
      setNewName('');
    } catch {
      // shown by the form (pointers) or the alert
    }
  };
  const deletePolicy = async (name: string) => {
    const cur = await fresh();
    const attachments = (cur.attachments ?? []).filter((a) => a.policy !== name);
    await patch
      .mutateAsync({ pbr: { policies: { [name]: null }, attachments } })
      .catch(() => undefined);
  };
  const saveAttachment = async (value: unknown) => {
    const cur = await fresh();
    try {
      // canonical order (policy, interface, family) — the order Retrieve reports, so the drift view stays clean
      const next = [...(cur.attachments ?? []), value as PbrAttachmentConfig].sort((a, b) =>
        attachmentKey(a) < attachmentKey(b) ? -1 : attachmentKey(a) > attachmentKey(b) ? 1 : 0,
      );
      await patch.mutateAsync({ pbr: { attachments: next } });
      setAttaching(false);
    } catch {
      // shown by the form
    }
  };
  const deleteAttachment = async (key: string) => {
    const cur = await fresh();
    await patch
      .mutateAsync({
        pbr: {
          attachments: (cur.attachments ?? []).filter((a) => attachmentKey(a) !== key),
        },
      })
      .catch(() => undefined);
  };
  const editingBase = `/routing/pbr/policies/${(editing?.name ?? newName).replace(/~/g, '~0').replace(/\//g, '~1')}`;

  return (
    <PageHeader title={t('pbr.title')}>
      <Typography color="text.secondary" sx={{ mb: 2 }}>
        {t('pbr.intro')}
      </Typography>
      {state.data?.pendingChange && (
        <Alert severity="info" sx={{ mb: 2 }} data-testid="pbr-pending">
          {t('pbr.pending')}
        </Alert>
      )}
      {aclNames.length === 0 && acls.isSuccess && (
        <Alert severity="warning" sx={{ mb: 2 }}>
          {t('pbr.noAcl')}
        </Alert>
      )}
      {(routing.isPending || state.isPending) && (
        <LinearProgress aria-label={t('config:loading')} />
      )}
      {state.isError && <ProblemAlert error={state.error} sx={{ mb: 1 }} />}
      {!editing && !attaching && patch.isError && (
        <ProblemAlert error={patch.error} sx={{ mb: 1 }} />
      )}

      <Stack direction="row" alignItems="center" sx={{ mb: 1 }} gap={2}>
        <Typography component="h3" variant="h6" sx={{ flexGrow: 1 }}>
          {t('pbr.policies')}
        </Typography>
        {state.data?.retrievedAt && (
          <Typography variant="body2" color="text.secondary">
            {t('pbr.retrievedAt', {
              time: fmt.time(new Date(state.data.retrievedAt)),
            })}
          </Typography>
        )}
        <Button
          variant="contained"
          startIcon={<AddIcon />}
          disabled={readOnly}
          onClick={() => {
            patch.reset();
            setNewName('');
            setEditing({ name: null, value: undefined });
          }}
        >
          {t('pbr.addPolicy')}
        </Button>
      </Stack>
      <TableContainer component={Paper} variant="outlined" sx={{ mb: 3 }}>
        {(() => {
          const gridItems = views;
          type GridRow = (typeof gridItems)[number];
          return (
            <ReactGeereedMaterial
              hideFilterBuilderComponent
              noFormWrapper
              embedded
              memoEnable={false}
              hidePagination
              hideToolbar
              search={false}
              exportToCSV={false}
              sortable={false}
              footer={false}
              items={gridItems}
              columns={[
                {
                  key: '0',
                  sortable: false,
                  filterable: false,
                  headerRenderer: () => <>{t('col.name')}</>,
                  renderer: (_value: unknown, gridRow: GridRow) => {
                    const v = gridRow;
                    return (
                      <>
                        <Box component="span" dir="ltr" sx={MONO}>
                          {v.name}
                        </Box>
                      </>
                    );
                  },
                },
                {
                  key: '1',
                  sortable: false,
                  filterable: false,
                  headerRenderer: () => <>{t('col.acl')}</>,
                  renderer: (_value: unknown, gridRow: GridRow) => {
                    const v = gridRow;
                    const shown = v.config ?? v.live;
                    return <>{shown?.acl ?? ''}</>;
                  },
                },
                {
                  key: '2',
                  sortable: false,
                  filterable: false,
                  headerRenderer: () => <>{t('col.priority')}</>,
                  renderer: (_value: unknown, gridRow: GridRow) => {
                    const v = gridRow;
                    const shown = v.config ?? v.live;
                    return <>{fmt.integer(shown?.priority ?? 100)}</>;
                  },
                },
                {
                  key: '3',
                  sortable: false,
                  filterable: false,
                  headerRenderer: () => <>{t('col.paths')}</>,
                  renderer: (_value: unknown, gridRow: GridRow) => {
                    const v = gridRow;
                    const shown = v.config ?? v.live;
                    const paths = (shown?.paths ?? []) as {
                      address?: string;
                      interface?: string;
                      vrf?: string;
                      weight?: number;
                    }[];
                    return (
                      <>
                        <Stack>
                          {paths.map((p, i) => (
                            <Box key={i} component="span" dir="ltr" sx={MONO}>
                              {pathText({ ...p, ...(p.interface ? { interface: interfaceLabel(p.interface) } : {}) }, (k, o) => t(k, o ?? {}))}
                            </Box>
                          ))}
                        </Stack>
                      </>
                    );
                  },
                },
                {
                  key: '4',
                  sortable: false,
                  filterable: false,
                  headerRenderer: () => <>{t('col.attachments')}</>,
                  renderer: (_value: unknown, gridRow: GridRow) => {
                    const v = gridRow;
                    return (
                      <>
                        {fmt.integer(
                          (pbr?.attachments ?? []).filter((a) => a.policy === v.name).length,
                        )}
                      </>
                    );
                  },
                },
                {
                  key: '5',
                  sortable: false,
                  filterable: false,
                  headerRenderer: () => <>{t('col.status')}</>,
                  renderer: (_value: unknown, gridRow: GridRow) => {
                    const v = gridRow;
                    return (
                      <>
                        <Stack direction="row" gap={0.5} flexWrap="wrap">
                          {v.live && (
                            <StatusChip
                              status={statusColour(v.live.status)}
                              label={t(`status.${v.live.status}`)}
                            />
                          )}
                          {v.pending && (
                            <Chip
                              size="small"
                              color={v.pending === 'removed' ? 'error' : 'warning'}
                              label={t(`pending.${v.pending}`)}
                            />
                          )}
                        </Stack>
                      </>
                    );
                  },
                },
                {
                  key: '6',
                  sortable: false,
                  filterable: false,
                  headerRenderer: () => <>{t('col.actions')}</>,
                  renderer: (_value: unknown, gridRow: GridRow) => {
                    const v = gridRow;
                    return (
                      <>
                        {v.config && (
                          <>
                            <IconButton
                              size="small"
                              aria-label={t('pbr.edit', { name: v.name })}
                              disabled={readOnly}
                              onClick={() => {
                                patch.reset();
                                setEditing({ name: v.name, value: v.config });
                              }}
                            >
                              <EditIcon fontSize="small" />
                            </IconButton>
                            <IconButton
                              size="small"
                              aria-label={t('pbr.delete', { name: v.name })}
                              disabled={readOnly || patch.isPending}
                              onClick={() => void deletePolicy(v.name)}
                            >
                              <DeleteIcon fontSize="small" />
                            </IconButton>
                          </>
                        )}
                      </>
                    );
                  },
                },
              ]}
              tableProps={{ size: 'small', 'aria-label': t('pbr.policies') }}
              rowKey={(nativeRow, _gridIndex) => {
                const gridRow = nativeRow as unknown as GridRow;
                const v = gridRow;
                return v.name;
              }}
              rowProps={(nativeRow, _gridIndex) => {
                const gridRow = nativeRow as unknown as GridRow;
                const v = gridRow;
                return { role: 'row', 'data-testid': `policy-${v.name}` };
              }}
              cellProps={(_nativeRow, column, _gridIndex) => {
                switch (column.key) {
                  case '0':
                    return {};
                  case '1':
                    return { dir: 'ltr' };
                  case '2':
                    return {};
                  case '3':
                    return {};
                  case '4':
                    return {};
                  case '5':
                    return {};
                  case '6':
                    return { sx: { textAlign: 'end' } };
                  default:
                    return {};
                }
              }}
              emptyContent={
                <>
                  <Typography color="text.secondary">{t('pbr.none')}</Typography>
                </>
              }
            />
          );
        })()}
      </TableContainer>

      <Stack direction="row" alignItems="center" sx={{ mb: 1 }}>
        <Typography component="h3" variant="h6" sx={{ flexGrow: 1 }}>
          {t('pbr.attachments')}
        </Typography>
        <Button
          variant="outlined"
          startIcon={<AddIcon />}
          disabled={readOnly || policyNames.length === 0}
          onClick={() => {
            patch.reset();
            setAttaching(true);
          }}
        >
          {t('pbr.addAttachment')}
        </Button>
      </Stack>
      <TableContainer component={Paper} variant="outlined">
        {(() => {
          const gridItems = pbr?.attachments ?? [];
          type GridRow = (typeof gridItems)[number];
          return (
            <ReactGeereedMaterial
              hideFilterBuilderComponent
              noFormWrapper
              embedded
              memoEnable={false}
              hidePagination
              hideToolbar
              search={false}
              exportToCSV={false}
              sortable={false}
              footer={false}
              items={gridItems}
              columns={[
                {
                  key: '0',
                  sortable: false,
                  filterable: false,
                  headerRenderer: () => <>{t('col.policy')}</>,
                  renderer: (_value: unknown, gridRow: GridRow) => {
                    const a = gridRow;
                    return <>{a.policy}</>;
                  },
                },
                {
                  key: '1',
                  sortable: false,
                  filterable: false,
                  headerRenderer: () => <>{t('col.interface')}</>,
                  renderer: (_value: unknown, gridRow: GridRow) => {
                    const a = gridRow;
                    return (
                      <>
                        <Box component="span" dir="ltr" sx={MONO}>
                          {interfaceLabel(a.interface)}
                        </Box>
                      </>
                    );
                  },
                },
                {
                  key: '2',
                  sortable: false,
                  filterable: false,
                  headerRenderer: () => <>{t('col.family')}</>,
                  renderer: (_value: unknown, gridRow: GridRow) => {
                    const a = gridRow;
                    return <>{t(`family.${a.family ?? 'ipv4'}`)}</>;
                  },
                },
                {
                  key: '3',
                  sortable: false,
                  filterable: false,
                  headerRenderer: () => <>{t('col.status')}</>,
                  renderer: (_value: unknown, gridRow: GridRow) => {
                    const a = gridRow;
                    const key = attachmentKey(a);
                    const live = liveAttach.get(key);
                    return (
                      <>
                        {live ? (
                          <StatusChip
                            status={statusColour(live.status)}
                            label={t(`status.${live.status}`)}
                          />
                        ) : (
                          <Chip size="small" color="warning" label={t('pending.new')} />
                        )}
                      </>
                    );
                  },
                },
                {
                  key: '4',
                  sortable: false,
                  filterable: false,
                  headerRenderer: () => <>{t('col.actions')}</>,
                  renderer: (_value: unknown, gridRow: GridRow) => {
                    const a = gridRow;
                    const key = attachmentKey(a);
                    return (
                      <>
                        <IconButton
                          size="small"
                          aria-label={t('pbr.detach', {
                            policy: a.policy,
                            interface: interfaceLabel(a.interface),
                          })}
                          disabled={readOnly || patch.isPending}
                          onClick={() => void deleteAttachment(key)}
                        >
                          <DeleteIcon fontSize="small" />
                        </IconButton>
                      </>
                    );
                  },
                },
              ]}
              tableProps={{ size: 'small', 'aria-label': t('pbr.attachments') }}
              rowKey={(nativeRow, _gridIndex) => {
                const gridRow = nativeRow as unknown as GridRow;
                const a = gridRow;
                const key = attachmentKey(a);
                return key;
              }}
              rowProps={(_nativeRow, _gridIndex) => {
                return {};
              }}
              cellProps={(_nativeRow, column, _gridIndex) => {
                switch (column.key) {
                  case '0':
                    return { dir: 'ltr' };
                  case '1':
                    return {};
                  case '2':
                    return {};
                  case '3':
                    return {};
                  case '4':
                    return { sx: { textAlign: 'end' } };
                  default:
                    return {};
                }
              }}
              emptyContent={
                <>
                  <Typography color="text.secondary">{t('pbr.noAttachments')}</Typography>
                </>
              }
            />
          );
        })()}
      </TableContainer>
      <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
        {state.data?.counters.available === false ? t('pbr.noCounters') : ''}
      </Typography>

      <Dialog
        open={editing !== null}
        onClose={patch.isPending ? undefined : () => setEditing(null)}
        maxWidth="md"
        fullWidth
        aria-labelledby="pbr-policy-title"
      >
        <DialogTitle id="pbr-policy-title">
          {editing?.name ? t('pbr.editTitle', { name: editing.name }) : t('pbr.addTitle')}
        </DialogTitle>
        <DialogContent dividers>
          {editing?.name === null && (
            <TextField
              label={t('form.name.title')}
              helperText={nameError ? t('form.name.invalid') : t('form.name.help')}
              error={nameError}
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              fullWidth
              size="small"
              sx={{ mb: 2 }}
              slotProps={{ htmlInput: NAME_INPUT }}
            />
          )}
          {editing && (
            <SchemaForm
              id="pbr-policy-form"
              schema={policySchema}
              value={editing.value}
              onSubmit={savePolicy}
              interfaceOptions={ifNames}
              problem={patch.error ? problemUnder(patch.error, editingBase) : null}
              hideActions
            />
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setEditing(null)} disabled={patch.isPending}>
            {t('config:cancel')}
          </Button>
          <Button
            type="submit"
            form="pbr-policy-form"
            variant="contained"
            disabled={patch.isPending || (editing?.name === null && (newName === '' || nameError))}
          >
            {t('saveToCandidate')}
          </Button>
        </DialogActions>
      </Dialog>

      <Dialog
        open={attaching}
        onClose={patch.isPending ? undefined : () => setAttaching(false)}
        maxWidth="sm"
        fullWidth
        aria-labelledby="pbr-attach-title"
      >
        <DialogTitle id="pbr-attach-title">{t('pbr.addAttachment')}</DialogTitle>
        <DialogContent dividers>
          {attaching && (
            <SchemaForm
              id="pbr-attach-form"
              schema={attachSchema}
              onSubmit={saveAttachment}
              interfaceOptions={ifNames}
              problem={
                patch.error
                  ? problemUnder(
                      patch.error,
                      `/routing/pbr/attachments/${(pbr?.attachments ?? []).length}`,
                    )
                  : null
              }
              hideActions
            />
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setAttaching(false)} disabled={patch.isPending}>
            {t('config:cancel')}
          </Button>
          <Button
            type="submit"
            form="pbr-attach-form"
            variant="contained"
            disabled={patch.isPending}
          >
            {t('saveToCandidate')}
          </Button>
        </DialogActions>
      </Dialog>
    </PageHeader>
  );
}
