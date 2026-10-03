import 'reflect-metadata';
import { describe, expect, it, vi } from 'vitest';
import { PkiService } from './pki.service.js';
import { certFacts, generateKey, parseDn, selfSignedCa } from './x509.js';

function expiryService() {
  const issuedAt = new Date('2030-01-01T00:00:00Z');
  const certificate = selfSignedCa(
    parseDn('CN=Expiry fixture'),
    generateKey({ type: 'ecdsa', curve: 'p256' }),
    1,
    issuedAt,
  );
  const expiresAt = Date.parse(certFacts(certificate.der).notAfter);
  const doc = {
    vpn: {
      pki: {
        cas: { root: { certificateRef: 'cert/fixture' } },
        certificates: { gateway: { certificateRef: 'cert/fixture', expiryAlertDays: 30 } },
      },
    },
  };
  const publish = vi.fn();
  const api = new PkiService(
    ...([
      {
        select: () => ({
          from: () => ({ where: async () => [{ ciphertext: 'public-certificate' }] }),
        }),
      },
      { decrypt: () => certificate.pem },
      { getRunning: async () => ({ doc }), getCandidate: async () => doc },
      { publish },
      { state: async () => ({ error: 'agent unavailable' }) },
    ] as unknown as ConstructorParameters<typeof PkiService>),
  );
  return { api, publish, expiresAt };
}

describe('PKI expiry alarm time boundary', () => {
  it('does not report still-valid CA/certificate as expired during their final partial day', async () => {
    const { api, publish, expiresAt } = expiryService();
    api.now = () => new Date(expiresAt - 60 * 60_000);
    const alerts = await api.checkExpiry();
    expect(alerts).toHaveLength(2);
    expect(alerts.map((alert) => alert.kind).sort()).toEqual(['ca', 'certificate']);
    expect(alerts.every((alert) => alert.daysLeft === 0 && alert.severity === 'warning')).toBe(
      true,
    );
    expect(publish).toHaveBeenCalledTimes(2);
    for (const [, event] of publish.mock.calls) {
      expect(event).toMatchObject({ type: 'raised', severity: 'warning', value: 0 });
      expect(event.message).toContain('expires in');
      expect(event.message).not.toContain('expired on');
    }
  });

  it('crosses the exact validity boundary for both kinds and keeps repeat checks quiet', async () => {
    const { api, publish, expiresAt } = expiryService();
    api.now = () => new Date(expiresAt);
    expect((await api.checkExpiry()).every((alert) => alert.severity === 'warning')).toBe(true);
    publish.mockClear();
    api.now = () => new Date(expiresAt + 1);
    const expired = await api.checkExpiry();
    expect(expired).toHaveLength(2);
    expect(expired.every((alert) => alert.severity === 'critical')).toBe(true);
    expect(publish).toHaveBeenCalledTimes(2);
    for (const [, event] of publish.mock.calls) {
      expect(event).toMatchObject({ type: 'raised', severity: 'critical' });
      expect(event.message).toContain('expired on');
    }
    publish.mockClear();
    await api.checkExpiry();
    expect(publish).not.toHaveBeenCalled();
  });
});
