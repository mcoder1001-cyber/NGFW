# P05a — questions for the manager (none blocking; work continued)

1. **`vpp.Client` method name.** The prompt lists `Invoke`, `Stream`, `Connected()`. I made `Client` a strict superset of
   govpp's `api.Connection` (`Invoke`, `NewStream`, `WatchEvent`) plus `Connected()`, compile-time asserted, so the generated
   `*_rpc.ba.go` clients (`interfaces.NewServiceClient(client)`) accept it without an adapter and P05 wraps `*core.Connection`
   directly. If you prefer the literal name `Stream`, it is a one-line `contract/` change before DF-* start; I recommend keeping
   `NewStream`.

2. **Fake and `control_ping`.** Generated dump clients end a dump on the *project's* `memclnt.ControlPingReply` type, which does
   not exist until P04's `apps/agent/binapi` merges. The fake therefore takes it as an option
   (`fake.WithControlPingReply(&memclnt.ControlPingReply{})`) and fails loudly when a dump runs without it. After P04 merges, a
   `contract/` follow-up could hard-wire the default (import `ngfw/agent/binapi/memclnt` in `fake`). Not needed for correctness.

3. **Dependency semantics for P05.** I documented (descriptor.go package doc) that a missing *mandatory* dependency fails the
   transaction before apply (strict, matches atomic commit), `Optional` only orders, and `ErrRecreate` re-creates dependents.
   Ligato instead keeps such objects "pending". If P05 wants pending semantics, the interface does not change, only the doc.

4. **golangci-lint** (2.13.2) turned out to be installed on the host after all (D-009 said go vet only). I ran
   `golangci-lint run ./...` by hand: 0 issues in the final commit. See item 6 for why `make lint` cannot show that.

5. **Owner-tag format** `"<owner>:<id>"` (`vpp.OwnerTag`, max 63 bytes) is my proposal for P05 step 3b; it is in `internal/vpp`
   so DF-* can use it now. Confirm or overturn before DF-1 lands.

6. **`apps/agent/Makefile` `lint` target swallows lint failures.** golangci-lint 2.13.2 *is* installed on the host now, but
   `command -v golangci-lint >/dev/null && golangci-lint run ./... || echo "golangci-lint not installed"` prints the fallback
   message and exits 0 whenever the linter finds issues. `make lint` therefore cannot fail the CI gate (`tools/ci.sh`). Not my
   file (P01/P09): suggest `if command -v golangci-lint >/dev/null; then golangci-lint run ./...; else echo ...; fi`.
   I ran `golangci-lint run ./...` by hand and fixed everything it reported in my packages (0 issues at the final commit).
