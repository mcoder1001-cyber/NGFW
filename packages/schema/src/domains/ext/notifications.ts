import { z } from 'zod';
import { hostOrIp, objectName, portNumber, secretRefOf } from '../../primitives.js';
import { withUi } from '../../ui.js';
import { DEFAULT_VRF } from '../vrfs.js';
const line = z
  .string()
  .min(1)
  .max(254)
  .regex(/^[^\r\n]+$/)
  .refine((value) => !value.includes('\0'), 'NUL is not allowed');
export const NotificationChannelSchema = z
  .strictObject({
    name: objectName,
    enabled: z.boolean().default(true),
    vrf: withUi(z.literal(DEFAULT_VRF).default(DEFAULT_VRF), {
      title: 'VRF',
      help: 'Only default API namespace routing is supported; other VRFs fail validation.',
    }),
    type: z.enum(['email', 'webhook']),
    email: z
      .strictObject({
        smtpHost: hostOrIp,
        port: portNumber.default(587),
        tls: z.enum(['starttls', 'tls']).default('starttls'),
        username: line.optional(),
        passwordRef: secretRefOf('password').optional(),
        from: z.email().max(254),
        to: z.array(z.email().max(254)).min(1).max(32),
      })
      .refine(
        (email) => (email.username === undefined) === (email.passwordRef === undefined),
        'SMTP username and password reference must be configured together',
      )
      .optional(),
    webhook: z
      .strictObject({
        url: z
          .url()
          .max(2048)
          .refine((s) => {
            const u = new URL(s);
            return u.protocol === 'https:' && !u.username && !u.password && !u.hash;
          }, 'HTTPS URL without credentials or fragment required'),
        secretRef: secretRefOf('token'),
      })
      .optional(),
  })
  .refine(
    (c) =>
      c.type === 'email'
        ? c.email !== undefined && c.webhook === undefined
        : c.webhook !== undefined && c.email === undefined,
    'exactly the selected channel configuration is required',
  );
export const NotificationRuleSchema = z.strictObject({
  name: objectName,
  enabled: z.boolean().default(true),
  events: z
    .array(z.enum(['alarm', 'commit', 'link', 'vpn', 'global-blocking']))
    .min(1)
    .max(5),
  minSeverity: z.enum(['info', 'warning', 'critical']).default('warning'),
  channels: z.array(objectName).min(1).max(32),
  throttleSec: z.number().int().min(1).max(86400).default(60),
});
export const NotificationsSchema = z
  .strictObject({
    channels: z.array(NotificationChannelSchema).max(32).default([]),
    rules: z.array(NotificationRuleSchema).max(64).default([]),
  })
  .superRefine((n, ctx) => {
    for (const key of ['channels', 'rules'] as const) {
      const names = new Set<string>();
      n[key].forEach((v, i) => {
        if (names.has(v.name))
          ctx.addIssue({ code: 'custom', path: [key, i, 'name'], message: 'duplicate name' });
        names.add(v.name);
      });
    }
    const names = new Set(n.channels.map((c) => c.name));
    n.rules.forEach((v, i) =>
      v.channels.forEach((c, j) => {
        if (!names.has(c))
          ctx.addIssue({
            code: 'custom',
            path: ['rules', i, 'channels', j],
            message: 'unknown channel',
          });
      }),
    );
  });
export type NotificationsConfig = z.infer<typeof NotificationsSchema>;
export type NotificationChannel = z.infer<typeof NotificationChannelSchema>;
export const managementNotificationsField = withUi(NotificationsSchema.optional(), {
  title: 'Notifications',
  group: 'observability',
});
