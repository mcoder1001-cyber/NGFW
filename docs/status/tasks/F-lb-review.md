# F-lb review (cloud session, at merge)

Reviewer: cloud manager session, 2026-09-26. Branch `task/F-lb` @ `c0a8ae09` (base `main@1d3ccf31`), reviewed as ported
onto main (`port/F-lb`): three-way apply of the task delta, generated files regenerated, integration fixes listed in the
merge commit. Checklist: prompts/REVIEW-PROMPT.md.

## Verdict: **APPROVE WITH CHANGES** — M1, M2 and L1 are fixed in the port commit; H1 is a follow-up row (`F-lb-host`)

## Findings

### H1 — no host proof yet (checklist 2, 8)
Only fake-VPP tests, the API e2e against the slot database and jsdom web tests ran. `docs/status/tasks/F-lb.md`
"Pending host steps" lists `TestLbOnHost` on slot 2, the globals-owner GC run and the UI screenshot; none of them ran
(they waited for TD-25, and a cloud session has no VPP). The host tests exist and are opt-in (`NGFW_LB_HOST=1`).
**Resolution:** merged with a follow-up row `F-lb-host` (ready, deps F-lb) that runs steps 1–3 and pastes NRestarts
before/after. Not a code finding.

### M1 — `LbFlushVip` checked and flushed without the transaction lock (`internal/agent/rpc_lb.go`)
`lbStored` took and released the transaction lock; `lb.FlushVIP` then ran `lb_as_dump` and `lb_flush_vip` with no lock
held. A transaction that deletes the VIP between the dump and the flush makes VPP 26.06 flush an uninitialised VIP
index — possibly every VIP of every owner (the defect the helper exists to avoid, `state.go`).
**Fixed:** the flush holds the transaction lock across the stored-state read, the ownership record, the dump and the
flush (`lbLock`/`lbStoredLocked`). Test: `TestLbApplyStateFlushRemove` — a flush while a transaction holds the lock
answers UNAVAILABLE and sends nothing (fails on the branch's code: the flush went through).

### M2 — garbage collection could interleave with a transaction (`internal/subsystems/lb.go`)
The debounced GC timer ran `lb.GarbageCollect` (two dumps, `GCSafe`, then `cli_inband`) from its own goroutine. A
transaction that changes a NAT VIP (delete + add — exactly the case `GCSafe` refuses as a likely VPP crash) could run
between the check and the command. **Fixed:** new `subsystems.Env.Exclusive` (the agent's transaction lock, set by
`agent.New` through an atomic pointer since the wiring is built before the service); the GC runs inside it. Test:
`TestLbGCRunsExclusivelyAndStops`.

### L1 — the GC timer outlived the agent (`internal/subsystems/lb.go`)
Nothing stopped a pending timer on shutdown. **Fixed:** `lbGC.stop` is registered with `Wiring.OnClose`; later deletes
schedule nothing. Same test.

### L2 — merge seam (integration, not a defect)
`desired/lb_services_seam.go` said "delete when F-rpf-adl-pbr is on the base"; it is. Deleted; lb registers in
`ServicesImplemented` and `ServicesUnsupported` (one call per projection) reports other members.

## Checked, no finding
1. Contract: additive only — `ServicesConfig.lb = 11` (allocated in wave-BC-numbers), LbState/LbFlushVip RPCs, schema
   `services.lb`; `buf breaking` against main clean; contract commits present; `F-lb-contract.md` present.
3. Restart safety: VIP/AS ownership through the persisted DF-7 BootStore (`CheckPersistent`), intf-nat claim-first
   (TD-11b); `TestLbRestartReappliesWithoutDuplicates` (fake VPP). Write-only objects (V20) are documented and noted in
   DryRun; Retrieve never reports them.
4. VPP API provenance: only existing binapi messages (`lb_*`, `cli_inband`); binapi untouched.
5. Shared-host rules: slot agents never send `lb_conf` or the GC (`TestLbSlotAgentNeverCollects`); the GC sentinel
   `0.0.0.0/8` is refused by the schema and the projection.
6. Security: the only CLI line is the constant `lb vip 0.0.0.0/32 del` (ALLOWLIST row); the flush route is an audited
   operator mutation behind the guard; no secrets.
7. Transaction semantics: lb objects go through the scheduler; a failed intf-nat enable releases its claim (`c.Undo`).
8. UI: the tab calls the real `/state/lb/vips` and flush routes; no TODO/mock/stub in product code; screenshot → H1.
10. i18n: en + fa namespaces; `check-logical-css` clean.
11. Tests (this session, on the port): gofmt, go vet, golangci-lint 2.13.2 (0 issues), `go test -race` agent,
    pnpm gen (no drift), build, typecheck, lint, vitest; `tools/ci.sh check --base`.
