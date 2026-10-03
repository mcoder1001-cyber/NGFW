import { Inject, Injectable } from '@nestjs/common';
import { DesiredState } from '@ngfw/proto';
import { parsePointer } from '@ngfw/schema';
import { and, eq } from 'drizzle-orm';
import { createDecipheriv } from 'node:crypto';
import { readKeyFileBytes } from '../auth/key-file.js';
import { problems } from '../common/problem.js';
import { ENV, type Env } from '../config.js';
import { secretRefs } from '../datastore/documents.js';
import { DB, type Db } from '../db/db.js';
import { secret, secretVersion } from '../db/schema.js';

export interface DeliveredSecrets {
  bundle: { values: Record<string, Buffer> };
  versions: Record<string, number>;
}

/** Socket-only native IPsec PSK delivery. No dependency on the commit engine or REST secret service. */
@Injectable()
export class SecretDeliveryService {
  constructor(
    @Inject(DB) private readonly db: Db,
    @Inject(ENV) private readonly env: Env,
  ) {}

  async resolve(state: DesiredState, versions?: Readonly<Record<string, number>>) {
    return (await this.resolveVersioned(state, versions)).bundle;
  }

  async resolveVersioned(
    state: DesiredState,
    versions?: Readonly<Record<string, number>>,
  ): Promise<DeliveredSecrets> {
    const refs = secretRefs(DesiredState.toJSON(state)).filter(({ pointer }) => {
      if (!/^\/vpn\/ipsec\/tunnels\/[^/]+\/auth\/secretRef$/.test(pointer)) return false;
      const name = parsePointer(pointer)[3];
      const tunnel = name === undefined ? undefined : state.vpn?.ipsec?.tunnels[name];
      return (
        tunnel?.engine === 'vpp-ikev2' && tunnel.auth?.method === 'psk' && tunnel.enabled !== false
      );
    });
    const result: DeliveredSecrets = { bundle: { values: {} }, versions: {} };
    // No DB/key-file access for an empty selection; a present empty bundle clears native bindings.
    for (const ref of new Set(refs.map((r) => r.ref))) {
      if (!ref.startsWith('psk/'))
        throw problems.validation(
          [
            {
              pointer: refs.find((r) => r.ref === ref)!.pointer,
              rule: 'secrets.kind',
              message: 'native IPsec authentication requires a PSK reference',
            },
          ],
          'native IPsec secret kind is invalid',
        );
      const [current] = await this.db.select().from(secret).where(eq(secret.ref, ref));
      if (!current || current.kind !== 'psk')
        throw problems.unavailable('native IPsec secret is unavailable');
      const pinned = versions?.[ref];
      if (pinned !== undefined && (!Number.isInteger(pinned) || pinned < 1))
        throw problems.unavailable('native IPsec secret version is unavailable');
      let ciphertext = current.ciphertext;
      let version = current.version;
      if (pinned !== undefined) {
        const [old] = await this.db
          .select()
          .from(secretVersion)
          .where(and(eq(secretVersion.ref, ref), eq(secretVersion.version, pinned)));
        if (!old) throw problems.unavailable('native IPsec secret version is unavailable');
        ciphertext = old.ciphertext;
        version = pinned;
      }
      try {
        const key = readKeyFileBytes(this.env.NGFW_SECRET_KEY_FILE).bytes;
        if (key.length !== 32) throw new Error('invalid key');
        const raw = Buffer.from(ciphertext, 'base64');
        const decipher = createDecipheriv('aes-256-gcm', key, raw.subarray(0, 12));
        decipher.setAAD(Buffer.from(ref, 'utf8'));
        decipher.setAuthTag(raw.subarray(12, 28));
        result.bundle.values[ref] = Buffer.concat([
          decipher.update(raw.subarray(28)),
          decipher.final(),
        ]);
      } catch {
        throw problems.unavailable('native IPsec secret cannot be decrypted');
      }
      result.versions[ref] = version;
    }
    return result;
  }
}
