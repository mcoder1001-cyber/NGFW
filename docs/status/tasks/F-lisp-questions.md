# F-lisp — questions (written, not waited on)

1. **Config home** `tunnels.lisp` (TunnelsConfig 10) as decided in wave-BC-numbers.md — no reason found against it. Contract
   commits are on this branch (F-lisp-contract.md); please pick them up.
2. **Host run**: request an opt-in manager VPP window for `VRX_DF6_LISP_HOST=1` / `VRX_INTEGRATION=1` (V14 leaks on every
   enable/disable). Not run; unit evidence only.
3. **Claims store**: the envelope says `df6.WithClaims(Wiring.IfaceClaims())`; the code (TD-11b) requires `PairClaims("df6")`
   and refuses start otherwise — used PairClaims.
4. **df6 gap** (read-only for me): df6 singleton/require descriptors declare neither CheckPersistent nor RecordsNoOwnership, so
   the persistence guard refused to start. Worked around with a decorator in `subsystems/lisp.go`; better fixed in df6.
5. **Plugin-absent tolerance**: a Retrieve of LISP on a VPP without the plugin (or the coretest fake without `InstallLisp`) would
   fail every Retrieve; the decorator reports no objects instead (duck-typed `Handles` check for the fake). Acceptable?
6. **service_test.go** (agent core tests) hard-code the subsystems list; I appended `tunnels` (2 lines). F-tunnels hits the same lines.
7. **Examples dir**: `packages/schema/examples/lisp-*.json` would fail examples.test.ts's "belongs to a group" rule (file not
   mine), so the full example lives only in `packages/proto/test/fixtures/lisp-full.json` and is tested from there.
8. **Feature flag** for LISP given V14: default kept — visible under "advanced" with an in-page warning; documented.
9. **gpe:false with enabled:true**: VPP enables GPE with LISP; projection warns `tunnels.lisp-gpe-implied`, Retrieve then shows gpe:true.
10. **Non-owner Retrieve**: switches/PITR are unreadable there (require variants are write-only); `enabled` is inferred from owned
    objects, `gpe`/`pitr` left unset. LispState is VPP-wide (LISP has no owner tags).
11. `tools/ci.sh` cannot run in this container (pnpm tool binary "Exec format error" under `pnpm gen`); please run it on the host.
