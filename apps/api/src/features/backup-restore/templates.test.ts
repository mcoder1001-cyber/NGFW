import { describe, expect, it } from 'vitest';
import { renderTemplate } from './templates.js';
import { assertSupportSafe } from './backup-restore.service.js';
import { cronMatches } from './schedule.js';
describe('operational configuration safety', () => {
  it('preserves parameter types and refuses unknown, missing and control-character parameters', () => {
    const template = {
      parameters: {
        hostname: { type: 'string' as const, required: true },
        enabled: { type: 'boolean' as const, required: true },
      },
      patch: {
        system: { hostname: '${hostname}' },
        management: { backup: { enabled: '${enabled}' } },
      },
    };
    expect(renderTemplate(template, { hostname: 'restored', enabled: true })).toEqual({
      system: { hostname: 'restored' },
      management: { backup: { enabled: true } },
    });
    expect(() => renderTemplate(template, { hostname: 'bad\nname', enabled: true })).toThrow();
    expect(() =>
      renderTemplate(template, { hostname: 'restored', enabled: true, other: 1 }),
    ).toThrow();
    expect(() => renderTemplate(template, { enabled: true })).toThrow();
    expect(() =>
      renderTemplate({ parameters: {}, patch: JSON.parse('{"__proto__":{"polluted":true}}') }, {}),
    ).toThrow();
    expect({}).not.toHaveProperty('polluted');
  });
  it('substitutes self-referential and cyclic-looking parameter values as literals', () => {
    const template = {
      parameters: {
        first: { type: 'string' as const, required: true },
        second: { type: 'string' as const, required: true },
      },
      patch: { system: { hostname: '${first}', domain: '${second}' } },
    };
    expect(renderTemplate(template, { first: '${first}', second: '${second}' })).toEqual({
      system: { hostname: '${first}', domain: '${second}' },
    });
    expect(renderTemplate(template, { first: '${second}', second: '${first}' })).toEqual({
      system: { hostname: '${second}', domain: '${first}' },
    });
    expect(() => renderTemplate(template, { first: 'bad\nname', second: '${first}' })).toThrow();
  });
  it('fails the final support scan on quoted or serialized credential keys and hashes', () => {
    for (const raw of [
      '{"psk":"raw"}',
      '{"password":"raw"}',
      '{"token":"raw"}',
      '{"passwordHash":"$argon2id$v=19"}',
      '{"privateKey":"-----BEGIN PRIVATE KEY"}',
      JSON.stringify({ host: ['{"secret":"raw"}'] }),
    ])
      expect(() => assertSupportSafe(raw)).toThrow();
    expect(() =>
      assertSupportSafe(
        '{"config":{"passwordRef":"password/backup"},"services":{"active":"running"}}',
      ),
    ).not.toThrow();
  });
  it('matches UTC cron day/month fields and step minutes', () => {
    const date = new Date('2026-10-05T02:00:00Z');
    expect(cronMatches('0 2 * * *', date)).toBe(true);
    expect(cronMatches('*/15 * * * *', date)).toBe(true);
    expect(cronMatches('0 2 5 10 1', date)).toBe(true);
    expect(cronMatches('1 2 * * *', date)).toBe(false);
    expect(cronMatches('0 2 5 11 1', date)).toBe(false);
  });
});
