import { applyDecorators } from '@nestjs/common';
import { ApiBearerAuth, ApiOkResponse, ApiResponse, ApiSecurity } from '@nestjs/swagger';
import type { z } from 'zod';
import { openapi, ref } from './zod.js';

/**
 * OpenAPI decorators shared by every protected route: both auth schemes and the problem+json error responses
 * (RFC 9457) the guard, validation and the commit engine produce.
 */
const problem = (status: number, description: string) =>
  ApiResponse({
    status,
    description,
    content: { 'application/problem+json': { schema: ref('Problem') } },
  });

export const Protected = (...extra: number[]) =>
  applyDecorators(
    ApiBearerAuth('bearer'),
    ApiSecurity('apiKey'),
    problem(401, 'Not authenticated'),
    problem(403, 'Role too low'),
    ...extra.map((s) => problem(s, PROBLEM_TEXT[s] ?? 'Error')),
  );

export const PublicDoc = (...extra: number[]) =>
  applyDecorators(...extra.map((s) => problem(s, PROBLEM_TEXT[s] ?? 'Error')));

const PROBLEM_TEXT: Record<number, string> = {
  400: 'Invalid request or configuration (errors[] with JSON pointers)',
  401: 'Invalid credentials',
  404: 'Not found',
  409: 'Conflict (candidate locked by another user, commit pending, …)',
  422: 'The agent failed to apply; running is unchanged (per-object results)',
  429: 'Rate limited',
  501: 'Not implemented',
  502: 'Agent error',
  503: 'Agent or database unavailable',
};

/**
 * ARCH-04 (TD-15): the ONE way to document a 2xx JSON response from a Zod DTO. Always the `output` side of the schema
 * (what the handler sends after defaults/transforms), so the OpenAPI document and `Out<typeof X>` describe the same
 * shape — never `openapi(X)` (input side) for a response.
 */
export const ApiOut = (schema: z.ZodType, description?: string) =>
  ApiOkResponse({
    ...(description !== undefined ? { description } : {}),
    schema: openapi(schema, 'output'),
  });

/** A value as JSON serialisation sends it: a `Date` leaves as an ISO string. */
export type Wire<T> = T extends Date
  ? string
  : T extends readonly (infer U)[]
    ? Wire<U>[]
    : T extends object
      ? { [K in keyof T]: Wire<T[K]> }
      : T;

/** The response type of a DTO schema (z.output — the single source of the documented shape). */
export type Out<S extends z.ZodType> = z.output<S>;

/**
 * Compile-time drift guard (ARCH-04): `true` only when the internal type `T` (what the handler returns, serialised)
 * and the DTO schema `S` have exactly the same top-level keys. Use as `const _x: SameKeys<Revision, typeof RevOut> =
 * true;` — a field added to the repo row but not to the documented DTO (or vice versa) fails `tsc`.
 */
export type SameKeys<T, S extends z.ZodType> = [Exclude<keyof Wire<T>, keyof Out<S>>] extends [
  never,
]
  ? [Exclude<keyof Out<S>, keyof Wire<T>>] extends [never]
    ? true
    : { missingFromInternal: Exclude<keyof Out<S>, keyof Wire<T>> }
  : { undocumented: Exclude<keyof Wire<T>, keyof Out<S>> };
