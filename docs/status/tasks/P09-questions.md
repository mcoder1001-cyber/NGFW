# P09 — questions for the manager (none blocking; decisions taken are in P09.md)

## Q1 — `full` holds the lab lock *shared* while the suites run (rules §1b say "exclusive")
`docs/lab/shared-host-rules.md` §1b: harnesses take `flock -s`, `tools/ci.sh full` takes `flock -x`. Both cannot be true at the
same time: P04's `test/integration/smoke/smoke_test.go` opens `/run/lock/vrx-lab.lock` itself and takes `LOCK_SH` on a fresh
file description, and flock is per open file description — inside `full`'s exclusive lock that call blocks until `go test`
times out (verified by reading the code; the same would hold for any harness that follows the 00-CONTEXT convention).
What `full` does now: exclusive for `rig up` → **convert to shared** for the suites (still held, so no VPP restart can start
underneath; other harnesses may run beside the gate on their own prefixes) → exclusive again for `rig down`. It also exports
`VRX_LAB_LOCK_HELD=1 VRX_CI_FULL=1`.
Options: (a) keep as implemented and amend §1b ("full: exclusive around rig up/down, shared while suites run"); (b) require
every harness to skip its own lock when `VRX_LAB_LOCK_HELD=1` (P04 fix round could add it to `sharedLock()`), then `full` can stay
exclusive throughout — but a single harness that forgets it hangs the gate for 20 min. I chose (a); (b) can be layered later.

## Q2 — P04's smoke module vs. the new quick step 8
Step 8 runs `gofmt -l`, `go vet`, `go test -count=1` in every Go module under `test/`. With P04 merged after P05a, the smoke
module's `go.sum` may lack entries that `apps/agent/go.mod` gained (reviewer F6) — the gate will then fail with the message
"go vet failed in test/integration/smoke (a stale go.sum …)". That is the intended behaviour; the fix belongs to whoever merges
P04 (run `go mod tidy` in `test/integration/smoke`, commit). See P09.md for what the evidence run on main+P04 showed.

## Q3 — `.github/gitleaks.toml` location
The gitleaks config lives under `.github/` (owned by P09, kept next to the workflow). If you prefer `tools/gitleaks.toml`, it is
a one-line move (`tools/ci.sh` looks for `.github/gitleaks.toml`).
