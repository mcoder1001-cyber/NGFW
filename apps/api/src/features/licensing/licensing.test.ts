import { readFileSync, mkdtempSync, rmSync, writeFileSync, existsSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { Logger } from '@nestjs/common';
import { Reflector } from '@nestjs/core';
import { validateConfig } from '@ngfw/schema';
import { afterAll, beforeEach, describe, expect, it, vi } from 'vitest';
import type { AgentClient } from '../../agent/agent.client.js';
import type { SystemEventsService } from '../../audit/system-events.service.js';
import { ROLE_KEY } from '../../auth/decorators.js';
import { ProblemError } from '../../common/problem.js';
import { ValidationService } from '../../commit/validation.service.js';
import type { ConfigRepo, Doc } from '../../datastore/repo.js';
import { COMMUNITY, SAMPLE_COMMUNITY, entitlementIssues, matchPointers } from './entitlements.js';
import {
  canonicalJson,
  evaluate,
  LicenseFormatError,
  machineIdHash,
  parseAndVerify,
} from './format.js';
import { licensingOptionsFromEnv, type LicensingOptions } from './licensing.config.js';
import { LicensingController } from './licensing.controller.js';
import { LicensingService } from './licensing.service.js';
import { days, sampleLicense, signFile, testKeys } from './testkit.js';

const NOW = new Date('2026-09-25T12:00:00Z');
const example = (f: string) =>
  JSON.parse(
    readFileSync(new URL(`../../../../../packages/schema/examples/${f}`, import.meta.url), 'utf8'),
  ) as Doc;
function parsed(f: string): Doc {
  const r = validateConfig(example(f));
  if (!r.ok) throw new Error(`example ${f} invalid`);
  return r.config as Doc;
}

describe('licence format: signature and time window', () => {
  const k = testKeys();
  const other = testKeys();
  const host = { machineIdHash: machineIdHash('abc123'), serial: 'VMware-42 11' };

  it('canonical JSON sorts keys recursively and drops undefined', () => {
    expect(canonicalJson({ b: 1, a: { d: [2, { z: 1, y: 2 }], c: undefined } })).toBe(
      '{"a":{"d":[2,{"y":2,"z":1}]},"b":1}',
    );
  });

  it('verifies a valid licence', () => {
    const lic = parseAndVerify(signFile(sampleLicense(NOW), k.privateKey), [k.publicKey]);
    expect(lic.customer).toBe('Example Customer');
    expect(evaluate(lic, NOW, host)).toEqual({ status: 'valid', daysLeft: 100 });
  });

  it('rejects a tampered byte', () => {
    const text = signFile(sampleLicense(NOW), k.privateKey).replace(
      '"Example Customer"',
      '"Example Customes"',
    );
    expect(() => parseAndVerify(text, [k.publicKey])).toThrow(LicenseFormatError);
    const f = JSON.parse(signFile(sampleLicense(NOW), k.privateKey)) as { signature: string };
    const sig = Buffer.from(f.signature, 'base64');
    sig[5] = sig[5]! ^ 1;
    const bad = JSON.stringify({ ...f, signature: sig.toString('base64') });
    expect(() => parseAndVerify(bad, [k.publicKey])).toThrow(/signature/);
  });

  it('rejects the wrong key and malformed files', () => {
    expect(() =>
      parseAndVerify(signFile(sampleLicense(NOW), other.privateKey), [k.publicKey]),
    ).toThrow(/signature/);
    expect(() => parseAndVerify('nope', [k.publicKey])).toThrow(/JSON/);
    expect(() => parseAndVerify('{"format":"x"}', [k.publicKey])).toThrow(/vrxlic/);
  });

  it('not-yet-valid, grace and expired', () => {
    const future = sampleLicense(NOW, {
      notBefore: new Date(NOW.getTime() + days(2)).toISOString(),
    });
    expect(evaluate(future, NOW, host).status).toBe('invalid');
    const graced = sampleLicense(NOW, {
      expiresAt: new Date(NOW.getTime() - days(10)).toISOString(),
    });
    expect(evaluate(graced, NOW, host)).toEqual({ status: 'grace', daysLeft: 20 });
    const expired = sampleLicense(NOW, {
      expiresAt: new Date(NOW.getTime() - days(31)).toISOString(),
    });
    expect(evaluate(expired, NOW, host)).toEqual({ status: 'expired', daysLeft: 0 });
  });

  it('host binding: serial and machine-id mismatch → invalid, match → valid', () => {
    const bySerial = sampleLicense(NOW, { binding: { serial: 'VMware-42 11' } });
    expect(evaluate(bySerial, NOW, host).status).toBe('valid');
    expect(evaluate(bySerial, NOW, { ...host, serial: 'other' })).toMatchObject({
      status: 'invalid',
      reason: expect.stringMatching(/serial/),
    });
    const byMid = sampleLicense(NOW, { binding: { machineIdHash: machineIdHash('abc123\n') } });
    expect(evaluate(byMid, NOW, host).status).toBe('valid');
    expect(evaluate(byMid, NOW, { machineIdHash: machineIdHash('zzz') }).status).toBe('invalid');
  });
});

describe('entitlement predicates on sample documents', () => {
  it('matchPointers expands * over records and arrays', () => {
    const d = { a: { x: [1, 2], 'y/z': [3] } };
    expect(matchPointers(d, '/a/*/*')).toEqual(['/a/x/0', '/a/x/1', '/a/y~1z/0']);
    expect(matchPointers(d, '/b/*')).toEqual([]);
  });

  it('VRRP document without the "ha" entitlement → pointer of the first VRRP instance', () => {
    const doc = parsed('ha-vrrp.json');
    const issues = entitlementIssues(doc, {}, SAMPLE_COMMUNITY);
    expect(issues[0]).toMatchObject({ pointer: '/ha/vrrp/lan-v4', rule: 'license.feature.ha' });
    expect(entitlementIssues(doc, {}, { features: ['ha'], limits: {} })).toEqual([]);
  });

  it('IPsec tunnels: feature and limit', () => {
    const doc = parsed('vpn-site-to-site.json');
    const tunnels = matchPointers(doc, '/vpn/ipsec/tunnels/*');
    expect(tunnels.length).toBeGreaterThan(0);
    expect(entitlementIssues(doc, {}, SAMPLE_COMMUNITY)[0]).toMatchObject({
      pointer: tunnels[0],
      rule: 'license.feature.ipsec',
    });
    const limited = entitlementIssues(
      doc,
      {},
      { features: ['ipsec'], limits: { ipsecTunnels: 0 } },
    );
    expect(limited).toEqual([expect.objectContaining({ rule: 'license.limit.ipsecTunnels' })]);
  });

  it('minimal document is fine under the community set', () => {
    expect(entitlementIssues(parsed('minimal.json'), {}, SAMPLE_COMMUNITY)).toEqual([]);
  });

  it('grandfathering: nodes already running never offend; a new one does', () => {
    const doc = parsed('ha-vrrp.json');
    expect(entitlementIssues(doc, doc, SAMPLE_COMMUNITY)).toEqual([]);
    const running = structuredClone(doc) as { ha: { vrrp: Record<string, unknown> } };
    delete running.ha.vrrp['customer-a'];
    expect(entitlementIssues(doc, running, SAMPLE_COMMUNITY)[0]).toMatchObject({
      pointer: '/ha/vrrp/customer-a',
    });
  });
});

describe('LicensingService + commit validation stage', () => {
  const k = testKeys();
  const dir = mkdtempSync(join(tmpdir(), 'vrx-lic-'));
  const pubFile = join(dir, 'pub.pem');
  writeFileSync(pubFile, k.publicPem);
  let now = NOW;
  let running: Doc | undefined;
  const opts: LicensingOptions = {
    file: join(dir, 'store', 'license.vrxlic'),
    extraPublicKeyFile: pubFile,
    serial: 'SER-1',
    machineIdFile: join(dir, 'machine-id'),
    dmiSerialFile: join(dir, 'none'),
    checkIntervalMs: 3_600_000,
    now: () => now,
    community: SAMPLE_COMMUNITY,
  };
  const repo = {
    latestRevision: async () => (running ? { payload: running } : null),
    userHashes: async () => new Map<string, string>(),
    existingSecretRefs: async (refs: readonly string[]) => new Set(refs),
  } as unknown as ConfigRepo;
  const events = { record: vi.fn(async () => undefined) };
  const dryRun = vi.fn(async () => ({ ok: true, errors: [], plan: [] }));
  const agent = {
    health: async () => ({ subsystems: [] }),
    dryRun,
  } as unknown as AgentClient;
  let svc: LicensingService;
  let validation: ValidationService;

  beforeEach(() => {
    now = NOW;
    running = undefined;
    rmSync(join(dir, 'store'), { recursive: true, force: true });
    events.record.mockClear();
    dryRun.mockClear();
    svc = new LicensingService(opts, repo, events as unknown as SystemEventsService);
    validation = new ValidationService(repo, agent, svc);
  });
  afterAll(() => rmSync(dir, { recursive: true, force: true }));

  async function reject(p: Promise<unknown>) {
    try {
      await p;
    } catch (e) {
      expect(e).toBeInstanceOf(ProblemError);
      return e as ProblemError;
    }
    throw new Error('expected a problem');
  }

  it('no licence = community entitlements', async () => {
    const st = await svc.state();
    expect(st).toMatchObject({ status: 'community', entitlements: SAMPLE_COMMUNITY });
  });

  it('install: valid licence stored; GET-state carries no signature or binding values', async () => {
    const st = await svc.install(
      signFile(sampleLicense(NOW, { binding: { serial: 'SER-1' } }), k.privateKey),
    );
    expect(st).toMatchObject({ status: 'valid', customer: 'Example Customer', daysLeft: 100 });
    expect(st.bound).toEqual({ machineId: false, serial: true });
    expect(existsSync(opts.file)).toBe(true);
    const out = JSON.stringify(await svc.state());
    expect(out).not.toContain('signature');
    expect(out).not.toContain('SER-1');
  });

  it('install: tampered / wrong key / bound elsewhere / expired → 400', async () => {
    const tampered = signFile(sampleLicense(NOW), k.privateKey).replace('"bgp"', '"bgq"');
    expect((await reject(svc.install(tampered))).getStatus()).toBe(400);
    expect(
      (await reject(svc.install(signFile(sampleLicense(NOW), testKeys().privateKey)))).getStatus(),
    ).toBe(400);
    const bound = signFile(sampleLicense(NOW, { binding: { serial: 'X' } }), k.privateKey);
    expect((await reject(svc.install(bound))).body().detail).toMatch(/serial/);
    const old = sampleLicense(NOW, { expiresAt: new Date(NOW.getTime() - days(40)).toISOString() });
    expect((await reject(svc.install(signFile(old, k.privateKey)))).getStatus()).toBe(400);
    expect(existsSync(opts.file)).toBe(false);
  });

  it('commit with an unlicensed feature → 403 problem+json with the pointer; licensed → passes to DryRun', async () => {
    const doc = example('ha-vrrp.json');
    const e = await reject(validation.validate(doc, 't1'));
    expect(e.getStatus()).toBe(403);
    expect(e.body()).toMatchObject({
      type: 'https://vrx.dev/problems/license-required',
      tier: 'license',
      errors: [expect.objectContaining({ pointer: '/ha/vrrp/lan-v4' })],
    });
    expect(dryRun).not.toHaveBeenCalled();
    await svc.install(signFile(sampleLicense(NOW), k.privateKey));
    const ok = await validation.validate(doc, 't2');
    expect(ok.ok).toBe(true);
    expect(dryRun).toHaveBeenCalledTimes(1);
  });

  it('grace: accepted with a warning + system_event; past grace: new features rejected, running untouched', async () => {
    await svc.install(signFile(sampleLicense(NOW), k.privateKey));
    const doc = example('ha-vrrp.json');
    running = parsed('ha-vrrp.json');
    const shrunk = structuredClone(running) as { ha: { vrrp: Record<string, unknown> } };
    delete shrunk.ha.vrrp['customer-a'];
    running = shrunk as unknown as Doc;

    now = new Date(NOW.getTime() + days(110)); // 10 days past expiry
    const g = await validation.validate(doc, 't3');
    expect(g.ok).toBe(true);
    expect(g.warnings[0]).toMatchObject({ rule: 'license.grace' });
    expect(events.record).toHaveBeenCalledWith(
      'warning',
      'licensing',
      'license.grace',
      expect.any(String),
      expect.objectContaining({ licenseId: 'LIC-TEST-0001' }),
    );

    now = new Date(NOW.getTime() + days(140)); // past grace
    const e = await reject(validation.validate(doc, 't4'));
    expect(e.body().errors).toEqual([
      expect.objectContaining({ pointer: '/ha/vrrp/customer-a', rule: 'license.feature.ha' }),
    ]);
    expect(events.record).toHaveBeenCalledWith(
      'error',
      'licensing',
      'license.expired',
      expect.any(String),
      expect.anything(),
    );
    // the running configuration itself (and a commit that keeps it) is still accepted: nothing is removed
    const keep = await validation.validate(structuredClone(running) as Doc, 't5');
    expect(keep.ok).toBe(true);
    expect((await svc.state()).status).toBe('expired');
  });

  it('default community set: no gated feature, zero limits; running config grandfathered (DEC-licensing-matrix)', () => {
    expect(COMMUNITY.features).toEqual([]);
    expect(COMMUNITY.limits).toEqual({ ipsecTunnels: 0, wireguardInterfaces: 0 });
    const ha = parsed('ha-vrrp.json');
    const vpn = parsed('vpn-site-to-site.json');
    expect(entitlementIssues(ha, {}, COMMUNITY)).not.toEqual([]);
    expect(entitlementIssues(vpn, {}, COMMUNITY).map((i) => i.rule)).toContain(
      'license.feature.ipsec',
    );
    expect(entitlementIssues(ha, ha, COMMUNITY)).toEqual([]);
    expect(entitlementIssues(vpn, vpn, COMMUNITY)).toEqual([]);
    expect(entitlementIssues(parsed('minimal.json'), {}, COMMUNITY)).toEqual([]);
  });

  it('VRX_LICENSE_PUBLIC_KEYS replaces the embedded key list', async () => {
    const other = testKeys();
    const env = {
      VRX_LICENSE_PUBLIC_KEYS: `${other.publicPem.trim().replace(/\n/g, '\\n')},${k.publicPem}`,
    } as NodeJS.ProcessEnv;
    const keys = licensingOptionsFromEnv(env).publicKeys;
    expect(keys).toHaveLength(2);
    expect(licensingOptionsFromEnv({}).publicKeys).toBeUndefined();
    const envSvc = new LicensingService(
      { ...opts, extraPublicKeyFile: undefined, publicKeys: keys },
      repo,
      events as unknown as SystemEventsService,
    );
    const st = await envSvc.install(signFile(sampleLicense(NOW), other.privateKey));
    expect(st).toMatchObject({ status: 'valid' });
    const noEnv = new LicensingService(
      { ...opts, extraPublicKeyFile: undefined },
      repo,
      events as unknown as SystemEventsService,
    );
    await reject(noEnv.install(signFile(sampleLicense(NOW), other.privateKey)));
  });

  it('unreadable stored licence → invalid with a reason and a warning log (no licence content)', async () => {
    const { mkdirSync } = await import('node:fs');
    mkdirSync(join(dir, 'store'), { recursive: true });
    const text = signFile(sampleLicense(NOW), k.privateKey);
    writeFileSync(opts.file, text.replace('"ha"', '"hb"'));
    const warn = vi.spyOn(Logger.prototype, 'warn').mockImplementation(() => undefined);
    const st = await svc.state();
    expect(st).toMatchObject({
      status: 'invalid',
      reason: expect.stringMatching(/cannot be verified/),
    });
    expect(warn).toHaveBeenCalledWith(expect.stringMatching(/does not verify \(signature\)/));
    const sig = (JSON.parse(text) as { signature: string }).signature;
    for (const c of warn.mock.calls) expect(String(c[0])).not.toContain(sig);
    warn.mockRestore();
  });

  it('RBAC: upload is admin-only; state uses the default (readonly) role', () => {
    const r = new Reflector();
    expect(r.get(ROLE_KEY, LicensingController.prototype.put)).toBe('admin');
    expect(r.get(ROLE_KEY, LicensingController.prototype.state)).toBeUndefined();
  });
});
