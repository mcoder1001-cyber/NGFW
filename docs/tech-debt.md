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
| TD-8b (D-129 Q3): the agent refuses to start without an id range — P10's packaged vrx-agent unit ships `VRX_VPP_ID_RANGE=all` in its EnvironmentFile (the product box is its own VPP), with a packaging test that the unit's environment resolves to `all` | P10 | P10 merge |

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
- WEB-3 review (0ea17400): apps/web/test/e2e/** is outside eslint and the forbidden-pattern/gitleaks scan (pre-existing) — extend `pnpm --filter @ngfw/web lint` and tools/ci.sh forbidden patterns to cover test/e2e. Owner: TD-18 (repo hygiene). — DONE 2026-09-25 (5784201: web lint covers test/e2e; ci.sh secret grep + gitleaks already scan the whole repo).

## Review follow-ups (2026-09-25, manager cycle 21)
- F-bonding verify (e30c49fb) LOW: bond.member accepts any interface with an L2 address — BVI, VXLAN, GENEVE, GRE-TEB, pipe, vhost-user and memif pass; deny those device classes in member.go:91-97 (crash-safe today, VPP fills l2_address only for Ethernet hw). Owner: F-bonding follow-up / TD-22.
- 2026-09-25 04:27 VPP crash (NRestarts 1→2): dns_resolve_name with the dns plugin disabled / no name server → NULL deref in ip4_sas (dns.c vnet_dns_resolve_name l.780 → vnet_send_dns4_request l.234). F-unbound guards its action (5c80aba0, V-entry). The DF-8 helpers dns.ResolveName/ResolveIP on main have no caller yet but carry no precondition: add the "only after dns_name_server_add_del + dns_enable_disable(1)" precondition to their doc comment and a guard parameter. Owner: F-unbound-chrony-syslog (DF-8 hunk) or TD-22.
- F-host-acl-nftables review M4 (298263fa): P10's planned `inet vrx_base` table with a drop policy would defeat host-ACL accepts in `inet vrx` (nftables evaluates every base chain; a drop in any hook chain is final). P10 must either not ship a drop-policy base table, or render host-ACL into the same table/chain. Owner: P10 (packaging), due before P10's nftables unit.
- F-nat44-ed-sessions verify (4421baec) V1: no test fails if the filtered sessions grid starts polling again (apps/web …/SessionsTab.tsx:314) — pin the D-132 "filtered grids refresh by hand only" rule with a fake-timer test. Owner: F-nat44 follow-up / TD-22.
- F-loopback review (777f629f): new row needed — ifsanitize should clear inherited SPAN source state and LLDP entries on interface Create (V19 family; today a reused sw_if_index silently mirrors nothing / keeps a stray LLDP enable). Owner: new TD row after TD-25 (manager-owned ifsanitize).
- F-loopback review Q9: web — a generic `<slug>:group.title` i18n fallback so drawers never show a raw schema group name. Owner: web track (WEB-4a/b or UI-domain-editor follow-up).
- F-loopback review Q2: add the F-loopback slug to SIBLING in packages/schema examples.test.ts — fold into TD-22 (already widens that regex).
- F-nat44-ei review (57d18bbc) H1/H2 → NEW core row: VPP 26.06 nat64 never releases its FIB table locks on a tenant VRF, so a VRF delete fails verify and rolls back the whole commit (also confirm-revert). Core VRF descriptor needs a tolerant delete: when VPP refuses because the table is still locked, leave the table, record it (D-076-style orphan record, cleaned at the next VPP restart/resync) and warn, instead of failing the transaction. Owner: new TD row (core), after TD-11c. Also M3: NAT64 session pages walk the whole session table + BIB per page (bound it like ED's H1). Owner: F-nat44-ei follow-up.

## Review follow-ups (2026-09-25, manager cycle 21b)
- TD-10b verify (faed447d) N1 LOW: apps/api …/key-file.ts:73 opens the JWT key file without O_NONBLOCK — a named pipe at that path would hang the API's 5 s key re-check (needs write access to the key dir). One flag. Owner: TD-22.
- TD-10b hand-offs: M2 (no cross-address guessing limit per account; one IPv6 /48 = 65 536 lockout buckets) accepted residual → SEC-auth; L2/L6/L9 → SEC-auth; L3 P10 nginx must OVERWRITE X-Forwarded-Proto (never pass through) and not expose the docs cookie beyond /api/docs → P10 row notes; L8 → TD-22.
- TD-8b review (f39560f3) C1 MEDIUM → NEW row before F-mpls-ldp / F-igmp-mfib: every per-key rerun rolls back and re-creates the whole transaction; in a resync after a VPP restart with >3 rejected dynamic objects all interfaces are created/deleted 4 extra times (sw_if_index 36 vs 18), feeding the ifsanitize placeholder pressure (TD-25). Fix: two-phase resync (config first, then dynamic objects); swap in TD-9's s.txnTimeout. Owner: new TD-8c row.
- P12 review (7931c596) H3 → NEW row P12-fib-proof (deps LAB-vpp-per-slot, parked on PENDING-vpp-host-hardening): prove BGP routes reach the VPP FIB through linux-nl on a private VPP; no global lcp default-netns change on the shared VPP (manager decision, reviewer recommendation). P12 merges with this acceptance item deferred.
- TD-24 Q1 (M): a VRF change on a DHCP-client interface with a bound lease is refused by VPP and rolls back — dhcp/client.go needs an optional dependency on interface-ip.table/<if> (one line). Owner: TD-22 (after F-kea merges; the file is F-kea's/DF-8's).
- TD-24 Q2 (M, bug on main): `dhcpClient: {}` without a hostname fails at apply ("hostname is empty") — default the hostname (system hostname or empty-allowed) or make the schema require it. Owner: TD-22.
- TD-24 Q3 (L): VPP 26.06 has no dump for addresses it installs itself for IPv6 (SLAAC, DHCPv6) — the same deletion bug will apply once a product path enables them; V26 in vpp-code-track. Owner: F-neighbors-ra / the DHCPv6 client row.
- F-kea verify (26a85d1b) V1/V2 LOW → TD-22: a lease read already in flight when an Apply drops the cache can store its pre-Apply result for ≤10 s; the "cancelled caller" test can flake (return early when the caller already cancelled). V3: with VRX_KEA_MODE=off (tools/app) any commit with a DHCP server is refused — intended until P12. Deferred L2→TD-22, L4→core Q5 row (before the next host run of that test), L5→F-unbound rebase, L6→TD-9, L8→DDNS/TSIG row, L9→F-kea at TD-23 rebase.

## Review follow-ups (2026-09-25, manager cycle 22)
- F-acl review (d58b9105) Q2 → NEW TD row: agent gRPC message limit 4 MiB < a 100k-rule ACL request (~12 MB). Raise to 32 MiB with MaxConcurrentStreams + an API-side 413 size check; no chunking now. Owner: new TD row (agent server + API client), after TD-9.
- F-acl review Q11 → NEW TD row: a candidate edit of a 100k-rule list takes ~40 s in the API datastore. Owner: TD-15 or a new datastore row.
- F-acl review V7: host doc docs/lab/host-vrx-a.md needs a row — the VPP ACL counters flag (acl_stats_intf_counters_enable) is volatile (off after a VPP restart), readable read-only, and only the globals-owner product agent may set it; tests save/restore it (§7).
- TD-13 fix round (04a51453) hand-offs: L8 one shared D-051 reference pattern for masking/validation across agent packages (owner TD-16); L9 validators abandoned on timeout keep running in their goroutine (bounded by their own ctx, but not joined) — document or join (owner TD-13 follow-up); apps/agent/internal/agent/dynsource.go:203 logs panic text unmasked (owner TD-8c).
- F-unbound review (b665904f) M4: on a real box nothing acts on unbound/chrony restart requests (D-079 records them; no actor restarts the daemon) — needs the agent-privileges decision (PENDING-agent-privileges) or an operator action in the UI. Owner: SEC/agent-privileges row.
- TD-25 review (0e7a39a0) L1: if unbinding the output-ACL probe placeholder P fails after P was bound, P is deleted anyway (near-impossible in 26.06) — keep P alive with a probeBound flag. Owner: TD-27 (ifsanitize follow-up). L2 (manager): docs/agent/descriptors/interface.md:128-193 still describes FreshRun/PlaceholderCap — apply TD-25-questions Q1 text with the reviewer's four amendments. Close tech-debt rows "TD-3 re-review M3" and "L1".
- F-object-model (8f5d90b8): move the objects counters from the core agent/metrics.go line onto TD-8's Wiring.AddMetricsCollector seam (renames vrx_agent_objects_* → vrx_objects_*; document the rename). Owner: F-object-model follow-up / TD-22.
- UI-domain-editor review (3ac4825e) D-UDE-3: createMergePatch exists in 3+ copies (interfaces model, WEB-2 kit, UI-domain-editor) — hoist ONE pure copy into packages/schema next to mergePatch/mergePatchAt, then delete the copies. Owner: web track (WEB-4a) after WEB-1/WEB-2/UI-domain-editor merge.
- UI-domain-editor review: WEB-2's "config screen kit over any candidate path" and UI-domain-editor's generic editor are two independently built mechanisms for the same job — reconcile into one (kit uses the editor's path/subtree primitives or vice versa) before a third appears. Owner: WEB-4a.
- F-wireguard verify (4797e681): follow-ups F4–F8, F12–F14 (see F-wireguard-review.md) + a builder warning for road-warrior peers with no endpoint whose 0/0 route sits in the underlay VRF; stack re-run (rollback + 0/0, ::/0, IPv6 routes) after TD-25. Owner: F-wireguard follow-up row.
- P12 verify (f16d754c) N4 → row P12-fib-proof: in private-VPP mode the rig, preflight, vppctl evidence, NRestarts and the stats socket must also point at the private VPP (today they still target the shared one) — add that plumbing to the row's prompt (LAB-vpp-per-slot).
- F-acl verify (7bc16c13) lows V1–V4: V1 an Apply overlapping many concurrent validates of one list can briefly show false drift; V2 host tests' flock -x → -s switch is not atomic (re-read the counters flag after it); V3 every resync reports created:N for acl.config objects (cosmetic, fix the report); V4 a binding onto another slot's interface is refused on the shared host (by design, document). Owner: F-acl follow-up. Also: DF-4's host test flips the ACL counters flag without save/restore (shared-host rules §7) → TD-22.
## From the TD-10a review (2026-09-25, TD-10a-review.md)
- **P10 (nginx):** `proxy_read_timeout ≥ 130 s` (and `proxy_send_timeout`) on `/api/v1/config/{commit,rollback/*,commit/confirm,validate}` — the server's commit budget is 111 s (apps/api/src/commit/budget.ts) and the web waits 130 s; nginx's default 60 s cuts every slow commit into a 504 that the web can only follow up (review M1). | owner: P10 | due-before: P10 merge
- **TD-15 (db pool):** the budget's "DB 15 s" is an assumption: add `connectionTimeoutMillis` to the pool and a `statement_timeout` for the API role so it is enforced (review L2). | owner: TD-15
- **TD-10b:** `users.service.ts:134` password set uses `commits.exclusive` (waits without bound behind a commit); switch to `commits.userExclusive` (409 `commit-busy` after 1 s, like secret delete — D-TD10a-1, review M2). | owner: TD-10b
- **apps/web/src/config/** (pending-change bar owner):** CommitDialog should follow a lost commit answer up with `followOutcome`/`applyOutcome` (net.ts, time-gated until the budget) as RevisionsPage does; the confirm banner should retry a confirm that got 409 `commit-busy` after `retryAfterSec` (review L5, else a confirm near the deadline can lose to a reconcile holding the lock). | owner: the row owning apps/web/src/config/**
- **Secrets (D-TD10a-5):** deleting a secret removes every `secret_version`; a tombstone or "refuse while a revision pins it" would keep old revisions rollback-able (today: a clear 400 `secrets.ref-exists`). | owner: F-backup-restore or a secrets row
