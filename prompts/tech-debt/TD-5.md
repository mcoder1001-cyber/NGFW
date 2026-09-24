# Task: TD-5 — V24 guard: the agent quiesces the Linux netdev before af_packet_delete   (prepend 00-CONTEXT.md)

## Goal
The agent must never call `af_packet_delete` while the host netdev is up. `af_packet_delete_if` closes the socket fds before `clib_file_del`
(V24, `docs/vpp-code-track.md`). Deleting with the veth up is the suspected cause of the shared-VPP SIGSEGV at 07:27:32 (D-101).

`tools/lab rig down/gc` and P08's topology fixture already bring the veth down first. The agent does not: `Delete` does not, and
`Create`'s rollback path does not (TD-3 re-review **L7**). Make **both** paths bring the Linux netdev down **via netlink** before
`af_packet_delete`, prove the order, and guard it so no third path appears. This is a small task (board est. 2 h) built on **TD-3's merged code**.

## Read first
- `docs/decisions/LOG.md` **D-101** (the crash, the interim rule, and this row), D-095 (V19), D-087 (one host package at a time)
- `docs/vpp-code-track.md` **V24** (`af_packet.c:895-900` close before `af_packet_rx_queue_free` → `clib_file_del_by_index`)
- `docs/status/tasks/TD-3-rereview.md` **L7**, and `docs/status/tasks/TD-3.md` (the ifsanitize wiring, the guard test pattern)
- `apps/agent/internal/descriptors/af_packet/host_interface.go`, as merged by TD-3. The `del` closure passed to `iface.AcquireAndTag` in `Create` is the
  rollback path. `Delete` runs `ifsanitize.BeforeDelete` and then `AfPacketDelete`. First check that these are there (`grep -n AcquireAndTag`). If they are not,
  TD-3 is not merged: stop and write that in the questions file.
- `tools/lab` `rig_side_down` (the shell version of this: `ip link set <veth> down; sleep 0.2`, then delete) and
  `test/topology/interfaces/interfaces_test.go` `rig.peers` (P08's fixture: veths down before `deleteBehindBack`)
- `apps/agent/internal/vpp/ifsanitize/guard_test.go` (TD-3's static-scan guard, the model for yours)
- `/root/vpp/src/plugins/af_packet/af_packet.c:683-692`, read only. `af_packet_create_if` sets `IFF_UP` itself, so nothing has to bring the link back up.

## Scope — build exactly this
1. **Quiesce helper** in the af_packet package (for example `quiesce_linux.go`), behind a small interface so that unit tests inject a fake:
   - Resolve `host_if_name` → ifindex. Then clear `IFF_UP` with a netlink `RTM_NEWLINK` (`ifi_change = IFF_UP`, `ifi_flags = 0`, with ACK), using
     `golang.org/x/sys/unix`. It is already a direct dependency: no new module, and no exec of `ip`, because `host_if_name` is config input (rule 9).
   - Confirm that the link reads down, then wait a short settle (200 ms, the same as `tools/lab`). The settle is injectable, so unit tests do not sleep.
   - The netdev is already gone (ENODEV) → there is nothing to quiesce. Log it and go on: without a device, no packets arrive.
   - The netdev is already down → no-op, go on.
   - Any other failure (EPERM, timeout) → **fail closed**. Do not call `af_packet_delete`. Return an error that names the netdev and D-101/V24.
2. **Delete:** quiesce → `ifsanitize.BeforeDelete` → `af_packet_delete`. Quiescing first means no packet crosses the interface while its bindings are cleared.
3. **Create's rollback** (the `del` closure): quiesce → `af_packet_delete`. When the quiesce fails, the closure returns an error that names the untagged
   orphan it left behind, and does not delete it. The ErrRecreate path goes through Delete, so it is covered too.
4. **Guard test** (af_packet package): a static scan of the non-test Go files under `apps/agent/` fails when `AfPacketDelete(` is sent anywhere other
   than through the quiesce helper. Plant a raw call in a temporary tree to show that the scan catches it, as TD-3's `TestGuardCatchesARawCreate` does.
5. **Restart-simulation fixtures:** grep the whole repo for every path that deletes af_packet interfaces behind the agent's back: `AfPacketDelete`,
   `af_packet_delete`, `delete host-interface`. List each one in TD-5.md with its quiesce status. Today these are `tools/lab` (quiesces), P08's `test/topology/interfaces` (quiesces) and
   this package's `integration_test.go` (it deletes with the veth up: fix it here). You may edit only files you own. For any other file that
   does not quiesce, write it in the questions file.
6. **Docs:** add a "Delete ordering (D-101/V24)" paragraph to `docs/agent/descriptors/af_packet.md`. Note in TD-5.md that the product agent needs
   `CAP_NET_ADMIN` for this; that is P10's systemd unit.

7. **ifsanitize placeholder cap (D-105, TD-3 re-review M1 option b):** replace the fixed `ifsanitize.MaxPlaceholders = 16` with
   `holes + 2 × FreshRun`, capped at 64, where `holes` is the number of freed classify-table indices the run has already seen. Keep the behaviour
   fail-closed at the cap (`ErrCapped` wraps `ErrNoCleanIndex`, quarantine only when a binding is proven unclearable). Unit tests: a free list of
   12 out-of-order indices now succeeds; a list that needs more than 64 still fails closed and bumps `vrx_agent_iface_sanitize_capped_total`.
   Update the cap paragraph in `docs/agent/descriptors/interface.md`. This is the only ifsanitize change you make.

## Acceptance (paste the evidence into docs/status/tasks/TD-5.md)
- [ ] Unit tests with the fake VPP (TD-3's sanitizing fake) and a fake link controller that records the call order:
  - Delete → `link-down(<dev>)` comes before `af_packet_delete`;
  - Create with the tag failing, and Create with the sanitize failing → `link-down` comes before the rollback `af_packet_delete`;
  - quiesce EPERM → no `af_packet_delete`, and the error names D-101;
  - ENODEV and already-down → the delete goes ahead.

  Command: `cd apps/agent && go test -count=1 -v ./internal/descriptors/af_packet/` (pasted).
- [ ] Guard: `TestEveryAfPacketDeleteIsQuiesced` passes on the tree, and the planted raw call is flagged (pasted).
- [ ] Host, on **your slot prefix** only (`vpptest.Prefix`/`vpptest.Name`, so names are `w<slot>…`). The shared VPP, one package, no packets:
  - The test veths get `disable_ipv6=1` on both ends **before** `up`.
  - The host test creates the pair (veth up), then Create through the descriptor, then Delete through the descriptor.
  - After Delete, the netdev reads **down**: `net.InterfaceByName(...).Flags&net.FlagUp == 0`, asserted in the test.
  - Retrieve has no key, and `vppctl show interface` has no `host-w<slot>…`.

  Commands, with the output and NRestarts before and after pasted:
  ```
  eval "$(tools/lab env <slot>)"
  systemctl show vpp -p NRestarts
  cd apps/agent
  VRX_INTEGRATION=1 flock -s /run/lock/vrx-lab.lock go test -count=1 -v -run 'OnHost' ./internal/descriptors/af_packet/
  systemctl show vpp -p NRestarts
  ```
  If NRestarts rises: stop all host runs and write it down.
- [ ] The grep inventory from scope item 5, pasted with a status for each hit
- [ ] `cd apps/agent && make lint test` green, and `tools/ci.sh --base main` green (under the CI lock named in your envelope)

## Out of scope (do not build)
- The VPP fix itself: moving `close()` after the rx-queue free, or `dont_close`. That is C code, and V24 stays in `docs/vpp-code-track.md`. You may append the agent-side
  status to the V24 row, and nothing else.
- Other interface types (tap, memif, bond, vhost). V24 is about af_packet only.
- `ifsanitize` beyond the cap in scope item 7, `descriptors/interface`, `iface.AcquireAndTag` internals, M3 binding readback, and `Release` wiring (TD-3 / tech-debt / P08).
- `tools/lab` (manager-owned) and `test/topology/**` (P08 / features). Read them, do not edit them.
- Bringing the netdev back up after a delete, or on Create (VPP does it at create). The netns-side peers of the rig.
- A deny-list that keeps the management NIC out of af_packet (DF-1 review L3 → schema / F-*), the vrx-agent systemd unit and capabilities (P10), and any
  new Go dependency (vishvananda/netlink and similar).
- Packet-level tests. A test must never demonstrate the crash: it proves the order and the down state, nothing more.

## Open questions to surface, not to decide silently
- Is fail-closed right for the Create-rollback orphan? It leaves an untagged `host-<dev>` behind when the quiesce fails. The alternative would risk the crash.
- Is 200 ms of settle enough when the kernel still has queued frames? Pick the value, state it in TD-5.md, and do not tune it (no performance work).
