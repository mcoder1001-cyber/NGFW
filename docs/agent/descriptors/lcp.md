# linux-cp descriptors (DF-8, WBS D3.1)

Package `apps/agent/internal/descriptors/lcp` — the linux-cp default namespace and interface pairs, plus the replace
transaction helpers P12 uses. Message names only from `apps/agent/binapi/lcp`. `lcp.Register(registry, client, owner, opts...)`;
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
- Ownership: pairs are owned through the VPP-side interface tag; Create refuses unowned interfaces (never pairs an
  `ens*` NIC or `local0` of another owner). The replace helpers act on **every** pair of every owner: call them only
  when this agent owns the whole VPP; the host test runs begin/end only while no pair exists.
- The default netns is VPP-global: the host test skips when it is set by someone else and unsets it in Cleanup.
