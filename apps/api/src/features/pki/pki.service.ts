import type { PkiIssued } from '@ngfw/schema';
import {
  Inject,
  Injectable,
  Logger,
  type OnApplicationBootstrap,
  type OnModuleDestroy,
} from '@nestjs/common';
import { X509Certificate, type KeyObject } from 'node:crypto';
import { eq, inArray } from 'drizzle-orm';
import type { ProblemIssue } from '../../common/problem.js';
import { problems } from '../../common/problem.js';
import type { Principal } from '../../common/principal.js';
import { DatastoreService } from '../../datastore/datastore.service.js';
import { DB, type Db } from '../../db/db.js';
import { secret } from '../../db/schema.js';
import { Bus } from '../../infra/bus.js';
import { SecretsService } from '../../secrets/secrets.service.js';
import { PkiAgentFiles } from './agent-files.js';
import { parsePkcs12 } from './pkcs12.js';
import {
  httpFetch,
  MAX_CRL,
  ocspCheck,
  parseCrl,
  type Fetcher,
  type OcspResult,
} from './revocation.js';
import {
  buildCsr,
  certFacts,
  daysLeft,
  generateKey,
  isPrivateKeyLabel,
  keySpecOf,
  normaliseKeySpec,
  parseCsr,
  parseDn,
  pemBlocks,
  pemCert,
  PkiError,
  privateKeyPem,
  readPrivateKey,
  sanValues,
  selfSignedCa,
  signCsr,
  toPem,
  type CertFacts,
  type KeySpec,
} from './x509.js';

type Json = Record<string, unknown>;

/** What the config document holds about one CA / certificate (the fields F-pki reads). */
interface CaCfg {
  certificateRef?: string;
  crl?: { url?: string; refreshIntervalSec?: number };
  ocspUrl?: string;
  issued?: Json;
  keySpec?: Json;
}
interface CertCfg {
  certificateRef?: string;
  privateKeyRef?: string;
  ca?: string;
  acme?: Json;
  expiryAlertDays?: number;
  issued?: Json;
}
interface PkiCfg {
  cas: Record<string, CaCfg>;
  certificates: Record<string, CertCfg>;
  hsm?: { enabled?: boolean };
}

export function pkiOf(doc: unknown): PkiCfg {
  const p = ((doc as Json | undefined)?.['vpn'] as Json | undefined)?.['pki'] as Json | undefined;
  return {
    cas: (p?.['cas'] ?? {}) as Record<string, CaCfg>,
    certificates: (p?.['certificates'] ?? {}) as Record<string, CertCfg>,
    ...(p?.['hsm'] !== undefined ? { hsm: p['hsm'] as { enabled?: boolean } } : {}),
  };
}

/** The `issued` facts recorded next to a reference (PkiIssued in packages/schema). */
export function issuedOf(f: CertFacts): PkiIssued & { ca: boolean } {
  return {
    subject: f.subject,
    issuer: f.issuer,
    serial: f.serial,
    notBefore: f.notBefore,
    notAfter: f.notAfter,
    fingerprint: f.fingerprint,
    ca: f.ca,
  };
}

/** The reference the API stores a CA's or certificate's material under, and the CRL convention the agent reads. */
export const refs = {
  cert: (name: string) => `cert/${name}`,
  key: (name: string) => `key/${name}`,
  crl: (ca: string) => `cert/${ca}.crl`,
};

const DAY = 86_400_000;
/** Expiry check period and the first check after start-up; CRL due-check period. */
const EXPIRY_EVERY_MS = DAY;
const FIRST_CHECK_MS = 60_000;
const CRL_EVERY_MS = 3_600_000;

export interface CrlState {
  url: string;
  fetchedAt: string | null;
  thisUpdate: string | null;
  nextUpdate: string | null;
  revoked: string[];
  number: string | null;
  error: string | null;
}

export interface OcspState {
  url: string;
  checkedAt: string;
  status: OcspResult['status'] | 'error';
  revokedAt: string | null;
  error: string | null;
}

export interface ExpiryAlert {
  kind: 'ca' | 'certificate';
  name: string;
  notAfter: string;
  daysLeft: number;
  alertDays: number;
  severity: 'warning' | 'critical';
}

/** A merge patch the action applied to the candidate (or would have: `staged: false` + reason). */
export interface Staging {
  staged: boolean;
  pointer: string;
  patch: Json;
  reason?: string;
}

/**
 * F-pki: the certificate store end to end on the API side — generate a CA, key + CSR, sign, import PEM/PKCS#12, export
 * public parts, refresh CRLs, check OCSP, report state, alert before expiry. Material lives in the secret store
 * (`cert/<name>`, `key/<name>`, AES-GCM at rest, D-051); the document holds references plus the public `issued` facts.
 * A CA's signing key is `key/<ca name>` and never leaves this process (signing happens here; the agent materialises only
 * keys of certificates a tunnel uses). Nothing here logs, returns or audits key material or passphrases.
 */
@Injectable()
export class PkiService implements OnApplicationBootstrap, OnModuleDestroy {
  private readonly log = new Logger('PkiService');
  private readonly crls = new Map<string, CrlState>();
  private readonly ocsp = new Map<string, OcspState>();
  private readonly alerts = new Map<string, ExpiryAlert>();
  private lastExpiryCheck: string | null = null;
  private timers: NodeJS.Timeout[] = [];
  /** HTTP for CRL/OCSP (tests replace it). */
  fetcher: Fetcher = httpFetch;
  /** Clock (tests move it). */
  now: () => Date = () => new Date();

  constructor(
    @Inject(DB) private readonly db: Db,
    @Inject(SecretsService) private readonly secrets: SecretsService,
    @Inject(DatastoreService) private readonly ds: DatastoreService,
    @Inject(Bus) private readonly bus: Bus,
    @Inject(PkiAgentFiles) private readonly files: PkiAgentFiles,
  ) {}

  // ---- lifecycle: the daily expiry check and the hourly CRL due check (timers only; no new scheduler) ----

  onApplicationBootstrap(): void {
    const first = setTimeout(() => void this.periodic(true), FIRST_CHECK_MS);
    const expiry = setInterval(() => void this.periodic(true), EXPIRY_EVERY_MS);
    const crl = setInterval(() => void this.periodic(false), CRL_EVERY_MS);
    for (const t of [first, expiry, crl]) t.unref();
    this.timers.push(first, expiry, crl);
  }

  onModuleDestroy(): void {
    for (const t of this.timers) clearTimeout(t);
    this.timers = [];
  }

  private async periodic(expiry: boolean): Promise<void> {
    try {
      await this.refreshDueCrls();
      if (expiry) await this.checkExpiry();
    } catch (e) {
      this.log.warn(`PKI periodic check failed: ${e instanceof Error ? e.message : String(e)}`);
    }
  }

  // ---- secret store ----

  private async readSecret(ref: string): Promise<string | null> {
    const [row] = await this.db
      .select({ ciphertext: secret.ciphertext })
      .from(secret)
      .where(eq(secret.ref, ref));
    if (row === undefined) return null;
    try {
      return this.secrets.decrypt(row.ciphertext, ref);
    } catch {
      throw problems.unavailable(`the secret store cannot decrypt ${ref}`);
    }
  }

  private async existing(refsToCheck: string[]): Promise<Set<string>> {
    if (refsToCheck.length === 0) return new Set();
    const rows = await this.db
      .select({ ref: secret.ref })
      .from(secret)
      .where(inArray(secret.ref, refsToCheck));
    return new Set(rows.map((r) => r.ref));
  }

  /** 409 before anything is written when a reference exists and `replace` was not asked for. */
  private async refuseExisting(refsToWrite: string[], replace: boolean): Promise<void> {
    if (replace) return;
    const taken = [...(await this.existing(refsToWrite))];
    if (taken.length > 0)
      throw problems.conflict(
        'secret-exists',
        `${taken.join(', ')} already exist${taken.length === 1 ? 's' : ''}; send replace: true to store new versions (the old ones stay available for rollback)`,
      );
  }

  private async store(
    ref: string,
    value: string,
    user: Principal,
    replace: boolean,
  ): Promise<number> {
    const [kind, ...rest] = ref.split('/');
    const r = await this.secrets.put(kind as 'cert' | 'key', rest.join('/'), value, {
      replace,
      userId: user.id,
    });
    return r.version;
  }

  // ---- config ----

  private async docs(): Promise<{ running: PkiCfg; candidate: PkiCfg }> {
    const [running, candidate] = await Promise.all([this.ds.getRunning(), this.ds.getCandidate()]);
    return { running: pkiOf(running.doc), candidate: pkiOf(candidate) };
  }

  private async stage(
    user: Principal,
    pointer: string,
    patch: Json,
    stage: boolean,
  ): Promise<Staging> {
    if (!stage) return { staged: false, pointer, patch, reason: 'stage: false' };
    try {
      await this.ds.patchCandidate(user, pointer, patch);
      return { staged: true, pointer, patch };
    } catch (e) {
      const status = (e as { getStatus?: () => number }).getStatus?.();
      const reason = e instanceof Error ? e.message : 'the candidate refused the change';
      if (status === 409 || status === 400 || status === 403)
        return { staged: false, pointer, patch, reason };
      throw e;
    }
  }

  /** A CA of the candidate (staged, preferred) or of running; 400 with `pointer` when there is none. */
  private async caByName(
    name: string,
    pointer: string,
  ): Promise<{ cfg: CaCfg; facts: CertFacts & { x509: X509Certificate } }> {
    const { running, candidate } = await this.docs();
    const cfg = candidate.cas[name] ?? running.cas[name];
    if (cfg === undefined)
      throw problems.validation(
        [
          {
            pointer,
            message: `CA '${name}' does not exist in vpn.pki.cas`,
            rule: 'vpn.pki-reference-exists',
          },
        ],
        `CA '${name}' does not exist`,
      );
    if (cfg.certificateRef === undefined)
      throw problems.validation([{ pointer, message: `CA '${name}' has no certificateRef` }]);
    const pem = await this.readSecret(cfg.certificateRef);
    if (pem === null)
      throw problems.validation([
        {
          pointer,
          message: `the certificate of CA '${name}' (${cfg.certificateRef}) is not in the secret store`,
        },
      ]);
    const facts = certFacts(firstCert(pem, pointer));
    if (!facts.ca)
      throw problems.validation([
        {
          pointer,
          message: `CA '${name}' is not a CA certificate (basicConstraints CA:TRUE is missing)`,
        },
      ]);
    return { cfg, facts };
  }

  /** The signing key of a CA (`key/<name>`), checked against its certificate. */
  private async caKey(
    name: string,
    facts: CertFacts & { x509: X509Certificate },
    pointer: string,
  ): Promise<KeyObject> {
    const pem = await this.readSecret(refs.key(name));
    if (pem === null)
      throw problems.validation([
        {
          pointer,
          message: `CA '${name}' has no signing key in the secret store (${refs.key(name)}): an imported CA cannot sign; import its key with a PKCS#12 file`,
        },
      ]);
    const key = readPrivateKey(pem, pointer);
    if (!facts.x509.checkPrivateKey(key))
      throw problems.validation([
        { pointer, message: `${refs.key(name)} does not match the certificate of CA '${name}'` },
      ]);
    return key;
  }

  // ---- actions ----

  /** Generates a self-signed CA: key `key/<name>`, certificate `cert/<name>`, staged as vpn.pki.cas.<name>. */
  async createCa(
    body: {
      name: string;
      subject: string;
      keySpec?: Partial<KeySpec> | undefined;
      days: number;
      description?: string | undefined;
      stage: boolean;
      replace: boolean;
    },
    user: Principal,
  ) {
    const rdns = parseDn(body.subject, '/subject');
    const spec = normaliseKeySpec(body.keySpec);
    await this.refuseExisting([refs.cert(body.name), refs.key(body.name)], body.replace);
    const key = generateKey(spec);
    const ca = selfSignedCa(rdns, key, body.days, this.now());
    await this.store(refs.key(body.name), privateKeyPem(key.privateKey), user, body.replace);
    await this.store(refs.cert(body.name), ca.pem, user, body.replace);
    const patch: Json = {
      certificateRef: refs.cert(body.name),
      keySpec: spec,
      issued: issuedOf(ca.facts),
    };
    if (body.description !== undefined) patch['description'] = body.description;
    const staging = await this.stage(user, `/vpn/pki/cas/${body.name}`, patch, body.stage);
    return {
      name: body.name,
      certificateRef: refs.cert(body.name),
      keyRef: refs.key(body.name),
      keySpec: spec,
      issued: issuedOf(ca.facts),
      certificatePem: ca.pem,
      ...staging,
    };
  }

  /** Generates a key pair (`key/<name>`) and a CSR for it (returned; the key never is). */
  async createCsr(
    body: {
      name: string;
      subject: string;
      san: string[];
      keySpec?: Partial<KeySpec> | undefined;
      replace: boolean;
    },
    user: Principal,
  ) {
    const rdns = parseDn(body.subject, '/subject');
    const spec = normaliseKeySpec(body.keySpec);
    await this.refuseExisting([refs.key(body.name)], body.replace);
    const key = generateKey(spec);
    const der = buildCsr(rdns, body.san, key);
    await this.store(refs.key(body.name), privateKeyPem(key.privateKey), user, body.replace);
    return {
      name: body.name,
      keyRef: refs.key(body.name),
      csr: { subject: body.subject, san: body.san, keySpec: spec },
      csrPem: toPem('CERTIFICATE REQUEST', der),
    };
  }

  /** The internal CA `ca` signs a CSR: `cert/<name>`, staged as vpn.pki.certificates.<name> when its key is ours. */
  async sign(
    body: {
      ca: string;
      name: string;
      csrPem: string;
      days: number;
      san?: string[] | undefined;
      stage: boolean;
      replace: boolean;
    },
    user: Principal,
  ) {
    const { cfg, facts } = await this.caByName(body.ca, '/ca');
    const key = await this.caKey(body.ca, facts, '/ca');
    const csr = parseCsr(body.csrPem, '/csrPem');
    await this.refuseExisting([refs.cert(body.name)], body.replace);
    const cert = signCsr(
      csr,
      { facts, key },
      {
        days: body.days,
        now: this.now(),
        ...(body.san !== undefined ? { san: body.san } : {}),
        ...(cfg.crl?.url !== undefined ? { crlUrl: cfg.crl.url } : {}),
        ...(cfg.ocspUrl !== undefined ? { ocspUrl: cfg.ocspUrl } : {}),
      },
    );
    await this.store(refs.cert(body.name), cert.pem, user, body.replace);
    const patch: Json = {
      certificateRef: refs.cert(body.name),
      ca: body.ca,
      issued: issuedOf(cert.facts),
    };
    const csrRecord: Json = { subject: csr.subject, san: body.san ?? sanValues(csr.san) };
    if (csr.keySpec !== undefined) csrRecord['keySpec'] = csr.keySpec;
    patch['csr'] = csrRecord;
    let staging: Staging;
    const ownKey = await this.readSecret(refs.key(body.name));
    if (
      ownKey !== null &&
      new X509Certificate(cert.der).checkPrivateKey(readPrivateKey(ownKey, '/name'))
    ) {
      patch['privateKeyRef'] = refs.key(body.name);
      Object.assign(patch, await this.alertDaysFix(body.name, cert.facts));
      staging = await this.stage(user, `/vpn/pki/certificates/${body.name}`, patch, body.stage);
    } else {
      staging = {
        staged: false,
        pointer: `/vpn/pki/certificates/${body.name}`,
        patch,
        reason: `the private key of this CSR is not ${refs.key(body.name)}: import the certificate with its key reference (POST /api/v1/actions/pki/import)`,
      };
    }
    return {
      name: body.name,
      certificateRef: refs.cert(body.name),
      issued: issuedOf(cert.facts),
      certificatePem: cert.pem,
      ...staging,
    };
  }

  /** expiryAlertDays must stay below the validity (schema rule): lower it in the patch when the current value would not. */
  private async alertDaysFix(name: string, f: CertFacts): Promise<Json> {
    const { candidate } = await this.docs();
    const current = candidate.certificates[name]?.expiryAlertDays ?? 30;
    const validity = (Date.parse(f.notAfter) - Date.parse(f.notBefore)) / DAY;
    return current < validity ? {} : { expiryAlertDays: Math.max(1, Math.floor(validity / 3)) };
  }

  /** Imports a CA or a certificate (PEM with key PEM / key reference, or PKCS#12). */
  async import(body: ImportBody, user: Principal) {
    if (
      body.format === 'pem' &&
      body.privateKeyPem !== undefined &&
      body.privateKeyRef !== undefined
    ) {
      throw problems.validation([
        {
          pointer: '/privateKeyRef',
          message: 'send either privateKeyPem or privateKeyRef, never both',
        },
      ]);
    }
    const now = this.now();
    let chain: Buffer[];
    let key: KeyObject | undefined;
    if (body.format === 'pkcs12') {
      const p12 = parsePkcs12(Buffer.from(body.pkcs12, 'base64'), body.passphrase);
      key = p12.key;
      const leafIdx = p12.certs.findIndex((c) => new X509Certificate(c).checkPrivateKey(p12.key));
      if (leafIdx < 0)
        throw problems.validation([
          {
            pointer: '/pkcs12',
            message: 'no certificate in the PKCS#12 file matches its private key',
          },
        ]);
      chain = [p12.certs[leafIdx]!, ...p12.certs.filter((_, i) => i !== leafIdx)];
    } else {
      const blocks = pemBlocks(body.certificatePem);
      if (blocks.some((b) => isPrivateKeyLabel(b.label)))
        throw problems.validation([
          {
            pointer: '/certificatePem',
            message:
              'the certificate PEM holds a private key: send the key in privateKeyPem (it is stored encrypted, never in a certificate file)',
          },
        ]);
      chain = blocks.filter((b) => b.label === 'CERTIFICATE').map((b) => b.der);
      if (chain.length === 0)
        throw problems.validation([
          { pointer: '/certificatePem', message: 'no PEM CERTIFICATE block' },
        ]);
      if (body.privateKeyPem !== undefined)
        key = readPrivateKey(body.privateKeyPem, '/privateKeyPem');
      else if (body.privateKeyRef !== undefined) {
        const pem = await this.readSecret(body.privateKeyRef);
        if (pem === null)
          throw problems.validation([
            {
              pointer: '/privateKeyRef',
              message: `${body.privateKeyRef} is not in the secret store`,
            },
          ]);
        key = readPrivateKey(pem, '/privateKeyRef');
      }
    }
    if (chain.length > 8)
      throw problems.validation([
        { pointer: '', message: 'at most 8 certificates (leaf + chain)' },
      ]);
    const certs = chain.map((der, i) => {
      try {
        return certFacts(der);
      } catch (e) {
        throw problems.validation([
          {
            pointer: body.format === 'pkcs12' ? '/pkcs12' : '/certificatePem',
            message: `certificate ${i + 1}: ${(e as Error).message}`,
          },
        ]);
      }
    });
    const leaf = certs[0]!;
    const src = body.format === 'pkcs12' ? '/pkcs12' : '/certificatePem';
    for (let i = 0; i + 1 < certs.length; i++) {
      if (
        !certs[i + 1]!.ca ||
        !certs[i]!.x509.checkIssued(certs[i + 1]!.x509) ||
        !certs[i]!.x509.verify(certs[i + 1]!.x509.publicKey)
      )
        throw problems.validation([
          {
            pointer: src,
            message: `the chain is broken: certificate ${i + 1} ('${certs[i]!.subject}') is not issued by certificate ${i + 2} ('${certs[i + 1]!.subject}')`,
          },
        ]);
    }
    if (Date.parse(leaf.notBefore) > now.getTime())
      throw problems.validation([
        { pointer: src, message: `the certificate is not valid before ${leaf.notBefore}` },
      ]);
    if (Date.parse(leaf.notAfter) < now.getTime())
      throw problems.validation([
        { pointer: src, message: `the certificate expired on ${leaf.notAfter}` },
      ]);
    if (key !== undefined && !leaf.x509.checkPrivateKey(key)) {
      const kp =
        body.format === 'pkcs12'
          ? '/pkcs12'
          : body.privateKeyPem !== undefined
            ? '/privateKeyPem'
            : '/privateKeyRef';
      throw problems.validation([
        { pointer: kp, message: 'the private key does not match the certificate' },
      ]);
    }
    const pem = chain.map(pemCert).join('');
    if (body.as === 'ca') {
      if (!leaf.ca)
        throw problems.validation([
          {
            pointer: src,
            message:
              'not a CA certificate (basicConstraints CA:TRUE is missing): import it as a certificate',
          },
        ]);
      const writes = [refs.cert(body.name), ...(key !== undefined ? [refs.key(body.name)] : [])];
      await this.refuseExisting(writes, body.replace);
      if (key !== undefined)
        await this.store(refs.key(body.name), privateKeyPem(key), user, body.replace);
      await this.store(refs.cert(body.name), pem, user, body.replace);
      const patch: Json = { certificateRef: refs.cert(body.name), issued: issuedOf(leaf) };
      const ks = key !== undefined ? keySpecOf(leaf.x509.publicKey) : undefined;
      if (ks !== undefined) patch['keySpec'] = ks;
      const staging = await this.stage(user, `/vpn/pki/cas/${body.name}`, patch, body.stage);
      return {
        name: body.name,
        as: 'ca' as const,
        certificateRef: refs.cert(body.name),
        keyRef: key !== undefined ? refs.key(body.name) : null,
        chainLength: certs.length,
        issued: issuedOf(leaf),
        ...staging,
      };
    }
    // a certificate: optionally checked against a CA of the configuration
    if (body.ca !== undefined) {
      const { facts } = await this.caByName(body.ca, '/ca');
      if (!leaf.x509.checkIssued(facts.x509) || !leaf.x509.verify(facts.x509.publicKey))
        throw problems.validation([
          {
            pointer: '/ca',
            message: `the certificate was not issued by CA '${body.ca}' ('${facts.subject}')`,
          },
        ]);
    }
    if (key === undefined)
      throw problems.validation([
        {
          pointer: '/privateKeyPem',
          message:
            'a certificate needs its private key: privateKeyPem, privateKeyRef or a PKCS#12 file',
        },
      ]);
    const keyRef =
      body.format === 'pem' && body.privateKeyRef !== undefined
        ? body.privateKeyRef
        : refs.key(body.name);
    const writes = [refs.cert(body.name), ...(keyRef === refs.key(body.name) ? [keyRef] : [])];
    await this.refuseExisting(writes, body.replace);
    if (keyRef === refs.key(body.name))
      await this.store(keyRef, privateKeyPem(key), user, body.replace);
    await this.store(refs.cert(body.name), pem, user, body.replace);
    const patch: Json = {
      certificateRef: refs.cert(body.name),
      privateKeyRef: keyRef,
      issued: issuedOf(leaf),
      ...(await this.alertDaysFix(body.name, leaf)),
    };
    if (body.ca !== undefined) patch['ca'] = body.ca;
    const staging = await this.stage(user, `/vpn/pki/certificates/${body.name}`, patch, body.stage);
    return {
      name: body.name,
      as: 'certificate' as const,
      certificateRef: refs.cert(body.name),
      keyRef,
      chainLength: certs.length,
      issued: issuedOf(leaf),
      ...staging,
    };
  }

  /** Public material of a CA or certificate (or a CA's CRL) as PEM; a private key is never exported (403). */
  async exportPem(name: string, kind: 'ca' | 'certificate' | 'crl' | 'key') {
    if (kind === 'key')
      throw problems.forbidden(
        'private keys are never exported (F-pki): only certificates, chains and CRLs',
      );
    const { running, candidate } = await this.docs();
    const ref =
      kind === 'crl'
        ? (candidate.cas[name] ?? running.cas[name]) !== undefined
          ? refs.crl(name)
          : undefined
        : kind === 'ca'
          ? (candidate.cas[name] ?? running.cas[name])?.certificateRef
          : (candidate.certificates[name] ?? running.certificates[name])?.certificateRef;
    if (ref === undefined)
      throw problems.notFound(`no ${kind === 'crl' ? 'CA' : kind} '${name}' in vpn.pki`);
    const text = await this.readSecret(ref);
    if (text === null)
      throw problems.notFound(
        `${ref} is not in the secret store${kind === 'crl' ? ' (refresh the CRL first)' : ''}`,
      );
    const want = kind === 'crl' ? 'X509 CRL' : 'CERTIFICATE';
    const blocks = pemBlocks(text).filter((b) => b.label === want); // defence in depth: nothing but public blocks leaves
    if (blocks.length === 0) throw problems.notFound(`${ref} holds no ${want}`);
    const pem = blocks.map((b) => toPem(want, b.der)).join('');
    const fingerprint = kind === 'crl' ? null : new X509Certificate(blocks[0]!.der).fingerprint256;
    return { name, kind, ref, pem, fingerprint };
  }

  /** Fetches and verifies the CRL of one CA (or of every CA with a CRL URL) and stores it as `cert/<ca>.crl`. */
  async refreshCrl(ca: string | undefined, user: Principal | null) {
    const { running } = await this.docs();
    const names =
      ca !== undefined
        ? [ca]
        : Object.keys(running.cas)
            .filter((n) => running.cas[n]?.crl?.url !== undefined)
            .sort();
    if (ca !== undefined && running.cas[ca] === undefined)
      throw problems.validation([
        { pointer: '/ca', message: `CA '${ca}' does not exist in the running vpn.pki.cas` },
      ]);
    if (ca !== undefined && running.cas[ca]?.crl?.url === undefined)
      throw problems.validation([{ pointer: '/ca', message: `CA '${ca}' has no crl.url` }]);
    const results: (CrlState & { ca: string; stored: boolean })[] = [];
    for (const name of names)
      results.push({ ca: name, ...(await this.refreshOne(name, running.cas[name]!, user)) });
    return { results };
  }

  private async refreshOne(
    name: string,
    cfg: CaCfg,
    user: Principal | null,
  ): Promise<CrlState & { stored: boolean }> {
    const url = cfg.crl!.url!;
    const prev = this.crls.get(name);
    const st: CrlState = {
      url,
      fetchedAt: this.now().toISOString(),
      thisUpdate: prev?.thisUpdate ?? null,
      nextUpdate: prev?.nextUpdate ?? null,
      revoked: prev?.revoked ?? [],
      number: prev?.number ?? null,
      error: null,
    };
    let stored = false;
    try {
      const caPem =
        cfg.certificateRef !== undefined ? await this.readSecret(cfg.certificateRef) : null;
      if (caPem === null)
        throw new PkiError(
          `the CA certificate (${cfg.certificateRef ?? 'no certificateRef'}) is not in the secret store`,
        );
      const facts = certFacts(firstCert(caPem, ''));
      const crl = parseCrl(
        await this.fetcher(url, {
          maxBytes: MAX_CRL,
          timeoutMs: 15_000,
          accept: 'application/pkix-crl',
        }),
        facts,
      );
      Object.assign(st, {
        thisUpdate: crl.thisUpdate,
        nextUpdate: crl.nextUpdate,
        revoked: crl.revoked.map((r) => r.serial),
        number: crl.number,
      });
      const current = await this.readSecret(refs.crl(name));
      if (current !== crl.pem) {
        // a CRL is public; it goes through the secret store only because that is the agent's (pending) channel
        await this.secrets.put('cert', `${name}.crl`, crl.pem, {
          replace: current !== null,
          userId: user?.id ?? null,
        });
        stored = true;
      }
    } catch (e) {
      st.error = e instanceof PkiError ? e.message : 'refresh failed';
      if (!(e instanceof PkiError))
        this.log.warn(
          `CRL refresh of CA ${name} failed: ${e instanceof Error ? e.message : String(e)}`,
        );
    }
    this.crls.set(name, st);
    return { ...st, stored };
  }

  /** The hourly part of the job: refresh every CRL whose refresh interval has elapsed (or that was never fetched). */
  async refreshDueCrls(): Promise<void> {
    const { running } = await this.docs();
    for (const [name, cfg] of Object.entries(running.cas)) {
      if (cfg.crl?.url === undefined) continue;
      const last = this.crls.get(name)?.fetchedAt;
      const every = (cfg.crl.refreshIntervalSec ?? 86_400) * 1000;
      if (last === null || last === undefined || Date.parse(last) + every <= this.now().getTime())
        await this.refreshOne(name, cfg, null);
    }
  }

  /** OCSP check of one certificate (or of every certificate whose CA names an OCSP responder). */
  async checkOcsp(name: string | undefined) {
    const { running } = await this.docs();
    const names = name !== undefined ? [name] : Object.keys(running.certificates).sort();
    if (name !== undefined && running.certificates[name] === undefined)
      throw problems.validation([
        {
          pointer: '/certificate',
          message: `certificate '${name}' does not exist in the running vpn.pki.certificates`,
        },
      ]);
    const results: (OcspState & { certificate: string })[] = [];
    for (const n of names) {
      const c = running.certificates[n]!;
      const ca = c.ca !== undefined ? running.cas[c.ca] : undefined;
      if (
        ca?.ocspUrl === undefined ||
        c.certificateRef === undefined ||
        ca.certificateRef === undefined
      ) {
        if (name !== undefined)
          throw problems.validation([
            { pointer: '/certificate', message: `certificate '${n}' has no CA with an ocspUrl` },
          ]);
        continue;
      }
      const st: OcspState = {
        url: ca.ocspUrl,
        checkedAt: this.now().toISOString(),
        status: 'error',
        revokedAt: null,
        error: null,
      };
      try {
        const [leafPem, caPem] = await Promise.all([
          this.readSecret(c.certificateRef),
          this.readSecret(ca.certificateRef),
        ]);
        if (leafPem === null || caPem === null)
          throw new PkiError('certificate material is not in the secret store');
        const leaf = certFacts(firstCert(leafPem, ''));
        const r = await ocspCheck(
          ca.ocspUrl,
          Buffer.from(leaf.serial.replace(/:/g, ''), 'hex'),
          certFacts(firstCert(caPem, '')),
          this.fetcher,
        );
        st.status = r.status;
        st.revokedAt = r.revokedAt;
      } catch (e) {
        st.error = e instanceof PkiError ? e.message : 'OCSP check failed';
      }
      this.ocsp.set(n, st);
      results.push({ certificate: n, ...st });
    }
    return { results };
  }

  /**
   * The expiry job (daily, and once a minute after start-up): every CA and certificate of running whose notAfter is
   * within its alert window (certificates: expiryAlertDays; CAs: 30 days) raises an alarm-shaped message on the bus,
   * and one that left the window (renewed, removed) clears it. Until F-dashboard-prom-alarms consumes a `pki.expiry`
   * topic (bus.ts is outside this envelope) the messages go to the existing `alarm.events` topic.
   */
  async checkExpiry(): Promise<ExpiryAlert[]> {
    const now = this.now();
    const { running } = await this.docs();
    const seen = new Map<string, ExpiryAlert>();
    const consider = async (
      kind: 'ca' | 'certificate',
      name: string,
      ref: string | undefined,
      alertDays: number,
    ) => {
      if (ref === undefined) return;
      const pem = await this.readSecret(ref).catch(() => null);
      if (pem === null) return;
      let facts: CertFacts;
      try {
        facts = certFacts(firstCert(pem, ''));
      } catch {
        return;
      }
      const left = daysLeft(facts.notAfter, now);
      if (left <= alertDays)
        seen.set(`${kind}/${name}`, {
          kind,
          name,
          notAfter: facts.notAfter,
          daysLeft: left,
          alertDays,
          severity: Date.parse(facts.notAfter) < now.getTime() ? 'critical' : 'warning',
        });
    };
    for (const [name, c] of Object.entries(running.cas))
      await consider('ca', name, c.certificateRef, 30);
    for (const [name, c] of Object.entries(running.certificates))
      await consider('certificate', name, c.certificateRef, c.expiryAlertDays ?? 30);
    for (const [key, a] of seen) {
      const prev = this.alerts.get(key);
      if (prev === undefined || prev.daysLeft !== a.daysLeft || prev.severity !== a.severity) {
        this.bus.publish('alarm.events', {
          type: 'raised',
          rule: 'pki.expiry',
          instance: key,
          metric: 'pki_days_left',
          value: a.daysLeft,
          severity: a.severity,
          message:
            a.severity === 'critical'
              ? `${a.kind} '${a.name}' expired on ${a.notAfter}`
              : `${a.kind} '${a.name}' expires in ${a.daysLeft} days (${a.notAfter})`,
        });
      }
      this.alerts.set(key, a);
    }
    for (const key of [...this.alerts.keys()]) {
      if (!seen.has(key)) {
        this.alerts.delete(key);
        this.bus.publish('alarm.events', { type: 'cleared', rule: 'pki.expiry', instance: key });
      }
    }
    this.lastExpiryCheck = now.toISOString();
    return [...seen.values()];
  }

  /** `GET /api/v1/state/pki`: every CA and certificate of running, read from the material (never key material). */
  async state() {
    const now = this.now();
    const { running } = await this.docs();
    const cas = await Promise.all(
      Object.entries(running.cas)
        .sort(([a], [b]) => a.localeCompare(b))
        .map(([name, cfg]) => this.caState(name, cfg, now)),
    );
    const caFacts = new Map(
      cas.flatMap((c) => (c.facts !== undefined ? [[c.name, c.facts] as const] : [])),
    );
    const certificates = await Promise.all(
      Object.entries(running.certificates)
        .sort(([a], [b]) => a.localeCompare(b))
        .map(([name, cfg]) => this.certState(name, cfg, caFacts, now)),
    );
    const files = await this.files.state();
    return {
      generatedAt: now.toISOString(),
      cas: cas.map(({ facts: _f, ...rest }) => rest),
      certificates,
      expiry: { lastCheck: this.lastExpiryCheck, active: [...this.alerts.values()] },
      agentFiles:
        'error' in files
          ? { root: null, unavailable: files.error, files: [] }
          : {
              root: files.root || null,
              unavailable: files.unavailable || null,
              files: files.files.map((f) => ({
                kind: f.kind,
                name: f.name,
                ref: f.ref,
                fingerprint: f.fingerprint,
                mode: f.mode.toString(8).padStart(4, '0'),
                size: f.size,
                present: f.present,
              })),
            },
      unsupported:
        running.hsm?.enabled === true
          ? ['vpn.pki.hsm: PKCS#11 / HSM is not supported in this build']
          : [],
    };
  }

  private async caState(name: string, cfg: CaCfg, now: Date) {
    const problemsOf: string[] = [];
    let facts: (CertFacts & { x509: X509Certificate }) | undefined;
    if (cfg.certificateRef !== undefined) {
      const pem = await this.readSecret(cfg.certificateRef);
      if (pem === null) problemsOf.push(`${cfg.certificateRef} is not in the secret store`);
      else {
        try {
          facts = certFacts(firstCert(pem, ''));
          if (!facts.ca)
            problemsOf.push('not a CA certificate (basicConstraints CA:TRUE is missing)');
        } catch (e) {
          problemsOf.push((e as Error).message);
        }
      }
    }
    let signingKey = false;
    if (facts !== undefined) {
      const keyPem = await this.readSecret(refs.key(name));
      if (keyPem !== null) {
        try {
          signingKey = facts.x509.checkPrivateKey(readPrivateKey(keyPem, ''));
        } catch {
          signingKey = false;
        }
      }
    }
    const crl = this.crls.get(name);
    return {
      name,
      facts,
      certificateRef: cfg.certificateRef ?? null,
      ...publicFacts(facts, now),
      signingKey,
      crl:
        cfg.crl?.url === undefined
          ? null
          : {
              url: cfg.crl.url,
              refreshIntervalSec: cfg.crl.refreshIntervalSec ?? 86_400,
              fetchedAt: crl?.fetchedAt ?? null,
              ageSec: crl?.thisUpdate
                ? Math.floor((now.getTime() - Date.parse(crl.thisUpdate)) / 1000)
                : null,
              thisUpdate: crl?.thisUpdate ?? null,
              nextUpdate: crl?.nextUpdate ?? null,
              revoked: crl?.revoked.length ?? null,
              error: crl?.error ?? null,
            },
      ocspUrl: cfg.ocspUrl ?? null,
      problems: problemsOf,
    };
  }

  private async certState(
    name: string,
    cfg: CertCfg,
    cas: Map<string, CertFacts & { x509: X509Certificate }>,
    now: Date,
  ) {
    const problemsOf: string[] = [];
    let facts: (CertFacts & { x509: X509Certificate }) | undefined;
    if (cfg.acme !== undefined)
      problemsOf.push('ACME is not supported in this build: the certificate is never issued');
    if (cfg.certificateRef !== undefined) {
      const pem = await this.readSecret(cfg.certificateRef);
      if (pem === null) problemsOf.push(`${cfg.certificateRef} is not in the secret store`);
      else {
        try {
          facts = certFacts(firstCert(pem, ''));
        } catch (e) {
          problemsOf.push((e as Error).message);
        }
      }
    }
    if (facts !== undefined && cfg.privateKeyRef !== undefined) {
      const keyPem = await this.readSecret(cfg.privateKeyRef);
      if (keyPem === null) problemsOf.push(`${cfg.privateKeyRef} is not in the secret store`);
      else {
        try {
          if (!facts.x509.checkPrivateKey(readPrivateKey(keyPem, '')))
            problemsOf.push(`${cfg.privateKeyRef} does not match the certificate`);
        } catch (e) {
          problemsOf.push((e as Error).message);
        }
      }
    }
    let revokedByCrl = false;
    if (facts !== undefined && cfg.ca !== undefined) {
      const ca = cas.get(cfg.ca);
      if (ca === undefined) problemsOf.push(`CA '${cfg.ca}' has no usable certificate`);
      else if (!facts.x509.checkIssued(ca.x509) || !facts.x509.verify(ca.x509.publicKey))
        problemsOf.push(`not issued by CA '${cfg.ca}'`);
      revokedByCrl = this.crls.get(cfg.ca)?.revoked.includes(facts.serial) ?? false;
      if (revokedByCrl) problemsOf.push(`revoked (listed in the CRL of CA '${cfg.ca}')`);
    }
    const alertDays = cfg.expiryAlertDays ?? 30;
    const pf = publicFacts(facts, now);
    if (facts !== undefined) {
      const validity = (Date.parse(facts.notAfter) - Date.parse(facts.notBefore)) / DAY;
      if (alertDays >= validity)
        problemsOf.push(
          `expiryAlertDays (${alertDays}) is not shorter than the validity (${Math.floor(validity)} days)`,
        );
    }
    const ocsp = this.ocsp.get(name);
    if (ocsp?.status === 'revoked') problemsOf.push('revoked (OCSP)');
    return {
      name,
      ca: cfg.ca ?? null,
      certificateRef: cfg.certificateRef ?? null,
      privateKeyRef: cfg.privateKeyRef ?? null,
      ...pf,
      expiryAlertDays: alertDays,
      expiring: pf.daysLeft !== null && pf.daysLeft <= alertDays,
      revokedByCrl,
      ocsp:
        ocsp === undefined
          ? null
          : {
              status: ocsp.status,
              checkedAt: ocsp.checkedAt,
              revokedAt: ocsp.revokedAt,
              error: ocsp.error,
            },
      problems: problemsOf,
    };
  }
}

/** The public facts the state shows (null fields when the material is not readable). */
function publicFacts(f: CertFacts | undefined, now: Date) {
  return {
    subject: f?.subject ?? null,
    issuer: f?.issuer ?? null,
    serial: f?.serial ?? null,
    san: f?.san ?? [],
    notBefore: f?.notBefore ?? null,
    notAfter: f?.notAfter ?? null,
    daysLeft: f !== undefined ? daysLeft(f.notAfter, now) : null,
    fingerprint: f?.fingerprint ?? null,
    keySpec: f?.keySpec ?? null,
    isCa: f?.ca ?? null,
  };
}

/** The first CERTIFICATE block of a PEM text, as DER. */
function firstCert(pem: string, pointer: string): Buffer {
  const b = pemBlocks(pem).find((x) => x.label === 'CERTIFICATE');
  if (b === undefined) throw new PkiError('no PEM CERTIFICATE block', pointer);
  return b.der;
}

export type ImportBody =
  | {
      format: 'pem';
      as: 'ca' | 'certificate';
      name: string;
      certificatePem: string;
      privateKeyPem?: string | undefined;
      privateKeyRef?: string | undefined;
      ca?: string | undefined;
      stage: boolean;
      replace: boolean;
    }
  | {
      format: 'pkcs12';
      as: 'ca' | 'certificate';
      name: string;
      pkcs12: string;
      passphrase: string;
      ca?: string | undefined;
      stage: boolean;
      replace: boolean;
    };

export type { ProblemIssue };
