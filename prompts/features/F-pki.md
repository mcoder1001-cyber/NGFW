# Task: F-pki — CA, CSR, import/export, CRL/OCSP, expiry alerts   (prepend 00-CONTEXT.md)

## Goal
A small **certificate authority and certificate store** end to end in FAST MODE: create an internal CA, generate key + CSR, sign
or import certificates, export public parts, refresh CRLs, check OCSP, alert before expiry — and deliver the files strongSwan needs
(`x509/`, `x509ca/`, `private/`, CRLs) through the agent. Reference: TNSR "PKI / Certificate Manager"; strongSwan swanctl file layout
(WBS D6.4 in `plan/wbs.csv`). No VPP objects.

## Inputs to read first
- `packages/schema/src/domains/vpn.ts` — `vpn.pki{cas.<name>{certificateRef (cert/<n>), crl{url, refreshIntervalSec}, ocspUrl},
  certificates.<name>{certificateRef?, privateKeyRef (key/<n>), ca?, acme?, expiryAlertDays}, hsm?}`; semantic refs in `semantic/vpn.ts`
  (IPsec/remote-access `certificate`/`remoteCa`/`clientCa` must name these)
- The P06 secret store (`apps/api/src/secrets/`, `POST /api/v1/secrets` returns a ref; D-051 kinds cert/key) — PKI material lives there,
  encrypted at rest; the config document only carries refs
- `prompts/P11-strongswan-vpp.md` and the merged renderer doc `docs/agent/renderers/strongswan.md` (RF-2 + P11): `certs = <name>.pem`,
  `cacerts = <ca>.pem`, "F-pki installs the files in `x509/`, `x509ca/`"
- `apps/agent/internal/renderers/{renderer.go,helpers_files.go}` — atomic file writes, modes; ALLOWLIST for any exec (prefer Go `crypto/x509`, no `openssl` exec)
- `docs/decisions/LOG.md` D-040 (no auth material over the agent boundary except what daemons need), D-046, D-051; the API→agent secret
  channel is `docs/decisions/PENDING-secret-channel.md` — the materialiser needs cert/key material in the agent: use what P11 merged, else
  build against the `vpn.Resolver` interface with a fixture and leave only the end-to-end step open

## Scope — build exactly this
1. **Schema**: semantic rules — a certificate needs `certificateRef` or `acme` (exists); `ca` must name a CA; CA certificates must be
   CA:TRUE; `expiryAlertDays` < validity. New fields (CSR subject/SAN/key type, `issued` metadata) → `contract(schema|proto): …` commits on
   your task branch, additive (no `contract/` branch; numbers from `docs/status/wave-BC-numbers.md`).
2. **API** (the bulk of this task, `apps/api/src/features/pki/`): actions `POST /api/v1/actions/pki/ca` (generate self-signed CA:
   ECDSA P-256/P-384 or RSA 2048/3072/4096), `…/csr` (key pair + CSR, key stored as secret, CSR returned), `…/sign` (internal CA signs a CSR),
   `…/import` (PEM / PKCS#12 with passphrase; validates chain, key match), `GET …/export/{name}` (certificate/chain PEM only — **never**
   private keys), `…/crl/refresh`; state `GET /api/v1/state/pki` (subject, issuer, SAN, notBefore/notAfter, days left, CRL age, OCSP
   status); a daily expiry job raising an event/alarm topic consumed later by F-dashboard-prom-alarms. Node `crypto` / a vetted pure-JS
   X.509 lib — no shell. Node 22 `crypto` parses certificates but cannot create certificates or CSRs, so the library is a new dependency:
   write the options (licence, size) in the questions file first; the manager installs it on main. OpenAPI; regenerate `packages/api-client`.
3. **Agent** (`apps/agent/internal/pki/`): a file materialiser (default: its own singleton scheduler descriptor that the strongSwan renderer
   depends on — D-109 d): given the resolved cert/key/CA refs of
   the desired state, write `/etc/swanctl/{x509,x509ca,private,x509crl}/<name>.pem` atomically (keys 0600 root), remove files no longer
   referenced, Retrieve = fingerprints of what is on disk (no key material). Tests use a temp root, never `/etc`.
4. **UI**: PKI page — CAs, certificates (expiry chips), CSR wizard, import dialog, export button; en + fa; screenshot against the real endpoint.
5. **Docs**: `docs/user/vpn/pki.md` — internal CA → server cert for an IKEv2 tunnel, importing a third-party chain, CLI equivalent.
Files you own (the envelope's list wins): `apps/agent/internal/pki/**`, `apps/agent/internal/{desired,subsystems}/pki*.go`,
`apps/agent/internal/agent/rpc_pki*.go`, `apps/api/src/features/pki/**`, `apps/web/src/domains/vpn/pki/**`, `apps/web/src/locales/*/pki.json`,
`docs/user/vpn/pki.md`, `test/topology/pki/**`. Shared files: one-line appends only. P11 is merged before you start (board dep): add the
renderer hook yourself as ONE named hunk in its strongSwan renderer (F-ra-vpn inherits it) — never change its S2S behaviour.

## Acceptance (paste the evidence)
- [ ] CA → CSR → sign → `openssl verify -CAfile ca.pem cert.pem` OK (run by the test, output pasted); PKCS#12 import round-trip
- [ ] Agent writes the files for a cert-auth tunnel; `swanctl --list-certs` inside the test namespace shows them (or, when the manager has
      not handed you daemon-owner strongswan, the file tree + modes pasted)
- [ ] Agent-restart simulation → missing files re-materialised within 30 s; rollback removes unreferenced files (Retrieve)
- [ ] Certificate with `ca: "missing"` → 400 problem+json with a `pointer`; export of a private key → 403/404, never material
- [ ] No private key or passphrase in logs, GET, audit, fixtures, status files (grep evidence); `tools/ci.sh --base main` green

## Out of scope (do not build)
ACME issuance (schema field exists — reject `acme` blocks with "not supported in this build", FAST MODE); PKCS#11/HSM (`vpn.pki.hsm` →
same); OCSP responder/CA revocation service of our own (only CRL fetch + OCSP check); the swanctl renderer and IPsec tunnels (P11);
native IKEv2 cert paths (F-ikev2-native); remote-access EAP-TLS (F-ra-vpn); HTTPS certificate of the management UI (P10/F-hardening-lite);
alarm delivery channels (F-dashboard-prom-alarms).

## Open questions to surface, not to decide silently
Whether CA private keys may leave the API host for the agent (only signing happens API-side — default: never sent).
Where F-ikev2-native's rsa-sig key file lives (`ikev2_set_local_key` needs a path on the VPP host) — propose the same materialiser.
