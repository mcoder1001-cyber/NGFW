# P12 verify (fix round 1): **APPROVE**

Focused re-verification of review 7931c596 (`P12-review.md`) on `task/P12` @ e7b374b5, 2026-09-25. No host runs.

**Result:**
- Verified fixed: H1, H3, M2, M3 and M4.
- H2: closed by this commit. My own review file was the last place that quoted the literal.
- Still open, by the manager's plan: M1, done by P12 once F-vrf-static-ecmp lands.
- New notes: three low items (N1–N3) and one medium note (N4) for the deferred row P12-fib-proof. None of them block
  the merge.

## Findings

| # | status | evidence |
|---|---|---|
| H1 | **fixed** | `subsystems/frr.go` `Dependencies` now returns nil, and `desired.FRRDependencies` is gone (7bc233b6). `TestFRRStageSurvivesPairChanges` (`frr_test.go:342-414`) runs the real scheduler with the core, alias, pair and FRR descriptors. Removing one of two pairs, and recreating a pair (tap → tun), each give frr.config exactly `[update]`, and the framework-only configuration is never written. **Mutation check:** I restored the old per-pair dependencies through `go test -overlay`, and the test fails with `frr.config ops [recreate], want exactly [update]`. With no dependencies at all, recreating an interface cannot cascade either. |
| H2 | **fixed** (this commit) | P12-questions Q16 was reworded (2fecb868). The one remaining hit was my review, `P12-review.md:27`, which is reworded here without the literal. `git diff 4f472cc7 HEAD \| gitleaks stdin`: at e7b374b5, `leaks found: 1` (the review line); on the tree with this commit, `INF no leaks found`, exit 0. The output is pasted below. |
| H3 | **fixed** (removed and deferred) | T2 is gone: no `LcpDefaultNsSet`, no globals lock, no restore (941adbd1). Acceptance #1 is marked **DEFERRED, row P12-fib-proof** in P12.md §7. On the shared VPP the test now sets no VPP-global value. The design's VPP-side logic is right; see N4 for what it still lacks. |
| M2 | **fixed** | Two new semantic rules, `routing.bgp-frr-description` and `routing.bgp-route-map-seq` (`semantic/bgp.ts`), mirror `frr.Description` and seq 1–65535; they come with 10 tests. The same length limit applies on both sides, since both require printable ASCII. Interface descriptions are no longer rendered into FRR. Render errors now carry `policy.FieldError` paths. My probe of `desired.FRR` gave: <br>• a neighbour description → `/routing/bgp/neighbors/10.0.0.1/description`<br>• the second of three unsorted route-map entries with seq 70000 → `…/routeMaps/rm/entries/1/seq` (the **document** index)<br>• a bad prefix-list rule → `…/prefixLists/pl/rules/1`<br>• an as-path → `…/entries/0/match/asPath`<br>• a route-map entry description containing `\|` → `…/entries/0/description`<br>• a Persian description on a paired interface: the commit now succeeds (kvs=1, no error)<br>Injection is still refused everywhere. |
| M3 | **fixed** | **Agent:** `FRR.State` takes a one-slot channel and answers `ErrStateBusy` → UNAVAILABLE after 3 s; the lcp pairs are read inside the same slot. `TestStateIsSerialised` passes. **Web:** `useRoutingEvents` invalidates only when `dataUpdatedAt` is at least 30 s old (`eventRefetchDue`, with a model test); Refresh stays immediate. **API:** `annotateFrr` calls the agent only when `proto` is given; without it, 0 agent calls are made and the page cannot fail (`routes.test.ts`, 3 tests). |
| M4 | **fixed** | The agent warns with `routing.bgp-lcp-netns` at `/interfaces/<n>/lcp/netns` whenever `netns` is set (`desired/lcp.go:31-36`, test `project_p12_test.go:62`). There is a user doc, a descriptor doc, and en/fa UI help ("leave empty: … the only one linux-nl hears"). The Q1/Q2/Q8 facts are corrected. |
| M1 | open (planned) | `merge-tree main task/P12` is still clean (77eabd8b, main 1d3ccf31). After F-vrf lands (tip 14900ad6, `TestSvsRangeFromSlot` fixed there), P12 merges main, resolves the 9 files as the review said, and regenerates. The squash subject starts with `contract(`. |
| L1, L2, L3, L4 | fixed | Q8 now says 100. The timing note is in P12.md. The L3 behaviour is in `frr-bgp.md`, and the L4 comment is in `frr.go`. The passwordRef help now says it is refused. |

## New notes (not blocking)

| # | sev | where | note |
|---|---|---|---|
| N1 | L | `packages/schema/src/domains/ext/frr-linuxcp.ts:64` | The schema's own `help` for `netns` still says "default: the linux-cp default namespace (where FRR runs)". The UI overrides it with the new en/fa text, but OpenAPI and JSON Schema still carry the old one. Change it together with the M1 merge. |
| N2 | L | `p12_topology_integration_test.go:399-401` | The private-mode refusal compares the socket path as a string with `/run/vpp/api.sock`, so a different spelling of the same socket passes (`/var/run/vpp/api.sock`, a symlink). The default-netns check behind it still refuses the shared VPP, whose default netns is unset. Suggested: `os.SameFile` on both sockets. |
| N3 | L | same file, `:499-505` vs `:443-485` | `checkPrivateFIB` runs at step 1, **after** the preflight, `tools/lab rig up`, the FRR harnesses and the agent start. Move the check to the top of the test so a mis-set run refuses before it touches anything. |
| N4 | M (for row P12-fib-proof, not P12) | P12.md §7 "Test design", and the same test file | "Runs the existing test unchanged, only the environment differs" is not true yet. Five things still go to the **shared** VPP and its unit: `tools/lab rig up/down` (plain `vppctl`, i.e. `/run/vpp/cli.sock`), the V19 preflight (`go run ./cmd/vrx-vpp-preflight` without `-socket`), the `vppctl show lcp` / `show ip fib summary` evidence, `nRestartsP12` (`systemctl show vpp`), and `hostConfig`'s hard-coded `VPPStatsSocket /run/vpp/stats.sock`. In private mode the rig's af_packet side would therefore land on the shared VPP, and `handRigToAgent` would delete through the private socket. The VPP-side logic is sound: the default netns comes from startup.conf, pairs carry no netns, the table-0 count must be 0 before and after, a withdraw/announce after the restart proves the recreated pairs are heard, and the rollback runs before any pair is deleted. P12-fib-proof must add socket plumbing: a VPP socket or CLI parameter for `tools/lab rig`, `-socket` for the preflight, `vppctl -s`, the private unit's NRestarts, and the stats socket. Write this into that row's prompt. |

## Tests run (reviewer, e7b374b5)

| what | result |
|---|---|
| `go test -race -count=1` over `renderers/frr/...`, `descriptors/lcp`, `lcpmap`, `subsystems`, `agent`, `desired` (with an overlay of F-vrf's `svs/ownership.go`, which is on F-vrf's tip) | all ok except `agent`: **only** F-vrf's `TestSvsRangeFromSlot` ("product range {0 0}", fixed on F-vrf 14900ad6); 0 data races |
| H1 mutation (old dependencies, applied through an overlay) | `TestFRRStageSurvivesPairChanges` FAILS as it should |
| `apps/web` vitest `src/domains/routing/bgp src/nav` | 10 passed |
| `apps/api` vitest `src/features/bgp` | 3 passed (`routes.test.ts`) |
| `packages/schema` `semantic/bgp.test.ts` | 10 passed |
| `merge-tree --write-tree main task/P12` | clean (77eabd8b) |

gitleaks over the squash diff, `git diff 4f472cc7 <tree> | gitleaks stdin --no-banner --redact --exit-code 1`:

```
== committed HEAD (e7b374b5) vs merge-base
INF scanned ~1185911 bytes (1.19 MB) in 1.65s
WRN leaks found: 1            (generic-api-key: P12-review.md:27, the reviewer's own quote)
== with this commit's P12-review.md fix
INF scanned ~1185975 bytes (1.19 MB) in 1.5s
INF no leaks found
exit 0
```

## Merge conditions

1. F-vrf-static-ecmp merges first.
2. P12 merges main (M1: resolve the 9 files, regenerate, fix N1), and `tools/ci.sh --base main` is green.
3. Squash per D-112 with a `contract(proto): …` subject, and re-run `gitleaks stdin` on the squash diff.
4. N2 and N3 can go into the same round (about 15 min). N4 belongs to row P12-fib-proof.
5. After TD-11c lands, remove `creators_guard_test.go:104` (Q17).
