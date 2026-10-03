# F-pki — contract changes (additive; for the manager's review)

Two commits on `task/F-pki`, numbers from `docs/status/wave-BC-numbers.md` § F-pki:

## contract(schema): pki csr/issued/key spec — `packages/schema/src/domains/vpn.ts` (PKI hunk only)

| leaf | type | written by | rule |
|---|---|---|---|
| `vpn.pki.cas.<n>.keySpec` | `PkiKeySpec {type ecdsa\|rsa, curve? p256\|p384, bits? 2048\|3072\|4096}` | `POST /actions/pki/ca` | curve only with ecdsa, bits only with rsa |
| `vpn.pki.cas.<n>.issued` | `PkiIssued {subject, issuer, serial, notBefore, notAfter, fingerprint, ca?}` | the PKI actions | `issued.ca === false` → error at `/vpn/pki/cas/<n>/issued/ca` |
| `vpn.pki.certificates.<n>.csr` | `PkiCsr {subject DN, san[] (DNS/IP/e-mail), keySpec}` | `POST /actions/pki/csr` | DN grammar (CN, O, OU, C, L, ST, DC, E, serialNumber) |
| `vpn.pki.certificates.<n>.issued` | `PkiIssued` | the PKI actions | `expiryAlertDays` ≥ validity days → error at `/vpn/pki/certificates/<n>/expiryAlertDays` |
| (PkiSchema) | — | — | `certificates.<n>.issued.issuer` ≠ `cas.<ca>.issued.subject` → error at `/vpn/pki/certificates/<n>/ca` |

Serial numbers and fingerprints are colon-separated hex (`4A:1F:…`) so the `vpn.no-inline-secret-material` guard (which
flags plain hex blobs of 16+ bytes) does not mistake a serial for key material.

## contract(proto): pki leaves + PkiFileState — `packages/proto/vrx/v1/dataplane.proto`

- `PkiCa`: **5 `key_spec`** (`PkiKeySpec`), **6 `issued`** (`PkiIssued`); `PkiCertificate`: **7 `csr`** (`PkiCsr`),
  **8 `issued`** (`PkiIssued`).
- New messages in the `// ----- F-pki -----` section: `PkiKeySpec` (1 type, 2 curve, 3 bits), `PkiCsr` (1 subject,
  2 san, 3 key_spec), `PkiIssued` (1 serial, 2 not_before, 3 not_after, 4 issuer, 5 fingerprint — the allocated ones —
  plus 6 subject, 7 ca), `PkiFileStateRequest`, `PkiFileStateFile`, `PkiFileStateSet`, `PkiFileStateResponse`.
- RPC `PkiFileState(PkiFileStateRequest) returns (PkiFileStateResponse)` under the service's `wave-BC: F-pki` anchor.
- No `ActionRequest`, no `EventKind` (the PKI actions run in the API; expiry is an API bus message).

### PkiFileState semantics (for docs/contracts/proto.md "F-pki: PkiFileState" — a file outside my envelope)

Read-only, never mutates, owner-checked like every feature RPC. Reports the agent's manifest of PKI files under the
swanctl directory of its charon (product `/etc/swanctl`, slot test `/run/vrx-test/w<N>/swan/a/swanctl`): `x509/<name>.pem`
(kind `cert`), `x509ca/<name>.pem` (`ca`), `private/<name>.pem` (`key`, mode 0600), `x509crl/<name>.pem` (`crl`). The
fingerprint is `sha256:<hex>` of the file content for public files and `hmac:<hex>` (HMAC-SHA256 under the agent-local
D-096 key file `vpn-<owner>.key`) for private keys: no key material and no plain hash of it ever crosses the socket.
`present = false` marks a manifest file that is missing on disk (the next reconcile writes it again). `unavailable` names
why there is nothing to report (no charon for this agent, or the materialiser is not wired into this build).
`PkiFileStateSet` is also the value of the singleton scheduler object `pki.files/vrx` (desired vs Retrieve compare kind,
name, ref, fingerprint and mode; size and present stay zero there).

Generated and committed: `apps/agent/gen/vrx/v1/*`, `packages/proto/gen/ts/vrx/v1/dataplane.ts`. Fixture
`packages/proto/test/fixtures/pki-full.json` exercises every new leaf (both contract guards pass on it; the one red case
in `packages/proto/test`, `examples/tunnels-gre-vxlan-ipip.json` l2tpv3 cookies `1 ≠ "1"`, fails on main too and is not
F-pki's).
