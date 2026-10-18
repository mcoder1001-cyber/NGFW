import { PgButton as Button, PgText as TextField } from '@amnafzar/common-ui-lib/forms';
import { ReactGeereedMaterial } from '@amnafzar/common-ui-lib/geereed';
import { PgDeleteIcon as DeleteIcon } from '@amnafzar/common-ui-lib/icons';
import {
  PgAlert as Alert,
  PgChip as Chip,
  PgFormControlLabel as FormControlLabel,
  PgIconButton as IconButton,
  PgLinearProgress as LinearProgress,
  PgMenuItem as MenuItem,
  PgPaper as Paper,
  PgStack as Stack,
  PgSwitch as Switch,
  PgTypography as Typography,
} from '@amnafzar/common-ui-lib/primitives';
import { useInterfaceLabel, useInterfaceName } from '@ngfw/ui-kit';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import {
  useCandidateHostStack,
  useHostStackState,
  usePutHostStack,
  type HostStackConfig,
  type HostStackRule,
} from './queries';

const NS = 'host-stack';
const DEFAULT_VRF = 'default';
const GLOBAL = 'global';
const TRANSPORTS: readonly HostStackRule['transport'][] = ['tcp', 'udp'];
const ACTIONS: readonly HostStackRule['action'][] = ['allow', 'deny'];
const END = { textAlign: 'end' } as const;
const BLOCK = { display: 'block' } as const;

function portText(p: number | undefined, any: string): string {
  return p === undefined || p === 0 ? any : String(p);
}

/** Services → Host stack (F-host-stack): session layer switch, app namespaces and the session-rules grid. */
export function HostStackTab() {
  const interfaceLabel = useInterfaceLabel();
  const interfaceName = useInterfaceName();
  const { t } = useTranslation(NS);
  const perms = usePermissions();
  const live = useHostStackState();
  const cand = useCandidateHostStack();
  const put = usePutHostStack();
  const hs: HostStackConfig = cand.data ?? {};
  const namespaces = hs.namespaces ?? {};
  const rules = hs.sessionRules ?? [];
  const ro = !perms.editConfig;

  const save = (next: HostStackConfig) => put.mutate({ ...hs, ...next });

  const [ns, setNs] = useState({ id: '', vrf: DEFAULT_VRF, iface: '' });
  const [rule, setRule] = useState<HostStackRule>({
    tag: '',
    transport: 'tcp',
    local: '',
    remote: '',
    action: 'deny',
    scope: 'global',
  });

  return (
    <Stack spacing={2}>
      <Alert severity="info" data-testid="host-stack-banner">
        {t('banner')}
      </Alert>
      {(cand.isPending || live.isPending) && <LinearProgress aria-label={t('loading')} />}
      {cand.isError && <ProblemAlert error={cand.error} />}
      {put.error !== null && <ProblemAlert error={put.error} />}

      <Paper sx={{ p: 2 }} aria-label={t('live')}>
        <Typography variant="subtitle1">{t('live')}</Typography>
        {live.isError ? (
          <>
            <Alert severity="warning">{t('stateError')}</Alert>
            <ProblemAlert error={live.error} sx={{ mt: 1 }} />
          </>
        ) : (
          live.data && (
            <Stack
              direction="row"
              spacing={1}
              alignItems="center"
              flexWrap="wrap"
              useFlexGap
              role="status"
            >
              <Chip
                color={live.data.sessionEnabled ? 'success' : 'default'}
                label={live.data.sessionEnabled ? t('sessionOn') : t('sessionOff')}
                title={live.data.sessionDetail}
              />
              <Chip
                label={t('ruleCount', {
                  own: live.data.ruleCount,
                  total: live.data.ruleCountTotal,
                })}
              />
              <Typography variant="body2">
                {t('appliedNs', { list: live.data.namespaces.join(', ') || t('none') })}
              </Typography>
            </Stack>
          )
        )}
      </Paper>

      <Paper sx={{ p: 2 }}>
        <FormControlLabel
          control={
            <Switch
              checked={hs.enabled === true}
              disabled={ro || put.isPending}
              onChange={(_, v) => save({ enabled: v })}
            />
          }
          label={t('enabled')}
        />
        <Typography variant="caption" sx={BLOCK} color="text.secondary">
          {t('enabledHelp')}
        </Typography>
      </Paper>

      <Paper sx={{ p: 2 }}>
        <Typography variant="subtitle1">{t('namespaces')}</Typography>
        {(() => {
          const nativeRows = Object.entries(namespaces).map(([id, n]) => {
            return { id, n };
          });
          return (
            <ReactGeereedMaterial
              footer={false}
              hideFilterBuilderComponent
              items={nativeRows}
              columns={[
                {
                  key: 'id',
                  headerRenderer: () => <>{t('col.id')}</>,
                  sortable: false,
                  filterable: false,
                  renderer: (_value: unknown, { id }: (typeof nativeRows)[number]) => <>{id}</>,
                },
                {
                  key: 'vrf',
                  headerRenderer: () => <>{t('col.vrf')}</>,
                  sortable: false,
                  filterable: false,
                  renderer: (_value: unknown, { n }: (typeof nativeRows)[number]) => (
                    <>{n.vrf ?? DEFAULT_VRF}</>
                  ),
                },
                {
                  key: 'interface',
                  headerRenderer: () => <>{t('col.interface')}</>,
                  sortable: false,
                  filterable: false,
                  renderer: (_value: unknown, { n }: (typeof nativeRows)[number]) => (
                    <>{n.interface ?? ''}</>
                  ),
                },
                {
                  key: 'actions',
                  headerRenderer: () => <></>,
                  sortable: false,
                  filterable: false,
                  renderer: (_value: unknown, { id }: (typeof nativeRows)[number]) => (
                    <>
                      <IconButton
                        aria-label={`${t('delete')} ${id}`}
                        disabled={ro}
                        onClick={() => {
                          const next = { ...namespaces };
                          delete next[id];
                          save({ namespaces: next });
                        }}
                      >
                        <DeleteIcon fontSize="small" />
                      </IconButton>
                    </>
                  ),
                },
              ]}
              rowKey={(item) => {
                const { id } = item as (typeof nativeRows)[number];
                return id;
              }}
              cellProps={(_item, column) => {
                switch (column.key) {
                  case 'actions':
                    return { sx: END };
                  default:
                    return {};
                }
              }}
              tableProps={{ size: 'small', 'aria-label': t('namespaces') }}
              emptyContent={<>{t('empty')}</>}
              noFormWrapper
              embedded
              memoEnable={false}
              hidePagination
              hideToolbar
              search={false}
              exportToCSV={false}
              sortable={false}
            />
          );
        })()}
        <Stack direction="row" spacing={1} sx={{ mt: 1 }} flexWrap="wrap" useFlexGap>
          <TextField
            size="small"
            label={t('col.id')}
            value={ns.id}
            onChange={(e) => setNs({ ...ns, id: e.target.value })}
          />
          <TextField
            size="small"
            label={t('col.vrf')}
            value={ns.vrf}
            onChange={(e) => setNs({ ...ns, vrf: e.target.value })}
          />
          <TextField
            size="small"
            label={t('col.interface')}
            value={interfaceLabel(ns.iface)}
            onChange={(e) => setNs({ ...ns, iface: interfaceName(e.target.value) })}
          />
          <Button
            variant="outlined"
            disabled={ro || ns.id === ''}
            onClick={() => {
              save({
                namespaces: {
                  ...namespaces,
                  [ns.id]: {
                    vrf: ns.vrf || DEFAULT_VRF,
                    ...(ns.iface ? { interface: ns.iface } : {}),
                  },
                },
              });
              setNs({ id: '', vrf: DEFAULT_VRF, iface: '' });
            }}
          >
            {t('add')}
          </Button>
        </Stack>
      </Paper>

      <Paper sx={{ p: 2 }}>
        <Typography variant="subtitle1">{t('rules')}</Typography>
        {(() => {
          const nativeRows = rules.map((r, i) => {
            return { r, i };
          });
          return (
            <ReactGeereedMaterial
              footer={false}
              hideFilterBuilderComponent
              items={nativeRows}
              columns={[
                {
                  key: 'tag',
                  headerRenderer: () => <>{t('col.tag')}</>,
                  sortable: false,
                  filterable: false,
                  renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => <>{r.tag}</>,
                },
                {
                  key: 'scope',
                  headerRenderer: () => <>{t('col.scope')}</>,
                  sortable: false,
                  filterable: false,
                  renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => (
                    <>{r.scope ?? GLOBAL}</>
                  ),
                },
                {
                  key: 'transport',
                  headerRenderer: () => <>{t('col.transport')}</>,
                  sortable: false,
                  filterable: false,
                  renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => (
                    <>{r.transport}</>
                  ),
                },
                {
                  key: 'local',
                  headerRenderer: () => <>{t('col.local')}</>,
                  sortable: false,
                  filterable: false,
                  renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => (
                    <>
                      {r.local} {t('port')} {portText(r.localPort, t('any'))}
                    </>
                  ),
                },
                {
                  key: 'remote',
                  headerRenderer: () => <>{t('col.remote')}</>,
                  sortable: false,
                  filterable: false,
                  renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => (
                    <>
                      {r.remote} {t('port')} {portText(r.remotePort, t('any'))}
                    </>
                  ),
                },
                {
                  key: 'action',
                  headerRenderer: () => <>{t('col.action')}</>,
                  sortable: false,
                  filterable: false,
                  renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => (
                    <>{r.action}</>
                  ),
                },
                {
                  key: 'namespace',
                  headerRenderer: () => <>{t('col.namespace')}</>,
                  sortable: false,
                  filterable: false,
                  renderer: (_value: unknown, { r }: (typeof nativeRows)[number]) => (
                    <>{r.appNamespace ?? ''}</>
                  ),
                },
                {
                  key: 'actions',
                  headerRenderer: () => <></>,
                  sortable: false,
                  filterable: false,
                  renderer: (_value: unknown, { r, i }: (typeof nativeRows)[number]) => (
                    <>
                      <IconButton
                        aria-label={`${t('delete')} ${r.tag}`}
                        disabled={ro}
                        onClick={() => save({ sessionRules: rules.filter((_, j) => j !== i) })}
                      >
                        <DeleteIcon fontSize="small" />
                      </IconButton>
                    </>
                  ),
                },
              ]}
              rowKey={(item) => {
                const { r } = item as (typeof nativeRows)[number];
                return r.tag;
              }}
              cellProps={(_item, column) => {
                switch (column.key) {
                  case 'actions':
                    return { sx: END };
                  default:
                    return {};
                }
              }}
              tableProps={{ size: 'small', 'aria-label': t('rules') }}
              emptyContent={<>{t('empty')}</>}
              noFormWrapper
              embedded
              memoEnable={false}
              hidePagination
              hideToolbar
              search={false}
              exportToCSV={false}
              sortable={false}
            />
          );
        })()}
        <Stack direction="row" spacing={1} sx={{ mt: 1 }} flexWrap="wrap" useFlexGap>
          <TextField
            size="small"
            label={t('col.tag')}
            value={rule.tag}
            onChange={(e) => setRule({ ...rule, tag: e.target.value })}
          />
          <TextField
            select
            size="small"
            label={t('col.transport')}
            value={rule.transport}
            onChange={(e) =>
              setRule({ ...rule, transport: e.target.value as HostStackRule['transport'] })
            }
          >
            {TRANSPORTS.map((x) => (
              <MenuItem key={x} value={x}>
                {x}
              </MenuItem>
            ))}
          </TextField>
          <TextField
            size="small"
            label={t('col.local')}
            value={rule.local}
            onChange={(e) => setRule({ ...rule, local: e.target.value })}
          />
          <TextField
            size="small"
            label={t('col.remote')}
            value={rule.remote}
            onChange={(e) => setRule({ ...rule, remote: e.target.value })}
          />
          <TextField
            select
            size="small"
            label={t('col.action')}
            value={rule.action}
            onChange={(e) =>
              setRule({ ...rule, action: e.target.value as HostStackRule['action'] })
            }
          >
            {ACTIONS.map((x) => (
              <MenuItem key={x} value={x}>
                {x}
              </MenuItem>
            ))}
          </TextField>
          <Button
            variant="outlined"
            disabled={ro || rule.tag === '' || rule.local === '' || rule.remote === ''}
            onClick={() => {
              save({ sessionRules: [...rules, rule] });
              setRule({ ...rule, tag: '' });
            }}
          >
            {t('add')}
          </Button>
        </Stack>
      </Paper>
    </Stack>
  );
}
