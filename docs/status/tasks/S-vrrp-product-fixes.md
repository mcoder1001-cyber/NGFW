# S-vrrp-product-fixes — current implementation, 2026-10-03

The shared bbd9 workspace implements the non-privileged product fixes; the board row is ready for review. Nothing was merged and no shared VPP or management interface was changed.

| Finding | Implementation and evidence |
|---|---|
| F1: accept-mode Master VIP appears as a desired interface address | The VRRP family registers an address classification hook on the core interface-address descriptor. Only this owner's active accept-mode Master VIPs are excluded. A single VR dump serves an address Retrieve; foreign/Backup VRs do not hide configuration and dump failures fail classification. Regression checks the unchanged zero-operation plan and the actual product registry hook. |
| F2: sorted VIPs cause presentation drift | `vrrp.meta` persists the input VIP order. Assemble restores it only when saved and live address sets match, retaining real membership changes. Tests cover reversed order and tampered membership. |
| F3: slot keepalived daemon must already exist | DryRun and Apply check main PID without starting or signaling the daemon. Missing daemon yields `keepalived is not running for this agent; slot harnesses start it`. The regression verifies no config write/reload on failure. README and renderer docs assign slot daemon lifetime to the harness and contain the TD-13 validator line. Product daemon-start privileges remain an explicit owner decision. |
| F4: interface vanished before VR delete | Delete logs the VR key, releases its previous qualified claim when metadata is available and drops its applied record without calling VPP on the dead index. Retrieve skips the orphan with a warning once per identity. VPP pool residue and authorized manual cleanup are documented; fake tests prove claims disappear and no raw delete is attempted. |

Required package checks passed through the heavy semaphore:

```text
go test -race -count=1 ./internal/descriptors/vrrp/... ./internal/descriptors/core/... ./internal/desired/... ./internal/subsystems/... ./internal/renderers/keepalived/...
ok ngfw/agent/internal/descriptors/vrrp  1.149s
ok ngfw/agent/internal/descriptors/core  1.391s
ok ngfw/agent/internal/desired          30.230s
ok ngfw/agent/internal/subsystems       25.976s
ok ngfw/agent/internal/renderers/keepalived 3.573s
```

Additional actual product hook regression passed with race detection in 1.539 seconds. Logs: [own-race.txt](S-vrrp-product-fixes-2026-10-03-evidence/own-race.txt), [product-wiring-race.txt](S-vrrp-product-fixes-2026-10-03-evidence/product-wiring-race.txt).

Private VPP, slot 5, explicit VRRP opt-in and the lab/globals locks proved F1/F2/F4. An accept-mode Master on loop570 installed VIPs; interface-address Retrieve returned only the primary address and an unchanged commit planned no operations. The assembler restored an intentionally reversed desired order over real dumped VIPs. After stopping the VR and deleting its loopback, VR Delete succeeded; Retrieve omitted the orphan, while the raw VR dump/CLI retained the documented pool residue. The strengthened check also confirmed three raw IPv4 addresses before filtering, then an `enabled:false` scheduler transaction finished APPLIED and stopped the VR. `TestVRRPProductFixesOnDisposableVPP` passed in 0.77 seconds; the private VPP then stopped. Full output: [live-product-fixes-applied.txt](S-vrrp-product-fixes-2026-10-03-evidence/live-product-fixes-applied.txt).

Shared VPP stayed at PID 1014 with NRestarts=0. The final private API/agent HTTP gate also passed with a signed two-day `ha` test licence, slot 5, shared lab lock and exclusive globals lock. Real Master accept-mode installed both VIPs; HTTP `GET /api/v1/state/drift` returned 200 and `changes: []` while desired VIP order was reversed relative to VPP. The `enabled:false` commit returned 200/APPLIED and subsequent full drift was empty. All API/agent processes stopped, the slot database was dropped and disposable VPP stopped. Output: [http-drift.txt](S-vrrp-product-fixes-2026-10-03-evidence/http-drift.txt). F3's requested non-privileged checks/docs are complete; daemon-spawn privileges remain a separate owner decision, rather than a missing implementation in this task. Finishing CI, independent review and an actual merge remain outstanding. F3 privileges are recorded in [the questions file](S-vrrp-product-fixes-questions.md).
