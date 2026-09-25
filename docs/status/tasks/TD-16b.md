# TD-16b — descriptors/kit follow-up (TD-16 open questions, ARCH-06)

## What
1. **Strict prefixes in df2/df6 (D-149)**: `df2.ParsePrefix` / `df6.ParsePrefix` now call `kit.ParsePrefix` (host bits rejected). Every df6 prefix helper (PrefixOf, CanonicalPrefix, IP4/IP6PrefixOf) follows. KeyOf of a host-bit object yields `<name>/invalid`; Create / the lisp projection reject it. `FromPrefix` (VPP wire side) still masks. `kit.MaskPrefix` + its test removed (no users left).
   Tests updated: df2 `TestAddresses`, ip6_nd `TestRaPrefixLifecycle`, lisp `TestLISP`, sr `TestPolicyAndSteering`, sr_mpls `TestPolicySteeringEndpointColor`, agent `TestLispProjectionWarnsForTunnelKindsNotWired`: canonical inputs for the key checks + a new assertion that a host-bit prefix is rejected.
2. **One sentinel**: `ErrRetrieveUnsupported` in df2/df6/dfkit `errors.go` is now `= kit.ErrRetrieveUnsupported` (the same value as `scheduler.ErrRetrieveUnsupported`). The exported names stay as aliases: ~35 files refer to them, so removing them would only add churn. `dfkit.RetrieveUnsupported` delegates to `kit.RetrieveUnsupported`.
3. **df6 globals declare persistence themselves**: `RequireDescriptor.RecordsNoOwnership()`. `SingletonDescriptor.CheckPersistent()` returns nil unless the new `SingletonSpec.Claims` is set. When it is set, the method checks that the claims store survives a restart, like keyed descriptors do. This differs from the brief ("declare RecordsNoOwnership"): pppoe.cp is a singleton that records per-boot claims, and a method set cannot depend on the spec. pppoe `cpSpec` sets `Claims`. The `lispGlobal` wrapper in `subsystems/lisp.go` has been removed. `lispTolerant` stays because it handles the missing-plugin case in Retrieve.
4. **dfkit/boot.go**: `FileBootStore.flush` uses `kit.WriteFileAtomic(path, raw, 0o600)`, which fsyncs the file and the directory after the rename.
5. Lint nit: `kit_test.go` G304 nolint.

## Verification (apps/agent)
```
$ gofmt -l .                    # empty
$ go vet ./...                  # clean
$ go test -race ./...           # all ok (subsystems incl. reachability: ok ngfw/agent/internal/subsystems 7.9s)
$ golangci-lint run ./...       # 2 issues, both pre-existing and untouched:
  internal/agent/service.go:550 revive time-naming (revertRetryMin)
  internal/descriptors/core/coretest/lisp.go:20 staticcheck QF1008
```
No VPP host needed: all tests use the fake client. Integration tests skip without VRX_INTEGRATION.

## Out of scope
- Item 5, moving family `Register` onto `kit.Register(r, Env)`: not cheap. It touches the register.go of every family and the subsystems wiring, and families take different option types (df2.Options, df6.Option, globals-owner flags). It needs its own row.
- `tools/ci.sh check` was not run in full (Go gates only).

## Open questions
1. The TS schema (packages/schema) may still accept prefixes with host bits for lisp EIDs, sr/sr-mpls steering, RA prefixes and 6rd. The agent now rejects them, so the schema owner should add a canonical-prefix check to keep the API and agent aligned.
2. Pre-existing lint findings above (agent/service.go, coretest/lisp.go).
