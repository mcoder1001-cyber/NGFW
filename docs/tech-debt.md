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
