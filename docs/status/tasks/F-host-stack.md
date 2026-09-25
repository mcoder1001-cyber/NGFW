# F-host-stack: VPP host stack (scoped down, D-085)

Branch `task/F-host-stack` (base be53867). The feature is built end to end on the fake VPP client and the fake agent.
Nothing ran against a real VPP: this cloud container has no VPP host (no `/run/vpp`, no `vppctl`, no systemd), no
PostgreSQL and no Chrome.

## What was built
- **Contract** (separate `contract(schema)` / `contract(proto)` commits, `F-host-stack-contract.md`):
  `services.hostStack?{enabled, namespaces, sessionRules, tcpSourceAddresses?, httpStatic?}` and `ServicesConfig.host_stack = 10`.
  - New messages: `HostStack*` messages and the `HostStackState` RPC.
  - Added beyond the prompt sketch: `localPort`, `remotePort` and `redirectAppIndex`, which `session_rule_add_del` needs.
- **Schema** (`packages/schema/src/domains/ext/host-stack.ts`):
  - Namespace secret: a `key/<name>` ref only.
  - Rule prefixes: canonical, one family per rule. Tags: unique, no `..`.
  - `wwwRootPath`: under `/var/lib/vrx/www/`, no `..`, no control characters.
  - Rules or namespaces need `enabled: true`.
- **Semantics** (`semantic/host-stack.ts`):
  - VRF, interface and rule-namespace references must exist, and a namespace's interface must be in the namespace's VRF.
  - `services.host-stack-rt-engine`: session rules together with `services.autoSdl.enabled` gives an error at the pointer
    `/services/hostStack/sessionRules`. The rule reads `autoSdl` defensively because F-rpf-adl-pbr has not landed yet.
- **Agent** (`descriptors/hoststack`, built on dfkit): the five descriptors below, the `desired/host_stack.go`
  projection and assembler, and `agent/rpc_host_stack.go` (`HostStackState`).
  - `hoststack.session` is write-only and never disables or re-engines the layer:
    - The globals owner registers it through `RegisterGlobals`. It enables rule-table only when the probe fails.
    - Other agents only require it, with the probe as the getter.
  - `hoststack.namespace` is write-only with an idempotent re-add. Its `appns_index` goes into a D-076 BootStore record
    bound to the boot identity.
  - `hoststack.session-rule` Retrieves from `session_rules_v2_dump`. VPP carries the tag as `<owner>:<tag>`.
  - `hoststack.tcp-src` uses a D-076 applied-once record and has no delete.
  - `hoststack.http-static` is registered only for the globals owner and only with `VRX_HOSTSTACK_HTTP_STATIC=1`.
  - DryRun refuses a namespace `secretRef` (`services.host-stack-secret-channel`).
  - DryRun refuses `httpStatic.enabled` without the opt-in.
- **API**: `GET /api/v1/state/host-stack` (`features/host-stack`). Configuration uses the generic pointer routes. The fake
  HostStackState lives in `features/host-stack/fake.ts`.
- **UI**: the Services → "Host stack" tab has a T3 banner, a live-state card, the enable switch, a namespaces table and a
  session-rules grid, in en and fa. `services` joins BUILT_DOMAINS.
- **Docs**: `docs/user/services/host-stack.md` (configurable leaves and the have-not list) and
  `docs/agent/descriptors/hoststack.md`.

## Verification (real output)
```
=== RUN   TestNamespaceAndRulesLifecycle        --- PASS   (dup add, Retrieve==desired, agent-restart sim, update, rollback → empty)
=== RUN   TestRuleValidation                    --- PASS
=== RUN   TestSessionGlobals                    --- PASS   (non-owner never sends session_enable_disable_v2; owner never re-engines/disables)
=== RUN   TestTCPSrcAppliedOnce                 --- PASS   (1 add per boot, re-add after VPP restart)
=== RUN   TestHTTPStaticOptIn                   --- PASS   (not registered by default; www_root validation)
=== RUN   TestStateSessionOff                   --- PASS
=== RUN   TestHostStackOnHost
    integration_test.go:23: integration test: set VRX_INTEGRATION=1 (and run under the shared lab lock)
--- SKIP: TestHostStackOnHost
ok  	ngfw/agent/internal/descriptors/hoststack	0.017s
--- PASS: TestHostStackProjection   ok ngfw/agent/internal/desired   (secretRef → services.host-stack-secret-channel; http_static opt-in)
drift guard: 897 scalar leaves and 198 messages compared, 4 accepted difference(s), 0 finding(s)
packages/schema  src/semantic/host-stack.test.ts  Tests 11 passed (inline secret, `..`/control chars/outside root in wwwRootPath, canonical/family, rt-engine)
apps/api         src/features/host-stack          Tests 4 passed  (state mapping; inline secret → 400 pointer /services/hostStack/namespaces/w1-app/secretRef; `..` → 400 pointer …/httpStatic/wwwRootPath)
apps/web         src/domains/services             Tests 2 passed  (banner, live state, grids, delete → PATCH; fa)
```
Acceptance, honestly:
- `vppctl show session rules` / `show app ns`: **not run**, because there is no VPP host here. The host test
  `TestHostStackOnHost` exists: prefixed tags, the slot's ports 3<SLOT>90+, and it skips unless the session layer is already
  on. The same checks ran on the fake client (Retrieve == desired, rollback, restart). No `NRestarts` reading was possible
  (no systemd).
- Agent-restart simulation: a new descriptor set over the same boot store gives an empty plan (fake). The 30 s
  wall-clock figure was not measured.
- Rollback removes rules and namespaces (fake Retrieve is empty). The session layer is untouched: Delete is a no-op (tested).
- Inline secret / `..` → 400 with a pointer: API unit test (in-memory datastore). `test/e2e/host-stack.e2e.test.ts` is
  written but needs PostgreSQL, which is not available here.
- UI screenshot against the real endpoint: **not produced**, because there is no Chrome, PostgreSQL or agent stack. The
  jsdom test covers the screen.
- `tools/ci.sh`: two environment problems, both worked around outside the repo; neither is committed.
  - Turbo cannot exec the shebang-less pnpm 12.5.1 launcher (ENOEXEC). Fix: an ELF shim at `node_modules/.bin/pnpm`
    (gitignored).
  - Host go is 1.24.7 and golangci-lint 2.5.0. Fix: go1.26.8 toolchain and golangci-lint 2.13.2 from the scratchpad.
  With those fixes, `--base be53867` passes these gates:
  - contract guard
  - gen and the clean generated output (after `buf` 1.73 — an older buf rewrites the timestamp.ts WKT)
  - forbidden patterns and gitleaks
  - turbo lint/typecheck/test/build: 30/30
  The run then **FAILS at `apps/agent make lint`** on one pre-existing finding in a file I may not touch:
  `internal/agent/service.go:549 revertRetryMin (revive time-naming)`. `service.go` is unchanged since be53867.
  - Run by hand after that: `make -C apps/agent test build` ok, `make -C apps/cli lint test build` ok, and `test/` modules
    vet+test ok.
  - `--base main` also reports a gitleaks hit in base commit c2a8ab7 (`packages/schema/src/domains/vpn.test.ts:252`).
    That commit is not mine.

## Obligations
- D-104/D-077 Q5: built on dfkit (Encode/Decode, Globals, BootStore, AppliedThisBoot, ResolveInterface, Drain,
  CheckBoot); nothing forked.
- D-063/D-076/D-080: session, namespace, tcp-src and http-static are write-only. Retrieve returns `ErrRetrieveUnsupported`
  and never echoes desired state. Boot records are keyed by the boot identity. Rules come from the dump, filtered to the
  owner prefix.
- D-071: session and http_static are globals.
  - Only `env.GlobalsOwner` calls `RegisterGlobals`.
  - Slot agents register only the require form.
- D-064/D-077 Q3: http_static is opt-in and was never enabled here.
- D-049/D-051: covered by the schema, the agent's ValidID and ValidWWWRoot, and the `key/` ref.
- PENDING-secret-channel: DryRun refuses a namespace `secretRef`.

## Shared hunks
Each hunk below is marked "unanchored" in the code:
- `subsystems.go`: the `"services": {…}` entry is under the F-host-stack anchor. The `hoststack.Register` and
  `RegisterGlobals` lines were appended after the last anchor, because the register block has no F-host-stack anchor.
- `projection.go`: one call in project() and one in assemble(), each appended after the last anchor, because neither
  function has an F-host-stack anchor.
- `packages/schema/src/domains/services.ts`: the `hostStack` key line and the import. Unanchored.
- `packages/schema/src/index.ts`: the export line. Unanchored.
- `semantic/index.ts`: the import and the spread. Unanchored.
- `packages/proto/.../dataplane.proto`: the RPC and field 10 under their anchors; the messages in the `----- F-host-stack -----`
  section.
- `docs/contracts/proto.md`: the section. Unanchored.
- `agent.client.ts` and `fake-agent.ts`: under their anchors, plus one fake import line.
- `app.module.ts`: under its anchors.
- `i18n.ts`: 4 anchors.
- `services/tabs.ts`: under its anchor, plus the `lazy` import.
- `nav.ts` and `nav.test.ts`: `'services'` appended. Unanchored.
- **Outside the anchors**: `apps/agent/internal/agent/service_test.go`, two assertions of the implemented-domain list
  (+ `services`). Adding the `services` domain requires this change.

## Out of scope (not built)
VCL/LD_PRELOAD, TLS engine tuning, QUIC, HTTP/3, SRTP, HSI, TCP/UDP buffer and congestion-control settings (startup.conf,
F-startup-gen), SDL knobs, the prom exporter and LB.

## Open questions
See `F-host-stack-questions.md`.
