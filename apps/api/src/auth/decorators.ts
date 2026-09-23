import { SetMetadata } from '@nestjs/common';
import type { Role } from '../db/schema.js';

export const PUBLIC_KEY = 'vrx:public';
export const ROLE_KEY = 'vrx:role';
export const NO_AUDIT_KEY = 'vrx:no-audit';

/**
 * No authentication. Only login/refresh/logout and liveness use it; the route-guard test pins the exact list, so a
 * new @Public() route fails the test until it is reviewed.
 */
export const Public = () => SetMetadata(PUBLIC_KEY, true);

/**
 * Minimum role for a route. Without it: GET/HEAD need `readonly`, every other method `operator` (P06 §6).
 */
export const MinRole = (role: Role) => SetMetadata(ROLE_KEY, role);

/** The handler writes its own audit entries (auth endpoints: the body carries credentials). */
export const NoAudit = () => SetMetadata(NO_AUDIT_KEY, true);
