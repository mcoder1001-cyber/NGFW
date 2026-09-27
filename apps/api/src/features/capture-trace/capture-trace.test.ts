import { CaptureDirection } from '@ngfw/proto';
import { describe, expect, it } from 'vitest';
import { CaptureBody, CapturesOut, pointerOf, toCaptureAction, toCaptures } from './dto.js';
import { captureTraceFake, fakePcap } from './fake.js';
import { resetActionHandlersForTest } from '../../testing/fake-agent.js';

describe('F-capture-trace DTOs', () => {
  it('defaults and maps the body onto CaptureAction', () => {
    const a = toCaptureAction(
      CaptureBody.parse({ interface: 'loop501', bpf: 'icmp and host 10.0.0.1' }),
    );
    expect(a).toMatchObject({
      interface: 'loop501',
      direction: CaptureDirection.CAPTURE_DIRECTION_BOTH,
      maxPackets: 1000,
      seconds: 30,
      snaplen: 9000,
      drop: false,
      errorFilter: '',
    });
  });

  it('rejects shell-ish BPF, errorFilter without drop, out-of-range limits', () => {
    for (const bad of [
      { interface: 'x', bpf: 'icmp; reboot' },
      { interface: 'x', bpf: 'host "a"' },
      { interface: 'x', errorFilter: 'ip4-input/ttl' },
      { interface: 'x', seconds: 601 },
      { interface: 'x', snaplen: 10 },
    ]) {
      expect(CaptureBody.safeParse(bad).success, JSON.stringify(bad)).toBe(false);
    }
    const r = CaptureBody.safeParse({ interface: 'x', bpf: "';'" });
    expect(r.success ? [] : r.error.issues[0]?.path).toEqual(['bpf']);
  });

  it('maps the agent error text to a pointer', () => {
    expect(pointerOf('agent: invalid capture request: bpf: bad')).toBe('/bpf');
    expect(pointerOf('other')).toBe('');
  });

  it('maps the list with trace / PG unavailable', () => {
    const { captureList } = captureTraceFake();
    resetActionHandlersForTest();
    captureList({ request: { owner: '' } } as never, (err, res) => {
      expect(err).toBeNull();
      const out = toCaptures(res!);
      expect(CapturesOut.parse(out)).toEqual(out);
      expect(out.trace.available).toBe(false);
      expect(out.pg.reason).not.toBe('');
    });
    expect(fakePcap().readUInt32LE(0)).toBe(0xa1b2c3d4);
  });
});
