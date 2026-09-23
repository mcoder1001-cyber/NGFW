import type { Principal } from '../common/principal.js';
import { loadEnv, type Env } from '../config.js';

/** Test principals and env shared by unit tests. Hash placeholders follow docs/contracts/schema.md (never real). */
export const ADMIN: Principal = { id: 1, username: 'admin', role: 'admin', via: 'jwt' };
export const OPERATOR: Principal = { id: 2, username: 'op', role: 'operator', via: 'jwt' };
export const READONLY: Principal = { id: 3, username: 'ro', role: 'readonly', via: 'jwt' };

export const TEST_HASH = '$vrx-test$VRX_TEST_HASH_P06';

export function testEnv(extra: Record<string, string> = {}): Env {
  return loadEnv({ VRX_LOCK_TTL_SEC: '60', ...extra });
}
