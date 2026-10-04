import { describe, expect, it, vi } from 'vitest';
import type { HostNic } from '@ngfw/proto';
import { existsSync, readFileSync, writeFileSync } from 'node:fs';
import { loadEnv } from '../../config.js';
import { emptyDocument } from '../../datastore/documents.js';
import type { Doc } from '../../datastore/repo.js';
import { pciIfName, reasonOnly, seedDocument, SeedService } from './seed.service.js';

const GOLDEN = new URL('./seed.golden.json', import.meta.url);
// the reference host (docs/lab/host-ngfw-a.md) plus one NIC already bound to vfio-pci (no netdev)
const NGFW_A: HostNic[] = [
  ['ens161', '0000:04:00.0', false],
  ['ens192', '0000:0b:00.0', true],
  ['ens193', '0000:0c:00.0', false],
  ['ens224', '0000:13:00.0', false],
  ['ens225', '0000:14:00.0', false],
  ['ens256', '0000:1b:00.0', false],
  ['ens257', '0000:1c:00.0', false],
  ['', '0000:1d:00.0', false],
].map(([netdev, pci, isManagement]) => ({
  netdev: netdev as string,
  pci: pci as string,
  driver: netdev ? 'vmxnet3' : 'vfio-pci',
  mac: '',
  isManagement: isManagement as boolean,
  boundToDpdk: !netdev,
  linkUp: true,
}));

const nic = (netdev: string, pci: string, isManagement: boolean): HostNic => ({
  netdev,
  pci,
  driver: 'vmxnet3',
  mac: '',
  isManagement,
  boundToDpdk: false,
  linkUp: true,
});

function service(nics: HostNic[], seedEnabled = true, hostNics?: () => Promise<unknown>) {
  let ready: (() => void) | undefined;
  const record = vi.fn(async () => undefined);
  const systemCommit = vi.fn(async () => ({ status: 'applied' }));
  const svc = new SeedService(
    {
      hostNics: hostNics ?? (async () => ({ nics, managementNotes: [], owner: 'w1' })),
      watchReady: (cb: () => void) => {
        ready = cb;
        return () => undefined;
      },
    } as never,
    { getRunning: async () => ({ revision: null, doc: {} }) } as never,
    { systemCommit } as never,
    { record } as never,
    { NGFW_SEED_DEFAULT_NICS: seedEnabled } as never,
  );
  return { svc, record, systemCommit, ready: () => ready?.() };
}

describe('SeedService (F-default-vpp-nics)', () => {
  it('refuses to seed when no NIC is flagged management (review R2R4 #2): warning event, no commit, retried later', async () => {
    const { svc, record, systemCommit } = service([
      nic('ens161', '0000:04:00.0', false),
      nic('ens192', '0000:0b:00.0', false),
    ]);
    expect(await svc.seedIfNeeded()).toBe('no-management');
    expect(systemCommit).not.toHaveBeenCalled();
    expect(record).toHaveBeenCalledWith(
      'warning',
      'system',
      'system.seed-defaults-deferred',
      expect.any(String),
      expect.anything(),
    );
    // not final: a later attempt tries again, but the deferral is recorded only once (review MINOR 2)
    expect(await svc.seedIfNeeded()).toBe('no-management');
    expect(record).toHaveBeenCalledTimes(1);
  });

  it('seeds when a management NIC is identified', async () => {
    const { svc, systemCommit } = service([
      nic('ens161', '0000:04:00.0', false),
      nic('ens192', '0000:0b:00.0', true),
    ]);
    expect(await svc.seedIfNeeded()).toBe('seeded');
    expect(systemCommit).toHaveBeenCalledTimes(1);
  });

  it('NGFW_SEED_DEFAULT_NICS defaults to false; only 1 / true enable it (fail-closed, D-192)', () => {
    const base = { NGFW_JWT_SECRET: 'x'.repeat(40) };
    expect(loadEnv(base).NGFW_SEED_DEFAULT_NICS).toBe(false);
    expect(loadEnv({ ...base, NGFW_SEED_DEFAULT_NICS: '1' }).NGFW_SEED_DEFAULT_NICS).toBe(true);
    expect(loadEnv({ ...base, NGFW_SEED_DEFAULT_NICS: 'true' }).NGFW_SEED_DEFAULT_NICS).toBe(true);
    expect(loadEnv({ ...base, NGFW_SEED_DEFAULT_NICS: '0' }).NGFW_SEED_DEFAULT_NICS).toBe(false);
  });

  it('is off unless NGFW_SEED_DEFAULT_NICS is set (fail-closed, D-192)', async () => {
    const { svc, systemCommit } = service([nic('ens192', '0000:0b:00.0', true)], false);
    expect(await svc.seedIfNeeded()).toBe('skipped');
    expect(systemCommit).not.toHaveBeenCalled();
  });

  it('persists only the reason class of a management note (review R2R4 #5)', () => {
    expect(reasonOnly('ens192 → 0000:0b:00.0 (control connection from 172.30.1.9)')).toBe(
      'ens192 → 0000:0b:00.0 (control connection)',
    );
    expect(reasonOnly('ens192 → 0000:0b:00.0 (default route)')).toBe(
      'ens192 → 0000:0b:00.0 (default route)',
    );
  });

  it('seedDocument: the ngfw-a inventory seeds the golden document (review R1R3 #6)', () => {
    const got = seedDocument(emptyDocument(), NGFW_A);
    if (!existsSync(GOLDEN) || process.env['UPDATE_GOLDEN'] === '1')
      writeFileSync(GOLDEN, JSON.stringify(got, null, 2) + '\n');
    expect(got).toEqual(JSON.parse(readFileSync(GOLDEN, 'utf8')));
  });

  it('seedDocument: never layers over a non-empty document; needs a management and a data NIC (review R1R3 #8)', () => {
    const touched = structuredClone(emptyDocument()) as Doc & { system: { hostname?: string } };
    touched.system.hostname = 'edge1';
    expect(seedDocument(touched, NGFW_A)).toBeNull();
    expect(
      seedDocument(
        emptyDocument(),
        NGFW_A.filter((n) => !n.isManagement),
      ),
    ).toBeNull();
    expect(
      seedDocument(
        emptyDocument(),
        NGFW_A.filter((n) => n.isManagement),
      ),
    ).toBeNull();
  });

  it('pciIfName names a DPDK-bound NIC from its PCI address', () => {
    expect(pciIfName('0000:1d:00.0')).toBe('enp29s0f0');
    expect(pciIfName('0001:04:00.1')).toBe('en1p4s0f1');
    expect(pciIfName('bogus')).toBeUndefined();
  });

  it('lifecycle: agent unreachable at boot → seeded on the next ready callback (review R1R3 #4)', async () => {
    let up = false;
    const nics = [nic('ens161', '0000:04:00.0', false), nic('ens192', '0000:0b:00.0', true)];
    const { svc, systemCommit, ready } = service(nics, true, async () => {
      if (!up) throw new Error('agent unreachable');
      return { nics, managementNotes: [], owner: 'w1' };
    });
    svc.retryMs = 60_000;
    svc.start();
    await vi.waitFor(() => expect(svc['retryTimer']).toBeDefined());
    expect(systemCommit).not.toHaveBeenCalled();
    up = true;
    ready();
    await vi.waitFor(() => expect(systemCommit).toHaveBeenCalledTimes(1));
    svc.stop();
  });

  it('lifecycle: a deferred seed is retried by a bounded timer while the agent stays connected (review R1R3 #4)', async () => {
    const nics = [nic('ens161', '0000:04:00.0', false), nic('ens192', '0000:0b:00.0', true)];
    const { svc, systemCommit } = service(nics);
    systemCommit
      .mockResolvedValueOnce({
        status: 'deferred',
        reason: 'a commit is waiting for confirmation',
      } as never)
      .mockResolvedValueOnce({ status: 'applied' } as never);
    svc.retryMs = 20;
    svc.start();
    await vi.waitFor(() => expect(systemCommit).toHaveBeenCalledTimes(2), { timeout: 2000 });
    expect(await svc.seedIfNeeded()).toBe('skipped'); // done: no third attempt
    expect(systemCommit).toHaveBeenCalledTimes(2);
    svc.stop();
  });
});
