# F-notifications recovery checkpoint — 2026-10-02

## Current status — 2026-10-02

**Bounded increment, not full F-notifications acceptance.** Reviewed product local integration `df35285ca5125cd8493a9f19ebd5aa3196860189`, tree `d08fd684ef890d25f0cb7edbe05282cd81809874`; manager-reported published PR [#66](https://github.com/mcoder1001-cyber/NGFW/pull/66) checkpoint `2cf55678` has hosted quick running. This documentation-only correction branches from that product; it does not change code or claim a green gate.

[R6 verification](notifications-resume-review-R6-verify.md) and [R8 verification](F-notifications-review-R8-verify.md) approve the corrected UI, recovery and socket cancellation. Their original BLOCK reports remain preserved. [R3 contract verification](notifications-resume-review-R3-verify.md), [R4](notifications-resume-review-R4.md) and [R5](notifications-resume-review-R5.md) reports are now retained alongside [R1](notifications-resume-review-R1-verify.md) and [R2](notifications-resume-review-R2.md). The [R7 audit](notifications-resume-review-R7.md) identified central debt/decision gaps; this correction addresses them through D-175, the [notification debt row](../../tech-debt.md#f-notifications-bounded-increment-follow-ups-2026-10-02), and the [central campaign](../DEFERRED-ACCEPTANCE.md). R7 documentation recheck remains pending.

Nondefault management-VRF binding and the IPsec notification adapter are **unfinished code**; the strongSwan producer exists. Successful real local SMTP/HTTPS receiver tests and browser acceptance are **unfinished host-independent tests**. The R8 actual TLS fixture verifies cancellation, not successful message delivery. Real routing, database/API restart and commit/rollback acceptance remain **NOT RUN**. Queue/history are bounded and process-local; API restart loses them. Full task status remains running.

Next: manager integrates this documentation-only change, obtains R7 recheck and verifies the unchanged hosted quick gate on the exact final PR tree before any merge. Local equivalent: `tools/ci.sh --base origin/main` with the required tools and environment. No additional broad local gate ran for this docs-only correction.

## Historical recovery evidence (superseded status retained)

Branch: `codex/notifications-resume-20261002`; base contract remote `dd820ce3`.
Owned files: notification API feature, Management notification tab/locales, notification-only app wiring, desired/drift projection exclusions, sanitized block-list failure bus adapter, generated outputs from `pnpm gen`, dependency lockfile.

Rebuilt SMTP (nodemailer, MIT, required STARTTLS/TLS) and signed HTTPS webhook transport, pinned DNS and public webhook destination filtering; no Telegram. Bounded queue 256, history 500, single dispatcher, maximum three attempts with backoff, throttling/dedup, queued-event configuration revalidation, admin-only bounded test action, latest-only singleflight reload. API-only notification configuration is omitted from agent desired/drift projection. UI uses the root schema and candidate transaction; send-test uses running channels.

Checkpoint is unfinished, not acceptance PASS. Complete `pnpm gen` running with pinned restored tools; focused dispatcher/SSRF regression tests running. Next: inspect `/tmp/notifications-gen.log` and `/tmp/notifications-tests.log`, fix actual failures, run schema/API/web typecheck/lint/tests, publish reviewable PR with unchanged full hosted quick gate. Independent security/contracts/correctness/UI review still required.

Lab/browser SMTP/HTTPS reception, management routing/VRF, real database restart and full commit/rollback acceptance NOT RUN; must be in `docs/status/DEFERRED-ACCEPTANCE.md`. No special management VRF socket binding exists: daemon sends using the API process namespace routing; non-default management VRF remains unimplemented and must not be advertised complete. The strongSwan/IPsec event producer exists; the notification adapter for those events is not implemented. WireGuard events are wired.

## Resumed checkpoint

Rebased onto origin/main `53a43ce5` (CI generation DAG fix). Preserved original worktree. Fixed SafeParamPipe instantiation for test-channel action. 29 dispatcher/address regression tests passed locally. `pnpm gen` running; generated API client must be committed after successful regeneration. Full unchanged quick gate and independent review remain. Publication attempted but automatic approval review rejected GitHub push because current root message is a dot; no remote SHA claimed and no workaround attempted. Manager informed.

Exact next command: `source ../toolchain/env.sh && tools/ci.sh --base origin/main`.

## SMTP security correction

Branch `codex/notifications-address-fix-20261002`, isolated worktree `NGFW-notifications-address-fix`, parent product `023fa022`. Independent BLOCK reproduced mapped/expanded loopback and link-local SMTP bypass. SMTP now parses addresses with shared schema bigint parsers, checks mapped IPv4 policy and full IPv6 fe80::/10, and rejects unspecified/loopback/link-local/multicast before nodemailer creation. Private IPv4 and ULA SMTP relays remain allowed; all DNS answers checked and first approved address pinned.

Focused Vitest: 53 tests passed (29 dispatcher, 24 actual sendNotification destination regressions with mocked DNS/nodemailer and schema-parsed channels). Initial collection failed because fresh worktree lacked schema/proto build outputs; building those packages resolved it. No live network or full gate claimed; remote publication still blocked, no retry. Independent re-review and new full integration gate remain.

Next: review this correction commit, then run unchanged complete quick gate on final integration tree.

## Current recovery, 2026-10-02

Owned branch `codex/notifications-resume-20261002`, worktree `/workspace/scratch/de92de7d9874/NGFW-notifications`, based on main `53a43ce5`. Recovered original product commits through old `4e7e03d2` into local `aeefa86c`; historical security approval applies to that original product only. Old worktrees were read only.

Fixed an additional throttle bypass: configuration reload pruned per-channel send-test throttles; retain entries for existing channels across reload. Regression exercises test → reload → rejection → expiry.

Actual focused command: `pnpm --filter @ngfw/api exec vitest run src/features/notifications/notifications.test.ts src/features/notifications/transport.test.ts`: **2 files / 54 tests passed**, 9.63 s. Full unchanged `tools/ci.sh --base origin/main` running with pinned tools and fresh writable caches; generated-output and forbidden-pattern gates passed so far. Ordinary sandbox dependency install blocked network; authorized escalated gate installed 597 packages and continued. No gate PASS claimed yet.

Publication: CLI push failed at proxy connection. Parent requested outbound publication pause pending parent review resolution; **no new remote SHA claimed**. Next command: inspect `/tmp/notifications-resume-gate-escalated.log`, fix real failures, then publish/check full gate and independent review.

Remaining product limitations: non-default management VRF binding and IPsec event mapping are unimplemented (not lab tests). Real SMTP/HTTPS fixture reception and browser acceptance still need host-independent tests; lab-only routing/restart/commit acceptance remains NOT RUN. This checkpoint is reviewable development, not complete feature acceptance.

## Final local review checkpoint

Product SHA `f57bf3940b72a351bb7ed9c68b5250bc80882151`. Fixed inherited schema/API/UI lint errors without relaxing lint, regenerated outputs via `pnpm gen` (13/13 tasks successful), added five schema CR/LF/NUL/SMTP/Telegram regressions, and restricted notification SchemaForm editing to administrators to match server policy. Operator UI regression passed; final ManagementPage suite **7/7 passed**, 6.11 s. Earlier notification dispatcher/transport **54/54 passed**, schema regressions **5/5 passed**. Final `tools/ci.sh check --base origin/main`: **check PASSED (0m02s)**.

Full unchanged quick attempt failed: three inherited lint issues subsequently fixed; licensing CLI 4 failures from this environment's `/tmp/.git` ancestor key-generation protection; web runner `ERR_IPC_CHANNEL_CLOSED` during concurrent full gates. 30/35 Turbo tasks passed. No quick gate PASS claimed. Parent requested hosted unchanged full gate instead of another resource-competing broad local run. Full log `/tmp/notifications-resume-gate-escalated.log`; step logs `/tmp/vrx-ci/NGFW-notifications-20261002-173659-266464`.

Independent R2 APPROVE at `a0b6e34b` is committed in `notifications-resume-review-R2.md`; subsequent UI admin restriction and test are being reviewed under R1. Full feature acceptance remains unfinished: see explicit VRF/IPsec/product-test limitations above.

Publication authorization received directly by parent, but one 6.1 MB create_tree connector attempt stalled for over 20 minutes and was interrupted; no remote success SHA returned, no commit/ref/PR created by this worker. Parent owns publication using manifest, final rebase onto fresh main, unchanged hosted quick, and sequential merge decision. Exact next action: publish frozen task tree and open reviewable PR through parent, verify hosted quick on integration tree.

Cleanly rebased onto current main `31355cef80b4e8ba3aaa6f95a47c7e7afc055fb6`; notification product paths are byte-identical to the pre-rebase reviewed checkpoint. Rebased `tools/ci.sh check --base origin/main` passed (0m02s); full quick still requires a fresh successful hosted run before merge. Parent publication metadata is `/tmp/notifications-upload-manifest.json`, containing file paths/blob hashes and remote-reuse evidence rather than embedded file contents.

## Independent review corrections (R6/R8)

R6 correction `0db184e3`: nested en/fa schema labels and enum maps now match actual channel/rule paths; server validation pointers are relative to the notification form; delivery timestamps/counts use locale formatters; history has translated headers and empty state. Targeted ManagementPage suite **9/9 passed**, including populated Persian SMTP/rule rendering, operator permissions, and field-level API error association.

R8 correction `70cc4961`: successful configuration reload clears degraded state before scheduling queued delivery, preventing a recovered worker from remaining idle. SMTP uses an explicitly owned TCP socket destroyed on cancellation (nodemailer non-pooled transport.close alone did not terminate active connections). TLS/STARTTLS validation remains mandatory with original server name and pinned destination. Notification tests **57/57 passed**, including read-failure recovery and abort both before/after socket connect. Independent re-reviews pending.

No additional full local gate run. Parent integrates these corrections onto the published PR tree and requires the unchanged hosted quick gate. IPsec notification adapter is a missing consumer, not an absent strongSwan producer.
