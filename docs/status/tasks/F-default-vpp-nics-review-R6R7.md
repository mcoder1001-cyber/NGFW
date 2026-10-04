> Historical recovery record from merge `79fff64a` (2026-09-28). Current recovery verification is in `F-default-vpp-nics-wip.md`; historical completion and publication claims do not describe the current main.

# F-default-vpp-nics — review R6+R7 @8f200fe6 — filed by the manager from the reviewer hand-back (2026-09-28)
Verdict: **BLOCK** (R6 1 BLOCKER/1 MAJOR/3 MINOR/2 NIT; R7 2 BLOCKER/4 MAJOR/4 MINOR/1 NIT). Web typecheck/lint OK, interfaces vitest 49/49 (none covers builtIn/awaitingDataplane/physical/release); merge-tree clean; CI log dir …-144732-2515428 matches (1874243e).
## R6
1 BLOCKER model.ts:28-35 interfaceFormSchema() does not strip `physical` → the SchemaForm shows it on every interface (incl. add): an operator can add it to loop5 (permanently undeletable), remove it (403), or change owner without the confirm dialog (owner-consistent fails) → strip it, show pci/owner read-only, release/reclaim the only owner path; test.
2 MAJOR D-155: en/fa interfaces.json:21,22,25,26 "the engine (VPP)", "DPDK"; schema help interfaces.ts:201,:206 → "the engine"/«موتور»; no field.physical keys (moot once stripped).
3 MINOR release/reclaim error rendered behind the modal → ProblemAlert inside DialogContent. 4 MINOR hand-copied DataplaneCandidate → @ngfw/schema types. 5 MINOR no web test for the new UI; the questions file claims coverage → add one InterfacesPage case or correct. 6 NIT status column vs chip. 7 NIT wording/glossary/DialogActions/LTR isolate.
## R7
1 BLOCKER unpasted pass claims (agent unit, schema "7 tests", API e2e "4 tests" — the e2e is not in the quick gate) → paste commands + summaries; R1 re-runs the e2e on slot w1.
2 BLOCKER (D-175) NRestarts / startup.conf sha256 only in prose; helpers return "" on error → log both, re-run under the lab lock, paste, evidence .txt.
3 MAJOR Shared hunks incomplete and unanchored: projection.go:236-255 (edits an existing line), main.ts, config.ts, state.controller.ts; unlisted datastore.service.ts:19-21,199-215 (duplicate import) and documents.ts:349-367 → anchors + full list (D-174).
4 MAJOR user doc CLI equivalent wrong (`ngfw config interfaces …` does not exist; whitelist/devices are not synced) → real sequence (`ngfw set interfaces ens161 physical owner host`, `ngfw delete dataplane pciWhitelist …`, `ngfw delete dataplane devices …`, `ngfw commit`) + a worked seeded-JSON example.
5 MAJOR D-177 #1 (non-PCI NICs never enumerated) not in the doc.
6 MAJOR CI evidence is one line; re-run after the fix round and paste the tail.
7 MINOR acceptance overclaims (no golden JSON, audit event unasserted, screenshot ticked). 8 MINOR -contract.md stale (semantic file, api-client regenerated, bound_to_dpdk by MAC not PCI [other: R3]). 9 MINOR decisions to the LOG (manager: D-193). 10 MINOR "For P10" lacks NGFW_SEED_DEFAULT_NICS=1 (D-192); link from basics.md. 11 NIT committed envelope copy.
Scope: no creep; the −2,689 lines are protoc regeneration (dataplane.pb.go −2,685) + 4 trivial lines.
