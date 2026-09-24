# Tech debt / backlog for idle workers

The manager pulls from here when nothing on the board is ready. Add items with a one-line why.

- [ ] golangci-lint binary install + config wired into `tools/ci.sh` (D-009)
- [ ] Remove libvirt/virbr0 from the host (installed by mistake; harmless) — needs product-owner ok? no: it is ours, purge it
- [ ] `packages/api-client`: switch `openapi-fetch` calls to typed helpers once P06 lands
- [ ] Generate the missing `prompts/features/F-*.md` files from FEATURE-TEMPLATE (one per board task) — do this early, it is pure text work
- [ ] `docs/user/` skeleton (MkDocs) so DOCS-GEN has a target

- 2026-09-24 (DF-1 merge): DF-4 acl stats host subtest flakes with "stats data busy" when agent packages run in parallel against the shared stats segment; passes alone. Fix: retry the stats read on busy (bounded) or serialise that subtest under the lab lock.
- 2026-09-24: DF-2 follow-ups N3 (same-shape table on a reused index still claimable while VPP stays up) and N5 (claim-store hygiene).
- 2026-09-24 (DF-6 re-review): N4 write-only 6rd / SR-MPLS policy+steering / GPE entries accept an existing object on re-apply, so parameter changes made while the agent was down are not applied (fix: re-create when the claim record's parameters differ from desired); N6 tunnel create reuses any own interface with the same id; N7 claim file never pruned + crash window between add and record.
- 2026-09-24: D-080 boot-identity triple must be retrofitted to merged DF-2 (classify store) and DF-4 (acl stats flag) — consolidate into one helper in P08.
- 2026-09-24 (F-startup-gen Q4): tools/lab provision still has its own startup.conf template — switch it to vrx-startupgen.
- 2026-09-24 (P05 verify): a new Apply that itself FAILS still drops an owed revert → unconfirmed config stays until reconnect/restart. Fix: clear the owed revert only when the superseding Apply succeeds. Low: lcp-rt/FRR source handling on route delete (P05-verify.md).
- 2026-09-24 (DF-8): L3 lcp host tap tagging (P12 decides — tagging would make DF-1's tap descriptor delete it); L4 merge `df2` and `dfkit` helpers; DHCP client `Reconnected()` must be called from P05's reconnect hook (P08).
- 2026-09-24 (F-startup-gen Q7): add deploy/vpp/test-apply-startup.sh + shellcheck deploy/**/*.sh to tools/ci.sh (manager-owned after P09).
- 2026-09-24 (F-vpp-debs Q5): add deploy/vpp/verify.sh to tools/ci.sh quick gate (sub-second).
- 2026-09-24 (DF-7): manager window — run `VRX_DF7_VRRP_HOST=1` and `VRX_DF7_IGMP_HOST=1` host tests each alone with VPP otherwise idle to pin the V22b ip4-options crash trigger; DF-7 L5/L6 open.
- 2026-09-24 (P06): JWT signing-key rotation, `VRX_TRUST_PROXY` setting, owner check of the secret key file.
- 2026-09-24 (F-sdk): add `sdk/test.sh` and `sdk/gen.sh --check` to tools/ci.sh; P06 per-API-key (not per-user) candidates so parallel pipelines of one user cannot edit each other's candidate.
- 2026-09-24 (P13): tools/ci.sh must run `make -C apps/cli lint test build` (apps/cli has its own Makefile, not in the pnpm workspace).
- 2026-09-24 (P13 review H1): API must reject C0/C1 control characters and bidi overrides in commit comments and all free-text fields not covered by the schema (D-049 applies to the API layer too).
| 2026-09-24 | D-100 | Document strings allow LF everywhere; single-line fields (names, descriptions, usernames) rely on per-field schema patterns that are not uniform — add a shared single-line pattern in packages/schema on the next additive contract branch (TD-2 Q4/L2) | — | header/log injection via a multi-line name in a rendered daemon config | low | packages/schema |
| 2026-09-24 | TD-3 re-review M3 | ifsanitize probes every create/delete: ~8 placeholder tables + 8 unbind probes per classify kind → 110–500 API calls per interface create on the fake, ~59 VPP journal lines per run on the host. Replace probing with an exact per-interface binding readback (classify_table_by_interface + the in/out ACL and policer dumps) where VPP offers one; keep probing only for kinds without a readback | — | slow bulk interface creation (1000 sub-interfaces ≈ 0.5 M API calls), log flooding on production boxes | medium | before any production image (P10/P14) |

## From the TD-6 review (D-116, 2026-09-24) — deploy/vpp/apply-startup.sh harness
- F3 harness cache key misses the test fixtures; F4 `VRX_TEST_ROOT` guard does not cover driverctl/ifup/networkctl/netplan; F5 own rollback goes FORCED after 60 s when only the holder died; F6 no harness timeout in CI; F7 SIGTERM trap path untested (systemd kills the run unit after 90 s); F8 a flaky pass is warned once then cached; F9 minor rollback edge cases (details: TD-6-review.md in refs/archive/TD-6)

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
