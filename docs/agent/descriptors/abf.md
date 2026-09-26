# abf descriptors (DF-2, WBS D2.4 ACL-based forwarding)

Package `apps/agent/internal/descriptors/abf`. Messages from `apps/agent/binapi/abf` (+ `binapi/acl` to resolve ACL names, `fib_types` paths).

| Object | Descriptor / key | Create / Update / Delete | Retrieve | Dependencies | Notes |
|---|---|---|---|---|---|
| policy | `abf.policy` / `abf.policy/<policy_id>` | `abf_policy_add_del` (policy_id, acl_index, paths); Update: path changes in place (the API is additive: add new paths first, then remove stale ones); ACL change → `ErrRecreate` | `abf_policy_dump` (acl_index → ACL name via DF-4 `acl.LookupIndex` for Create; `acl_dump` tags with DF-4's duplicate rule for Retrieve, paths decoded to `df2.FibPath` with interface names) | `acl.acl/<name>` (DF-4 key, mandatory), path interfaces (Optional) | No tag: owned by policy id in the agent's `df2.IDRange` **and** an ACL of this owner. Meta `{ACLIndex}`. Pass desired through `NormalizePolicy` (path defaults, sort). |
| interface attach | `abf.attach` / `abf.attach/<policy_id>/<ifname>/<ipv4\|ipv6>` | `abf_itf_attach_add_del` (policy, sw_if_index, priority, is_ipv6); Update → `ErrRecreate` | `abf_itf_attach_dump`, owned policy id + owner-tagged interface | `abf.policy/<id>`, `interface/<ifname>` | Meta `{SwIfIndex}`. |

ACL key contract (D-066, DF-4 merged): dependency `acl.KeyACL(name)` = `acl.acl/<name>`, VPP tag `"<owner>:<name>"`. Create resolves the index with `acl.LookupIndex` (lowest index canonical when a tag is duplicated). Retrieve reports a policy on a non-canonical duplicate as `acl: "<name>#<index>"`, so it diffs and is recreated on the canonical ACL — the one DF-4 keeps. Host tests create the ACL through `binapi/acl` with that tag (`df2test.ACL`).
`abf_plugin_get_version` = health check only. CLI: `show abf policy`, `show abf attach <if>`.

## Ownership on untagged interfaces (review H3)
Objects on interfaces tagged `"<owner>:<name>"` are ours. Physical ports (DPDK NICs) carry no tag: an object created on
an untagged interface is recorded by its **key** in the claim store (`df2.WithClaims(store)`, DF-4's `acl.ClaimStore`
interface; `df2.FileClaimStore` persists it in the agent state dir) and Retrieve reports it only while claimed. Interfaces
tagged by another owner — and `local0` — are refused with `df2.ErrForeignInterface`. Dependencies use the interface
alias key `interface/<name>` (D-065, DF-1).

## Deletes re-verify identity (D-071, fix round 2)
A Delete that acts on a stored `sw_if_index` first re-dumps the interfaces (`df2.SkipDelete`): the index must still name the
object's interface and that interface must still be ours (own tag, or untagged and the key claimed). Otherwise nothing is
sent to VPP — our object went with the interface, or the index now belongs to someone else — and only the claim is dropped.

`abf.policy` Delete re-verifies right before removing the paths: the policy id must still exist, be in the agent's id range,
carry the ACL index the policy was created with, and that ACL must still carry this owner's tag; otherwise it is left alone.

## Product wiring (F-rpf-adl-pbr)
- Registered by `subsystems/rpf_adl_pbr.go` with the persisted "acl" claim store and the policy-id range `SlotIDRange()`
  (the slot's `VRX_VPP_TABLE_BASE … +999` on the shared host, nil = every id in the product agent).
- `routing.pbr.policies.<name>` → `abf.policy/<id>`: the id is FNV-1a of the name into the range, linear probing in name
  order (`desired.PolicyIDs`). VPP keeps no name, so the agent-local `pbr.policy/<name>` record (persisted
  `<state dir>/pbr-<owner>.json`, depends on its `abf.policy`) maps names back to ids for Retrieve and keeps the policy's
  priority (VPP has priorities only on attachments: `abf.attach` gets the policy's priority).
- Until F-acl registers DF-4's `acl.acl`, the observe-only `pbr.acl-ref` descriptor resolves the policy's mandatory
  `acl.acl/<name>` dependency from the owner-tagged ACLs in VPP (KeyProvider aliases); it switches itself off once
  `acl.acl` is registered.
- V23 (b): ABF attachments are not sanitized on interface delete (no crash path known) — left as is.
