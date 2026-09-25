import { type PipeTransform, type Type } from '@nestjs/common';
import { ROUTE_ARGS_METADATA } from '@nestjs/common/constants.js';
import { describe, expect, it } from 'vitest';
import { AuthController, PasswordBody } from '../auth/auth.controller.js';
import { loadEnv } from '../config.js';
import { ProblemError } from '../common/problem.js';
import { openapi } from '../common/zod.js';
import { assertPasswordPolicy, newPassword, PASSWORD_MIN } from './password-policy.js';
import { SetPasswordBody, UsersController } from './users.controller.js';

function problemOf(fn: () => unknown): ProblemError {
  try {
    fn();
  } catch (e) {
    if (e instanceof ProblemError) return e;
    throw e;
  }
  throw new Error('no problem thrown');
}
const pointers = (e: ProblemError) => (e.errors ?? []).map((i) => i.pointer).sort();
const BODY = { type: 'body' } as const;
/**
 * The body pipe a controller method really uses (Nest's route-argument metadata), built as DI would build it for the
 * given flag — so these tests cover the controllers' wiring, not a copy of it.
 */
function routeBodyPipe(controller: Type, method: string, flag: '0' | '1'): PipeTransform {
  const args = Reflect.getMetadata(ROUTE_ARGS_METADATA, controller, method) as Record<string, { pipes: unknown[] }>;
  const pipes = Object.values(args).flatMap((a) => a.pipes);
  const Pipe = pipes.find((p) => typeof p === 'function' && (p as { envZodPipe?: boolean }).envZodPipe === true) as
    | (new (env: ReturnType<typeof loadEnv>) => PipeTransform)
    | undefined;
  if (Pipe === undefined) throw new Error(`${controller.name}.${method}: no EnvZodPipe on the body`);
  return new Pipe(loadEnv({ VRX_DEV_WEAK_PASSWORDS: flag }));
}
const authPipe = (flag: '0' | '1') => routeBodyPipe(AuthController, 'password', flag);
const usersPipe = (flag: '0' | '1') => routeBodyPipe(UsersController, 'setPassword', flag);

describe('password policy', () => {
  it('assertPasswordPolicy: 400 on /password below PASSWORD_MIN unless dev mode', () => {
    const e = problemOf(() => assertPasswordPolicy('x'.repeat(PASSWORD_MIN - 1), false));
    expect(e.getStatus()).toBe(400);
    expect(pointers(e)).toEqual(['/password']);
    expect(() => assertPasswordPolicy('x'.repeat(PASSWORD_MIN), false)).not.toThrow();
    expect(() => assertPasswordPolicy('a', true)).not.toThrow();
  });

  it('newPassword: the product rule, or any non-empty value in dev mode', () => {
    expect(newPassword().safeParse('x'.repeat(PASSWORD_MIN - 1)).success).toBe(false);
    expect(newPassword(true).safeParse('a').success).toBe(true);
    expect(newPassword(true).safeParse('').success).toBe(false);
    expect(newPassword(true).safeParse('x'.repeat(1025)).success).toBe(false);
  });

  it('flag off: the route pipes parse exactly the product schema — every issue in one 400, as before', () => {
    // /auth/password: a short password AND a missing `current` are both reported
    const auth = problemOf(() => authPipe('0').transform({ password: 'short' }, BODY));
    expect(pointers(auth)).toEqual(['/current', '/password']);
    // /users/{name}/password: a short password AND an unknown key are both reported
    const users = problemOf(() =>
      usersPipe('0').transform({ password: 'short', bogus: 1 }, BODY),
    );
    expect(pointers(users)).toContain('/password');
    expect(pointers(users).length).toBeGreaterThan(1);
  });

  it('flag on: short passwords pass the pipes; strictness, `current` and the empty-password rule stay', () => {
    const auth = authPipe('1');
    expect(auth.transform({ current: 'c', password: 'admin' }, BODY)).toEqual({ current: 'c', password: 'admin' });
    expect(pointers(problemOf(() => auth.transform({ password: 'admin' }, BODY)))).toEqual(['/current']);
    expect(pointers(problemOf(() => auth.transform({ current: 'c', password: '' }, BODY)))).toEqual(['/password']);
    const users = usersPipe('1');
    expect(users.transform({ password: 'adm' }, BODY)).toEqual({ password: 'adm' });
    expect(problemOf(() => users.transform({ password: 'adm', bogus: 1 }, BODY)).getStatus()).toBe(400);
  });

  it('the documented schemas keep the product rule (the OpenAPI contract is unchanged)', () => {
    for (const s of [PasswordBody, SetPasswordBody]) {
      expect(openapi(s)).toMatchObject({ properties: { password: { minLength: PASSWORD_MIN, maxLength: 1024 } } });
    }
    expect(PASSWORD_MIN).toBe(12);
  });
});
