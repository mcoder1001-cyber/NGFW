# DF-6 shared descriptor rules (`internal/descriptors/df6`)

Used by gre, ipip, vxlan, vxlan_gpe, gtpu, l2tp, pppoe, sr, sr_mpls, lisp. Mirrors DF-2's helper package; folds into
P05's shared helpers when they land.

- **Builders.** `df6.IfDescriptor` (objects that are VPP interfaces: tunnels, sessions, GTP-U forward entries),
  `df6.BypassDescriptor` / `NewToggleDescriptor` (per-interface features without a dump), `df6.SingletonDescriptor`
  (globals, key `<name>/global`), `df6.KeyedDescriptor` (untagged objects keyed by their own fields: SR, LISP).
- **Interface references are logical names** (D-065/D-069): resolved with DF-1's one resolver
  (`iface.Table.IndexByName`, via `df6.Interfaces`), which refuses interfaces tagged by another owner
  (`iface.ErrForeignInterface`); Retrieve reports logical names (`iface.Table.Logical`). Dependencies use the alias
  key `interface/<name>`.
- **Ownership (D-071).** Interface objects are stamped with the owner tag `vpp.OwnerTag(owner, id)` right after the
  add (rolled back if tagging fails); Retrieve keeps only interfaces with this owner's tag *and* the id the object
  itself produces. Untagged objects (SR, SR-MPLS, LISP) are ours **only through a ClaimStore record** written after
  our own successful Create (`df6.KeyedDescriptor`; store = DF-1's `iface.Claims(owner)`, P05/P08 install a persisted
  one with `iface.SetClaimStore`, e.g. `df6.OpenFileClaimStore`). Retrieve reports claimed ids only; Create refuses to
  take over an existing unclaimed object (`df6.ErrNotOurs`); Delete never touches an unclaimed one and re-verifies
  identity (key fields beyond the id, e.g. the BSID of a steering entry). Per-interface toggles on untagged interfaces
  claim the interface (`iface.Table.ClaimIfUntagged`).
- **Globals (D-071).** VPP-global settings — `lisp.enable`, `lisp-gpe.enable`, `lisp.pitr`, `sr.encap-source`,
  `sr.encap-hop-limit`, `l2tp.lookup-key`, `pppoe.cp` — are registered as setters only with
  `df6.WithGlobalsOwner(true)`; every other agent gets `df6.RequireDescriptor` (Create checks the value and fails with
  `ErrNotGlobalsOwner`, always for write-only globals; Delete no-op; `DeleteOnAbsence() == false`). The owner's LISP /
  GPE switches are never deleted on absence and are switched off only when `lisp.SafeToDisable` finds no LISP object
  of any owner.
- **Idempotent re-apply (D-063/D-076).** Write-only Creates are safe to repeat on every resync:
  tunnels adopt the interface tagged `<owner>:<id>` instead of adding again (6RD is also deletable after an agent
  restart, located by its tag); keyed write-only types (SR-MPLS, GPE entries) see the object with an exact probe and
  do nothing; feature toggles whose VPP enable stacks (gtpu / vxlan-gpe / vxlan bypass, l2tp decap, pppoe cp) and the
  SR-MPLS endpoint/color record a claim `<name>@vpp-<boot>` and send the enable once per VPP instance. The boot
  identity is the D-080 triple (kernel boot_id, VPP PID from `control_ping`, VPP start time from `/proc/<pid>/stat`
  field 22; `df6.BootID`). Toggle records key on logical name **and** sw_if_index, and every toggle whose feature
  node is known reads VPP's actual state with `feature_is_enabled` (`df6.FeatureProbe`), so an interface recreated on
  the same boot (new or reused index) gets its feature back exactly once. vxlan's handler keeps a per-index bitmap VPP
  never clears on interface delete; `ResetBeforeEnable` sends a (no-op when clear) disable before the enable.
- **Claims expire with the VPP instance (D-080).** Keyed claims (SR, SR-MPLS, LISP) are held by
  `<name>@vpp-<boot>`: after a VPP or host restart none is valid, so a foreign object that reuses an id is never
  reported, adopted, updated or deleted; ours are re-claimed by our own successful Create.
- **Identity-verified deletes (D-071, review M1).** Tunnels: the interface tagged with the object's id is located
  and the dump record at that index must decode to the same id; the delete uses the key fields VPP reports. Toggles:
  the interface is re-resolved by logical name, must match Meta (when known) and still be ours, and only families
  this agent enabled on the running VPP are disabled. A missing object is already deleted: nothing is sent (V8).
- **Keys of other tasks' objects** (`df6/keys.go`): `interface/<name>`, `vrf/<id>` (table 0 → no dependency),
  `bridge-domain/<id>` (DF-1), `mpls-table/<id>` (DF-7).
- **Typed errors.** `ErrPluginNotLoaded` (unknown message → tests skip), `ErrRetrieveUnsupported` (write-only /
  partial descriptors), `ErrNoDelete` (VPP has no delete message), `ErrNoSuchInterface`, `ErrNoSuchTable`,
  `ErrBadMeta`, `ErrBadValue`.
- **Models.** Agent-internal protos next to each package (`model.proto` → `model.pb.go`, `go generate ./internal/descriptors/df6`),
  until P03b adds the leaf messages (D-055).
- **Tests.** Unit: `df6test.FakeVPP` (interface table, tags, loopbacks, `SetBoot` for VPP restarts) + per-plugin
  stateful fakes that count requests the real VPP would crash on and model duplicate adds / stacked features. Host: `df6test.Connect` (VRX_INTEGRATION=1, shared lab lock, slot prefix),
  prefixed fixtures cleaned in `t.Cleanup`, `Host.AssertEmptyPlan` (re-apply → empty plan), `VRX_DF6_HOLD=<s>`
  pauses while objects exist (evidence capture).

## Write-only / partial object types (Retrieve → `ErrRetrieveUnsupported`)

| Object | Reason |
|---|---|
| vxlan.bypass, vxlan-gpe.bypass, gtpu.bypass, l2tp.interface-enable | VPP has no dump for the feature |
| l2tp.lookup-key, sr.encap-source, sr.encap-hop-limit, pppoe.cp | global without getter (globals owner only) |
| ipip.sixrd | `ipip_tunnel_dump` lacks the 6RD prefixes |
| sr-mpls.policy, sr-mpls.steering, sr-mpls.endpoint-color | no SR-MPLS dump; `mpls_route_dump` drops the first segment |
| lisp-gpe.fwd-entry | V9: path details sent with a wrong message id |

No-delete: `l2tp.tunnel` (`ErrNoDelete`). `sr-mpls.endpoint-color` Delete is a documented no-op (cleared with the
policy).

Naming (review L2/L4): tunnel ids double as logical interface names (`interface/<id>`); ids share one namespace per
owner — do not reuse a name across vxlan-gpe / gtpu / 6rd / DF-1 interfaces, and do not name a config object like a
VPP interface (`gre5`). `pppoe.session` ids (`<mac>/<id>`) are not usable as interface references. An agent crash
between add and tag leaves an untagged tunnel that blocks re-creation (instance/tuple in use) and is never adopted;
remove it by hand.
