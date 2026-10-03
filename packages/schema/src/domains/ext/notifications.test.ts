import { describe, expect, it } from 'vitest';
import { NotificationChannelSchema, NotificationsSchema } from './notifications.js';

const email = (username: string) => ({
  name: 'relay',
  type: 'email',
  email: {
    smtpHost: 'mail.example.com',
    username,
    passwordRef: 'password/relay',
    from: 'sender@example.com',
    to: ['admin@example.com'],
  },
});

describe('notification SMTP contract', () => {
  it.each(['user\rname', 'user\nname', 'user\0name'])(
    'rejects header controls in username %j',
    (username) => expect(NotificationChannelSchema.safeParse(email(username)).success).toBe(false),
  );
  it('accepts an ordinary SMTP account with encrypted password reference', () => {
    expect(NotificationChannelSchema.parse(email('relay-user')).email?.tls).toBe('starttls');
  });
  it('defaults routing to default and rejects unsupported management VRFs', () => {
    expect(NotificationChannelSchema.parse(email('relay-user')).vrf).toBe('default');
    expect(
      NotificationChannelSchema.safeParse({ ...email('relay-user'), vrf: 'management' }).success,
    ).toBe(false);
  });
  it('rejects removed Telegram channels', () => {
    expect(NotificationChannelSchema.safeParse({ name: 'bot', type: 'telegram' }).success).toBe(
      false,
    );
  });
});


describe('notification authentication and rule references', () => {
  it('allows an explicitly unauthenticated relay', () => {
    const { username: _username, passwordRef: _passwordRef, ...relay } = email('relay-user').email;
    expect(NotificationChannelSchema.safeParse({ ...email('relay-user'), email: relay }).success).toBe(true);
  });
  it.each(['username', 'passwordRef'] as const)('rejects missing SMTP %s at the nested field', (missing) => {
    const relay: Record<string, unknown> = { ...email('relay-user').email };
    delete relay[missing];
    const result = NotificationsSchema.safeParse({ channels: [{ ...email('relay-user'), email: relay }] });
    expect(result.success).toBe(false);
    if (!result.success) expect(result.error.issues).toEqual(expect.arrayContaining([
      expect.objectContaining({ path: ['channels', 0, 'email', missing] }),
    ]));
  });
  const rules = (channels: string[]) => ({
    channels: [email('relay-user'), { ...email('relay-user'), name: 'backup' }],
    rules: [{ name: 'alarms', events: ['alarm'], channels }],
  });
  it('accepts distinct references to configured channels', () => {
    expect(NotificationsSchema.safeParse(rules(['relay', 'backup'])).success).toBe(true);
  });
  it('rejects a nonadjacent repeated channel at its duplicate index', () => {
    const result = NotificationsSchema.safeParse(rules(['relay', 'backup', 'relay']));
    expect(result.success).toBe(false);
    if (!result.success) expect(result.error.issues).toEqual(expect.arrayContaining([
      expect.objectContaining({ path: ['rules', 0, 'channels', 2], message: 'duplicate channel reference' }),
    ]));
  });
});
