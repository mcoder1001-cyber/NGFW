import { credentials, status, type ClientReadableStream, type ServiceError } from '@grpc/grpc-js';
import { DataplaneClient, type ActionOutput } from '@ngfw/proto';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { afterAll, afterEach, beforeEach, describe, expect, it } from 'vitest';
import {
  FakeAgent,
  registerActionHandler,
  resetActionHandlersForTest,
  type ActionHandler,
} from './fake-agent.js';

/**
 * Unit tests of the generic Action dispatch table (D-134/TD-23): a registered handler runs for its
 * oneof case, an unregistered case answers UNIMPLEMENTED, a handler that throws synchronously or
 * rejects asynchronously still ends the call with a status — all over a real gRPC connection, so a
 * regression to `call.destroy()` (which never reaches the client — the bug F-nat44-ed-sessions found)
 * shows up as a failure, not a pass. Every `action(...)` call below sets an explicit short deadline
 * (fix round 1, F2) so a hang fails in ~2 s with a specific DEADLINE_EXCEEDED, not Vitest's global
 * 30 s timeout (`apps/api/vitest.config.ts`) with a generic message.
 */
const DEADLINE_MS = 2000;

function collect(
  stream: ClientReadableStream<ActionOutput>,
): Promise<{ chunks: ActionOutput[]; err?: ServiceError }> {
  return new Promise((resolve) => {
    const chunks: ActionOutput[] = [];
    stream.on('data', (d: ActionOutput) => chunks.push(d));
    stream.on('error', (err: ServiceError) => resolve({ chunks, err }));
    stream.on('end', () => resolve({ chunks }));
  });
}

describe('FakeAgent action dispatch', () => {
  const dir = mkdtempSync(join(tmpdir(), 'vrx-td23-'));
  const socket = join(dir, 'agent.sock');
  let fake: FakeAgent;
  let client: DataplaneClient;

  beforeEach(async () => {
    fake = new FakeAgent({ owner: 'w1' });
    await fake.start(socket);
    resetActionHandlersForTest(); // only this file's stubs: feature fakes register theirs when the fake agent starts
    client = new DataplaneClient(`unix:${socket}`, credentials.createInsecure());
  });

  afterEach(async () => {
    client.close();
    await fake.stop();
    resetActionHandlersForTest();
  });

  afterAll(() => {
    rmSync(dir, { recursive: true, force: true });
  });

  it('dispatches to the handler registered for the request kind', async () => {
    const seen: unknown[] = [];
    const ping: ActionHandler = (call) => {
      seen.push(call.request.ping?.target);
      call.write({ line: `PING ${call.request.ping?.target}` });
      call.write({ done: { summary: '1 sent, 1 received', exitCode: 0, stats: {} } });
      call.end();
    };
    registerActionHandler('ping', ping);

    const stream = client.action(
      {
        ping: {
          target: '10.0.0.1',
          vrf: '',
          count: 1,
          size: 0,
          intervalMs: 0,
          timeoutMs: 0,
          source: '',
        },
      },
      { deadline: Date.now() + DEADLINE_MS },
    );
    const { chunks, err } = await collect(stream);

    expect(err).toBeUndefined();
    expect(seen).toEqual(['10.0.0.1']);
    expect(chunks[0]?.line).toBe('PING 10.0.0.1');
    expect(chunks[1]?.done?.summary).toBe('1 sent, 1 received');
    expect(fake.calls.some((c) => c.method === 'Action')).toBe(true);
  });

  it('answers an unregistered kind with UNIMPLEMENTED, not a hung call', async () => {
    const stream = client.action(
      {
        traceroute: {
          target: '10.0.0.1',
          vrf: '',
          maxHops: 0,
          probes: 0,
          timeoutMs: 0,
          source: '',
        },
      },
      { deadline: Date.now() + DEADLINE_MS },
    );
    const { chunks, err } = await collect(stream);

    expect(chunks).toHaveLength(0);
    expect(err).toBeDefined();
    expect(err?.code).toBe(status.UNIMPLEMENTED);
  });

  it('ends the call with a status when a handler throws synchronously, instead of hanging', async () => {
    const capture: ActionHandler = () => {
      throw new Error('boom');
    };
    registerActionHandler('capture', capture);

    const stream = client.action(
      {
        capture: {
          interface: 'loop0',
          bpf: '',
          maxPackets: 0,
          seconds: 0,
          direction: 0,
          snaplen: 0,
        },
      },
      { deadline: Date.now() + DEADLINE_MS },
    );
    const { err } = await collect(stream);

    expect(err).toBeDefined();
    expect(err?.code).toBe(status.INTERNAL);
    expect(err?.details).toContain('boom');
  });

  it('ends the call with a status when a handler rejects asynchronously, instead of hanging (F3)', async () => {
    const traceroute: ActionHandler = async () => {
      await Promise.resolve(); // force a microtask turn, so this really is a post-`await` rejection
      throw new Error('async boom');
    };
    registerActionHandler('traceroute', traceroute);

    const stream = client.action(
      {
        traceroute: {
          target: '10.0.0.1',
          vrf: '',
          maxHops: 0,
          probes: 0,
          timeoutMs: 0,
          source: '',
        },
      },
      { deadline: Date.now() + DEADLINE_MS },
    );
    const { err } = await collect(stream);

    expect(err).toBeDefined();
    expect(err?.code).toBe(status.INTERNAL);
    expect(err?.details).toContain('async boom');
  });

  it('registerActionHandler throws on a duplicate kind instead of silently replacing it', () => {
    const first: ActionHandler = (call) => call.end();
    const second: ActionHandler = (call) => call.end();
    registerActionHandler('ping', first);
    expect(() => registerActionHandler('ping', second)).toThrow(/already registered/);
  });
});
