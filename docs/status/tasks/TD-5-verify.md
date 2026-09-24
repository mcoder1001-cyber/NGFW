# TD-5 verify: fix round 1 (focused, per D-115)

Verifier on branch `task/TD-5` @ `f63c1e3`, code commit `5c1148f`, fix diff `7f90854..f63c1e3`, 2026-09-24.
I read REVIEW-PROMPT, the verify envelope, TD-5-review.md (H1, M1, L1–L3, N1–N5), TD-5.md "Fix round 1", TD-5-questions.md F1–F3,
and LOG D-113 and D-115. The scope is limited to whether each finding is fixed and whether anything regressed. This is not a new full review.
I did no host run. The negative controls ran through `go test -overlay` from my scratchpad, and `git status` stayed clean.

## Per finding

| Finding | Status | Evidence |
|---|---|---|
| **H1** TX frame overrun | **Fixed** | `host_interface.go:37-38` sets `TxFrameSize = 2048*33` (67584) and `TxFramesPerBlock = 16`, and RX stays 2048 × 8. `host_interface_test.go:108` asserts all four values plus the queue counts. `:111` checks that the TX block (1,081,344 B) is a page multiple and ≤ 2 MiB. `af_packet.md:59` ("Rings (D-108, amended by D-113)") explains why the frame size must not shrink: `device.c:561-573` copies with no length check, GSO/jumbo frames reach that path, and an overrun of the last slot writes past the mmap. The code matches `tools/lab:335` (`tx-size 67584 tx-per-block 16 rx-size 2048 rx-per-block 8`) and D-113. |
| **M1** unbroken run above a hole capped at 17 | **Fixed** | `sanitize.go:336-344` is the reviewer's fix, applied verbatim. The success condition at `:346` is unchanged and the cap is still ≤ 64. `TestAscendingRunAboveAHole` (`sanitize_test.go:416`) passes at the tip: r = 16/17/20/40/55 succeed with exactly the needed count on both runs, and r = 56 is capped at 64 (`ErrCapped`) on both runs. **Negative control (my run):** the same test with the pre-fix `sanitize.go` (`git show 7f90854:…`) swapped in through `-overlay` fails with `r=17 run 1: needed 26, placeholders 17, cap 17, holes seen 1 … capped true` and `dirty "output acl"`, which matches the review. |
| **L2** rollback on the caller's ctx | **Fixed** | `host_interface.go:117-119`: `context.WithTimeout(context.WithoutCancel(ctx), RollbackTimeout)`, with `RollbackTimeout` = 30 s at `:47` (F3). `TestRollbackOutlivesTheCreateContext` (`quiesce_test.go:476`) passes. **Negative control (my run):** with `quiescedDelete(ctx, …)` swapped in through `-overlay`, the test fails with `untagged orphan host-w2-w0 … settle: context canceled`, so the test does catch the regression. |
| **L3** quiesce takes down any netdev | **Fixed** | `quiesce.go:117` refuses an **up** netdev whose kind is not `veth` with `ErrQuiesce` wrapping `ErrNotVeth`. The `:58` message names D-105. An already-down non-veth netdev goes ahead (F2). The kind is read from `IFLA_LINKINFO/IFLA_INFO_KIND` (`quiesce_linux.go:71-103`, NLA flags masked). A missing or truncated attribute reads as `""` and is refused. `TestQuiesceRefusesNonVeth` (none/bond/vlan/tun plus the rollback case) checks that no VPP request is sent and the netdev stays up. `TestParseAnswer` covers the veth, missing-linkinfo and truncated cases. In the host run the log shows `kind=veth` from the real kernel. |
| **L1** failed Delete leaves the netdev down | **Fixed as F1** | `quiesce.go:126` records `downed` only after a successful link-down. `:165` and `:172` restore the link when the quiesce fails after that link-down (confirm/settle) or when `between` (BeforeDelete) fails. In both cases the interface is provably still in VPP. A failed `af_packet_delete` has an unknown outcome: the netdev stays down and `:179` logs a WARN. `TestFailedDeleteRestoresTheLink` (both branches) and `TestSettleCancelled` (link-up after the cancelled settle) pass. |
| **N1** CLI abbreviations | **Fixed** | `guard_test.go:43` uses `(?i)\bdel\w*\s+host-int` on string literals. The planted `cli.go` flags `del host-int` and `DELETE Host-Interface` and does not flag `show host-interface`. |
| **N2** unchecked quiesce passes | **Fixed** | `checkedQuiesce` (`guard_test.go:163`) requires an if-init or an assign followed by `if err != nil { …; return }`. The planted `unchecked.go` (`_, _ =`) and `no_return.go` are flagged. `good.go` and `good2.go` (the product form) are the 2 permitted sites. |
| **N3** shared netns assumption | **Fixed** | Doc bullet at `af_packet.md:50`, next to CAP_NET_ADMIN. |
| **N4** veth removed under an attached interface | **Fixed** | `integration_test.go:47`: the Cleanup keeps the pair when a Cleanup Delete failed (`deleteFailed`, `:114`). LIFO order puts the descriptor Cleanup before the veth Cleanup. |
| **N5** | n/a | The reviewer accepted it. |

**Guard coverage (no regression):** the `scanV24` walk (`guard_test.go:54-88`) is unchanged. It walks every non-test `.go` file under
`apps/agent` (`agentRoot = "../../.."`) and skips only `binapi/`, top-level `bin/` and dot-dirs. `:235` still requires exactly 1
permitted site. My run: `apps/agent: 0 violations; af_packet_delete is sent at 1 site`.

**Scope and contract:** `git diff --name-only main...task/TD-5` touches only `apps/agent/internal/descriptors/af_packet/`,
`apps/agent/internal/vpp/ifsanitize/` and `docs/`. It has no contract, `binapi/`, `tools/binapi-gen.sh` or go.mod/go.sum paths.

## Test results (my runs, HEAD f63c1e3)
```
$ cd apps/agent && go test -count=1 ./internal/descriptors/af_packet/ ./internal/vpp/ifsanitize/
ok  	ngfw/agent/internal/descriptors/af_packet	0.920s
ok  	ngfw/agent/internal/vpp/ifsanitize	1.469s
$ go test -count=1 -v ./internal/descriptors/af_packet/     # all PASS (TestHostInterfaceOnHost SKIP), matches the TD-5.md paste test for test
$ go test -count=1 -overlay <pre-fix sanitize.go> -run TestAscendingRunAboveAHole ./internal/vpp/ifsanitize/   → FAIL at r=17 (expected)
$ go test -count=1 -overlay <rollback on caller ctx> -run TestRollbackOutlivesTheCreateContext ./internal/descriptors/af_packet/ → FAIL (expected)
$ go vet (both packages) && gofmt -l (both dirs)   → clean
```

## Non-blocking observation
- O1 `quiesce.go:120-124`: if `SetDown` times out (2 s) after the kernel has already applied it, `downed` is still 0. No restore
  happens, the interface stays in VPP, and the netdev may be down. Calling `SetUp` there would do no harm, because the netdev read
  up at the lookup. This case is outside the wording of F1, is very unlikely, and needs no change for merge.

**APPROVE**
