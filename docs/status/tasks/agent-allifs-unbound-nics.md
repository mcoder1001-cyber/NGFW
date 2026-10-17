# agent-allifs-unbound-nics — status

PR: https://github.com/mcoder1001-cyber/NGFW/pull/241 · branch `claude/agent-allifs-unbound-nics` · base `origin/main` `99a774734` · product commit `9a63c09ce` · status docs `8d34740d4` and this file. Agent only; no schema/proto/API change; no VPP, lab slot or host service touched.

## What was built

- `apps/agent/internal/desired/acl.go`: `desired.ACL` takes the Linux netdev lookup (`NetdevKind`, the one `desired.Interfaces` gets); the expander carries it.
- `apps/agent/internal/desired/global_blocking.go`: the implicit all-interfaces target set of the block lists (`allInterfaces` lists and the auto-block overlay, which always selects all interfaces) is now `sortedKeys(kept)` of `SplitUnboundPhysical(ds.GetInterfaces(), lookup)` — the rows the interfaces projection keeps — computed lazily once per projection, only when a list asks for it. Explicit interface lists are unchanged.
- `apps/agent/internal/agent/projection.go`: passes the lookup; comment on the shared split. Test callers of `ACL` pass `nil`.
- Why: an unbound seeded NIC (still a Linux kernel interface) or a NIC released to the host has no `interface/<row>` alias (never projected), so `acl.interface-binding/<row>` failed the plan with a missing mandatory dependency; every interfaces/security commit was refused and `AutoBlockSet` returned `Unavailable` while such a list or live auto-block entries existed (PR #238 review R4 finding #1; `docs/tech-debt.md` 2026-10-17 row on the #238 branch; pre-existing on `main`).

## How verified (pasted output)

Red/green on the product commit, run from `apps/agent` (the fix hunk of `global_blocking.go` reversed with `git apply -R`, tests run, hunk restored):

```
$ env -u NGFW_INTEGRATION go test -count=1 -run 'TestGlobalBlockingAllInterfacesSkipsUnboundAndReleasedNICs|TestProjectAutoBlockOverlaySkipsUnboundAndReleasedNICs|TestAutoBlockWithUnboundAndReleasedNICsOnFake' ./internal/desired/ ./internal/agent/      # WITHOUT the fix
--- FAIL: TestGlobalBlockingAllInterfacesSkipsUnboundAndReleasedNICs (0.01s)
    global_blocking_test.go:262: a released NIC is not an all-interfaces target
FAIL	ngfw/agent/internal/desired	0.126s
--- FAIL: TestProjectAutoBlockOverlaySkipsUnboundAndReleasedNICs (0.01s)
    physical_nics_acl_test.go:54: no binding for ens192: its interface alias is never projected
--- FAIL: TestAutoBlockWithUnboundAndReleasedNICsOnFake (0.04s)
    physical_nics_acl_test.go:86: auto-block set with an unbound and a released NIC: <nil> rpc error: code = Unavailable desc = auto-block enforcement: validation failed: acl.interface-binding/ens161: mandatory dependency interface/ens161 is neither desired nor present
FAIL	ngfw/agent/internal/agent	0.203s

$ env -u NGFW_INTEGRATION go test -count=1 -run 'TestGlobalBlocking|TestProjectAutoBlockOverlaySkipsUnboundAndReleasedNICs|TestAutoBlockWithUnboundAndReleasedNICsOnFake|TestProjectPhysicalNic' ./internal/desired/ ./internal/agent/      # WITH the fix
ok  	ngfw/agent/internal/desired	5.057s
ok  	ngfw/agent/internal/agent	0.299s
```

Agent gate on `9a63c09ce` (`env -u NGFW_INTEGRATION make -C apps/agent lint test`, log `agent-gate.log` in the session scratchpad):

```
NGFW_TIMING BEGIN agent.vet 2026-10-17T11:20:11.518605850Z
go vet ./...
NGFW_TIMING END agent.vet 2026-10-17T11:20:21.471552061Z
NGFW_TIMING BEGIN agent.lint-dispatch 2026-10-17T11:20:21.493224189Z      (golangci-lint run --allow-serial-runners ./... — no findings)
NGFW_TIMING END agent.lint-dispatch 2026-10-17T11:20:39.628441029Z
NGFW_TIMING BEGIN agent.race-test 2026-10-17T11:20:39.651486494Z
go test -race -count=1 ./...
ok  	ngfw/agent/internal/agent	58.744s
ok  	ngfw/agent/internal/desired	30.261s
…  143 packages ok, 0 FAIL
NGFW_TIMING END agent.race-test 2026-10-17T11:22:47.232680868Z
AGENT_GATE_EXIT=0
```

Complete `tools/ci.sh quick` (unchanged, from the worktree root, `env -u NGFW_INTEGRATION`) on `8d34740d4` — the product tree is identical to the head `552b9bcce` (docs/status commit only); started 2026-10-17 11:23:50Z, log `ci-quick.log` in the session scratchpad and `/root/ngfw-wt/logs/ci/funny-meninsky-c9ef19-20261017-112350-1815599`:

```
branch    claude/agent-allifs-unbound-nics @ 8d34740d4
Tasks:    35 successful, 35 total Cached:    28 cached, 35 total Time:    54.892s   (lint · typecheck · unit tests · build, turbo)
== summary (quick) ==
  tools (golangci-lint, gitleaks)                    0m02s
  install (pnpm --frozen-lockfile --prefer-offline)   0m07s
  generate + generated-output gate                   0m59s
  forbidden patterns (+ gitleaks)                    0m16s
  packet-trace ban on the shared VPP (D-128)         0m00s
  ip classify reset after every shell interface create (D-185)   0m02s
  slot resource scheme (1..32, no collisions)        0m04s
  lint · typecheck · unit tests · build (turbo)   0m56s
  apps/agent: make lint test build                   1m32s
  apps/cli: make lint test build                     0m16s
  test/ Go modules, unit mode (test/integration/reachability test/integration/smoke test/topology/ab-upgrade test/topology/acl test/topology/backup-restore test/topology/bonding test/topology/bridge-l2 test/topology/det44 test/topology/ha-state-sync test/topology/hardening-lite test/topology/host-acl-nftables test/topology/images test/topology/interfaces test/topology/ipfix-sflow test/topology/kea-dhcp-relay test/topology/loopback-bvi-gso-lldp-span test/topology/nat44-ed-sessions test/topology/nat44-ei-64-66-nptv6 test/topology/neighbors-ra test/topology/object-model test/topology/qos-flat test/topology/system-identity test/topology/traffic-a/globals test/topology/traffic-c/globals test/topology/unbound-chrony-syslog test/topology/vlan-qinq test/topology/vrf-static-ecmp)   1m30s
  deploy/vpp: shellcheck + apply-startup fake-host harness   4m41s
  warnings:
    - control-plane lines exempted with 'ALLOW:' — reviewer, check each justification:
      apps/api/src/features/mgmt-tls/mgmt-tls.test.ts:5:import { execFileSync } from 'node:child_process'; // ALLOW: test-only openssl cert
      apps/api/src/features/mgmt-tls/mgmt-tls.test.ts:19:    execFileSync('openssl', ['version'], { stdio: 'ignore' }); // ALLOW: test-only openssl cert
      apps/api/src/features/mgmt-tls/mgmt-tls.test.ts:26:const openssl = (args: string[]) => execFileSync('openssl', args, { stdio: 'ignore' }); // ALLOW: test cert
      apps/api/src/features/notifications/sinks.test.ts:1:import { execFileSync } from 'node:child_process'; // ALLOW: fixed-argument openssl creates temporary test-only TLS certificate; no runtime exec, shell or committed key
      apps/api/src/features/notifications/sinks.test.ts:103:  execFileSync( // ALLOW: fixed-argv test-only OpenSSL generates a temporary SMTP TLS certificate; no runtime shell
      apps/api/src/features/pki/testkit.test.ts:3:import { execFileSync, spawnSync } from 'node:child_process'; // ALLOW: test-only verifier (openssl), never product code
      apps/api/src/features/pki/testkit.test.ts:32:  const result = spawnSync('openssl', args, options); // ALLOW: test-only fixed-argv verifier
      apps/api/src/features/pki/testkit.test.ts:44:  return execFileSync('openssl', args, { input, stdio: ['pipe', 'pipe', 'pipe'] }); // ALLOW: test-only verifier
  mode quick · wall time 10m26s · logs /root/ngfw-wt/logs/ci/funny-meninsky-c9ef19-20261017-112350-1815599

CI GATE PASSED
CI_QUICK_EXIT=0
```

## Out of scope / left undone

- The tech-debt row and the user-doc exception (`docs/user/interfaces/default-dataplane-nics.md`) live on the PR #238 branch, not on `main`; the #238 developer points the row at PR #241 (suggested text in the PR description) and can drop the exception paragraph once this PR is merged.
- Not run: `tools/ci.sh full`, lab or appliance acceptance (unit tests on the fake VPP only, by the task's fence).
- A NIC bound after the fact (its kernel netdev handed to DPDK) gets its binding at the next acl projection — the resync after the start-up apply, the next interfaces/security commit or the next auto-block reconcile — exactly when its own interface configuration is applied; before this fix no projection succeeded at all.
- The agent still decides "unbound" by the row's logical name (`desired/hostnics.go`), as recorded in the #238 debt rows; unchanged here.

## Open questions

None for the owner. Merge only with the owner's go-ahead.

## Decisions

- D-250 (`docs/decisions/LOG.md`): pass the lookup into `desired.ACL` and re-split inside `globalBlocking` (vs. a kept-map parameter or a process-global env field).

## Review

Independent panel R1, R2, R4, R7 (read-only reviewer agents; hand-backs saved as `agent-allifs-unbound-nics-review-R<n>.md`): in progress.
