import 'reflect-metadata';
import { PkiCsrSchema } from '@ngfw/schema';
import { PgDialect } from 'drizzle-orm/pg-core';
import { describe, expect, it, vi } from 'vitest';
import type { Principal } from '../../common/principal.js';
import { openapi } from '../../common/zod.js';
import {
  PkiCaOut,
  PkiCsrOut,
  PkiSignOut,
  PkiImportOut,
  PkiExportOut,
  PkiCrlOut,
  PkiOcspOut,
  PkiStateOut,
} from './dto.js';
import { PkiController } from './pki.controller.js';
import { PkiService } from './pki.service.js';
import { openssl, scratch } from './testkit.test.js';
import { parseCsr } from './x509.js';

const user: Principal = { id: 1, username: 'pki-contract', role: 'admin', via: 'jwt' };

function service() {
  const material = new Map<string, string>();
  const pki: { cas: Record<string, unknown>; certificates: Record<string, unknown> } = {
    cas: {},
    certificates: {},
  };
  const doc = { vpn: { pki } };
  const dialect = new PgDialect();
  const db = {
    select: () => ({
      from: () => ({
        where: (sql: Parameters<PgDialect['sqlToQuery']>[0]) => {
          const refs = dialect
            .sqlToQuery(sql)
            .params.flatMap((p: unknown) => (typeof p === 'string' ? [p] : []));
          return Promise.resolve(
            refs.filter((ref) => material.has(ref)).map((ref) => ({ ref, ciphertext: ref })),
          );
        },
      }),
    }),
  };
  const secrets = {
    decrypt: (ref: string) => material.get(ref),
    put: async (kind: string, name: string, value: string) => {
      material.set(`${kind}/${name}`, value);
      return { version: 1 };
    },
  };
  const datastore = {
    getRunning: async () => ({ doc }),
    getCandidate: async () => doc,
    patchCandidate: async (_user: Principal, pointer: string, patch: unknown) => {
      const parts = pointer.split('/');
      const target = parts[3] === 'cas' ? pki.cas : pki.certificates;
      target[parts[4]!] = patch;
    },
  };
  const api = new PkiService(
    ...([
      db,
      secrets,
      datastore,
      { publish: vi.fn() },
      { state: async () => ({ error: 'agent unavailable' }) },
    ] as unknown as ConstructorParameters<typeof PkiService>),
  );
  api.now = () => new Date('2030-01-01T00:00:00Z');
  return api;
}

describe('PKI public response contracts', () => {
  it('validates real CA → CSR → sign → import → public export and populated state responses', async () => {
    const api = service();
    const ca = await api.createCa(
      { name: 'ca', subject: 'CN=CA', days: 3650, stage: true, replace: false },
      user,
    );
    expect(PkiCaOut.safeParse(ca).success).toBe(true);
    const csr = await api.createCsr(
      {
        name: 'server',
        subject: 'CN=server.example.test',
        san: ['server.example.test'],
        replace: false,
      },
      user,
    );
    expect(PkiCsrOut.safeParse(csr).success).toBe(true);
    const signed = await api.sign(
      { ca: 'ca', name: 'server', csrPem: csr.csrPem, days: 365, stage: true, replace: false },
      user,
    );
    expect(PkiSignOut.safeParse(signed).success).toBe(true);
    const imported = await api.import(
      {
        format: 'pem',
        as: 'certificate',
        name: 'imported',
        certificatePem: signed.certificatePem + ca.certificatePem,
        privateKeyRef: 'key/server',
        ca: 'ca',
        stage: true,
        replace: false,
      },
      user,
    );
    expect(PkiImportOut.safeParse(imported).success).toBe(true);
    const importedCa = await api.import(
      {
        format: 'pem',
        as: 'ca',
        name: 'external',
        certificatePem: ca.certificatePem,
        stage: false,
        replace: false,
      },
      user,
    );
    expect(PkiImportOut.parse(importedCa).keyRef).toBeNull();
    const exported = await api.exportPem('imported', 'certificate');
    expect(PkiExportOut.parse(exported).pem).toBe(signed.certificatePem + ca.certificatePem);
    const state = await api.state();
    const validated = PkiStateOut.parse(state);
    expect(validated.cas[0]?.signingKey).toBe(true);
    expect(validated.certificates.map((c) => c.name)).toEqual(['imported', 'server']);
    expect(validated.agentFiles).toEqual({
      root: null,
      unavailable: 'agent unavailable',
      files: [],
    });
    expect(JSON.stringify([ca, csr, signed, imported, exported, state])).not.toContain(
      ['BEGIN', 'PRIVATE KEY'].join(' '),
    );
  });

  it('validates unavailable/empty state and empty revocation result lists', async () => {
    const api = service();
    expect(PkiStateOut.parse(await api.state()).cas).toEqual([]);
    expect(PkiCrlOut.parse(await api.refreshCrl(undefined, user))).toEqual({ results: [] });
    expect(PkiOcspOut.parse(await api.checkOcsp(undefined))).toEqual({ results: [] });
  });

  it('documents concrete required public fields on every endpoint', () => {
    const endpoints = [
      ['ca', PkiCaOut, 'certificatePem'],
      ['csr', PkiCsrOut, 'csrPem'],
      ['sign', PkiSignOut, 'issued'],
      ['import', PkiImportOut, 'keyRef'],
      ['export', PkiExportOut, 'pem'],
      ['crl', PkiCrlOut, 'results'],
      ['ocsp', PkiOcspOut, 'results'],
      ['state', PkiStateOut, 'certificates'],
    ] as const;
    for (const [method, schema, field] of endpoints) {
      const documented = Reflect.getMetadata(
        'swagger/apiResponse',
        PkiController.prototype[method],
      ) as Record<number, { schema: unknown }>;
      expect(documented[200]?.schema).toEqual(openapi(schema, 'output'));
      expect(openapi(schema, 'output').required).toContain(field);
      expect(openapi(schema, 'output').additionalProperties).toBe(false);
    }
  });

  it('rejects accidental material fields in otherwise valid public responses', async () => {
    const response = await service().createCsr(
      { name: 'server', subject: 'CN=server', san: [], replace: false },
      user,
    );
    expect(PkiCsrOut.safeParse({ ...response, privateKeyPem: 'unexpected' }).success).toBe(false);
    expect(PkiCsrOut.safeParse({ ...response, passphrase: 'unexpected' }).success).toBe(false);
  });

  it.each(['P-256', 'secp521r1'])(
    'documents an unstaged external %s CSR with a dotted-OID subject without inventing metadata',
    async (curve) => {
      const api = service();
      await api.createCa(
        { name: 'ca', subject: 'CN=CA', days: 3650, stage: true, replace: false },
        user,
      );
      const files = scratch();
      try {
        openssl([
          'req',
          '-new',
          '-newkey',
          'ec',
          '-pkeyopt',
          `ec_paramgen_curve:${curve}`,
          '-sha512',
          '-nodes',
          '-keyout',
          files.file('external-key.pem', ''),
          '-subj',
          '/title=Role',
          '-out',
          files.dir + '/external.csr',
        ]);
        const csrPem = openssl(['req', '-in', files.dir + '/external.csr']);
        const parsed = parseCsr(csrPem);
        expect(parsed.subject).toBe('2.5.4.12=Role');
        const response = await api.sign(
          { ca: 'ca', name: 'external', csrPem, days: 10, stage: true, replace: false },
          user,
        );
        const documented = PkiSignOut.parse(response);
        expect(documented.staged).toBe(false);
        expect(documented.patch.csr?.subject).toBe(parsed.subject);
        if (curve === 'secp521r1') {
          expect(parsed.keySpec).toBeUndefined();
          expect(documented.patch.csr).not.toHaveProperty('keySpec');
        } else {
          expect(documented.patch.csr?.keySpec).toEqual({ type: 'ecdsa', curve: 'p256' });
        }
        // Broad public observation metadata does not weaken candidate configuration validation.
        expect(PkiCsrSchema.safeParse(documented.patch.csr).success).toBe(false);
        expect(JSON.stringify(documented)).not.toContain(['BEGIN', 'PRIVATE KEY'].join(' '));
      } finally {
        files.done();
      }
    },
  );
});
