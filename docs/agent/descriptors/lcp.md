# linux-cp descriptors (DF-8, WBS D3.1)

Package `apps/agent/internal/descriptors/lcp` — the linux-cp default namespace and interface pairs, plus the replace
transaction helpers P12 uses. Message names only from `apps/agent/binapi/lcp`. `lcp.Register(registry, client, owner, opts...)` (+ `RegisterGlobals` for the default netns);
option `WithInterfaceKey`.

| Object type | Key | VPP messages | Retrieve | Update | Dependencies |
|---|---|---|---|---|---|
| `lcp.default-netns` (singleton) | `lcp.default-netns/global` | `lcp_default_ns_set`; delete = "" (unset) | `lcp_default_ns_get` while set | in place (existing pairs keep their netns) | — |
| `lcp.itf-pair` | `lcp.itf-pair/<ifname>` (VPP side) | `lcp_itf_pair_add_del_v3` is_add=1 / 0 (delete also removes the host tap) | `lcp_itf_pair_get` (cursor), pairs on owned interfaces | `ErrRecreate` | `interface/<ifname>` (D-065 alias key, mandatory), `lcp.default-netns/global` optional |
| `lcp-replace` | — (helper, not a descriptor) | `lcp_itf_pair_replace_begin` / `_end` → `lcp.ReplaceBegin`, `lcp.ReplaceEnd` | — | — | — |

Value fields: `ItfPair{interface, host_if_name (≤ 15, Linux name characters; tests "w<N>-…"), host_if_type tap|tun,
netns (≤ 31)}`; `DefaultNetns{netns}`. Meta: `PairMeta{PhySwIfIndex, HostSwIfIndex, VifIndex}` (Create and Retrieve
fill it identically).

## Notes and limitations
- Plugin state: `linux_cp` and `linux_nl` are loaded on vrx-a since D-060 and the host test creates a **real pair**
  (loopback `loop586` ↔ tap `w5-lcp0`). Where the plugin is not loaded every call fails with `dfkit.ErrPluginNotLoaded`
  (govpp unknown message) and the host test skips via `CheckCompatiblity` ("plugin not loaded: linux_cp …").
- `linux_nl` has **no binary API** (netlink listener, configured in startup.conf only) — nothing to describe.
- `lcp_itf_pair_get_v2` is not used: for sw_if_index ~0 VPP 26.06 answers it with the **v1** reply id
  (`lcp_api.c`: `REPLY_AND_DETAILS_MACRO_END (VL_API_LCP_ITF_PAIR_GET_REPLY …)` in the v2 handler), which the generated
  v2 client rejects. v1 carries the same details.
- **netns must be the effective namespace:** a pair created with netns "" lands in the default namespace and VPP
  reports that name; desire "" only while no default namespace is set (the API layer fills in the effective value).
- A second, different pair on the same interface is `VALUE_EXIST` (error); an identical re-apply returns the
  existing pair's Meta. VPP refuses a tap for non-ethernet interfaces (use tun).
- Ownership: pairs are owned through the VPP-side interface (logical name): this owner's tagged interfaces, or an
  untagged NIC claimed on Create (P12's DPDK ports); another owner's interface is refused and `local0` never resolves.
  Host tests pair only this slot's loopbacks, never an `ens*` NIC. The replace helpers act on **every** pair of every owner: call them only
  when this agent owns the whole VPP; the host test runs begin/end only while no pair exists.
- The default netns is VPP-global: the host test skips when it is set by someone else and unsets it in Cleanup.

## Registration, ownership and restarts (D-069, D-071, D-074, D-076)
- `lcp.Register(...)` registers the per-owner object types (`lcp.itf-pair`); `lcp.RegisterGlobals(...)` registers the
  VPP-global singletons (`lcp.default-netns`) constructed as **globals owner** — P08 calls it only in the designated globals
  owner's agent (D-071), before `Register`. A descriptor constructed without the role (`dfkit.GlobalsOwner(false)`)
  only *requires* the value: Create succeeds when VPP already has it (checked through the getter where one exists,
  otherwise `dfkit.ErrNotGlobalsOwner`), Delete is a no-op, Retrieve is write-only.
- Interfaces are named by their **logical name** and resolved with DF-1's `iface.ResolveName` (D-069): this owner's
  tag id first, then an untagged interface's VPP name; another owner's interface fails with
  `iface.ErrForeignInterface`, local0 never resolves. Objects on an **untagged** interface (a DPDK NIC) are recorded
  in the owner's ClaimStore (`iface.Claims`, shared with DF-1; P05/P08 install a persisted one) **only after VPP
  accepted the add**, released on Delete, and reported by Retrieve only while claimed (D-071 claim rule). An object that
  already exists on an untagged interface without our claim is **never adopted** (Create fails with
  `dfkit.ErrNotOurs`, nothing is claimed) and Delete never touches it (review H1). Claims are bound to the D-080 VPP
  boot identity (kernel boot_id, VPP main PID, VPP start time — `internal/vpp/bootid` via `dfkit.BootIdentity`) and the sw_if_index, so they
  expire when VPP restarts or the name moves to another interface.
- Deletes re-resolve the logical name right before acting by sw_if_index (never a Meta index — indexes are reused
  after a VPP restart) and first check that the object still exists (D-074); "already gone" is success.
- Retrieve never reports a key twice (`dfkit.Dedupe`).
- Restart simulation (fresh connection + fresh descriptors → empty plan; objects deleted via binapi → exactly their
  re-creation planned → empty plan again): `internal/descriptors/dfkit/restarttest`, output in `DF-8.md`.

## Review fixes (H1, L3, M3, VPP bug)
- An existing pair on an untagged interface without our claim is never adopted — neither a different nor an identical
  one (`dfkit.ErrNotOurs`); host regression: a raw pair on an untagged slot loopback survives Create/Retrieve/Delete.
- L3 (open, P12): the VPP-side host tap (`tapN`) stays untagged; tagging it `<owner>:…` would make DF-1's tapv2
  descriptor treat it as an owned, undesired tap and delete it.
- VPP 26.06 bug: `lcp_default_ns_get` returns uninitialised bytes while no default netns is set (`REPLY_MACRO_DETAILS2`
  does not zero `netns`; host run returned `"\xfd\x11"`); `Current` treats an invalid name as unset.
- Host tests: `default-netns` is opt-in (`VRX_DF8_GLOBALS=1`: a nonexistent default netns would break other slots'
  pairs meanwhile), `replace helpers` is opt-in (`VRX_DF8_LCP_REPLACE=1`: `replace_end` deletes pairs other slots create
  meanwhile); both are unit-tested with the fake.

## P12 wiring (`interfaces.<n>.lcp`)
- The configuration leaf `interfaces.<name>.lcp{hostIfName?, hostIfType = tap, netns?}` (proto `Interface.lcp` 22) is
  projected by `internal/desired/lcp.go` into `lcp.itf-pair/<name>` (depends on `interface/<name>`); the Linux name
  defaults to the VPP name when that is a valid Linux name (else a validation error asks for `hostIfName`). Retrieve
  assembles the leaf back (hostIfName left out when it equals the VPP name, netns when empty).
- Registered in `internal/subsystems/frr.go` (Domains[interfaces]) behind a tap gate: VPP's end of every pair is an
  auto-named tap/tun (`tap4096…`, lcp_interface.c `auto_id_offset 4096`), so while VPP has no such interface Retrieve
  answers from the interface dump alone and never sends `lcp_itf_pair_get` (a VPP without pairs, or without linux_cp).
- TD-11b: the wrapper declares `CheckPersistent()` (pairs on untagged NICs are claimed in DF-1's iface claim store).
- L3 decided: the host tap stays untagged. It is VPP's end of the pair, created and deleted with the pair by
  `lcp_itf_pair_add_del_v3`; no descriptor of this agent owns it separately (DF-1's tapv2 never sees a tagged one).
- The linux-nl facts P12 depends on (VPP 26.06 source): linux_nl's netlink socket opens when the first pair of the whole
  VPP is added, in the lcp default netns of that moment, and closes with the last pair; kernel table T → VPP table T
  (254/255 → 0); FRR routes get FIB source `lcp-rt-dynamic`, kernel/static ones `lcp-rt`; VPP→Linux: the tap carrier
  follows the phy's hardware link, admin state/MTU/addresses only at pair creation unless `lcp lcp-sync` is on.
