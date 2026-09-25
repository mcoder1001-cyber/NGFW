import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/Delete';
import RefreshIcon from '@mui/icons-material/Refresh';
import Alert from '@mui/material/Alert';
import AlertTitle from '@mui/material/AlertTitle';
import Box from '@mui/material/Box';
import Breadcrumbs from '@mui/material/Breadcrumbs';
import Button from '@mui/material/Button';
import Chip from '@mui/material/Chip';
import CircularProgress from '@mui/material/CircularProgress';
import Divider from '@mui/material/Divider';
import IconButton from '@mui/material/IconButton';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import { humanize, SchemaForm, type JsonSchema, type ProblemDetails } from '@ngfw/ui-kit/schema-form';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link as RouterLink, useNavigate, useParams } from 'react-router';
import { usePermissions } from '../../auth/AuthProvider';
import { ApiError } from '../../api-problem';
import { useDiff } from '../../config/queries';
import { useConfirmState } from '../../config/confirm-store';
import { ProblemAlert } from '../../config/ProblemAlert';
import { domainByKey, domainSchemas, domains, ROOT_KEYS, type RootKey } from '../../schema/registry';
import { PageHeader } from '../../shell/PageHeader';
import { usePatchDomain, useDomainCandidate } from './queries';
import {
  childPropertyKeys,
  configPathTo,
  createMergePatch,
  parseConfigPath,
  resolveNode,
  valueAt,
  withoutChildProperties,
  withoutChildValues,
  wrapAtPath,
  type ResolvedNode,
} from './schemaPath';
import { absolutePointer, inSubtree, notAppliedForDomain } from './subtree';

function isPlainObject(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null && !Array.isArray(v);
}

/** A record key is an identifier (interface/VRF/object name, …), never rendered right-to-left. */
const LTR_INPUT = { dir: 'ltr' } as const;

/** Server pointers under `base` (RFC 6901) mapped onto `<SchemaForm>` field paths relative to the edited node. */
function problemForNode(error: unknown, base: string): ProblemDetails | null {
  if (!(error instanceof ApiError)) return null;
  const p = error.toFormProblem();
  return {
    ...p,
    errors: (p.errors ?? []).map((e) => ({ ...e, pointer: e.pointer.startsWith(base) ? e.pointer.slice(base.length) : e.pointer })),
  };
}

/** One crumb of the breadcrumb: `segments.slice(0, depth)`, resolved against the domain schema. */
interface Crumb {
  depth: number;
  label: string;
}

function breadcrumbTrail(domainTitle: string, domainSchema: JsonSchema, segments: readonly string[]): Crumb[] {
  const crumbs: Crumb[] = [{ depth: 0, label: domainTitle }];
  let parent: ResolvedNode | null = resolveNode(domainSchema, []);
  for (let depth = 1; depth <= segments.length && parent; depth++) {
    const seg = segments[depth - 1]!;
    const here = resolveNode(domainSchema, segments.slice(0, depth));
    const label = parent.record ? seg : (here?.schema.title ?? humanize(seg));
    crumbs.push({ depth, label });
    parent = here;
  }
  return crumbs;
}

/** `/config/<domain>[/<segment>...]`: a generic SchemaForm for any JSON-pointer subtree of the candidate (D-125). */
export function AdvancedEditorPage() {
  const { t } = useTranslation(['advanced', 'nav']);
  const params = useParams();
  const navigate = useNavigate();
  const perms = usePermissions();
  const readOnly = !perms.editConfig;

  const cfgPath = useMemo(() => parseConfigPath(params['*'], ROOT_KEYS), [params]);
  const domainKey = cfgPath?.domainKey as RootKey | undefined;
  const segments = useMemo(() => cfgPath?.segments ?? [], [cfgPath]);
  const domainSchema = domainKey ? domainSchemas[domainKey] : undefined;
  const node = useMemo(() => (domainSchema ? resolveNode(domainSchema, segments) : null), [domainSchema, segments]);

  const candidate = useDomainCandidate((domainKey ?? 'system') as RootKey, domainKey !== undefined);
  const patch = usePatchDomain((domainKey ?? 'system') as RootKey);
  const diffQuery = useDiff();
  const confirmState = useConfirmState();

  const [newKey, setNewKey] = useState('');
  const [saved, setSaved] = useState(false);

  if (!cfgPath || !domainKey || !domainSchema) {
    return (
      <PageHeader title={t('notFound.title')}>
        <Alert severity="warning" sx={{ mb: 2, maxInlineSize: 720 }}>
          {t('notFound.badDomain', { path: params['*'] ?? '' })}
        </Alert>
        <Box component="ul" sx={{ ps: 2 }}>
          {domains.map((d) => (
            <li key={d.key}>
              <Button component={RouterLink} to={`/${configPathTo(d.key)}`}>
                {t(`nav:domains.${d.key}`, { defaultValue: d.title })}
              </Button>
            </li>
          ))}
        </Box>
      </PageHeader>
    );
  }

  const domainTitle = t(`nav:domains.${domainKey}`, { defaultValue: domainByKey(domainKey)?.title ?? domainKey });
  const pointer = absolutePointer(domainKey, segments);
  const parentSegments = segments.slice(0, -1);

  if (!node) {
    return (
      <PageHeader title={domainTitle}>
        <Alert severity="warning" sx={{ mb: 2, maxInlineSize: 720 }}>
          {t('notFound.badPath', { path: pointer })}
        </Alert>
        <Button component={RouterLink} to={`/${configPathTo(domainKey, parentSegments)}`} variant="outlined">
          {t('actions.back')}
        </Button>
      </PageHeader>
    );
  }

  const candidateDomainValue = candidate.data;
  const candidateNodeValue = valueAt(candidateDomainValue, segments);
  const changesHere = inSubtree(diffQuery.data?.changes ?? [], pointer);
  const summary = confirmState.tracked?.summary ?? confirmState.outcome?.summary;
  const notAppliedHere = summary ? notAppliedForDomain(summary.notApplied, domainKey) : false;
  const warningsHere = summary ? inSubtree(summary.warnings, pointer) : [];

  // Child-container members (nested objects/records) are their own tree node, never part of this node's own
  // `<SchemaForm>` — so the diff base for a save must drop them too, or their absence from the form's value would
  // merge-patch them to `null` (schemaPath.test.ts "withoutChildValues"). Memoised: `<SchemaForm schema>` resets the
  // form to its initial value whenever the schema's *identity* changes (SchemaForm.tsx), so a fresh object here on
  // every render (e.g. from the WS status tick re-rendering this page) would silently wipe an unsaved edit.
  const childKeys = useMemo(() => (node.record ? [] : childPropertyKeys(node.schema, domainSchema)), [node, domainSchema]);
  const formSchema = useMemo(() => (node.record ? node.schema : withoutChildProperties(node.schema, childKeys)), [node, childKeys]);

  const save = async (value: unknown) => {
    setSaved(false);
    const base = withoutChildValues(candidateNodeValue, childKeys);
    const body = base === undefined ? value : createMergePatch(base, value);
    await patch.mutateAsync(wrapAtPath(segments, body));
    setSaved(true);
  };

  const removeAt = async (atSegments: readonly string[]) => {
    setSaved(false);
    await patch.mutateAsync(wrapAtPath(atSegments, null));
  };

  const goto = (extra: readonly string[]) => navigate(`/${configPathTo(domainKey, [...segments, ...extra])}`);

  const crumbs = breadcrumbTrail(domainTitle, domainSchema, segments);

  return (
    <PageHeader title={domainTitle}>
      <Breadcrumbs sx={{ mb: 2 }} aria-label={t('breadcrumb.label')}>
        {crumbs.map((c, i) =>
          i === crumbs.length - 1 ? (
            <Typography key={c.depth} color="text.primary" dir="auto">
              {c.label}
            </Typography>
          ) : (
            <Typography
              key={c.depth}
              component={RouterLink}
              to={`/${configPathTo(domainKey, segments.slice(0, c.depth))}`}
              dir="auto"
              sx={{ color: 'inherit', textDecoration: 'none', '&:hover': { textDecoration: 'underline' } }}
            >
              {c.label}
            </Typography>
          ),
        )}
      </Breadcrumbs>

      <Stack direction="row" gap={1} sx={{ mb: 2 }} flexWrap="wrap">
        <Button size="small" startIcon={<RefreshIcon />} onClick={() => void candidate.refetch()}>
          {t('actions.refresh')}
        </Button>
        {readOnly && <Chip size="small" color="default" label={t('readOnly')} />}
      </Stack>

      {notAppliedHere && (
        <Alert severity="warning" sx={{ mb: 2 }}>
          {t('warnings.notAppliedDomain', { domain: domainTitle })}
        </Alert>
      )}
      {warningsHere.length > 0 && (
        <Alert severity="info" sx={{ mb: 2 }}>
          <AlertTitle>{t('warnings.title')}</AlertTitle>
          <Box component="ul" sx={{ m: 0, ps: 2 }}>
            {warningsHere.map((w, i) => (
              <li key={`${w.pointer}:${i}`} dir="auto">
                <Box component="code" dir="ltr" sx={{ fontFamily: (th) => th.vrx.monoFontFamily }}>
                  {w.pointer}
                </Box>
                {` — ${w.message}`}
              </li>
            ))}
          </Box>
        </Alert>
      )}

      <Typography variant="subtitle2" gutterBottom>
        {t('diff.title')}
      </Typography>
      {changesHere.length === 0 ? (
        <Typography color="text.secondary" gutterBottom>
          {t('diff.none')}
        </Typography>
      ) : (
        <Box component="ul" sx={{ mt: 0, ps: 2, mb: 2 }}>
          {changesHere.map((c, i) => (
            <li key={`${c.pointer}:${i}`} dir="auto">
              <Box component="code" dir="ltr" sx={{ fontFamily: (th) => th.vrx.monoFontFamily }}>
                {c.pointer || '/'}
              </Box>
              {`: ${t(`diff.op.${c.op}`)}`}
              {c.redacted && ` (${t('diff.redacted')})`}
            </li>
          ))}
        </Box>
      )}

      <Divider sx={{ mb: 2 }} />

      {candidate.isPending && <CircularProgress aria-label={t('loading')} />}
      {candidate.isError && <ProblemAlert error={candidate.error} sx={{ mb: 2 }} />}

      {candidate.isSuccess && node.record && (
        <RecordChildren
          entries={isPlainObject(candidateNodeValue) ? Object.keys(candidateNodeValue).sort() : []}
          readOnly={readOnly}
          newKey={newKey}
          onNewKeyChange={setNewKey}
          onAdd={() => {
            const key = newKey.trim();
            if (key === '') return;
            setNewKey('');
            goto([key]);
          }}
          onOpen={(key) => goto([key])}
          onRemove={(key) => void removeAt([...segments, key])}
        />
      )}

      {candidate.isSuccess && !node.record && (
        <NodeEditor
          key={pointer}
          formSchema={formSchema}
          childKeys={childKeys}
          domainSchema={domainSchema}
          segments={segments}
          value={candidateNodeValue}
          pointer={pointer}
          readOnly={readOnly}
          error={patch.error}
          saved={saved}
          canRemove={segments.length > 0 && candidateNodeValue !== undefined}
          onOpenChild={(key) => goto([key])}
          onSave={(value) => void save(value)}
          onRemove={() => {
            void removeAt(segments).then(() => navigate(`/${configPathTo(domainKey, parentSegments)}`));
          }}
        />
      )}
    </PageHeader>
  );
}

function RecordChildren({
  entries,
  readOnly,
  newKey,
  onNewKeyChange,
  onAdd,
  onOpen,
  onRemove,
}: {
  entries: readonly string[];
  readOnly: boolean;
  newKey: string;
  onNewKeyChange: (v: string) => void;
  onAdd: () => void;
  onOpen: (key: string) => void;
  onRemove: (key: string) => void;
}) {
  const { t } = useTranslation('advanced');
  return (
    <Stack gap={1}>
      {entries.length === 0 && <Typography color="text.secondary">{t('record.empty')}</Typography>}
      {entries.map((key) => (
        <Paper key={key} variant="outlined" sx={{ p: 0.5, display: 'flex', alignItems: 'center', gap: 1 }}>
          <Button
            onClick={() => onOpen(key)}
            sx={{ flex: 1, justifyContent: 'flex-start', textTransform: 'none' }}
          >
            <Box component="code" dir="ltr" sx={{ fontFamily: (th) => th.vrx.monoFontFamily }}>
              {key}
            </Box>
          </Button>
          <IconButton
            aria-label={t('record.remove', { key })}
            disabled={readOnly}
            onClick={() => onRemove(key)}
          >
            <DeleteIcon />
          </IconButton>
        </Paper>
      ))}
      <Stack direction="row" gap={1} alignItems="center" sx={{ mt: 1 }}>
        <TextField
          size="small"
          label={t('addChild.label')}
          placeholder={t('addChild.placeholder')}
          value={newKey}
          onChange={(e) => onNewKeyChange(e.target.value)}
          disabled={readOnly}
          slotProps={{ htmlInput: LTR_INPUT }}
        />
        <Button startIcon={<AddIcon />} onClick={onAdd} disabled={readOnly || newKey.trim() === ''}>
          {t('actions.add')}
        </Button>
      </Stack>
    </Stack>
  );
}

function NodeEditor({
  formSchema,
  childKeys,
  domainSchema,
  segments,
  value,
  pointer,
  readOnly,
  error,
  saved,
  canRemove,
  onOpenChild,
  onSave,
  onRemove,
}: {
  formSchema: JsonSchema;
  childKeys: readonly string[];
  domainSchema: JsonSchema;
  segments: readonly string[];
  value: unknown;
  pointer: string;
  readOnly: boolean;
  error: unknown;
  saved: boolean;
  canRemove: boolean;
  onOpenChild: (key: string) => void;
  onSave: (value: unknown) => void;
  onRemove: () => void;
}) {
  const { t } = useTranslation('advanced');
  const hasForm = Object.keys(formSchema.properties ?? {}).length > 0 || formSchema.type !== 'object';

  return (
    <Stack gap={2}>
      {childKeys.length > 0 && (
        <Box>
          <Typography variant="subtitle2" gutterBottom>
            {t('children.title')}
          </Typography>
          <Stack direction="row" gap={1} flexWrap="wrap">
            {childKeys.map((key) => {
              const set = valueAt(value, [key]) !== undefined;
              const label = resolveNode(domainSchema, [...segments, key])?.schema.title ?? humanize(key);
              return (
                <Chip
                  key={key}
                  label={label}
                  color={set ? 'primary' : 'default'}
                  variant={set ? 'filled' : 'outlined'}
                  onClick={() => onOpenChild(key)}
                />
              );
            })}
          </Stack>
        </Box>
      )}

      {error !== undefined && error !== null && <ProblemAlert error={error} />}
      {saved && !error && (
        <Alert severity="success" data-testid="saved">
          {t('saved')}
        </Alert>
      )}

      {hasForm && (
        <SchemaForm
          schema={formSchema}
          value={withoutChildValues(value, childKeys)}
          readOnly={readOnly}
          submitLabel={t('actions.save')}
          resetLabel={t('actions.reset')}
          problem={problemForNode(error, pointer)}
          onSubmit={onSave}
        >
          {canRemove && (
            <Button color="error" variant="outlined" startIcon={<DeleteIcon />} disabled={readOnly} onClick={onRemove}>
              {t('actions.remove')}
            </Button>
          )}
        </SchemaForm>
      )}
      {!hasForm && canRemove && (
        <Button color="error" variant="outlined" startIcon={<DeleteIcon />} disabled={readOnly} onClick={onRemove} sx={{ alignSelf: 'flex-start' }}>
          {t('actions.remove')}
        </Button>
      )}
    </Stack>
  );
}
