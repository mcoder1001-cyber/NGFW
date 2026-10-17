# Review R4 (data plane / VPP safety & shared-host rules) — boot-nic-registration-port

Reviewed head `db5042e38` (base `origin/main` `99a774734`), PR #238. Reviewer: panel R4 (Opus 5.5, read-only agent). The
reviewer handed its report back in chat and did not write this file itself (read-only run); the developer saved it here
unedited (abridged only in the long evidence listing, marked …). Developer answers follow at the end.

## Verdict: BLOCK — 1 MAJOR without an accepted debt row (#1), 4 MINOR (#2–#5), 3 NIT (#6–#8), one [other: R8] note
Becomes APPROVE WITH CHANGES once #1 has (a) the small agent fix merged first, or (b) a manager-accepted `docs/tech-debt.md`
row (owner + date) **and** the two user-doc sentences it falsifies corrected in this branch.

**Summary.** This branch binds no NIC, does not write `startup.conf`, restart VPP or touch the shared host; the diff is API and
docs only. Takeover still needs the admin approve → apply flow, the root executor and `apply-startup.sh` (SHA-bound approval,
handover check, management snapshot, dead-man rollback). The generator still refuses a host-flagged management NIC in
`pciWhitelist`/`devices`; the `management-engine-owned` rule is the right call. The block comes from the agent's "unbound NIC is
only a warning" chain being bypassed by one implicit all-rows consumer, which D-249 makes reachable on existing configurations.

1. **MAJOR — unbound or released physical rows break `allInterfaces` block lists and auto-block enforcement.**
   `apps/agent/internal/desired/global_blocking.go:100,129-131` (`allIfs := sortedKeys(ds.GetInterfaces())` takes every row,
   including those `SplitUnboundPhysical` dropped at `projection.go:241`); `acl.go:640-662` emits `acl.interface-binding/<row>`
   with a MANDATORY dependency on `interface/<row>` (`descriptors/acl/register.go:58-64`); `reconciler.go:522-536` →
   `CodeDependencyMissing`; `autoblock/runtime.go:123` overlays `AllInterfaces: true` whenever auto-block is enabled with live
   entries; `service.go:1385-1389`, `rpc_autoblock.go:111-114`. Scenario: a never-seeded appliance with `security.autoBlock.enabled`
   (e.g. 172.30.110.211, 18 NICs) → first post-upgrade API start registers 17 kernel NICs (correctly only `agent.nic-not-bound`)
   → the next auto-block hit plans bindings for 17 interfaces without alias → plan fails, enforcement stops on every interface,
   every interfaces/security commit is refused while entries exist; releasing does not help (a released row is a permanent
   trigger). If a list or entry exists when registration runs, the registration commit itself is rejected (safe, but registration
   never happens). Contradicts `default-dataplane-nics.md:87` ("the commit succeeds …") and `:128-129` ("a released row … does not
   make commits fail"). Pre-existing on `main` (fresh appliances, any released NIC); widened by D-249. Fix (agent): compute
   `allIfs` from the rows `SplitUnboundPhysical` keeps; projection test (auto-block overlay + one unbound + one released row → no
   binding, no plan issue). Land as a separate small agent PR, or a manager-accepted debt row + the two doc sentences reworded.
   Evidence: code reading only.
2. **MINOR — the agent decides "unbound" by the row's logical name; registration can pick a name the kernel does not have.**
   `seed.service.ts:421` (`device?.name ?? n.name`), `:295-306` (`pciIfName`); `desired/hostnics.go:30` (`lookup(name)`). A saved
   device name `lan` for a NIC still known to the kernel as `ens161` → the agent finds no netdev `lan`, treats the NIC as bound,
   projects alias `lan` → ErrNotFound → the apply fails, at every API start; a DPDK-bound NIC without a saved name gets `enp4s0f0`
   while VPP knows `GigabitEthernet4/0/0` → same; a rollback drops `devices.<pci>.name` so a bound NIC comes back as `enp…`;
   bifurcated drivers (mlx5) keep their netdev (pre-existing). Fails closed for VPP. With iteration 3's `FINAL_PROBLEMS`,
   `apply-failed` retries with backoff — bounded noise. Fix in branch: skip such NICs one at a time (`bound-unnamed`); correct the
   rollback sentence. Debt: decide bound/unbound by `physical.pci`.
3. **MINOR — the saved management set is append-only but the generator demands an exact match** (`seed.service.ts:449-461` vs
   `vppstartup/model.go:529-537`). The branch's own test produces `managementPci ["0000:04:00.0","0000:0b:00.0"]` while the host
   flags only `0b` → the next start-up preview/apply refuses ("does not match the host's management NIC(s)"); no event says so;
   the preview path does not read `NGFW_MGMT_IF/PCI/NGFW_CONTROL_PORTS` (pre-existing gap). Fix: one doc sentence; DEFERRED case.
4. **MINOR — the DEFERRED-ACCEPTANCE row does not name the data-plane evidence**: (a) registration revision APPLIED with
   `agent.nic-not-bound` per new kernel NIC / `agent.nic-released` per released row; (b) VPP unchanged (`sw_interface_dump`,
   `show hardware-interfaces`, no lcp pair, `/sys/bus/pci/devices/<pci>/driver`, `startup.conf` sha256, `vpp.service`
   MainPID/NRestarts); (c) preview shows the new `dev` entries and refuses on a `management-engine-owned` conflict or stale
   `managementPci`; (d) after `apply-startup.sh` the NICs exist under their logical names, `Retrieve()` equals desired, warnings
   clear, management keeps its addresses; (e) block lists / auto-block with an unbound and a released row (#1); (f) saved name ≠
   kernel name (#2); (g) agent restart with registered-but-unbound rows.
5. **MINOR (shared host) — dev and test stacks inherit the caller's environment** (`tools/app:130-132` `env` without `-i`;
   topology stacks). Nothing in the repo sets the flag except the appliance unit; under D-249 an exported `=1` would register the
   shared host's six vmxnet3 NICs into the integrated database (VPP untouched). Fix (not owned here): `tools/app` passes `=0`, or the
   API honours the flag only when `/var/lib/ngfw/firstboot-complete` exists.
6. **NIT** — "registration only creates configuration rows" is imprecise: the system commit is a full agent Apply; a newly
   registered NIC VPP already owns gets `enabled: true` and comes admin-up (harmless without L2/L3 config; say so).
7. **NIT** — HA wording: any primary physical row whose PCI the backup does not whitelist fails `dataplane.owner-consistent` (same
   NIC count with different addresses included); churn only when the primary carries no rows.
8. **NIT** — `pciIfName` docstring / user doc `:18`: systemd omits `f<fn>` for single-function devices and VMware NICs are `ens<slot>`.
- **[other: R8]** `no-management` is retried every 60 s without backoff (DB read + HostNics call per minute on an in-band-managed box).
- Out of scope (pre-existing): PCI renumbering vs the installed startup.conf when a NIC is added (no `ExecStartPre` guard for
  `vpp.service`); multiple netdevs on one PCI function (mlx4, switchdev representors) deduplicated by PCI.

### Answers (abridged)
(1) bind / startup.conf / VPP / shared host: no — read-only `HostNics`, DB reads, one `systemCommit`, events; startup apply only via
admin approve/apply → `apply-executor.py` → `apply-startup.sh`; `firstboot.sh` renders from the static no-pci document; the
unbound-row chain holds at any revision except #1/#2 (both fail closed). (2) management protection: right call; the deciding
protection is at takeover (`model.go:597-600`, `:529-537`; hostile tests pass); cold boot = default route + `NGFW_MGMT_*` + saved PCIs;
control ports {22} only. (3) shared host: no repo path sets the flag; residual risk #5. (4) PCI identity: re-slotted NICs matched by
PCI, `name-in-use` on clash, stale rows must be released; virtio-pci one level; DPDK-bound names diverge (#2, #8). (5) HA: not a
blocker — `/dataplane` node-local is the generator's only input; mismatched merges fail closed; same-layout requirement + debt row
acceptable once worded per #7. (6) deferred row: evidence list in #4.

### Evidence (abridged)
```
$ git diff --name-only origin/main...HEAD -- apps/agent deploy/vpp tools test .github → (empty)
$ git diff origin/main...HEAD -- 'apps/**' 'deploy/**' | grep '^+' | grep -nE 'child_process|execFile|spawn\(|vppctl|cli_inband|systemctl|startup\.conf|apply-startup|/sys/bus|writeFile|unlink|restart' → 3 prose hits
$ grep -rn "driver_override\|drivers_probe\|/unbind\|new_id" apps/agent apps/api/src → (none)
$ tools/ci.sh check --base origin/main → check PASSED (0m10s)
$ (apps/agent) env -u NGFW_INTEGRATION go test -count=1 ./internal/renderers/vppstartup/... ./internal/desired/... → ok · ok
$ go test -run 'Management|HostNICs|Virtio|Mgmt|Blacklist|DPDK|Device' ./internal/renderers/vppstartup/ → PASS (…)
$ go test -run TestHostile ./internal/renderers/vppstartup/ → PASS (host_mgmt_in_devices, _in_whitelist, _by_case, superset, swapped)
$ go test -run 'PhysicalNic|Physical' ./internal/agent/ → PASS (AwaitingDataplane, BoundProjectsNormally, LcpTapIsNotUnbound, UnboundHasNoLcpPair, ReleasedToHost, …)
observed scratchpad/ci-quick-2.log: @ db5042e38 … CI GATE PASSED exit=0 (iteration 3 needs its own gate)
```

## Developer answers (iteration 4)
- #1: option (b) — the agent fix is outside this port's scope (apps/agent) and is proposed as a separate small follow-up; a
  `docs/tech-debt.md` row (owner: manager, proposed Due 2026-10-31) records the fix and the projection test; the two user-doc
  sentences now state the exception (all-interfaces block list / active auto-block → commit refused until bound/narrowed). The
  debt row's acceptance (owner + date) is the manager's call and is flagged in the PR.
- #2: fixed in branch — a kernel NIC whose saved device name differs from its kernel name is skipped (`name-mismatch`) until the
  start-up configuration binds it under that name; a bound NIC without a saved device name is skipped (`bound-unnamed`); no row is
  registered under a name the engine does not have. The golden document loses the `enp29s0f0` row; `pciIfName` is a report/display
  name only. Rollback sentence corrected; debt row for the name-based bound/unbound decision.
- #3: user doc states the exact-match requirement (remove stale addresses before applying); DEFERRED case (c).
- #4: the DEFERRED-ACCEPTANCE row carries the data-plane evidence (a)–(g). #5: debt row. #6: sentence added (admin-up). #7: wording
  corrected in the user doc, the PENDING doc and the debt row. #8: docstring and doc bullet corrected. [other: R8] no-management
  keeps the documented one-minute retry (operator action pending; one DB read + one read-only HostNics call per minute).
