# Combined carrier completion, 2026-10-09

Owner: carrier_finish. Branch: codex/completion-recovery-20261009.
Product ownership: combined PPP carrier, PD, VLAN, WAN projection/runtime and generated contracts in this worktree. Root owns publication, final board/status and final CI/merge.

Integrated remote carrier 42b0c993 into combined completion 11a7125d, preserving the credential startup/projection hooks, unnumbered projection, wizard, agent privilege unit and RA ownership wrappers. Carrier already contains helper 216ee225 product bytes, WAN ownership context and scheduler prerequisite recovery bytes; these were compared rather than duplicated. Added packaging 8823d89b and a compile-time assertion that the real PppoeRuntime satisfies the WAN forwarding interface.

Executed combined packaging/helper Python fixtures: 32 tests PASS (0.313s), direct helper 28 controls PASS (0.044s). No host activation or CI. Go/Node focused checks and generation await restored verified toolchain. Independent scheduler recovery review is assigned by root. Native PPP/VPP, VLAN packet forwarding, reboot and ISP acceptance remain NOT RUN.

Next: run focused Go carrier/PD/scheduler/agent tests, regenerate public contracts with restored tools, resolve actual failures and request final combined review. This is a checkpoint, not a completion or lab-only claim.

## Combined source candidate 87cbddc0

All identified product fixes integrated; final independent review and root final CI remain pending. Carrier includes the reviewed latest broker/resolver assets, route membership context, VLAN guards, PD registration and missing-TAP prerequisite recovery.

Two combined correctness defects were found and fixed:
- WAN route writes had no PPP daemon dependency. Added the optional PPP configuration dependency to the actual WAN route descriptor. Real scheduler + ClientConfig + PppoeRuntime + route descriptor test exercises failed WAN route write rollback, successful join, health-down withdrawal and leave. Negative control with the previous plain route descriptor fails because route CREATE precedes PPP daemon UPDATE; corrected product passes.
- NCP can reconnect inside one persistent process. Forwarding verification now brackets helper/VPP observations with matching NCP generations. Actual runtime test changes generation during verification and fails on the previous source, passes after correction.

Integrated observation-failure cleanup: malformed/unreadable hook state revokes readiness, removes remembered mirrored addresses/defaults and independently withdraws verified kernel forwarding. Failed cleanup retains retry tracking. Native boundaries are fake in these tests; no native acceptance is claimed.

Verification actually executed:
- Focused Go race controls across subsystems, descriptors/pppoe, desired, agent and scheduler PASS (1.479s, 1.089s, 1.313s, 1.313s, 1.066s).
- Combined actual forwarding positive, seven drift scenarios, three in-flight replacement scenarios, NCP verification replacement, WAN transaction and legacy IPv6 mirror controls PASS (subsystems 1.554s).
- Existing IPv6 mirror fixture now supplies the current PD admission identity and future lease deadlines; a bare legacy prefix is correctly no longer accepted by product code.
- Schema PPP-parent/PD/setup: 27 tests PASS; interface UI model: 7 PASS; TS protobuf PD roundtrip: 1 PASS.
- Source generation completed 13/13 tasks via `pnpm exec turbo run gen --env-mode=loose`, preserving the restored toolchain cache variables. Initial strict-env generation failed because Turbo dropped cache environment and network lookup was unavailable; no successful generation was claimed until rerun. Generated API client and YANG changes committed.

Remaining: independent R2/R4 final delta receipts, one final unchanged combined CI campaign and merge by root. Native/lab execution remains NOT RUN. Product license remains an external release input, not a source implementation task. No CI or host activation performed by this worker.

## Final hosted campaign correction, 2026-10-09

The final hosted candidate 69e28859 failed Go lint (62 findings), before Go tests. TS and three fixture workflows passed; root preserved the exact failure log separately. Corrections preserve every enabled gate: exported API documentation, shadowed identifier renames, removal of unused SNMP wrapper, explicit VLAN operation nonnegative bound and PD lifetime upper bound, uint64-only test counter, propagated resolver close errors, tighter test fixture permissions. Six narrowly scoped gosec false-positive comments cover three fixed validated product paths and three private fixture paths/credentials, with per-line rationale; R4 independently reviews these.

Pinned local lint installation initially failed because archive uid/gid ownership cannot map in this environment. Recovery uses tar --no-same-owner only in installer environment, without changing repository scripts or validation. Actual pinned lint result and final test outcomes remain pending at this checkpoint.

### Final correction verification

Frozen product `1d7f3d96` resolves all established source failures from the complete baseline agent run:
- D039 explicit presence for PPP delegation target fields; regenerated Go/TS contracts and API generation completed (13/13 tasks).
- Generic fake wiring injects an explicitly empty PPP inventory. Production nil injection still uses the packaged broker and discovers orphan namespaces; failures never become empty inventory.
- PD summary fixture carries unexpired lease evidence; stale lease rejection remains unchanged.
- SNMP projection selects transaction owner for both checker and sealed fingerprint, preserving fail-closed ambiguity for ownerless callers and refusing foreign fallback.
- BFD unscoped (`all`) ID range no longer dereferences nil; numbered and invalid ranges remain bounded/fail-closed.

Actual pinned golangci-lint **2.13.2** complete run: exit **0**, `0 issues.` Full `go vet ./...`: exit **0**. First local lint pass found six additional fixed-path/test-mode findings; final correction uses nonexecuted fixture content files at 0600 and three additional precise G304 path rationales. No linter configuration, enabled gate, workflow or security check was weakened.

Corrected original contract/PPP globals-owner/carrier/PD summary controls passed race in five packages: contracttest1.054s, descriptors/pppoe1.094s, subsystems1.507s, agent2.070s, renderers/pppoe1.068s. Final original-agent-failure/SNMP/host-service controls PASS3.140s; final carrier/PD/BFD/IPv6 subsystem controls PASS1.846s. TS protobuf PD roundtrip PASS1 test. BFD direct all/invalid/numbered-range regression PASS1.104s.

The unchanged complete baseline Go suite was executed by independent reviewer. Its remaining process/UID/socket failures correlate with this execution environment's PID/proc namespace mismatch, unmapped foreign UID/GID and denied sockets; they are not waived or labeled passing and must pass the unchanged hosted gate. Initial broader focused selection also encountered those real process fixture restrictions. Native appliance/ISP execution remains NOT RUN. Final hosted campaign rerun and merge remain manager-owned, pending final R2/R4 receipts.

## Final integrated source receipt — 2026-10-09

Source candidate `30de26ee6a5697a3713fe4375399b86c5588a472` passed the unchanged complete hosted quick gate
[37903333143](https://github.com/mcoder1001-cyber/NGFW/actions/runs/37903333143) and all applicable fixture workflows.
[PR214](https://github.com/mcoder1001-cyber/NGFW/pull/214) merged as `d58db1a673d716ebcf595a9e49847a54991c58da`; merge tree equals tested tree
`d1de8b7c01220ce03b311d0a7dcafd5fe12ab1ad`. Reviewed history is preserved at
`codex/archive-completion-corrected-20261009` (`3f98838119ed8d67aab9c226308829ac49af9def`).
Final R2/R4 correction receipts approve the scoped source; initial failed CI and
local environment failures remain historical evidence, not retroactive PASS.

| Final check | Result |
|---|---|
| Mandatory quick | PASS, complete unchanged hosted gate |
| [Packaging37903333245](https://github.com/mcoder1001-cyber/NGFW/actions/runs/37903333245) | 81 fixtures, zero failures/errors/skips; seven gate-policy controls PASS |
| [Provisioning37903333184](https://github.com/mcoder1001-cyber/NGFW/actions/runs/37903333184) | 46 strict +23 offline Debian +11 trusted installer +18 portable export PASS |
| [Python37903333309](https://github.com/mcoder1001-cyber/NGFW/actions/runs/37903333309) | 14 synthetic-wheel +5 release-contract tests PASS |

Board: **205 merged,7 parked,0 review,0 running**, total212. The eight reviewed
source rows are merged. PPP-host and MultiWAN-host source is integrated; their
native acceptance joins the five already parked rows. No live source worker is
claimed. All older pending-source/CI statements in this document are historical
and superseded by this receipt.

Native acceptance of this cumulative source remains **NOT RUN**. Known prior RA
supplier/post-ACK identity failures and P12 mgmtd startup failure at the unchanged
30-second deadline before the200-route proof still require diagnosis, any necessary
fix and rerun on the real target. No unit/fixture result closes these cases.
Product license text/name/copyright authority remains a separate release input
under `docs/decisions/PENDING-P10-product-license.md`; no license is invented.
Plan exclusions remain unchanged. This is source completion, not release certification.
