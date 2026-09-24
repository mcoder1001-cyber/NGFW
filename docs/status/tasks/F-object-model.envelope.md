# TASK ENVELOPE — F-object-model
id: F-object-model   branch: task/F-object-model   worktree: /root/ngfw-wt/F-object-model   base: main@<BASE>   started: <STARTED>
title: Wave A (day 7-9): addresses, groups, FQDN (agent-resolved), services, schedules, zones, tags
prompt: prompts/features/F-object-model.md   (template: prompts/FEATURE-TEMPLATE.md; checked against main + P08 in wave-A prep)   wbs: D5.1
scope: agent library `internal/objects` (Expand/ExpandService, range→CIDR, v4/v6 split, schedules, 10 000-entry cap), the agent-side FQDN resolver with persisted last-good state, `objects` as an implemented agent-state domain (no VPP objects), a read-only `FqdnObjectState` RPC, API FQDN state + where-used, the Objects page + an exported ObjectPicker widget, docs. The config schema, its semantic rules and `ObjectsConfig` already exist — use, do not rebuild.
merged deps you can rely on: P08
  - P08: subsystems.Register/Domains + Wiring stores, desired/ (Sink, pointers), projection.go, InterfaceState state-RPC pattern, SchemaForm screen pattern (`widgets` prop for custom widgets), test/topology/interfaces
  - through P08: P02b `packages/schema/src/domains/objects.ts` + `semantic/objects.ts` (rules `objects.names-disjoint`, `.tags-exist`, `.address-range-valid`, `.address-group-members`, `.service-group-members`, `.service-valid`, `.schedule-valid`, `.zone-interfaces`) and `semantic/acl.ts` `acl.rule-references`; `ObjectsConfig` proto; D-078 `vrx.model.acl.v1`; DF-4 `descriptors/acl/spec.go` (the shape F-acl will expand into); RF-3 unbound renderer (reference only)
  - also on main: TD-3 (V19 sanitizer), TD-2 (API auth/users follow-ups)
downstream: F-acl and F-host-acl-nftables depend on this task (board) — keep the exported Go API of `internal/objects` small, documented (docs/agent/objects.md) and stable, and export the web `ObjectPicker` from your directory; they wire the re-projection trigger and the picker on their side
read first: prompts/features/F-object-model.md · docs/status/wave-A-hotspots.md (§0 rules, §2 numbers, A1 A2 C1–C7 P1 P4 P5 P6 W1–W3) · docs/contracts/schema-nat-objects-acl.md · docs/status/vertical-slice.md · docs/decisions/LOG.md D-003, D-062, D-063, D-073, D-078, D-094, D-104
slot: <SLOT> → VRX_SLOT=<SLOT> VRX_TEST_PREFIX=w<SLOT> VRX_HTTP_PORT=3000+100·<SLOT> VRX_WEB_PORT=5000+100·<SLOT> VRX_METRICS_PORT=9100+10·<SLOT>+1 VRX_AGENT_SOCKET=/run/vrx-test/w<SLOT>/agent.sock VRX_PG_DATABASE=vrx_w<SLOT> VRX_VALKEY_DB=<SLOT> VRX_VPP_TABLE_BASE=<SLOT>000 VRX_LAB_LOCK=/run/lock/vrx-lab.lock
  - source of truth: `eval "$(tools/lab env <SLOT>)"`
  - object addresses in fixtures: documentation ranges or 10.<SLOT>.0.0/16; FQDN names under a test-only zone served by your in-process responder
  - test agents run with VRX_GLOBALS_OWNER=0 (D-071)
  - slots 1–11 only; 12 is CI
daemon-owner: none (the resolver is Go code in the agent; it never configures, starts or reloads unbound, systemd-resolved or any other daemon)
obligations:
  - D-104: use, do not rebuild — P02b's schema and rules, `ObjectsConfig`; add a schema rule only where an acceptance item proves one is missing (e.g. the ACL-reference check should already come from `acl.rule-references`)
  - D-062: one namespace across addresses/services/groups; groups may be empty, a rule may not reference an empty group; never read `nat.enabled` raw
  - agent-state domain: `objects` must be implemented (`Domains["objects"]`) and Retrieve must return the applied objects document, else `/state/drift` reports every object missing. Default (prompt): an agent-local `objects.*` descriptor family inside `internal/objects` (Create/Delete update a store persisted in the agent state dir, Retrieve reads it; the resolver and `Expand` read the same store). Agent core (`service.go`) is read-only (A5) — the alternative hook there needs a questions-file answer first
  - D-063: the store is real agent state, not an echo of the request — Retrieve after an agent restart comes from the persisted store
  - FQDN: Go's `net.Resolver` returns no TTL — either query the /etc/resolv.conf servers with `golang.org/x/net/dns/dnsmessage` (x/net is already required in apps/agent/go.mod as indirect, so no new module; if the gate wants `// indirect` removed that is a D4 change → questions file, the manager tidies on main) or a fixed refresh clamped to [30 s, 1 h]; pick one, document it. Last-good answers survive failures; an unresolved FQDN expands to nothing with a warning, never an error
  - the resolver goroutine starts and stops from your owned `subsystems/object_model.go` (A1), never from agent.go; after an agent restart it reloads the persisted results and does not re-query everything at once (acceptance)
  - 00-CONTEXT rule 9: no exec, no shell — resolution only through Go
files you own exclusively:
  - apps/agent/internal/objects/** (library, resolver, the `objects.*` descriptor family and its store), docs/agent/objects.md
  - apps/agent/internal/desired/object_model*.go (builder + assembler), apps/agent/internal/subsystems/object_model*.go (registration, resolver lifecycle), apps/agent/internal/agent/rpc_object_model*.go (`FqdnObjectState`)
  - apps/api/src/features/object-model/** (index.ts exports {controllers, providers}; `ObjectModelController`; `GET /state/objects/fqdn`, `GET /state/objects/usage?name=`; real fake behaviour in fake.ts), apps/api/test/e2e/object-model*
  - apps/web/src/domains/firewall/object-model/** (incl. the exported `ObjectPicker` custom widget), apps/web/src/locales/{en,fa}/object-model.json
  - packages/schema/src/domains/ext/object-model*.ts and packages/schema/src/semantic/object-model*.ts (only if a field/rule is needed; rule ids `objects.object-model-…`), packages/schema/examples/object-model-*.json, packages/proto/test/fixtures/object-model-*.json
  - docs/user/firewall/object-model.md, test/topology/object-model/**, docs/status/tasks/F-object-model*
shared hotspots (append-only, conflicts resolved by the manager at merge):
  - protocol: docs/status/wave-A-hotspots.md §0. Insert only directly below your own anchor `// wave-A: F-object-model`. Never reorder or reformat other lines. List every hunk under "Shared hunks" in docs/status/tasks/F-object-model.md
  - A1 apps/agent/internal/subsystems/subsystems.go: `Domains["objects"]` (your descriptor names) + one call into your registration/lifecycle helper
  - A2 apps/agent/internal/agent/projection.go: one call in project(), one in assemble()
  - C1/C2/C3 packages/schema: only if you add a config field (e.g. `fqdn.refreshSec`, `fqdn.maxAddresses`) or a missing rule — one key line / one spread line / one export line · C4 new fixture files only
  - C5 packages/proto/vrx/v1/dataplane.proto: `FqdnObjectState` under the service anchor; new messages in a `// ----- F-object-model -----` section at the end
  - allocated numbers (wave-A-hotspots §2, a merge blocker if reused): **EventKind 11 `EVENT_KIND_FQDN_CHANGED`** (only if the change is published to the API) · ObjectsConfig 8–9 only if you add a config field; nothing else
  - C6 docs/contracts/proto.md: `### F-object-model: FqdnObjectState` · C7 generated, regenerated and never hand-edited: apps/agent/gen/**, packages/proto/gen/ts/**, packages/api-client/src/generated/**, apps/cli/internal/api/operations_gen.go, docs/user/cli/reference.md (`packages/proto/gen.sh`, `pnpm gen`, `make -C apps/cli gen docs`)
  - P1 apps/api/src/app.module.ts · P4 apps/api/src/agent/agent.client.ts (`fqdnObjectState`) · P5 apps/api/src/testing/fake-agent.ts (UNIMPLEMENTED stub under the anchor on the contract commit) · P6 apps/api/src/infra/bus.ts `TOPICS` + apps/api/src/telemetry/relay.service.ts: topic `objects.events` + its EventKind case (only with EventKind 11)
  - A5 (only with EventKind 11): publishing needs the `subsystems.Env` event sink the manager seeds in the anchor commit (the agent bus is unexported); if it is absent, keep FQDN changes as state only (FqdnObjectState) and write the question — never edit agent.go
  - W1 apps/web/src/router.tsx · W2 apps/web/src/nav/nav.ts + nav.test.ts (`objects` into BUILT_DOMAINS) · W3 apps/web/src/i18n.ts
contract: commit `contract(proto): FqdnObjectState` (and `contract(schema): …` only if you add a field) as separate commits on YOUR branch first (the ci.sh contract guard checks the subject), plus docs/status/tasks/F-object-model-contract.md; tell the manager in the questions file and keep building. No own branches (P08 pattern)
files you must not touch:
  - everything else
  - never: /root/ngfw (main), other worktrees, /etc (incl. /etc/resolv.conf, /etc/unbound, /etc/systemd, /etc/vpp), /root/vpp
  - apps/agent/binapi + tools/binapi-gen.sh (P04/manager-owned), tools/lab, tools/ci.sh, plan/tasks.yaml, docs/decisions/LOG.md, docs/status/PROGRESS.md, packages/ui-kit/** (custom widgets go through SchemaForm's `widgets` prop; a need → questions file), apps/agent/go.mod/go.sum (D4 → questions file)
  - agent core (A5): apps/agent/internal/agent/{agent,service,ifstate}.go, apps/agent/cmd/**; apps/agent/internal/desired/interfaces.go (A3)
  - apps/agent/internal/descriptors/** (acl belongs to F-acl — read spec.go only), apps/agent/internal/renderers/** (unbound = F-unbound-chrony-syslog, nftables = F-host-acl-nftables)
  - packages/schema/src/{domains,semantic}/{objects,acl}.ts (P02b's; C1/C2 own-file rule); apps/web/src/domains/firewall/{acl,host-acl-nftables,nat44-*}/**; apps/api/src/actions/** (F-vrf-static-ecmp), apps/api/src/state/** (P08), apps/api/src/{auth,users}/** (TD-4); apps/web/src/locales/*/{nav,common}.json (W4)
host rules:
  - no VPP objects, no plugins, no data NICs, no daemons, no network: FQDN tests use an in-process DNS responder on 127.0.0.1:0 (ephemeral port), never port 53
  - the topology test starts a real agent + API + DB on your slot (the agent connects to the shared VPP but creates nothing): hold `flock -s` on the lab lock only during that run (D-094) and stop every process by PID
  - unit tests carry most of the evidence: nested groups, range→CIDR, v4/v6 split, cap exceeded, schedule edges (DST change, `once` window) with a fake clock, resolver refresh/last-good with a fake clock
  - run `systemctl show vpp -p NRestarts` before and after the topology run (it should not move; if it does, stop and write the questions file)
coordination: F-acl and F-host-acl-nftables start after you merge (board deps) and import `internal/objects` + `ObjectPicker` — tell them in docs/agent/objects.md what is stable · F-nat44-ed-sessions does not consume objects in this wave · F-unbound-chrony-syslog (later) owns the box's DNS resolver; you only read /etc/resolv.conf
evidence: Playwright is not installed. Take the screenshots (en + fa/RTL: each tab, where-used drawer, FQDN resolution column) with the headless Chrome approach from P07a/P07b/P08 (`test/topology/interfaces/shots_test.go`, kept outside the product code), and say so.
time box: 15 h — when exceeded: stop, commit WIP, write docs/status/tasks/F-object-model.md with what is left
WIP: commit at least every 45 min; keep docs/status/tasks/F-object-model-wip.md current
CI: `TMPDIR=/tmp/g-w<SLOT> tools/ci.sh --base main` — short TMPDIR (unix socket paths ≤ 108 chars); no host-wide CI lock: golangci-lint serializes itself since main fc0fe68 (D-106 rejected serialising whole gates). Ports 3000/8080/9101 and /run/vrx/agent.sock belong to the running product stack (tools/app) — never touch them
finish: `tools/ci.sh --base main` green in the worktree · docs/status/tasks/F-object-model.md with pasted real output · everything committed · final message = 10-line summary (branch, last commit, CI result, evidence, open questions, decisions taken with options)
cleanup: stop every process you started (API/agent/vite/DNS responder), by PID · lab lock released · vrx_w<SLOT> dropped · your slot's agent state dir removed · dist/ and apps/agent/bin removed
questions: docs/status/tasks/F-object-model-questions.md — write and keep going; never wait for a human
never: merge · restart/kill VPP · Docker · pkill · secrets in files · edit files you do not own
