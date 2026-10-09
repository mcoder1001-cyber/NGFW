import { Inject, Injectable } from '@nestjs/common';
import { DesiredState } from '@ngfw/proto';
import { parsePointer } from '@ngfw/schema';
import { and, eq } from 'drizzle-orm';
import { createDecipheriv, createPrivateKey, X509Certificate } from 'node:crypto';
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

/** Socket-only operational credentials; CA signing keys remain API-side. */
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
      const parts = parsePointer(pointer);
      if (parts[0] === 'routing') {
        if (/^\/routing\/bgp\/(peerGroups|neighbors)\/[^/]+\/passwordRef$/.test(pointer))
          return true;
        // wave-BC: F-bfd-redistribution: auth refs use the existing sealed channel.
        if (/^\/routing\/bfd\/sessions\/[0-9]+\/auth\/keyRef$/.test(pointer)) return true;
        if (/^\/routing\/isis\/(areaPasswordRef|domainPasswordRef)$/.test(pointer)) return true;
        if (
          parts.length !== 6 ||
          parts[2] !== 'interfaces' ||
          parts[4] !== 'auth' ||
          parts[5] !== 'keyRef'
        )
          return false;
        const protocol = parts[1];
        if (protocol !== 'ospf' && protocol !== 'rip') return false;
        return state.routing?.[protocol]?.interfaces[parts[3]!]?.auth?.type === 'md5';
      }
      if (
        parts[0] === 'interfaces' &&
        parts.length === 4 &&
        parts[2] === 'pppoe' &&
        parts[3] === 'passwordRef'
      ) {
        return state.interfaces[parts[1]!]?.pppoe?.enabled !== false;
      }
      if (/^\/services\/hostStack\/namespaces\/[^/]+\/secretRef$/.test(pointer)) return true;
      if (/^\/services\/ntp\/servers\/[0-9]+\/keyRef$/.test(pointer))
        return state.services?.ntp?.enabled === true;
      if (/^\/management\/syslog\/[0-9]+\/tls\/(caRef|certRef|keyRef)$/.test(pointer))
        return state.management?.syslog[Number(parts[2])]?.protocol === 'tls';
      if (parts[0] === 'services' && parts[1] === 'snmp') {
        if (state.services?.snmp?.enabled !== true) return false;
        return (
          /^\/services\/snmp\/communities\/[^/]+\/secretRef$/.test(pointer) ||
          /^\/services\/snmp\/v3Users\/[^/]+\/(authRef|privRef)$/.test(pointer)
        );
      }
      if (parts[0] !== 'vpn') return false;
      if (parts[1] === 'wireguard') {
        return (
          /^\/vpn\/wireguard\/interfaces\/[^/]+\/privateKeyRef$/.test(pointer) ||
          /^\/vpn\/wireguard\/interfaces\/[^/]+\/peers\/[^/]+\/presharedKeyRef$/.test(pointer)
        );
      }
      if (parts[1] === 'remoteAccess') {
        const profile = state.vpn?.remoteAccess[parts[2]!];
        if (!profile || profile.enabled === false) return false;
        if (profile.auth === 'eap-mschapv2')
          return (
            parts.length === 6 &&
            parts[3] === 'users' &&
            /^(0|[1-9][0-9]*)$/.test(parts[4]!) &&
            parts[5] === 'passwordRef'
          );
        if (profile.auth === 'eap-radius')
          return (
            parts.length === 7 &&
            parts[3] === 'radius' &&
            parts[4] === 'servers' &&
            /^(0|[1-9][0-9]*)$/.test(parts[5]!) &&
            parts[6] === 'secretRef'
          );
        return false;
      }
      if (parts[1] === 'pki') {
        if (parts.length !== 5) return false;
        if (parts[2] === 'cas') return parts[4] === 'certificateRef';
        const certificate = state.vpn?.pki?.certificates[parts[3]!];
        return (
          parts[2] === 'certificates' &&
          !!certificate?.certificateRef &&
          (parts[4] === 'certificateRef' || parts[4] === 'privateKeyRef')
        );
      }
      if (!/^\/vpn\/ipsec\/tunnels\/[^/]+\/auth\/secretRef$/.test(pointer)) return false;
      const tunnel = state.vpn?.ipsec?.tunnels[parts[3]!];
      return (
        tunnel?.engine === 'vpp-ikev2' && tunnel.auth?.method === 'psk' && tunnel.enabled !== false
      );
    });
    const caKeyRefs = new Set(
      Object.entries(state.vpn?.pki?.cas ?? {}).flatMap(([name, ca]) => [
        `key/${name}`,
        `key/${ca.certificateRef?.slice('cert/'.length) ?? name}`,
      ]),
    );
    const explicit = new Set(refs.map(({ ref }) => ref));
    const optional = new Set<string>();
    for (const name of Object.keys(state.vpn?.pki?.cas ?? {})) {
      if (name.length <= 59) {
        const ref = `cert/${name}.crl`;
        if (!explicit.has(ref)) optional.add(ref);
        refs.push({ ref, pointer: `/vpn/pki/cas/${name}/crl` });
      }
    }
    const result: DeliveredSecrets = { bundle: { values: {} }, versions: {} };
    // Match the existing agent sealed-channel bounds across every active feature.
    const required = [...new Set(refs.filter((r) => !optional.has(r.ref)).map((r) => r.ref))];
    if (required.length > 1024)
      throw problems.validation([
        {
          pointer: refs.find((r) => r.ref === required[1024])!.pointer,
          rule: 'secrets.transport-reference-limit',
          message: 'active features exceed the sealed channel limit of 1024 distinct references',
        },
      ]);
    let snapshotBytes = 2; // Go json.Marshal(map[string][]byte): braces and base64 values.
    let deliveredCount = 0;
    // No DB/key-file access for an empty selection; a present empty bundle clears native bindings.
    try {
      for (const ref of new Set(
        refs
          .map((r) => r.ref)
          .sort(
            (a, b) =>
              (optional.has(a) ? 2 : Number(a.startsWith('key/'))) -
              (optional.has(b) ? 2 : Number(b.startsWith('key/'))),
          ),
      )) {
        if (versions !== undefined && optional.has(ref) && versions[ref] === undefined) continue;
        const selections = refs.filter((r) => r.ref === ref);
        const kinds = new Set(
          selections.map(({ pointer }) =>
            pointer.startsWith('/services/hostStack/') ||
            pointer.startsWith('/services/ntp/') ||
            /^\/management\/syslog\/[0-9]+\/tls\/keyRef$/.test(pointer) ||
            pointer.startsWith('/routing/bfd/sessions/') ||
            /^\/vpn\/wireguard\/interfaces\/[^/]+\/privateKeyRef$/.test(pointer)
              ? 'key'
              : pointer.startsWith('/routing/') ||
                  pointer.startsWith('/interfaces/') ||
                  pointer.startsWith('/services/snmp/') ||
                  /^\/vpn\/remoteAccess\/[^/]+\/users\/[0-9]+\/passwordRef$/.test(pointer)
                ? 'password'
                : pointer.startsWith('/management/syslog/')
                  ? 'cert'
                  : pointer.startsWith('/vpn/pki/')
                    ? pointer.endsWith('/privateKeyRef')
                      ? 'key'
                      : 'cert'
                    : 'psk',
          ),
        );
        const kind = [...kinds][0]!;
        if (kinds.size !== 1 || !ref.startsWith(`${kind}/`) || caKeyRefs.has(ref))
          throw problems.validation(
            [
              {
                pointer: refs.find((r) => r.ref === ref)!.pointer,
                rule: 'secrets.kind',
                message: `this operational reference requires a ${kind} secret`,
              },
            ],
            'operational secret kind is invalid',
          );
        const bfdOnly = selections.every(({ pointer }) =>
          pointer.startsWith('/routing/bfd/sessions/'),
        );
        // Check every syslog use independently of other consumers sharing the key.
        // Certificates were resolved first; reject CA/missing leaves before key lookup.
        const syslogLeaves: X509Certificate[] = [];
        for (const { pointer } of selections) {
          if (!/^\/management\/syslog\/[0-9]+\/tls\/keyRef$/.test(pointer)) continue;
          const tls = state.management?.syslog[Number(parsePointer(pointer)[2])]?.tls;
          try {
            if (!tls?.certRef) throw new Error('missing certificate');
            const certificate = new X509Certificate(result.bundle.values[tls.certRef]!);
            if (certificate.ca) throw new Error('CA signing key');
            syslogLeaves.push(certificate);
          } catch {
            throw problems.unavailable('TLS syslog requires an operational end-entity certificate');
          }
        }
        if (
          kind === 'key' &&
          !bfdOnly &&
          !selections.every(
            ({ pointer }) =>
              pointer.startsWith('/vpn/wireguard/') ||
              pointer.startsWith('/services/ntp/') ||
              pointer.startsWith('/services/hostStack/') ||
              pointer.startsWith('/management/syslog/'),
          )
        ) {
          const certificates = Object.values(state.vpn?.pki?.certificates ?? {}).filter(
            (c) => c.privateKeyRef === ref,
          );
          try {
            if (
              certificates.length === 0 ||
              certificates.some(
                (c) =>
                  !c.certificateRef ||
                  new X509Certificate(result.bundle.values[c.certificateRef]!).ca,
              )
            )
              throw new Error('not operational');
          } catch {
            throw problems.unavailable(
              'CA signing keys cannot be delivered as operational PKI keys',
            );
          }
        }
        const [current] = await this.db.select().from(secret).where(eq(secret.ref, ref));
        if (!current && optional.has(ref) && versions?.[ref] === undefined) continue;
        if (!current || current.kind !== kind)
          throw problems.unavailable('operational secret is unavailable');
        const pinned = versions?.[ref];
        if (pinned !== undefined && (!Number.isInteger(pinned) || pinned < 1))
          throw problems.unavailable('operational secret version is unavailable');
        let ciphertext = current.ciphertext;
        let version = current.version;
        if (pinned !== undefined) {
          const [old] = await this.db
            .select()
            .from(secretVersion)
            .where(and(eq(secretVersion.ref, ref), eq(secretVersion.version, pinned)));
          if (!old) throw problems.unavailable('operational secret version is unavailable');
          ciphertext = old.ciphertext;
          version = pinned;
        }
        let key: Buffer | undefined;
        let plaintext: Buffer | undefined;
        let tail: Buffer | undefined;
        try {
          key = readKeyFileBytes(this.env.NGFW_SECRET_KEY_FILE).bytes;
          if (key.length !== 32) throw new Error('invalid key');
          const raw = Buffer.from(ciphertext, 'base64');
          const decipher = createDecipheriv('aes-256-gcm', key, raw.subarray(0, 12));
          decipher.setAAD(Buffer.from(ref, 'utf8'));
          decipher.setAuthTag(raw.subarray(12, 28));
          plaintext = decipher.update(raw.subarray(28));
          tail = decipher.final();
          result.bundle.values[ref] = Buffer.concat([plaintext, tail]);
        } catch {
          throw problems.unavailable('operational secret cannot be decrypted');
        } finally {
          plaintext?.fill(0);
          tail?.fill(0);
          key?.fill(0);
        }
        if (result.bundle.values[ref]!.length > 64 * 1024) {
          result.bundle.values[ref]!.fill(0);
          throw problems.unavailable('operational secret exceeds the agent transport limit');
        }
        if (syslogLeaves.length > 0) {
          try {
            const privateKey = createPrivateKey(result.bundle.values[ref]!);
            if (syslogLeaves.some((certificate) => !certificate.checkPrivateKey(privateKey)))
              throw new Error('key mismatch');
          } catch {
            throw problems.unavailable('TLS syslog certificate and private key do not match');
          }
        }
        const count = ++deliveredCount;
        snapshotBytes +=
          Buffer.byteLength(JSON.stringify(ref)) +
          3 + // colon and the base64 string's quotes
          4 * Math.ceil(result.bundle.values[ref]!.length / 3) +
          (count > 1 ? 1 : 0);
        if (count > 1024 || snapshotBytes > 4 * 1024 * 1024) {
          for (const value of Object.values(result.bundle.values)) value.fill(0);
          throw problems.validation([
            {
              pointer: selections[0]!.pointer,
              rule: 'secrets.transport-bundle-limit',
              message:
                'active credentials exceed the sealed channel reference or bundle size limit',
            },
          ]);
        }
        if (
          bfdOnly &&
          (result.bundle.values[ref]!.length < 1 || result.bundle.values[ref]!.length > 20)
        ) {
          result.bundle.values[ref]!.fill(0);
          throw problems.validation([
            {
              pointer: selections[0]!.pointer,
              rule: 'routing.bfd-key-length',
              message: 'BFD SHA1 keys must contain 1–20 bytes',
            },
          ]);
        }
        result.versions[ref] = version;
      }
      return result;
    } catch (error) {
      for (const value of Object.values(result.bundle.values)) value.fill(0);
      throw error;
    }
  }
}
