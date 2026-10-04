import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import type { HostNic } from '@ngfw/proto';
import { AgentClient } from '../../src/agent/agent.client.js';
import { SystemEventsService } from '../../src/audit/system-events.service.js';
import { CommitService } from '../../src/commit/commit.service.js';
import { DatastoreService } from '../../src/datastore/datastore.service.js';
import { SeedService } from '../../src/features/default-vpp-nics/index.js';
import { startHarness, type Harness } from '../support/harness.js';

const MP = { 'content-type': 'application/merge-patch+json' };

// ngfw-a-like inventory: ens192 is the management NIC, ens161/ens193 are dataplane NICs.
const MGMT_PCI = '0000:0b:00.0';
const D1_PCI = '0000:04:00.0';
const D2_PCI = '0000:0c:00.0';
const NICS: HostNic[] = [
  {
    netdev: 'ens192',
    pci: MGMT_PCI,
    driver: 'vmxnet3',
    mac: '00:0c:29:00:00:92',
    isManagement: true,
    boundToDpdk: false,
    linkUp: true,
  },
  {
    netdev: 'ens161',
    pci: D1_PCI,
    driver: 'vmxnet3',
    mac: '00:0c:29:00:00:61',
    isManagement: false,
    boundToDpdk: false,
    linkUp: true,
  },
  {
    netdev: 'ens193',
    pci: D2_PCI,
    driver: 'vmxnet3',
    mac: '00:0c:29:00:00:93',
    isManagement: false,
    boundToDpdk: false,
    linkUp: false,
  },
];

/**
 * F-default-vpp-nics (D-164) e2e on the host PostgreSQL with the fake agent: first-boot seeding (revision 1 from the
 * HostNics inventory), idempotency, the physical-NIC delete guard (403 on PATCH-null / DELETE / import-without-it) and
 * release (owner → host keeps pciWhitelist consistent).
 */
describe('default-vpp-nics e2e (PostgreSQL + fake agent)', () => {
  let h: Harness;
  let admin: string;

  beforeAll(async () => {
    h = await startHarness({ NGFW_SEED_DEFAULT_NICS: '1' });
    h.fake.hostNicList = NICS;
    h.fake.hostNicNotes = ['ens192 → 0000:0b:00.0 (default route)'];
    // the data NICs are not bound to the data plane yet: VPP has no such live interface (awaiting dataplane)
    h.fake.liveMissing = new Set(['ens161', 'ens193']);
    admin = await h.login('admin', h.adminPassword);
  });
  afterAll(async () => h?.close());

  it('seeds revision 1 from the host inventory and is idempotent', async () => {
    const seed = h.app.get(SeedService);
    expect(await seed.seedIfNeeded()).toBe('seeded');
    // a second start never re-seeds (no new revision)
    expect(await seed.seedIfNeeded()).not.toBe('seeded');
    const revs = await h.call(admin, 'GET', '/api/v1/config/revisions');
    expect(revs.body.items).toHaveLength(1);
    // an API restart on the same database (a fresh service instance) finds the revision and never re-seeds
    const fresh = new SeedService(
      h.app.get(AgentClient),
      h.app.get(DatastoreService),
      h.app.get(CommitService),
      h.app.get(SystemEventsService),
      h.env,
    );
    expect(await fresh.seedIfNeeded()).toBe('exists');
    expect((await h.call(admin, 'GET', '/api/v1/config/revisions')).body.items).toHaveLength(1);
    // the seed is audited as system.seed-defaults
    const ev = await h.call(admin, 'GET', '/api/v1/state/events');
    expect(
      (ev.body.items as { code: string }[]).some((e) => e.code === 'system.seed-defaults'),
    ).toBe(true);

    const run = await h.call(admin, 'GET', '/api/v1/config');
    expect(run.status).toBe(200);
    const doc = run.body as {
      interfaces: Record<
        string,
        { enabled: boolean; physical?: { pci: string; owner: string; builtIn: boolean } }
      >;
      dataplane: {
        managementPci: string[];
        pciWhitelist: string[];
        devices: Record<string, { name: string }>;
      };
    };
    // both data NICs are seeded as built-in dataplane interfaces; the management NIC is not an interface
    expect(doc.interfaces['ens161']).toMatchObject({
      enabled: true,
      physical: { pci: D1_PCI, owner: 'dataplane', builtIn: true },
    });
    expect(doc.interfaces['ens193']?.physical?.owner).toBe('dataplane');
    expect(doc.interfaces['ens192']).toBeUndefined();
    expect(doc.dataplane.managementPci).toEqual([MGMT_PCI]);
    expect(doc.dataplane.pciWhitelist.sort()).toEqual([D1_PCI, D2_PCI].sort());
    expect(doc.dataplane.devices[D1_PCI]).toEqual({ name: 'ens161' });

    // the /state/interfaces view marks the built-in rows and (no live VPP interface) awaiting dataplane
    const st = await h.call(admin, 'GET', '/api/v1/state/interfaces');
    expect(st.status).toBe(200);
    const ens161 = (
      st.body.items as {
        name: string;
        builtIn?: boolean;
        awaitingDataplane?: boolean;
        physical?: unknown;
      }[]
    ).find((i) => i.name === 'ens161');
    expect(ens161).toMatchObject({ builtIn: true, awaitingDataplane: true });
  });

  it('refuses to delete a physical NIC (PATCH-null, DELETE and import), 403 with its pointer', async () => {
    const patch = await h.call(admin, 'PATCH', '/api/v1/config/interfaces', { ens161: null }, MP);
    expect(patch.status).toBe(403);
    expect(patch.body).toMatchObject({
      type: expect.stringContaining('interfaces.physical-nic-not-deletable'),
      errors: [{ pointer: '/interfaces/ens161' }],
    });

    const del = await h.call(admin, 'DELETE', '/api/v1/config/interfaces/ens161');
    expect(del.status).toBe(403);
    expect(del.body.errors?.[0]?.pointer).toBe('/interfaces/ens161');

    // import a document that omits the physical row → 403 (re-adding a deleted-by-import document)
    const run = await h.call(admin, 'GET', '/api/v1/config');
    const without = structuredClone(run.body);
    delete (without as { interfaces: Record<string, unknown> }).interfaces['ens161'];
    const imp = await h.call(admin, 'POST', '/api/v1/config/import', without);
    expect(imp.status).toBe(403);
    expect(
      imp.body.errors?.some((e: { pointer: string }) => e.pointer === '/interfaces/ens161'),
    ).toBe(true);
  });

  it('refuses a user edit that adds the physical marker or changes its pci/builtIn (review R2R4 #4)', async () => {
    const add = await h.call(
      admin,
      'PATCH',
      '/api/v1/config/interfaces',
      { loop5: { physical: { pci: '0000:99:00.0' } } },
      MP,
    );
    expect(add.status).toBe(403);
    expect(add.body).toMatchObject({
      type: expect.stringContaining('interfaces.physical-marker-readonly'),
      errors: [{ pointer: '/interfaces/loop5/physical' }],
    });
    const pci = await h.call(
      admin,
      'PATCH',
      '/api/v1/config/interfaces',
      { ens193: { physical: { pci: '0000:99:00.0' } } },
      MP,
    );
    expect(pci.status).toBe(403);
    expect(pci.body.errors?.[0]?.pointer).toBe('/interfaces/ens193/physical/pci');
    const builtIn = await h.call(
      admin,
      'PATCH',
      '/api/v1/config/interfaces',
      { ens193: { physical: { builtIn: false } } },
      MP,
    );
    expect(builtIn.status).toBe(403);
    expect(builtIn.body.errors?.[0]?.pointer).toBe('/interfaces/ens193/physical/builtIn');
  });

  it('refuses a PUT of an interface without its physical marker (review R1R3 #7)', async () => {
    const put = await h.call(admin, 'PUT', '/api/v1/config/interfaces/ens161', { enabled: true });
    expect(put.status).toBe(403);
    expect(put.body.errors?.[0]?.pointer).toBe('/interfaces/ens161');
  });

  it('refuses PUT and import that create or alter the marker: 403 physical-marker-readonly (re-review MINOR 3)', async () => {
    const put = await h.call(admin, 'PUT', '/api/v1/config/interfaces/loop5', {
      enabled: true,
      physical: { pci: '0000:99:00.0' },
    });
    expect(put.status).toBe(403);
    expect(put.body).toMatchObject({
      type: expect.stringContaining('interfaces.physical-marker-readonly'),
      errors: [{ pointer: '/interfaces/loop5/physical' }],
    });

    const run = await h.call(admin, 'GET', '/api/v1/config');
    const doc = structuredClone(run.body) as {
      interfaces: Record<string, { physical?: { pci: string } } & Record<string, unknown>>;
    };
    doc.interfaces['ens193']!.physical!.pci = '0000:99:00.0';
    const imp = await h.call(admin, 'POST', '/api/v1/config/import', doc);
    expect(imp.status).toBe(403);
    expect(imp.body).toMatchObject({
      type: expect.stringContaining('interfaces.physical-marker-readonly'),
      errors: [{ pointer: '/interfaces/ens193/physical/pci' }],
    });
  });

  it('allows disabling a physical NIC (it is not a deletion)', async () => {
    const patch = await h.call(
      admin,
      'PATCH',
      '/api/v1/config/interfaces',
      { ens161: { enabled: false } },
      MP,
    );
    expect(patch.status).toBe(200);
    await h.call(admin, 'POST', '/api/v1/config/discard');
  });

  it('releases a NIC to the host and removes it from the DPDK whitelist', async () => {
    const body = {
      interfaces: { ens161: { physical: { owner: 'host' } } },
      dataplane: { pciWhitelist: [D2_PCI], devices: { [D1_PCI]: null } },
    };
    const rel = await h.call(admin, 'PATCH', '/api/v1/config', body, MP);
    expect(rel.status).toBe(200);
    const commit = await h.call(admin, 'POST', '/api/v1/config/commit?comment=release-ens161');
    expect(commit.status).toBe(200);

    const run = await h.call(admin, 'GET', '/api/v1/config');
    const doc = run.body as {
      interfaces: Record<string, { physical?: { owner: string } }>;
      dataplane: { pciWhitelist: string[]; devices: Record<string, unknown> };
    };
    expect(doc.interfaces['ens161']?.physical?.owner).toBe('host');
    expect(doc.dataplane.pciWhitelist).toEqual([D2_PCI]);
    expect(doc.dataplane.devices[D1_PCI]).toBeUndefined();
    // the released NIC is still a (non-deletable) physical row
    expect(doc.interfaces['ens161']?.physical).toBeDefined();
  });
});
