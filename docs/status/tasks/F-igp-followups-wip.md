# F-igp-followups recovery

Branch/worktree: codex/igp-followups-20261004 /root/ngfw-wt/igp-followups-20261004.
Base local/remote: d5557440c. No checkpoint published yet.
Owned files: see F-igp-followups.envelope.md.
Completed audit: historical OSPFv3/auth/Event20/state/UI, IS-IS families/passwords/Event21/state/UI, RIPng/version, VRRP RPC/events and D9.2 sync already integrated in origin/main. RIP interface authentication missing.
Current change: additive schema/proto RIPv2 interface auth; RIPng schema rejects auth.
Tests: not yet run. Dependencies installing.
Remaining: generate/commit/publish contract; FRR MD5 chain/interface render and tests; documentation; full quick CI and independent review.
Production authentication secret delivery remains PENDING-secret-channel; neither live MD5 nor laboratory packet acceptance claimed.
Exact next command: pnpm gen

Checkpoint: contract local a0f15168a. CLI push attempted immediately and rejected HTTP403; GitHub connector publication in progress, no remote success claimed yet.
Generated through pnpm gen successfully, including API client; schema auth tests8/8 PASS.
RIP MD5 key-chain/interface renderer, redaction/removal/fail-closed tests and RIPng rejection implemented. Targeted go tests rip, ripng and contracttest PASS using task-private TMPDIR (global /tmp inode exhaustion prevented initial compilation).
Shared RipInterface carries auth only for v2; drift guard documents the deliberately unsupported RIPng superset alongside existing protocol-specific wire supersets. Schema and renderer both reject RIPng auth.
Full unchanged quick gate running with private TMPDIR and bounded concurrency; next command: tail -50 /root/ngfw-wt/igp-quick.log.

Durable connector checkpoints: contract remote c0f31ba39865f0f4284fc11ca78fe9c27eaed599; consumers/form remote 26db6989b731471fa4e8d7dd0ebeb0b022916395. Both GitHub ref updates confirmed; CLI403 was bypassed with authorized GitHub connector. Local consumer HEAD007e3fa96 has equivalent published tree.
Independent root review: APPROVE RIP contract/renderer/tests, contingent complete quick gate.
Found and fixed additional real gap: sealed FRR cache already wired, but API selected IS-IS passwords only. OSPF/RIP MD5 interface references now select password secrets through existing channel; non-MD5/RIPng excluded, foreign kind rejected before DB. API secret-delivery18/18 PASS. No privilege/channel boundary changed and live authenticated routing is not claimed.
First quick rejected new schema tests under strict optional indexing; fixed by optional access in007e3fa96. Repeated quick running; manager identified baseline schema defaults and product-text failures and is fixing prerequisite branch. Next: commit/publish API delivery and docs, wait gate, rebase onto manager prerequisite once merged then rerun full quick.

2026-10-04 final source handoff: local13cfbbb0f, remote d57d46d2d19febc0237196ae23616e89888cd715 (verified ref update), PR164. Root second review of API selection APPROVE; independent wan_pppoe security review APPROVE (exact decoded path, MD5-only, existing password/CA-key restrictions; no plaintext exposure). Targeted Go -race -count=1 three packages PASS; schema8/8, API18/18, web2/2 PASS.
Actual repeated full quick has reached Turbo; generation clean and forbidden/secret checks passed. Existing group-a.test.ts448 failure reproduced (new main IS-IS family defaults omitted from old expected object); baseline fix in manager PR162. Current ongoing session80159, log /root/ngfw-wt/igp-quick.log, step logs /root/ngfw-wt/logs/ci/igp-followups-20261004-20261004-190210-3722677. No quickPASS claimed. API source edit landed while gate was running; mandatory final integration gate must rerun after prerequisite/rebase.
Exact next command: git fetch origin; then, once manager prerequisite merged and current gate finished, archive reviewed refs and squash/rebase under D112, rerun TMPDIR=/root/ngfw-wt/igp-tmp NGFW_CI_TASK_CONCURRENCY=2 GOMAXPROCS=2 GOFLAGS=-p=2 tools/ci.sh --base origin/main.

Manager-directed handoff: original full gate process tree3722677 stopped after observing known main schema failure (avoids superseded unrelated tests). No active own CI worker remains; final manager integration gate after baseline correction is mandatory. Root and wan_pppoe independently APPROVE actual API routing secret-selection delta. Remote final source checkpoint c248d63b2dde89e211d1bd8363004cb2d48c2bc4 confirmed. Local31198bd13 equivalent product tree. PR164 attach_artifact request issued but still awaiting connector completion; no attachment success claimed.

D112 integration preparation: manager prerequisite PR162 merged into current main f8fcd6c2fc8cfddfe8c34397681acec44724225d. Integrated current main, resolving only root WIP add/add with main version. Reviewed history preserved local refs/archive/F-igp-followups-reviewed-20261004 (9cea1ccdc) and confirmed remote archive/igp-followups-reviewed-20261004 (6d8120ef9f7f9b31aa4a67e7d96bdb1ce9ae6f3a). Final branch squashed to one contract commit above that main. Next: publish exact tree, verify fetched tree equality, unchanged complete hosted quick. No gate/merge success claimed before actual completion.

Preflight hosted gate completed PASS: final c6f9eb0ea39b4b47665af53207828714928f28ed, mandatory complete quick run37219102134 (job111485609937 and repository-gate step success). Preserved this full-green head in local refs/archive/F-igp-followups-full-green-20261004 and confirmed remote archive/igp-followups-full-green-20261004. Manager PR167 independently reviewed precise public-digest exceptions, complete quick37219414200PASS, merged main38fa5d4e5d98ad8bc6ae6dcf0dc19d8df3becab4. Source unchanged; final integration now uses that exact main+one contract commit. Superseded local f8-base gate stopped per manager after full hosted success; it had passed TS35/35, Go vet/lint and was still executing race tests. Mandatory hosted quick on the new exact integration head is next; no new-head success claimed yet.
