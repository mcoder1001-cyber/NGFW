import { afterEach, describe, expect, it, vi } from 'vitest';
import { NotificationsSchema } from '@ngfw/schema';
import { Bus } from '../../infra/bus.js';
import { NotificationsService } from './notifications.service.js';
import { DeliveryError, publicAddress } from './transport.js';
import { ValidationService } from '../../commit/validation.service.js';
import { DesiredState, EventKind, Event } from '@ngfw/proto';
import type { DatastoreService } from '../../datastore/datastore.service.js';
import type { Db } from '../../db/db.js';
import type { ModuleRef } from '@nestjs/core';
const config = () =>
  NotificationsSchema.parse({
    channels: [
      {
        name: 'sink',
        type: 'webhook',
        webhook: { url: 'https://example.com/hook', secretRef: 'token/sink' },
      },
    ],
    rules: [
      {
        name: 'test',
        events: ['alarm', 'commit', 'link', 'vpn', 'global-blocking'],
        channels: ['sink'],
        throttleSec: 1,
        minSeverity: 'info',
      },
    ],
  });
const services: NotificationsService[] = [];
function service(
  ds = { getRunning: async () => ({ doc: { management: { notifications: config() } } }) },
) {
  const bus = new Bus();
  const s = new NotificationsService(
    ds as unknown as DatastoreService,
    bus,
    {} as Db,
    {} as ModuleRef,
  );
  services.push(s);
  s.configure(config());
  s.deliver = vi.fn(async () => undefined);
  return { s, bus };
}
afterEach(() => {
  services.forEach((s) => s.onModuleDestroy());
  services.length = 0;
  vi.useRealTimers();
});
describe('notification dispatcher', () => {
  it('100 event alarm storm throttles to one job and only fixed scalars leave', async () => {
    const { s, bus } = service();
    for (let i = 0; i < 100; i++)
      bus.publish('alarm.events', {
        rule: 'cpu',
        type: 'raised',
        severity: 'critical',
        message: 'VRX_TEST_PSK_sensitive',
        password: 'VRX_TEST_PSK_sensitive',
      });
    expect(s.state().queued).toBe(1);
    await s.run();
    expect(s.deliver).toHaveBeenCalledTimes(1);
    expect(JSON.stringify(vi.mocked(s.deliver).mock.calls)).not.toContain('sensitive');
    expect(s.state().deliveries[0]?.result).toBe('sent');
  });
  it('retries are bounded/backed off and provider error payload is redacted', async () => {
    const { s } = service();
    let clock = 1000;
    s.now = () => clock;
    s.deliver = vi.fn(async () => {
      throw new Error('VRX_TEST_PSK_sensitive https://user:pass@private/');
    });
    s.emit('alarm', 'warning', 'cpu', 'raised');
    await s.run();
    expect(s.state().queued).toBe(1);
    await s.run();
    expect(s.deliver).toHaveBeenCalledTimes(1);
    clock += 1000;
    await s.run();
    clock += 2000;
    await s.run();
    expect(s.deliver).toHaveBeenCalledTimes(3);
    expect(s.state().queued).toBe(0);
    expect(JSON.stringify(s.state())).not.toContain('sensitive');
  });
  it('queued events revalidate rule/channel/events/severity, including a rule literally named test', async () => {
    const { s } = service();
    s.emit('alarm', 'warning', 'cpu', 'raised');
    const c = config();
    c.rules[0]!.events = ['commit'];
    s.configure(c);
    await s.run();
    expect(s.deliver).not.toHaveBeenCalled();
    expect(s.state().deliveries[0]?.result).toBe('discarded');
  });
  it('test cannot use candidate-only/disabled channels and is independently throttled', () => {
    const { s } = service();
    expect(() => s.test('missing')).toThrow();
    s.test('sink');
    expect(() => s.test('sink')).toThrow();
    const c = config();
    c.channels[0]!.enabled = false;
    s.configure(c);
    expect(() => s.test('sink')).toThrow();
  });
  it('queue/history/dedup stay bounded under unique source storm', async () => {
    const { s } = service();
    let clock = 1000;
    s.now = () => clock;
    for (let i = 0; i < 1000; i++) {
      clock += 1001;
      s.emit('link', 'warning', `iface${i}`, 'down');
    }
    expect(s.state().queued).toBe(256);
    for (let i = 0; i < 600; i++) {
      await s.run();
      clock += 1001;
      s.emit('link', 'warning', `extra${i}`, 'down');
    }
    expect(s.state().deliveries).toHaveLength(500);
    expect(s.state().queued).toBeLessThanOrEqual(256);
  });
  it('configuration reload cannot bypass the per-channel test throttle', async () => {
    const { s } = service();
    let clock = 1000;
    s.now = () => clock;
    s.test('sink');
    await s.reload();
    expect(() => s.test('sink')).toThrow();
    clock += 10000;
    expect(s.test('sink')).toEqual({ queued: true });
  });
  it('reload commit storm has one outstanding read plus one latest follow-up; shutdown ignores late data', async () => {
    let resolve!: (v: { doc: unknown }) => void;
    const read = vi.fn(
      () =>
        new Promise<{ doc: unknown }>((r) => {
          resolve = r;
        }),
    );
    const { s } = service({ getRunning: read } as never);
    const first = s.reload();
    for (let i = 0; i < 1000; i++) void s.reload();
    expect(read).toHaveBeenCalledTimes(1);
    resolve({ doc: { management: { notifications: config() } } });
    await Promise.resolve();
    await Promise.resolve();
    expect(read).toHaveBeenCalledTimes(2);
    s.onModuleDestroy();
    resolve({ doc: { management: { notifications: config() } } });
    await first;
    expect(s.state().queued).toBe(0);
  });
  it('preserves config while excluding API-owned notifications from agent desired state', () => {
    const doc = {
      management: {
        notifications: config(),
        syslog: [{ address: '192.0.2.2', port: 514, protocol: 'udp' }],
      },
    };
    const result = DesiredState.toJSON(ValidationService.desiredState(doc)) as {
      management?: { notifications?: unknown; syslog?: unknown[] };
    };
    expect(result.management?.notifications).toBeUndefined();
    expect(result.management?.syslog).toHaveLength(1);
    expect(doc.management.notifications).toEqual(config());
  });
  it('maps link/WG/global-blocking events without raw message/attribute leakage', async () => {
    const { s, bus } = service();
    let clock = 1000;
    s.now = () => clock;
    bus.agentEvent(
      Event.fromPartial({
        kind: EventKind.EVENT_KIND_LINK_DOWN,
        interface: 'wan',
        message: 'VRX_TEST_PSK_sensitive',
        attributes: { private_key: 'VRX_TEST_PSK_sensitive' },
      }),
    );
    clock += 1001;
    bus.agentEvent(
      Event.fromPartial({
        kind: EventKind.EVENT_KIND_WIREGUARD_PEER_CHANGED,
        interface: 'wg0',
        attributes: { dead: 'true', private_key: 'VRX_TEST_PSK_sensitive' },
      }),
    );
    clock += 1001;
    bus.publish('security.events', {
      type: 'global-blocking-fetch-failed',
      list: 'threats',
      reason: 'VRX_TEST_PSK_sensitive',
    });
    await s.run();
    await s.run();
    await s.run();
    expect(s.deliver).toHaveBeenCalledTimes(3);
    expect(JSON.stringify(vi.mocked(s.deliver).mock.calls)).not.toContain('sensitive');
  });
  it('one worker stays busy during a blocked delivery and cannot accumulate parallel sends', async () => {
    let resolve!: () => void;
    const { s } = service();
    let clock = 1000;
    s.now = () => clock;
    s.deliver = vi.fn(
      () =>
        new Promise<void>((r) => {
          resolve = r;
        }),
    );
    s.emit('link', 'warning', 'wan', 'down');
    const pending = s.run();
    clock += 1001;
    s.emit('link', 'warning', 'lan', 'down');
    await s.run();
    expect(s.deliver).toHaveBeenCalledTimes(1);
    resolve();
    await pending;
  });
});
describe('webhook address safety', () => {
  it.each([
    '127.0.0.1',
    '10.0.0.2',
    '169.254.169.254',
    '172.16.0.1',
    '192.168.1.1',
    '100.64.0.1',
    '::1',
    '0:0:0:0:0:0:0:1',
    '::ffff:127.0.0.1',
    '::ffff:7f00:1',
    'fe80::1',
    'fc00::1',
    '2001:db8::1',
    'ff00::1',
    '2002:7f00:1::',
  ])('rejects %s', (ip) => expect(publicAddress(ip)).toBe(false));
  it.each(['8.8.8.8', '1.1.1.1', '2606:4700:4700::1111', '2001:4860:4860::8888'])(
    'accepts global %s',
    (ip) => expect(publicAddress(ip)).toBe(true),
  );
  it('only delivery error reason is safe to expose', () =>
    expect(new DeliveryError('smtp-failed').message).toBe('smtp-failed'));
});
