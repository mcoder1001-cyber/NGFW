# TASK ENVELOPE — F-pki
id: F-pki   branch: task/F-pki   worktree: /root/ngfw-wt/F-pki   base: main@<BASE>   started: <STARTED>
title: Wave B (day 10-12): CA, CSR, import/export, CRL/OCSP, expiry alerts
prompt: prompts/features/F-pki.md   (template: prompts/FEATURE-TEMPLATE.md; refreshed on task/prep-rest against main + P08 + the P11 envelope)   wbs: D6.4
scope: the certificate store end to end — API-side CA/CSR/sign/import/export/CRL refresh/OCSP check/state + a daily expiry check on the bus (the bulk), an agent-side file materialiser for strongSwan's `x509/`, `x509ca/`, `private/`, `x509crl/` (Retrieve = fingerprints, never key material), the hook that makes the strongSwan renderer use it, a PKI tab, docs. No VPP objects, no ACME, no HSM.
merged deps you can rely on: P08, P11
  - P08: desired/ + subsystems/ + projection.go patterns, `Wiring` persisted stores
  - P11: the strongSwan renderer (RF-2 + P11: `Auth.Certs/CACerts`, `Model.Authorities`, file-name conventions `<certificate>.pem` / `<ca>.pem`), its singleton scheduler descriptor under `Domains["vpn"]` (D-109 d), the charon lifecycle (`apps/agent/internal/charon`), and whatever it merged for PENDING-secret-channel
  - P06/TD-2 (on main): the secrets service (`apps/api/src/secrets/`, D-051 kinds cert/key, AES-GCM with the name as AAD, versions + rollback, admin-only writes), RBAC, audit
  - also on main: W-seed (anchors, the vpn page shell + tabs.ts), TD-3, TD-4
read first: prompts/features/F-pki.md · docs/status/wave-A-hotspots.md (§0 rules, A1 A2 C1–C7 P1 P4 P5 P6 W3 D4) · docs/status/wave-BC-numbers.md (section F-pki) · docs/agent/renderers/strongswan.md (RF-2 + P11 sections) · apps/agent/internal/renderers/{renderer.go,helpers_files.go} (atomic writes, modes) + ALLOWLIST.md · packages/schema/src/domains/vpn.ts (`vpn.pki`) + semantic/vpn.ts · docs/status/tasks/P11.md · docs/decisions/PENDING-secret-channel.md · docs/decisions/LOG.md D-040, D-046, D-051, D-079 (restart-request pattern), D-091, D-097, D-109
slot: <SLOT> → VRX_SLOT=<SLOT> VRX_TEST_PREFIX=w<SLOT> VRX_HTTP_PORT=3000+100·<SLOT> VRX_WEB_PORT=5000+100·<SLOT> VRX_METRICS_PORT=9100+10·<SLOT>+1 VRX_AGENT_SOCKET=/run/vrx-test/w<SLOT>/agent.sock VRX_PG_DATABASE=vrx_w<SLOT> VRX_VALKEY_DB=<SLOT> VRX_VPP_TABLE_BASE=<SLOT>000 VRX_LAB_LOCK=/run/lock/vrx-lab.lock
  - source of truth: `eval "$(tools/lab env <SLOT>)"`
  - materialiser test root: t.TempDir() or /run/vrx-test/w<SLOT>/pki/ — never /etc/swanctl; the local CRL/OCSP test responder listens on 127.0.0.1:(3000+100·<SLOT>+61) only (no outside network)
  - certificate/CA names in tests start with w<SLOT>
  - slots 1–11 only; 12 is CI
daemon-owner: none — `swanctl --list-certs` inside a slot namespace only if the manager hands you strongswan for that step (the stock/vrx charon under /run/vrx-test/w<SLOT>/, D-083); otherwise the acceptance fallback applies: the materialised file tree + modes pasted
obligations:
  - D-040/D-051: the config document carries only `cert/<name>` / `key/<name>` refs; private keys and PKCS#12 passphrases are never returned by GET/export, never logged, never in audit rows, fixtures or status files (grep evidence). Export = certificate/chain PEM only; a private-key export request → 403/404 without material
  - CA private keys never leave the API process (signing happens API-side — the prompt's default; keep it unless the manager answers otherwise)
  - PENDING-secret-channel (D-109 b): the materialiser needs cert/key material in the agent. If P11 did not merge a channel, build the materialiser against the `vpn.Resolver` interface with a slot-local fixture (`VRX_TEST_PSK_FPKI_*`-style literals, generated throwaway keys) and leave only the end-to-end step open in the questions file
  - no shell (00-CONTEXT rule 9): Node 22 `crypto` parses X.509 (`X509Certificate`) but cannot create certificates or CSRs. A pure-JS X.509 library is a new dependency (D4): before adding it write the options in F-pki-questions.md — (a) @peculiar/x509 (MIT, WebCrypto), (b) pkijs (BSD-3), (c) node-forge (BSD-3/GPL-2 dual) — with licence (decision-policy #5) and size; the manager runs `pnpm install` on main. Meanwhile keep building behind your own interface; tests may exec the host's `openssl` as a verifier only (never product code)
  - the agent materialiser: atomic writes, keys 0600 root, certs 0644, removes files no longer referenced (only files it wrote — keep a manifest), Retrieve = SHA-256 fingerprints of what is on disk (certificates are public; for keys store a keyed HMAC, D-096 style, never a plain hash of key material)
  - ACME and HSM blocks are refused with "not supported in this build" (schema fields exist; FAST MODE)
  - the expiry check runs inside the feature's provider (timer; no new scheduler dependency) and publishes on the bus; alarm delivery is F-dashboard-prom-alarms'
files you own exclusively:
  - apps/agent/internal/pki/** (materialiser + its singleton scheduler descriptor), apps/agent/internal/subsystems/pki*.go, apps/agent/internal/desired/pki*.go, apps/agent/internal/agent/rpc_pki*.go (`PkiFileState`)
  - docs/agent/pki.md
  - packages/schema/src/domains/ext/pki*.ts (CSR subject/SAN/key type, issued metadata), packages/schema/src/semantic/pki*.ts (rule ids `vpn.pki-…`)
  - packages/schema/examples/pki-*.json, packages/proto/test/fixtures/pki-*.json
  - apps/api/src/features/pki/** (index.ts exports {controllers, providers}; `PkiController`; POST /actions/pki/{ca,csr,sign,import,crl/refresh}, GET /actions/pki/export/{name}, GET /state/pki; the expiry provider; real fake behaviour in fake.ts), apps/api/test/e2e/pki*
  - apps/web/src/domains/vpn/pki/** (CAs, certificates with expiry chips, CSR wizard, import dialog, export button), apps/web/src/locales/{en,fa}/pki.json
  - docs/user/vpn/pki.md, test/topology/pki/**, docs/status/tasks/F-pki*
shared hotspots (append-only, conflicts resolved by the manager at merge):
  - protocol: docs/status/wave-A-hotspots.md §0. Insert only directly below your own anchor `// wave-BC: F-pki` if the manager seeded one, else at the end of the block. Never reorder or reformat other lines. List every hunk under "Shared hunks" in docs/status/tasks/F-pki.md
  - P11's strongSwan renderer (dep-chained; P11 is merged, F-ra-vpn inherits it after you): ONE named hunk — the renderer's scheduler descriptor depends on your `pki.files/…` object when a connection references a certificate, and asks charon to `load-creds` after the files changed. Default: your own singleton descriptor + that dependency line; write the choice with options (dependency vs direct call) in the questions file. Never change the S2S behaviour; keep every P11/RF-2 test green
  - docs/agent/renderers/strongswan.md: one "PKI files (F-pki)" paragraph at the end
  - A1 apps/agent/internal/subsystems/subsystems.go: append `pki.files` to `Domains["vpn"]` + one Register line calling your subsystems/pki.go
  - A2 apps/agent/internal/agent/projection.go: one call in project(), one in assemble()
  - C1 packages/schema/src/domains/vpn.ts: key lines for the new PkiCa/PkiCertificate leaves (sub-schemas in your ext file) · C2 packages/schema/src/semantic/index.ts: one spread line · C3 packages/schema/src/index.ts: one export line
  - C4 packages/schema/examples/ + packages/proto/test/fixtures/: new files only; vpn-*.json and all-domains.json are read-only
  - C5 packages/proto/vrx/v1/dataplane.proto: rpc `PkiFileState` under the service anchor; new messages (PkiCsr, PkiIssued, PkiKeySpec, PkiFileState*) in a `// ----- F-pki -----` section at the end
  - allocated numbers (docs/status/wave-BC-numbers.md, a merge blocker if reused): **PkiCa 5 `key_spec`, 6 `issued`**, **PkiCertificate 7 `csr`, 8 `issued`**; no ActionRequest (the PKI actions are API-side), no EventKind (expiry is an API bus topic)
  - C6 docs/contracts/proto.md: `### F-pki: PkiFileState` · C7 generated, regenerated and never hand-edited: apps/agent/gen/**, packages/proto/gen/ts/**, packages/api-client/src/generated/**, apps/cli/internal/api/operations_gen.go, docs/user/cli/reference.md (`packages/proto/gen.sh`, `pnpm gen`, `make -C apps/cli gen docs`)
  - P1 apps/api/src/app.module.ts · P4 apps/api/src/agent/agent.client.ts (`pkiFileState`) · P5 apps/api/src/testing/fake-agent.ts (UNIMPLEMENTED stub under the anchor on the contract commit) · P6 apps/api/src/infra/bus.ts: one `pki.expiry` topic
  - W3 apps/web/src/i18n.ts · the vpn page tab registry apps/web/src/domains/vpn/tabs.ts (W-seed shell): one line
  - D4 apps/api/package.json + pnpm-lock.yaml: the X.509 library only after the manager's answer (the manager installs it on main)
  - apps/agent/internal/renderers/ALLOWLIST.md: rows only if you add an exec (none expected — Go crypto/x509 in the agent)
contract: commit `contract(schema): pki csr/issued/key spec` and `contract(proto): pki leaves + PkiFileState` as separate commits on YOUR branch first (the ci.sh contract guard checks the subject), plus docs/status/tasks/F-pki-contract.md; tell the manager in the questions file and keep building. No own branches (the prompt's old `contract/F-pki` wording is void)
files you must not touch:
  - everything else
  - never: /root/ngfw (main), other worktrees, /etc (incl. /etc/swanctl, /etc/vpp), /root/vpp
  - apps/agent/binapi (P04/manager-owned), tools/lab, tools/ci.sh, plan/tasks.yaml, docs/decisions/LOG.md, docs/status/PROGRESS.md, packages/ui-kit/**
  - apps/api/src/{secrets,commit,auth,users}/** (use the secrets service and RBAC guards; a need → questions file), apps/api/migrations/** (no migration: material lives in the secret store, metadata in the document)
  - apps/agent/internal/renderers/strongswan/** beyond the one hook hunk, apps/agent/internal/{charon,secrets}/**, apps/agent/internal/descriptors/{ipsec,ikev2,vpn,wireguard}/** (P11 / F-ikev2-native / DF-5 / F-wireguard)
  - apps/api/src/features/{ipsec,ikev2-native,wireguard,ra-vpn}/**, apps/web/src/domains/vpn/{ipsec,ikev2-native,wireguard,ra-vpn}/**
  - agent core (A5): apps/agent/internal/agent/{agent,service,state,ifstate}.go, apps/agent/cmd/**
host rules:
  - no VPP objects: no V19 preflight or NRestarts check needed unless you start a charon; if you do (the optional `swanctl --list-certs` step): slot namespace only, VICI socket under /run/vrx-test/w<SLOT>/swan/, stop it by PID
  - no outside network: CRL/OCSP URLs in tests point at your local responder on the slot port; never fetch a real CRL
  - with VRX_INTEGRATION=1, one Go package at a time; hold `flock -s` on the lab lock only during a run (D-094)
coordination: P11 (merged) owns the strongSwan renderer and the IPsec tab — you add only the hook hunk · F-ikev2-native (may run in parallel) needs a key file path for `ikev2_set_local_key`: offer your materialiser (open question in the prompt), do not edit its files · F-ra-vpn (after you) references CA/cert names for EAP-TLS/pubkey and inherits your renderer hook · F-dashboard-prom-alarms consumes the `pki.expiry` topic · the management-UI HTTPS certificate is P10/F-hardening-lite's
evidence: Playwright is not installed. Take the screenshots (en + fa/RTL: PKI tab with expiry chips, CSR wizard, import dialog) with the headless Chrome approach from P07a/P07b/P08 (`test/topology/interfaces/shots_test.go`, kept outside the product code), and say so. Paste `openssl verify -CAfile ca.pem cert.pem` (run by the test) and the materialised tree with modes (`stat -c '%a %U %n'`)
time box: 15 h — when exceeded: stop, commit WIP, write docs/status/tasks/F-pki.md with what is left
WIP: commit at least every 45 min; keep docs/status/tasks/F-pki-wip.md current
CI: `TMPDIR=/tmp/g-w<SLOT> tools/ci.sh --base main` — short TMPDIR (unix socket paths ≤ 108 chars); no host-wide CI lock: golangci-lint serializes itself since main fc0fe68 (D-106 rejected serialising whole gates). Ports 3000/8080/9101 and /run/vrx/agent.sock belong to the running product stack (tools/app) — never touch them
finish: `tools/ci.sh --base main` green in the worktree · docs/status/tasks/F-pki.md with pasted real output · everything committed · final message = 10-line summary (branch, last commit, CI result, evidence, open questions, decisions taken with options)
cleanup: stop every process you started (API/agent/vite/test responder/charon), by PID · lab lock released · vrx_w<SLOT> dropped · no key or certificate files left under /run/vrx-test/w<SLOT>/ · dist/ and apps/agent/bin removed
questions: docs/status/tasks/F-pki-questions.md — write and keep going; never wait for a human
never: merge · restart/kill VPP · Docker · pkill · secrets in files · edit files you do not own
