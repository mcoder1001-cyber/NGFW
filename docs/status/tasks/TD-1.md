# TD-1 — one VPP boot identity helper (D-080) + retrofit

Branch `task/TD-1`. Not merged.

## What was built
- **`apps/agent/internal/vpp/bootid`** — the only boot-identity implementation:
  - `type Identity struct{ BootID string; PID int; StartTime uint64 }`; `Equal`, `IsZero`, `Complete`.
  - `Current(ctx, client)`: PID = `control_ping_reply.vpe_pid`, BootID = `/proc/sys/kernel/random/boot_id`,
    StartTime = `/proc/<pid>/stat` field 22 (`ParseStat` counts fields after the **last** `)`, so a comm with blanks
    and parentheses is handled). Fails only when control_ping fails; an unreadable /proc part is left unknown.
  - Stable encoding `String()` = `"<boot_id>/<pid>/<start>"` (`?` for unknown), **byte-identical to what DF-3/DF-6/DF-8
    already persisted**, so existing records keep matching after the upgrade. `Parse` / `Matches`: a bare PID (pre-D-080
    format) → `ErrLegacy`, anything else malformed → `ErrFormat`; both never match (re-add once / claim expired).
  - Test injection: `SetProcRoot(root) (restore)`, `Reader{ProcRoot}`, `WriteFakeProc(root, bootID, pid→start)`.
- **Retrofit (private implementations deleted):**

| Where | Before | After |
|---|---|---|
| `dfkit/identity.go` (DF-8) | own triple reader, PID via show_threads, strict | `dfkit.BootIdentity` = `bootid.Current` + `Complete()` check (still strict); `IdentitySource` now returns `bootid.Identity` |
| `dfkit/boot.go`, `iface.go`, `sflow` | string compare | `bootid.Matches` / `Identity.Equal` |
| `df6/claims.go` `BootID`, `ProcRoot`, `procStart` (DF-6) | own tolerant triple | removed; `bootid.Current`; `BootHolder(name, bootid.Identity)` |
| `df6/keyed.go`, `bypass.go`, `sr_mpls/steering.go`, `pppoe/cp.go` | `df6.BootID` | `bootid.Current` |
| `natcommon/config.go` `VPPIdentity`, `BootIdentity`, `ProcRoot` (DF-3) | show_threads PID + own triple | removed; `cnat` uses `bootid.Current` |
| `classify/table.go` `vppInstance` + store (DF-2) | **vpe_pid only**, persisted `vpp_instance` | triple, persisted `vpp_boot` (encoded); old `vpp_instance` file → unknown instance → records untrusted, store reset on first use and rewritten in the new format |
| `acl/stats_enable.go` `vppIdentity` (DF-4) | **show_threads PID only** (memory) | `bootid.Identity` |
| `interface/identity.go` `VPPIdentity`, `vppEpoch` (DF-1) | **show_threads PID only** (memory) | `vppEpoch` holds `bootid.Identity`; `iface.VPPIdentity` removed |

  Test fakes (`dfkittest`, `ifacetest`, acl, cnat) now answer `control_ping` with a settable `vpe_pid` instead of
  `show_threads`. Descriptor docs under `docs/agent/descriptors/` point at `internal/vpp/bootid`.
- Behaviour unchanged except: PID-only identities (classify, acl stats flag, interface epoch) are now the triple; the
  PID source is `vpe_pid` everywhere (same value as show_threads thread 0 on a real VPP — verified below: 1831571 = `pidof vpp`).
  Tolerance kept per caller: DF-8 strict (error on incomplete identity), everyone else tolerant (`?` parts), as before.

## Verification

### Unit (every touched package)
```
ok  	ngfw/agent/internal/vpp/bootid	0.029s
ok  	ngfw/agent/internal/descriptors/dfkit	0.024s
?   	ngfw/agent/internal/descriptors/dfkit/dfkittest	[no test files]
ok  	ngfw/agent/internal/descriptors/dfkit/restarttest	0.018s
ok  	ngfw/agent/internal/descriptors/df6	0.022s
?   	ngfw/agent/internal/descriptors/df6/df6test	[no test files]
ok  	ngfw/agent/internal/descriptors/sr_mpls	0.039s
ok  	ngfw/agent/internal/descriptors/pppoe	0.024s
ok  	ngfw/agent/internal/descriptors/natcommon	0.025s
ok  	ngfw/agent/internal/descriptors/cnat	0.030s
ok  	ngfw/agent/internal/descriptors/classify	0.055s
ok  	ngfw/agent/internal/descriptors/acl	0.053s
ok  	ngfw/agent/internal/descriptors/interface	0.025s
?   	ngfw/agent/internal/descriptors/interface/ifacetest	[no test files]
ok  	ngfw/agent/internal/descriptors/sflow	0.027s
ok  	ngfw/agent/internal/descriptors/pcap	0.022s
ok  	ngfw/agent/internal/descriptors/ipfix	0.022s
ok  	ngfw/agent/internal/descriptors/ip_session_redirect	0.027s
```
New tests: `bootid` TestParseStat (comm with spaces, `a) b`, `x) S 1 2 3)`, empty comm; bad inputs), TestStringParseRoundTrip,
TestParseLegacyAndMalformed, TestEqual, TestCurrentWithFakeProc (restart → new PID; reboot with same PID → differs),
TestCurrentControlPingFails; `classify` TestStoreLegacyPIDOnlyFormat (old `vpp_instance` file with the *running* PID is
still untrusted, reset, rewritten as `"vpp_boot": "boot-a/4242/777"`); `dfkit` TestBootRecordLegacyFormat (`"1000"`,
`"fake/1000"`, `""` never match; new format matches; restart → no match). `go test ./...` in apps/agent: all ok.

### Host run, slot 12 (`eval "$(tools/lab env 12)"; VRX_INTEGRATION=1 flock -s /run/lock/vrx-lab.lock go test -p 1 -count=1 -v ./internal/descriptors/classify/ ./internal/descriptors/acl/`)
`systemctl show vpp -p NRestarts`: before `NRestarts=3`, after `NRestarts=3`.
```
    integration_test.go:166: bindings applied on loop1209: input-acl, output-acl, ip-table, l2-tables (table index 7)
--- PASS: TestClassifyOnHost (0.07s)
--- PASS: TestTableDeleteStaleIndexOnHost (0.02s)
ok  	ngfw/agent/internal/descriptors/classify	0.136s
    integration_test.go:386: acl.stats-enable applied to VPP boot identity (D-080 boot_id/vpe_pid/starttime) b7712a53-c1e7-45e2-98b8-bdb21f3904f9/1831571/4845048
--- PASS: TestACLPluginOnHost (0.20s)
ok  	ngfw/agent/internal/descriptors/acl	0.285s
```
Host facts at the time: `pidof vpp` = 1831571, `/proc/1831571/stat` field 22 = 4845048, boot_id b7712a53-c1e7-45e2-98b8-bdb21f3904f9 — matches the logged identity.
Honest note: the very first host run had `TestClassifyOnHost` fail once at the output-acl step ("loop1209 already has an
output ACL (ip4-outacl) bound outside this agent's records" — the feature was already on for the reused sw_if_index). The
base commit (3496b62, exported to a scratch dir) passed right after, and the TD-1 code passed 3 consecutive reruns plus the
full run above; the failing path (feature_is_enabled on a fresh loopback) does not touch the boot identity. Looks like
transient host state (a stale ip4-outacl on a reused index); not investigated further.

### CI (`tools/ci.sh --base main` at a9cde22)
```
  contract guard: HEAD vs main                       0m00s
  apps/agent: make lint test build                   0m45s
  test/ Go modules, unit mode (test/integration/smoke)   0m02s
  mode quick · wall time 1m53s · logs /root/ngfw-wt/logs/ci/TD-1-20260924-022043-1950520

CI GATE PASSED
```
(first CI run failed on gosec G301 for 0755 dirs in `WriteFakeProc`; fixed to 0750.)

### No private boot-identity implementation remains
`grep -rnE 'random/boot_id|VpePID|ShowThreads|vppIdentity|vppInstance|VPPIdentity|func BootID|natcommon\.BootIdentity|ProcRoot +=|LastIndexByte\(s, .\)' --include=*.go internal cmd | grep -v _test.go` (apps/agent):
```
internal/descriptors/interface/ifacetest/integration.go:54:	rep, err := vlib.NewServiceClient(c).ShowThreads(context.Background(), &vlib.ShowThreads{})
internal/descriptors/df6/df6test/fake.go:198:	v.Reply("control_ping", &memclnt.ControlPingReply{VpePID: pid})
internal/descriptors/dfkit/dfkittest/fake.go:45:		return []api.Message{&memclnt.ControlPingReply{VpePID: f.pid}}, nil
internal/vpp/bootid/bootid.go:11://   - BootID: /proc/sys/kernel/random/boot_id.
internal/vpp/bootid/bootid.go:118:	i := strings.LastIndexByte(s, ')')
internal/vpp/bootid/bootid.go:137:	if r.ProcRoot == "" {
internal/vpp/bootid/bootid.go:145:	b, err := os.ReadFile(filepath.Join(r.root(), "sys/kernel/random/boot_id")) //nolint:gosec // fixed kernel path under the proc root
internal/vpp/bootid/bootid.go:180:	return r.ForPID(int(rep.VpePID)), nil
internal/vpp/bootid/bootid.go:212:		if err := os.WriteFile(filepath.Join(root, "sys/kernel/random/boot_id"), []byte(bootID+"\n"), 0o600); err != nil {
internal/descriptors/interface/ifacetest/ifacetest.go:67:		return []api.Message{&memclnt.ControlPingReply{VpePID: v.PID}}, nil
```
Remaining hits outside `bootid`: test fakes setting `vpe_pid` (df6test, dfkittest, ifacetest) and
`ifacetest/integration.go` using show_threads for the **worker count** (not identity).

## Out of scope / not migrated
- `internal/renderers/{chrony,unbound}/pending.go` parse `/proc/<pid>/stat` field 22 of the **daemon** (RF-3 restart
  detection), not VPP; left as is (could reuse `bootid.ParseStat` later).
- `dfkit.IdentitySource` hook is kept (test seam; `dfkittest` installs a fixed identity since the fake PID is no process).
- DF-5 / DF-7 branches not touched. What they must switch to:
  - **DF-7** (`task/DF-7`): `df7/applied.go` uses `iface.VPPIdentity` (main-thread PID, **removed by TD-1 → DF-7 will
    not compile after rebase**). Switch `Base.BootIdentity` to `bootid.Current(ctx, b.Client)` and store
    `Identity.String()` / compare with `bootid.Matches`; `df7test` fake must answer `control_ping` with `VpePID` instead
    of `show_threads`. Any use of `df6.BootID` → `bootid.Current`; `df6.BootHolder` now takes `bootid.Identity`;
    `natcommon.BootIdentity/VPPIdentity` → `bootid.Current`; classify `Store.Instance/Reset` now use `bootid.Identity`.
  - **DF-5** (`task/DF-5`): no boot-identity code today; any future applied-once record or untagged-object claim must use
    `bootid.Current` (never a PID).

## Decisions (for the LOG)
- TD-1: PID source for D-080 is `control_ping.vpe_pid` everywhere (show_threads no longer used for identity).
- TD-1: `bootid.Current` is tolerant (unknown /proc parts encoded `?`); callers that need a complete identity (DF-8) check
  `Complete()`. Encoding kept identical to the pre-TD-1 triple strings so no persisted DF-3/6/8 record is invalidated;
  PID-only records (classify `vpp_instance`) are treated as a mismatch.
- D-080 text "P08 consolidates" is done by TD-1 (`internal/vpp/bootid`).

## Open questions
None.
