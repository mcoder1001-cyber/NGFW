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

Complete `tools/ci.sh quick` on the head: **running at the time of writing** — the result is appended to `agent-allifs-unbound-nics-wip.md` and the PR description when it ends.

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
