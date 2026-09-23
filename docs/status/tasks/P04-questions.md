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
