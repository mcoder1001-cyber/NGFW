import 'reflect-metadata';
import { describe, expect, it, vi } from 'vitest';
import type { NgfwRequest } from '../../common/principal.js';
import { ProblemError } from '../../common/problem.js';
import { PkiController } from './pki.controller.js';
import { PkiService } from './pki.service.js';
import {
  buildCsr,
  generateKey,
  parseCsr,
  parseDn,
  selfSignedCa,
  signCsr,
  PkiError,
} from './x509.js';

describe('PKI action security boundaries', () => {
  it('converts crypto refusals into pointer-bearing validation problems', async () => {
    const service = {
      createCsr: vi.fn().mockRejectedValue(new PkiError('invalid subject', '/subject')),
    };
    const controller = new PkiController(service as unknown as PkiService);
    const req = {
      principal: { id: 1, username: 'test', role: 'admin', via: 'jwt' },
    } as NgfwRequest;
    const error = await controller
      .csr({ name: 'server', subject: 'bad', san: [], replace: false }, req)
      .catch((e: unknown) => e);
    expect(error).toBeInstanceOf(ProblemError);
    expect((error as ProblemError).body()).toMatchObject({
      status: 400,
      errors: [{ pointer: '/subject' }],
    });
    expect(req.audit).toBeUndefined();
  });

  it('audits import references without copying private key or passphrase input', async () => {
    const service = {
      import: vi.fn().mockResolvedValue({
        certificateRef: 'cert/server',
        keyRef: 'key/server',
        issued: { fingerprint: 'public-fingerprint' },
        staged: true,
      }),
    };
    const controller = new PkiController(service as unknown as PkiService);
    const req = {
      principal: { id: 1, username: 'test', role: 'admin', via: 'jwt' },
    } as NgfwRequest;
    await controller.import(
      {
        format: 'pkcs12',
        as: 'certificate',
        name: 'server',
        pkcs12: 'dGVzdA==',
        passphrase: 'NGFW_TEST_PSK_pki',
        stage: true,
        replace: false,
      },
      req,
    );
    expect(req.audit).toEqual({
      resource: 'pki/certificate/server',
      after: {
        format: 'pkcs12',
        certificateRef: 'cert/server',
        keyRef: 'key/server',
        fingerprint: 'public-fingerprint',
        staged: true,
      },
    });
    expect(JSON.stringify(req.audit)).not.toContain('NGFW_TEST_PSK_pki');
    expect(JSON.stringify(req.audit)).not.toContain('dGVzdA==');
  });

  it('rejects key export before reading configuration or secrets', async () => {
    const service = new PkiService(
      ...([{}, {}, {}, {}, {}] as unknown as ConstructorParameters<typeof PkiService>),
    );
    const error = await service.exportPem('server', 'key').catch((e: unknown) => e);
    expect(error).toBeInstanceOf(ProblemError);
    expect((error as ProblemError).getStatus()).toBe(403);
  });

  it('rejects a missing signing CA with its request pointer before reading a key', async () => {
    const datastore = {
      getRunning: vi.fn().mockResolvedValue({ doc: {} }),
      getCandidate: vi.fn().mockResolvedValue({}),
    };
    const service = new PkiService(
      ...([{}, {}, datastore, {}, {}] as unknown as ConstructorParameters<typeof PkiService>),
    );
    const error = await service
      .sign(
        { ca: 'missing', name: 'server', csrPem: 'unused', days: 365, stage: true, replace: false },
        { id: 1, username: 'test', role: 'admin', via: 'jwt' },
      )
      .catch((e: unknown) => e);
    expect(error).toBeInstanceOf(ProblemError);
    expect((error as ProblemError).body()).toMatchObject({
      status: 400,
      errors: [{ pointer: '/ca', rule: 'vpn.pki-reference-exists' }],
    });
  });

  it('rejects a not-yet-valid imported certificate before storing material', async () => {
    const service = new PkiService(
      ...([{}, {}, {}, {}, {}] as unknown as ConstructorParameters<typeof PkiService>),
    );
    const now = new Date('2030-01-01T00:00:00Z');
    service.now = () => now;
    const ca = selfSignedCa(
      parseDn('CN=Future CA'),
      generateKey({ type: 'ecdsa', curve: 'p256' }),
      365,
      new Date('2030-01-02T00:00:00Z'),
    );
    const error = await service
      .import(
        {
          format: 'pem',
          as: 'ca',
          name: 'future',
          certificatePem: ca.pem,
          stage: false,
          replace: false,
        },
        { id: 1, username: 'test', role: 'admin', via: 'jwt' },
      )
      .catch((e: unknown) => e);
    expect(error).toBeInstanceOf(ProblemError);
    expect((error as ProblemError).body()).toMatchObject({
      status: 400,
      errors: [
        { pointer: '/certificatePem', message: expect.stringContaining('not valid before') },
      ],
    });
  });

  it('rejects a cryptographically signed chain whose issuer is not a CA', async () => {
    const service = new PkiService(
      ...([{}, {}, {}, {}, {}] as unknown as ConstructorParameters<typeof PkiService>),
    );
    const rootKey = generateKey({ type: 'ecdsa', curve: 'p256' });
    const issuerKey = generateKey({ type: 'ecdsa', curve: 'p256' });
    const leafKey = generateKey({ type: 'ecdsa', curve: 'p256' });
    const root = selfSignedCa(parseDn('CN=Root'), rootKey, 365);
    const issuer = signCsr(
      parseCsr(buildCsr(parseDn('CN=Not a CA'), [], issuerKey)),
      { facts: root.facts, key: rootKey.privateKey },
      { days: 30 },
    );
    const leaf = signCsr(
      parseCsr(buildCsr(parseDn('CN=Leaf'), [], leafKey)),
      { facts: issuer.facts, key: issuerKey.privateKey },
      { days: 7 },
    );
    const error = await service
      .import(
        {
          format: 'pem',
          as: 'certificate',
          name: 'leaf',
          certificatePem: leaf.pem + issuer.pem + root.pem,
          stage: false,
          replace: false,
        },
        { id: 1, username: 'test', role: 'admin', via: 'jwt' },
      )
      .catch((e: unknown) => e);
    expect(error).toBeInstanceOf(ProblemError);
    expect((error as ProblemError).body()).toMatchObject({
      status: 400,
      errors: [{ pointer: '/certificatePem', message: expect.stringContaining('chain is broken') }],
    });
  });

  it('rejects simultaneous key PEM and key reference before reading or storing material', async () => {
    const service = new PkiService(
      ...([{}, {}, {}, {}, {}] as unknown as ConstructorParameters<typeof PkiService>),
    );
    const error = await service
      .import(
        {
          format: 'pem',
          as: 'certificate',
          name: 'server',
          certificatePem: 'unused',
          privateKeyPem: 'unused',
          privateKeyRef: 'key/other',
          stage: false,
          replace: false,
        },
        { id: 1, username: 'test', role: 'admin', via: 'jwt' },
      )
      .catch((e: unknown) => e);
    expect(error).toBeInstanceOf(ProblemError);
    expect((error as ProblemError).body()).toMatchObject({
      status: 400,
      errors: [{ pointer: '/privateKeyRef', message: expect.stringContaining('never both') }],
    });
  });
});

describe('public-only peer imports', () => {
  const principal = { id: 1, username: 'test', role: 'admin' as const, via: 'jwt' as const };
  const make = () => {
    const service = new PkiService(
      ...([{}, {}, {}, {}, {}] as unknown as ConstructorParameters<typeof PkiService>),
    );
    const internals = service as unknown as {
      refuseExisting: (refs: string[], replace: boolean) => Promise<void>;
      store: (ref: string, value: string, user: unknown, replace: boolean) => Promise<void>;
      stage: (
        user: unknown,
        pointer: string,
        patch: Record<string, unknown>,
        stage: boolean,
      ) => Promise<{ staged: boolean }>;
      alertDaysFix: () => Promise<Record<string, unknown>>;
    };
    vi.spyOn(internals, 'refuseExisting').mockResolvedValue(undefined);
    const store = vi.spyOn(internals, 'store').mockResolvedValue(undefined);
    const stage = vi.spyOn(internals, 'stage').mockResolvedValue({ staged: true });
    vi.spyOn(internals, 'alertDaysFix').mockResolvedValue({});
    return { service, store, stage };
  };
  it('stores and stages a peer leaf without creating a private key', async () => {
    const { service, store, stage } = make();
    const caKey = generateKey({ type: 'rsa', bits: 2048 });
    const ca = selfSignedCa(parseDn('CN=Root'), caKey, 365);
    const peerKey = generateKey({ type: 'rsa', bits: 2048 });
    const peer = signCsr(
      parseCsr(buildCsr(parseDn('CN=Peer'), [], peerKey)),
      { facts: ca.facts, key: caKey.privateKey },
      { days: 30 },
    );
    const got = await service.import(
      {
        format: 'pem',
        as: 'certificate',
        name: 'peer',
        certificatePem: peer.pem,
        publicOnly: true,
        stage: true,
        replace: false,
      },
      principal,
    );
    expect(got.keyRef).toBeNull();
    expect(store).toHaveBeenCalledTimes(1);
    expect(store.mock.calls[0]?.[0]).toBe('cert/peer');
    expect(stage.mock.calls[0]?.[2]).toMatchObject({
      certificateRef: 'cert/peer',
      privateKeyRef: null,
    });
  });
  it('rejects a CA as a peer leaf before writes', async () => {
    const { service, store } = make();
    const key = generateKey({ type: 'rsa', bits: 2048 });
    const ca = selfSignedCa(parseDn('CN=Root'), key, 365);
    await expect(
      service.import(
        {
          format: 'pem',
          as: 'certificate',
          name: 'peer',
          certificatePem: ca.pem,
          publicOnly: true,
          stage: true,
          replace: false,
        },
        principal,
      ),
    ).rejects.toBeInstanceOf(ProblemError);
    expect(store).not.toHaveBeenCalled();
  });
  it('rejects supplied key and CA options before crypto or writes', async () => {
    for (const extra of [{ privateKeyPem: 'unused' }, { ca: 'root' }]) {
      const { service, store } = make();
      await expect(
        service.import(
          {
            format: 'pem',
            as: 'certificate',
            name: 'peer',
            certificatePem: 'unused',
            publicOnly: true,
            stage: true,
            replace: false,
            ...extra,
          },
          principal,
        ),
      ).rejects.toBeInstanceOf(ProblemError);
      expect(store).not.toHaveBeenCalled();
    }
  });
});
