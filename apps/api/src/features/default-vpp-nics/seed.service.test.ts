import { describe, expect, it, onTestFinished, vi } from 'vitest';
import type { HostNic } from '@ngfw/proto';
import { validateConfig, type DataplaneConfig, type InterfaceConfig } from '@ngfw/schema';
import { existsSync, readFileSync, writeFileSync } from 'node:fs';
import { loadEnv } from '../../config.js';
import { emptyDocument } from '../../datastore/documents.js';
import type { Doc } from '../../datastore/repo.js';
import { Logger } from '@nestjs/common';
import { ProblemError } from '../../common/problem.js';
import {
  isReservedIfName,
  pciIfName,
  planRegistration,
  reasonOnly,
  seedDocument,
  SeedService,
} from './seed.service.js';

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

/**
 * A SeedService over fakes. `running` is the persisted state: the fake systemCommit applies the change to it like the
 * commit engine does (null → unchanged, else one more revision), so tests see what a later boot would read back.
 */
function service(
  nics: HostNic[],
  seedEnabled = true,
  hostNics?: () => Promise<unknown>,
  running: { revision: number | null; doc: Doc } = { revision: null, doc: emptyDocument() },
) {
  let ready: (() => void) | undefined;
  const record = vi.fn(async () => undefined);
  const systemCommit = vi.fn(async (change: (doc: Doc) => Doc | null) => {
    const next = change(structuredClone(running.doc));
    if (next === null) return { status: 'unchanged' };
    running.doc = next;
    running.revision = (running.revision ?? 0) + 1;
    return { status: 'applied' };
  });
  const stopWatch = vi.fn();
  const watchReady = vi.fn((cb: () => void) => {
    ready = cb;
    return stopWatch;
  });
  const svc = new SeedService(
    {
      hostNics: hostNics ?? (async () => ({ nics, managementNotes: [], owner: 'w1' })),
      watchReady,
    } as never,
    { getRunning: async () => structuredClone(running) } as never,
    { systemCommit } as never,
    { record } as never,
    { NGFW_SEED_DEFAULT_NICS: seedEnabled } as never,
  );
  return { svc, record, systemCommit, running, watchReady, stopWatch, ready: () => ready?.() };
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

  it('seedDocument: preserves other configuration (D-249) and needs a management and a data NIC (review R1R3 #8)', () => {
    const touched = structuredClone(emptyDocument()) as Doc & { system: { hostname?: string } };
    touched.system.hostname = 'edge1';
    const next = seedDocument(touched, NGFW_A)!;
    expect(next['system']).toEqual(touched.system);
    expect(Object.keys(next['interfaces'] as object)).toHaveLength(6); // the bound, unnamed NIC is skipped
    expect(touched['interfaces']).toEqual({}); // pure: the input is not mutated
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

  it('registers NICs after other settings have already been committed, without repeating the revision on restart', async () => {
    const doc = structuredClone(emptyDocument());
    (doc['system'] as Record<string, unknown>)['hostname'] = 'edge1';
    const initial = { revision: 4, doc };
    const { svc, systemCommit, running, record } = service(NGFW_A, true, undefined, initial);
    expect(await svc.seedIfNeeded()).toBe('seeded');
    expect(systemCommit).toHaveBeenCalledTimes(1);
    expect(running.revision).toBe(5);
    expect(running.doc['system']).toEqual(doc['system']);
    expect(Object.keys(running.doc['interfaces'] as object)).toHaveLength(6);
    expect(record).toHaveBeenCalledWith(
      'info',
      'system',
      'system.seed-defaults',
      'registered 6 missing dataplane NIC(s) from the host inventory',
      expect.objectContaining({
        dataplane: expect.arrayContaining([{ name: 'ens161', pci: '0000:04:00.0' }]),
        skipped: [{ name: 'enp29s0f0', pci: '0000:1d:00.0', reason: 'bound-unnamed' }],
      }),
    );
    // the next boot (a fresh service instance on the same database) finds nothing to add: no revision 6
    const fresh = service(NGFW_A, true, undefined, running);
    expect(await fresh.svc.seedIfNeeded()).toBe('exists');
    expect(fresh.systemCommit).toHaveBeenCalledTimes(1);
    expect(running.revision).toBe(5);
    // the bound NIC without a saved name stays unregistered and is reported again at every start
    expect(fresh.record).toHaveBeenCalledTimes(1);
    expect(fresh.record).toHaveBeenCalledWith(
      'warning',
      'system',
      'system.seed-defaults-skipped',
      expect.stringContaining('enp29s0f0 (0000:1d:00.0): bound-unnamed'),
      { skipped: [{ name: 'enp29s0f0', pci: '0000:1d:00.0', reason: 'bound-unnamed' }] },
    );
  });

  it('adds a newly discovered NIC while preserving released and disabled physical NICs and device tuning', () => {
    const doc = seedDocument(emptyDocument(), NGFW_A.slice(0, 3))!;
    const interfaces = doc['interfaces'] as Record<string, InterfaceConfig>;
    const dp = doc['dataplane'] as DataplaneConfig;
    // an operator released ens161 to the host, disabled ens193 with an address, and tuned the ens193 device
    interfaces['ens161']!.physical!.owner = 'host';
    interfaces['ens193']!.enabled = false;
    interfaces['ens193']!.ipv4 = ['192.0.2.1/24'];
    dp.pciWhitelist = dp.pciWhitelist.filter((pci) => pci !== '0000:04:00.0');
    delete dp.devices['0000:04:00.0'];
    dp.devices['0000:0c:00.0']!.rxQueues = 4;
    const next = seedDocument(doc, NGFW_A)!;
    expect((next['interfaces'] as Record<string, unknown>)['ens161']).toEqual(interfaces['ens161']);
    expect((next['interfaces'] as Record<string, unknown>)['ens193']).toEqual(interfaces['ens193']);
    expect((next['dataplane'] as typeof dp).devices['0000:0c:00.0']).toEqual(
      dp.devices['0000:0c:00.0'],
    );
    expect((next['dataplane'] as typeof dp).pciWhitelist).not.toContain('0000:04:00.0');
    expect(Object.keys(next['interfaces'] as object)).toHaveLength(6);
    expect((next['interfaces'] as Record<string, InterfaceConfig>)['ens224']).toEqual({
      enabled: true,
      physical: { pci: '0000:13:00.0', owner: 'dataplane', builtIn: true },
    });
    // and the same inventory on the next boot adds nothing
    expect(seedDocument(next, NGFW_A)).toBeNull();
  });

  it('keeps explicit logical names, device tuning and interface settings when adopting an existing NIC', () => {
    const doc = structuredClone(emptyDocument());
    doc['interfaces'] = { wan: { enabled: false, ipv4: ['192.0.2.1/24'], mtu: 1400 } };
    const dp = doc['dataplane'] as { pciWhitelist: string[]; devices: Record<string, unknown> };
    dp.pciWhitelist = ['0000:0C:00.0'];
    dp.devices['0000:0C:00.0'] = { name: 'wan', rxQueues: 4 };
    // while the NIC is still the kernel's ens193, a row named wan would be applied as a VPP interface that does not
    // exist yet (the agent tells unbound NICs by their kernel name): wait for the start-up configuration to bind it
    const kernel = planRegistration(doc, [
      nic('ens192', '0000:0b:00.0', true),
      nic('ens193', '0000:0c:00.0', false),
    ]);
    expect(kernel.next).toBeNull();
    expect(kernel.skipped).toEqual([{ name: 'wan', pci: '0000:0c:00.0', reason: 'name-mismatch' }]);
    // bound by DPDK under its saved name: adopted with every setting kept
    const next = seedDocument(doc, [
      nic('ens192', '0000:0b:00.0', true),
      { ...nic('', '0000:0c:00.0', false), driver: 'vfio-pci', boundToDpdk: true },
    ])!;
    expect(next['interfaces']).toEqual({
      wan: {
        enabled: false,
        ipv4: ['192.0.2.1/24'],
        mtu: 1400,
        physical: { pci: '0000:0c:00.0', owner: 'dataplane', builtIn: true },
      },
    });
    expect((next['dataplane'] as typeof dp).devices).toEqual(dp.devices);
    expect((next['dataplane'] as typeof dp).pciWhitelist).toEqual(dp.pciWhitelist);
  });

  it('never attaches a NIC to a bond or a second physical row, nor adopts a reserved name — each skip is reported', () => {
    const doc = seedDocument(emptyDocument(), [
      nic('ens192', '0000:0b:00.0', true),
      nic('ens193', '0000:0c:00.0', false),
    ])!;
    // a bond named like the new NIC's kernel name, and a device whose saved name is the one ens193 already uses
    (doc['interfaces'] as Record<string, unknown>)['ens224'] = {
      enabled: true,
      bond: { mode: 'lacp', members: {} },
    };
    (doc['dataplane'] as DataplaneConfig).devices['0000:14:00.0'] = { name: 'ens193' };
    const plan = planRegistration(doc, [
      nic('ens192', '0000:0b:00.0', true),
      nic('ens193', '0000:0c:00.0', false),
      nic('ens224', '0000:13:00.0', false),
      nic('ens225', '0000:14:00.0', false),
      nic('loop7', '0000:1b:00.0', false),
      nic('Bad-Name', '0000:1c:00.0', false),
    ]);
    expect(plan.next).toBeNull();
    expect(plan.added).toEqual([]);
    expect(plan.skipped).toEqual([
      { name: 'ens224', pci: '0000:13:00.0', reason: 'name-in-use' },
      { name: 'ens193', pci: '0000:14:00.0', reason: 'name-mismatch' }, // saved name ≠ kernel name ens225
      { name: 'loop7', pci: '0000:1b:00.0', reason: 'reserved-name' },
      { name: 'Bad-Name', pci: '0000:1c:00.0', reason: 'invalid-name' },
    ]);
  });

  it('isReservedIfName follows the startup generator: a stem alone or followed by a digit, _ or -', () => {
    for (const name of ['local0', 'default', 'loop0', 'tap1', 'bond-0', 'host-eth0', 'bvi', 'pppoe3', 'wg_1'])
      expect(isReservedIfName(name), name).toBe(true);
    for (const name of ['green', 'loopback', 'wan', 'ens161', 'bonding', 'enp29s0f0', 'spanner'])
      expect(isReservedIfName(name), name).toBe(false);
  });

  it('never writes a management NIC the configuration hands to the engine into managementPci; reports the conflict', () => {
    // boot 1: ens161 and ens193 registered as engine-owned; boot 2: the host now reaches the box through ens161
    // (new default route) and a NIC was added
    const doc = seedDocument(emptyDocument(), [
      nic('ens192', '0000:0b:00.0', true),
      nic('ens161', '0000:04:00.0', false),
      nic('ens193', '0000:0c:00.0', false),
    ])!;
    const boot2 = [
      nic('ens192', '0000:0b:00.0', true),
      nic('ens161', '0000:04:00.0', true),
      nic('ens193', '0000:0c:00.0', false),
      nic('ens224', '0000:13:00.0', false),
    ];
    const plan = planRegistration(doc, boot2);
    expect(plan.added).toEqual([{ name: 'ens224', pci: '0000:13:00.0' }]);
    expect((plan.next!['dataplane'] as DataplaneConfig).managementPci).toEqual(['0000:0b:00.0']);
    expect((plan.next!['dataplane'] as DataplaneConfig).pciWhitelist).toEqual([
      '0000:04:00.0',
      '0000:0c:00.0',
      '0000:13:00.0',
    ]);
    expect(plan.skipped).toEqual([
      { name: 'ens161', pci: '0000:04:00.0', reason: 'management-engine-owned' },
    ]);
    // the conflict is reported even when there is nothing to register (every data NIC already has its row)
    const again = planRegistration(doc, boot2.slice(0, 3));
    expect(again.next).toBeNull();
    expect(again.skipped).toEqual([
      { name: 'ens161', pci: '0000:04:00.0', reason: 'management-engine-owned' },
    ]);
  });

  it('never exceeds the four management PCI addresses the schema allows', () => {
    const doc = structuredClone(emptyDocument());
    (doc['dataplane'] as DataplaneConfig).managementPci = [
      '0000:0b:00.0',
      '0000:0b:00.1',
      '0000:0b:00.2',
      '0000:0b:00.3',
    ];
    const plan = planRegistration(doc, [
      nic('ens192', '0000:0b:00.0', true),
      nic('ens200', '0000:0d:00.0', true),
      nic('ens161', '0000:04:00.0', false),
    ]);
    expect(plan.added).toEqual([{ name: 'ens161', pci: '0000:04:00.0' }]);
    expect((plan.next!['dataplane'] as DataplaneConfig).managementPci).toHaveLength(4);
    expect(plan.skipped).toEqual([
      { name: 'ens200', pci: '0000:0d:00.0', reason: 'management-limit' },
    ]);
  });

  it('never registers a bound NIC without a saved device name (VPP\'s name for it is unknown)', () => {
    const plan = planRegistration(emptyDocument(), [
      nic('ens192', '0000:0b:00.0', true),
      { ...nic('', '0000:1d:00.0', false), driver: 'vfio-pci', boundToDpdk: true },
      nic('ens161', '0000:04:00.0', false),
    ]);
    expect(plan.added).toEqual([{ name: 'ens161', pci: '0000:04:00.0' }]);
    expect(plan.skipped).toEqual([{ name: 'enp29s0f0', pci: '0000:1d:00.0', reason: 'bound-unnamed' }]);
    expect((plan.next!['dataplane'] as DataplaneConfig).pciWhitelist).toEqual(['0000:04:00.0']);
  });

  it('matches existing NICs by PCI after binding removes or renames the kernel netdev', () => {
    const doc = seedDocument(emptyDocument(), NGFW_A)!;
    const bound = NGFW_A.map((n) =>
      n.isManagement ? n : { ...n, netdev: '', boundToDpdk: true, driver: 'vfio-pci' },
    );
    expect(seedDocument(doc, bound)).toBeNull();
    const renamed = NGFW_A.map((n) => (n.netdev ? { ...n, netdev: `x${n.netdev}` } : n));
    expect(seedDocument(doc, renamed)).toBeNull();
  });

  it('protects persisted management PCI addresses even when current discovery does not flag them', () => {
    const doc = structuredClone(emptyDocument());
    (doc['dataplane'] as { managementPci: string[] }).managementPci = ['0000:04:00.0'];
    const next = seedDocument(doc, NGFW_A)!;
    expect((next['interfaces'] as Record<string, unknown>)['ens161']).toBeUndefined();
    expect((next['dataplane'] as { managementPci: string[] }).managementPci).toEqual([
      '0000:04:00.0',
      '0000:0b:00.0',
    ]);
  });

  it('uses the persisted management identity while the management route is still coming up', async () => {
    const doc = structuredClone(emptyDocument());
    (doc['dataplane'] as DataplaneConfig).managementPci = ['0000:0b:00.0'];
    const { svc, running, record } = service(
      NGFW_A.map((n) => ({ ...n, isManagement: false })),
      true,
      undefined,
      { revision: 1, doc },
    );
    expect(await svc.seedIfNeeded()).toBe('seeded');
    expect((running.doc['interfaces'] as Record<string, unknown>)['ens192']).toBeUndefined();
    expect(Object.keys(running.doc['interfaces'] as object)).toHaveLength(6);
    expect(record).not.toHaveBeenCalledWith(
      'warning',
      'system',
      'system.seed-defaults-deferred',
      expect.any(String),
      expect.anything(),
    );
  });

  it('never takes a logical name another PCI already holds as its saved device name, whatever the PCI order', () => {
    // an operator pre-named 0000:14:00.0 "ens193" (no row yet); the host's 0000:0c:00.0 carries the kernel name ens193
    const doc = structuredClone(emptyDocument());
    const dp = doc['dataplane'] as DataplaneConfig;
    dp.pciWhitelist = ['0000:14:00.0'];
    dp.devices['0000:14:00.0'] = { name: 'ens193' };
    const inventory = [
      nic('ens192', '0000:0b:00.0', true),
      nic('ens193', '0000:0c:00.0', false), // sorts first (HostNics sorts by PCI)
      nic('ens224', '0000:13:00.0', false),
      nic('ens225', '0000:14:00.0', false),
    ];
    const plan = planRegistration(doc, inventory);
    expect(validateConfig(plan.next).ok).toBe(true);
    expect(plan.added).toEqual([{ name: 'ens224', pci: '0000:13:00.0' }]);
    expect(plan.skipped).toEqual([
      { name: 'ens193', pci: '0000:0c:00.0', reason: 'name-in-use' }, // the kernel name belongs to 0000:14:00.0
      { name: 'ens193', pci: '0000:14:00.0', reason: 'name-mismatch' }, // registered as ens193 once bound
    ]);
    // 0000:14:00.0 bound under its saved name while the kernel still calls 0000:0c:00.0 "ens193": the agent would take a
    // row named ens193 for that kernel NIC and drop it as unbound — neither NIC is registered under the name
    (doc['interfaces'] as Record<string, unknown>)['ens193'] = { enabled: false, mtu: 1400 };
    inventory[3] = { ...nic('', '0000:14:00.0', false), driver: 'vfio-pci', boundToDpdk: true };
    const clash = planRegistration(doc, inventory);
    expect(clash.added).toEqual([{ name: 'ens224', pci: '0000:13:00.0' }]);
    expect(clash.skipped).toEqual([
      { name: 'ens193', pci: '0000:0c:00.0', reason: 'name-in-use' },
      { name: 'ens193', pci: '0000:14:00.0', reason: 'name-in-use' },
    ]);
    // once the kernel NIC carries another name, the row without a marker is adopted for the bound NIC that owns the name
    inventory[1] = nic('ens195', '0000:0c:00.0', false);
    const adopted = planRegistration(doc, inventory);
    expect(validateConfig(adopted.next).ok).toBe(true);
    expect((adopted.next!['interfaces'] as Record<string, InterfaceConfig>)['ens193']).toEqual({
      enabled: false,
      mtu: 1400,
      physical: { pci: '0000:14:00.0', owner: 'dataplane', builtIn: true },
    });
    expect(adopted.added).toEqual([
      { name: 'ens195', pci: '0000:0c:00.0' },
      { name: 'ens224', pci: '0000:13:00.0' },
      { name: 'ens193', pci: '0000:14:00.0' },
    ]);
    expect(adopted.skipped).toEqual([]);
  });

  it('every planned document passes the schema and semantic validators', () => {
    const cases: [Doc, HostNic[]][] = [
      [emptyDocument(), NGFW_A],
      [seedDocument(emptyDocument(), NGFW_A.slice(0, 3))!, NGFW_A],
      [
        seedDocument(emptyDocument(), [nic('ens192', '0000:0b:00.0', true), nic('ens161', '0000:04:00.0', false)])!,
        [nic('ens192', '0000:0b:00.0', true), nic('ens161', '0000:04:00.0', true), nic('ens193', '0000:0c:00.0', false)],
      ],
    ];
    for (const [doc, nics] of cases) {
      const plan = planRegistration(doc, nics);
      expect(plan.next).not.toBeNull();
      expect(validateConfig(plan.next), JSON.stringify(plan.skipped)).toMatchObject({ ok: true });
    }
  });

  it('never exceeds the 64 whitelisted addresses the schema allows; a PPPoE client is never adopted; rows match by PCI case-insensitively', () => {
    const doc = structuredClone(emptyDocument());
    const dp = doc['dataplane'] as DataplaneConfig;
    const many: HostNic[] = [nic('ens192', '0000:0b:00.0', true)];
    for (let i = 0; i < 64; i++) {
      const pci = `0000:${(0x10 + i).toString(16).padStart(2, '0')}:00.0`;
      dp.pciWhitelist.push(pci);
      dp.devices[pci] = { name: `d${i}` };
      (doc['interfaces'] as Record<string, unknown>)[`d${i}`] = {
        enabled: true,
        physical: { pci: pci.toUpperCase(), owner: 'dataplane', builtIn: true },
      };
      many.push(nic(`d${i}`, pci, false));
    }
    // the 64 registered rows carry upper-case PCI addresses: still matched, nothing added twice
    expect(planRegistration(doc, many).next).toBeNull();
    (doc['interfaces'] as Record<string, unknown>)['ens161'] = { enabled: true, pppoe: { parent: 'd0' } };
    const plan = planRegistration(doc, [
      ...many,
      nic('ens161', '0000:04:00.0', false),
      nic('ens193', '0000:0c:00.0', false),
    ]);
    expect(plan.next).toBeNull();
    expect(plan.skipped).toEqual([
      { name: 'ens161', pci: '0000:04:00.0', reason: 'name-in-use' },
      { name: 'ens193', pci: '0000:0c:00.0', reason: 'whitelist-limit' },
    ]);
  });

  it('reports skipped NICs in the audit event, and as a warning event when nothing could be registered', async () => {
    const doc = seedDocument(emptyDocument(), [
      nic('ens192', '0000:0b:00.0', true),
      nic('ens161', '0000:04:00.0', false),
    ])!;
    (doc['interfaces'] as Record<string, unknown>)['ens193'] = {
      enabled: true,
      bond: { mode: 'lacp', members: {} },
    };
    const inventory = [
      nic('ens192', '0000:0b:00.0', true),
      nic('ens161', '0000:04:00.0', false),
      nic('ens193', '0000:0c:00.0', false),
      nic('ens224', '0000:13:00.0', false),
    ];
    const { svc, record, running } = service(inventory, true, undefined, { revision: 1, doc });
    expect(await svc.seedIfNeeded()).toBe('seeded');
    expect(record).toHaveBeenCalledWith(
      'info',
      'system',
      'system.seed-defaults',
      'registered 1 missing dataplane NIC(s) from the host inventory',
      expect.objectContaining({
        dataplane: [{ name: 'ens224', pci: '0000:13:00.0' }],
        management: [{ name: 'ens192', pci: '0000:0b:00.0' }],
        skipped: [{ name: 'ens193', pci: '0000:0c:00.0', reason: 'name-in-use' }],
      }),
    );
    // next boot: nothing to add, but the unregistered NIC is still visible to the operator
    const fresh = service(inventory, true, undefined, running);
    expect(await fresh.svc.seedIfNeeded()).toBe('exists');
    expect(fresh.record).toHaveBeenCalledTimes(1);
    expect(fresh.record).toHaveBeenCalledWith(
      'warning',
      'system',
      'system.seed-defaults-skipped',
      expect.stringContaining('ens193 (0000:0c:00.0): name-in-use'),
      { skipped: [{ name: 'ens193', pci: '0000:0c:00.0', reason: 'name-in-use' }] },
    );
  });

  it('a commit the validators reject is recorded once as system.seed-defaults-failed and is final for this boot', async () => {
    const nics = [nic('ens161', '0000:04:00.0', false), nic('ens192', '0000:0b:00.0', true)];
    const { svc, systemCommit, record } = service(nics);
    systemCommit.mockRejectedValue(
      new ProblemError(400, 'validation', 'Bad Request', 'tier-2 validation failed', [
        {
          pointer: '/dataplane/managementPci/1',
          message: 'management NIC must never be a DPDK device',
          rule: 'dataplane.management-not-dpdk',
        },
      ]),
    );
    svc.retryMs = 20;
    svc.start();
    await vi.waitFor(() => expect(record).toHaveBeenCalledTimes(1));
    expect(record).toHaveBeenCalledWith(
      'warning',
      'system',
      'system.seed-defaults-failed',
      'default NIC registration failed (not retried before the next API start): tier-2 validation failed',
      {
        reason: 'tier-2 validation failed',
        pointers: ['/dataplane/managementPci/1'],
        issues: [
          {
            pointer: '/dataplane/managementPci/1',
            message: 'management NIC must never be a DPDK device',
          },
        ],
        final: true,
        type: 'validation',
      },
    );
    expect(svc['done']).toBe(true);
    expect(svc['retryTimer']).toBeUndefined();
    expect(await svc.seedIfNeeded()).toBe('skipped'); // not retried until the next boot
    expect(systemCommit).toHaveBeenCalledTimes(1);
  });

  it('a transient commit failure is retried with exponential backoff and recorded once per reason', async () => {
    const nics = [nic('ens161', '0000:04:00.0', false), nic('ens192', '0000:0b:00.0', true)];
    const { svc, systemCommit, record } = service(nics);
    systemCommit.mockRejectedValue(new Error('database unavailable'));
    svc.retryMs = 20;
    svc.maxRetryMs = 80;
    onTestFinished(() => svc.stop());
    svc.start();
    await vi.waitFor(() => expect(systemCommit.mock.calls.length).toBeGreaterThanOrEqual(4), {
      timeout: 3000,
    });
    expect(svc['backoffMs']).toBe(80); // 20 → 40 → 80, capped
    expect(record).toHaveBeenCalledTimes(1);
    expect(record).toHaveBeenCalledWith(
      'warning',
      'system',
      'system.seed-defaults-failed',
      'default NIC registration failed (will retry): database unavailable',
      { reason: 'database unavailable', pointers: [], issues: [], final: false },
    );
    expect(svc['done']).toBe(false);
  });

  it('an agent refusal (4xx that a later attempt may pass) is not final: retried with backoff, recorded once', async () => {
    const nics = [nic('ens161', '0000:04:00.0', false), nic('ens192', '0000:0b:00.0', true)];
    const { svc, systemCommit, record } = service(nics);
    systemCommit.mockRejectedValue(
      new ProblemError(409, 'agent-precondition', 'Conflict', 'the agent holds a pending transaction'),
    );
    expect(await svc.seedIfNeeded()).toBe('failed');
    expect(svc['done']).toBe(false);
    expect(await svc.seedIfNeeded()).toBe('failed');
    expect(record).toHaveBeenCalledTimes(1);
    expect(record).toHaveBeenCalledWith(
      'warning',
      'system',
      'system.seed-defaults-failed',
      'default NIC registration failed (will retry): the agent holds a pending transaction',
      {
        reason: 'the agent holds a pending transaction',
        pointers: [],
        issues: [],
        final: false,
        type: 'agent-precondition',
      },
    );
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
    onTestFinished(() => svc.stop());
    svc.start();
    await vi.waitFor(() => expect(svc['retryTimer']).toBeDefined());
    expect(systemCommit).not.toHaveBeenCalled();
    up = true;
    ready();
    await vi.waitFor(() => expect(systemCommit).toHaveBeenCalledTimes(1));
  });

  it('lifecycle: a deferred seed is retried by a bounded timer while the agent stays connected (review R1R3 #4)', async () => {
    const nics = [nic('ens161', '0000:04:00.0', false), nic('ens192', '0000:0b:00.0', true)];
    const { svc, systemCommit, record } = service(nics);
    systemCommit.mockImplementationOnce(async () => ({
      status: 'deferred',
      reason: 'a commit is waiting for confirmation',
    }));
    svc.retryMs = 20;
    onTestFinished(() => svc.stop());
    svc.start();
    await vi.waitFor(() => expect(systemCommit).toHaveBeenCalledTimes(2), { timeout: 2000 });
    expect(await svc.seedIfNeeded()).toBe('skipped'); // done: no third attempt
    expect(systemCommit).toHaveBeenCalledTimes(2);
    expect(record).toHaveBeenCalledWith(
      'info',
      'system',
      'system.seed-defaults',
      'registered 1 missing dataplane NIC(s) from the host inventory',
      expect.objectContaining({ dataplane: [{ name: 'ens161', pci: '0000:04:00.0' }] }),
    );
  });

  it('backs off an empty inventory, resets the backoff after another outcome, and lets the latest outcome set the delay', async () => {
    const nics = [nic('ens192', '0000:0b:00.0', true)];
    const { svc, systemCommit } = service(nics);
    svc.retryMs = 20;
    svc.maxRetryMs = 5000;
    onTestFinished(() => svc.stop());
    svc.start();
    // no-nics backs off: 20 → 40 → 80 → 160 … (the pending timer now waits at least 160 ms)
    await vi.waitFor(() => expect(svc['backoffMs']).toBeGreaterThanOrEqual(160), { timeout: 3000 });
    // the operator is editing when the NIC appears: that attempt is deferred — the backoff is reset and the next
    // attempt follows in retryMs, not after the long backoff timer
    const calls: number[] = [];
    systemCommit.mockImplementationOnce(async () => {
      calls.push(Date.now());
      return { status: 'deferred', reason: 'the candidate has uncommitted changes' };
    });
    nics.push(nic('ens161', '0000:04:00.0', false));
    await vi.waitFor(() => expect(svc['done']).toBe(true), { timeout: 5000 });
    expect(systemCommit).toHaveBeenCalledTimes(2);
    expect(svc['backoffMs']).toBe(0);
    const [deferredAt, appliedAt] = [calls[0]!, Date.now()];
    expect(appliedAt - deferredAt).toBeLessThan(150); // retryMs-class delay, not the ≥ 160 ms backoff
  });

  it('a 5xx problem from the commit path is transient: retried, not final', async () => {
    const nics = [nic('ens161', '0000:04:00.0', false), nic('ens192', '0000:0b:00.0', true)];
    const { svc, systemCommit, record } = service(nics);
    systemCommit.mockRejectedValueOnce(
      new ProblemError(503, 'agent-unavailable', 'Service unavailable', 'agent not connected'),
    );
    expect(await svc.seedIfNeeded()).toBe('failed');
    expect(svc['done']).toBe(false);
    expect(record).toHaveBeenCalledWith(
      'warning',
      'system',
      'system.seed-defaults-failed',
      'default NIC registration failed (will retry): agent not connected',
      expect.objectContaining({ final: false, type: 'agent-unavailable', issues: [] }),
    );
    expect(await svc.seedIfNeeded()).toBe('seeded');
  });

  it('a failure alternating with an unreachable inventory is recorded once; the log never carries a peer address', async () => {
    const warn = vi.spyOn(Logger.prototype, 'warn').mockImplementation(() => undefined);
    onTestFinished(() => warn.mockRestore());
    let reachable = false;
    const nics = [nic('ens161', '0000:04:00.0', false), nic('ens192', '0000:0b:00.0', true)];
    const { svc, systemCommit, record } = service(nics, true, async () => {
      if (!reachable) throw new Error('unresolved br0 (control connection from 172.30.1.9): no PCI device');
      return { nics, managementNotes: [], owner: 'w1' };
    });
    systemCommit.mockRejectedValue(new Error('database unavailable'));
    expect(await svc.seedIfNeeded()).toBe('unreachable');
    reachable = true;
    expect(await svc.seedIfNeeded()).toBe('failed');
    reachable = false;
    expect(await svc.seedIfNeeded()).toBe('unreachable');
    reachable = true;
    expect(await svc.seedIfNeeded()).toBe('failed');
    expect(record).toHaveBeenCalledTimes(1);
    const logged = warn.mock.calls.map((c) => String(c[0])).join('\n');
    expect(logged).toContain('control connection');
    expect(logged).not.toContain('172.30.1.9');
  });

  it('retries an initially empty data inventory while the agent stays connected', async () => {
    const nics = [nic('ens192', '0000:0b:00.0', true)];
    const { svc, systemCommit } = service(nics);
    svc.retryMs = 20;
    onTestFinished(() => svc.stop());
    svc.start();
    await vi.waitFor(() => expect(svc['retryTimer']).toBeDefined());
    expect(systemCommit).not.toHaveBeenCalled();
    nics.push(nic('ens161', '0000:04:00.0', false));
    await vi.waitFor(() => expect(systemCommit).toHaveBeenCalledTimes(1), { timeout: 2000 });
  });

  it('does not recreate a retry timer when the application shuts down during an inventory request', async () => {
    let reject!: (e: Error) => void;
    const { svc, watchReady, stopWatch } = service(
      [],
      true,
      () =>
        new Promise((_, no) => {
          reject = no;
        }),
    );
    svc.start();
    svc.start(); // idempotent: one watch, one attempt in flight
    await vi.waitFor(() => expect(reject).toBeDefined());
    expect(watchReady).toHaveBeenCalledTimes(1);
    svc.onModuleDestroy();
    reject(new Error('agent unreachable'));
    await vi.waitFor(() => expect(svc['inFlight']).toBe(false));
    expect(svc['retryTimer']).toBeUndefined();
    expect(stopWatch).toHaveBeenCalledTimes(1);
  });
});
