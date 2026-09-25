# Tech debt / backlog for idle workers

The manager pulls from here when nothing on the board is ready. Add items with a one-line why.

- [x] golangci-lint binary install + config wired into `tools/ci.sh` (D-009) — done: tools/ci.sh:13, :269 (D-125 triage)
- [ ] Remove libvirt/virbr0 from the host (installed by mistake; harmless) — needs product-owner ok? no: it is ours, purge it
- [x] `packages/api-client`: switch `openapi-fetch` calls to typed helpers once P06 lands — done: TD-2 (7082cc6)
- [x] Generate the missing `prompts/features/F-*.md` files from FEATURE-TEMPLATE (one per board task) — do this early, it is pure text work — done: prompts/features/
- [ ] `docs/user/` skeleton (MkDocs) so DOCS-GEN has a target

- 2026-09-24 (DF-1 merge): DF-4 acl stats host subtest flakes with "stats data busy" when agent packages run in parallel against the shared stats segment; passes alone. Fix: retry the stats read on busy (bounded) or serialise that subtest under the lab lock.
- 2026-09-24: DF-2 follow-ups N3 (same-shape table on a reused index still claimable while VPP stays up) and N5 (claim-store hygiene).
- 2026-09-24 (DF-6 re-review): N4 write-only 6rd / SR-MPLS policy+steering / GPE entries accept an existing object on re-apply, so parameter changes made while the agent was down are not applied (fix: re-create when the claim record's parameters differ from desired); N6 tunnel create reuses any own interface with the same id; N7 claim file never pruned + crash window between add and record.
- **[done: TD-1]** 2026-09-24: D-080 boot-identity triple must be retrofitted to merged DF-2 (classify store) and DF-4 (acl stats flag) — consolidate into one helper in P08.
- 2026-09-24 (F-startup-gen Q4): tools/lab provision still has its own startup.conf template — switch it to vrx-startupgen.
- 2026-09-24 (P05 verify): a new Apply that itself FAILS still drops an owed revert → unconfirmed config stays until reconnect/restart. Fix: clear the owed revert only when the superseding Apply succeeds. Low: lcp-rt/FRR source handling on route delete (P05-verify.md).
- 2026-09-24 (DF-8): L3 lcp host tap tagging (P12 decides — tagging would make DF-1's tap descriptor delete it); L4 merge `df2` and `dfkit` helpers; DHCP client `Reconnected()` must be called from P05's reconnect hook (P08).
- **[done: tools/ci.sh deploy/vpp step — shellcheck + harness]** 2026-09-24 (F-startup-gen Q7): add deploy/vpp/test-apply-startup.sh + shellcheck deploy/**/*.sh to tools/ci.sh (manager-owned after P09).
- 2026-09-24 (F-vpp-debs Q5): add deploy/vpp/verify.sh to tools/ci.sh quick gate (sub-second).
- 2026-09-24 (DF-7): manager window — run `VRX_DF7_VRRP_HOST=1` and `VRX_DF7_IGMP_HOST=1` host tests each alone with VPP otherwise idle to pin the V22b ip4-options crash trigger; DF-7 L5/L6 open.
- 2026-09-24 (P06): JWT signing-key rotation, `VRX_TRUST_PROXY` setting, owner check of the secret key file.
- **[partly done: per-API-key candidates by TD-2 (7082cc6); the sdk checks are still open, see the table]** 2026-09-24 (F-sdk): add `sdk/test.sh` and `sdk/gen.sh --check` to tools/ci.sh; P06 per-API-key (not per-user) candidates so parallel pipelines of one user cannot edit each other's candidate.
- **[done: tools/ci.sh:410-412]** 2026-09-24 (P13): tools/ci.sh must run `make -C apps/cli lint test build` (apps/cli has its own Makefile, not in the pnpm workspace).
- **[done: TD-2 (7082cc6)]** 2026-09-24 (P13 review H1): API must reject C0/C1 control characters and bidi overrides in commit comments and all free-text fields not covered by the schema (D-049 applies to the API layer too).
| 2026-09-24 | D-100 | Document strings allow LF everywhere; single-line fields (names, descriptions, usernames) rely on per-field schema patterns that are not uniform — add a shared single-line pattern in packages/schema on the next additive contract branch (TD-2 Q4/L2) | — | header/log injection via a multi-line name in a rendered daemon config | low | packages/schema |
| 2026-09-24 | TD-3 re-review M3 | ifsanitize probes every create/delete: ~8 placeholder tables + 8 unbind probes per classify kind → 110–500 API calls per interface create on the fake, ~59 VPP journal lines per run on the host. Replace probing with an exact per-interface binding readback (classify_table_by_interface + the in/out ACL and policer dumps) where VPP offers one; keep probing only for kinds without a readback | — | slow bulk interface creation (1000 sub-interfaces ≈ 0.5 M API calls), log flooding on production boxes | medium | before any production image (P10/P14) |

## From the TD-6 review (D-116, 2026-09-24) — deploy/vpp/apply-startup.sh harness
- F3 harness cache key misses the test fixtures; F4 `VRX_TEST_ROOT` guard does not cover driverctl/ifup/networkctl/netplan; F5 own rollback goes FORCED after 60 s when only the holder died; F6 no harness timeout in CI; F7 SIGTERM trap path untested (systemd kills the run unit after 90 s); F8 a flaky pass is warned once then cached; F9 minor rollback edge cases (details: TD-6-review.md in refs/archive/TD-6)
- (pre-existing, TD-6 review) rollback verification never checks that VPP's boot identity changed (`apply-startup.sh` ~:753); `kill_recorded` may SIGKILL already-reaped PIDs (PID reuse)
- (TD-7 finding) `check_health` (`deploy/vpp/apply-startup.sh:568`) reads `systemctl show` once without retry: a timed-out/partial D-Bus read counts as "vpp.service restarted" → needless rollback (never a false commit); harness scenarios 5/28/29 start with an unguarded apply so a very loaded host kills a whole shard instead of getting a rerun

## Open items: owner and due-before (D-125, 2026-09-24 triage of the REVIEW-2026-09-24 report and the architecture audit)
Items above that are not ticked keep their text; this table gives each one an owner and a due-before. "idle pool" = no row yet, the manager pulls it when a slot is free.

| item | owner | due-before |
|---|---|---|
| P06: JWT signing-key rotation, `VRX_TRUST_PROXY`, owner check of the secret key file | TD-10b | P10 |
| P05 verify: a failing superseding Apply drops the owed revert | TD-9 | P10, INTEGRATE-E2E |
| DF-8: DHCP client `Reconnected()` called from the reconnect hook | TD-8 | TD-8 merge |
| R2-stores (P08 re-review): claim-index refresh bounded at 5 s, attribute Create should claim before writing | TD-11b | TD-11b merge (before F-nat44-* merge) |
| TD-3 re-review M3: ifsanitize probing → exact per-interface binding readback | idle pool | before P10/P14 |
| ApplyResponse has no warnings field (2.5 agent part) | the first row that emits an Apply-time warning, with its contract commit after the wave-A proto edits | that row's merge |
| `sdk/test.sh`, `gen.sh --check` and `deploy/vpp/verify.sh` are not in tools/ci.sh; audit ARCH-03/A7 adds a go-generate check for the 28 internal `*.pb.go`, and the SDK on main lacks `Users_setPassword`/`Config_revisionDiff` (whichever of P08/TD-4 merges second commits `chore(sdk)`) | manager | P10 (audit: now — TD-6's ci.sh change has landed) |
| No `mkdocs.yml` (`docs/user/` skeleton) | manager | DOCS-GEN |
| libvirt not purged (virbr0) | manager | P14 |
| DF-2 N3/N5 and DF-6 N4/N6/N7 claim hygiene | TD-11b if it touches those files, else a row | F-tunnels / F-lisp / F-srv6 merge |
| DF-4 stats-busy flake | F-acl | F-acl merge |
| Stray `fail.out` in /root/ngfw and its `.git/info/exclude` line | manager | now, 0.1 h |
| Remote mirror stale (origin/main 3364aca is 758 commits behind); contributing.md:3/:194 say "no remote" | product owner via PENDING-remote-mirror | — |
| New `docs/status/have-not.md` register: D-059 six, Ansible, NETCONF, host-stack VCL/TLS/QUIC, bulk provisioning, SRv6 proxies/mobile, air-gapped bundle, marketplace, secure boot/image signing, hsflowd, dedicated system/dataplane/management screens; STATUS-FINAL notes point at it | manager | STATUS-FINAL |
| Audit ARCH-06, TD-16 full scope: fsync first (`df2.WriteFileAtomic` and the df6 claims save lose data on a host crash), then one `descriptors/kit` with one ParsePrefix policy (reject host bits; df2/df6 mask them today; copies in dfkit/df2/df6/df7/vpn), one `ErrRetrieveUnsupported`, `Register(r, Env)`; DF-8 L4 (merge the df2 and dfkit helpers) belongs here | TD-16 | wave B spawn |
| Audit ARCH-07: 6 resolver interfaces; the Go regexes are looser than Zod (chrony accepts 123 characters) | with the PENDING-secret-channel answer (P11) | P11 secret step |
| Audit ARCH-09: D-054 dedupe — `_shared` NO_CONTROL_CHARS rejects TAB, the root pattern allows it (same area as the D-100 single-line pattern above) | a row (manager opens it) | next additive contract branch |
| Audit ARCH-13 docs pass: drop "vppctl passthrough" from docs/04:88 (workers read it as sanctioned), apply D-050's ntp move, log Go 1.26; review C: docs/04:66 `/state/sessions` → `/state/nat/sessions` | manager | next docs pass |
| Audit ARCH-14: vpn.proto comments say sha256 where the code uses hmac; fix and regenerate | next contract commit (P11) | P11 merge |
| Review C/B prompt edits: 00-CONTEXT FAST-MODE DoD (2), FEATURE-TEMPLATE Acceptance and REVIEW-PROMPT §8 get the "reachable through API and UI" sentence; MANAGER-PROMPT §2 D-112 step gets "merge only after every merge-after row has merged"; P10 envelope time box 15 → 24 (:59); prompts/SECURITY-REVIEW.md (1 h) | manager | next spawn / next merge |
| DF-7 manager window: pin the V22b ip4-options trigger (`VRX_DF7_VRRP_HOST=1`, `VRX_DF7_IGMP_HOST=1`, VPP idle) | manager | PENDING-vpp-c-track option 2 |
| VRX_DEV_WEAK_PASSWORDS (dev-weak-passwords, product-owner request): the P10 vrx-api unit must set `Environment=NODE_ENV=production` (the API then refuses the flag at boot) and nothing packaged may set VRX_DEV_WEAK_PASSWORDS; add a P10 check that greps the unit/env files for it | P10 | P10 merge |
| F-startup-gen Q4: tools/lab provision still has its own startup.conf template | idle pool (TD-19 owns the same provision hunk) | first `tools/lab provision vrx-b\|vrx-c --apply` |
| D-100 shared single-line pattern in packages/schema; TD-6 review F3–F9 | idle pool | — |

## P08 re-review (D-118, 2026-09-24) — interfaces vertical slice, no code in P08's fix round 2
- **R2-stores** (low; `apps/agent/internal/subsystems/stores.go:124`, DF-1 `attributes.go:259-260`): a DF-1 attribute Create on an
  untagged interface writes VPP first and claims after; the claim's index refresh (`fileClaims.claim` → Invalidate + Resolve) has its
  own 5 s bound (`subsystems.go:119`), shorter than the transaction's 60 s. A VPP API stall > 5 s between the write and the claim fails
  the Create after VPP changed (N7's `ErrClaimUnbound`); the scheduler does not journal a failed Create → ROLLED_BACK with the value
  still in VPP, invisible to Retrieve, never reverted by resync. Untagged (DPDK) NICs only; af_packet is tagged. Fix: bound the claim
  refresh by the caller's context (≥ the API deadline) or retry it once after a reconnect; and in DF-1, claim before writing (release
  on write failure) or undo the write when the claim fails — or let the scheduler journal a Create that returns Meta with an error.
- **R3-gauge** (low; `apps/agent/internal/subsystems/subsystems.go:162`, ifsanitize = TD-3/TD-5 files): `AfterResync` calls
  `ifsanitize.Release` but never sets `vrx_agent_iface_quarantined` from the holders it found; after an agent restart the gauge reads 0
  while this owner's still-dirty quarantine holders are in VPP (TD-3 re-review L6). Fix: `Release` returns the number of holders left
  (holders − released) and the wiring sets `Stats.Quarantined` to that absolute number after every Release (not deltas).

## From the wave-A reviews (D-129, 2026-09-24)
- (F-nat44-ed Q3 d) the agent should return a canonical form of the running config so /state/drift stops flagging NAT pool names/descriptions and owner-mode `enabled`/timeouts
- (TD-8 R5) metrics collectors run serially with a per-collector 5 s deadline that only a cooperative collector honours; bound the whole scrape before the first collector merges
- (F-object-model Q2) FQDN refresh uses a fixed interval; DNS TTLs need golang.org/x/net promoted in go.mod
- (WEB-2 M3) the config kit keeps writeOnly members (passwordHash) in mutation variables/React state — fix before any secret-leaf kit screen
- (F-vrf-static-ecmp Q8) govpp drops dump replies on a loaded host — all descriptors exposed; (Q9) CLI `vrx ping` sends no body (400 since ping is implemented)

## From the wave-A reviews, batch 2 (D-131/D-132, 2026-09-24)
- (TD-7 finding) `check_health` (`deploy/vpp/apply-startup.sh:568`) reads `systemctl show` once without retry: a timed-out/partial D-Bus read counts as "vpp.service restarted" → needless rollback (never a false commit); harness scenarios 5/28/29 start with an unguarded apply so a very loaded host kills a whole shard instead of getting a rerun
- (F-vrf-static-ecmp Q4) packages/schema/src/examples.test.ts rejects feature example files (`<slug>-*.json`) — widen the SIBLING regex once (F-vlan-qinq Q3, F-bridge-l2 Q4, F-neighbors-ra Q5 hit the same)
- (F-vrf-static-ecmp Q8) govpp drops dump replies on a loaded host — needs a fix in apps/agent/internal/vpp (all descriptors exposed)
- (F-vrf-static-ecmp Q9/L5) CLI `vrx ping`/`traceroute` send no body (400 since ping is implemented) and the CLI docs for /state/routes are stale
- (F-vrf-static-ecmp Q10, F-neighbors-ra Q9) the shared fake agent needs a per-feature Action dispatch table
- (F-vrf-static-ecmp M2) FIB browser: keyset cursor instead of offset paging

## Review follow-ups (2026-09-25, manager cycle 20)
- TD-10a verify nits (00d3ab9f): apps/web net.ts:106 compares the pending commit against the client clock — use the pending snapshot taken before the request; tech-debt text says followOutcome lives in net.ts, it is RevisionsPage.tsx:101; MIN_CROSS_WAIT_MS lets the lock wait reach 1.25 s while budget.ts documents 1 s. Owner: apps/web/src/config/** owner / TD-15.
- F-vrf-static-ecmp verify (19d250c0): two API e2e files in one vitest call → the second fails DB setup with PostgreSQL 28P01 (auth) — harness bug in the e2e DB bootstrap (each file alone passes). Owner: TD-15 / API test harness. Also: tests don't yet pin the no-polling UI, the 100k cap or selector-at-init (F-vrf-static-ecmp follow-up).
- TD-11c verify (e83c6318): prompts/features/F-tunnels.md and F-mpls-srmpls.md must state the TD-11c obligation — every interface creator registers iface.RegisterKind or provides the interface/<name> KeyProvider, and removes itself from the guard allowlist; add a guard that fails if a creator on the gap list gets wired while still allowlisted. (manager: prompt edits in the next batch)
- WEB-3 review (0ea17400): apps/web/test/e2e/** is outside eslint and the forbidden-pattern/gitleaks scan (pre-existing) — extend `pnpm --filter @ngfw/web lint` and tools/ci.sh forbidden patterns to cover test/e2e. Owner: TD-18 (repo hygiene).
