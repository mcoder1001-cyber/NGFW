# TD-13 focused verify, fix round 1: **APPROVE**

This verify covers `task/TD-13 @ 04a51453` against the review at `c8b34d4e`. It checks the review's findings only and
does not re-review the rest. No code was edited; every probe and mutation ran in scratch copies.

## Per finding

| finding | verified how | result |
|---|---|---|
| **M1** masking | `validator.go`: the name list and the map-key rule are gone. `collectRefs` masks D-051-form strings anywhere, plus `*_ref` string fields. Inside a `google.protobuf.Struct`, `_ref` keys count, and `Value`/`ListValue` pass the mark on. I re-ran my review probe on HEAD (see below), and ran mutation K2 (the `_ref` rule off). | ✔. The probe keeps `psk0`, its address and description, and the BGP community; `password/snmp-ro` is masked. K2 → `validator_test.go:384: finding leaks "VRX_TEST_PSK_TD13"` (caught). |
| **M2** drift | `PlanOptions{SkipValidators}` + `PlanWith` set the unexported `ApplyOptions.skipValidators`, so Apply can't set it. `validate()` returns at once when it is set. Mutation K1 (the skip ignored). A scratch run of the rebase steps from TD-13.md on TD-9 (see below). | ✔. K1 → `validator_test.go:559: drift plan: … issues [daemon.fake/dns: validator: …] ops []` (caught). The rebase steps work (below). |
| **L1** stage wording | stage.go, descriptor.go, the doc page and D-entry (5): "first tie-breaker, greedy, not a phase split". The overclaim is gone, and stage.go describes my probe case exactly. | ✔ |
| **L2** panic log | `validator.go` recover: `boundText(RedactLeaves(fmt.Sprint(r), value), …)`. Mutation K3 (back to `fmt.Sprint(r)`). | ✔. K3 → `validator_test.go:521: log:` (caught) |
| **L3** no callbacks | The rule is in the `Validator` godoc, the package doc (`descriptor.go`) and the doc page. | ✔ |
| **L4** `InvalidAt` "/" | `TestPlanRunsValidators` rejects with `InvalidAt("services/dns", …)` and expects the object's pointer. My review mutation R2 (prefix check removed). | ✔. R2 is now caught: `validator_test.go:246: issue daemon.fake/dns: …` |
| **L5** D-136 | D-136 is now in main's LOG (`511a6419`, the F-kea/F-host-acl gate lift), so the citation is valid. | ✔ (resolved by the LOG) |
| **L6** constant | The godoc says it is scheduler-local on purpose; it is not aliased. | ✔ |
| **L7** template | `FEATURE-TEMPLATE.md:27-29`: "masks every plaintext it resolved (`rfkit.Redactor`)" and the adopter test. Clear. | ✔ |
| **Q4** proto.md §3 | `7eeafc34` is a separate `docs(contracts):` commit that touches only `docs/contracts/proto.md` (+5 lines, §3). The text matches the behaviour. | ✔ |
| **D-entry** | TD-13.md §6 is my corrected text, plus additions that match the fixes: the panic mask, the "/" rule, "object names and BGP communities stay readable", `PlanWith` in (6), and the masking options (m)/(n). | ✔ |

Probe on HEAD (`RedactLeaves` on a real `DesiredState`):
```
in : interface psk0: address 10.0.0.1/24 overlaps (description to branch); route-map rm1 seq 10: set community 65000:70000: malformed; ref password/snmp-ro
out: interface psk0: address 10.0.0.1/24 overlaps (description to branch); route-map rm1 seq 10: set community 65000:70000: malformed; ref <redacted>
```

TD-9 rebase steps (TD-13.md "For TD-9's rebase"), simulated in scratch:
1. `git archive task/TD-9 apps/agent`, then `git apply` of `4f472cc7..04a51453 -- apps/agent`. It applies cleanly.
2. `Plan` delegates to `PlanWith(…, PlanOptions{})`, and `PlanWith` is wrapped in TD-9's `s.guard("plan", …)`.
3. `planSources` takes an optional `PlanOptions` and passes it to all four `s.sched.Plan` sites. `CheckDrift` passes
   `SkipValidators: true` at its single call.

Result:
- vet and gofmt clean;
- `go test -race -count=1` passes: scheduler 1.8 s, agent 20.6 s, subsystems 1.2 s (TD-9's own drift and guard tests
  included).

The steps are correct and enough. The merger or the rebasing worker should add one test: a drifted daemon object
whose validator rejects gives `vrx_agent_drift_objects` = the number of drifted ops.

## Runs

```
$ cd apps/agent && TMPDIR=/tmp/g-td13rv go test -race -count=1 ./internal/scheduler/... ./internal/agent/... ./internal/subsystems/...
ok  	ngfw/agent/internal/scheduler	1.499s
ok  	ngfw/agent/internal/agent	12.505s
ok  	ngfw/agent/internal/subsystems	6.777s
$ go test -race -count=5 -run 'TestValidator|TestStage|TestFailingValidator|TestPlanRunsValidators|TestPlanWith|TestRedactLeaves' ./internal/scheduler/
ok  	ngfw/agent/internal/scheduler	1.920s
go vet, gofmt -l: clean
git merge-tree --write-tree main(1d3ccf31) task/TD-13 → clean; task/TD-11c × task/TD-13 → clean
```

## Nit (not blocking; fix at TD-13's rebase)

`validator_test.go:363-364`: the doc comment of `TestValidatorFindingRedactsSecretLeaves` still says
"secret-named fields … everything under a secret-named key". It should say "D-051 references and `*_ref` values;
object names and communities stay readable".

## Merge notes

- M2's condition stands: whichever of TD-9 and TD-13 rebases second applies the three steps above, plus the drift
  test.
- The D-112 squash subject can be `feat(agent): …`. No contract path is touched: proto.md is outside
  `CONTRACT_PATHS`.
- The tech debt from the review (L8, L9, `dynsource.go:203`) is for the manager to file.
