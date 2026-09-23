# DF-6 shared descriptor rules (`internal/descriptors/df6`)

Used by gre, ipip, vxlan, vxlan_gpe, gtpu, l2tp, pppoe, sr, sr_mpls, lisp. Mirrors DF-2's helper package; folds into
P05's shared helpers when they land.

- **Builders.** `df6.IfDescriptor` (objects that are VPP interfaces: tunnels, sessions, GTP-U forward entries),
  `df6.BypassDescriptor` / `NewToggleDescriptor` (per-interface features without a dump), `df6.SingletonDescriptor`
  (globals, key `<name>/global`), `df6.KeyedDescriptor` (untagged objects keyed by their own fields: SR, LISP).
- **Ownership.** Interface objects are stamped with the owner tag `vpp.OwnerTag(owner, id)` right after the add (rolled
  back if tagging fails) and Retrieve keeps only interfaces with this owner's tag *and* the id the object itself
  produces. Untagged objects are attributed by `df6.Scope` (slot tables/labels/VNIs `N000–N999`, addresses
  `10.N.0.0/16` + `fdNN::/16`, names `w<N>-`); production uses a nil scope (everything on the VPP is the agent's).
- **Idempotent delete.** No descriptor sends a delete for an object VPP no longer has (interface gone or not tagged
  ours, key absent from the dump): VPP 26.06 has handlers that crash on a failed add/del (V8, gtpu) and several that
  index FIBs with an unchecked `~0`. `df6.RequireTable` checks referenced IP tables (right family) before sending.
- **Keys of other tasks' objects** (`df6/keys.go`): `interface/<name>`, `vrf/<id>` (table 0 → no dependency),
  `bridge-domain/<id>` (DF-1), `mpls-table/<id>` (DF-7).
- **Typed errors.** `ErrPluginNotLoaded` (unknown message → tests skip), `ErrRetrieveUnsupported` (write-only /
  partial descriptors), `ErrNoDelete` (VPP has no delete message), `ErrNoSuchInterface`, `ErrNoSuchTable`,
  `ErrBadMeta`, `ErrBadValue`.
- **Models.** Agent-internal protos next to each package (`model.proto` → `model.pb.go`, `go generate ./internal/descriptors/df6`),
  until P03b adds the leaf messages (D-055).
- **Tests.** Unit: `df6test.FakeVPP` (interface table, tags, loopbacks) + per-plugin stateful fakes that also count
  requests the real VPP would crash on. Host: `df6test.Connect` (VRX_INTEGRATION=1, shared lab lock, slot prefix),
  prefixed fixtures cleaned in `t.Cleanup`, `Host.AssertEmptyPlan` (re-apply → empty plan), `VRX_DF6_HOLD=<s>`
  pauses while objects exist (evidence capture).

## Write-only / partial object types (Retrieve → `ErrRetrieveUnsupported`)

| Object | Reason |
|---|---|
| vxlan.bypass, vxlan-gpe.bypass, gtpu.bypass, l2tp.interface-enable, pppoe.cp | VPP has no dump for the feature |
| l2tp.lookup-key, sr.encap-source, sr.encap-hop-limit | global without getter |
| ipip.sixrd | `ipip_tunnel_dump` lacks the 6RD prefixes |
| sr-mpls.policy, sr-mpls.steering, sr-mpls.endpoint-color | no SR-MPLS dump; `mpls_route_dump` drops the first segment |
| lisp-gpe.fwd-entry | V9: path details sent with a wrong message id |

No-delete: `l2tp.tunnel` (`ErrNoDelete`), `sr-mpls.endpoint-color` (while the policy exists).
