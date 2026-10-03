import { EventEmitter } from 'node:events';
import { status as GrpcStatus } from '@grpc/grpc-js';
import { describe, expect, it, vi } from 'vitest';
import type { CaptureAction } from '@ngfw/proto';
import { AgentClient } from './agent.client.js';
import { testEnv } from '../testing/fixtures.js';

function fixture() {
  const stream = new EventEmitter();
  const cancel = vi.fn(() => stream.emit('error', { code: GrpcStatus.CANCELLED }));
  Object.assign(stream, { cancel });
  const close = vi.fn();
  const agent = new AgentClient(testEnv());
  Reflect.set(agent, 'client', { action: vi.fn(() => stream), close });
  const list = vi.spyOn(agent, 'captureList').mockResolvedValue({ captures: [] } as never);
  return { agent, stream, cancel, close, list };
}
const plan = { interface: 'any' } as CaptureAction;

describe('capture stream lifecycle', () => {
  it('cancels the exact held capture and refuses a second stop after completion', async () => {
    const { agent, stream, cancel, list } = fixture();
    const started = agent.startCapture(plan);
    stream.emit('data', { line: 'capture w5-active started' });
    expect(await started).toBe('w5-active');
    await agent.stopCapture('w5-active');
    expect(cancel).toHaveBeenCalledTimes(1);
    expect(list).not.toHaveBeenCalled();
    list.mockResolvedValue({ captures: [{ id: 'w5-active', state: 'done' }] } as never);
    await expect(agent.stopCapture('w5-active')).rejects.toMatchObject({ status: 409 });
    agent.close();
  });
  it('distinguishes unknown captures from captures running in another API process', async () => {
    const { agent, cancel, list } = fixture();
    await expect(agent.stopCapture('missing')).rejects.toMatchObject({ status: 404 });
    list.mockResolvedValue({ captures: [{ id: 'foreign', state: 'running' }] } as never);
    await expect(agent.stopCapture('foreign')).rejects.toMatchObject({ status: 409 });
    expect(cancel).not.toHaveBeenCalled();
    agent.close();
  });
  it('releases completed streams and cancels outstanding streams on API shutdown', async () => {
    const { agent, stream, cancel, list, close } = fixture();
    let started = agent.startCapture(plan);
    stream.emit('data', { line: 'capture complete started' });
    await started;
    stream.emit('end');
    await expect(agent.stopCapture('complete')).rejects.toMatchObject({ status: 404 });
    expect(list).toHaveBeenCalled();
    started = agent.startCapture(plan);
    stream.emit('data', { line: 'capture active started' });
    await started;
    agent.close();
    expect(cancel).toHaveBeenCalledTimes(1);
    expect(close).toHaveBeenCalledTimes(1);
  });
  it('logs a late failure using only the capture id and status code', async () => {
    const { agent, stream } = fixture();
    const warning = vi
      .spyOn(Reflect.get(agent, 'captureLog'), 'warn')
      .mockImplementation(() => undefined);
    const started = agent.startCapture(plan);
    stream.emit('data', { line: 'capture late started' });
    await started;
    stream.emit('error', {
      code: GrpcStatus.UNAVAILABLE,
      details: 'private path must not be echoed',
    });
    expect(warning).toHaveBeenCalledWith({
      captureId: 'late',
      grpcCode: GrpcStatus.UNAVAILABLE,
      message: 'Capture stream failed after start',
    });
    agent.close();
  });
});
