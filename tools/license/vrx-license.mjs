#!/usr/bin/env node
// vrx-license — issue and check VRX `.vrxlic` licence files (F-licensing). Dependency-free (node:crypto only).
//
//   vrx-license keygen  --out-dir <dir outside any git repo>
//   vrx-license issue   --key <private.pem> --customer <name> [--id <licenseId>] (--days <n> | --expires <iso>)
//                       [--not-before <iso>] [--serial <s>] [--machine-id <id> | --machine-id-hash <sha256hex>]
//                       [--features a,b,c] [--limit name=n ...] --out <file.vrxlic>
//   vrx-license verify  --pub <public.pem> [--at <iso>] <file.vrxlic>
//   vrx-license inspect <file.vrxlic>
//
// The canonical JSON and the signature rule match apps/api/src/features/licensing/format.ts (cli.test.ts checks both).
import {
  createHash,
  createPrivateKey,
  createPublicKey,
  generateKeyPairSync,
  randomUUID,
  sign,
  verify,
} from 'node:crypto';
import { chmodSync, existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';

export const LICENSE_FORMAT = 'vrxlic/1';
export const GRACE_DAYS = 30;

export function canonicalJson(value) {
  if (Array.isArray(value)) return `[${value.map(canonicalJson).join(',')}]`;
  if (value !== null && typeof value === 'object') {
    return `{${Object.keys(value)
      .filter((k) => value[k] !== undefined)
      .sort()
      .map((k) => `${JSON.stringify(k)}:${canonicalJson(value[k])}`)
      .join(',')}}`;
  }
  return JSON.stringify(value);
}

export function machineIdHash(id) {
  return createHash('sha256').update(id.trim(), 'utf8').digest('hex');
}

function fail(msg) {
  process.stderr.write(`vrx-license: ${msg}\n`);
  process.exit(2);
}

function parseArgs(argv) {
  const flags = {};
  const pos = [];
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i];
    if (!a.startsWith('--')) {
      pos.push(a);
      continue;
    }
    const k = a.slice(2);
    const v = argv[++i];
    if (v === undefined) fail(`--${k} needs a value`);
    if (k === 'limit') (flags.limit ??= []).push(v);
    else flags[k] = v;
  }
  return { flags, pos };
}

function insideGitRepo(dir) {
  let d = resolve(dir);
  for (;;) {
    if (existsSync(join(d, '.git'))) return true;
    const up = dirname(d);
    if (up === d) return false;
    d = up;
  }
}

function keygen(flags) {
  const dir = flags['out-dir'];
  if (!dir) fail('keygen needs --out-dir');
  if (insideGitRepo(dir))
    fail(`refusing to write a signing key inside a git repository (${resolve(dir)})`);
  mkdirSync(dir, { recursive: true, mode: 0o700 });
  const { privateKey, publicKey } = generateKeyPairSync('ed25519');
  const priv = join(dir, 'vrx-license-signing.pem');
  const pub = join(dir, 'vrx-license-public.pem');
  if (existsSync(priv)) fail(`${priv} exists; not overwriting`);
  writeFileSync(priv, privateKey.export({ type: 'pkcs8', format: 'pem' }), { mode: 0o600 });
  chmodSync(priv, 0o600);
  writeFileSync(pub, publicKey.export({ type: 'spki', format: 'pem' }), { mode: 0o644 });
  process.stdout.write(`signing key: ${resolve(priv)}\npublic key:  ${resolve(pub)}\n`);
}

function isoOr(v, name) {
  if (v === undefined) return undefined;
  const t = Date.parse(v);
  if (Number.isNaN(t)) fail(`--${name}: not an ISO date`);
  return new Date(t).toISOString();
}

function issue(flags) {
  if (!flags.key || !flags.customer || !flags.out) fail('issue needs --key, --customer and --out');
  const key = createPrivateKey(readFileSync(flags.key, 'utf8'));
  if (key.asymmetricKeyType !== 'ed25519') fail('--key is not an Ed25519 key');
  const now = new Date();
  const notBefore = isoOr(flags['not-before'], 'not-before') ?? now.toISOString();
  let expiresAt = isoOr(flags.expires, 'expires');
  if (expiresAt === undefined) {
    const days = Number(flags.days);
    if (!Number.isFinite(days)) fail('issue needs --days or --expires');
    expiresAt = new Date(Date.parse(notBefore) + days * 86_400_000).toISOString();
  }
  const binding = {};
  if (flags.serial) binding.serial = flags.serial;
  if (flags['machine-id']) binding.machineIdHash = machineIdHash(flags['machine-id']);
  if (flags['machine-id-hash']) binding.machineIdHash = flags['machine-id-hash'];
  const limits = {};
  for (const l of flags.limit ?? []) {
    const m = /^([A-Za-z0-9_.-]+)=(\d+)$/.exec(l);
    if (!m) fail(`--limit ${l}: expected name=<integer>`);
    limits[m[1]] = Number(m[2]);
  }
  const license = {
    version: 1,
    licenseId: flags.id ?? randomUUID(),
    customer: flags.customer,
    issuedAt: now.toISOString(),
    notBefore,
    expiresAt,
    binding,
    entitlements: {
      features: (flags.features ?? '')
        .split(',')
        .map((s) => s.trim())
        .filter(Boolean),
      limits,
    },
  };
  const signature = sign(null, Buffer.from(canonicalJson(license), 'utf8'), key).toString('base64');
  writeFileSync(
    flags.out,
    `${JSON.stringify({ format: LICENSE_FORMAT, license, signature }, null, 2)}\n`,
    { mode: 0o644 },
  );
  process.stdout.write(`issued ${license.licenseId} → ${resolve(flags.out)}\n`);
}

function readLicense(file) {
  let f;
  try {
    f = JSON.parse(readFileSync(file, 'utf8'));
  } catch {
    fail(`${file}: not a licence file`);
  }
  if (
    f?.format !== LICENSE_FORMAT ||
    typeof f.signature !== 'string' ||
    typeof f.license !== 'object'
  )
    fail(`${file}: not a ${LICENSE_FORMAT} file`);
  return f;
}

function verifyCmd(flags, pos) {
  if (!flags.pub || !pos[0]) fail('verify needs --pub and a file');
  const pub = createPublicKey(readFileSync(flags.pub, 'utf8'));
  const f = readLicense(pos[0]);
  const ok = verify(
    null,
    Buffer.from(canonicalJson(f.license), 'utf8'),
    pub,
    Buffer.from(f.signature, 'base64'),
  );
  if (!ok) {
    process.stdout.write('INVALID: signature does not verify\n');
    process.exit(1);
  }
  const at = Date.parse(isoOr(flags.at, 'at') ?? new Date().toISOString());
  const exp = Date.parse(f.license.expiresAt);
  let status = 'valid';
  if (at < Date.parse(f.license.notBefore)) status = 'not-yet-valid';
  else if (at > exp + GRACE_DAYS * 86_400_000) status = 'expired';
  else if (at > exp) status = 'grace';
  process.stdout.write(
    `signature OK; status ${status} (licence ${f.license.licenseId}, expires ${f.license.expiresAt})\n`,
  );
  process.exit(status === 'valid' || status === 'grace' ? 0 : 1);
}

function inspect(pos) {
  if (!pos[0]) fail('inspect needs a file');
  const f = readLicense(pos[0]);
  process.stdout.write(`${JSON.stringify(f.license, null, 2)}\n`);
}

const isMain =
  process.argv[1] && resolve(process.argv[1]) === resolve(new URL(import.meta.url).pathname);
if (isMain) {
  const [cmd, ...rest] = process.argv.slice(2);
  const { flags, pos } = parseArgs(rest);
  if (cmd === 'keygen') keygen(flags);
  else if (cmd === 'issue') issue(flags);
  else if (cmd === 'verify') verifyCmd(flags, pos);
  else if (cmd === 'inspect') inspect(pos);
  else
    fail(
      'usage: vrx-license keygen|issue|verify|inspect (see the header of tools/license/vrx-license.mjs)',
    );
}
