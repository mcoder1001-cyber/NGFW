# IP unnumbered — R4 review

Reviewed local source `675cd9eaf6ef49e30f799019af5c04d9112d31c8`, tree `b726b40c5a5692d8dfb9d449e2947eeaca5f8dce`.

The generated API provenance confirms `SwIfIndex` is the numbered donor and `UnnumberedSwIfIndex` the borrower. Create checks both IPv4 and IPv6 table identity before mutation, refuses unclaimed existing relationships and nested donors, persists physical-interface claims before writes and releases a newly acquired claim on failed writes. Delete re-dumps live borrower identity/ownership and refuses a changed donor. Retrieve uses a fresh authoritative unnumbered dump; it does not rely on process-cached relationships. Donor/borrower aliases and table dependencies preserve creation/removal ordering. Actual product registration and claim-holder wiring are present.

Resolved MAJOR: the initial implementation could convert a live numbered donor into a borrower while another live, potentially foreign, interface still borrowed from it. The final source scans every live relationship and rejects conversion before claim acquisition or API mutation. `TestUnnumberedRejectsConvertingLiveDonor` asserts foreign relationship preservation.

Independent focused verification from `apps/agent`:

```text
GOMODCACHE=/workspace/scratch/9baf7442ffbf/toolchain/module-cache GOCACHE=/workspace/scratch/9baf7442ffbf/toolchain/build-cache GOPROXY=off /workspace/scratch/9baf7442ffbf/toolchain/go/bin/go test -race -count=1 ./internal/descriptors/interface ./internal/desired ./internal/agent -run 'TestUnnumbered'
ok ngfw/agent/internal/descriptors/interface 1.021s
ok ngfw/agent/internal/desired 1.099s
ok ngfw/agent/internal/agent 1.183s
```

The controls include live mapping, ownership refusals, cross-IPv6 VRF rejection, foreign borrower preservation and scheduler Apply/Retrieve/idempotence/restart/donor-change/rollback/revoke paths. `git diff --check` passed. No full CI or native VPP forwarding acceptance was run; no host state was changed.

Verdict: **APPROVE** for source ownership and data-plane safety. Native acceptance remains separate.
