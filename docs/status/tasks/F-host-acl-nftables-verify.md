# Verify — F-host-acl-nftables fix round 1 (task/F-host-acl-nftables @ 3377e14b; code 32ad5184, contract 723c92cf)

Reviewer, 2026-09-25. A focused verify of the findings in `F-host-acl-nftables-review.md` (298263fa), not a full
re-review. Diff checked: `298263fa..3377e14b`, 19 files.

## Verdict: **APPROVE**

Every finding that was sent back is fixed and holds up under independent probes, including real nft 1.1.6 in a slot-9
namespace. No regressions found.

## Findings

| id | status | where | evidence |
|---|---|---|---|
| H1 | **fixed** | agent `renderers/nftables/build.go:61-62,106-111`; schema `packages/schema/src/semantic/host-acl-nftables.ts:24` (`acl.host-output-priority`, contract 723c92cf, additive, 2 files) | Probe on the agent's `Build`: output −300 and −200 → one error at `/acl/hostAttachments/0/priority`. −199, a disabled output at −300, input −300 and forward −300 → no error. The schema test covers the same matrix (`host-acl-nftables.test.ts`). Both tiers skip disabled attachments, so they agree. Both surface as a 400 with that pointer. Refusing `≤ −200` rather than `< −200` is correct: at exactly −200, ordering against conntrack is undefined. The docs were corrected (`docs/user/firewall/host-acl-nftables.md` attachment row, "Anti-lockout" intro) |
| H2 | **fixed** | `renderers/nftables/paths.go:73-80` (`ProductPaths`: apply only for owner `vrx` **and** globals owner), `:104-133` (`VRX_HOST_ACL_MODE=apply` refused without the globals owner), `:148` (`Validate` refuses apply without it); `subsystems/host_acl.go:30` passes `Env.GlobalsOwner`; `tools/app:108` | `tools/app` has exactly one changed line, which adds `VRX_HOST_ACL_MODE=check`. That line is redundant with `VRX_GLOBALS_OWNER=0`, and the redundancy is harmless. A real box (owner `vrx`, `VRX_GLOBALS_OWNER` unset → true, `agent.go` `ConfigFromEnv`) still gets `apply`. Slot agents: `check`, or `netns` with `VRX_HOST_ACL_NETNS`. `TestPathsFromEnv` covers the new cases. No other caller of the changed signatures exists (grep) |
| M1 | **fixed** | `parse.go:56` (`exprHash`: `expr` JSON compacted, counter values stripped), `:92-100` (table `flags dormant`); `descriptor.go:123` (hashes read back after `nft -f` and stored), `:180-194` (`annotate` pairs a rule only while both its verdict and its body hash match); `state.go` strips the hashes | **Real nft, slot-9 netns** (reviewer probe `TestVerifyM1RealHandEdits`, run from a scratch copy, nothing added to the branch). Each of these made Retrieve ≠ desired: `nft replace rule` flipping drop→accept with the comment kept; changing the port with the comment kept; removing `log` with the comment kept; `add table … { flags dormant; }`. After each, a resync restored Retrieve == desired, and the listing had `dport 2323` and no `dormant`. nft 1.1.6 prints `"flags": ["dormant"]` as an array, which I checked in a throwaway `ns-w9-rvp`, so the unit fixture matches the real format. A store written before this round (no hashes) still pairs by comment and verdict (`TestKernelRoundTrip`). The worker's netns test also passed against real read-back hashes (Retrieve == desired after apply, update, rollback and restart), so the hash is stable across listings |
| M2 | **fixed** | `build.go:307-313` (`dynamic`, `canMatch`), `:622-671` (non-rendered "ghost" family variants for the simulation only; `Build` skips them when rendering), `:675` (`fqdnBearing`, recursive through groups); `lockout.go:192,212` (`dynamic` → `relPartial`) | Probe `TestVerifyM2`: 4 documents × 5 DNS answer sets (none, v4 only, v6 only, both, a remote address). The error set was **identical across all answers** in each document. An FQDN accept as the only protection, with `defaultInput: drop` → refused. An FQDN-bearing group as a drop source → counts as a drop. A static accept followed by an FQDN drop → accepted. An FQDN as the destination of an IPv4 drop → counts, even unresolved. Ghost rules never reach the rendering (golden tests unchanged) |
| M3 | **fixed** | `agent/service_test.go:602` | The example is the first `rootKeys` entry with no `subsystems.Domains` entry; the check is skipped when none is left. This follows D-129 F5 and survives F-wireguard (`vpn`) and F-unbound (`management`) |
| L1 | **fixed** | `lockout.go:93-97` | With no `sources`, the error tells the operator to set `antiLockout.sources`. It also says that FQDN matches never count as protection |
| L4 | **fixed** | `docs/user/firewall/host-acl-nftables.md:53` | A confirmed-commit paragraph. The REST (`?confirm=<sec>`) and CLI (`commit confirm <sec>`, `confirm`) syntax match `docs/user/cli/reference.md:77,79`. It names what the check cannot see: other tables and a wrong `sources` |
| L5 | **fixed** | `web/…/host-acl-nftables/queries.ts:12` | The stale "5 s poll" comment is gone |

Not in this round (as agreed): L2, L3 and L6 stay low. M4 (P10 `vrx_base` vs `inet vrx`) is a manager decision before P10;
the user doc now names other tables as the confirmed-commit case.

## Per-key failure scoping: the manager's answer is sufficient

Commits stay atomic (00-CONTEXT rule 4, AD-4), so a per-key partial success would contradict the architecture.
Removing the DNS dependency closes M2 at its cause:
- After this round, the only runtime input to `Build` is the FQDN answer set.
- The answers now change only set elements, which are canonical, aggregated prefixes that `nft -c` accepts.
- They never change the lockout result.
- Every other `Build` error is a pure function of the document, which already passed at commit.

So a document that committed cannot fail host-ACL validation in a later resync. The one theoretical exception is an
FQDN-bearing object whose answers exceed `objects.MaxEntries` (10 000 prefixes, `objects/expand.go:34`). That is not
realistic for DNS answers, needs no action now, and would be a one-line cap in the resolver if it ever matters.

## Regressions: none

| check | result |
|---|---|
| `go test -race -count=1 ./internal/renderers/nftables/... ./internal/subsystems/... ./internal/agent/... ./internal/desired/...` | `ok` for all four packages |
| `pnpm --filter @ngfw/web test` | 18 files, 116 tests passed (host-acl: model 9, page 3) |
| `packages/schema` `vitest run` (all) | 38 files, 1219 tests passed (includes the new `acl.host-output-priority` case) |
| netns rerun on slot 9 (`TestIntegrationHostFirewallInSlotNetns`, lab lock shared) | PASS 2.87 s. Port 2323 dropped, counter 0 → 2. 2424 rejected after the update. Restart re-render 190 ms with an empty diff |
| root netns `nft list tables`, before and after each of my three slot-9 runs | identical every time (`ip filter`, `ip nat`, `ip mangle`, `ip6 filter`, `ip6 nat`, `ip6 mangle`). `nftables.service` inactive and disabled. No w9 netns or links left. The empty `/run/vrx-test/w9` was removed. Only `nft list` ran in the root netns |
| merge fit | Unchanged from the review. Rebased onto F-object-model: 1 conflict, the generated `operations_gen.go`. This task's diff onto main: 4 conflicts, all generated. `tools/app` auto-merges |
| worktree | clean. The `dist/` directories my builds created were removed |

Merge notes carried over from the review: the squash subject must start with `contract(…)`, now for the schema, proto
and api-client paths. The F-acl reconciliation (Q8) is unchanged. The TD-11a and TD-13 board gates stay as the manager
decided.
