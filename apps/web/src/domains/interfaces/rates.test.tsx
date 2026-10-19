import { StrictMode, type PropsWithChildren } from 'react';
import { act, renderHook } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { useIfaceRates, type WsCountersData } from './rates';

const stream = vi.hoisted(() => ({
  onBatch: undefined as undefined | ((batch: { data: WsCountersData; ts: number }[]) => void),
}));
vi.mock('@ngfw/ui-kit/ws', () => ({
  useTopic: (_topic: string, options: { onBatch: typeof stream.onBatch }) => {
    stream.onBatch = options.onBatch;
    return { status: 'live' };
  },
}));

const sample = (second: number) => ({
  ts: second * 1000,
  data: {
    interfaces: [
      {
        name: 'eth0',
        rxBytes: String(second * 100),
        txBytes: String(second * 200),
        rxPackets: String(second * 10),
        txPackets: String(second * 20),
      },
    ],
  },
});

describe('live interface rates', () => {
  it('retains every sample when React StrictMode replays state updaters', () => {
    const { result } = renderHook(useIfaceRates, {
      wrapper: ({ children }: PropsWithChildren) => <StrictMode>{children}</StrictMode>,
    });
    for (let second = 1; second <= 5; second++) {
      act(() => stream.onBatch?.([sample(second)]));
    }
    expect(result.current.rates.get('eth0')).toEqual({
      rxBps: 800,
      txBps: 1600,
      rxPps: 10,
      txPps: 20,
      history: [30, 30, 30, 30],
    });
  });
  it('processes multiple queued batches without dropping history', () => {
    const { result } = renderHook(useIfaceRates);
    act(() => {
      stream.onBatch?.([sample(1), sample(2)]);
      stream.onBatch?.([sample(3), sample(4)]);
    });
    expect(result.current.rates.get('eth0')?.history).toEqual([30, 30, 30]);
  });
});
