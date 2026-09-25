import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { startHarness, type Harness } from '../support/harness.js';

/**
 * F-host-stack on the host PostgreSQL with the fake agent: config through the pointer route, commit, the live
 * `/api/v1/state/host-stack` view, and the 400 problem+json pointers of the D-049/D-051 rules. Slot names: w1-*,
 * prefixes in 10.1.0.0/16, ports 3190–3199.
 */
describe('host-stack e2e (PostgreSQL + fake agent)', () => {
  let h: Harness;
  let admin: string;
  const mp = { 'content-type': 'application/merge-patch+json' };
  const HS = {
    enabled: true,
    namespaces: { 'w1-app': { vrf: 'default' } },
    sessionRules: [
      {
        tag: 'w1-deny',
        transport: 'tcp',
        local: '10.1.1.0/24',
        localPort: 3190,
        remote: '10.1.2.0/24',
        action: 'deny',
      },
    ],
  };

  beforeAll(async () => {
    h = await startHarness({});
    admin = await h.login('admin', h.adminPassword);
  });
  afterAll(async () => h?.close());

  it('commits services.hostStack and reports it under /state/host-stack', async () => {
    expect((await h.call(admin, 'PUT', '/api/v1/config/services/hostStack', HS)).status).toBe(200);
    expect((await h.call(admin, 'POST', '/api/v1/config/commit?comment=host-stack')).status).toBe(
      200,
    );
    const st = await h.call(admin, 'GET', '/api/v1/state/host-stack');
    expect(st.status).toBe(200);
    expect(st.body).toMatchObject({ sessionEnabled: true, namespaces: ['w1-app'], ruleCount: 1 });
  });

  it('inline secret and `..` in wwwRootPath → 400 problem+json with a pointer', async () => {
    const bad1 = await h.call(
      admin,
      'PATCH',
      '/api/v1/config/services/hostStack',
      { namespaces: { 'w1-app': { vrf: 'default', secretRef: 'hunter2' } } },
      mp,
    );
    expect(bad1.status).toBe(400);
    expect(JSON.stringify(bad1.body)).toContain('/services/hostStack/namespaces/w1-app/secretRef');
    const bad2 = await h.call(
      admin,
      'PATCH',
      '/api/v1/config/services/hostStack',
      {
        httpStatic: {
          enabled: true,
          wwwRootPath: '/var/lib/vrx/www/../x',
          uri: 'tcp://10.1.1.1/80',
        },
      },
      mp,
    );
    expect(bad2.status).toBe(400);
    expect(JSON.stringify(bad2.body)).toContain('/services/hostStack/httpStatic/wwwRootPath');
  });
});
