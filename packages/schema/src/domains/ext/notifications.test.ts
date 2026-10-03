import { describe, expect, it } from 'vitest';
import { NotificationChannelSchema } from './notifications.js';

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
