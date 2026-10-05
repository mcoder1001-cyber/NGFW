import { describe, expect, it } from 'vitest';
import { decryptArchive, encryptArchive, hashDocument, type BackupArchive } from './archive.js';
const doc = { system: { hostname: 'restored' } };
const archive: BackupArchive = {
  manifest: {
    format: 1,
    schemaVersion: '1.0',
    revision: 1,
    hash: hashDocument(doc),
    createdAt: new Date().toISOString(),
    createdBy: 'admin',
  },
  running: doc,
  revisions: [],
  secrets: [{ ref: 'psk/test', value: 'NGFW_TEST_PSK_FBR_ROUNDTRIP', version: 1 }],
  audit: [],
};
describe('passphrase backup authentication', () => {
  it('round-trips without exposing plaintext and binds header and ciphertext', async () => {
    const bytes = await encryptArchive(archive, 'test-only-passphrase-123');
    expect(bytes.includes(Buffer.from('NGFW_TEST_PSK_FBR_ROUNDTRIP'))).toBe(false);
    expect(await decryptArchive(bytes, 'test-only-passphrase-123')).toEqual(archive);
    await expect(decryptArchive(bytes, 'wrong-passphrase-123')).rejects.toMatchObject({
      status: 400,
    });
    for (const index of [8, 24, 36, 60]) {
      const corrupt = Buffer.from(bytes);
      corrupt[index] = corrupt[index]! ^ 1;
      await expect(decryptArchive(corrupt, 'test-only-passphrase-123')).rejects.toMatchObject({
        status: 400,
      });
    }
  });
  it('rejects unknown major schema and short passphrases', async () => {
    await expect(
      encryptArchive(
        { ...archive, manifest: { ...archive.manifest, schemaVersion: '2.0' } },
        'test-only-passphrase-123',
      ),
    ).rejects.toThrow();
    await expect(encryptArchive(archive, 'short')).rejects.toMatchObject({ status: 400 });
  });
});
