# Certificate manager

PKI actions store certificate and key material in the encrypted secret store. Configuration contains references; exporting a certificate never exports its private key. The API supports ECDSA P-256/P-384 and RSA 2048/3072/4096. ACME issuance and PKCS#11/HSM are not supported in this build.

## Internal CA and an IKEv2 server certificate

Authenticate as an administrator and acquire the candidate configuration lock before staging changes. All paths below are under `/api/v1`.

1. POST `actions/pki/ca` with `{"name":"vpn-ca","subject":"CN=VPN CA","days":3650}`. The response contains the public CA certificate and the references `cert/vpn-ca` and `key/vpn-ca`.
2. POST `actions/pki/csr` with `{"name":"vpn-server","subject":"CN=vpn.example.test","san":["vpn.example.test"]}`. Save the returned `csrPem`; the private key remains in the secret store.
3. POST `actions/pki/sign` with `ca: "vpn-ca"`, `name: "vpn-server"`, that `csrPem`, and `days: 365`. Verify the response says `staged: true`. A false value means the secrets were stored but the candidate was not changed; the response explains why.
4. Validate, inspect the candidate diff, then commit through the standard configuration workflow. Native IKEv2 certificate authentication is a separate feature; staging files does not establish a certificate-authenticated tunnel.
5. GET `state/pki` to inspect subject, issuer, validity, expiry alerts and agent file fingerprints. Public CA export is GET `actions/pki/export/vpn-ca?kind=ca`; certificate export is GET `actions/pki/export/vpn-server`.

The CA signing key stays on the API host. Operational private keys accompanying configured end-entity certificates are delivered for agent materialisation in `private/<name>.pem`, with mode 0600. CA certificates go in `x509ca`, leaf certificates in `x509`, and verified CRLs in `x509crl` under `<agent StateDir>/pki-<owner>`.

## Importing a third-party chain

POST `actions/pki/import` with `format: "pem"`, `as: "certificate"`, a `name`, and `certificatePem` containing leaf first, followed by its issuing certificates. Supply either the write-only `privateKeyPem` or an existing `privateKeyRef`, never both. An optional `ca` must name a configured CA that issued the leaf. The key must match; the certificate must be currently valid and chain signatures must verify with CA issuers.

For PKCS#12 use `format: "pkcs12"`, base64 `pkcs12`, and a write-only `passphrase`. The imported leaf is selected by its matching key. Import a trust CA using `as: "ca"`; its certificate must carry CA:TRUE. Use `replace: true` explicitly to replace existing secret references. Existing secret versions are retained.

Configure a CA's `crl.url` and `refreshIntervalSec` to enable periodic CRL retrieval. POST `actions/pki/crl/refresh` optionally selects a CA by name. Configure `ocspUrl`, then POST `actions/pki/ocsp/check` optionally selects a certificate. State shows refresh errors and revocation results. Expiry checks publish alarm events; delivery channels are managed separately.

## CLI equivalent and current integration limits

No dedicated PKI CLI command is implemented in this feature. The authenticated REST actions above are the current equivalent; do not pass private keys or import passphrases in command-line arguments or shell history.

The PKI tab provides CA creation, CSR generation, signing, PEM/PKCS12 import, public PEM export, CRL refresh and OCSP checks in English and Persian. Actions stage supported configuration changes in the candidate; a separate standard commit applies them. Private inputs are cleared on success, failure and closing the dialog.

Agent registration, projection and public state RPC are wired. Only configured public CAs, operational end-entity certificates and their matching private keys, and available CRLs cross the existing authorized socket channel. CSR-only keys and CA signing keys remain API-side; CA signing certificates cannot be used as operational leaves. The agent uses its sealed cache before startup reconciliation. Each delivered item is bounded to 64 KiB (including CRLs); oversized material is rejected explicitly.

The materializer uses an agent-owned directory, creates private directories 0700 and keys 0600, refuses symlink targets, records only fingerprints in scheduler values, and removes only manifest-owned files. Restart recovery, candidate-only cache rotation rollback and mode protection are tested in temporary directories. No product strongSwan process, system `/etc/swanctl` mutation or VPP restart is performed. Native certificate consumers and deployed browser/daemon acceptance remain separate acceptance work.
