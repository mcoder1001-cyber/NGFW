import { Valkey as ValkeyClient } from 'iovalkey';
import type { Env } from '../config.js';

/** DI token of the Valkey client (rate limits, refresh-token families). */
export const VALKEY = Symbol('NGFW_VALKEY');

/**
 * Valkey on localhost (deploy/dev/README.md). Shared host: every key carries `NGFW_VALKEY_PREFIX` (`ngfw:w<N>:`) and
 * lives in logical db `NGFW_VALKEY_DB`; the API never issues FLUSHALL/FLUSHDB. Lazy connect: building the application
 * does not open a socket.
 */
export function createValkey(env: Env): ValkeyClient {
  const client = new ValkeyClient(env.NGFW_VALKEY_URL, {
    db: env.NGFW_VALKEY_DB,
    keyPrefix: env.NGFW_VALKEY_PREFIX,
    lazyConnect: true,
    maxRetriesPerRequest: 2,
    connectTimeout: 3000,
    retryStrategy: (n: number) => Math.min(n * 200, 3000),
  });
  // connection errors surface on the commands; do not crash the process on the 'error' event
  client.on('error', () => undefined);
  return client;
}

export type Valkey = ValkeyClient;
