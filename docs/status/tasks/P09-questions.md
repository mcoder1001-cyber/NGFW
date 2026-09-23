# P09 — questions for the manager (none blocking; decisions taken are in P09.md)

## Q1 — `full` holds the lab lock *shared* after an exclusive barrier (rules §1b say "exclusive")
`docs/lab/shared-host-rules.md` §1b: harnesses take `flock -s`, `tools/ci.sh full` takes `flock -x`. Two facts make "exclusive
throughout" impossible with the code that exists:
1. P04's fix round (`6601efd`) made `tools/lab rig up` refuse while the lock is exclusively held:
   `error: rig: /run/lock/vrx-lab.lock is held exclusively (manager ci full / VPP restart) — retry when 'tools/lab lock status' says free`
   — observed in the evidence run D4 (`CI GATE FAILED — tools/lab rig up w12 failed`). The gate itself was the exclusive holder.
2. `test/integration/smoke/smoke_test.go` opens the lock file itself and takes `LOCK_SH` on a fresh file description; flock is per
   open file description, so inside an exclusive lock held by the same process tree that call blocks until `go test` times out.
What `full` does now: acquire **exclusive** (barrier: returns only when no restart and no harness is running) → **convert to
shared** → `rig up` → suites → `rig down` → release; exports `VRX_LAB_LOCK_HELD=1 VRX_CI_FULL=1` throughout.
Options: (a) keep as implemented and amend §1b ("full: exclusive barrier, then shared"); (b) make `tools/lab rig up|down` skip
its exclusive-holder guard when `VRX_CI_FULL=1` and every harness skip its own lock when `VRX_LAB_LOCK_HELD=1`, then `full` can
stay exclusive — but one harness that forgets it hangs the gate for 20 min, and P04's tool is not mine to change. I chose (a);
(b) can be layered on later without touching the gate's interface.

## Q2 — P04's smoke module vs. the new quick step 8
Step 8 runs `gofmt -l`, `go vet`, `go test -count=1` in every Go module under `test/`. With P04 merged after P05a, the smoke
module's `go.sum` may lack entries that `apps/agent/go.mod` gained (reviewer F6) — the gate will then fail with the message
"go vet failed in test/integration/smoke (a stale go.sum …)". That is the intended behaviour; the fix belongs to whoever merges
P04 (run `go mod tidy` in `test/integration/smoke`, commit). See P09.md for what the evidence run on main+P04 showed.

## Q3 — `.github/gitleaks.toml` location
The gitleaks config lives under `.github/` (owned by P09, kept next to the workflow). If you prefer `tools/gitleaks.toml`, it is
a one-line move (`tools/ci.sh` looks for `.github/gitleaks.toml`).
