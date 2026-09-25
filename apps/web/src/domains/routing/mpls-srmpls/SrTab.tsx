import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import type { GridColDef } from '@ngfw/ui-kit/data-grid';
import type { JsonSchema } from '@ngfw/ui-kit/schema-form';
import { useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { Mono } from './common';
import { ListEditor, type EditorRow } from './ListEditor';
import {
  itemSchema,
  keyedSchema,
  localizeSchema,
  mplsSchema,
  NS,
  segmentListsText,
  type MplsConfig,
  type MplsSrPolicyConfig,
} from './model';
import { useMpls } from './useMpls';

type PolicyRow = EditorRow & { bsid: number; lists: string; spray: string; steered: number };
type SteeringRow = EditorRow & {
  index: number;
  vrf: string;
  prefix: string;
  bsid: number;
  vpnLabel: string;
};
type Steering = MplsConfig['sr']['steering'][number];

/** Form member that carries the record key (tunnel name / binding SID). */
const KEY_FIELD = 'name';
/**
 * SR-MPLS (`routing.mpls.sr`): policies by binding SID (segment lists of labels) and the steering of IP prefixes into
 * a policy. VPP 26.06 has no SR-MPLS dump: the agent applies both write-only (re-applied once per VPP instance, D-076)
 * and never reports them back.
 */
export function SrTab() {
  const { t } = useTranslation(NS);
  const { routing, mpls, update, write } = useMpls();
  const tr = useCallback((k: string, o?: Record<string, unknown>) => t(k, o ?? {}), [t]);
  const policySchema = useMemo(() => {
    const sr = (mplsSchema().properties as Record<string, JsonSchema>)['sr'] as JsonSchema;
    const policies = (sr.properties as Record<string, JsonSchema>)['policies'] as JsonSchema;
    const keyed = keyedSchema(
      policies.additionalProperties as JsonSchema,
      {
        ...(policies.propertyNames as JsonSchema),
        title: 'Binding SID',
      } as JsonSchema,
    );
    return localizeSchema(keyed, tr, 'policy.');
  }, [tr]);
  const steeringSchema = useMemo(
    () => localizeSchema(itemSchema('sr', 'steering'), tr, 'steering.'),
    [tr],
  );
  const steering = mpls?.sr.steering ?? [];
  const policyRows: PolicyRow[] = Object.entries(mpls?.sr.policies ?? {})
    .sort(([a], [b]) => Number(a) - Number(b))
    .map(([bsid, p]) => ({
      id: bsid,
      bsid: Number(bsid),
      lists: segmentListsText(p),
      spray: p.spray ? t('yes') : t('no'),
      steered: steering.filter((s) => s.bsid === Number(bsid)).length,
    }));
  const steeringRows: SteeringRow[] = steering.map((s, index) => ({
    id: `${index}`,
    index,
    vrf: s.vrf,
    prefix: s.prefix,
    bsid: s.bsid,
    vpnLabel: s.vpnLabel === undefined ? '' : String(s.vpnLabel),
  }));
  const policyCols = useMemo<GridColDef<PolicyRow>[]>(
    () => [
      {
        field: 'bsid',
        headerName: t('sr.col.bsid'),
        type: 'number',
        width: 130,
        renderCell: (p) => <Mono>{p.row.bsid}</Mono>,
      },
      {
        field: 'lists',
        headerName: t('sr.col.lists'),
        minWidth: 260,
        flex: 1,
        renderCell: (p) => <Mono>{p.row.lists}</Mono>,
      },
      { field: 'spray', headerName: t('sr.col.spray'), width: 100 },
      { field: 'steered', headerName: t('sr.col.steered'), type: 'number', width: 120 },
    ],
    [t],
  );
  const steeringCols = useMemo<GridColDef<SteeringRow>[]>(
    () => [
      { field: 'vrf', headerName: t('bindings.col.vrf'), width: 130 },
      {
        field: 'prefix',
        headerName: t('bindings.col.prefix'),
        minWidth: 180,
        flex: 1,
        renderCell: (p) => <Mono>{p.row.prefix}</Mono>,
      },
      {
        field: 'bsid',
        headerName: t('sr.col.bsid'),
        type: 'number',
        width: 130,
        renderCell: (p) => <Mono>{p.row.bsid}</Mono>,
      },
      {
        field: 'vpnLabel',
        headerName: t('sr.col.vpnLabel'),
        width: 130,
        renderCell: (p) => <Mono>{p.row.vpnLabel}</Mono>,
      },
    ],
    [t],
  );
  return (
    <Box>
      <Alert severity="info" sx={{ mb: 2 }}>
        {t('sr.writeOnly')}
      </Alert>
      {routing.isError && <ProblemAlert error={routing.error} sx={{ mb: 1 }} />}
      <ListEditor<PolicyRow>
        title={t('sr.policies')}
        intro={t('sr.intro')}
        columns={policyCols}
        rows={policyRows}
        version={routing.dataUpdatedAt}
        schema={policySchema}
        keyField={KEY_FIELD}
        valueOf={(r) =>
          r ? { name: String(r.bsid), ...mpls?.sr.policies[String(r.bsid)] } : undefined
        }
        error={write.error}
        pending={write.isPending}
        pointerOf={(r) => `/routing/mpls/sr/policies/${r?.bsid ?? ''}`}
        addLabel={t('sr.addPolicy')}
        addTitle={t('sr.addPolicyTitle')}
        editTitle={(r) => t('sr.editPolicyTitle', { bsid: r.bsid })}
        height={260}
        onSave={(v, r) =>
          update((m) => {
            const { name, ...value } = v as MplsSrPolicyConfig & { name: string };
            const policies = { ...m.sr.policies };
            if (r && String(r.bsid) !== name) delete policies[String(r.bsid)];
            policies[name] = value;
            return { ...m, sr: { ...m.sr, policies } };
          })
        }
        onRemove={(r) =>
          update((m) => {
            const policies = { ...m.sr.policies };
            delete policies[String(r.bsid)];
            return { ...m, sr: { ...m.sr, policies } };
          })
        }
      />
      <ListEditor<SteeringRow>
        title={t('sr.steering')}
        intro={t('sr.steeringIntro')}
        columns={steeringCols}
        rows={steeringRows}
        version={routing.dataUpdatedAt}
        schema={steeringSchema}
        valueOf={(r) => (r ? steering[r.index] : undefined)}
        error={write.error}
        pending={write.isPending}
        pointerOf={(r) => `/routing/mpls/sr/steering/${r ? r.index : steering.length}`}
        addLabel={t('sr.addSteering')}
        addTitle={t('sr.addSteeringTitle')}
        editTitle={(r) => t('sr.editSteeringTitle', { prefix: r.prefix, vrf: r.vrf })}
        height={240}
        onSave={(v, r) =>
          update((m) => {
            const list = [...m.sr.steering];
            if (r) list[r.index] = v as Steering;
            else list.push(v as Steering);
            return { ...m, sr: { ...m.sr, steering: list } };
          })
        }
        onRemove={(r) =>
          update((m) => ({
            ...m,
            sr: { ...m.sr, steering: m.sr.steering.filter((_, i) => i !== r.index) },
          }))
        }
      />
    </Box>
  );
}
