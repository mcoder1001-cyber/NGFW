# TD-20 — verification (D-128 trace ban)

**Verdict: APPROVE**

Branch `task/TD-20` @ 57e055f, worktree /root/ngfw-wt/TD-20, working tree clean. All checks below were run
read-only against the worktree and against a disposable local clone (`git clone --no-hardlinks` off /root/ngfw
into scratch, never touching /root/ngfw or /root/ngfw-wt/TD-20's git state); no `show trace` / `trace add` /
`clear trace` and no host/VPP test was run.

## 1. Diff scope — exact match
`git -C /root/ngfw diff main...task/TD-20 --stat`:
```
docs/lab/shared-host-rules.md               |  18 +++
docs/status/tasks/TD-20.envelope.md         |  14 +++ (new — docs/status/tasks/TD-20*, in scope)
docs/status/tasks/TD-20.md                  | 108 +++ (new — status doc, in scope)
docs/vpp-code-track.md                      |   1 +
test/topology/interfaces/helpers_test.go    |  78 +++
test/topology/interfaces/interfaces_test.go | 184 +++
test/topology/interfaces/vpp_test.go        |   7 +
tools/ci.sh                                 |  32 +++
```
Nothing outside the envelope's owned files. `tools/ci.sh` diff is exactly the new `do_trace_ban` function plus
two one-line call sites in `check` and `quick|full`; no other step touched. `docs/lab/shared-host-rules.md` diff
is exactly the new §11 append; `docs/vpp-code-track.md` diff is exactly the one new `V-new` row.

## 2. Replacement evidence — as strict as the old trace assertion (read the code)
Old assertion (`ourTrace`) only string-matched node names (`af-packet-input`, `ip4-lookup`, `ip4-rewrite`,
`<wanIf>-output`) in one trace block for the forward leg; `af-packet-input` is a generic node name shared by
every af_packet interface, so the old test never actually pinned the *ingress* interface, and it said nothing
about the reply leg at all.
The new evidence is at least as strict, and stricter on two axes:
- **Which interfaces**: `fibLookup` binds `wanIP/32` and `lanIP/32` to an exact `sw_if_index` via
  `ip_route_lookup`, then `fibTo`/`showIntCounters` check rx/tx **by interface name** on both `r.lanIf` and
  `r.wanIf`, in **both directions** (request in on lan/out on wan, reply in on wan/out on lan) — the old test
  only named the wan side, and only for the forward leg.
- **How many packets**: the FIB load-balance to-counter and both interfaces' counters must rise by *exactly*
  n packets / n×ipLen bytes (`echoFrames`, `test/topology/interfaces/helpers_test.go`), with the small-ARP-frame
  tolerance proven incapable of forging an extra/missing echo (`smallFrameMax*maxSmallFrames < frameLen`,
  independently checked: 4×200=800 < 1042). The old test only checked node-name presence, not packet counts.
`TestEchoFramesExactlyN` (9 cases) and `TestPingCounts`/`TestShowIntCountersRegex` in helpers_test.go, and
`go vet ./...` + `gofmt -l .` (both clean) + the four listed unit tests (`go test -run
'TestEchoFramesExactlyN|TestPingCounts|TestShowIntCountersRegex|TestMkdirSharedModes'`, no TestMain/init in the
package, no VPP/rig touched) — all PASS, independently re-run.

## 3. The guard (`do_trace_ban`, tools/ci.sh:401-419)
- `bash -n tools/ci.sh` — OK.
- Independently re-derived the exact regex/pathspec from the script and ran it standalone: correctly flags
  Go argv (`vppctl(t, "show", "trace", ...)`, `vppctl(['clear','trace'])`), command strings
  (`cli_inband("show trace")`), shell (`sudo vppctl -s /run/vpp/cli.sock trace add ...`), and the tracedump API
  (`c.TraceDump(...)`); correctly does **not** flag `# vppctl show trace` / `// vppctl(t, "show", "trace")`
  comments, `show tracer`, `show traces-config`, or `echo "never run show trace here"` — reproduced every case
  from TD-20.md's "pattern probe" independently, same results. Confirmed docs/ and apps/agent/binapi/ exclusion
  pathspecs work (planted the banned pattern in both — zero hits).
- Ran `tools/ci.sh check` on the branch in the worktree (TMPDIR/LOG_DIR redirected to scratch): guard **passes**
  (`ok: no packet trace ... outside docs and the generated bindings`), rest of `check` (forbidden patterns +
  gitleaks) also green.
- Built a disposable local clone (`git clone --no-hardlinks -q /root/ngfw`, never touching the real worktrees),
  checked out `task/TD-20`, swapped in `git show main:.../interfaces_test.go`, ran `tools/ci.sh check`: guard
  **fails** with the exact same message and line numbers (291/297/302) pasted in TD-20.md. This independently
  reproduces the claimed before/after result.
- `CUR_LOG=""` and the `sed 's/\\/\\\\/g'` backslash-escape before the `fail` call (which prints with `%b`) are
  present and correct as described.

## 4. shellcheck
Default `shellcheck tools/ci.sh` reports only pre-existing SC2015 (info) / SC2001 (style) hits — diffed against
`git show main:tools/ci.sh` run through the same shellcheck: byte-identical set of findings, none touch the new
`do_trace_ban` lines (394-419) or the two dispatch edits. At the repo's own convention for shell-script
cleanliness (`deploy/vpp/verify.sh` runs `shellcheck -x -S warning`), `shellcheck -S warning tools/ci.sh` exits 0
— clean.

## Minor, non-blocking observation
The envelope's scope item 2 says "a new step in `tools/ci.sh quick`"; the branch wires `do_trace_ban` into
`check`, `quick`, and `full` (TD-20.md explains this explicitly). This is a superset within the same owned file
and the same new step — not scope creep into other steps or files — and is strictly more protective. Not a
blocker.

## Conclusion
All four checks confirmed by independent reproduction, not just by reading the pasted evidence. **APPROVE.**
