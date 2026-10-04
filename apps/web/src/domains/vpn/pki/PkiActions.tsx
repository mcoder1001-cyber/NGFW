import { useEffect, useRef, useState } from 'react';
import {
  Alert,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  MenuItem,
  Stack,
  TextField,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import { session } from '../../../auth/session';
import { api } from '../../../api';
import { call } from '../../../api-problem';

type Action = 'ca' | 'csr' | 'sign' | 'import' | 'export' | 'crl' | 'ocsp';
const actions: Action[] = ['ca', 'csr', 'sign', 'import', 'export', 'crl', 'ocsp'];

/** Sensitive inputs live only in this dialog and are cleared on close and after submission. */
export function PkiActions({ onChanged }: { onChanged: () => void }) {
  const { t } = useTranslation('pkiInventory');
  const [role, setRole] = useState(session.state.user?.role ?? null);

  const [action, setAction] = useState<Action | null>(null);
  const [name, setName] = useState('');
  const [subject, setSubject] = useState('');
  const [san, setSan] = useState('');
  const [ca, setCa] = useState('');
  const [days, setDays] = useState('365');
  const [keyType, setKeyType] = useState('p256');
  const [format, setFormat] = useState<'pem' | 'pkcs12'>('pem');
  const [kind, setKind] = useState<'ca' | 'certificate'>('certificate');
  const [pem, setPem] = useState('');
  const [key, setKey] = useState('');
  const [passphrase, setPassphrase] = useState('');
  const [pending, setPending] = useState(false);
  const [error, setError] = useState(false);
  const [result, setResult] = useState('');
  const [done, setDone] = useState(false);
  const [staged, setStaged] = useState<boolean | null>(null);
  const user = useRef(session.state.user?.id);
  const generation = useRef(0);
  useEffect(
    () =>
      session.subscribe((s) => {
        setRole(s.user?.role ?? null);
        if (s.user?.id !== user.current) {
          user.current = s.user?.id;
          generation.current += 1;
          setAction(null);
          setPem('');
          setKey('');
          setPassphrase('');
          setResult('');
          setDone(false);
          setStaged(null);
        }
      }),
    [],
  );
  function close() {
    if (pending) return;
    setAction(null);
    setName('');
    setSubject('');
    setSan('');
    setCa('');
    setPem('');
    setKey('');
    setPassphrase('');
    setResult('');
    setError(false);
    setDone(false);
    setStaged(null);
  }
  async function submit() {
    if (!action || pending) return;
    const turn = generation.current;
    setPending(true);
    setStaged(null);
    setError(false);
    setResult('');
    setDone(false);
    try {
      const keySpec = keyType.startsWith('p')
        ? { type: 'ecdsa' as const, curve: keyType as 'p256' | 'p384' }
        : { type: 'rsa' as const, bits: Number(keyType) as 2048 | 3072 | 4096 };
      const base = { name, stage: true, replace: false };
      if (action === 'ca') {
        const r = await call(
          api.POST('/api/v1/actions/pki/ca', {
            body: { ...base, subject, days: Number(days), keySpec },
          }),
        );
        if (generation.current === turn) {
          setResult(r.data.certificatePem);
          setStaged(r.data.staged);
        }
      } else if (action === 'csr') {
        const r = await call(
          api.POST('/api/v1/actions/pki/csr', {
            body: {
              name,
              subject,
              san: san
                .split(',')
                .map((s) => s.trim())
                .filter(Boolean),
              keySpec,
              replace: false,
            },
          }),
        );
        if (generation.current === turn) setResult(r.data.csrPem);
      } else if (action === 'sign') {
        const r = await call(
          api.POST('/api/v1/actions/pki/sign', {
            body: { ...base, ca, csrPem: pem, days: Number(days) },
          }),
        );
        if (generation.current === turn) {
          setResult(r.data.certificatePem);
          setStaged(r.data.staged);
        }
      } else if (action === 'import') {
        const r = await call(
          api.POST('/api/v1/actions/pki/import', {
            body:
              format === 'pem'
                ? {
                    ...base,
                    format,
                    as: kind,
                    certificatePem: pem,
                    ...(key ? { privateKeyPem: key } : {}),
                    ...(ca ? { ca } : {}),
                  }
                : { ...base, format, as: kind, pkcs12: pem, passphrase, ...(ca ? { ca } : {}) },
          }),
        );
        if (generation.current === turn) setStaged(r.data.staged);
      } else if (action === 'export') {
        const r = await call(
          api.GET('/api/v1/actions/pki/export/{name}', {
            params: { path: { name }, query: { kind } },
          }),
        );
        if (generation.current === turn) setResult(r.data.pem);
      } else if (action === 'crl') {
        await call(api.POST('/api/v1/actions/pki/crl/refresh', { body: ca ? { ca } : {} }));
      } else {
        await call(
          api.POST('/api/v1/actions/pki/ocsp/check', { body: name ? { certificate: name } : {} }),
        );
      }
      if (generation.current !== turn) return;
      setPem('');
      setKey('');
      setPassphrase('');
      setDone(true);
      onChanged();
    } catch {
      if (generation.current !== turn) return;
      // Server diagnostics may contain external material; show a safe localized error.
      setError(true);
      setPem('');
      setKey('');
      setPassphrase('');
    } finally {
      setPending(false);
    }
  }
  const needsName = action !== 'crl';
  const creation = action === 'ca' || action === 'csr';
  const valid =
    (!needsName || !!name) &&
    (!creation || !!subject) &&
    (!(action === 'sign' || action === 'import') || !!pem) &&
    (action !== 'sign' || !!ca) &&
    (action !== 'import' || format !== 'pkcs12' || !!passphrase) &&
    (!(action === 'ca' || action === 'sign') ||
      (Number.isInteger(Number(days)) && Number(days) > 0));
  return (
    <>
      <Stack direction="row" gap={1} useFlexGap flexWrap="wrap">
        {actions
          .filter(
            (a) => a === 'export' || role === 'admin' || (a === 'ocsp' && role === 'operator'),
          )
          .map((a) => (
            <Button
              key={a}
              onClick={() => {
                setAction(a);
                setDays(a === 'ca' ? '3650' : '365');
              }}
            >
              {t(`actions.${a}`)}
            </Button>
          ))}
      </Stack>
      <Dialog open={action !== null} onClose={close} fullWidth maxWidth="sm">
        <DialogTitle>{action ? t(`actions.${action}`) : ''}</DialogTitle>
        <DialogContent>
          <Stack spacing={2} sx={{ pt: 1 }}>
            {action !== 'export' && <Alert severity="info">{t('actions.staging')}</Alert>}
            {error && <Alert severity="error">{t('actions.failed')}</Alert>}
            {done && (
              <Alert severity={staged === false ? 'warning' : 'success'}>
                {t(staged === false ? 'actions.storedOnly' : 'actions.done')}
              </Alert>
            )}
            {needsName && (
              <TextField
                label={t('name')}
                value={name}
                onChange={(e) => setName(e.target.value)}
                disabled={pending}
              />
            )}
            {creation && (
              <>
                <TextField
                  label={t('subject')}
                  value={subject}
                  onChange={(e) => setSubject(e.target.value)}
                  disabled={pending}
                />
                <TextField
                  select
                  label={t('actions.keyType')}
                  value={keyType}
                  onChange={(e) => setKeyType(e.target.value)}
                  disabled={pending}
                >
                  {['p256', 'p384', '2048', '3072', '4096'].map((k) => (
                    <MenuItem key={k} value={k}>
                      {t(`actions.keys.${k}`)}
                    </MenuItem>
                  ))}
                </TextField>
              </>
            )}
            {action === 'csr' && (
              <TextField
                label={t('actions.san')}
                value={san}
                onChange={(e) => setSan(e.target.value)}
                disabled={pending}
              />
            )}
            {(action === 'ca' || action === 'sign') && (
              <TextField
                type="number"
                label={t('actions.days')}
                value={days}
                onChange={(e) => setDays(e.target.value)}
                disabled={pending}
              />
            )}
            {(action === 'sign' || action === 'import' || action === 'crl') && (
              <TextField
                label={t('actions.caName')}
                value={ca}
                onChange={(e) => setCa(e.target.value)}
                disabled={pending}
              />
            )}
            {(action === 'import' || action === 'export') && (
              <TextField
                select
                label={t('actions.kind')}
                value={kind}
                onChange={(e) => setKind(e.target.value as typeof kind)}
                disabled={pending}
              >
                <MenuItem value="certificate">{t('certificates')}</MenuItem>
                <MenuItem value="ca">{t('authorities')}</MenuItem>
              </TextField>
            )}
            {action === 'import' && (
              <TextField
                select
                label={t('actions.format')}
                value={format}
                onChange={(e) => {
                  setFormat(e.target.value as typeof format);
                  setPem('');
                  setKey('');
                  setPassphrase('');
                }}
                disabled={pending}
              >
                <MenuItem value="pem">{t('actions.pem')}</MenuItem>
                <MenuItem value="pkcs12">{t('actions.pkcs12')}</MenuItem>
              </TextField>
            )}
            {(action === 'sign' || action === 'import') && (
              <TextField
                multiline
                minRows={4}
                label={t(
                  action === 'sign'
                    ? 'actions.csrPem'
                    : format === 'pem'
                      ? 'actions.certificatePem'
                      : 'actions.pkcs12',
                )}
                value={pem}
                onChange={(e) => setPem(e.target.value)}
                disabled={pending}
              />
            )}
            {action === 'import' &&
              (format === 'pem' ? (
                <TextField
                  multiline
                  minRows={3}
                  label={t('actions.privateKey')}
                  value={key}
                  onChange={(e) => setKey(e.target.value)}
                  disabled={pending}
                  autoComplete="off"
                />
              ) : (
                <TextField
                  type="password"
                  label={t('actions.passphrase')}
                  value={passphrase}
                  onChange={(e) => setPassphrase(e.target.value)}
                  disabled={pending}
                  autoComplete="new-password"
                />
              ))}
            {result && (
              <TextField
                multiline
                minRows={6}
                label={t('actions.publicResult')}
                value={result}
                slotProps={{ input: { readOnly: true } }}
              />
            )}
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button disabled={pending} onClick={close}>
            {t('actions.close')}
          </Button>
          <Button disabled={pending || !valid || done} onClick={() => void submit()}>
            {t('actions.submit')}
          </Button>
        </DialogActions>
      </Dialog>
    </>
  );
}
