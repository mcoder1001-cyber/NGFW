# npt66 descriptor (F-nat44-ei-64-66-nptv6)

> **Ownership, globals (D-071), claims and write-only re-application: see [nat-common.md](nat-common.md).**

Package `apps/agent/internal/descriptors/npt66`, binapi `apps/agent/binapi/npt66` (plugin `npt66_plugin.so`, loaded on
vrx-a since D-060; `default_disabled` in VPP, enabled in startup.conf). Entry point
`npt66.Register(registry, client, owner, natcommon.WithClaims(…))`; projected from `nat.nptv6.bindings[]` by
`internal/desired/nptv6.go`. NPTv6 is RFC 6296: stateless, checksum-neutral IPv6-to-IPv6 prefix translation.

| Descriptor | Key id | Create / Delete | Update | Retrieve | Dependencies | Notes / limitations |
|---|---|---|---|---|---|---|
| `npt66.binding` | `<interface>/<internal prefix>` | `npt66_binding_add_del` (is_add; sw_if_index, internal, external) | same interface and internal prefix, another external prefix: a second add (VPP overwrites the binding in place) | `ErrRetrieveUnsupported` — **write-only** (D-063): VPP 26.06 has no dump | `interface/<name>` (mandatory: the binding is deleted before its interface, D-095c) | One binding per interface (VPP `interface_by_sw_if_index`); prefixes of equal length ≤ /64 (VPP answers INVALID_VALUE above /64); host bits masked. |

**How VPP applies it.** The add enables `npt66-input` (arc `ip6-unicast`) and `npt66-output` (arc `ip6-output`) on the
interface: packets leaving it from the internal prefix get the external prefix as source, packets arriving for the
external prefix get the internal prefix as destination; one 16-bit word is adjusted so that transport checksums stay
valid (RFC 6296 §3.2: the subnet word for a prefix ≤ /48, else a word of the interface id). The binding therefore goes on
the **outside** interface.

**Write-only re-application (D-063/D-076).** The reconciler re-applies every desired binding on each resync. VPP's add is
idempotent — an add on an interface that already has a binding updates it in place and enables the features only for a
new binding (`npt66.c npt66_binding_add_del`: `configure_feature` only when there was none) — so no applied-once record
is needed. Proven by the fake that models the duplicate add (`coretest/npt66.go`, feature-enable counter;
`TestBindingResyncIsIdempotent`, `TestBindingThroughReconciler`: apply + 3 resyncs = 1 binding, features enabled once)
and on the host (`TestNpt66OnHost`: 3 adds → exactly one line in `show npt66 bindings`; the topology test's second agent
restart without loss leaves exactly one).

**Delete.** VPP deletes the interface's binding by sw_if_index (the prefixes are ignored): the interface is resolved again
by name (it still exists — the binding depends on it), the Meta is the fallback; NO_SUCH_ENTRY counts as done.

**Limitations (no dump; V-new in docs/vpp-code-track.md).** Retrieve never reports NPTv6, so `/state/drift` cannot see
it; `GET /api/v1/state/nat/nptv6` shows the running bindings with a write-only marker instead. A binding removed from the
configuration while the agent was down stays in VPP until VPP restarts (the agent forgets what it applied). A binding
whose interface was deleted first — a crash mid-transaction, another owner, a manual delete; the agent itself deletes
the binding first (D-095c) — stays on the freed `sw_if_index`: npt66 has no interface-delete hook, while
`feature.c` clears the interface's features. A later interface that reuses the index then gets no npt66 feature from
its first add (VPP sees an update of the existing binding) until the binding is deleted and added again (review L5).
Evidence on the host: `vppctl show npt66 bindings` (all owners, without the interface).

Tests: `npt66_test.go` (keys, masking, write-only contract, idempotent resync through the reconciler, simulated loss,
validation, foreign interface), `npt66_integration_test.go` (loopback `loop<N>66`, prefixes `fd00:<N>:10::/48 →
fd00:<N>:20::/48`, NRestarts logged before/after), agent level `rpc_nat44_ei_test.go` (restart simulation, delete before
the loopback), topology `test/topology/nat44-ei-64-66-nptv6` (packets through the af_packet rig).
