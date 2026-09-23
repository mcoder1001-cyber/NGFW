import { applyDecorators } from '@nestjs/common';
import { ApiBearerAuth, ApiResponse, ApiSecurity } from '@nestjs/swagger';
import { ref } from './zod.js';

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
