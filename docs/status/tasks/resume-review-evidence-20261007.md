# R7 independent evidence review, 2026-10-07

Verdict: APPROVE the bounded documentation/board candidate `c0922dcd9da509c7ca5b14f9121fb0ed9b6d75aa` on `codex/resume-manager-20261007`, compared read-only with main `0ec397e327123cadfd5d278a9a1cda37532fdc2c`. This is R7 approval, not security approval or the final integration gate. Recheck the final single-commit candidate and unchanged complete hosted quick before sequential merge. No product, manager worktree, main or board changes were made by this review.

## Evidence and scope

Read AGENTS.md, prompts/00-CONTEXT.md, docs/contributing.md, docs/decisions/decision-policy.md, prompts/REVIEW-PROMPT.md and prompts/reviewers/R7-docs-evidence.md. Owner's durable GitHub workflow supersedes historical local-only instructions. Review owns only this report and its envelope/WIP. Live developer inventory is unverifiable independently here; manager WIP lists dispatched workers, which is not independently observed process evidence.

Fetched origin and queried GitHub PR179/180/181/195 and main workflow state. PR195 is MERGED; run `37583587580` has `status=completed`, `conclusion=success`, head SHA `0ec397e327123cadfd5d278a9a1cda37532fdc2c`, job `Mandatory quick gate=success`. This was observed through `gh run view`, not rerun by R7. No large build or live lab test was run.

## PR179 and exact adjusted candidate

Old PR179 head `883978a24df19f6d9d18854955372937dadafab7` contains stale source/gate-pending text. Its historical green check does not certify the current candidate. The adjusted manager candidate changes six documentation/board files only and closes F-srv6-host, F-mpls-srmpls-host, and LAB-vpp-per-slot with explicit boundaries. Its counts are correct: 205 merged, 1 running, 6 parked, 212 total; 1551.0/1578.5 planned hours.

PR177 integration `492c0156e` is an ancestor of main (`git merge-base --is-ancestor` exit 0). Original campaign11 receipts and native13 manifest are tracked on main under `docs/status/tasks/closeout-mpls-srv6-browser-live-evidence/`. Parsed native-host/manifest.json: six groups all exit 0; 13 named host tests outcome pass, including TestMplsOnHost, TestSrv6OnHost and TestSrv6GlobalsOnHost. Native source is `64fffb9dda441f169834ecb1dfc06865a1c6d37d`, not a new current-appliance execution.

Parsed final/cleanup.json: actualExit 0; 16 browser cases, zero browser exceptions/5xx; baseline rollback applied with native/config absence assertions; owned DB/role absent, remaining Valkey keys 0; shared VPP MainPID1014/NRestarts0. final/actual.log contains DATAPLANE_REAL_API_ACCEPTANCE=PASS, BROWSER_MPLS_SRV6_CONFIGURED_EN_FA=PASS routes=16, MPLS_SRV6_REAL_API_LIFECYCLE=PASS, MANAGEMENT_REAL_API_ACCEPTANCE=PASS. Artifact-before/after lists are exactly equal with 1049 entries. These are historical recorded results, not tests rerun by R7.

native-host/integration-source-provenance.json attributes campaign11, 13 native passes, frozen HTTP service source64ff and agent/web sourceff5333; selected original-source comparison covers 65 agent, 8 API, 18 UI files. It expressly limits mixed components, zero SR packet counters and unresolved earlier504. Thus bounded original H1 closure is justified; packet/performance/whole-appliance/fresh-machine certification is not.

Read attempt7/actual.log: status504 Agent timeout, 15.000s deadline. attempt9/actual.log: status504 Outcome unknown, 18.187s deadline; both assert expected200 and fail. They remain failures. The later committed `claude-autoblock-noop-wip.md` records 13 attempts, zero HTTP504, four failures (health connection1/2/7, browser evaluation4), nine successful attempts, and launcher cache warmup11–13. Its tested source774b82548 predates current integration. It supports the attributed bounded observation, not all13-PASS or universal reliability repair.

Evidence limitation: `claude-autoblock-noop-evidence/` on main contains four launcher/helper scripts only; raw per-attempt replay logs are not tracked there. R7 inspected the committed summary and cannot independently recount the reported zero504 from raw logs. Candidate honestly says that the WIP records this result. Preserve that attribution; optionally publish original replay logs before stronger claims. Original campaign11/native13 raw evidence is present.

P12/OSPF peer inventory and fresh acceptance remain separate. PR180 head `661cbc21bf4bead10abb93333f493406f4142db1` remains OPEN/draft with unknown mgmtd startup failure at unchanged30s deadline before route proof. No bounded host-row closure waives that real failure or certifies the unrelated cleanup runner.

## PR181 supersession

PR181 head `4d85327789db65669e4de63a16f1616c2ca114ed` is OPEN. Exact production and regression-test files are byte-identical to main after PR195:

| file under apps/agent/internal/agent | Git blob (both refs) | SHA256 (both refs) |
|---|---|---|
| rpc_autoblock.go | bfec514824e7d3adc2ae833ce3773434aaa62a49 | 1b251f17fd162727f25e34b9ff33adcd86f938438f6bf30c18cdb720c2179445 |
| rpc_autoblock_test.go | 8ae3d1833e41f319efe65435279c318265c4a74b | ce3a34ef78f273b3a286a79287229c8bbae07f3fb44971ef2fceba55a5f28614 |

Actual command `git diff --name-only origin/codex/autoblock-inactive-publish-contention -- <two paths>` produced no output. `git show <ref>:<path> | sha256sum` produced the above digests for both refs. Manager should close PR181 as superseded source by195, preserving its remote/archive reviewed evidence; do not merge its entire older dependency tree. P12 proof remains180, not superseded by AutoBlock integration.

## Recovery branch ancestry and original delta

Recovery head `c6e4fb1c3` has two original commits after merge-base `980d54be574d3fbfa3442080d92dd7940b8d7998`: `9528824ca` and `c6e4fb1c3`. Original delta contains173 paths. Main contains squash integration `121c09747a3d2f9fa85a6b845f251d60a3df261a` (PR146), which is an ancestor of current main (exit0).

`git diff --stat origin/codex/recover-eight-review-20261004 121c09747` returned empty. Both whole-tree IDs are `327bc41ab9205bf7d533e2c7be7898486baeca97`. Therefore every original recovered source/doc delta is already integrated, with no genuinely unmerged delta identified. Differences from today's main are later development, not missing recovery work. Retain archive; mark branch superseded/integrated by146, do not merge old HEAD.

## LAB and TD19 manager actions

APPROVE LAB-vpp-per-slot source state merged by195. Main has actual slot20 up receipt (owned sockets, unit active, CLI version, rc0) and down receipt (unit not-found/inactive, runtime removed, zero owned shm, fallback shared env, shared MainPID1014/NRestarts0). Keep default cap2 developer instances plus CI, MemoryMax1G, no shared-host change; full CI-slot opt-in and remaining harness migration are not proven complete. Candidate preserves these limits. Historical WIP's hugepage counts vary across a busy shared host; do not infer a global constant count.

TD19 may remain parked for real target installation/provisioning/boot acceptance, NOTRUN, but parked_on=PENDING-TD19-repository-trust is stale. Decision238 explicitly records owner choice and exact anchors, implemented195. Manager should change parked reason to target acceptance/prerequisites, replace unanswered-trust/source-gap wording in board worker_status/completion_evidence and PROGRESS, and update DEFERRED-ACCEPTANCE's obsolete missing-pin claim. Do not mark actual target installation PASS. This adjacent pre-existing stale row is not changed or newly certified by the three-row candidate.

DEFERRED-ACCEPTANCE's blanket remaining-host NOTRUN list also predates original evidence. Manager should narrow MPLS/SRv6 entries to broader outstanding criteria and link the historical closeouts; do not erase real failure records or convert P12 failures to lab-only deferrals. MINOR follow-up; candidate closeout docs already clearly limit the certification.

## Actual lightweight checks and receipt digests

Commands ran in this review worktree:

```text
git diff --check origin/main..codex/resume-manager-20261007
(no output; exit0)
tools/ci.sh check --base origin/main
no contract files changed in the 0 commit(s) of HEAD since origin/main (0ec397e32)
board valid: 212 tasks; read-only validation
ok: 30 developer slots + CI slot 12; 964 ports, 32 id ranges, no collision; tools/lab env verified for 31 slots
check PASSED (0m14s)
```

The check above ran on the review base main, not on another worktree's candidate. Candidate YAML was separately parsed read-only from git show to verify states/hours. No full candidate quick PASS is claimed. An exploratory `board.py --help` unexpectedly regenerated own-tree PROGRESS (tool does not implement help); restored that incidental file exactly, with no board edits retained.

SHA256 of inspected main receipts:

```text
6f409f9dc45140d8f00d98911f9225f6a62be97a37e481f50ca629206ebd5fab native-host/manifest.json
2f44e38f24f64092bb42a56584876ad5db1665dd7d3cee2665564ae86af1db84 final/actual.log
bb8704a29bd80b728a0c71ea9f2dc57481933185c24edb432144986e3e65447b claude-autoblock-noop-wip.md
78cd985b74f83f941bd88ba7f5b18e933a2a4381c89def86992b289072ce0f5a claude-vpp-per-slot-evidence/01-up.txt
0e661942665793b371e6be0a6cec5d7c5130e5b4cb18b9b68f585f80dfa1cd14 claude-vpp-per-slot-evidence/06-down.txt
```

Final R7 verdict: APPROVE exact c0922dcd candidate for bounded/source row reconciliation, with MINOR adjacent stale-doc followups above; final R2 and current integration hosted checks remain required. Decisions made: none outside requested evidence classification. Open questions: original later-replay raw logs not durably included; no unconditional reliability claim can follow from their summary.
