import { useEffect, useRef, useState } from 'react';
import type { TopicMessage, WsStatus } from './client.js';
import { useWsClient, useWsStatus } from './WsProvider.js';

export interface UseTopicOptions<T> {
  /** Receives every buffered batch (≤ 1 per second) — for charts that need all samples. */
  onBatch?: (batch: TopicMessage<T>[]) => void;
  /** Set false to pause without unmounting (unsubscribes from the server). */
  enabled?: boolean;
}

export interface TopicState<T> {
  /** Latest payload seen for the topic (updated at most once per flush period). */
  data: T | undefined;
  /** Client time of the last update, ms since epoch. */
  updatedAt: number | undefined;
  /** Number of messages received in the last flushed batch. */
  batchSize: number;
  status: WsStatus;
}

/**
 * Subscribe to a stream topic (e.g. `iface.counters`) for the lifetime of the component.
 * Subscribes on mount / unsubscribes on unmount; the shared client handles reconnects.
 */
export function useTopic<T = unknown>(topic: string, opts: UseTopicOptions<T> = {}): TopicState<T> {
  const client = useWsClient();
  const status = useWsStatus();
  const { enabled = true } = opts;
  const onBatchRef = useRef(opts.onBatch);
  onBatchRef.current = opts.onBatch;
  const [state, setState] = useState<Omit<TopicState<T>, 'status'>>({
    data: undefined,
    updatedAt: undefined,
    batchSize: 0,
  });

  useEffect(() => {
    if (!enabled) return;
    return client.subscribe<T>(topic, (batch) => {
      const last = batch[batch.length - 1];
      if (last) setState({ data: last.data, updatedAt: last.ts, batchSize: batch.length });
      onBatchRef.current?.(batch);
    });
  }, [client, topic, enabled]);

  return { ...state, status };
}
