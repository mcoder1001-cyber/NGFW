import { createPublicKey } from 'node:crypto';
import { mkdtempSync, readFileSync, rmSync, statSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { Test } from '@nestjs/testing';
import { APP_FILTER } from '@nestjs/core';
import { FastifyAdapter, type NestFastifyApplication } from '@nestjs/platform-fastify';
import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest';
import { SystemEventsService } from '../../audit/system-events.service.js';
import { ProblemFilter } from '../../common/problem.js';
import { CONFIG_REPO } from '../../datastore/datastore.service.js';
import { canonicalJson, parseAndVerify } from './format.js';
import { LICENSING_OPTIONS, type LicensingOptions } from './licensing.config.js';
import { LicensingController } from './licensing.controller.js';
import { LicensingService } from './licensing.service.js';

const CLI = fileURLToPath(new URL('../../../../../tools/license/vrx-license.mjs', import.meta.url));
const REPO_ROOT = fileURLToPath(new URL('../../../../../', import.meta.url));

/** tools/license/vrx-license.mjs end to end; keys are generated under a fresh temp dir (0700) and deleted. */
describe('vrx-license CLI', () => {
  const dir = mkdtempSync(join(tmpdir(), 'vrx-lic-cli-'));
  type Cli = {
    main: (argv: string[], io: { out: (s: string) => void; err: (s: string) => void }) => number;
  };
  let cli: Cli;
  beforeAll(async () => {
    cli = (await import(pathToFileURL(CLI).href)) as Cli;
  });
  /** The CLI in-process (apps/api may not spawn processes — tools/ci.sh forbidden patterns). */
  const run = (...args: string[]) => {
    let stdout = '';
    let stderr = '';
    const status = cli.main(args, { out: (x) => (stdout += x), err: (x) => (stderr += x) });
    return { status, stdout, stderr };
  };
  const keyDir = join(dir, 'keys');
  const lic = join(dir, 'a.vrxlic');

  afterAll(() => rmSync(dir, { recursive: true, force: true }));

  it('keygen writes outside the repo (0600) and refuses inside a git repository', () => {
    const r = run('keygen', '--out-dir', keyDir);
    expect(r.status).toBe(0);
    expect(r.stdout).toContain('signing key:');
    expect(statSync(join(keyDir, 'vrx-license-signing.pem')).mode & 0o777).toBe(0o600);
    const inRepo = run('keygen', '--out-dir', join(REPO_ROOT, 'tmp-should-not-exist'));
    expect(inRepo.status).toBe(2);
    expect(inRepo.stderr).toMatch(/refusing/);
  });

  it('issue → verify → inspect, and the API verifies what the CLI signed (same canonical JSON)', () => {
    const r = run(
      'issue',
      '--key',
      join(keyDir, 'vrx-license-signing.pem'),
      '--customer',
      'ACME Test',
      '--id',
      'LIC-CLI-1',
      '--days',
      '365',
      '--serial',
      'SER-9',
      '--features',
      'ipsec,ha',
      '--limit',
      'ipsecTunnels=50',
      '--out',
      lic,
    );
    expect(r.status, r.stderr).toBe(0);
    const pub = join(keyDir, 'vrx-license-public.pem');
    const v = run('verify', '--pub', pub, lic);
    expect(v.status).toBe(0);
    expect(v.stdout).toMatch(/signature OK; status valid/);
    expect(run('verify', '--pub', pub, '--at', '2099-01-01T00:00:00Z', lic).stdout).toMatch(
      /expired/,
    );
    const shown = JSON.parse(run('inspect', lic).stdout);
    expect(shown).toMatchObject({ licenseId: 'LIC-CLI-1', binding: { serial: 'SER-9' } });
    expect(JSON.stringify(shown)).not.toContain('signature');
    const text = readFileSync(lic, 'utf8');
    const parsed = parseAndVerify(text, [createPublicKey(readFileSync(pub, 'utf8'))]);
    expect(parsed.entitlements).toEqual({
      features: ['ipsec', 'ha'],
      limits: { ipsecTunnels: 50 },
    });
    expect(canonicalJson(JSON.parse(text).license)).toContain('"customer":"ACME Test"');
  });

  it('verify rejects a tampered byte', () => {
    const bad = join(dir, 'bad.vrxlic');
    writeFileSync(bad, readFileSync(lic, 'utf8').replace('ACME Test', 'ACME Tesu'));
    const v = run('verify', '--pub', join(keyDir, 'vrx-license-public.pem'), bad);
    expect(v.status).toBe(1);
    expect(v.stdout).toMatch(/INVALID/);
  });

  describe('HTTP: PUT /api/v1/system/license + GET /api/v1/state/license', () => {
    let app: NestFastifyApplication;
    const events = { record: vi.fn(async () => undefined) };

    beforeAll(async () => {
      const opts: LicensingOptions = {
        file: join(dir, 'store', 'license.vrxlic'),
        extraPublicKeyFile: join(keyDir, 'vrx-license-public.pem'),
        serial: 'SER-9',
        machineIdFile: join(dir, 'no-machine-id'),
        dmiSerialFile: join(dir, 'no-serial'),
        checkIntervalMs: 3_600_000,
        now: () => new Date(),
      };
      const mod = await Test.createTestingModule({
        controllers: [LicensingController],
        providers: [
          { provide: LICENSING_OPTIONS, useValue: opts },
          { provide: CONFIG_REPO, useValue: { latestRevision: async () => null } },
          { provide: SystemEventsService, useValue: events },
          { provide: APP_FILTER, useClass: ProblemFilter },
          LicensingService,
        ],
      }).compile();
      app = mod.createNestApplication<NestFastifyApplication>(new FastifyAdapter(), {
        logger: false,
      });
      await app.init();
      await app.getHttpAdapter().getInstance().ready();
    });
    afterAll(async () => app?.close());

    it('upload → state valid + entitlements; tampered → 400 problem+json', async () => {
      const body = JSON.parse(readFileSync(lic, 'utf8')) as Record<string, unknown>;
      const put = await app.inject({ method: 'PUT', url: '/api/v1/system/license', payload: body });
      expect(put.statusCode).toBe(200);
      const get = await app.inject({ method: 'GET', url: '/api/v1/state/license' });
      expect(get.json()).toMatchObject({
        status: 'valid',
        customer: 'ACME Test',
        entitlements: { features: ['ipsec', 'ha'], limits: { ipsecTunnels: 50 } },
      });
      expect(get.body).not.toContain(String(body.signature));
      const tampered = structuredClone(body) as { license: { customer: string } };
      tampered.license.customer = 'ACME Tesu';
      const bad = await app.inject({
        method: 'PUT',
        url: '/api/v1/system/license',
        payload: tampered,
      });
      expect(bad.statusCode).toBe(400);
      expect(bad.headers['content-type']).toContain('application/problem+json');
      expect(bad.json()).toMatchObject({ detail: 'licence signature is invalid' });
    });
  });
});
