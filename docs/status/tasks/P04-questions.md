# P04 — questions / observations for the manager (none blocking; work continued)

1. **Six extra vmxnet3 NICs exist on vrx-a** (seen 2026-09-23 13:17, all DOWN, kernel `vmxnet3` driver, not in
   `startup.conf`): `ens161` 0000:04:00.0, `ens193` 0000:0c:00.0, `ens224` 0000:13:00.0, `ens225` 0000:14:00.0,
   `ens256` 0000:1b:00.0, `ens257` 0000:1c:00.0. `docs/lab/host-vrx-a.md` still says "Data-plane NICs: none".
   Recorded in `test/topology/vrx-a.yml` as `segment: unassigned`. Binding them to DPDK needs `dpdk { dev <pci> }` in
   `/etc/vpp/startup.conf` → after handover (D-012). Not touched by P04 (out of scope). Please ask the product owner
   which port groups they are on, then update host-vrx-a.md (not P04-owned).
2. **`apps/agent/go.mod` / `go.sum` changed** (added `go.fd.io/govpp v0.13.0` + transitive deps) — required so the
   generated `apps/agent/binapi/**` compiles (`go build ./...`). Not in P04's ownership list but unavoidable; P05a also
   works in `apps/agent` → expect a trivial additive go.mod merge.
3. **`VRX_METRICS_PORT=91<N>1`** from shared-host-rules §1 is not a valid port for slots 10–12 (91101 > 65535).
   `tools/lab env` computes `9100 + 10·N + 1` (identical for N ≤ 9: 9111 … 9191; 9201/9211/9221 for 10–12). Confirm or amend.
4. **Rig naming vs shared-host-rules §2**: the rules say host-interfaces/veth/netns are named `w<N>-*`; the P04 prompt and
   its acceptance list say `<prefix>l0`/`<prefix>w0` → VPP `host-w9l0`. P04 follows the prompt (`host-<p>l0`, `ns-<p>-lan`);
   suggest wording §2 as "carries the prefix" rather than a literal `w<N>-` pattern.
5. **PostgreSQL is 18.6** (Ubuntu 26.04 archive), not 16 — satisfies "≥ 16". 00-CONTEXT/deploy README said 16;
   cosmetic, but P06 should not assume 16-only behaviour.
6. `vppctl show plugins` lists **84 loaded** (prompt: "94 on disk"); `tools/lab status` counts loaded plugins only.
7. Remote `provision --apply` (vrx-b/c) is implemented but **untested** — no remote VM exists yet (docs/lab/vmware.md).
8. Operator tooling note: `wt.sh pull` uses `rsync --delete` and `push` never deletes — a worker who edits locally
   after running generators on the host (go mod tidy) silently reverts them on push. P04 hit this once (fixed in
   commit 2). Suggest WORKER-OPS say "pull immediately after any host-side generator, push immediately after writing".

Added after the review (`P04-review.md`, fixes in the "Review fixes" section of `P04.md`):

9. **For P09 (review F6):** `test/integration/smoke` is its own Go module and `tools/ci.sh` only runs `make lint/test/build`
   in `apps/agent`, so a gofmt/vet/compile regression in the smoke module stays invisible to the quick gate until someone runs
   `run.sh`. Please add `(cd test/integration/smoke && test -z "$(gofmt -l .)" && go vet ./... && go test -count=1 ./...)` to
   `tools/ci.sh` — without `VRX_INTEGRATION` the VPP test skips and the pure unit test `TestRigObjectMatchIsAnchored` runs (so the
   step is meaningful even on a non-VPP checkout). Alternative: a root `go.work` listing both modules (one `go vet ./...` then covers
   both and the second `go.sum` goes away). P04 did not touch `tools/ci.sh` (not P04-owned).
10. **VPP log quirk — one line for `docs/lab/host-vrx-a.md` (not P04-owned):** every `delete host-interface` logs
    `vlib/file: vlib_file_update: epoll_ctl() failed on epfd (7), file 'host-<p>w0 queue 0' (fd N), errno 9` in `journalctl -u vpp`
    (`/var/log/vpp/vpp.log` does not exist on this host; the unit logs to the journal). af_packet quirk of this build, harmless —
    the interface is gone and the next `create` works. Seen 48× during the review-fix session (every rig down).
11. **`CANON_ROOT=/root/ngfw` is hard-coded in `tools/lab`** (review F3 asked for no env override): `restart-vpp`/`kill-vpp` read
    `docs/lab/host-<vm>.md` from there AND from the calling worktree. If the canonical checkout ever moves, that constant must follow
    (the `git show main:` fallback covers a missing directory, not a moved one). For a future remote VM the guard requires
    `docs/lab/host-<vm>.md` to exist and say `done` in both places (vmware.md checklist step 4) — until then refuse, fail-safe.
12. **`rig up` addressing for non-`w<N>` prefixes now requires `VRX_SLOT`** (D-P04-1 amended, review F2/F7). The reviewer's own
    `rva`/`rvb` style runs need `VRX_SLOT=<13..254>`; `w<N>` prefixes ignore `VRX_SLOT` (warning) — the prefix wins.
