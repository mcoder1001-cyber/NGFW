# Frozen PR196 independent review — R2 / R3 / R4 / R8

## Final frozen-source regrade (2026-10-07)

Source commit `4bcf9f4977e2a9c248548de23cf552e4bcef94da`, exact tree `7d914836662eb129ada7ac33459b97d4ce449b28`, verified with git show. Clean existing branch resumed; freeze merged only in assigned reviewer worktree. No product edits. Detailed final findings are in `review-pppoe-security-20261007-review-R2.md`, `-review-R3.md`, `-review-R4.md`, `-review-R8.md`; these supersede historical provisional findings below.

| Aspect | Frozen verdict | Remaining finding |
|---|---|---|
| R2 | APPROVE | prior PID signalling/path findings resolved |
| R3 | APPROVE | contract record and generated provenance verified |
| R4 | BLOCK | new-generation admission during config transition; unpinned pppd parent identity |
| R8 | BLOCK | same transition/recovery and parent-liveness defects |

Combined verdict: BLOCK. Developer must fence the complete stop/invalidate/replace/restart transition against hook admission and pin original pppd identity. Add deterministic transition, reconnect, failure/rollback and reused-parent regressions. This is implementable owner-scoped code work; no new host privilege or human authorization is required. Product discovery/transit failures remain explicit and cannot be classified as deferred lab-only acceptance.

### Immediate reproduction and A2 handoff

Owner steering: preserve source failures; manager repairs in its own isolated tree; third review round requires A2 arbitration and a fresh independent reviewer assigned by manager after P12. This reviewer retains review-only role; no further repair/review loop or human blocker inferred.

Admission reproduction **specified from source, not independently executed end to end**: (1) create an active private fixture session; (2) pause Go Apply/Remove/Reconnect just after StopIPv6 returns and before state removal/helper replacement; (3) execute old session's IPv6-up hook while pppd is still alive; (4) verify fresh identity/generation is admitted after the stop action lock has ended; (5) resume invalidation/replacement and check writer identity, late PD events and state. Go transaction exclusion cannot block an external hook. A real successful systemd restart may subsequently kill the unit's descendants; the vulnerable window and failed restart/rollback still need proof. Do not claim this source interleaving was a completed live reproduction.

Mandatory repair criteria: fence hook admission through whole transition (including errors and rollback); verify original pppd session identity with starttime/pidfd; preserve handles if extinction fails; old generation and DHCP event writers cannot publish/delete new state; deterministic interleaving tests for edit/mode/off/removal/credentials/reconnect plus failed apply/rollback; no foreign signals, pattern kills, host changes or privilege widening; complete unchanged hosted quick on final composition. Applicable independent reviewer/A2 verdict remains required.

Exact independently executed parent-reuse probe (read/execute template in memory; no files or processes created):

```python
from pathlib import Path
import contextlib, types
source = Path('apps/agent/internal/renderers/pppoe/templates/ipv6.tmpl').read_text()
source = source.replace('{{if .DefaultRoute}}True{{else}}False{{end}}', 'False')
for label in ['StateDir', 'HostIf', 'IPv6UpHook', 'IPv6Helper', 'IPv6', 'DhcpcdBin', 'DhcpcdConf']:
    source = source.replace('{{quoted .' + label + '}}', '"fixture"')
n = {'__name__': 'review_fixture'}
exec(compile(source, 'frozen-ipv6-helper', 'exec'), n)
class FakePath:
    def __truediv__(self, other): return self
    def is_dir(self): return True
    def exists(self): return True
    def unlink(self, **kw): pass
n.update(SYSCTL=FakePath(), HOOK=FakePath(), PID=FakePath(), PD=FakePath(),
         MODE='slaac', locked=lambda: contextlib.nullcontext(), current=lambda g: True,
         collect=lambda i: [], identity=lambda p: 'REUSED_NEW_STARTTIME', print=lambda *a, **k: None)
observed = []
def publish(phase, *args, **kw):
    observed.append(phase)
    n['stopping'] = True
n['state'] = publish
n['sys'] = types.SimpleNamespace(stdout=types.SimpleNamespace(close=lambda: None))
n['refresh']('original-session', 'ppp0', '', '', '12345')
print('parent PID reused with different starttime: published', observed)
assert observed[0] == 'up'
```

Output: `parent PID reused with different starttime: published ['up', 'down']`. This establishes the code accepts any non-null identity; it does not claim actual kernel PID reuse was forced. Mandatory repaired regression must retain the original parent's identity and reject the replacement.

Duplicate local quick stopped at owner's explicit request after coherent partial evidence: generated clean, all35 workspace tasks, agent vet/lint/race/build and CLI lint/test/build PASS; topology unit modules passed through object-model, gate unfinished. Session62224 exited143 after pidfd TERM only to verified spawned controller PID2325819/start2026-10-07 09:21:26UTC and then-owned child2419253. No complete `CI GATE PASSED` claimed, no check disabled or weakened. Hosted freeze run37601246627 is manager's active authority; final hosted outcome must be verified there.

Actual reviewer command: `GOTOOLCHAIN=local go test -race -count=1 -timeout 180s ./internal/renderers/pppoe ./internal/subsystems -run 'PPPoE|Pppoe|IPv6|ReadSession'` from apps/agent:

```text
ok ngfw/agent/internal/renderers/pppoe 35.617s
ok ngfw/agent/internal/subsystems 2.041s
```

This executes real rendered Python lifecycle with private sysctl/ip/dhcpcd substitutes, including reserved/foreign/stale PID refusal, TERM-resistant child shutdown, late event rejection, paused collection revocation, link/hook/parent loss, generation replacement, mode/credential/removal edits, surfaced failures and reconnect exclusion. No live systemctl, host sysctl or VPP changes. No end-to-end rollback acceptance claimed. An additional in-memory execution of frozen refresh with a different non-null parent starttime printed `parent PID reused with different starttime: published ['up', 'down']`; the fixture forces stop after first up. Source does not capture the original parent starttime. First probe attempt had a template-substitution NameError; corrected probe exited 0.

`tools/ci.sh check --base origin/main`: `check PASSED (0m13s)`, contract commit recognized, gitleaks ~696283 bytes/no leaks, 212-task board validation read-only, slot/trace/classify guards passed. Complete `tools/ci.sh quick --base origin/main` currently running; generation reported all generated directories clean and workspace reported `35 successful, 35 total`, now in agent lint/test/build. Final receipt will be appended before handoff. Product diff against frozen source is empty. Lab execution not performed.

PR196 is still OPEN at historical head `992b264b2084a8adfcee755d2b5650b771a6a8f1`, with historical hosted green. `gh run list --commit 4bcf9f4977e2a9c248548de23cf552e4bcef94da` returned `[]`; no hosted freeze PASS asserted. Integration needs applicable independent panels/tester receipts and unchanged hosted quick on final integration tree.

Publication recovery: CLI push rejected the merge; connector reconstructed the identical tree `ab33282a122c96ec7e0c01b5e0a6ae9bef7e2fa7` with the two original parents and published remote merge `2735bfcef9c2cc5e79d7fbdbf6d33aa5593c46be`, independently confirmed by git ls-remote. Local merge `95da82d1f977efe3bc2622376be8944c80adad41` differs only in commit metadata. Remote reviewed parent history is retained. Final report checkpoint SHA will be verified and reported after publication.

---

## Latest read-only delta verification: checkpoint17de38549

Reviewed developer commit `17de38549129ab742fe18051b629cc93ba1be558` against initial `8fbf4e1491505427f3a0935a03dfd2048f38ff7f`; no developer/product edits or test execution. This update supersedes resolved portions of the initial findings below. Final freeze still pending. All four provisional aspect verdicts remain BLOCK for the specific residuals below; do not keep asking developer to repair already corrected family-state logic.

- **R4-1 resolved by source inspection:** ReadSessionState clears IPCP address/peer/DNS when IPv4 is down and accepts an explicit enabled-family flag; runtime observation and status now pass IPv6Enabled. New tests cover actual IPv4-down hook with IPv6-up and stale IPv6-off observation/status. Developer WIP reports focused/race PASS, not independently run here.
- **R4-2 partially resolved:** refresher now runs as a separate shell, checks PID-file generation ownership, records down on parent/link loss, and preserves replacement files on ownership loss. Its EXIT cleanup now kills and waits for the directly spawned DHCPv6 child, resolving the original unconditional no-wait complaint. Do not report those old code paths as unchanged.
- **R2-1 remains BLOCKER:** hook6.tmpl:26-34 is unchanged; stop still signals arbitrary digit-only PID including zero, without process identity/start-time verification. PID equality used by a writer does not authenticate the process signalled by another hook. Existing root-only directory reduces attacker reach but does not remove stale/reused-PID risk.
- **R4/R8 remaining BLOCKER:** hook6.tmpl:32 still silently continues after its five-second stop budget, while cleanup:70 can wait indefinitely for dhcpcd. TERM-resistant/slow child can outlive the down hook or block refresher cleanup. Require owner-safe verified bounded shutdown and explicit refusal/failure when extinction is not established. Current added lifecycle tests use SLAAC, so they do not exercise DHCPv6 child termination.
- **R4/R8 remaining BLOCKER, generation race:** hook6.tmpl:83-84 tests owns before write, but write:39-53 executes ip/grep/cmp/mv without a synchronized generation fence. Apply still removes state/PID handles before stopping/restarting the old unit (pppoe.go:204-211). An old iteration that passed owns can pause in ip, then publish up after revocation/new state publication. Cleanup:72-74 similarly checks ownership once then writes/removes shared names, allowing a concurrent replacement between check and deletion. New replacement test only changes a PID-file token and observes ordinary scheduling; it does not force the check-to-publication interleaving. Preserve fixes already made; close this narrower race with owner/session serialization or an equivalent proven generation-safe publication protocol, plus a deterministic paused-writer regression. No global host workaround.
- **R8-3 remains MAJOR:** DHCPv6 startup/sysctl/write failures remain silent; child status is not checked while refreshing. New EXIT waiting does not establish successful startup or actionable failure reporting.
- **R3-1/R8-2 remain MAJOR in this checkpoint:** no contract record or debcontrol changes in the exact delta. Manager has requested the contract record and narrowly authorized dependency additions; developer should complete them without seeking duplicate authorization. No broader package rewrite is requested.

Actual lightweight checks: source delta `git diff --check 8fbf4e149 17de38549 -- apps packages deploy scripts` exited0; redacted gitleaks delta scanned1 commit/~12239 bytes, no leaks. Read the new tests and WIP receipts; no claimed independent runtime PASS. Main/current hosted final quick still required. No new privilege, GlobalsOwner widening, VPP registration change or discovery/encapsulation workaround in the inspected repair delta. Genuine discovery/transit product failures remain outside these repaired hook paths and cannot be relabelled lab-only acceptance.

Manager handoff: family-state defect is fixed at source level; residual merge blockers are PID ownership, bounded verified child shutdown, publication/cleanup generation race, hook failure visibility, contract record and DHCPv6 packaging declaration. Receive final freeze to verify exact repairs and regrade each aspect. Only this reviewer report is changed/published.

Envelope: reviewer did not author source. Own only this report; worktree `/root/ngfw-wt/resume-review-security-20261007`, branch `codex/review-pppoe-security-20261007`, created from clean local `codex/resume-pppoe-20261007` at `8fbf4e1491505427f3a0935a03dfd2048f38ff7f`, tree `49455c4ceab75cea221e78f5be583f4076aa1036`. Main comparison `0ec397e327123cadfd5d278a9a1cda37532fdc2c`; original PR196 head `992b264b2084a8adfcee755d2b5650b771a6a8f1`. Read R2/R4 and shared instructions earlier this session; read R3/R8 prompts for this task. No developer tree, product, board, host service, sysctl, namespace or privilege edits. Developer is already repairing withdrawal/refresher paths; findings below describe this exact snapshot, not later work. Final freeze/delta verification remains outstanding.

| Aspect | Provisional verdict | Reason |
|---|---|---|
| R2 security | BLOCK | bare PID-file termination does not prove ownership |
| R3 contract boundary | BLOCK | required contract status record absent |
| R4 lifecycle / globals | BLOCK | stale family state and refresher withdrawal races |
| R8 hooks / operations | BLOCK | incomplete child shutdown, silent failures and undeclared packaged dependency |

## R2 security

**BLOCKER R2-1:** `apps/agent/internal/renderers/pppoe/templates/hook6.tmpl:26-34`. stop accepts any digit-only PID and signals it without verifying session/process identity. Zero passes the filter and means process-group signalling; a stale positive PID can be reused by an unrelated process. This violates kill-only-owned-process rules even though the runtime directory is trusted/root-owned (not claiming a remotely writable PID file). Fix: reject zero/reserved PIDs; keep verifiable session identity/start-time or equivalent owner-scoped supervision; do not signal a foreign/reused process. Test zero, stale/reused identities and normal owned shutdown without touching real host processes. Existing test deliberately registers a plain sleep PID; it proves termination, not ownership validation.

**MINOR R2-2, trusted-path hardening:** `paths.go:76-85`, `templates/hook6.tmpl:13,74`, `templates/dhcp6.tmpl:8`, `templates/dhcpcd.tmpl:8`. Paths.Validate accepts arbitrary absolute paths while `quoted` is daemon quoting, not shell quoting (helpers_template.go escapes only quote/backslash). A custom path containing dollar expansion/backticks is interpreted in shell templates; the dhcpcd script path is unquoted config syntax. Actual ProductPaths and owner-derived slot paths are fixed and safe, so no demonstrated user-input injection in the product path. Restrict custom paths to clean allowed tokens or use syntax-specific quoting; document trusted-only WithPaths. Do not describe C-like quoting as shell injection protection.

Positive checks: resolved password stays in Secret=true 0600 chap/pap files; operational sessions clear Password; IPv6 hook/config files do not include it. Owner/route policy remains fail-closed for ordinary slots; reconnect is globals-owner-only and systemctl uses argv. HostIf validation and network literal filtering prevent shell metacharacter injection in their normal inputs; ReadIPv6 parses addresses/prefixes with netip before mirroring. dhcpcd binary is a fixed absolute path. No new auth/session model, secret channel, capabilities or appliance privilege exception was found or approved. Gitleaks redacted branch scan: 6 commits, ~585419 bytes, no leaks.

## R3 contract boundary

**MAJOR R3-1:** `docs/status/tasks/` (missing PPPoE contract record). Branch has `39abb388f contract(pppoe): ...` and changes packages/schema plus generated YANG, but no matching PPPoE contract status file was found (`rg --files docs/status/tasks` filtered for pppoe/contract returned none). R3 prompt explicitly requires both the contract commit and record. Add a scoped record describing unchanged shapes/defaults, changed MTU validation and generated provenance; document any old stored IPv6/MTU<1280 compatibility impact. No accepted debt row was observed.

No field rename, enum reshaping or proto-number change in inspected delta: existing ipv6 off/slaac/dhcpv6 enum and default off persist; help text changes and IPv6 minimum MTU semantic rule matches renderer. No new route/DTO. This review does not infer authorization for assigning delegated prefixes to LAN; it remains outside scope and needs its own contract/owner decision if proposed. Generation was not run by this reviewer; final generated verification and contract guard remain required. Semantic restriction can reject previously accepted low-MTU IPv6 configurations; document that boundary rather than claim universal stored-config compatibility.

## R4 lifecycle / globals

**BLOCKER R4-1:** `state.go:49-70,80-89` and `subsystems/pppoe_watch.go:114-138`. IPv4 down records still populate local/peer addresses. ReadState upgrades phase to up from IPv6 state unconditionally; watcher then mirrors retained IPv4 along with IPv6. A down IPv4 NCP with live IPv6 can keep/re-add withdrawn IPv4 address/default route. IPv6-off configuration with stale state6 can also become operationally up through ReadState even though observe skips explicit IPv6 parsing. Fix: track family-up independently, clear down-family payload and ignore disabled-family state. Regressions must exercise IPv4-down/IPv6-up and both-down with IPv6 configured off, including withdrawals and no stale re-add. Developer WIP independently lists these failures.

**BLOCKER R4-2:** `subsystems/pppoe.go:204-211`, `supervisor.go:43-51,68-79`, `hook6.tmpl:80-90`. Apply deletes .ipv6.pid before renderer stops/restarts the unit; down hook may lose its only shutdown handle. On config replacement the old loop checks only hook existence, not generation/content; replacement can remain present. On exit due to pppd/interface loss, it removes state6 only if hook disappeared, leaving phase=up otherwise. Fix: stop and verify the owned old generation before deleting state handles/replacing hooks; clear or mark down on every terminal path without letting old generation overwrite new/down state. Cover credential rotation, removal, mode change, reconnect, abrupt pppd/interface loss and up/down overlap.

**BLOCKER R4-3 (also R8 lifecycle):** `hook6.tmpl:30-34,74-87`. Wait is bounded but timeout ignored; TERM trap kills dhcpcd without waiting, then exits. Therefore stop returning does not establish writer/client extinction. Late dhcpcd event can recreate .pd, and an old refresher can publish up after down or collide with the next generation. Fix owner-safe verified termination with child wait and explicit failure when shutdown cannot complete; serialize generation changes. No pattern kill, foreign process kill or global flush as a workaround.

Positive checks: VPP uses existing generated IP/interface/MSS bindings; owner-scoped interface resolution, per-family table checks before mutation, IsMultipath single-path default updates, routes-before-address withdrawal remain. No binapi/VPP C edits. Product globals gate is preserved; ordinary slot runtime writes its own files without systemd supervision. Diagnostic driver/native-host results are expressly non-product and cannot justify enabling GlobalsOwner on a shared slot or copying experimental host capabilities. Security boundary deviations require owner decision under decision-policy #4; none is approved here. Product discovery/encapsulation failures remain real failures, not lab-only deferrals or release PASS.

## R8 hooks / operations

**BLOCKER R8-1:** child shutdown/writer-generation failures in R4-2/R4-3 above independently block operational correctness. Unit membership alone is insufficient proof: hooks can execute through pppd packaging/dispatch paths, and implementation explicitly relies on a PID file. No unit lifecycle run was performed here. Require owned-child/refresher absence plus stable down/removed state, not merely a fake runner recording systemctl stop.

**MAJOR R8-2:** `deploy/debian/ngfw/debian/control:12,30` and `docs/09-os-packages.md:77`. New DHCPv6 runtime requires `/usr/sbin/dhcpcd`, but neither ngfw-agent nor ngfw-meta declares dhcpcd-base. Documentation claiming Ubuntu priority/base-image availability does not guarantee package upgrade/minimal-image installation. Declare appropriate runtime dependency or demonstrate an enforced image/package prerequisite and explicit optional-feature refusal. Existing ppp/pppoe omissions are related baseline packaging debt; do not broaden this review into an unrequested packaging rewrite.

**MAJOR R8-3:** `hook6.tmpl:61-74,89,96`. Hook uses set -u without explicit error handling; sysctl writes may fail, DHCPv6 exec failure is redirected to /dev/null, and hook reports exit0 while state is up. Refresher continues even when client launch failed. Missing binary/permission/full-disk failures can leave the UI reporting an active session with no DHCPv6 enforcement and no actionable reason. Surface owned session failure without secrets, fail refused prerequisites, and distinguish IPv6CP-up from DHCPv6 client failure. Tests should cover missing client, failed state writes and child exit. Network event values use a restrictive alphabet; invalid numeric prefix length is subsequently rejected by netip reader, but stale prior prefix behavior should be tested.

## Actual checks / recovery

Static source/evidence audit only; no product edits or live acceptance. `git diff --check origin/main...HEAD -- apps packages deploy scripts`: exit0. Full diff-check reports whitespace errors inside preserved raw driver patch/probe evidence; retain raw receipts rather than silently reformatting them. `gitleaks git --redact --no-banner --config .github/gitleaks.toml --log-opts='origin/main..HEAD' .`: exit0, no leaks. Packaging dependency and contract-record searches described above were actually run. No heavy tests, generation, shellcheck of rendered hooks or mandatory quick executed; do not interpret source inspection as a test PASS. Original PR196 hosted/local gates and diagnostic lifecycle results are historical, not validation of fixes/final composition.

Completed reviewer work: provisional four-aspect audit at snapshot8fbf4e149. Remaining: receive final frozen source/tree, inspect exact delta and actual targeted regression receipts; regrade each aspect and verify unchanged required final gate separately. Current merge failure: above unresolved findings, not lack of an available lab. Publication: commit/push this report only on assigned review branch; exact local/remote report SHA is reported to manager after remote verification. Next command after freeze: `git diff 8fbf4e1491505427f3a0935a03dfd2048f38ff7f <frozen-sha> -- apps/agent/internal/renderers/pppoe apps/agent/internal/subsystems/pppoe* packages/schema deploy/debian docs/status/tasks`, then update this report and publish. No product merge approval before final exact-delta verify.
