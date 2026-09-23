# Review — task/P04 (lab tooling, veth/netns rig, binapi, local services)

reviewer: review agent (slot w11) · 2026-09-23 · branch `task/P04` @ `6f9f330` vs `main@c8867a5` · host `vrx-a`, real VPP v26.06-release (pid 1109)
Everything below was re-run by the reviewer on the host; the worker's pasted output in `P04.md` matches what I observed.

## Verified (matches the report)

| check | result |
|---|---|
| 1. contract paths (`packages/schema`, `packages/proto`, `apps/agent/gen`, `packages/api-client/src/generated`) | no changes |
| 4. VPP API provenance | `apps/agent/binapi/` (267 files, 143 pkgs) + `tools/binapi-gen.sh` are P04-owned; regen → `git status --porcelain` **empty**; `go build ./... && go vet ./...` OK; `go mod tidy -diff` clean for `apps/agent` and the smoke module; go.mod delta is exactly `go.fd.io/govpp v0.13.0` + `struc` indirect (+8 go.sum lines) |
| 3. no VPP restart/kill while `handover: pending` | `tools/lab kill-vpp vrx-a` → `REFUSED … (D-012)` rc=2; `restart-vpp` idem; `MainPID` 1109, `ActiveEnterTimestamp 12:42:53` unchanged (task started 13:13); journal shows no restart |
| host not modified | `/var/log/apt/history.log`: last apt run 11:29 (before the task); `/etc/vpp/startup.conf` mtime 11:55; `dpkg`: all `vpp*` 26.06-release still installed; PostgreSQL 18.6 / Valkey 9.0.4 were pre-existing |
| 2. real verification | `rig up w11` → `host-w11l0`/`host-w11w0` with 10.11.{1,2}.1/24; `ping ns-w11-lan → 10.11.2.2` ttl=63 (one hop = VPP); `show interface` rx 10 / 6; `rig down w11` → no netns/veth/VPP object left. Smoke test `VRX_TEST_PREFIX=w11 test/integration/smoke/run.sh` **PASS 3.4 s** (govpp `show_version` 26.06-release, `sw_interface_dump`, stats-segment rx 2→9 / 0→6, leftovers = none); without `VRX_INTEGRATION` it SKIPs with a message |
| 5. shared-host rules (static) | no `pkill`/`killall`; `rm -rf` only on a `mktemp -d`; no system unit started; prefix regex-validated before use in any pattern; `run_on` quotes with `%q`; pg role/db names validated `^[a-z][a-z0-9_]{0,15}$`, password `[A-Za-z0-9_.-]` only |
| 6. secrets | pg password generated from urandom, written only to `/run/vrx-test/<name>/pg.env` (0600, verified) and printed redacted; nothing in the repo |
| pg-test | `create rv11` / `list` / `dsn` / `drop rv11` OK, `rv11;x` and `''` refused |
| `status` / `env` / `inventory` / `provision vrx-a` | output identical in shape to P04.md; `env 13` refused; `provision vrx-a` PASS all, rc=0; `provision vrx-b --apply` refuses (planned, rc=3) |
| 11. gate | `tools/ci.sh --base main` → `CI GATE PASSED` (reviewer run, log `/root/ngfw-wt/logs/P04-review-ci.log`; worker log 13:35:46 also PASSED); `shellcheck -S warning` on all four scripts clean |

## Findings (ranked)

### F1 — HIGH — `rig gc`/`rig down` match other slots' VPP interfaces: `rig gc w1` deletes slot 10/11/12's rig
- `tools/lab:299` (`rig_leftovers`): `vpp_ifnames | grep -E "^host-$P"` and `tools/lab:337` (`gc`): `for n in $(vpp_ifnames | grep -E "^host-$P")` — no terminator after the prefix, so `w1` matches `host-w11l0`, `host-w12w0`, `host-w10…`. Same bug in `test/integration/smoke/smoke_test.go:249` (`strings.HasPrefix(name, "host-"+prefix)`).
- **Reproduced** with only a `w11` rig up: `tools/lab rig down w1` → `LEFTOVER host-w11l0 / host-w11w0 … error: rig down w1 left objects behind (try: rig gc w1)` rc=1; then `tools/lab rig gc w1` → `delete vpp host-w11l0`, `delete vpp host-w11w0`, rc=0. Slot 11's VPP interfaces are gone while its namespaces/veths remain (`rig show w11` → `partial (4/6)`), and the tool's own error text steers slot 1 into doing it.
- Failure scenario: any day slot 1 and any of slots 10–12 both have a rig (a routine `rig down w1` at the end of a test) → w1's teardown fails and the suggested `gc` destroys the other worker's data plane mid-test. Directly violates shared-host-rules §2 ("never touch anything without your prefix") in the tool meant to enforce it; also makes slot 1's smoke test fail spuriously whenever w10–w12 have a rig up.
- Fix: anchor the VPP match to the exact rig names — `grep -E "^host-${P}[lw][0-9]+$"` at `:299` and `:337` (netns `^ns-$P-` and veth `^${P}[lw][0-9]+$` are already correct); in Go, match `name == "host-"+prefix+"l0" || name == "host-"+prefix+"w0"` (or the same regexp). Add a regression check to the smoke test or a bash self-test: with a `w11` rig up, `rig down w1` must report `down` and `rig gc w1` must delete nothing.

### F2 — MEDIUM — `rig up` reports `up` when VPP rejected the address; `VRX_SLOT` silently overrides the prefix-derived slot
- `tools/lab:288`: `vpp_cli_ok set interface ip address "$vif" "$gw/24" >/dev/null || true` swallows every error (intended only for "already assigned" on reuse). `tools/lab:243-245`: an exported `VRX_SLOT` wins over the `w<N>` in the prefix; non-`w` prefixes hash into 100 buckets (collisions are silent).
- **Reproduced**: `VRX_SLOT=77 rig up rva` then `VRX_SLOT=77 rig up rvb` → both print `rig: up`, but `show interface addr` shows `host-rvbl0 (up):` / `host-rvbw0 (up):` with **no L3 address** (VPP refused the overlapping subnet), and ping from `ns-rvb-lan` fails 100 %.
- Failure scenario: a worker with `eval "$(tools/lab env 3)"` in the shell runs `rig up w9` (the prompt's own example) → the w9 rig lands on 10.3.x.x; if a w3 rig exists the second one comes up addressless and every later test fails with confusing "no route" symptoms; the tool told them `rig: up`.
- Fix: (a) for `w<N>` prefixes derive the slot from the prefix and treat a conflicting `VRX_SLOT` as an error (or warn and use the prefix); (b) before `set interface ip address`, check `vppctl show interface addr $vif` for `$gw/24` and skip; otherwise fail the `up` on any error and print VPP's message; (c) in `rig_print` include the addresses actually read back from VPP, not the intended ones.

### F3 — MEDIUM — the handover guard trusts a per-worktree file and an env override
- `tools/lab:96-102` reads `vpp.handover_doc` (default `docs/lab/host-vrx-a.md`) relative to `$ROOT` = the caller's worktree, and `TOPO_DIR` comes from `VRX_TOPOLOGY_DIR` (`:23`).
- **Reproduced (status only, nothing killed)**: `VRX_TOPOLOGY_DIR=<dir with a vrx-a.yml pointing handover_doc at a file saying \`handover: done\`> tools/lab status` → `handover: done … → restart-vpp/kill-vpp allowed for the manager under flock -x`. The same happens if a worker's branch edits its copy of `docs/lab/host-vrx-a.md`.
- Failure scenario: an agent on a branch with a stale/edited doc, or with a test inventory exported for another purpose, runs `restart-vpp vrx-a` "to test restart-safety" and kills the shared VPP (D-012). Fail-safe default (unreadable → pending) is good; the override surface is not.
- Fix: for `restart-vpp`/`kill-vpp` resolve the flag from the canonical repo (`/root/ngfw/docs/lab/host-vrx-a.md`, or `git -C "$ROOT" show main:docs/lab/host-vrx-a.md`) **and** the worktree copy, require both to say `done`, and ignore `handover_doc`/`VRX_TOPOLOGY_DIR` for this decision (or accept only paths under `$ROOT/docs/lab`).

### F4 — MEDIUM — remote `provision --apply` has no "am I provisioning myself?" guard and ignores `role`
- `tools/lab:424-452`: once a yml is `state: active` with an IP in `mgmt`, `--apply` does `echo vm.nr_hugepages=1024 > /etc/sysctl.d/80-vpp.conf`, `apt-get install -y ./*.deb`, `cat > /etc/vpp/startup.conf`, `driverctl set-override`, `systemctl enable --now vpp` on `root@$mgmt` — with no check that `$mgmt` is not this host (172.30.126.195 / 127.x) and no `refuse_if_pending`. `docs/lab/vmware.md:53-54` says host-*/peer-* get "plan only", but `cmd_provision` never looks at `role`, so `--apply` on an active `host-lan` would install VPP and a startup.conf on a traffic host.
- Failure scenario: someone fills `vrx-b.yml` with this host's IP to "try remote mode" → the tool overwrites `/etc/vpp/startup.conf` and `/etc/sysctl.d/80-vpp.conf` on `vrx-a` via ssh-to-self (both forbidden by the handover rule); `systemctl enable --now` is a no-op on a running unit, but the next VPP restart picks up the rendered conf (`dpdk { no-pci }`, `full-coredump`, different log line). Untested code (worker disclosed) makes this more likely, not less.
- Fix: in `run_on`/`cmd_provision` refuse when `mgmt` ∈ `hostname -I` ∪ `127.0.0.0/8` ∪ `::1`; refuse `--apply` unless `role == vrx`; require an explicit `--i-know-this-writes-etc-vpp`-style confirmation or `VRX_LAB_REMOTE_APPLY=1`; keep the dry run as default.

### F5 — LOW — lock windows (post-handover only)
- `tools/lab:459-460`, `:466-467`: `flock -x $VPP_LOCK flock -x $LAB_LOCK systemctl restart|kill …` releases both locks as soon as `systemctl` returns; the 30 s wait for `api.sock` runs unlocked, so shared-lock holders (integration tests) can start against a VPP that is still booting. `rig up|down` themselves never take the shared lock (only `run.sh`/the Go test do), so a bare `rig up` can race a manager restart.
- Fix: run restart/kill **and** the wait loop inside one `flock -x` (`flock -x "$VPP_LOCK" bash -c '…'`); document `tools/lab lock shared tools/lab rig up <p>` for hand use, or take the shared lock inside `cmd_rig up|down`.

### F6 — LOW — smoke module is invisible to the quick gate; two go.sum files to keep in sync
- `test/integration/smoke/go.mod` is its own module (`replace ngfw/agent => ../../../apps/agent`); `tools/ci.sh` only runs `make lint/test/build` in `apps/agent`, so a `gofmt`/`vet`/compile regression in the smoke test is not caught until someone runs it (D-P04-5 is fine, the gap is in the gate). When `apps/agent/go.mod` grows (P05a), `smoke/go.sum` may lack entries and `run.sh` fails with "missing go.sum entry".
- Fix: ask P09 to add `(cd test/integration/smoke && go vet ./... && go test -count=1 ./...)` (skips without `VRX_INTEGRATION`) to `tools/ci.sh`, or add a `go.work` at the repo root; note it in `P04.md` "left undone".

### F7 — LOW — smaller items
- `smoke_test.go:264` `var _ = fmt.Sprintf // keep fmt available` and `:230/:237` `if out, _ := exec.Command(…).Output(); true {` — dead import kept alive and errors from `ip netns list`/`ip -o link` ignored (a failing `ip` makes "no leftovers" vacuously true). Return/propagate the error.
- `tools/lab:288` MTU: VPP af_packet host-interfaces come up with L3 MTU 9000 while the veths are 1500; fine for the smoke ping, but the first test that sends >1500 B through the rig will see silent drops. Set `vppctl set interface mtu packet 1500 $vif` in `rig_side_up` (or document).
- `tools/lab:245` hash-to-slot for non-`w` prefixes: 100 buckets, silent collisions (see F2). Consider refusing non-`w<N>` prefixes without an explicit `VRX_SLOT`.
- `deploy/dev/pg-test.sh:31`: on reuse the role's password is re-set even when the same password was read back from `pg.env` (harmless, message says "refreshed" but it is the same value).
- `P04.md` §2 evidence uses `w3` not the prompt's `w9` ("identical by construction") — acceptable; verified here with `w11`.
- VPP logs `vlib_file_update: epoll_ctl() failed … 'host-<p>l0 queue 0' errno 9` on every `delete host-interface` — VPP af_packet quirk on this build, not P04's doing; worth a line in `docs/lab/host-vrx-a.md` so nobody chases it.

## Scope check (9)
Small additions beyond the prompt: `rig show`, `lock status`, `inventory`, `pg-test.sh dsn|list`, `VRX_VALKEY_DB`/`VRX_LAB_LOCK` in `env`, `binapi/MANIFEST`, `binapi/README.md`. All small, useful, no objection. `apps/agent/go.mod` change is outside the ownership list but unavoidable, minimal, tidy-clean and disclosed (questions #2). `docs/lab/host-vrx-a.md` untouched (correct — not P04-owned; the 6 unassigned NICs are raised in questions #1). Remote `provision --apply` is in the prompt's scope; it is untested and the worker says so.

## Required before merge
1. F1 — anchor the VPP name match in `rig_leftovers`, `rig gc` and `smoke_test.go:leftovers`; re-run: `rig up w11; rig down w1` → `rig: down`, `rig gc w1` deletes nothing; `rig down w11` clean.
2. F2 — fail `rig up` on any `set interface ip address` error other than "already"; prefix-derived slot wins over `VRX_SLOT` for `w<N>`.
3. F3 — guard reads the canonical (main) copy of `host-vrx-a.md` and ignores inventory/env overrides for `restart-vpp`/`kill-vpp`.
4. F4 — self-IP and `role` guards on remote `--apply`.
F5–F7 can follow in the same commit or be noted for P09/the manager.

## Reviewer clean-up
`rig down w11`, `rig down rva/rvb`, `pg-test.sh drop rv11` done; `ip netns list` empty, VPP has only `local0`, `/run/vrx-test/` empty, `vrx-lab.lock` free. VPP pid 1109 untouched throughout.

**BLOCK** — the `gc`/`down` prefix collision (F1) is a reproduced cross-slot destructive bug in the one tool whose contract is "deletes exactly its objects"; fix F1–F4 (small, well-localised) and re-run the three commands above, then this is an approve.
