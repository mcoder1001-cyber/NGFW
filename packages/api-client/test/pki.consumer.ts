// PKI consumer coverage against the built generated client; kept separate from the shared SDK probes.
import { createApiClient } from '@ngfw/api-client';
const api = createApiClient('');

export async function pkiConsumer(): Promise<void> {
  const csr = await api.POST('/api/v1/actions/pki/csr', {
    body: { name: 'server', subject: 'CN=server.example.test' },
  });
  if (csr.data) {
    const publicRequest: string = csr.data.csrPem;
    const reference: string = csr.data.keyRef;
    const algorithm: 'ecdsa' | 'rsa' = csr.data.csr.keySpec.type;
    void [publicRequest, reference, algorithm];
    // @ts-expect-error — private material is absent from the output contract.
    void csr.data.privateKeyPem;
  }
  const imported = await api.POST('/api/v1/actions/pki/import', {
    body: { format: 'pem', as: 'ca', name: 'ca', certificatePem: 'public-certificate' },
  });
  if (imported.data) {
    const signingKey: string | null = imported.data.keyRef;
    const fingerprint: string = imported.data.issued.fingerprint;
    const staged: boolean = imported.data.staged;
    void [signingKey, fingerprint, staged];
    // @ts-expect-error — write-only passphrase is never an import response field.
    void imported.data.passphrase;
  }
  const exported = await api.GET('/api/v1/actions/pki/export/{name}', {
    params: { path: { name: 'server' }, query: { kind: 'ca' } },
  });
  await api.GET('/api/v1/actions/pki/export/{name}', {
    params: {
      path: { name: 'server' },
      // @ts-expect-error — export selection is a bounded enum.
      query: { kind: 'unknown' },
    },
  });
  if (exported.data) {
    const pem: string = exported.data.pem;
    const kind: 'certificate' | 'ca' | 'crl' = exported.data.kind;
    void [pem, kind];
  }
  const state = await api.GET('/api/v1/state/pki');
  if (state.data) {
    const left: number | null | undefined = state.data.certificates[0]?.daysLeft;
    const ocsp: 'good' | 'revoked' | 'unknown' | 'error' | null | undefined =
      state.data.certificates[0]?.ocsp?.status;
    const fileMode: string | undefined = state.data.agentFiles.files[0]?.mode;
    void [left, ocsp, fileMode];
    // @ts-expect-error — state carries references/fingerprints, never a private key.
    void state.data.certificates[0]?.privateKeyPem;
  }
  // @ts-expect-error — CSR input requires its distinguished-name subject.
  await api.POST('/api/v1/actions/pki/csr', { body: { name: 'server' } });
  await api.POST('/api/v1/actions/pki/import', {
    // @ts-expect-error — PKCS#12 input requires its write-only passphrase.
    body: { format: 'pkcs12', as: 'certificate', name: 'server', pkcs12: 'dGVzdA==' },
  });
}
