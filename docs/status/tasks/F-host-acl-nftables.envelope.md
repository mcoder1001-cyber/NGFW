# TASK ENVELOPE — F-host-acl-nftables
id: F-host-acl-nftables   branch: task/F-host-acl-nftables   worktree: /root/ngfw-wt/F-host-acl-nftables   base: main@task/F-object-model@31249d3 (SPECULATIVE D-114: F-object-model done, under review)   started: 2026-09-24T19:52
title: Wave A (day 7-9): local-in ACL + nftables host policy renderer
prompt: prompts/features/F-host-acl-nftables.md   (template: prompts/FEATURE-TEMPLATE.md; updated on task/prep-waveA for P08's layout)   wbs: D5.3
scope: the local-in / management-plane ACL (acl.host*, acl.hostAttachments), rendered by a new agent renderer to ONE nftables table `inet vrx`. This renderer is the single owner of the host firewall (D-057). Includes the anti-lockout rule and per-rule counters.
merged deps you can rely on: P08, DF-4, F-object-model
  - P08: desired/ (Sink), subsystems.Register/Domains, projection.go, InterfaceState state-RPC pattern, test/topology/interfaces
  - DF-4: context only (VPP data-plane ACLs); host rules do not use its descriptors
  - F-object-model: internal/objects (Expand/ExpandService/Active, FQDN resolver) — nft sets are built from it
  - renderer framework (merged): internal/renderers/{renderer.go,helpers_*,ALLOWLIST.md}, renderers/frr (+frrtest netns harness), renderers/strongswan/swantest (setns on a locked thread), renderers/chrony
  - also on main: TD-3, TD-2
read first: prompts/features/F-host-acl-nftables.md · docs/status/wave-A-hotspots.md (§0 rules, A1 A2 A4 A5 C5–C7 P1 P4 P5 W1–W3) · apps/agent/internal/renderers/README.md + ALLOWLIST.md · docs/agent/renderers/frr.md, strongswan.md · docs/status/vertical-slice.md · docs/status/tasks/F-object-model.md · prompts/P10-packaging-deb.md item 5 · prompts/features/F-hardening-lite.md · docs/decisions/LOG.md D-057, D-079, D-089, D-094
slot: 9 → VRX_SLOT=9 VRX_TEST_PREFIX=w9 VRX_HTTP_PORT=3000+100·9 VRX_WEB_PORT=5000+100·9 VRX_METRICS_PORT=9100+10·9+1 VRX_AGENT_SOCKET=/run/vrx-test/w9/agent.sock VRX_PG_DATABASE=vrx_w9 VRX_VALKEY_DB=9 VRX_VPP_TABLE_BASE=9000 VRX_LAB_LOCK=/run/lock/vrx-lab.lock
  - source of truth: `eval "$(tools/lab env 9)"`
  - your test netns / veths / nft table carry the prefix: ns-w9-…, table inet vrx_w9
  - test agents run with VRX_GLOBALS_OWNER=0 (D-071)
  - slots 1–11 only; 12 is CI
daemon-owner: none. nftables is a kernel ruleset, not a daemon: `nftables.service` is inactive and disabled on this host and stays that way (never start, enable or restart it)
HOST FIREWALL SAFETY: every worker and the manager reach this host over SSH on ens192.
  - NEVER load, flush or delete anything in the ROOT netns ruleset: no `nft -f`, `nft flush`, `nft delete`, no iptables
  - only `nft -c` (check) and `nft list` may run in the root netns
  - every apply goes to `table inet vrx_w9` inside your own netns: the unit harness, the integration tests AND the slot agent in stack/topology runs
  - paste `nft list tables` of the root netns before and after every host run; the two must be identical
decision you must log: the agent runs no renderer in its Apply path yet (prompt "Inputs").
  - (a) a singleton scheduler descriptor that wraps the renderer, registered under Domains["acl"] — the default; needs no core change
  - (b) a renderer step in service.go — the core is read-only in wave A (A5), so (b) only after a manager answer
  - record options + choice in docs/status/tasks/F-host-acl-nftables.md → Decisions and in the questions file; P11/P12/F-kea/F-unbound will build on it
obligations:
  - the renderer lifecycle from renderers/renderer.go: Render pure + golden; Validate with `nft -c` on a staged copy; Apply atomic, restore on failure; Retrieve = normalised `nft -j`
  - no user input reaches argv or a shell: use the fixed-argv allow-listed runner; hostile-input golden tests (quote/brace/newline)
  - anti-lockout: an error in DryRun with a pointer
  - restart-safety trap: the agent persists only implemented domains, so `objects` must be one (prompt "Inputs"). If F-object-model did not register it, add the one line in A1 and say so
files you own exclusively:
  - apps/agent/internal/renderers/nftables/** and docs/agent/renderers/nftables.md
  - apps/agent/internal/desired/hostacl*.go
  - apps/agent/internal/agent/rpc_host_acl*.go
  - apps/agent/internal/subsystems/host_acl*.go
  - apps/api/src/features/host-acl-nftables/** (index.ts exports {controllers, providers}; `HostAclNftablesController`; real fake behaviour in fake.ts)
  - apps/api/test/e2e/host-acl*
  - apps/web/src/domains/firewall/host-acl-nftables/**
  - apps/web/src/locales/*/host-acl-nftables.json
  - docs/user/firewall/host-acl-nftables.md
  - test/topology/host-acl-nftables/**
  - only for a config gap (contract commit): packages/schema/src/domains/ext/host-acl-nftables.ts, packages/schema/src/semantic/host-acl-nftables*.ts
  - docs/status/tasks/F-host-acl-nftables*
shared hotspots (append-only, conflicts resolved by the manager at merge):
  - protocol: docs/status/wave-A-hotspots.md §0. Insert only directly below your own anchor `// wave-A: F-host-acl-nftables` if the manager seeded one, else at the end of the block. Never reorder or reformat other lines. List every hunk under "Shared hunks" in docs/status/tasks/F-host-acl-nftables.md
  - apps/agent/internal/renderers/ALLOWLIST.md: one row for /usr/sbin/nft (fixed argv: `nft -c -f <staged>`, `nft -f <staged>`, `nft -j list table inet <name>`). `ip` stays test-harness-only
  - A1 apps/agent/internal/subsystems/subsystems.go: registration + Domains["acl"], shared with F-acl
  - A2 apps/agent/internal/agent/projection.go: one call in project(), one in assemble()
  - C5 packages/proto/vrx/v1/dataplane.proto: HostAclState RPC under the service anchor; messages in a `// ----- F-host-acl-nftables -----` section at the end. No DesiredState field numbers are allocated to you: a config gap (e.g. AclConfig 8 `host_settings`) needs a number from the manager first (questions file)
  - C1/C2/C3 (only with a config gap): one key line in domains/acl.ts, one spread line in semantic/index.ts, one export line in src/index.ts
  - C6 docs/contracts/proto.md: `### F-host-acl-nftables: HostAclState`
  - C7 generated, regenerated and never hand-edited: apps/agent/gen/**, packages/proto/gen/ts/**, packages/api-client/src/generated/**, apps/cli/internal/api/operations_gen.go, docs/user/cli/reference.md
  - P1 apps/api/src/app.module.ts · P4 apps/api/src/agent/agent.client.ts · P5 apps/api/src/testing/fake-agent.ts (UNIMPLEMENTED stub under the anchor)
  - W1 apps/web/src/router.tsx · W2 apps/web/src/nav/nav.ts (+ nav.test.ts) · W3 apps/web/src/i18n.ts
contract: one additive `contract(proto): host acl state` commit on YOUR branch first (read-only HostAclState: rendered chains/rules + per-rule packets/bytes) + docs/status/tasks/F-host-acl-nftables-contract.md. Tell the manager in the questions file and keep building. No own branches (P08 pattern)
files you must not touch:
  - everything else
  - never: /root/ngfw (main), other worktrees, /etc (incl. /etc/nftables.conf), /etc/vpp, /root/vpp
  - apps/agent/binapi (P04/manager-owned), tools/lab, tools/ci.sh, plan/tasks.yaml, docs/decisions/LOG.md, docs/status/PROGRESS.md, packages/ui-kit/** (a need → questions file)
  - agent core (A5): apps/agent/internal/agent/{agent,service,ifstate}.go, apps/agent/cmd/**
  - other renderers' directories and renderers/{renderer.go,helpers_*}
  - apps/agent/internal/descriptors/acl/** + apps/agent/internal/desired/acl*.go + apps/web/src/domains/firewall/acl/** (F-acl, parallel), apps/agent/internal/objects/** (F-object-model)
host rules:
  - HOST FIREWALL SAFETY above; delete your netns/veths in t.Cleanup and at the end
  - this task sends no packets through VPP; the slot agent still connects to the shared VPP: check `systemctl show vpp -p NRestarts` before and after stack runs
  - hold `flock -s` on the lab lock only during a run (D-094)
coordination: F-acl runs in parallel on the same `acl` root key; each reports the other's leaves as agent.unsupported-field until it lands · F-hardening-lite (later) consumes this renderer; P10's static base policy stays a separate table · reuse the rule grid only through packages/ui-kit (ServerDataGrid), never by importing from F-acl's domain folder
evidence: Playwright is not installed. Take the screenshots (en + fa/RTL) with the headless Chrome approach from P07a/P07b/P08 (`test/topology/interfaces/shots_test.go`, kept outside the product code), and say so.
time box: 15 h — when exceeded: stop, commit WIP, write docs/status/tasks/F-host-acl-nftables.md with what is left
WIP: commit at least every 45 min; keep docs/status/tasks/F-host-acl-nftables-wip.md current
CI: `TMPDIR=/tmp/g-w9 tools/ci.sh --base main` — short TMPDIR (unix socket paths ≤ 108 chars); no host-wide CI lock: golangci-lint serializes itself since main fc0fe68 (D-106 rejected serialising whole gates). Ports 3000/8080/9101 and /run/vrx/agent.sock belong to the running product stack (tools/app) — never touch them
finish: `tools/ci.sh --base main` green in the worktree · docs/status/tasks/F-host-acl-nftables.md with pasted real output · everything committed · final message = 10-line summary (branch, last commit, CI result, evidence, open questions, decisions taken with options)
cleanup: stop every process you started (API/agent/vite), by PID · lab lock released · vrx_w9 dropped · test netns/veths deleted · root-netns `nft list tables` unchanged (pasted) · dist/ and apps/agent/bin removed
questions: docs/status/tasks/F-host-acl-nftables-questions.md — write and keep going; never wait for a human
never: merge · restart/kill VPP · Docker · pkill · secrets in files · edit files you do not own

MANAGER ADDENDA: D-128 — never run `show trace` / `trace add` / `clear trace` on the shared VPP. The ROOT-netns nftables rule above is absolute: the management NIC (ens192, 172.30.126.195) must stay reachable. GIT RULE: git only inside your own worktree, NEVER in /root/ngfw. CI: if your branch copy of tools/ci.sh fails in the contract guard with SIGPIPE, run main's copy (`git show main:tools/ci.sh > /tmp/g-w9/ci.sh`, D-127).
