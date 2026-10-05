import { z } from 'zod';
import { objectName, secretRefOf } from '../../primitives.js';
import { withUi } from '../../ui.js';

const unsafe = (value: string): boolean =>
  [...value].some((c) => c.charCodeAt(0) < 32 || c.charCodeAt(0) === 127);
const text = z
  .string()
  .min(1)
  .max(2048)
  .refine((value) => !unsafe(value), 'control characters are forbidden');
const cron = z
  .string()
  .regex(
    /^(?:\*|\*\/[1-9][0-9]*|[0-9]+)(?: (?:\*|\*\/[1-9][0-9]*|[0-9]+)){4}$/,
    'five UTC cron fields: *, */n or number',
  );
export const BackupTargetSchema = z
  .strictObject({
    type: z.enum(['local', 'sftp', 'https']),
    path: text,
    host: text.optional(),
    port: z.number().int().min(1).max(65535).optional(),
    username: objectName.optional(),
    credentialRef: secretRefOf(['password', 'key', 'token']).optional(),
    hostKeySha256: z
      .string()
      .regex(/^[a-f0-9]{64}$/)
      .optional(),
  })
  .superRefine((v, ctx) => {
    if (v.type === 'local' && (!v.path.startsWith('/') || v.path.split('/').includes('..')))
      ctx.addIssue({
        code: 'custom',
        path: ['path'],
        message: 'absolute directory without traversal required',
      });
    if (v.type === 'https') {
      try {
        const u = new URL(v.path);
        if (u.protocol !== 'https:' || u.username || u.password || u.hash) throw new Error();
      } catch {
        ctx.addIssue({
          code: 'custom',
          path: ['path'],
          message: 'HTTPS URL without credentials required',
        });
      }
    }
    if (v.type === 'sftp' && (!v.host || !v.username || !v.credentialRef || !v.hostKeySha256))
      ctx.addIssue({
        code: 'custom',
        message: 'SFTP host, username, credential reference and pinned host key required',
      });
  });
export const BackupSchema = z
  .strictObject({
    enabled: z.boolean().default(false),
    schedule: cron
      .superRefine((value, ctx) => {
        const fields = value.split(' '),
          limits = [
            [0, 59],
            [0, 23],
            [1, 31],
            [1, 12],
            [0, 6],
          ];
        fields.forEach((field, index) => {
          const bound = limits[index]!;
          if (
            (/^\d+$/.test(field) && (Number(field) < bound[0]! || Number(field) > bound[1]!)) ||
            (field.startsWith('*/') && Number(field.slice(2)) > bound[1]! + 1)
          )
            ctx.addIssue({ code: 'custom', message: 'cron field is outside its valid range' });
        });
      })
      .default('0 2 * * *'),
    target: BackupTargetSchema.optional(),
    retention: z.number().int().min(1).max(365).default(7),
    revisions: z.number().int().min(1).max(1000).default(100),
    passphraseRef: secretRefOf('password').optional(),
  })
  .superRefine((v, ctx) => {
    if (v.enabled && (!v.target || !v.passphraseRef))
      ctx.addIssue({
        code: 'custom',
        message: 'enabled backups require target and passphrase reference',
      });
  });
export const ConfigTemplateSchema = z.strictObject({
  description: z.string().max(1024).default(''),
  parameters: z
    .record(
      objectName,
      z.strictObject({
        type: z.enum(['string', 'number', 'boolean']),
        required: z.boolean().default(true),
      }),
    )
    .default({}),
  patchJson: z
    .string()
    .max(1024 * 1024)
    .superRefine((value, ctx) => {
      try {
        const walk = (node: unknown): void => {
          if (typeof node === 'string' && unsafe(node)) throw new Error();
          if (node && typeof node === 'object')
            for (const [key, child] of Object.entries(node)) {
              if (
                [
                  '__proto__',
                  'constructor',
                  'prototype',
                  'passwordHash',
                  'password',
                  'privateKey',
                  'psk',
                  'secret',
                  'token',
                ].includes(key) ||
                unsafe(key)
              )
                throw new Error();
              walk(child);
            }
        };
        const patch: unknown = JSON.parse(value);
        if (!patch || typeof patch !== 'object' || Array.isArray(patch)) throw new Error();
        walk(patch);
      } catch {
        ctx.addIssue({
          code: 'custom',
          message: 'safe JSON object without inline credentials required',
        });
      }
    }),
});
export const managementBackupField = withUi(BackupSchema.prefault({}), {
  title: 'Scheduled backup',
});
export const managementTemplatesField = z.record(objectName, ConfigTemplateSchema).default({});
